package app

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
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
	"github.com/butler/butler/apps/orchestrator/internal/completion"
	durableinput "github.com/butler/butler/apps/orchestrator/internal/input"
	flow "github.com/butler/butler/apps/orchestrator/internal/orchestrator"
	runservice "github.com/butler/butler/apps/orchestrator/internal/run"
	"github.com/butler/butler/apps/orchestrator/internal/session"
	"github.com/butler/butler/apps/orchestrator/internal/task"
	"github.com/butler/butler/apps/orchestrator/internal/tools"
	"github.com/butler/butler/internal/acceptance/utilitytrace"
	"github.com/butler/butler/internal/actionauth"
	"github.com/butler/butler/internal/budget"
	"github.com/butler/butler/internal/extensions"
	"github.com/butler/butler/internal/extensions/projectfixture"
	commonv1 "github.com/butler/butler/internal/gen/common/v1"
	runv1 "github.com/butler/butler/internal/gen/run/v1"
	"github.com/butler/butler/internal/ingress"
	"github.com/butler/butler/internal/memory/catalog"
	"github.com/butler/butler/internal/memory/transcript"
	"github.com/butler/butler/internal/owneridentity"
	"github.com/butler/butler/internal/security/content"
	"github.com/butler/butler/internal/security/egress/egresstest"
	"github.com/butler/butler/internal/skills"
	"github.com/butler/butler/internal/transport"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// Scripted inference supplies proposals only. Real production Broker, current
// PostgreSQL authority, recipient receipts and owned continuation execute them.
type procedureProvider struct {
	digest                       string
	starts                       int
	sawDescription, sawProcedure bool
}

func (*procedureProvider) Name() string                    { return "fixture" }
func (*procedureProvider) InferenceLimits() (int64, int64) { return 4096, 1024 }
func (*procedureProvider) Capabilities(context.Context, transport.TransportRunContext) (transport.CapabilitySnapshot, error) {
	return transport.CapabilitySnapshot{SupportsToolCalls: true}, nil
}
func (p *procedureProvider) StartRun(_ context.Context, r transport.StartRunRequest) (transport.EventStream, error) {
	p.starts++
	if r.Context.ProviderSessionRef != nil {
		return nil, errors.New("restart reused provider reference")
	}
	phase := 0
	artifact := ""
	for _, item := range r.InputItems {
		origin := content.ItemOrigin(item)
		for _, ref := range origin.SourceRefs {
			if strings.HasPrefix(ref, "skill-index:") && strings.Contains(item.Content, `"skill_descriptions"`) {
				p.sawDescription = true
			}
			if ref == "skill:"+p.digest {
				if origin.IsInstruction() || item.Role != "user" {
					return nil, errors.New("procedure became policy")
				}
				p.sawProcedure = true
			}
		}
		if origin.Kind != content.ToolOutput {
			continue
		}
		if item.Role != "user" || origin.IsInstruction() {
			return nil, errors.New("known observation became authority")
		}
		var saved struct {
			Data struct {
				AlreadyExecuted bool   `json:"already_executed"`
				ToolName        string `json:"tool_name"`
				Result          struct {
					Status string `json:"status"`
					Result struct {
						Body string `json:"body"`
					} `json:"result"`
				} `json:"result"`
			} `json:"data"`
		}
		if json.Unmarshal([]byte(item.Content), &saved) != nil {
			return nil, errors.New("known result malformed")
		}
		if !saved.Data.AlreadyExecuted {
			continue
		}
		if saved.Data.Result.Status != "completed" {
			return nil, errors.New("failed result cannot progress procedure")
		}
		switch saved.Data.ToolName {
		case "skills.load":
			if phase < 1 {
				phase = 1
			}
		case "extension.project.inspect":
			if phase < 2 {
				phase = 2
			}
		case "extension.project.prepare-patch":
			var receipt struct {
				ArtifactID string `json:"artifact_id"`
			}
			if json.Unmarshal([]byte(saved.Data.Result.Result.Body), &receipt) != nil || receipt.ArtifactID == "" {
				return nil, errors.New("recipient patch absent")
			}
			artifact = receipt.ArtifactID
			if phase < 3 {
				phase = 3
			}
		case "extension.project.check":
			var checked struct {
				Passed  bool `json:"checks_passed"`
				Applied bool `json:"applied"`
			}
			if json.Unmarshal([]byte(saved.Data.Result.Result.Body), &checked) != nil || !checked.Passed || checked.Applied {
				return nil, errors.New("objective prepared patch check failed")
			}
			phase = 4
		}
	}
	stream := make(chan transport.TransportEvent, 2)
	name := ""
	args := map[string]any{}
	switch {
	case phase == 4:
		stream <- transport.NewAssistantFinalEvent(r.Context.RunID, "fixture", transport.AssistantFinal{Content: "Recipient stored and checked the exact prepared patch; original remains unapplied", FinishReason: "completed"})
		stream <- transport.NewTerminalEvent(r.Context.RunID, transport.EventTypeRunCompleted, "fixture")
	case phase == 0 && p.digest != "":
		name = "skills.load"
		args = map[string]any{"digest": p.digest, "max_bytes": 16384}
	case phase < 2:
		name = "extension.project.inspect"
		args["url"] = egresstest.Origin + "/project"
	case phase == 2:
		name = "extension.project.prepare-patch"
		args = map[string]any{"url": egresstest.Origin + "/project/patches", "body": projectfixture.Proposal{ProjectRevision: projectfixture.Revision(), Changes: []projectfixture.Change{{Path: "README.md", Before: projectfixture.Original, After: projectfixture.Prepared}}}}
	case phase == 3:
		name = "extension.project.check"
		args["url"] = egresstest.Origin + "/project/patches/" + artifact + "/checks"
	}
	if name != "" {
		raw, _ := json.Marshal(args)
		stream <- transport.TransportEvent{RunID: r.Context.RunID, EventType: transport.EventTypeToolCallRequested, ToolCall: &transport.ToolCallRequest{ToolCallRef: r.Context.RunID + "-" + name, ToolName: name, ArgsJSON: string(raw)}}
	}
	close(stream)
	return stream, nil
}
func (*procedureProvider) ContinueRun(context.Context, transport.ContinueRunRequest) (transport.EventStream, error) {
	return nil, errors.New("unexpected continuation")
}
func (p *procedureProvider) SubmitToolResult(_ context.Context, r transport.SubmitToolResultRequest) (transport.EventStream, error) {
	// A local read may continue live. Persisted external approvals always start
	// a fresh provider; they cannot use an orphan native function response.
	if !strings.HasSuffix(r.ToolCallRef, "-skills.load") {
		return nil, errors.New("external approval reused live provider context")
	}
	var selected struct {
		Status string `json:"status"`
		Result struct {
			Digest    string `json:"digest"`
			Authority bool   `json:"instruction_authority"`
		} `json:"result"`
	}
	if json.Unmarshal([]byte(r.ToolResultJSON), &selected) != nil || selected.Status != "completed" || selected.Result.Digest != p.digest || selected.Result.Authority {
		return nil, errors.New("selected procedure unavailable or promoted to authority")
	}
	raw, _ := json.Marshal(map[string]string{"url": egresstest.Origin + "/project"})
	stream := make(chan transport.TransportEvent, 1)
	stream <- transport.TransportEvent{RunID: r.RunID, EventType: transport.EventTypeToolCallRequested, ToolCall: &transport.ToolCallRequest{ToolCallRef: r.RunID + "-extension.project.inspect", ToolName: "extension.project.inspect", ArgsJSON: string(raw)}}
	close(stream)
	return stream, nil
}
func (*procedureProvider) CancelRun(context.Context, transport.CancelRunRequest) (*transport.TransportEvent, error) {
	return nil, nil
}

func TestReviewedSkillRealBrokerDurableApprovalE2E(t *testing.T) {
	if os.Getenv("BUTLER_EXTENSION_E2E") != "1" {
		t.Skip("isolated production Broker/executor proof required")
	}
	runReviewedSkillRealBrokerE2E(t, nil)
}

func runReviewedSkillRealBrokerE2E(t *testing.T, native *skillBrokerNative) {
	duration := 150 * time.Second
	if native != nil {
		duration = 15 * time.Minute
	}
	ctx, cancel := context.WithTimeout(context.Background(), duration)
	defer cancel()
	var initialPool *pgxpool.Pool
	var dsn string
	if native == nil {
		initialPool, dsn = e2eDatabase(t)
	} else {
		initialPool, dsn = native.database(t)
	}
	initialPool.Close()
	poolConfig, err := pgxpool.ParseConfig(dsn)
	if err != nil {
		t.Fatal(err)
	}
	poolConfig.MaxConns = 2
	pool, err := pgxpool.NewWithConfig(context.Background(), poolConfig)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(pool.Close)
	reads := pool
	if native != nil {
		readConfig := pool.Config().Copy()
		readConfig.MaxConns, readConfig.MinConns = 1, 0
		reads, err = pgxpool.NewWithConfig(ctx, readConfig)
		if err != nil {
			t.Fatal("authority reserve unavailable")
		}
		t.Cleanup(reads.Close)
	}
	sessions := session.NewPostgresRepository(pool)
	key := "web:owner"
	if _, _, err := sessions.CreateSession(ctx, session.CreateSessionParams{SessionKey: key, UserID: "owner", Channel: "web", MetadataJSON: "{}"}); err != nil {
		t.Fatal(err)
	}
	runs := runservice.NewService(runservice.NewPostgresRepository(pool), nil)
	leases := session.NewPostgresLeaseManagerWithAuthorityReads(pool, reads)
	tasks := task.NewStoreWithAuthorityReads(pool, reads)
	history := transcript.NewStore(pool)
	inputs := durableinput.NewStore(pool)
	repo := approvals.NewPostgresRepository(pool)
	repo.SetCheckpointHooks(inputs)
	decisions := approvals.NewService(repo, nil)
	actor := func(id string) context.Context {
		return owneridentity.WithActor(ctx, owneridentity.Actor{OwnerID: "owner", Channel: "web", SessionKey: key, DeliveryID: id})
	}
	anchorPath := filepath.Join(t.TempDir(), "deletions")
	if native != nil {
		anchorPath = "/out/source-deletions"
	}
	anchor := catalog.NewFileManifest(anchorPath)
	if err := anchor.Initialize(); err != nil {
		t.Fatal(err)
	}
	sources := catalog.New(pool, anchor)
	if native != nil {
		sources = sources.WithRunObservationPool(reads)
	}
	results := completion.New(pool)
	journal := actionauth.NewPostgresStore(pool)
	results.Evidence = effectEvidence{journal: journal}
	results.DataGuard = func(c context.Context, tx pgx.Tx, id string) error {
		if err := sources.CheckRunSourcesTx(c, tx, id); err != nil {
			return err
		}
		return skills.CheckPinnedLifecycleTx(c, tx, id)
	}
	policy := budget.DefaultPolicy()
	policy.Models["fixture:fixture"] = budget.ModelPrice{Basis: "local_no_billing"}
	if native != nil {
		policy.Models["antigravity:gemini-3.8-flash-high"] = budget.ModelPrice{Basis: "subscription_no_overage"}
		policy.ModelMillis, policy.OutputTokens = 90000, 4096
	}
	budgets := budget.NewControllerWithAuthorityReads(pool, reads, policy)
	budgets.SetAuthorityResolver(actionauth.ResolveBudgetAuthority)
	coreToken := strings.Repeat("skills-core-fixture-", 3)
	runtimeToken := strings.Repeat("skills-runtime-fixture-", 3)
	ownerToken := strings.Repeat("skills-owner-fixture-", 3)
	reader := &actionContextReader{runs: runs, sessions: sessions, leases: leases, tasks: tasks, approvals: decisions, pool: reads}
	private := http.NewServeMux()
	private.Handle("/internal/skill-evidence", skills.NewEvidenceHandler(coreToken, &skillEvidence{tasks: tasks, journal: journal, transcript: history, budgets: budget.NewLedger(pool), results: results, sources: sources}))
	private.Handle("/", actionauth.NewContextHandler(coreToken, reader))
	authority := httptest.NewServer(private)
	defer authority.Close()
	fixtureDirectory := t.TempDir()
	if native != nil {
		fixtureDirectory = "/out/recipient"
		if err := os.Mkdir(fixtureDirectory, 0700); err != nil {
			t.Fatal("new retained recipient required")
		}
	}
	fixture, err := projectfixture.New(fixtureDirectory)
	if err != nil {
		t.Fatal(err)
	}
	defer fixture.Close()
	var postCount atomic.Int32
	var failChecks atomic.Bool
	site := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == "POST" {
			postCount.Add(1)
		}
		if failChecks.Load() && r.Method == "GET" && strings.HasSuffix(r.URL.Path, "/checks") {
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]any{"checks_passed": false, "applied": false, "error_kind": "fixture_failed_check"})
			return
		}
		fixture.ServeHTTP(w, r)
	}))
	defer site.Close()
	runtimeAddress, brokerAddress, brokerHTTP := freeAddress(t), freeAddress(t), freeAddress(t)
	runtimeBinary := os.Getenv("BUTLER_EXTENSION_E2E_BINARY")
	binary, err := os.ReadFile(runtimeBinary)
	if err != nil {
		t.Fatal(err)
	}
	sum := sha256.Sum256(binary)
	installed, err := projectfixture.Manifest(hex.EncodeToString(sum[:]), "reviewed-skills-e2e-v1")
	if err != nil {
		t.Fatal(err)
	}
	profiles, _ := json.Marshal(map[string]extensions.Profile{"reference-http": {Target: runtimeAddress, Dependency: installed.Manifest.Dependencies[0]}})
	secretFile := func(name, value string) string {
		path := filepath.Join(t.TempDir(), name)
		if err := os.WriteFile(path, []byte(value), 0600); err != nil {
			t.Fatal(err)
		}
		return path
	}
	coreFile, runtimeFile := secretFile("core.token", coreToken), secretFile("runtime.token", runtimeToken)
	extensionChild(t, runtimeBinary, runtimeAddress, "127.0.0.1:0", []string{"-test.run=^TestExtensionRuntimeEgressHarness$"}, "BUTLER_EXTENSION_EGRESS_HARNESS=1", "BUTLER_EXTENSION_FIXTURE_ENDPOINT="+site.URL, "BUTLER_EXTENSION_PROFILES_JSON="+string(profiles), "BUTLER_EXTENSION_RUNTIME_TOKEN_FILE="+runtimeFile)
	extensionChild(t, os.Getenv("BUTLER_BROKER_E2E_BINARY"), brokerAddress, brokerHTTP, nil, "BUTLER_POSTGRES_URL="+dsn, "BUTLER_POSTGRES_MAX_CONNS=2", "BUTLER_TOOL_REGISTRY_PATH=configs/tools.json", "BUTLER_ACTION_CONTEXT_URL="+authority.URL, "BUTLER_ACTION_AUTHORITY_TOKEN_FILE="+coreFile, "BUTLER_EXTENSION_PROFILES_JSON="+string(profiles), "BUTLER_EXTENSION_RUNTIME_TOKEN_FILE="+runtimeFile, "BUTLER_OWNER_CONTROL_OWNER_ID=owner", "BUTLER_MEMORY_DELETION_MANIFEST_FILE="+anchorPath, "BUTLER_SKILLS_ENABLED=true")
	extensionClient := extensions.NewControlClient("http://"+brokerHTTP+"/internal/extensions", coreToken)
	control := func(request extensions.ControlRequest) extensions.ControlResponse {
		t.Helper()
		r, err := extensionClient.Control(actor("extension-"+request.Operation), request)
		if err != nil || r.Error != "" {
			t.Fatalf("extension operation %s: %v %s", request.Operation, err, r.Error)
		}
		return r
	}
	control(extensions.ControlRequest{Operation: "stage", ManifestJSON: installed.Raw})
	control(extensions.ControlRequest{Operation: "review", ExtensionID: "project", ManifestHash: installed.Digest})
	state := control(extensions.ControlRequest{Operation: "list"}).Items[0].Record
	toolNames := []string{"extension.project.inspect", "extension.project.prepare-patch", "extension.project.check"}
	state = control(extensions.ControlRequest{Operation: "configure", ExtensionID: "project", ManifestHash: installed.Digest, Generation: state.Generation, Configuration: map[string]string{"origin": egresstest.Origin}, Tools: toolNames}).Item.Record
	control(extensions.ControlRequest{Operation: "probe", ExtensionID: "project", Generation: state.Generation})
	state = control(extensions.ControlRequest{Operation: "enable", ExtensionID: "project", Generation: state.Generation}).Item.Record
	client, err := tools.DialWithTokenFile(brokerAddress, coreFile)
	if err != nil {
		t.Fatal(err)
	}
	defer client.Close()
	skillClient := skills.NewClient("http://"+brokerHTTP+"/internal/skills", coreToken)
	ownerSkills := httptest.NewServer(api.ReviewedSkills(api.NewOwnerControlAuth(ownerToken, "owner"), skillClient))
	defer ownerSkills.Close()
	denied, err := http.Get(ownerSkills.URL)
	if err != nil {
		t.Fatal(err)
	}
	denied.Body.Close()
	if denied.StatusCode != 401 {
		t.Fatal("skill history allowed unauthenticated owner")
	}
	skillControl := func(request skills.ControlRequest) skills.ControlResponse {
		t.Helper()
		r, err := skillClient.Control(actor("skill-"+request.Operation), request)
		if err != nil || r.Error != "" {
			t.Fatalf("skill operation %s: %v %s", request.Operation, err, r.Error)
		}
		return r
	}
	executeProcedure := func(digest string, expectFailedCheck bool) (task.Record, string) {
		t.Helper()
		record, err := tasks.Create(actor("procedure-task"), task.CreateRequest{SessionKey: key, OwnerID: "owner", Goal: "Prepare reviewed fixture README correction", CompletionCriteria: []string{"recipient prepared patch checks pass; original unapplied"}})
		if err != nil {
			t.Fatal(err)
		}
		control(extensions.ControlRequest{Operation: "grant_task", ExtensionID: "project", Access: extensions.TaskAccess{TaskID: record.TaskID, TaskRevision: record.Revision, ConstraintsRevision: record.ConstraintsRevision, ManifestHash: installed.Digest, Generation: state.Generation, Tools: toolNames, Resources: []actionauth.ResourceScope{{Origin: egresstest.Origin, PathPrefix: "/project"}}, ExpiresAt: time.Now().Add(time.Hour)}})
		text := "Prepare one checked patch, stop on any failed or unknown effect"
		if native != nil && digest != "" {
			text = "Inspect http://external.test/project. Prepare one immutable unapplied README.md patch changing ready=false to ready=true through http://external.test/project/patches, and check the exact stored artifact. Never apply or commit it. Stop on any failed or unknown effect."
		}
		payload, _ := json.Marshal(map[string]string{"text": text, "user_id": "owner", "task_id": record.TaskID})
		event := ingress.InputEvent{EventID: "input-" + record.TaskID, IdempotencyKey: "input-" + record.TaskID, SessionKey: key, Source: "web", EventType: runv1.InputEventType_INPUT_EVENT_TYPE_USER_MESSAGE, PayloadJSON: string(payload), CreatedAt: time.Now().UTC()}
		accepted, _, err := inputs.Accept(actor("accept-"+record.TaskID), event, "owner", record.TaskID)
		if err != nil {
			t.Fatal(err)
		}
		runID := ""
		started := 0
		for attempt := 0; attempt < 6; attempt++ {
			claimed, err := inputs.Claim(ctx, time.Minute)
			if err != nil || claimed.InputID != accepted.InputID {
				t.Fatal("current ordered input claim unavailable", err)
			}
			ownedClaim := durableinput.WithClaim(ctx, claimed)
			// Discard every provider/flow instance at a durable approval. The only
			// continuation authority is the persisted Butler input/action state.
			scripted := &procedureProvider{digest: digest}
			var provider transport.ModelProvider = scripted
			var observed *skillBrokerObservedProvider
			if native != nil && digest != "" {
				observed = native.provider(t, digest)
				provider = observed
			}
			cfg := flow.Config{ProviderName: "fixture", ModelName: "fixture", OwnerID: "worker", LeaseTTL: 60, AutonomyMode: commonv1.AutonomyMode_AUTONOMY_MODE_2, Tasks: tasks, Checkpoints: inputs, ApprovalService: decisions, Tools: client, ActionAuthorizer: client, ToolCatalog: client, Completion: results, Budgets: budgets, MemorySources: sources}
			if observed != nil {
				cfg.ProviderName, cfg.ModelName = "antigravity", "gemini-3.8-flash-high"
			}
			if digest != "" {
				cfg.Skills = skillClient
			}
			executor := flow.NewService(sessions, leases, runs, history, provider, cfg, nil)
			var output *flow.ExecutionResult
			if runID == "" {
				output, err = executor.Execute(ownedClaim, event)
			} else {
				output, err = executor.Resume(ownedClaim, runID, event)
			}
			if observed != nil {
				observed.close()
			}
			if expectFailedCheck && err != nil {
				if attempt != 3 || postCount.Load() != 3 {
					t.Fatal("failed check replayed a prior mutation or skipped approval boundaries")
				}
				if _, finalErr := results.GetByRun(ctx, runID); finalErr == nil {
					t.Fatal("failed objective check published authoritative final result")
				}
				if finishErr := inputs.Finish(ctx, claimed, "failed", "fixture_check_failed", "", 0); finishErr != nil {
					t.Fatal(finishErr)
				}
				return record, ""
			}
			if err != nil {
				t.Fatalf("procedure attempt %d: %v", attempt, err)
			}
			if output == nil || output.ReceiptStatus == "blocked" {
				t.Fatalf("procedure stopped: %+v", output)
			}
			if runID != "" && runID != output.RunID {
				t.Fatal("approval changed run identity")
			}
			runID = output.RunID
			starts, sawDescription, sawProcedure := scripted.starts, scripted.sawDescription, scripted.sawProcedure
			if observed != nil {
				starts, sawDescription, sawProcedure = observed.starts, observed.sawDescription, observed.sawProcedure
			}
			started += starts
			if output.CurrentState == commonv1.RunState_RUN_STATE_COMPLETED {
				if err = inputs.Finish(ctx, claimed, "completed", "", "", 0); err != nil {
					t.Fatal(err)
				}
				if digest != "" && (!sawProcedure || started < 4) {
					t.Fatal("pinned procedure not restored through fresh provider context")
				}
				final, err := results.GetByRun(ctx, runID)
				if err != nil || final.Verdict != "unverified" {
					t.Fatal("model final became owner verification", err)
				}
				effects, err := journal.GetRunEvidence(ctx, runID)
				if err != nil {
					t.Fatal(err)
				}
				refs := []string{"final:" + final.ResultID}
				for _, e := range effects {
					if e.EffectState != actionauth.StateConfirmed {
						t.Fatal("procedure continued unconfirmed effect")
					}
					refs = append(refs, e.EvidenceRefs...)
				}
				verified, err := tasks.SaveVerifiedResult(actor("verify-"+record.TaskID), task.Result{TaskID: record.TaskID, TaskRevision: record.Revision, RunID: runID, Summary: "Recipient evidence confirms checked prepared patch", EvidenceRefs: refs, VerifiedCriteria: record.CompletionCriteria, Verifier: "owner"})
				if err != nil {
					t.Fatal(err)
				}
				if native != nil && digest != "" {
					native.verify(t, pool, record, runID, verified.ResultID, digest, postCount.Load(), site.URL)
				}
				return record, verified.ResultID
			}
			if output.CurrentState != commonv1.RunState_RUN_STATE_AWAITING_APPROVAL {
				t.Fatal("expected durable approval boundary")
			}
			if digest != "" && attempt == 0 && (!sawDescription || sawProcedure) {
				t.Fatal("procedure was not loaded progressively")
			}
			if err = inputs.Finish(ctx, claimed, "waiting_for_user", "", "", 0); err != nil {
				t.Fatal(err)
			}
			decision, _, err := decisions.LoadContinuationByRun(ctx, runID)
			if err != nil {
				t.Fatal(err)
			}
			decisionActor := approvals.WithActor(ctx, approvals.Actor{OwnerID: "owner", Channel: "web", SessionKey: key, DeliveryID: "approve-" + decision.ToolCallID})
			if _, _, err = decisions.ResolveByApprovalID(decisionActor, approvals.ResolveByApprovalIDParams{ApprovalID: decision.ApprovalID, Approved: true, ResolvedVia: "web", ActorType: "web", ActorID: "owner", ResolvedBy: "owner"}); err != nil {
				t.Fatal(err)
			}
		}
		t.Fatal("procedure exceeded bounded approval steps")
		return task.Record{}, ""
	}
	baseline, baselineResult := executeProcedure("", false)
	baselineProofs, err := (&skillEvidence{tasks: tasks, journal: journal, transcript: history, budgets: budget.NewLedger(pool), results: results, sources: sources}).VerifySkillSources(ctx, "owner", []skills.EvidenceRef{{TaskID: baseline.TaskID, TaskRevision: baseline.Revision, ResultID: baselineResult}})
	if err != nil || len(baselineProofs) != 1 {
		t.Fatal("actual source proof unavailable", err)
	}
	messageID := strings.TrimPrefix(baselineProofs[0].ContentParents[0].ID, "message:")
	if _, err = sources.VerifiedResultParents(ctx, "owner", messageID, "unrelated-run"); err == nil {
		t.Fatal("historical source identity accepted an unrelated result run")
	}
	manifest := skills.Manifest{Format: skills.Format, ID: "prepare-fixture-patch", Version: "1.0.0", Source: skills.Source{URI: "https://example.invalid/reviewed/prepare-fixture", Revision: "owner-reviewed-v1"}, Description: "procedure-private-body-marker: prepare one immutable README correction and check the stored artifact", When: []string{"owner asks for exact reviewed fixture correction"}, Inputs: []string{"current project revision and exact owner-approved change"}, NotApplicable: []string{"unknown effect, changed dependency, changed project revision"}, Dependencies: baselineProofs[0].ConfirmedContracts, Sources: []skills.EvidenceRef{{TaskID: baseline.TaskID, TaskRevision: baseline.Revision, ResultID: baselineResult}}, Tests: []skills.Scenario{{Name: "verified-success", Kind: "verified_success", Expected: "recipient patch check and owner verification"}, {Name: "failed-check", Kind: "failure_stop", Expected: "stop without claimed success"}, {Name: "changed-interface", Kind: "interface_change", Expected: "reject changed contract"}}, Steps: []skills.Step{{ID: "inspect", Tool: toolNames[0], Intent: "Read current project revision", SuccessCheck: "exact revision", OnError: "stop", Retry: "read_again"}, {ID: "prepare", Tool: toolNames[1], Intent: "Request exact prepared README patch", SuccessCheck: "recipient receipt and artifact ID", OnError: "stop", Retry: "reconcile", ApprovalCheckpoint: true}, {ID: "check", Tool: toolNames[2], Intent: "Check stored patch; leave project unapplied", SuccessCheck: "checks_passed=true and applied=false", OnError: "stop", Retry: "read_again"}}}
	raw, _ := json.Marshal(manifest)
	pack, err := skills.Parse(raw)
	if err != nil {
		t.Fatal(err)
	}
	staged := skillControl(skills.ControlRequest{Operation: "stage", ManifestText: string(raw)}).Item
	skillControl(skills.ControlRequest{Operation: "test", Digest: pack.Digest})
	skillControl(skills.ControlRequest{Operation: "activate", Digest: pack.Digest, Generation: staged.Generation})
	record, verifiedID := executeProcedure(pack.Digest, false)
	if os.Getenv("BUTLER_UTILITY_TRACE_OFFLINE") == "1" {
		var runID string
		if err := pool.QueryRow(ctx, `SELECT last_run_id FROM tasks WHERE task_id=$1`, record.TaskID).Scan(&runID); err != nil {
			t.Fatal(err)
		}
		var body catalog.SourceRef
		var rawRef []byte
		if err := pool.QueryRow(ctx, `SELECT source_ref_json FROM skill_versions WHERE digest=$1`, pack.Digest).Scan(&rawRef); err != nil {
			t.Fatal(err)
		}
		if json.Unmarshal(rawRef, &body) != nil {
			t.Fatal("skill body reference")
		}
		binding := utilitytrace.Binding{Arm: "C", TaskID: record.TaskID, SessionKey: record.SessionKey, OwnerID: record.OwnerID, TaskRevision: record.Revision, ConstraintsRevision: record.ConstraintsRevision, Skills: []utilitytrace.SkillBinding{{Digest: pack.Digest, Body: body}}}
		snapshot, err := utilitytrace.Capture(ctx, pool, sources, binding, runID, map[string]string{})
		if err != nil {
			t.Fatal(err)
		}
		var calls []struct {
			ID   string `json:"tool_call_id"`
			Name string `json:"tool_name"`
		}
		_ = json.Unmarshal(snapshot.Tables["calls"], &calls)
		loads := 0
		for _, call := range calls {
			if call.Name != "skills.load" {
				continue
			}
			loads++
			if _, err := utilitytrace.VerifySkillLoad(snapshot, binding, call.ID); err != nil {
				t.Fatal(err, "authority", snapshot.AuthorityCurrent, "sources", snapshot.SourcesCurrent)
			}
			for _, table := range []string{"skill_usages", "skill_heads", "intents", "effects", "dispatches", "evidence"} {
				bad := cloneUtilityTrace(t, snapshot)
				bad.Tables[table] = json.RawMessage(`[]`)
				if _, err := utilitytrace.VerifySkillLoad(bad, binding, call.ID); err == nil {
					t.Fatal("skill evidence omission accepted", table)
				}
			}
			badBinding := binding
			badBinding.Arm = "B"
			if _, err := utilitytrace.VerifySkillLoad(snapshot, badBinding, call.ID); err == nil {
				t.Fatal("B admitted skill")
			}
		}
		if loads != 1 {
			t.Fatal("expected one actual Broker load", loads)
		}
		if root := os.Getenv("BUTLER_UTILITY_TRACE_OUTPUT"); root != "" {
			raw, _ := json.MarshalIndent(map[string]any{"binding": binding, "snapshot": snapshot, "actual_broker_loads": loads, "corruption_controls_rejected": 7, "native_requests": 0}, "", "  ")
			if err := os.WriteFile(filepath.Join(root, "skill-broker.private.json"), raw, 0600); err != nil {
				t.Fatal(err)
			}
		}
	}
	if verifiedID == "" || record.TaskID == baseline.TaskID || postCount.Load() != 2 {
		t.Fatal("reusable procedure duplicated recipient mutation or lacked separate outcome")
	}
	if native != nil {
		return
	} // Native task receipt retained; failure branch remains a separate scripted regression.
	failChecks.Store(true)
	failedTask, failedResult := executeProcedure(pack.Digest, true)
	if failedResult != "" || failedTask.TaskID == record.TaskID || postCount.Load() != 3 {
		t.Fatal("failed procedure became a verified result or repeated an effect")
	}
	quality := skillControl(skills.ControlRequest{Operation: "quality", Digest: pack.Digest}).Quality
	if quality == nil || quality.VerifiedSuccesses != 1 || quality.UnverifiedAttempts != 1 || len(quality.Baseline) != 1 || len(quality.Outcomes) != 2 || quality.Outcomes[0].ModelUsage == nil || quality.Outcomes[0].ModelUsage.Attempts < 5 {
		t.Fatal("quality omitted actual task/attempt measurements")
	}
	for _, outcome := range quality.Outcomes {
		// Current live provider inputs do contain the marker. Durable replay,
		// transcript, artifacts and auxiliary scratch must retain refs only.
		for _, query := range []string{
			`SELECT COALESCE(string_agg(to_jsonb(x)::text,''),'') FROM accepted_inputs x WHERE run_id=$1`,
			`SELECT COALESCE(string_agg(to_jsonb(x)::text,''),'') FROM approval_actions x JOIN approvals a USING(approval_id) WHERE a.run_id=$1`,
			`SELECT COALESCE(string_agg(to_jsonb(x)::text,''),'') FROM tool_calls x WHERE run_id=$1`,
			`SELECT COALESCE(string_agg(to_jsonb(x)::text,''),'') FROM artifacts x WHERE run_id=$1`,
			`SELECT COALESCE(string_agg(to_jsonb(x)::text,''),'') FROM tasks x WHERE task_id=(SELECT task_id FROM runs WHERE run_id=$1)`,
		} {
			var saved string
			if err = pool.QueryRow(ctx, query, outcome.Usage.RunID).Scan(&saved); err != nil || strings.Contains(saved, "procedure-private-body-marker") {
				t.Fatal("procedure copied outside deletion-aware source graph", err)
			}
		}
	}
}
