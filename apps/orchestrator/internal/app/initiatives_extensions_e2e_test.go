package app

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/butler/butler/apps/orchestrator/internal/api"
	approvals "github.com/butler/butler/apps/orchestrator/internal/approval"
	"github.com/butler/butler/apps/orchestrator/internal/goal"
	"github.com/butler/butler/apps/orchestrator/internal/initiative"
	durableinput "github.com/butler/butler/apps/orchestrator/internal/input"
	runservice "github.com/butler/butler/apps/orchestrator/internal/run"
	"github.com/butler/butler/apps/orchestrator/internal/session"
	"github.com/butler/butler/apps/orchestrator/internal/task"
	"github.com/butler/butler/apps/orchestrator/internal/tools"
	"github.com/butler/butler/internal/actionauth"
	"github.com/butler/butler/internal/budget"
	"github.com/butler/butler/internal/extensions"
	"github.com/butler/butler/internal/extensions/projectfixture"
	commonv1 "github.com/butler/butler/internal/gen/common/v1"
	runv1 "github.com/butler/butler/internal/gen/run/v1"
	sessionv1 "github.com/butler/butler/internal/gen/session/v1"
	toolbrokerv1 "github.com/butler/butler/internal/gen/toolbroker/v1"
	"github.com/butler/butler/internal/memory/transcript"
	"github.com/butler/butler/internal/owneridentity"
	"github.com/butler/butler/internal/ownership"
	"github.com/butler/butler/internal/security/egress/egresstest"
	"github.com/jackc/pgx/v5"
)

// The public-origin recipient, exact owner approvals, durable Broker journal,
// and isolated runtime are real processes. Model planning is deliberately not
// simulated as verified completion: the owner verifies the checked artifact.
func TestInitiativeProjectChecksRealBrokerE2E(t *testing.T) {
	if os.Getenv("BUTLER_EXTENSION_E2E") != "1" {
		t.Skip("isolated real-process initiative acceptance")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()
	pool, dsn := e2eDatabase(t)
	sessions := session.NewPostgresRepository(pool)
	const sessionKey, owner = "web:initiative-fixture", "owner"
	if _, _, err := sessions.CreateSession(ctx, session.CreateSessionParams{SessionKey: sessionKey, UserID: owner, Channel: "web", MetadataJSON: "{}"}); err != nil {
		t.Fatal(err)
	}
	actor := func(id string) context.Context {
		return owneridentity.WithActor(ctx, owneridentity.Actor{OwnerID: owner, Channel: "web", DeliveryID: id})
	}
	runs := runservice.NewService(runservice.NewPostgresRepository(pool), nil)
	leases := session.NewPostgresLeaseManager(pool)
	policy := budget.DefaultPolicy()
	policy.Models["fixture:fixture"] = budget.ModelPrice{Basis: "subscription_no_overage"}
	controller := budget.NewController(pool, policy)
	tasks := task.NewStore(pool)
	tasks.SetBudgets(controller, "")
	goals := goal.New(pool, tasks, nil)
	repo := approvals.NewPostgresRepository(pool)
	repo.SetCheckpointHooks(boundaryCheckpointHooks{})
	approvalService := approvals.NewService(repo, nil)
	coreToken, runtimeToken, ownerToken := strings.Repeat("core-test-", 6), strings.Repeat("runtime-test-", 6), strings.Repeat("owner-test-", 6)
	reader := &actionContextReader{runs: runs, sessions: sessions, leases: leases, tasks: tasks, approvals: approvalService, pool: pool}
	authority := httptest.NewServer(actionauth.NewContextHandler(coreToken, reader))
	defer authority.Close()
	fixture, err := projectfixture.New(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer fixture.Close()
	site := httptest.NewServer(fixture)
	defer site.Close()
	runtimeAddress, brokerAddress, brokerHTTP := freeAddress(t), freeAddress(t), freeAddress(t)
	runtimeBinary := os.Getenv("BUTLER_EXTENSION_E2E_BINARY")
	data, err := os.ReadFile(runtimeBinary)
	if err != nil {
		t.Fatal("fixture runtime artifact unavailable")
	}
	sum := sha256.Sum256(data)
	p, err := projectfixture.Manifest(hex.EncodeToString(sum[:]), "reviewed-initiative-fixture-v1")
	if err != nil {
		t.Fatal(err)
	}
	profileJSON, _ := json.Marshal(map[string]extensions.Profile{"reference-http": {Target: runtimeAddress, Dependency: p.Manifest.Dependencies[0]}})
	file := func(name, value string) string {
		path := filepath.Join(t.TempDir(), name)
		if err := os.WriteFile(path, []byte(value), 0600); err != nil {
			t.Fatal(err)
		}
		return path
	}
	coreFile, runtimeFile := file("core.token", coreToken), file("runtime.token", runtimeToken)
	extensionChild(t, runtimeBinary, runtimeAddress, "127.0.0.1:0", []string{"-test.run=^TestExtensionRuntimeEgressHarness$"}, "BUTLER_EXTENSION_EGRESS_HARNESS=1", "BUTLER_EXTENSION_FIXTURE_ENDPOINT="+site.URL, "BUTLER_EXTENSION_PROFILES_JSON="+string(profileJSON), "BUTLER_EXTENSION_RUNTIME_TOKEN_FILE="+runtimeFile)
	extensionChild(t, os.Getenv("BUTLER_BROKER_E2E_BINARY"), brokerAddress, brokerHTTP, nil, "BUTLER_POSTGRES_URL="+dsn, "BUTLER_TOOL_REGISTRY_PATH=configs/tools.json", "BUTLER_ACTION_CONTEXT_URL="+authority.URL, "BUTLER_ACTION_AUTHORITY_TOKEN_FILE="+coreFile, "BUTLER_EXTENSION_PROFILES_JSON="+string(profileJSON), "BUTLER_EXTENSION_RUNTIME_TOKEN_FILE="+runtimeFile, "BUTLER_OWNER_CONTROL_OWNER_ID="+owner)
	controlServer := httptest.NewServer(api.ReviewedExtensions(api.NewOwnerControlAuth(ownerToken, owner), extensions.NewControlClient("http://"+brokerHTTP+"/internal/extensions", coreToken)))
	defer controlServer.Close()
	controlSequence := 0
	control := func(req extensions.ControlRequest) extensions.ControlResponse {
		t.Helper()
		controlSequence++
		body, _ := json.Marshal(req)
		request, _ := http.NewRequestWithContext(ctx, "POST", controlServer.URL, strings.NewReader(string(body)))
		request.Header.Set("Authorization", "Bearer "+ownerToken)
		request.Header.Set("X-Butler-Delivery-ID", fmt.Sprintf("initiative-extension-%d", controlSequence))
		response, err := http.DefaultClient.Do(request)
		if err != nil {
			t.Fatal(err)
		}
		defer response.Body.Close()
		var out extensions.ControlResponse
		if json.NewDecoder(response.Body).Decode(&out) != nil || response.StatusCode != 200 || out.Error != "" {
			t.Fatalf("owner operation %s failed: %s", req.Operation, out.Error)
		}
		return out
	}
	control(extensions.ControlRequest{Operation: "stage", ManifestJSON: p.Raw})
	control(extensions.ControlRequest{Operation: "review", ExtensionID: "project", ManifestHash: p.Digest})
	state := control(extensions.ControlRequest{Operation: "list"}).Items[0].Record
	selected := []string{"extension.project.inspect", "extension.project.prepare-patch", "extension.project.check"}
	state = control(extensions.ControlRequest{Operation: "configure", ExtensionID: "project", ManifestHash: p.Digest, Generation: state.Generation, Configuration: map[string]string{"origin": egresstest.Origin}, Tools: selected}).Item.Record
	control(extensions.ControlRequest{Operation: "probe", ExtensionID: "project", Generation: state.Generation})
	state = control(extensions.ControlRequest{Operation: "enable", ExtensionID: "project", Generation: state.Generation}).Item.Record
	now := time.Now().UTC()
	goalResponse, err := goals.PutGoal(actor("fixture-goal"), goal.GoalRequest{Goal: actionauth.GoalPolicy{Kind: "objective", Status: "active", Title: "Maintain one synthetic project", Purpose: "fixture_maintenance", ExpiresAt: now.Add(48 * time.Hour)}})
	if err != nil {
		t.Fatal(err)
	}
	u := policy.Mandate
	mandateResponse, err := goals.PutMandate(actor("fixture-mandate"), goal.MandateRequest{Mandate: actionauth.Mandate{GoalID: goalResponse.SubjectID, GoalRevision: goalResponse.Revision, Status: "active", Purpose: "fixture_maintenance", Tools: selected, Extensions: []actionauth.ExtensionPermission{{Source: p.Manifest.Source.URI, ExtensionID: p.Manifest.ID, ManifestHash: p.Digest, Tools: selected}}, ResourceScopes: []actionauth.ResourceScope{{Origin: egresstest.Origin, PathPrefix: "/project"}}, Effects: []actionauth.Effect{actionauth.EffectRead, actionauth.EffectCommit}, Privacy: "cloud_allowed", Budget: actionauth.MandateBudget{ModelTurns: u.ModelTurns, Tokens: u.Tokens, CostMicros: u.CostMicros, ExternalCalls: u.ExternalCalls, Children: u.Children, ActiveMillis: u.ActiveMillis, EgressBytes: u.EgressBytes, ProviderInputBytes: u.ProviderInputBytes}, ExpiresAt: now.Add(24 * time.Hour), AllowInitiative: true, Intervention: []string{"effect_unknown", "budget_exhausted"}, Reporting: "exceptions"}})
	if err != nil {
		t.Fatal(err)
	}
	store := initiative.NewStore(pool)
	// This recipient has only synthetic public data, with no private source
	// lifecycle records. Separate catalog tests cover revoked-source rejection.
	store.RunDataGuardTx = func(context.Context, pgx.Tx, string) error { return nil }
	observer := initiative.ProjectObserver{Store: store, RunDataGuard: func(context.Context, string) error { return nil }}
	source := initiative.Source{ID: "project-fixture-checks", OwnerID: owner, GoalID: goalResponse.SubjectID, MandateID: mandateResponse.SubjectID, SessionKey: sessionKey, ProjectRef: projectfixture.ProjectRef, ProjectRevision: projectfixture.Revision(), ObserverKind: "broker_project", CheckManifestHash: p.Digest, CheckToolName: "extension.project.inspect", CheckResource: egresstest.Origin + "/project", DigestChannel: "web", Purpose: "fixture_maintenance", TimeZone: "UTC", TaskGoal: "Prepare the exact synthetic README correction and check the stored patch, leaving the project unapplied", ExecutionPolicy: "normal", CompletionCriteria: []string{"stored prepared patch checks pass", "original project remains unapplied"}, ActiveStart: now.Hour(), ActiveEnd: (now.Hour() + 23) % 24, FailureThreshold: 3, ConfidenceThreshold: 90, MinimumImportance: 60, CooldownSeconds: 3600, Enabled: true}
	if _, err = store.SaveSource(actor("fixture-source"), source, 0); err != nil {
		t.Fatal(err)
	}
	source.Revision = 1
	promoter := &initiative.Promoter{Store: store, Goals: goals, Tasks: tasks, Inputs: durableinput.NewStore(pool), Budgets: controller, Allowance: budget.ModelAllowance{Provider: "fixture", Model: "fixture", InputTokens: 1024, OutputTokens: 8192}}
	client, err := tools.DialWithTokenFile(brokerAddress, coreFile)
	if err != nil {
		t.Fatal(err)
	}
	defer client.Close()
	transcripts := transcript.NewStore(pool)
	sequence := 0
	var owned context.Context
	var runID string
	var lease session.LeaseRecord
	start := func(record task.Record) {
		t.Helper()
		sequence++
		run, err := runs.CreateRun(ctx, &sessionv1.CreateRunRequest{SessionKey: sessionKey, InputEvent: &runv1.InputEvent{EventId: fmt.Sprintf("initiative-run-%d", sequence)}, AutonomyMode: commonv1.AutonomyMode_AUTONOMY_MODE_2, ModelProvider: "fixture", MetadataJson: "{}"})
		if err != nil {
			t.Fatal(err)
		}
		runID = run.GetRunId()
		lease, err = leases.AcquireLease(ctx, session.AcquireLeaseParams{SessionKey: sessionKey, RunID: runID, OwnerID: "worker", TTL: 3 * time.Minute})
		if err != nil {
			t.Fatal(err)
		}
		if _, err = pool.Exec(ctx, `UPDATE runs SET status='tool_pending',current_state='tool_pending',lease_id=$2 WHERE run_id=$1`, runID, lease.LeaseID); err != nil {
			t.Fatal(err)
		}
		if err = tasks.AttachRun(ctx, record.TaskID, record.Revision, runID, sessionKey); err != nil {
			t.Fatal(err)
		}
		owned = ownership.WithToken(ctx, leases.Authority(), ownership.Token{SessionKey: sessionKey, RunID: runID, LeaseID: lease.LeaseID, FenceEpoch: lease.FenceEpoch})
		control(extensions.ControlRequest{Operation: "grant_task", ExtensionID: "project", Access: extensions.TaskAccess{TaskID: record.TaskID, TaskRevision: record.Revision, ConstraintsRevision: record.ConstraintsRevision, ManifestHash: p.Digest, Generation: state.Generation, Tools: selected, Resources: []actionauth.ResourceScope{{Origin: egresstest.Origin, PathPrefix: "/project"}}, ExpiresAt: now.Add(time.Hour)}})
	}
	finish := func() {
		t.Helper()
		if _, err := pool.Exec(ctx, `UPDATE runs SET status='completed',current_state='completed',finished_at=clock_timestamp() WHERE run_id=$1`, runID); err != nil {
			t.Fatal(err)
		}
		if _, err := leases.ReleaseLease(ctx, lease.LeaseID); err != nil {
			t.Fatal(err)
		}
	}
	dispatch := func(name, path string, body any) (map[string]any, string) {
		t.Helper()
		sequence++
		args := map[string]any{"url": egresstest.Origin + path}
		if body != nil {
			args["body"] = body
		}
		raw, _ := json.Marshal(args)
		call := &toolbrokerv1.ToolCall{RunId: runID, ToolCallId: fmt.Sprintf("initiative-call-%d", sequence), ToolName: name, ArgsJson: string(raw), AutonomyMode: commonv1.AutonomyMode_AUTONOMY_MODE_2}
		started := time.Now().UTC()
		prepared, err := client.PrepareAction(ctx, call)
		if err != nil {
			t.Fatal(err)
		}
		expiry := prepared.ExpiresAt
		approval, err := approvalService.CreatePendingApproval(owned, approvals.CreatePendingParams{RunID: runID, SessionKey: sessionKey, ToolCallID: call.ToolCallId, ToolName: name, ArgsJSON: call.ArgsJson, RequestedVia: "web", ActionBinding: prepared.Binding, ContinuationJSON: `{"version":1}`, Summary: prepared.Summary, ExpiresAt: &expiry})
		if err != nil {
			t.Fatal(err)
		}
		approvalActor := approvals.WithActor(ctx, approvals.Actor{OwnerID: owner, Channel: "web", SessionKey: sessionKey, DeliveryID: "approve-" + call.ToolCallId})
		if _, _, err = approvalService.ResolveByApprovalID(approvalActor, approvals.ResolveByApprovalIDParams{ApprovalID: approval.ApprovalID, Approved: true, ResolvedVia: "web", ActorType: "web", ActorID: owner, ResolvedBy: owner}); err != nil {
			t.Fatal(err)
		}
		grant, err := client.AuthorizeAction(ctx, call, prepared.Binding.ActionID, approval.ApprovalID, "")
		if err != nil {
			t.Fatal(err)
		}
		result, err := client.ExecuteAuthorizedToolCall(ctx, call, grant)
		if err != nil || result.GetStatus() != "completed" || result.GetEffectState() != string(actionauth.StateConfirmed) || len(result.GetEvidenceRefs()) != 1 {
			t.Fatal("real Broker did not confirm exact recipient observation/effect", err)
		}
		finished := time.Now().UTC()
		if _, err = transcripts.AppendToolCall(owned, transcript.ToolCall{ToolCallID: call.ToolCallId, RunID: runID, ToolName: name, ArgsJSON: call.ArgsJson, Status: "completed", RuntimeTarget: runtimeAddress, StartedAt: started, FinishedAt: &finished, ResultJSON: result.GetResultJson()}); err != nil {
			t.Fatal(err)
		}
		var envelope struct{ Body string }
		var value map[string]any
		if json.Unmarshal([]byte(result.GetResultJson()), &envelope) != nil || json.Unmarshal([]byte(envelope.Body), &value) != nil {
			t.Fatal("invalid recipient observation")
		}
		if _, err = pool.Exec(ctx, `UPDATE runs SET status='tool_pending',current_state='tool_pending' WHERE run_id=$1`, runID); err != nil {
			t.Fatal(err)
		}
		return value, call.ToolCallId
	}
	observe := func(id string) initiative.Candidate {
		t.Helper()
		event, err := observer.Check(ctx, source, initiative.ClaimedTick{})
		if err != nil || event == nil {
			t.Fatal("real Broker observation missing", err)
		}
		event.ID, event.SourceID = id, source.ID
		candidate, _, err := store.Observe(ctx, owner, *event, source.Revision, time.Now().UTC())
		if err != nil {
			t.Fatal(err)
		}
		return candidate
	}
	var candidate initiative.Candidate
	for i := 1; i <= 3; i++ {
		record, err := goals.CreateOwnerTask(actor(fmt.Sprintf("owner-check-%d", i)), goal.TaskRequest{SessionKey: sessionKey, GoalID: source.GoalID, MandateID: source.MandateID, Purpose: source.Purpose, ProjectRef: source.ProjectRef, ProjectRevision: source.ProjectRevision, Goal: "Inspect the synthetic project", ObservationRef: "owner:fixture-check", InitiativeReason: "Owner requests a bounded project check", CompletionCriteria: []string{"fixed project check observed"}})
		if err != nil {
			t.Fatal(err)
		}
		start(record)
		checked, _ := dispatch("extension.project.inspect", "/project", nil)
		if checked["checks_passed"] != false {
			t.Fatal("fixture unexpectedly passed")
		}
		if i == 1 {
			dispatch("extension.project.inspect", "/project", nil)
		}
		finish()
		candidate = observe(fmt.Sprintf("observed-check-%d", i))
		if candidate.Occurrences != i {
			t.Fatal("calls in one attempt were counted as separate failures")
		}
	}
	if candidate.Status != "ready" {
		t.Fatal("three distinct real project checks did not qualify")
	}
	candidate, err = promoter.Promote(ctx, candidate.ID, owner)
	if err != nil || candidate.Status != "promoted" || candidate.TaskID == "" || candidate.InputID == "" {
		t.Fatal("ordinary initiative admission failed", err)
	}
	record, err := tasks.Get(ctx, candidate.TaskID)
	if err != nil {
		t.Fatal(err)
	}
	start(record)
	dispatch("extension.project.inspect", "/project", nil)
	ownCandidate := observe("own-initiative-observation")
	if ownCandidate.Reason != "own_change" {
		t.Fatal("own task observation generated recursive work")
	}
	patch, patchCall := dispatch("extension.project.prepare-patch", "/project/patches", projectfixture.Proposal{ProjectRevision: projectfixture.Revision(), Changes: []projectfixture.Change{{Path: "README.md", Before: projectfixture.Original, After: projectfixture.Prepared}}})
	artifact, _ := patch["artifact_id"].(string)
	if artifact == "" {
		t.Fatal("immutable recipient patch absent")
	}
	checked, checkCall := dispatch("extension.project.check", "/project/patches/"+artifact+"/checks", nil)
	if checked["checks_passed"] != true || checked["applied"] != false {
		t.Fatal("stored prepared correction lacks objective verification")
	}
	if err = store.RefreshCompleted(ctx); err != nil {
		t.Fatal(err)
	}
	current, err := store.Get(ctx, candidate.ID, owner)
	if err != nil || current.Status != "promoted" {
		t.Fatal("tool success falsely marked task completion", err)
	}
	verified, err := tasks.SaveVerifiedResult(actor("verify-prepared-artifact"), task.Result{TaskID: record.TaskID, TaskRevision: record.Revision, RunID: runID, Summary: "Recipient stored and checked the exact synthetic patch; project unapplied", EvidenceRefs: []string{"tool:" + patchCall, "tool:" + checkCall, "recipient-artifact:" + artifact}, VerifiedCriteria: record.CompletionCriteria, Verifier: "owner"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err = tasks.Transition(ctx, record.TaskID, record.Revision, "completed", verified.ResultID); err != nil {
		t.Fatal(err)
	}
	finish()
	if err = store.RefreshCompleted(ctx); err != nil {
		t.Fatal(err)
	}
	current, err = store.Get(ctx, candidate.ID, owner)
	if err != nil || current.Status != "completed" || current.Reason != "verified_task_result" {
		t.Fatal("verified prepared result did not finish initiative", err)
	}
	for i := 0; i < 2; i++ {
		if err = store.QueueDigests(ctx); err != nil {
			t.Fatal(err)
		}
	}
	var count int
	if err = pool.QueryRow(ctx, `SELECT count(*) FROM initiative_digest_outbox`).Scan(&count); err != nil || count != 1 {
		t.Fatal("quiet grouped notification was duplicated", err)
	}
}
