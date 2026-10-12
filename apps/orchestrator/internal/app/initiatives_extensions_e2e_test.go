package app

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/butler/butler/apps/orchestrator/internal/api"
	approvals "github.com/butler/butler/apps/orchestrator/internal/approval"
	"github.com/butler/butler/apps/orchestrator/internal/channel/telegram"
	"github.com/butler/butler/apps/orchestrator/internal/completion"
	"github.com/butler/butler/apps/orchestrator/internal/deliveryoutbox"
	"github.com/butler/butler/apps/orchestrator/internal/goal"
	"github.com/butler/butler/apps/orchestrator/internal/initiative"
	durableinput "github.com/butler/butler/apps/orchestrator/internal/input"
	flow "github.com/butler/butler/apps/orchestrator/internal/orchestrator"
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
	"github.com/butler/butler/internal/ingress"
	"github.com/butler/butler/internal/memory/catalog"
	"github.com/butler/butler/internal/memory/transcript"
	"github.com/butler/butler/internal/owneridentity"
	"github.com/butler/butler/internal/ownership"
	"github.com/butler/butler/internal/security/egress/egresstest"
	"github.com/butler/butler/internal/transport"
	"github.com/jackc/pgx/v5/pgxpool"
)

	// The public-origin recipient, exact owner approvals, durable Broker journal,
// and isolated runtime are real processes. Model planning is deliberately not
// simulated as verified completion: the owner verifies the checked artifact.
func TestInitiativeProjectChecksRealBrokerE2E(t *testing.T) {
	provider := &initiativeProjectScriptedProvider{}
	runInitiativeProjectAcceptance(t, func(context.Context, *pgxpool.Pool, string) (transport.ModelProvider, error) { return provider, nil }, provider.Name(), "fixture")
	if provider.entries.Load() != 4 {
		t.Fatal("ordinary initiative did not use exact four scripted model turns")
	}
}

// The injected provider owns only the promoted task's four model turns. Three
// inexpensive recorded checks below use actual exact-approved Broker actions.
// Native factories are composed by a separately reviewed, sealed caller.
func runInitiativeProjectAcceptance(t *testing.T, factory func(context.Context, *pgxpool.Pool, string) (transport.ModelProvider, error), providerName, modelName string) {
	t.Helper()
	if os.Getenv("BUTLER_EXTENSION_E2E") != "1" {
		t.Skip("isolated real-process initiative acceptance")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Minute)
	defer cancel()
	pool, dsn := e2eDatabase(t)
	manifestPath := filepath.Join(t.TempDir(), "deletions.log")
	manifest := catalog.NewFileManifest(manifestPath)
	if err := manifest.Initialize(); err != nil {
		t.Fatal(err)
	}
	sourceCatalog := catalog.New(pool, manifest)
	if err := sourceCatalog.Reconcile(ctx); err != nil {
		t.Fatal(err)
	}
	sessions := session.NewPostgresRepository(pool)
	const sessionKey, owner = "telegram:chat:424242", "owner"
	if _, _, err := sessions.CreateSession(ctx, session.CreateSessionParams{SessionKey: sessionKey, UserID: owner, Channel: "telegram", MetadataJSON: "{}"}); err != nil {
		t.Fatal(err)
	}
	actor := func(id string) context.Context {
		return owneridentity.WithActor(ctx, owneridentity.Actor{OwnerID: owner, Channel: "web", DeliveryID: id})
	}
	runs := runservice.NewService(runservice.NewPostgresRepository(pool), nil)
	leases := session.NewPostgresLeaseManager(pool)
	policy := budget.DefaultPolicy()
	policy.Models[providerName+":"+modelName] = budget.ModelPrice{Basis: "subscription_no_overage"}
	policy.Task.ModelTurns, policy.Task.Tokens, policy.Task.ActiveMillis = 4, 8*1048576, 600000
	policy.Mandate.ModelTurns, policy.Mandate.Tokens, policy.Mandate.ActiveMillis = 8, 16*1048576, 1200000
	policy.Day.ModelTurns, policy.Day.Tokens, policy.Day.ActiveMillis = 8, 16*1048576, 1200000
	policy.Conversation = policy.Task
	policy.ModelMillis, policy.TaskLifetimeMillis = 120000, 1800000
	policy.ToolMillis, policy.ModelInputBytes = 30000, 131072
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
	projectDirectory := t.TempDir()
	fixture, err := projectfixture.New(projectDirectory)
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
	policyJSON, _ := json.Marshal(policy)
	extensionChild(t, os.Getenv("BUTLER_BROKER_E2E_BINARY"), brokerAddress, brokerHTTP, nil, "BUTLER_POSTGRES_URL="+dsn, "BUTLER_TOOL_REGISTRY_PATH=configs/tools.json", "BUTLER_ACTION_CONTEXT_URL="+authority.URL, "BUTLER_ACTION_AUTHORITY_TOKEN_FILE="+coreFile, "BUTLER_EXTENSION_PROFILES_JSON="+string(profileJSON), "BUTLER_EXTENSION_RUNTIME_TOKEN_FILE="+runtimeFile, "BUTLER_OWNER_CONTROL_OWNER_ID="+owner, "BUTLER_MEMORY_DELETION_MANIFEST_FILE="+manifestPath, "BUTLER_EXECUTION_BUDGET_POLICY_JSON="+string(policyJSON))
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
	store.RunDataGuardTx = sourceCatalog.CheckRunSourcesTx
	observer := initiative.ProjectObserver{Store: store, RunDataGuard: func(ctx context.Context, runID string) error {
		return checkRecordedInitiativeSources(ctx, pool, sourceCatalog, runID)
	}}
	source := initiative.Source{ID: "project-fixture-checks", OwnerID: owner, GoalID: goalResponse.SubjectID, MandateID: mandateResponse.SubjectID, SessionKey: sessionKey, ProjectRef: projectfixture.ProjectRef, ProjectRevision: projectfixture.Revision(), ObserverKind: "broker_project", CheckManifestHash: p.Digest, CheckToolName: "extension.project.inspect", CheckResource: egresstest.Origin + "/project", DigestChannel: "telegram", Purpose: "fixture_maintenance", TimeZone: "UTC", TaskGoal: "Prepare one immutable correction for the synthetic project. First call extension.project.inspect with url " + egresstest.Origin + "/project. Read its exact project_revision and README.md content. Next call extension.project.prepare-patch with url " + egresstest.Origin + "/project/patches and body {project_revision, changes:[{path:README.md,before:exact inspected text,after:same text replacing ready=false with ready=true}]}. Keep every other byte unchanged. Then call extension.project.check with url " + egresstest.Origin + "/project/patches/<actual returned artifact_id>/checks. Leave the original project unapplied. After the three tools return, final output must be only JSON with exact fields artifact_id (actual artifact), checks_passed (actual check boolean), applied (actual check boolean), no Markdown or extra fields. Do not claim verified task completion.", ExecutionPolicy: "normal", CompletionCriteria: []string{"stored prepared patch checks pass", "original project remains unapplied"}, ActiveStart: now.Hour(), ActiveEnd: (now.Hour() + 23) % 24, FailureThreshold: 3, ConfidenceThreshold: 90, MinimumImportance: 60, CooldownSeconds: 3600, Enabled: true}
	if _, err = store.SaveSource(actor("fixture-source"), source, 0); err != nil {
		t.Fatal(err)
	}
	source.Revision = 1
	inputs := durableinput.NewStore(pool)
	promoter := &initiative.Promoter{Store: store, Goals: goals, Tasks: tasks, Inputs: inputs, Budgets: controller, Allowance: budget.ModelAllowance{Provider: providerName, Model: modelName, InputTokens: 1048576, OutputTokens: 8192}}
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
	var projectSource catalog.Source
	pinProject := func(live context.Context) error {
		if projectSource.ID == "" {
			return nil
		}
		tx, err := pool.Begin(live)
		if err != nil {
			return err
		}
		defer tx.Rollback(live)
		_, err = sourceCatalog.AccessTx(live, tx, []catalog.SourceRef{{ID: projectSource.ID, Revision: projectSource.Revision, AccessRevision: projectSource.AccessRevision, Hash: projectSource.Hash}}, "memory_read")
		if err != nil {
			return err
		}
		return tx.Commit(live)
	}
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
		if err = sourceCatalog.PinTranscript(owned, sessionKey, 0); err != nil {
			t.Fatal(err)
		}
		if err = pinProject(owned); err != nil {
			t.Fatal(err)
		}
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
		if projectSource.ID == "" {
			body, _ := json.Marshal(checked)
			projectSource, err = sourceCatalog.Put(actor("pin-real-project-observation"), catalog.PutSource{ExternalKey: "fixed-project-observation", Kind: "document", ScopeType: "user", ScopeID: owner, Title: "Synthetic project observed through actual Broker", Body: string(body)})
			if err != nil || pinProject(owned) != nil {
				t.Fatal("current original project asset pin unavailable", err)
			}
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
	replayed, err := observer.Check(ctx, source, initiative.ClaimedTick{})
	if err != nil || replayed != nil {
		t.Fatal("replayed actual check created a distinct condition", err)
	}
	if err = store.QueueDigests(ctx); err != nil {
		t.Fatal(err)
	}
	var quietNotices int
	if err = pool.QueryRow(ctx, `SELECT count(*) FROM initiative_digest_outbox`).Scan(&quietNotices); err != nil || quietNotices != 0 {
		t.Fatal("unadmitted observations interrupted the owner", err)
	}
	candidate, err = promoter.Promote(ctx, candidate.ID, owner)
	if err != nil || candidate.Status != "promoted" || candidate.TaskID == "" || candidate.InputID == "" {
		t.Fatalf("ordinary initiative admission failed: status=%s reason=%s err=%v", candidate.Status, candidate.Reason, err)
	}
	record, err := tasks.Get(ctx, candidate.TaskID)
	if err != nil {
		t.Fatal(err)
	}
	provider, err := factory(ctx, pool, record.TaskID)
	if err != nil || provider == nil {
		t.Fatal("injected initiative provider unavailable")
	}
	repo.SetCheckpointHooks(inputs)
	gate := flow.NewApprovalGate()
	approvalService = approvals.NewService(repo, gate)
	reader.approvals = approvalService
	results := completion.New(pool)
	results.DataGuard = sourceCatalog.CheckRunSourcesTx
	results.Evidence = effectEvidence{journal: actionauth.NewPostgresStore(pool)}
	executor := flow.NewService(sessions, leases, runs, transcripts, provider, flow.Config{
		ProviderName: providerName, ModelName: modelName, OwnerID: "initiative-ordinary-worker", LeaseTTL: 60, AutonomyMode: commonv1.AutonomyMode_AUTONOMY_MODE_3,
		Tasks: tasks, Checkpoints: inputs, Completion: results, Budgets: controller, OwnerGoals: goals, MemorySources: sourceCatalog,
		InputSourcePreparation: func(live context.Context, event ingress.InputEvent) (ingress.InputEvent, error) {
			return event, pinProject(live)
		},
		Tools: client, ToolCatalog: client, ActionAuthorizer: client, ActionMandateID: source.MandateID, ApprovalChecker: client, ApprovalGate: gate, ApprovalService: approvalService,
	}, nil)
	worker := durableinput.NewService(inputs, sessions, executor, runs, nil)
	workerCtx, stop := context.WithCancel(ctx)
	done := make(chan error, 1)
	go func() { done <- worker.Run(workerCtx) }()
	stopped := false
	defer func() {
		if !stopped {
			stop()
			<-done
		}
	}()
	deadline := time.NewTimer(8 * time.Minute)
	defer deadline.Stop()
	for {
		input, err := inputs.Get(ctx, candidate.InputID)
		if err != nil {
			t.Fatal(err)
		}
		if input.Status == "completed" {
			runID = input.RunID
			break
		}
		if input.Status == "blocked" || input.Status == "failed" || input.Status == "expired" || input.Status == "waiting_for_user" {
			t.Fatalf("ordinary initiative input stopped: status=%s code=%s", input.Status, input.ErrorCode)
		}
		select {
		case <-ctx.Done():
			t.Fatal("ordinary initiative context deadline")
		case <-deadline.C:
			t.Fatal("ordinary initiative execution deadline")
		case <-time.After(20 * time.Millisecond):
		}
	}
	stop()
	if <-done != nil {
		t.Fatal("ordinary input worker stopped unexpectedly")
	}
	stopped = true
	final, err := results.GetByRun(ctx, runID)
	if err != nil || final.Verdict != "unverified" || final.PrivacyRedacted || final.AnswerState != "final" {
		t.Fatal("ordinary initiative final unavailable or promoted", err)
	}
	ownCandidate := observe("own-initiative-observation")
	if ownCandidate.Reason != "own_change" {
		t.Fatal("own task observation generated recursive work")
	}
	var admitted, receipts int
	if err = pool.QueryRow(ctx, `SELECT (SELECT count(*) FROM initiatives WHERE task_id<>''),(SELECT count(*) FROM accepted_inputs WHERE task_id=$1)`, record.TaskID).Scan(&admitted, &receipts); err != nil || admitted != 1 || receipts != 1 {
		t.Fatal("actual checks admitted duplicated initiative tasks or inputs", err)
	}
	actualTranscript, err := transcripts.GetTranscript(ctx, sessionKey)
	if err != nil {
		t.Fatal(err)
	}
	var patchCall, checkCall, artifact string
	for _, call := range actualTranscript.ToolCalls {
		if call.RunID != runID {
			continue
		}
		if call.Status != "completed" {
			t.Fatal("ordinary initiative tool failed")
		}
		body, err := initiativeProjectToolBody(call.ResultJSON)
		if err != nil {
			t.Fatal(err)
		}
		switch call.ToolName {
		case "extension.project.prepare-patch":
			patchCall = call.ToolCallID
			artifact, _ = body["artifact_id"].(string)
		case "extension.project.check":
			if body["checks_passed"] != true || body["applied"] != false {
				t.Fatal("stored prepared correction lacks objective checks")
			}
			checkCall = call.ToolCallID
		}
	}
	if artifact == "" {
		t.Fatal("immutable recipient patch absent")
	}
	if patchCall == "" || checkCall == "" || initiativeProjectFinal(final.Content, artifact) != nil {
		t.Fatal("model final failed exact actual artifact/check projection")
	}
	storedBytes, err := os.ReadFile(filepath.Join(projectDirectory, "patches.json"))
	var stored map[string]projectfixture.Patch
	if err != nil || json.Unmarshal(storedBytes, &stored) != nil || len(stored) != 1 {
		t.Fatal("exactly one actual stored patch required", err)
	}
	for _, patch := range stored {
		if patch.ArtifactID != artifact || patch.Proposal.ProjectRevision != projectfixture.Revision() || len(patch.Proposal.Changes) != 1 || patch.Proposal.Changes[0] != (projectfixture.Change{Path: "README.md", Before: projectfixture.Original, After: projectfixture.Prepared}) {
			t.Fatal("independent stored artifact check failed")
		}
	}
	if _, err = tasks.Transition(ctx, record.TaskID, record.Revision, "completed", final.ResultID); err == nil {
		t.Fatal("model final bypassed independent task verification")
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
	var sends atomic.Int32
	bot := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != "POST" || r.URL.Path != "/botfixture-initiative-only/sendMessage" {
			http.Error(w, "fixture route denied", 400)
			return
		}
		var message struct {
			ChatID int64  `json:"chat_id"`
			Text   string `json:"text"`
		}
		decoder := json.NewDecoder(io.LimitReader(r.Body, 4096))
		decoder.DisallowUnknownFields()
		if decoder.Decode(&message) != nil || message.ChatID != 424242 || !strings.Contains(message.Text, "начато поручений — 0") || !strings.Contains(message.Text, "проверено результатов — 1") || strings.Contains(message.Text, artifact) || strings.Contains(message.Text, source.TaskGoal) || strings.Contains(message.Text, projectfixture.Original) {
			http.Error(w, "count-only digest required", 400)
			return
		}
		sends.Add(1)
		w.Header().Set("Content-Type", "application/json")
		io.WriteString(w, `{"ok":true,"result":{"message_id":101,"chat":{"id":424242,"type":"private"}}}`)
	}))
	defer bot.Close()
	botClient, err := telegram.NewClient(bot.URL, "fixture-initiative-only", bot.Client())
	if err != nil {
		t.Fatal(err)
	}
	channel, err := telegram.NewAdapter(telegram.Config{AllowedChatIDs: []string{"424242"}}, botClient, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	digestWorker := deliveryoutbox.Worker{Store: initiative.DigestStore{Store: store}, Sender: channel}
	if processed, err := digestWorker.ProcessOne(ctx); err != nil || !processed {
		t.Fatal("actual count-only channel delivery failed", err)
	}
	var deliveredStatus, deliveredCode string
	if err = pool.QueryRow(ctx, `SELECT status,error_code FROM initiative_digest_outbox`).Scan(&deliveredStatus, &deliveredCode); err != nil || deliveredStatus != "sent" || sends.Load() != 1 {
		t.Fatalf("actual digest send status=%s code=%s accepted=%d error=%v", deliveredStatus, deliveredCode, sends.Load(), err)
	}
	if processed, err := digestWorker.ProcessOne(ctx); err != nil || processed || sends.Load() != 1 {
		t.Fatal("actual digest delivery duplicated", err)
	}
	var noticeStatus, ack string
	if pool.QueryRow(ctx, `SELECT status,channel_ack FROM initiative_digest_outbox`).Scan(&noticeStatus, &ack) != nil || noticeStatus != "sent" || ack != "101" {
		t.Fatal("actual channel ACK not saved")
	}
	kept, err := results.GetByRun(ctx, runID)
	if err != nil || kept.Verdict != "unverified" || kept.ContentHash != final.ContentHash {
		t.Fatal("owner verification or delivery changed model final", err)
	}
	t.Log("actual_broker_checks=3 distinct_attempts=3 initiatives=1 ordered_inputs=1 ordinary_core=true actual_stored_patch=1 checks_passed=true applied=false owner_verified_task=true model_final_unverified=true count_only_channel_sends=1 actual_channel_ack=true")
}
