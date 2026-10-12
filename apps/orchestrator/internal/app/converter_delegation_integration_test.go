package app

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/butler/butler/apps/orchestrator/internal/completion"
	"github.com/butler/butler/apps/orchestrator/internal/goal"
	durableinput "github.com/butler/butler/apps/orchestrator/internal/input"
	"github.com/butler/butler/apps/orchestrator/internal/run"
	"github.com/butler/butler/apps/orchestrator/internal/session"
	"github.com/butler/butler/apps/orchestrator/internal/task"
	"github.com/butler/butler/apps/orchestrator/internal/tools"
	"github.com/butler/butler/internal/actionauth"
	"github.com/butler/butler/internal/budget"
	"github.com/butler/butler/internal/delegation"
	commonv1 "github.com/butler/butler/internal/gen/common/v1"
	converterv1 "github.com/butler/butler/internal/gen/converter/v1"
	toolbrokerv1 "github.com/butler/butler/internal/gen/toolbroker/v1"
	"github.com/butler/butler/internal/generatedtools"
	"github.com/butler/butler/internal/memory/catalog"
	"github.com/butler/butler/internal/owneridentity"
	"github.com/butler/butler/internal/ownership"
	"github.com/jackc/pgx/v5"
	"google.golang.org/grpc"
)

// This fixture has actual PG owner/task/source/action/child/budget stores. Its
// runtime is explicit; the separate actual-process acceptance replaces it.
type developerRuntime struct{ profile generatedtools.Profile }

func (r developerRuntime) Develop(ctx context.Context, q *converterv1.DevelopRequest, _ ...grpc.CallOption) (*converterv1.DevelopResponse, error) {
	var n generatedtools.Need
	if generatedtools.Decode(q.NeedSource.Body, &n) != nil {
		return nil, generatedtools.ErrPackage
	}
	p, e := generatedtools.Build(n, q.Version, r.profile)
	if e != nil {
		return nil, e
	}
	return &converterv1.DevelopResponse{Package: p.Raw, PackageHash: p.Digest}, nil
}
func (r developerRuntime) TestPackage(ctx context.Context, q *converterv1.TestPackageRequest, _ ...grpc.CallOption) (*converterv1.TestPackageResponse, error) {
	p, e := generatedtools.Parse(q.PackageSource.Body)
	if e != nil {
		return nil, e
	}
	var n generatedtools.Need
	if generatedtools.Decode(q.NeedSource.Body, &n) != nil {
		return nil, generatedtools.ErrPackage
	}
	report, e := generatedtools.Check(ctx, p, n, r.profile)
	if e != nil {
		return nil, e
	}
	raw, _ := json.Marshal(report)
	return &converterv1.TestPackageResponse{ReportJson: string(raw)}, nil
}
func (r developerRuntime) Convert(ctx context.Context, q *converterv1.ConvertRequest, _ ...grpc.CallOption) (*converterv1.ConvertResponse, error) {
	p, e := generatedtools.Parse(q.PackageSource.Body)
	if e != nil {
		return nil, e
	}
	var n generatedtools.Need
	if generatedtools.Decode(q.NeedSource.Body, &n) != nil {
		return nil, generatedtools.ErrPackage
	}
	out, e := generatedtools.Convert(ctx, p.Manifest.Code, n.Limits, q.InputSource.Body)
	if e != nil {
		return nil, e
	}
	return &converterv1.ConvertResponse{Output: out, OutputHash: generatedtools.HashBytes(out), MediaType: "application/json"}, nil
}

type developerEnvironment struct{ *actionContextReader }

func (developerEnvironment) Discover(context.Context, string, generatedtools.Need) (generatedtools.Discovery, error) {
	return generatedtools.Discovery{Tools: []generatedtools.Capability{}, Skills: []generatedtools.SkillIdentity{}}, nil
}

type developerFixture struct {
	f                              *delegationGuardFixture
	goals                          *goal.Manager
	goal                           actionauth.GoalPolicy
	profile                        generatedtools.Profile
	need                           generatedtools.Need
	needRef, specRef, document     catalog.SourceRef
	service                        *ConverterDelegation
	env                            developerEnvironment
	converters                     *generatedtools.Store
	auth                           *actionauth.Service
	journal                        *actionauth.PostgresStore
	parentRun                      string
	next                           int
	deadline                       time.Time
	initialInputID, admissionInput string
	existingClient                 *tools.BrokerClient
	modelConfig                    *releaseChainModelConfig
	restartBroker                  func() error
	channel                        *releaseChainChannel
	projectDirectory               string
}

func newDeveloperFixture(t *testing.T) *developerFixture {
	return newDeveloperWorkerFixture(t, generatedtools.Profile{Dependency: generatedtools.Dependency{ID: "butler-converter-ir", Version: "1.0.0", SHA256: strings.Repeat("a", 64)}, Checker: generatedtools.Checker}, "", false)
}

// The ordinary-worker proof declares a separate finite fixture model quote;
// production quotas and model/provider credentials are unchanged.
func newDeveloperWorkerFixture(t *testing.T, profile generatedtools.Profile, manifestPath string, modelWorkflow bool) *developerFixture {
	t.Helper()
	var f *delegationGuardFixture
	if manifestPath == "" {
		f = newDelegationGuardFixture(t)
	} else {
		f = newDelegationGuardFixtureWithManifest(t, manifestPath)
	}
	if modelWorkflow {
		f.budgets.Policy.OutputTokens = 128
		// Includes compound current Catalog/055 checks within the same admitted
		// turn; this finite fixture quote changes no production policy.
		f.budgets.Policy.ModelMillis = 5000
		f.budgets.Policy.ModelInputBytes = 128 << 10
		f.budgets.Policy.Models["ollama:converter-worker-fixture"] = budget.ModelPrice{Basis: "local_no_billing"}
	}
	n := generatedtools.Need{ID: "developer-need", ConverterID: "developer-inventory", Class: generatedtools.Class, Input: generatedtools.InputSchema{MediaType: "text/csv", Delimiter: ",", Header: true, Columns: []string{"name", "count"}}, Output: generatedtools.OutputSchema{MediaType: "application/json", Fields: []generatedtools.OutputField{{Name: "label", Type: "string", SourceIndex: 0}, {Name: "quantity", Type: "integer", SourceIndex: 1}}}, Limits: generatedtools.Limits{InputBytes: 8192, OutputBytes: 32768, Rows: 128, FieldBytes: 1024, Millis: 1000}}
	gm := goal.New(f.pool, f.store, nil)
	g := actionauth.GoalPolicy{Kind: "objective", Status: "active", Title: "Exact converter development", Purpose: "converter_work", ExpiresAt: time.Now().Add(24 * time.Hour)}
	receipt, e := gm.PutGoal(owneridentity.WithActor(f.ctx, owneridentity.Actor{OwnerID: f.parent.OwnerID, Channel: "web", DeliveryID: "developer-goal"}), goal.GoalRequest{Goal: g})
	if e != nil {
		t.Fatal(e)
	}
	g.ID, g.Revision = receipt.SubjectID, receipt.Revision
	tools := []string{generatedtools.DeveloperAdmissionTool, "converter.developer-inventory.convert"}
	scopes := []actionauth.ResourceScope{{Origin: "tool:" + generatedtools.DeveloperAdmissionTool}, {Origin: "tool:converter.developer-inventory.convert"}}
	for _, c := range generatedtools.Contracts() {
		tools = append(tools, c.ToolName)
		scopes = append(scopes, actionauth.ResourceScope{Origin: "tool:" + c.ToolName})
	}
	m := actionauth.Mandate{GoalID: g.ID, GoalRevision: g.Revision, Status: "active", Purpose: g.Purpose, Tools: tools, ResourceScopes: scopes, Effects: []actionauth.Effect{actionauth.EffectRead, actionauth.EffectPrepare}, Privacy: "local_only", Converters: []actionauth.ConverterPermission{generatedtools.ExpectedPermission(n, profile, 2)}, Budget: actionauth.MandateBudget{ModelTurns: 8, Tokens: 100000, ExternalCalls: 20, Children: 4, ActiveMillis: 3600000, EgressBytes: 64 << 20, ProviderInputBytes: 64 << 20}, ExpiresAt: time.Now().Add(time.Hour), Intervention: []string{"scope_change", "effect_unknown", "budget_exhausted"}, Reporting: "exceptions"}
	if modelWorkflow {
		m.Budget.ModelTurns, m.Budget.Tokens = 32, 500000
		// The ordinary ModelProvider admission is the cloud-capable contract.
		// This fixture owner explicitly permits it; the deterministic provider
		// itself performs no network IO. Production local_only stays enforced.
		m.Privacy = "cloud_allowed"
	}
	mr, e := gm.PutMandate(owneridentity.WithActor(f.ctx, owneridentity.Actor{OwnerID: f.parent.OwnerID, Channel: "web", DeliveryID: "developer-mandate"}), goal.MandateRequest{Mandate: m})
	if e != nil {
		t.Fatal(e)
	}
	f.parent, e = gm.CreateOwnerTask(owneridentity.WithActor(f.ctx, owneridentity.Actor{OwnerID: f.parent.OwnerID, Channel: "web", DeliveryID: "developer-owner-task"}), goal.TaskRequest{SessionKey: f.parent.SessionKey, GoalID: g.ID, MandateID: mr.SubjectID, Purpose: g.Purpose, Goal: "Develop from selected synthetic contract only", CompletionCriteria: []string{"Independent owner output verification"}, ObservationRef: "owner:selected-need", InitiativeReason: "explicit owner task"})
	if e != nil {
		t.Fatal(e)
	}
	owned := f.leasedWorker(t, f.parent, "converter-parent-run")
	if _, e = f.pool.Exec(f.ctx, `UPDATE runs SET status='tool_running',current_state='tool_running',autonomy_mode='mode_2' WHERE run_id='converter-parent-run'`); e != nil {
		t.Fatal(e)
	}
	if e = f.sources.PinTranscript(owned, f.parent.SessionKey, 0); e != nil {
		t.Fatal(e)
	}
	document := catalog.SourceRef(f.source(t, "original-document", "task", f.parent.TaskID, "name,count\nprivate-original-document,2\n", nil))
	var needParents []catalog.SourceRef
	if modelWorkflow {
		needParents = []catalog.SourceRef{document}
	}
	raw, _ := json.Marshal(n)
	needRef := catalog.SourceRef(f.source(t, "developer-need", "task", f.parent.TaskID, string(raw), needParents))
	spec := generatedtools.DeveloperSpec{Format: generatedtools.DeveloperFormat, NeedHash: generatedtools.Hash(n), Version: "1.0.0", Checker: generatedtools.Checker}
	raw, _ = json.Marshal(spec)
	specRef := catalog.SourceRef(f.source(t, "developer-spec", "task", f.parent.TaskID, string(raw), nil))
	s, e := NewConverterDelegation(f.pool, f.store, session.NewPostgresRepository(f.pool), durableinput.NewStore(f.pool), f.sources, task.DefaultDelegationLimits(), profile)
	if e != nil {
		t.Fatal(e)
	}
	reader := &actionContextReader{runs: run.NewService(run.NewPostgresRepository(f.pool), nil), sessions: session.NewPostgresRepository(f.pool), leases: f.leases, tasks: f.store, pool: f.pool, converterLifecycleReady: true, delegationReady: true, converterDelegationReady: true}
	env := developerEnvironment{reader}
	converters := generatedtools.NewStore(f.pool, f.parent.OwnerID, profile, f.sources, env, developerRuntime{profile})
	journal := actionauth.NewPostgresStore(f.pool)
	journal.SetBudgets(f.budgets)
	journal.SetRunDataGuard(f.sources.CheckRunSourcesTx)
	journal.SetContractGuard(converters)
	auth := actionauth.NewService(journal, env, nil)
	auth.SetCapabilities(converters)
	return &developerFixture{f: f, goals: gm, goal: g, profile: profile, need: n, needRef: needRef, specRef: specRef, document: document, service: s, env: env, converters: converters, auth: auth, journal: journal, parentRun: "converter-parent-run"}
}
func (f *developerFixture) dispatch(t *testing.T, runID, name string, args any) (*toolbrokerv1.ToolResult, actionauth.PreparedAction, actionauth.Grant) {
	t.Helper()
	f.next++
	raw, _ := json.Marshal(args)
	call := &toolbrokerv1.ToolCall{RunId: runID, ToolCallId: fmt.Sprintf("developer-call-%d", f.next), ToolName: name, ArgsJson: string(raw), AutonomyMode: commonv1.AutonomyMode_AUTONOMY_MODE_2}
	c := generatedtools.DeveloperAdmissionContract()
	var e error
	if name != generatedtools.DeveloperAdmissionTool {
		if strings.HasPrefix(name, "converter.") {
			c, e = f.converters.GetContract(f.f.ctx, name)
			if e != nil {
				t.Fatal(e)
			}
		}
		for _, known := range generatedtools.Contracts() {
			if known.ToolName == name {
				c = known
			}
		}
	}
	prepared, e := f.auth.Prepare(f.f.ctx, call, c)
	if e != nil || prepared.RequiresApproval {
		t.Fatal("current owner preparation", name, e)
	}
	grant, e := f.auth.Authorize(f.f.ctx, call, c, prepared.Binding.ActionID, "", "")
	if e != nil {
		t.Fatal("authorize", name, e)
	}
	if e = f.auth.Claim(f.f.ctx, grant.ID); e != nil {
		t.Fatal("claim", name, e)
	}
	effect, e := f.auth.EffectForGrant(f.f.ctx, grant)
	if e != nil {
		t.Fatal(e)
	}
	ctx := actionauth.WithDispatchIdentity(f.f.ctx, actionauth.DispatchIdentity{ActionID: effect.ActionID, Fingerprint: effect.Fingerprint})
	var result *toolbrokerv1.ToolResult
	if name == generatedtools.DeveloperAdmissionTool {
		receipt, e := f.service.AdmitDispatched(ctx, delegation.AdmissionEnvelope{RunID: runID, ToolCallID: call.ToolCallId, EffectID: effect.ActionID, ArgsJSON: call.ArgsJson, ContractHash: prepared.Binding.ContractHash})
		if e != nil {
			t.Fatal("admit", e)
		}
		data, _ := json.Marshal(receipt)
		result = &toolbrokerv1.ToolResult{RunId: runID, ToolCallId: call.ToolCallId, ToolName: name, Status: "completed", ResultJson: string(data)}
	} else {
		result, e = (&generatedtools.RoutedExecutor{Store: f.converters}).Execute(ctx, call, c, nil)
		if e != nil || result.Status != "completed" {
			t.Fatal("execute", name, result.GetError().GetMessage(), e)
		}
	}
	outcome, e := f.auth.Outcome(f.f.ctx, grant, result, false)
	if e != nil || outcome.State != actionauth.StateConfirmed {
		t.Fatal("actual journal outcome", name, e)
	}
	if _, e = f.f.pool.Exec(f.f.ctx, `INSERT INTO tool_calls(tool_call_id,run_id,tool_name,args,status,runtime_target,started_at,finished_at,result) VALUES($1,$2,$3,$4::jsonb,$5,$6,clock_timestamp(),clock_timestamp(),$7::jsonb)`, call.ToolCallId, runID, name, call.ArgsJson, result.Status, c.RuntimeTarget, result.ResultJson); e != nil {
		t.Fatal(e)
	}
	return result, prepared, grant
}
func (f *developerFixture) args() generatedtools.DeveloperArgs {
	deadline := f.deadline
	if deadline.IsZero() {
		deadline = time.Now().Add(5 * time.Minute).UTC().Truncate(time.Microsecond)
	}
	return generatedtools.DeveloperArgs{NeedSource: f.needRef, ContractSource: f.specRef, Budget: budget.Units{ModelTurns: 2, Tokens: 10000, ExternalCalls: 4, ActiveMillis: 600000, EgressBytes: 16 << 20, ProviderInputBytes: 16 << 20}, Deadline: deadline, Dependencies: []string{}}
}

func TestConverterDeveloperMissingParentReceiptIntegration(t *testing.T) {
	f := newDeveloperFixture(t)
	raw, _ := json.Marshal(f.args())
	call := &toolbrokerv1.ToolCall{RunId: f.parentRun, ToolCallId: "no-parent-receipt", ToolName: generatedtools.DeveloperAdmissionTool, ArgsJson: string(raw), AutonomyMode: commonv1.AutonomyMode_AUTONOMY_MODE_2}
	if _, e := f.auth.Prepare(f.f.ctx, call, generatedtools.DeveloperAdmissionContract()); e == nil || !strings.Contains(e.Error(), "missing_prerequisite") {
		t.Fatal("admission hid discovery", e)
	}
	var count int
	if e := f.f.pool.QueryRow(f.f.ctx, `SELECT count(*) FROM converter_developer_snapshots`).Scan(&count); e != nil || count != 0 {
		t.Fatal(count, e)
	}
}

func TestConverterDeveloperExactChildIsolationIntegration(t *testing.T) {
	f := newDeveloperFixture(t)
	f.dispatch(t, f.parentRun, "converters.discover", map[string]any{"source_ref": f.needRef})
	result, _, _ := f.dispatch(t, f.parentRun, generatedtools.DeveloperAdmissionTool, f.args())
	var receipt delegation.AdmissionReceipt
	if json.Unmarshal([]byte(result.ResultJson), &receipt) != nil {
		t.Fatal("receipt")
	}
	child, e := f.f.store.Get(f.f.ctx, receipt.TaskID)
	if e != nil || child.SessionKey == f.f.parent.SessionKey || child.ParentTaskID != f.f.parent.TaskID {
		t.Fatal("independent child identity", e)
	}
	worker := f.f.leasedWorker(t, child, "converter-child-run")
	if _, e = f.f.pool.Exec(f.f.ctx, `UPDATE runs SET autonomy_mode='mode_2',status='tool_running',current_state='tool_running' WHERE run_id='converter-child-run'`); e != nil {
		t.Fatal(e)
	}
	if e = f.f.sources.PinTranscript(worker, child.SessionKey, 0); e != nil {
		t.Fatal(e)
	}
	initial, e := NewDelegationAdmission(f.f.pool, f.f.store, session.NewPostgresRepository(f.f.pool), durableinput.NewStore(f.f.pool), f.f.sources, task.DefaultDelegationLimits())
	if e != nil {
		t.Fatal(e)
	}
	combined, e := NewDelegationLifecycleManager(f.f.store, session.NewPostgresRepository(f.f.pool), durableinput.NewStore(f.f.pool), f.f.sources, task.DefaultDelegationLimits(), initial, f.service)
	if e != nil {
		t.Fatal(e)
	}
	if _, e = combined.MarkExecuting(worker, child.TaskID); e != nil {
		t.Fatal("accepted lifecycle", e)
	}
	tx, e := f.f.pool.Begin(worker)
	if e != nil {
		t.Fatal(e)
	}
	_, e = f.f.sources.AccessTx(worker, tx, []catalog.SourceRef{f.document}, "memory_read")
	_ = tx.Rollback(worker)
	if e == nil {
		t.Fatal("child read original document")
	}
	d, _, _ := f.dispatch(t, "converter-child-run", "converters.discover", map[string]any{"source_ref": f.needRef})
	var discovered struct {
		Discovery generatedtools.Discovery `json:"discovery"`
	}
	if json.Unmarshal([]byte(d.ResultJson), &discovered) != nil || len(discovered.Discovery.Tools) != 3 || len(discovered.Discovery.Skills) != 0 {
		t.Fatal("child global catalog leak")
	}
	candidate, _, _ := f.dispatch(t, "converter-child-run", "converters.develop", map[string]any{"source_ref": f.needRef, "version": "1.0.0"})
	var record generatedtools.Record
	if json.Unmarshal([]byte(candidate.ResultJson), &record) != nil {
		t.Fatal("candidate")
	}
	checked, _, _ := f.dispatch(t, "converter-child-run", "converters.test", map[string]any{"digest": record.Digest})
	var report generatedtools.Report
	if json.Unmarshal([]byte(checked.ResultJson), &report) != nil || !report.Passed {
		t.Fatal("independent checker")
	}
	for _, name := range []string{"converters.trial", "converters.activate", "skills.search", generatedtools.DeveloperAdmissionTool} {
		raw, _ := json.Marshal(map[string]any{"digest": record.Digest, "input_ref": f.document, "generation": record.Generation})
		call := &toolbrokerv1.ToolCall{RunId: "converter-child-run", ToolCallId: "widen-" + name, ToolName: name, ArgsJson: string(raw), AutonomyMode: commonv1.AutonomyMode_AUTONOMY_MODE_2}
		contract := generatedtools.DeveloperAdmissionContract()
		for _, c := range generatedtools.Contracts() {
			if c.ToolName == name {
				contract = c
			}
		}
		if _, e = f.auth.Prepare(f.f.ctx, call, contract); e == nil {
			t.Fatal("child acquired broader tool", name)
		}
	}
	var active int
	if e = f.f.pool.QueryRow(f.f.ctx, `SELECT count(*) FROM converter_catalog WHERE active_digest IS NOT NULL`).Scan(&active); e != nil || active != 0 {
		t.Fatal("child changed working catalog", active, e)
	}
	f.goal.Status = "paused"
	if _, e = f.goals.PutGoal(owneridentity.WithActor(f.f.ctx, owneridentity.Actor{OwnerID: f.f.parent.OwnerID, Channel: "web", DeliveryID: "developer-pause-goal"}), goal.GoalRequest{Goal: f.goal, ExpectedRevision: f.goal.Revision}); e != nil {
		t.Fatal(e)
	}
	if _, e = combined.MarkExecuting(worker, child.TaskID); e == nil {
		t.Fatal("accepted snapshot survived current goal pause")
	}
}

func TestConverterDeveloperOrdinaryRunLifecycleIntegration(t *testing.T) {
	f := newDeveloperFixture(t)
	if _, err := f.f.pool.Exec(f.f.ctx, `UPDATE runs SET task_id=NULL,task_revision=NULL WHERE run_id=$1`, f.parentRun); err != nil {
		t.Fatal(err)
	}
	tx, err := f.f.pool.Begin(f.f.ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Rollback(f.f.ctx)
	if err = generatedtools.CheckDeveloperLifecycleTx(f.f.ctx, tx, f.f.sources, f.parentRun); err != nil {
		t.Fatal("ordinary conversation blocked", err)
	}
	if err = generatedtools.CheckDeveloperLifecycleTx(f.f.ctx, tx, f.f.sources, "missing-run"); err == nil {
		t.Fatal("missing run allowed")
	}
}

func TestDelegationLifecycleAcceptedRoleIntegration(t *testing.T) {
	f := newDispatchAdmissionFixture(t)
	f.claim(t)
	receipt, err := f.service.AdmitDispatched(f.f.ctx, f.envelope)
	if err != nil {
		t.Fatal(err)
	}
	profile := generatedtools.Profile{Dependency: generatedtools.Dependency{ID: "butler-converter-ir", Version: "1.0.0", SHA256: strings.Repeat("a", 64)}, Checker: generatedtools.Checker}
	developer, err := NewConverterDelegation(f.f.pool, f.f.store, session.NewPostgresRepository(f.f.pool), durableinput.NewStore(f.f.pool), f.f.sources, task.DefaultDelegationLimits(), profile)
	if err != nil {
		t.Fatal(err)
	}
	combined, err := NewDelegationLifecycleManager(f.f.store, session.NewPostgresRepository(f.f.pool), durableinput.NewStore(f.f.pool), f.f.sources, task.DefaultDelegationLimits(), f.service, developer)
	if err != nil {
		t.Fatal(err)
	}
	child, err := f.f.store.Get(f.f.ctx, receipt.TaskID)
	if err != nil {
		t.Fatal(err)
	}
	worker := f.f.leasedWorker(t, child, "combined-context-child")
	if _, err = combined.MarkExecuting(worker, child.TaskID); err != nil {
		t.Fatal("context-only role selected", err)
	}
	tx, err := f.f.pool.Begin(f.f.ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Rollback(f.f.ctx)
	router := delegationLifecycleReader{f.service.reader, developer.reader}
	if _, err = router.ReadDelegationCapabilitiesTx(f.f.ctx, tx, f.f.parent, ""); err == nil {
		t.Fatal("composed admission allowed")
	}
	if _, err = router.ReadDelegationCapabilitiesTx(f.f.ctx, tx, f.f.parent, "unaccepted-child"); err == nil {
		t.Fatal("unknown role allowed")
	}
	if _, err = tx.Exec(f.f.ctx, `INSERT INTO converter_developer_snapshots(task_id,owner_id,parent_task_id,parent_revision,parent_constraints_revision,execution_action_id,effect_id,authorization_id,action_hash,args_hash,contract_hash,snapshot_hash,accepted_json) SELECT task_id,owner_id,parent_task_id,parent_revision,parent_constraints_revision,execution_action_id,effect_id,authorization_id,action_hash,args_hash,contract_hash,$2,'{}'::jsonb FROM delegation_admission_snapshots WHERE task_id=$1`, child.TaskID, strings.Repeat("b", 64)); err != nil {
		t.Fatal(err)
	}
	if _, err = router.ReadDelegationCapabilitiesTx(f.f.ctx, tx, f.f.parent, child.TaskID); err == nil {
		t.Fatal("ambiguous accepted role allowed")
	}
}

func (f *developerFixture) testedChild(t *testing.T) (task.Record, context.Context, generatedtools.Record) {
	t.Helper()
	f.dispatch(t, f.parentRun, "converters.discover", map[string]any{"source_ref": f.needRef})
	out, _, _ := f.dispatch(t, f.parentRun, generatedtools.DeveloperAdmissionTool, f.args())
	var receipt delegation.AdmissionReceipt
	if json.Unmarshal([]byte(out.ResultJson), &receipt) != nil {
		t.Fatal("receipt")
	}
	child, err := f.f.store.Get(f.f.ctx, receipt.TaskID)
	if err != nil {
		t.Fatal(err)
	}
	worker := f.f.leasedWorker(t, child, "converter-child-run")
	if _, err = f.f.pool.Exec(f.f.ctx, `UPDATE runs SET autonomy_mode='mode_2',status='tool_running',current_state='tool_running' WHERE run_id='converter-child-run'`); err != nil {
		t.Fatal(err)
	}
	if err = f.f.sources.PinTranscript(worker, child.SessionKey, 0); err != nil {
		t.Fatal(err)
	}
	if _, err = f.service.Manager.MarkExecuting(worker, child.TaskID); err != nil {
		t.Fatal(err)
	}
	f.dispatch(t, "converter-child-run", "converters.discover", map[string]any{"source_ref": f.needRef})
	candidate, _, _ := f.dispatch(t, "converter-child-run", "converters.develop", map[string]any{"source_ref": f.needRef, "version": "1.0.0"})
	var record generatedtools.Record
	if json.Unmarshal([]byte(candidate.ResultJson), &record) != nil {
		t.Fatal("candidate")
	}
	f.dispatch(t, "converter-child-run", "converters.test", map[string]any{"digest": record.Digest})
	return child, worker, record
}

func (f *developerFixture) finishChild(t *testing.T, child task.Record, worker context.Context) task.Result {
	t.Helper()
	if _, err := f.f.pool.Exec(f.f.ctx, `UPDATE runs SET status='finalizing',current_state='finalizing' WHERE run_id='converter-child-run'`); err != nil {
		t.Fatal(err)
	}
	c := completion.New(f.f.pool)
	c.DataGuard = func(ctx context.Context, tx pgx.Tx, id string) error {
		if err := f.f.sources.CheckRunSourcesTx(ctx, tx, id); err != nil {
			return err
		}
		return generatedtools.CheckDeveloperLifecycleTx(ctx, tx, f.f.sources, id)
	}
	c.Evidence = effectEvidence{journal: f.journal}
	if _, err := c.Save(worker, completion.Request{RunID: "converter-child-run", SessionKey: child.SessionKey, TaskID: child.TaskID, TaskRevision: child.Revision, ConstraintsRevision: child.ConstraintsRevision, Content: "Candidate DATA prepared; independent owner task verification remains pending", AnswerState: "final"}); err != nil {
		t.Fatal(err)
	}
	results, err := f.f.store.Results(f.f.ctx, child.TaskID)
	if err != nil || len(results) != 1 || results[0].Verdict != "unverified" {
		t.Fatal("technical final promoted task", results, err)
	}
	return results[0]
}

func TestConverterDeveloperFinalReturnParentActionsIntegration(t *testing.T) {
	f := newDeveloperFixture(t)
	child, worker, candidate := f.testedChild(t)
	args := map[string]any{"child_task_id": child.TaskID}
	raw, _ := json.Marshal(args)
	var collect *toolbrokerv1.ToolContract
	for _, c := range generatedtools.Contracts() {
		if c.ToolName == "converters.collect" {
			collect = c
		}
	}
	call := &toolbrokerv1.ToolCall{RunId: f.parentRun, ToolCallId: "premature-return", ToolName: "converters.collect", ArgsJson: string(raw), AutonomyMode: commonv1.AutonomyMode_AUTONOMY_MODE_2}
	if _, err := f.auth.Prepare(f.f.ctx, call, collect); err == nil {
		t.Fatal("candidate returned before immutable final/report")
	}
	result := f.finishChild(t, child, worker)
	collected, err := f.service.Manager.CollectGenerated(f.f.ctx, f.f.parent.TaskID, child.TaskID, result.ResultID)
	if err != nil || collected.Contract.State != "result_ready" || collected.Report == nil {
		t.Fatal("technical collect", collected, err)
	}
	var lease session.LeaseRecord
	if f.f.pool.QueryRow(f.f.ctx, `SELECT lease_id,fence_epoch FROM session_ownership WHERE run_id=$1`, f.parentRun).Scan(&lease.LeaseID, &lease.FenceEpoch) != nil {
		t.Fatal("parent lease")
	}
	parentCtx := runContextForDeveloperTest(f, f.parentRun, lease)
	tx, err := f.f.pool.Begin(parentCtx)
	if err != nil {
		t.Fatal(err)
	}
	_, err = f.f.sources.AccessTx(parentCtx, tx, []catalog.SourceRef{candidate.BodySource}, "memory_read")
	_ = tx.Rollback(parentCtx)
	if err == nil {
		t.Fatal("technical collection granted candidate without parent Broker action")
	}
	out, bound, _ := f.dispatch(t, f.parentRun, "converters.collect", args)
	if bound.Binding.ConverterPackageHash != candidate.Digest || len(bound.Binding.DataSources) != 3 {
		t.Fatal("server omitted exact return hash before authorization")
	}
	var received generatedtools.Record
	if json.Unmarshal([]byte(out.ResultJson), &received) != nil || received.BodySource != candidate.BodySource {
		t.Fatal("wrong candidate return")
	}
	tx, err = f.f.pool.Begin(parentCtx)
	if err != nil {
		t.Fatal(err)
	}
	_, err = f.f.sources.AccessTx(parentCtx, tx, []catalog.SourceRef{catalog.SourceRef(*collected.Report)}, "memory_read")
	_ = tx.Rollback(parentCtx)
	if err == nil {
		t.Fatal("parent received child narrative/report scope")
	}
	f.dispatch(t, f.parentRun, "converters.test", map[string]any{"digest": candidate.Digest})
	trial, _, _ := f.dispatch(t, f.parentRun, "converters.trial", map[string]any{"digest": candidate.Digest, "input_ref": f.document})
	var checked generatedtools.TrialResult
	if json.Unmarshal([]byte(trial.ResultJson), &checked) != nil || checked.Status != "passed" || checked.Output == nil {
		t.Fatal("independent original trial")
	}
	f.dispatch(t, f.parentRun, "converters.activate", map[string]any{"digest": candidate.Digest, "generation": candidate.Generation})
	original, _, _ := f.dispatch(t, f.parentRun, "converter."+candidate.ID+".convert", map[string]any{"input_ref": f.document})
	if json.Unmarshal([]byte(original.ResultJson), &checked) != nil || checked.Output == nil {
		t.Fatal("original conversion")
	}
	source, err := f.f.sources.Get(parentCtx, checked.Output.Source.ID)
	if err != nil || source.Body != `[{"label":"private-original-document","quantity":2}]` {
		t.Fatal("original output mismatch", err)
	}
	for _, id := range []string{f.f.parent.TaskID, child.TaskID} {
		current, e := f.f.store.Get(f.f.ctx, id)
		if e != nil || current.Status != "active" {
			t.Fatal("technical converter flow completed semantic task", e)
		}
	}
}

func runContextForDeveloperTest(f *developerFixture, id string, lease session.LeaseRecord) context.Context {
	return ownership.WithToken(f.f.ctx, f.f.leases.Authority(), ownership.Token{SessionKey: f.f.parent.SessionKey, RunID: id, LeaseID: lease.LeaseID, FenceEpoch: lease.FenceEpoch})
}
