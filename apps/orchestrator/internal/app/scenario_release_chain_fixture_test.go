package app

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"github.com/butler/butler/apps/orchestrator/internal/api"
	approvals "github.com/butler/butler/apps/orchestrator/internal/approval"
	"github.com/butler/butler/apps/orchestrator/internal/completion"
	"github.com/butler/butler/apps/orchestrator/internal/goal"
	"github.com/butler/butler/apps/orchestrator/internal/initiative"
	durableinput "github.com/butler/butler/apps/orchestrator/internal/input"
	runservice "github.com/butler/butler/apps/orchestrator/internal/run"
	"github.com/butler/butler/apps/orchestrator/internal/session"
	"github.com/butler/butler/apps/orchestrator/internal/task"
	"github.com/butler/butler/apps/orchestrator/internal/tools"
	"github.com/butler/butler/internal/actionauth"
	"github.com/butler/butler/internal/budget"
	"github.com/butler/butler/internal/delegation"
	"github.com/butler/butler/internal/extensions"
	"github.com/butler/butler/internal/extensions/projectfixture"
	commonv1 "github.com/butler/butler/internal/gen/common/v1"
	runv1 "github.com/butler/butler/internal/gen/run/v1"
	sessionv1 "github.com/butler/butler/internal/gen/session/v1"
	toolbrokerv1 "github.com/butler/butler/internal/gen/toolbroker/v1"
	"github.com/butler/butler/internal/generatedtools"
	"github.com/butler/butler/internal/memory/catalog"
	"github.com/butler/butler/internal/memory/transcript"
	"github.com/butler/butler/internal/owneridentity"
	"github.com/butler/butler/internal/ownership"
	"github.com/butler/butler/internal/security/egress/egresstest"
	"github.com/butler/butler/internal/transport"
	"github.com/jackc/pgx/v5/pgxpool"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

const releaseChainAdmissionGoal = "Prepare a verified readiness correction for the observed synthetic project; leave the original project unapplied. First convert its exact original inventory CSV to JSON: discover the missing converter, delegate bounded development from selected Need/contract, independently check and trial its return, activate only under this original mandate, then convert the original document. Next inspect " + egresstest.Origin + "/project, read exact project_revision and README.md, prepare one immutable patch at " + egresstest.Origin + "/project/patches with body {project_revision,changes:[{path:README.md,before:actual inspected bytes,after:same bytes replacing ready=false with ready=true}]}, and check " + egresstest.Origin + "/project/patches/<actual artifact_id>/checks. Both original inventory conversion and stored ready=true patch are required; never apply the patch or claim model verification."

type releaseChainModelConfig struct {
	Provider, Model                 string
	InputTokens, OutputTokens       int64
	Policy                          budget.Policy
	ChildBudget                     budget.Units
	FixtureTimeout                  time.Duration
	ExtensionHarnessLifetime        time.Duration
	SourceObserver                  *releaseChainSourceObserver
	PostgreSQLPoolSize              int32
	AllowRepeatedPatchChecks        bool
	TaskScopedOriginalProof         bool
	PreserveInvalidOriginalEvidence bool
	FailFastTerminalContinuation    bool
	Factory                         func(context.Context, *pgxpool.Pool, string) (transport.ModelProvider, error)
}

func newReleaseChainFixture(t *testing.T, profile generatedtools.Profile, manifestPath string) *developerFixture {
	return newReleaseChainFixtureWithModel(t, profile, manifestPath, nil)
}

// This bound belongs to the isolated scenario fixture, not production policy.
// The prospective High window separately pins its forty-minute case deadline.
func releaseChainFixtureTimeoutAllowed(timeout time.Duration) bool {
	return timeout > 0 && timeout <= 40*time.Minute
}

func newReleaseChainFixtureWithModel(t *testing.T, profile generatedtools.Profile, manifestPath string, modelConfig *releaseChainModelConfig) *developerFixture {
	t.Helper()
	if os.Getenv("BUTLER_EXTENSION_E2E") != "1" {
		t.Skip("isolated real-process initiative acceptance")
	}
	fixtureTimeout := 10 * time.Minute
	if modelConfig != nil && modelConfig.FixtureTimeout > 0 {
		if !releaseChainFixtureTimeoutAllowed(modelConfig.FixtureTimeout) {
			t.Fatal("bounded fixture timeout required")
		}
		fixtureTimeout = modelConfig.FixtureTimeout
	}
	ctx, cancel := context.WithTimeout(context.Background(), fixtureTimeout)
	t.Cleanup(cancel)
	var pool *pgxpool.Pool
	var dsn string
	poolSize := int32(2)
	if modelConfig != nil && modelConfig.PostgreSQLPoolSize != 0 {
		poolSize = modelConfig.PostgreSQLPoolSize
	}
	if modelConfig != nil && modelConfig.SourceObserver != nil {
		pool, dsn = e2eDatabaseWithTracerAndPool(t, modelConfig.SourceObserver, poolSize)
	} else {
		pool, dsn = e2eDatabaseWithTracerAndPool(t, nil, poolSize)
	}
	// One current deletion anchor and one Broker span every actual phase.
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
	providerName, modelName := "ollama", "converter-worker-fixture"
	policy.Models[providerName+":"+modelName] = budget.ModelPrice{Basis: "local_no_billing"}
	policy.Task.ModelTurns, policy.Task.Tokens, policy.Task.ActiveMillis = 32, 500000, 1200000
	policy.Mandate.ModelTurns, policy.Mandate.Tokens, policy.Mandate.ActiveMillis = 64, 1000000, 2400000
	policy.Day.ModelTurns, policy.Day.Tokens, policy.Day.ActiveMillis = 64, 1000000, 2400000
	policy.Conversation = policy.Task
	policy.ModelMillis, policy.TaskLifetimeMillis = 20000, 1800000
	policy.OutputTokens = 128
	policy.ToolMillis, policy.ModelInputBytes = 30000, 131072
	inputTokens, outputTokens := int64(8192), int64(128)
	if modelConfig != nil {
		if modelConfig.Factory == nil || modelConfig.Provider == "" || modelConfig.Model == "" || modelConfig.InputTokens <= 0 || modelConfig.OutputTokens <= 0 {
			t.Fatal("explicit bounded release chain model config required")
		}
		providerName, modelName = modelConfig.Provider, modelConfig.Model
		policy = modelConfig.Policy
		inputTokens, outputTokens = modelConfig.InputTokens, modelConfig.OutputTokens
	}
	controller := budget.NewController(pool, policy)
	controller.SetAuthorityResolver(actionauth.ResolveBudgetAuthority)
	tasks := task.NewStore(pool)
	tasks.SetVerifiedResultSourceGuard(sourceCatalog.CheckRunSourcesTx)
	tasks.SetBudgets(controller, "")
	goals := goal.New(pool, tasks, nil)
	repo := approvals.NewPostgresRepository(pool)
	repo.SetCheckpointHooks(boundaryCheckpointHooks{})
	approvalService := approvals.NewService(repo, nil)
	coreToken, runtimeToken, ownerToken := strings.Repeat("core-test-", 6), strings.Repeat("runtime-test-", 6), strings.Repeat("owner-test-", 6)
	inputs := durableinput.NewStore(pool)
	developer, e := NewConverterDelegation(pool, tasks, sessions, inputs, sourceCatalog, task.DefaultDelegationLimits(), profile)
	if e != nil {
		t.Fatal(e)
	}
	initial, e := NewDelegationAdmission(pool, tasks, sessions, inputs, sourceCatalog, task.DefaultDelegationLimits())
	if e != nil {
		t.Fatal(e)
	}
	reader := &actionContextReader{runs: runs, sessions: sessions, leases: leases, tasks: tasks, approvals: approvalService, pool: pool, converterLifecycleReady: true, delegationReady: true, converterDelegationReady: true}
	mux := http.NewServeMux()
	mux.Handle("/internal/action-context", actionauth.NewContextHandler(coreToken, reader))
	admissionHandler, e := delegation.NewAdmissionHandler(coreToken, initial)
	if e != nil {
		t.Fatal(e)
	}
	developerHandler, e := delegation.NewAdmissionHandler(coreToken, developer)
	if e != nil {
		t.Fatal(e)
	}
	mux.Handle("/internal/delegation", admissionHandler)
	mux.Handle("/internal/converter-delegation", developerHandler)
	authority := httptest.NewServer(mux)
	t.Cleanup(authority.Close)
	projectDirectory := t.TempDir()
	fixture, err := projectfixture.New(projectDirectory)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = fixture.Close() })
	site := httptest.NewServer(fixture)
	t.Cleanup(site.Close)
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
	harnessLifetime := time.Duration(0)
	if modelConfig != nil {
		harnessLifetime = modelConfig.ExtensionHarnessLifetime
	}
	harnessArgs, harnessEnv, err := releaseChainExtensionHarnessInvocation(ctx, harnessLifetime, fixtureTimeout)
	if err != nil {
		t.Fatal("bounded fixture harness lifetime required")
	}
	harnessEnv = append(harnessEnv, "BUTLER_EXTENSION_EGRESS_HARNESS=1", "BUTLER_EXTENSION_FIXTURE_ENDPOINT="+site.URL, "BUTLER_EXTENSION_PROFILES_JSON="+string(profileJSON), "BUTLER_EXTENSION_RUNTIME_TOKEN_FILE="+runtimeFile)
	extensionChild(t, runtimeBinary, runtimeAddress, "127.0.0.1:0", harnessArgs, harnessEnv...)
	policyJSON, _ := json.Marshal(policy)
	restartBroker := startReleaseChainBroker(t, os.Getenv("BUTLER_BROKER_E2E_BINARY"), brokerAddress, brokerHTTP, "BUTLER_POSTGRES_URL="+dsn, "BUTLER_TOOL_REGISTRY_PATH=configs/tools.json", "BUTLER_ACTION_CONTEXT_URL="+authority.URL+"/internal/action-context", "BUTLER_ACTION_AUTHORITY_TOKEN_FILE="+coreFile, "BUTLER_EXTENSION_PROFILES_JSON="+string(profileJSON), "BUTLER_EXTENSION_RUNTIME_TOKEN_FILE="+runtimeFile, "BUTLER_OWNER_CONTROL_OWNER_ID="+owner, "BUTLER_MEMORY_DELETION_MANIFEST_FILE="+manifestPath, "BUTLER_EXECUTION_BUDGET_POLICY_JSON="+string(policyJSON),
		"BUTLER_SKILLS_ENABLED=true", "BUTLER_CONVERTERS_ENABLED=true", "BUTLER_DELEGATION_ENABLED=true", "BUTLER_DELEGATION_CONTROL_URL="+authority.URL+"/internal/delegation", "BUTLER_CONVERTER_DELEGATION_ENABLED=true", "BUTLER_CONVERTER_DELEGATION_CONTROL_URL="+authority.URL+"/internal/converter-delegation", "BUTLER_CONVERTER_PROFILE_JSON="+os.Getenv("BUTLER_CONVERTER_PROFILE_JSON"), "BUTLER_CONVERTER_RUNTIME_TARGET="+generatedtools.RuntimeTarget, "BUTLER_CONVERTER_RUNTIME_TOKEN_FILE="+os.Getenv("BUTLER_CONVERTER_RUNTIME_TOKEN_FILE"))
	controlServer := httptest.NewServer(api.ReviewedExtensions(api.NewOwnerControlAuth(ownerToken, owner), extensions.NewControlClient("http://"+brokerHTTP+"/internal/extensions", coreToken)))
	t.Cleanup(controlServer.Close)
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
	n := generatedtools.Need{ID: "developer-need", ConverterID: "developer-inventory", Class: generatedtools.Class, Input: generatedtools.InputSchema{MediaType: "text/csv", Delimiter: ",", Header: true, Columns: []string{"name", "count"}}, Output: generatedtools.OutputSchema{MediaType: "application/json", Fields: []generatedtools.OutputField{{Name: "label", Type: "string", SourceIndex: 0}, {Name: "quantity", Type: "integer", SourceIndex: 1}}}, Limits: generatedtools.Limits{InputBytes: 8192, OutputBytes: 32768, Rows: 128, FieldBytes: 1024, Millis: 1000}}
	converterTools := []string{generatedtools.DeveloperAdmissionTool, "converter.developer-inventory.convert"}
	scopes := []actionauth.ResourceScope{{Origin: egresstest.Origin, PathPrefix: "/project"}}
	for _, c := range generatedtools.Contracts() {
		converterTools = append(converterTools, c.ToolName)
	}
	for _, name := range converterTools {
		scopes = append(scopes, actionauth.ResourceScope{Origin: "tool:" + name})
	}
	allowed := append(append([]string{}, selected...), converterTools...)
	goalResponse, err := goals.PutGoal(actor("fixture-goal"), goal.GoalRequest{Goal: actionauth.GoalPolicy{Kind: "objective", Status: "active", Title: "Maintain one synthetic project", Purpose: "fixture_maintenance", ExpiresAt: now.Add(48 * time.Hour)}})
	if err != nil {
		t.Fatal(err)
	}
	u := policy.Mandate
	mandateResponse, err := goals.PutMandate(actor("fixture-mandate"), goal.MandateRequest{Mandate: actionauth.Mandate{GoalID: goalResponse.SubjectID, GoalRevision: goalResponse.Revision, Status: "active", Purpose: "fixture_maintenance", Tools: allowed, Converters: []actionauth.ConverterPermission{generatedtools.ExpectedPermission(n, profile, 2)}, Extensions: []actionauth.ExtensionPermission{{Source: p.Manifest.Source.URI, ExtensionID: p.Manifest.ID, ManifestHash: p.Digest, Tools: selected}}, ResourceScopes: scopes, Effects: []actionauth.Effect{actionauth.EffectRead, actionauth.EffectPrepare, actionauth.EffectCommit}, Privacy: "cloud_allowed", Budget: actionauth.MandateBudget{ModelTurns: u.ModelTurns, Tokens: u.Tokens, CostMicros: u.CostMicros, ExternalCalls: u.ExternalCalls, Children: u.Children, ActiveMillis: u.ActiveMillis, EgressBytes: u.EgressBytes, ProviderInputBytes: u.ProviderInputBytes}, ExpiresAt: now.Add(24 * time.Hour), AllowInitiative: true, Intervention: []string{"effect_unknown", "budget_exhausted"}, Reporting: "exceptions"}})
	if err != nil {
		t.Fatal(err)
	}
	store := initiative.NewStore(pool)
	store.RunDataGuardTx = sourceCatalog.CheckRunSourcesTx
	observer := initiative.ProjectObserver{Store: store, RunDataGuard: func(ctx context.Context, runID string) error {
		return checkRecordedInitiativeSources(ctx, pool, sourceCatalog, runID)
	}}
	asset := func(key, body string, parents []catalog.SourceRef) catalog.SourceRef {
		s, e := sourceCatalog.Put(actor("source-"+key), catalog.PutSource{ExternalKey: key, Kind: "document", ScopeType: "user", ScopeID: owner, Title: "Owner selected synthetic project asset", Body: body, Parents: parents})
		if e != nil {
			t.Fatal(e)
		}
		return catalog.SourceRef{ID: s.ID, Revision: s.Revision, AccessRevision: s.AccessRevision, Hash: s.Hash}
	}
	document := asset("original-document", "name,count\nprivate-original-document,2\n", nil)
	needRaw, _ := json.Marshal(n)
	needRef := asset("developer-need", string(needRaw), []catalog.SourceRef{document})
	spec := generatedtools.DeveloperSpec{Format: generatedtools.DeveloperFormat, NeedHash: generatedtools.Hash(n), Version: "1.0.0", Checker: generatedtools.Checker}
	specRaw, _ := json.Marshal(spec)
	specRef := asset("developer-spec", string(specRaw), nil)
	selectedRefs, _ := json.Marshal(map[string]catalog.SourceRef{"need_source": needRef, "contract_source": specRef, "input_ref": document})
	admissionGoal := releaseChainAdmissionGoal + " Selected current typed DATA refs: " + string(selectedRefs)
	if modelConfig != nil && modelConfig.Provider == "antigravity" {
		allowanceData, _ := json.Marshal(modelConfig.ChildBudget)
		childBound := modelConfig.FixtureTimeout
		if childBound == 0 {
			childBound = 10 * time.Minute
		}
		latest := time.Now().UTC().Add(childBound)
		if current, ok := ctx.Deadline(); ok && current.Before(latest) {
			latest = current.UTC()
		}
		admissionGoal += " Owner-selected bounded child budget DATA: " + string(allowanceData) + ". Use a finite child deadline no later than " + latest.Format(time.RFC3339Nano) + ". This allowance never replaces the current mandate or Broker policy."
	}
	source := initiative.Source{ID: "project-fixture-checks", OwnerID: owner, GoalID: goalResponse.SubjectID, MandateID: mandateResponse.SubjectID, SessionKey: sessionKey, ProjectRef: projectfixture.ProjectRef, ProjectRevision: projectfixture.Revision(), ObserverKind: "broker_project", CheckManifestHash: p.Digest, CheckToolName: "extension.project.inspect", CheckResource: egresstest.Origin + "/project", ResultDelivery: "verified_task", Purpose: "fixture_maintenance", TimeZone: "UTC", TaskGoal: admissionGoal, ExecutionPolicy: "normal", CompletionCriteria: []string{"exact original CSV converted to JSON", "independently tested converter activated within original mandate", "exact immutable ready=true README patch checked; original project remains unapplied"}, ActiveStart: now.Hour(), ActiveEnd: (now.Hour() + 23) % 24, FailureThreshold: 3, ConfidenceThreshold: 90, MinimumImportance: 60, CooldownSeconds: 3600, Enabled: true}
	sourceAPI := httptest.NewServer(api.NewInitiativeServer(store, api.NewOwnerControlAuth(ownerToken, owner)).Handle())
	t.Cleanup(sourceAPI.Close)
	sourceBody, _ := json.Marshal(map[string]any{"source": source, "expected_revision": 0})
	configureSource := func(auth bool) int {
		req, e := http.NewRequestWithContext(ctx, http.MethodPost, sourceAPI.URL+"/api/v3/initiatives/sources", strings.NewReader(string(sourceBody)))
		if e != nil {
			t.Fatal(e)
		}
		req.Header.Set("X-Butler-Delivery-ID", "fixture-source")
		if auth {
			req.Header.Set("Authorization", "Bearer "+ownerToken)
		}
		response, e := sourceAPI.Client().Do(req)
		if e != nil {
			t.Fatal("actual authenticated Source configuration unavailable")
		}
		defer response.Body.Close()
		return response.StatusCode
	}
	if configureSource(false) != 401 || configureSource(true) != 200 {
		t.Fatal("verified_task Source owner API configuration failed")
	}
	source.Revision = 1
	promoter := &initiative.Promoter{Store: store, Goals: goals, Tasks: tasks, Inputs: inputs, Budgets: controller, Allowance: budget.ModelAllowance{Provider: providerName, Model: modelName, InputTokens: inputTokens, OutputTokens: outputTokens}}
	client, err := tools.DialWithTokenFile(brokerAddress, coreFile)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = client.Close() })
	transcripts := transcript.NewStore(pool)
	sequence := 0
	var owned context.Context
	var runID string
	var lease session.LeaseRecord
	var projectSource catalog.Source
	var checkTask task.Record
	transition := func(from, to commonv1.RunState) {
		live := ownership.WithToken(ctx, leases.Authority(), ownership.Token{SessionKey: sessionKey, RunID: runID, LeaseID: lease.LeaseID, FenceEpoch: lease.FenceEpoch})
		if _, e := runs.TransitionRun(live, &sessionv1.UpdateRunStateRequest{RunId: runID, FromState: from, ToState: to, LeaseId: lease.LeaseID}); e != nil {
			t.Fatal(e)
		}
	}
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
		checkTask = record
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
		transition(commonv1.RunState_RUN_STATE_CREATED, commonv1.RunState_RUN_STATE_QUEUED)
		transition(commonv1.RunState_RUN_STATE_QUEUED, commonv1.RunState_RUN_STATE_ACQUIRED)
		transition(commonv1.RunState_RUN_STATE_ACQUIRED, commonv1.RunState_RUN_STATE_PREPARING)
		transition(commonv1.RunState_RUN_STATE_PREPARING, commonv1.RunState_RUN_STATE_MODEL_RUNNING)
		if err = tasks.AttachRun(ctx, record.TaskID, record.Revision, runID, sessionKey); err != nil {
			t.Fatal(err)
		}
		owned = ownership.WithToken(ctx, leases.Authority(), ownership.Token{SessionKey: sessionKey, RunID: runID, LeaseID: lease.LeaseID, FenceEpoch: lease.FenceEpoch})
		currentTranscript, e := transcripts.GetTranscript(ctx, sessionKey)
		if e != nil {
			t.Fatal(e)
		}
		if err = sourceCatalog.PinTranscript(owned, sessionKey, currentTranscript.SourceRevision); err != nil {
			t.Fatal(err)
		}
		if err = pinProject(owned); err != nil {
			t.Fatal(err)
		}
		control(extensions.ControlRequest{Operation: "grant_task", ExtensionID: "project", Access: extensions.TaskAccess{TaskID: record.TaskID, TaskRevision: record.Revision, ConstraintsRevision: record.ConstraintsRevision, ManifestHash: p.Digest, Generation: state.Generation, Tools: selected, Resources: []actionauth.ResourceScope{{Origin: egresstest.Origin, PathPrefix: "/project"}}, ExpiresAt: now.Add(time.Hour)}})
	}
	finish := func() {
		t.Helper()
		transition(commonv1.RunState_RUN_STATE_MODEL_RUNNING, commonv1.RunState_RUN_STATE_FINALIZING)
		r, e := tasks.Get(ctx, checkTask.TaskID)
		if e != nil {
			t.Fatal(e)
		}
		finalizer := completion.New(pool)
		finalizer.DataGuard = sourceCatalog.CheckRunSourcesTx
		finalizer.Evidence = effectEvidence{journal: actionauth.NewPostgresStore(pool)}
		if _, e = finalizer.Save(owned, completion.Request{RunID: runID, SessionKey: sessionKey, TaskID: r.TaskID, TaskRevision: r.Revision, ConstraintsRevision: r.ConstraintsRevision, Content: "Fixed cheap check observed failure; no verified result", AnswerState: "final"}); e != nil {
			t.Fatal(e)
		}
		if _, e = leases.ReleaseLease(ctx, lease.LeaseID); e != nil {
			t.Fatal(e)
		}
	}
	dispatch := func(name, path string, body any) (map[string]any, string) {
		t.Helper()
		transition(commonv1.RunState_RUN_STATE_MODEL_RUNNING, commonv1.RunState_RUN_STATE_TOOL_PENDING)
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
			t.Fatal("cheap check preparation: ", err)
		}
		expiry := prepared.ExpiresAt
		approval, err := approvalService.CreatePendingApproval(owned, approvals.CreatePendingParams{RunID: runID, SessionKey: sessionKey, ToolCallID: call.ToolCallId, ToolName: name, ArgsJSON: call.ArgsJson, RequestedVia: "web", ActionBinding: prepared.Binding, ContinuationJSON: `{"version":1}`, Summary: prepared.Summary, ExpiresAt: &expiry})
		if err != nil {
			t.Fatal("cheap check approval creation: ", err)
		}
		approvalActor := approvals.WithActor(ctx, approvals.Actor{OwnerID: owner, Channel: "web", SessionKey: sessionKey, DeliveryID: "approve-" + call.ToolCallId})
		if _, _, err = approvalService.ResolveByApprovalID(approvalActor, approvals.ResolveByApprovalIDParams{ApprovalID: approval.ApprovalID, Approved: true, ResolvedVia: "web", ActorType: "web", ActorID: owner, ResolvedBy: owner}); err != nil {
			t.Fatal("cheap check owner approval: ", err)
		}
		transition(commonv1.RunState_RUN_STATE_AWAITING_APPROVAL, commonv1.RunState_RUN_STATE_TOOL_RUNNING)
		grant, err := client.AuthorizeAction(ctx, call, prepared.Binding.ActionID, approval.ApprovalID, "")
		if err != nil {
			t.Fatal("cheap check grant: ", err)
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
		transition(commonv1.RunState_RUN_STATE_TOOL_RUNNING, commonv1.RunState_RUN_STATE_AWAITING_MODEL_RESUME)
		transition(commonv1.RunState_RUN_STATE_AWAITING_MODEL_RESUME, commonv1.RunState_RUN_STATE_MODEL_RUNNING)
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
		record, err := goals.CreateOwnerTask(actor(fmt.Sprintf("owner-check-%d", i)), goal.TaskRequest{ResultDelivery: "verified_task", SessionKey: sessionKey, GoalID: source.GoalID, MandateID: source.MandateID, Purpose: source.Purpose, ProjectRef: source.ProjectRef, ProjectRevision: source.ProjectRevision, Goal: "Inspect the synthetic project", ObservationRef: "owner:fixture-check", InitiativeReason: "Owner requests a bounded project check", CompletionCriteria: []string{"fixed project check observed"}})
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
			projectSource, err = sourceCatalog.Put(actor("pin-real-project-observation"), catalog.PutSource{ExternalKey: "fixed-project-observation", Kind: "document", ScopeType: "user", ScopeID: owner, Title: "Synthetic project observed through actual Broker", Body: string(body), Parents: []catalog.SourceRef{document}})
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
	control(extensions.ControlRequest{Operation: "grant_task", ExtensionID: "project", Access: extensions.TaskAccess{TaskID: record.TaskID, TaskRevision: record.Revision, ConstraintsRevision: record.ConstraintsRevision, ManifestHash: p.Digest, Generation: state.Generation, Tools: selected, Resources: []actionauth.ResourceScope{{Origin: egresstest.Origin, PathPrefix: "/project"}}, ExpiresAt: now.Add(time.Hour)}})

	f := &delegationGuardFixture{ctx: ctx, owner: actor("release-chain-source"), pool: pool, sources: sourceCatalog, store: tasks, parent: record, leases: leases, budgets: controller}
	env := developerEnvironment{reader}
	journal := actionauth.NewPostgresStore(pool)
	journal.SetBudgets(controller)
	journal.SetRunDataGuard(sourceCatalog.CheckRunSourcesTx)
	t.Log("cheap_checks=3 actual_approved_broker_reads=4 real_ProjectObserver=true promoted_initiatives=1 same_task=true mode=verified_task")
	return &developerFixture{f: f, goals: goals, profile: profile, need: n, needRef: needRef, specRef: specRef, document: document, service: developer, env: env, journal: journal, initialInputID: candidate.InputID, admissionInput: admissionGoal, existingClient: client, modelConfig: modelConfig, restartBroker: restartBroker, channel: newReleaseChainChannel(t), projectDirectory: projectDirectory}
}
