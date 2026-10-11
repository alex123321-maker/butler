package app

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/butler/butler/apps/orchestrator/internal/goal"
	durableinput "github.com/butler/butler/apps/orchestrator/internal/input"
	"github.com/butler/butler/apps/orchestrator/internal/run"
	"github.com/butler/butler/apps/orchestrator/internal/session"
	"github.com/butler/butler/apps/orchestrator/internal/task"
	"github.com/butler/butler/internal/actionauth"
	"github.com/butler/butler/internal/budget"
	"github.com/butler/butler/internal/delegation"
	commonv1 "github.com/butler/butler/internal/gen/common/v1"
	converterv1 "github.com/butler/butler/internal/gen/converter/v1"
	toolbrokerv1 "github.com/butler/butler/internal/gen/toolbroker/v1"
	"github.com/butler/butler/internal/generatedtools"
	"github.com/butler/butler/internal/memory/catalog"
	"github.com/butler/butler/internal/owneridentity"
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
func (r developerRuntime) Convert(context.Context, *converterv1.ConvertRequest, ...grpc.CallOption) (*converterv1.ConvertResponse, error) {
	return nil, generatedtools.ErrAccess
}

type developerEnvironment struct{ *actionContextReader }

func (developerEnvironment) Discover(context.Context, string, generatedtools.Need) (generatedtools.Discovery, error) {
	return generatedtools.Discovery{Tools: []generatedtools.Capability{}, Skills: []generatedtools.SkillIdentity{}}, nil
}

type developerFixture struct {
	f                          *delegationGuardFixture
	goals                      *goal.Manager
	goal                       actionauth.GoalPolicy
	profile                    generatedtools.Profile
	need                       generatedtools.Need
	needRef, specRef, document catalog.SourceRef
	service                    *ConverterDelegation
	env                        developerEnvironment
	converters                 *generatedtools.Store
	auth                       *actionauth.Service
	journal                    *actionauth.PostgresStore
	parentRun                  string
	next                       int
}

func newDeveloperFixture(t *testing.T) *developerFixture {
	t.Helper()
	f := newDelegationGuardFixture(t)
	n := generatedtools.Need{ID: "developer-need", ConverterID: "developer-inventory", Class: generatedtools.Class, Input: generatedtools.InputSchema{MediaType: "text/csv", Delimiter: ",", Header: true, Columns: []string{"name", "count"}}, Output: generatedtools.OutputSchema{MediaType: "application/json", Fields: []generatedtools.OutputField{{Name: "label", Type: "string", SourceIndex: 0}, {Name: "quantity", Type: "integer", SourceIndex: 1}}}, Limits: generatedtools.Limits{InputBytes: 8192, OutputBytes: 32768, Rows: 128, FieldBytes: 1024, Millis: 1000}}
	profile := generatedtools.Profile{Dependency: generatedtools.Dependency{ID: "butler-converter-ir", Version: "1.0.0", SHA256: strings.Repeat("a", 64)}, Checker: generatedtools.Checker}
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
	raw, _ := json.Marshal(n)
	needRef := catalog.SourceRef(f.source(t, "developer-need", "task", f.parent.TaskID, string(raw), nil))
	spec := generatedtools.DeveloperSpec{Format: generatedtools.DeveloperFormat, NeedHash: generatedtools.Hash(n), Version: "1.0.0", Checker: generatedtools.Checker}
	raw, _ = json.Marshal(spec)
	specRef := catalog.SourceRef(f.source(t, "developer-spec", "task", f.parent.TaskID, string(raw), nil))
	document := catalog.SourceRef(f.source(t, "original-document", "task", f.parent.TaskID, "name,count\nprivate-original-document,2\n", nil))
	s, e := NewConverterDelegation(f.pool, f.store, session.NewPostgresRepository(f.pool), durableinput.NewStore(f.pool), f.sources, task.DefaultDelegationLimits(), profile)
	if e != nil {
		t.Fatal(e)
	}
	reader := &actionContextReader{runs: run.NewService(run.NewPostgresRepository(f.pool), nil), sessions: session.NewPostgresRepository(f.pool), leases: f.leases, tasks: f.store, pool: f.pool, converterLifecycleReady: true}
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
	if name != generatedtools.DeveloperAdmissionTool {
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
	return result, prepared, grant
}
func (f *developerFixture) args() generatedtools.DeveloperArgs {
	return generatedtools.DeveloperArgs{NeedSource: f.needRef, ContractSource: f.specRef, Budget: budget.Units{ModelTurns: 2, Tokens: 10000, ExternalCalls: 4, ActiveMillis: 600000, EgressBytes: 16 << 20, ProviderInputBytes: 16 << 20}, Deadline: time.Now().Add(5 * time.Minute).UTC().Truncate(time.Microsecond), Dependencies: []string{}}
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
