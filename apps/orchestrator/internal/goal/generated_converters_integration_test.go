package goal_test

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"testing"

	"github.com/butler/butler/apps/orchestrator/internal/goal"
	"github.com/butler/butler/internal/actionauth"
	"github.com/butler/butler/internal/budget"
	commonv1 "github.com/butler/butler/internal/gen/common/v1"
	converterv1 "github.com/butler/butler/internal/gen/converter/v1"
	toolbrokerv1 "github.com/butler/butler/internal/gen/toolbroker/v1"
	"github.com/butler/butler/internal/generatedtools"
	"github.com/butler/butler/internal/memory/catalog"
	"google.golang.org/grpc"
)

// This fixture proves the actual PG admission/journal/source boundary. The
// separate container acceptance uses the production isolated runtime binary.
type converterRuntimeFixture struct {
	profile                    generatedtools.Profile
	developCalls, convertCalls atomic.Int64
	beforeConvert              func()
	badProgram                 bool
	convertError               bool
}

func (r *converterRuntimeFixture) Develop(ctx context.Context, req *converterv1.DevelopRequest, _ ...grpc.CallOption) (*converterv1.DevelopResponse, error) {
	r.developCalls.Add(1)
	var n generatedtools.Need
	if generatedtools.Decode(req.NeedSource.Body, &n) != nil {
		return nil, generatedtools.ErrPackage
	}
	p, e := generatedtools.Build(n, req.Version, r.profile)
	if e != nil {
		return nil, e
	}
	if r.badProgram {
		p.Manifest.Code.Columns[0].InputIndex = 1
		p.Raw, _ = json.Marshal(p.Manifest)
		p, e = generatedtools.Parse(p.Raw)
		if e != nil {
			return nil, e
		}
	}
	return &converterv1.DevelopResponse{Package: p.Raw, PackageHash: p.Digest}, nil
}
func (r *converterRuntimeFixture) TestPackage(ctx context.Context, req *converterv1.TestPackageRequest, _ ...grpc.CallOption) (*converterv1.TestPackageResponse, error) {
	p, e := generatedtools.Parse(req.PackageSource.Body)
	if e != nil {
		return nil, e
	}
	var n generatedtools.Need
	if generatedtools.Decode(req.NeedSource.Body, &n) != nil {
		return nil, generatedtools.ErrPackage
	}
	report, e := generatedtools.Check(ctx, p, n, r.profile)
	if e != nil && !errors.Is(e, generatedtools.ErrChecks) {
		return nil, e
	}
	b, _ := json.Marshal(report)
	return &converterv1.TestPackageResponse{ReportJson: string(b)}, nil
}
func (r *converterRuntimeFixture) Convert(ctx context.Context, req *converterv1.ConvertRequest, _ ...grpc.CallOption) (*converterv1.ConvertResponse, error) {
	r.convertCalls.Add(1)
	if r.beforeConvert != nil {
		r.beforeConvert()
	}
	if r.convertError {
		return nil, generatedtools.ErrInput
	}
	p, e := generatedtools.Parse(req.PackageSource.Body)
	if e != nil {
		return nil, e
	}
	b, e := generatedtools.Convert(ctx, p.Manifest.Code, p.Manifest.Limits, req.InputSource.Body)
	if e != nil {
		return nil, e
	}
	return &converterv1.ConvertResponse{Output: b, OutputHash: generatedtools.HashBytes(b), MediaType: "application/json"}, nil
}

type converterEnvironment struct {
	f                 *fixture
	run               string
	discovery         generatedtools.Discovery
	lifecycleDisabled bool
}

func (e *converterEnvironment) GetActionContext(ctx context.Context, id string) (actionauth.ContextSnapshot, error) {
	c := actionauth.ContextSnapshot{RunID: id, OwnerID: e.f.owner, AutonomyMode: 3, ConverterLifecycleReady: !e.lifecycleDisabled}
	err := e.f.pool.QueryRow(ctx, `SELECT r.session_key,r.current_state,r.task_id,r.task_revision,t.revision,t.constraints_revision,t.status,t.execution_policy,o.lease_id,o.fence_epoch,o.expires_at FROM runs r JOIN tasks t ON t.task_id=r.task_id JOIN session_ownership o ON o.run_id=r.run_id WHERE r.run_id=$1`, id).Scan(&c.SessionKey, &c.RunState, &c.TaskID, &c.PinnedTaskRevision, &c.TaskRevision, &c.ConstraintsRevision, &c.TaskStatus, &c.ExecutionPolicy, &c.LeaseID, &c.LeaseEpoch, &c.LeaseExpiresAt)
	if err != nil {
		return c, err
	}
	a, err := actionauth.GetTaskAssignment(ctx, e.f.pool, c.TaskID)
	if err == nil {
		c.Authority = &a
		c.MandateID = a.MandateID
	}
	return c, err
}
func (e *converterEnvironment) VerifyActionApproval(context.Context, actionauth.ApprovalRequest) (actionauth.ApprovalSnapshot, error) {
	return actionauth.ApprovalSnapshot{}, generatedtools.ErrAccess
}
func (e *converterEnvironment) Discover(context.Context, string, generatedtools.Need) (generatedtools.Discovery, error) {
	return e.discovery, nil
}

type converterFlow struct {
	f                 *fixture
	store             *generatedtools.Store
	auth              *actionauth.Service
	journal           *actionauth.PostgresStore
	sources           *catalog.Store
	env               *converterEnvironment
	runtime           *converterRuntimeFixture
	need              generatedtools.Need
	needRef, inputRef catalog.SourceRef
	manifestPath      string
	next              int
}

func setupConverter(t *testing.T, profiles ...generatedtools.Profile) *converterFlow {
	t.Helper()
	f := setup(t)
	n := generatedtools.Need{ID: "need-inventory", ConverterID: "inventory", Class: generatedtools.Class, Input: generatedtools.InputSchema{MediaType: "text/csv", Delimiter: ",", Header: true, Columns: []string{"name", "count"}}, Output: generatedtools.OutputSchema{MediaType: "application/json", Fields: []generatedtools.OutputField{{Name: "label", Type: "string", SourceIndex: 0}, {Name: "quantity", Type: "integer", SourceIndex: 1}}}, Limits: generatedtools.Limits{InputBytes: 8192, OutputBytes: 32768, Rows: 128, FieldBytes: 1024, Millis: 1000}}
	p := generatedtools.Profile{Dependency: generatedtools.Dependency{ID: "butler-converter-ir", Version: "1.0.0", SHA256: strings.Repeat("a", 64)}, Checker: generatedtools.Checker}
	if len(profiles) == 1 {
		p = profiles[0]
	}
	m := f.m
	m.ID = ""
	m.Revision = 0
	m.Converters = []actionauth.ConverterPermission{generatedtools.ExpectedPermission(n, p, 2)}
	m.Effects = append(m.Effects, actionauth.EffectPrepare)
	m.Tools = nil
	m.ResourceScopes = nil
	m.Budget.ExternalCalls = 40
	m.Budget.ActiveMillis = 3600000
	for _, c := range generatedtools.Contracts() {
		m.Tools = append(m.Tools, c.ToolName)
		m.ResourceScopes = append(m.ResourceScopes, actionauth.ResourceScope{Origin: "tool:" + c.ToolName})
	}
	m.Tools = append(m.Tools, "converter.inventory.convert")
	m.ResourceScopes = append(m.ResourceScopes, actionauth.ResourceScope{Origin: "tool:converter.inventory.convert"})
	r, e := f.manager.PutMandate(f.actor("converter-grant"), goal.MandateRequest{Mandate: m})
	if e != nil {
		t.Fatal(e)
	}
	f.m, e = actionauth.NewPostgresStore(f.pool).GetMandate(f.ctx, r.SubjectID)
	if e != nil {
		t.Fatal(e)
	}
	task := f.create(t, "converter-task")
	id, _ := f.run(t, task, "tool_pending")
	manifestPath := filepath.Join(t.TempDir(), "privacy")
	manifest := catalog.NewFileManifest(manifestPath)
	if e = manifest.Initialize(); e != nil {
		t.Fatal(e)
	}
	sources := catalog.New(f.pool, manifest)
	put := func(key, body string) catalog.SourceRef {
		src, e := sources.Put(f.actor("source-"+key), catalog.PutSource{Kind: "document", ExternalKey: key, ScopeType: "user", ScopeID: f.owner, Body: body})
		if e != nil {
			t.Fatal(e)
		}
		return catalog.SourceRef{ID: src.ID, Revision: src.Revision, AccessRevision: src.AccessRevision, Hash: src.Hash}
	}
	raw, _ := json.Marshal(n)
	needRef := put("need", string(raw))
	inputRef := put("input", "name,count\nwidget,2\n")
	env := &converterEnvironment{f: f, run: id, discovery: generatedtools.Discovery{Tools: []generatedtools.Capability{{Name: "http.request", ContractHash: strings.Repeat("b", 64), Revision: "1"}}, Skills: []generatedtools.SkillIdentity{}}}
	runtime := &converterRuntimeFixture{profile: p}
	s := generatedtools.NewStore(f.pool, f.owner, p, sources, env, runtime)
	j := actionauth.NewPostgresStore(f.pool)
	j.SetContractGuard(s)
	j.SetRunDataGuard(sources.CheckRunSourcesTx)
	b := budget.NewController(f.pool, budget.DefaultPolicy())
	b.SetAuthorityResolver(actionauth.ResolveBudgetAuthority)
	j.SetBudgets(b)
	a := actionauth.NewService(j, env, nil)
	a.SetCapabilities(s)
	return &converterFlow{f: f, store: s, auth: a, journal: j, sources: sources, env: env, runtime: runtime, need: n, needRef: needRef, inputRef: inputRef, manifestPath: manifestPath}
}
func (f *converterFlow) action(t *testing.T, name string, args any) (context.Context, *toolbrokerv1.ToolCall, *toolbrokerv1.ToolContract, actionauth.Grant) {
	t.Helper()
	f.next++
	raw, _ := json.Marshal(args)
	call := &toolbrokerv1.ToolCall{RunId: f.env.run, ToolCallId: fmt.Sprint("converter-", f.next), ToolName: name, ArgsJson: string(raw), AutonomyMode: commonv1.AutonomyMode(3)}
	var contract *toolbrokerv1.ToolContract
	for _, c := range generatedtools.Contracts() {
		if c.ToolName == name {
			contract = c
		}
	}
	if contract == nil {
		var e error
		contract, e = f.store.GetContract(f.f.ctx, name)
		if e != nil {
			t.Fatal(e)
		}
	}
	p, e := f.auth.Prepare(f.f.ctx, call, contract)
	if e != nil {
		t.Fatal(name, "prepare:", e)
	}
	if p.RequiresApproval {
		t.Fatal("exact current owner mandate did not authorize bounded preparation")
	}
	g, e := f.auth.Authorize(f.f.ctx, call, contract, p.Binding.ActionID, "", "")
	if e != nil {
		t.Fatal(name, "authorize:", e)
	}
	if e = f.auth.Claim(f.f.ctx, g.ID); e != nil {
		t.Fatal(name, "claim:", e)
	}
	effect, e := f.auth.EffectForGrant(f.f.ctx, g)
	if e != nil {
		t.Fatal(e)
	}
	ctx := actionauth.WithDispatchIdentity(f.f.ctx, actionauth.DispatchIdentity{ActionID: effect.ActionID, Fingerprint: effect.Fingerprint})
	return ctx, call, contract, g
}
func (f *converterFlow) finish(t *testing.T, g actionauth.Grant, value any) {
	t.Helper()
	raw, _ := json.Marshal(value)
	res := &toolbrokerv1.ToolResult{RunId: g.Binding.RunID, ToolCallId: g.Binding.ToolCallID, ToolName: g.Binding.ToolName, Status: "completed", ResultJson: string(raw)}
	r, e := f.auth.Outcome(f.f.ctx, g, res, false)
	if e != nil || r.State != actionauth.StateConfirmed {
		t.Fatal("local preparation journal", r.State, e)
	}
}
func (f *converterFlow) candidate(t *testing.T, v string) generatedtools.Record {
	t.Helper()
	ctx, call, c, g := f.action(t, "converters.discover", map[string]any{"source_ref": f.needRef})
	n, d, e := f.store.Discover(ctx, call, c, f.needRef)
	if e != nil {
		t.Fatal("discover:", e)
	}
	f.finish(t, g, map[string]any{"need_id": n.ID, "discovery": d})
	ctx, call, c, g = f.action(t, "converters.develop", map[string]any{"source_ref": f.needRef, "version": v})
	r, e := f.store.Develop(ctx, call, c, f.needRef, v)
	if e != nil {
		t.Fatal("develop:", e)
	}
	f.finish(t, g, r)
	return r
}
func (f *converterFlow) tested(t *testing.T, r generatedtools.Record) generatedtools.Report {
	t.Helper()
	ctx, call, c, g := f.action(t, "converters.test", map[string]any{"digest": r.Digest})
	report, e := f.store.Test(ctx, call, c, r.Digest)
	if e != nil {
		t.Fatal("test:", e)
	}
	f.finish(t, g, report)
	return report
}
func (f *converterFlow) trial(t *testing.T, r generatedtools.Record) generatedtools.TrialResult {
	t.Helper()
	ctx, call, c, g := f.action(t, "converters.trial", map[string]any{"digest": r.Digest, "input_ref": f.inputRef})
	result, e := f.store.Apply(ctx, call, c, r.Digest, f.inputRef, true)
	if e != nil {
		t.Fatal("trial:", e)
	}
	f.finish(t, g, result)
	return result
}
func TestGeneratedConverterOwnedLifecycleAndPhysicalDeleteIntegration(t *testing.T) {
	f := setupConverter(t)
	r := f.candidate(t, "1.0.0")
	if !f.tested(t, r).Passed {
		t.Fatal("fixed checker failed")
	}
	trial := f.trial(t, r)
	if trial.Status != "passed" || trial.Output == nil {
		t.Fatal("trial not verified", trial.Status)
	}
	ctx, call, c, g := f.action(t, "converters.activate", map[string]any{"digest": r.Digest, "generation": r.Generation})
	active, e := f.store.Activate(ctx, call, c, r.Digest, r.Generation)
	if e != nil || !active.Active || active.Generation != 2 {
		t.Fatal("activation", e)
	}
	f.finish(t, g, active)
	ctx, call, c, g = f.action(t, "converter.inventory.convert", map[string]any{"input_ref": f.inputRef})
	out, e := f.store.Apply(ctx, call, c, r.Digest, f.inputRef, false)
	if e != nil || out.Output == nil {
		t.Fatal("active conversion", e)
	}
	f.finish(t, g, out)
	read, e := f.sources.Get(f.f.actor("output"), out.Output.Source.ID)
	if e != nil || read.Body != `[{"label":"widget","quantity":2}]` || out.Output.InstructionAuthority {
		t.Fatal("objective output", e, read.Body)
	}
	var status string
	if e = f.f.pool.QueryRow(f.f.ctx, `SELECT status FROM tasks WHERE task_id=$1`, g.Binding.TaskID).Scan(&status); e != nil || status != "active" {
		t.Fatal("technical conversion falsely completed owner task", e)
	}
	if _, e = f.sources.Delete(f.f.actor("delete-input"), f.inputRef.ID, "delete", f.inputRef.Revision); e != nil {
		t.Fatal(e)
	}
	for _, ref := range []catalog.SourceRef{trial.Output.Source, out.Output.Source} {
		var body string
		if e = f.f.pool.QueryRow(f.f.ctx, `SELECT body FROM memory_source_revisions WHERE source_id=$1`, ref.ID).Scan(&body); e != nil || body != "" {
			t.Fatal("private converter output retained after input deletion", e)
		}
	}
	if _, e = f.auth.Prepare(f.f.ctx, &toolbrokerv1.ToolCall{RunId: f.env.run, ToolCallId: "deleted-input", ToolName: "converter.inventory.convert", ArgsJson: call.ArgsJson, AutonomyMode: commonv1.AutonomyMode(3)}, c); e == nil {
		t.Fatal("deleted input remained executable")
	}
}
func TestGeneratedConverterFailedIndependentCandidateDoesNotChangeCatalogIntegration(t *testing.T) {
	f := setupConverter(t)
	f.runtime.badProgram = true
	r := f.candidate(t, "1.0.0")
	if f.tested(t, r).Passed {
		t.Fatal("author mapping changed acceptance")
	}
	var digest *string
	var generation int64
	if e := f.f.pool.QueryRow(f.f.ctx, `SELECT active_digest,generation FROM converter_catalog WHERE converter_id='inventory'`).Scan(&digest, &generation); e != nil || digest != nil || generation != 1 {
		t.Fatal("failed candidate changed working catalog", e)
	}
	if _, e := f.store.GetContract(f.f.ctx, "converter.inventory.convert"); e == nil {
		t.Fatal("failed candidate entered provider tools")
	}
	var count int
	if e := f.f.pool.QueryRow(f.f.ctx, `SELECT count(*) FROM converter_trials`).Scan(&count); e != nil || count != 0 {
		t.Fatal("failed checker created trial", e)
	}
}

func TestGeneratedConverterFiniteConcurrentTrialsAndUnknownSlotIntegration(t *testing.T) {
	f := setupConverter(t)
	r := f.candidate(t, "1.0.0")
	if !f.tested(t, r).Passed {
		t.Fatal("checks failed")
	}
	ctx1, call1, c1, g1 := f.action(t, "converters.trial", map[string]any{"digest": r.Digest, "input_ref": f.inputRef})
	// Parallel independent task runs share the same bounded converter trial
	// allowance; one run still cannot start another effect while executing.
	newRun := func(suffix string) {
		f.f.key = "web:converter-" + suffix
		if _, e := f.f.pool.Exec(f.f.ctx, `INSERT INTO sessions(session_key,user_id,channel) VALUES($1,$2,'web')`, f.f.key, f.f.owner); e != nil {
			t.Fatal(e)
		}
		task := f.f.create(t, "trial-task-"+suffix)
		id, _ := f.f.run(t, task, "tool_pending")
		f.env.run = id
	}
	newRun("two")
	ctx2, call2, c2, g2 := f.action(t, "converters.trial", map[string]any{"digest": r.Digest, "input_ref": f.inputRef})
	newRun("three")
	ctx3, call3, c3, _ := f.action(t, "converters.trial", map[string]any{"digest": r.Digest, "input_ref": f.inputRef})
	entered := make(chan struct{}, 2)
	release := make(chan struct{})
	f.runtime.beforeConvert = func() { entered <- struct{}{}; <-release }
	results := make(chan generatedtools.TrialResult, 2)
	failures := make(chan error, 2)
	var workers sync.WaitGroup
	for _, a := range []struct {
		ctx      context.Context
		call     *toolbrokerv1.ToolCall
		contract *toolbrokerv1.ToolContract
	}{{ctx1, call1, c1}, {ctx2, call2, c2}} {
		workers.Add(1)
		go func() {
			defer workers.Done()
			result, e := f.store.Apply(a.ctx, a.call, a.contract, r.Digest, f.inputRef, true)
			results <- result
			failures <- e
		}()
	}
	for i := 0; i < 2; i++ {
		select {
		case <-entered:
		case <-f.f.ctx.Done():
			t.Fatal("RPC retained PG pool slot", f.f.ctx.Err())
		}
	}
	if _, e := f.store.Apply(ctx3, call3, c3, r.Digest, f.inputRef, true); !errors.Is(e, generatedtools.ErrLimit) {
		t.Fatal("concurrent finite trial allowance exceeded", e)
	}
	if f.runtime.convertCalls.Load() != 2 {
		t.Fatal("denied trial reached runtime")
	}
	close(release)
	workers.Wait()
	for i := 0; i < 2; i++ {
		if e := <-failures; e != nil {
			t.Fatal(e)
		}
		result := <-results
		if result.Status != "passed" {
			t.Fatal(result.Status)
		}
	}
	f.finish(t, g1, map[string]string{"status": "passed"})
	f.finish(t, g2, map[string]string{"status": "passed"})
	var count int
	if e := f.f.pool.QueryRow(f.f.ctx, `SELECT count(*) FROM converter_trials`).Scan(&count); e != nil || count != 2 {
		t.Fatal("trial slots are not durable", e)
	}
	if _, e := f.store.Apply(ctx1, call1, c1, r.Digest, f.inputRef, true); e == nil || f.runtime.convertCalls.Load() != 2 {
		t.Fatal("replayed trial reinvoked runtime", e)
	}
}

func TestGeneratedConverterFailedRPCConsumesTrialAndPreservesTaskIntegration(t *testing.T) {
	f := setupConverter(t)
	r := f.candidate(t, "1.0.0")
	f.tested(t, r)
	f.runtime.convertError = true
	for i := 0; i < 2; i++ {
		result := f.trial(t, r)
		if result.Status != "unknown" || result.Output != nil {
			t.Fatal("failed RPC invented an output")
		}
	}
	ctx, call, c, _ := f.action(t, "converters.trial", map[string]any{"digest": r.Digest, "input_ref": f.inputRef})
	if _, e := f.store.Apply(ctx, call, c, r.Digest, f.inputRef, true); !errors.Is(e, generatedtools.ErrLimit) {
		t.Fatal("unknown attempt refunded trial", e)
	}
	var active *string
	var status string
	if e := f.f.pool.QueryRow(f.f.ctx, `SELECT active_digest FROM converter_catalog WHERE converter_id='inventory'`).Scan(&active); e != nil || active != nil {
		t.Fatal("failed trial activated candidate", e)
	}
	if e := f.f.pool.QueryRow(f.f.ctx, `SELECT status FROM tasks WHERE task_id=(SELECT task_id FROM runs WHERE run_id=$1)`, f.env.run).Scan(&status); e != nil || status != "active" {
		t.Fatal("trial failure lost original owner task", e)
	}
}

func TestGeneratedConverterLateResultDeniedAfterAuthorityOrSourceChangeIntegration(t *testing.T) {
	for _, kind := range []string{"generation", "source_delete", "mandate_revoke", "lease_expiry"} {
		t.Run(kind, func(t *testing.T) {
			f := setupConverter(t)
			r := f.candidate(t, "1.0.0")
			f.tested(t, r)
			ctx, call, c, _ := f.action(t, "converters.trial", map[string]any{"digest": r.Digest, "input_ref": f.inputRef})
			var mutation error
			f.runtime.beforeConvert = func() {
				switch kind {
				case "generation":
					_, mutation = f.f.pool.Exec(f.f.ctx, `UPDATE converter_catalog SET generation=generation+1 WHERE converter_id='inventory'`)
				case "source_delete":
					_, mutation = f.sources.Delete(f.f.actor("delete-during-rpc"), f.inputRef.ID, "delete", f.inputRef.Revision)
				case "mandate_revoke":
					m := f.f.m
					m.Status = "revoked"
					_, mutation = f.f.manager.PutMandate(f.f.actor("revoke-during-rpc"), goal.MandateRequest{Mandate: m, ExpectedRevision: m.Revision})
				case "lease_expiry":
					_, mutation = f.f.pool.Exec(f.f.ctx, `UPDATE session_ownership SET expires_at=clock_timestamp()-INTERVAL '1 second' WHERE run_id=$1`, f.env.run)
				}
			}
			if _, e := f.store.Apply(ctx, call, c, r.Digest, f.inputRef, true); e == nil {
				t.Fatal("stale RPC published output")
			}
			if mutation != nil {
				t.Fatal("fixture mutation", mutation)
			}
			var count int
			if e := f.f.pool.QueryRow(f.f.ctx, `SELECT count(*) FROM converter_trials WHERE output_ref_json IS NOT NULL`).Scan(&count); e != nil || count != 0 {
				t.Fatal("late output retained", e)
			}
			if e := f.f.pool.QueryRow(f.f.ctx, `SELECT count(*) FROM converter_trials WHERE status='running'`).Scan(&count); e != nil || count != 1 {
				t.Fatal("interrupted slot disappeared", e)
			}
		})
	}
}

func TestGeneratedConverterActualDiscoveryRequiredAndCompatibleToolReusedIntegration(t *testing.T) {
	f := setupConverter(t)
	ctx, call, c, _ := f.action(t, "converters.develop", map[string]any{"source_ref": f.needRef, "version": "1.0.0"})
	if _, e := f.store.Develop(ctx, call, c, f.needRef, "1.0.0"); e == nil || f.runtime.developCalls.Load() != 0 {
		t.Fatal("model's asserted search skipped durable discovery", e)
	}
	// An existing exact schema-compatible converter is determined by service
	// metadata, not a model-supplied absence or a compatible flag.
	f = setupConverter(t)
	f.env.discovery.Tools = append(f.env.discovery.Tools, generatedtools.Capability{Name: "converter.existing.convert", ContractHash: strings.Repeat("c", 64), Revision: "1", Class: generatedtools.Class, InputSchemaHash: generatedtools.Hash(f.need.Input), OutputSchemaHash: generatedtools.Hash(f.need.Output)})
	ctx, call, c, g := f.action(t, "converters.discover", map[string]any{"source_ref": f.needRef})
	_, d, e := f.store.Discover(ctx, call, c, f.needRef)
	if e != nil || len(d.Compatible) != 1 {
		t.Fatal("existing capability not found", e)
	}
	f.finish(t, g, d)
	ctx, call, c, _ = f.action(t, "converters.develop", map[string]any{"source_ref": f.needRef, "version": "1.0.0"})
	if _, e = f.store.Develop(ctx, call, c, f.needRef, "1.0.0"); !errors.Is(e, generatedtools.ErrChecks) || f.runtime.developCalls.Load() != 0 {
		t.Fatal("duplicated a known compatible converter", e)
	}
}

func TestGeneratedConverterVersionRollbackAndOwnerRevokeFenceClaimIntegration(t *testing.T) {
	f := setupConverter(t)
	first := f.candidate(t, "1.0.0")
	f.tested(t, first)
	f.trial(t, first)
	second := f.candidate(t, "1.0.1")
	f.tested(t, second)
	f.trial(t, second)
	activate := func(r generatedtools.Record, generation int64, operation string) generatedtools.Record {
		ctx, call, c, g := f.action(t, operation, map[string]any{"digest": r.Digest, "generation": generation})
		result, e := f.store.Activate(ctx, call, c, r.Digest, generation)
		if e != nil {
			t.Fatal(operation, e)
		}
		f.finish(t, g, result)
		return result
	}
	active := activate(first, 1, "converters.activate")
	if active.Generation != 2 {
		t.Fatal("first generation")
	}
	active = activate(second, 2, "converters.activate")
	if active.Generation != 3 {
		t.Fatal("update generation")
	}
	active = activate(first, 3, "converters.rollback")
	if active.Generation != 4 || active.Digest != first.Digest {
		t.Fatal("exact rollback failed")
	}
	contract, e := f.store.GetContract(f.f.ctx, "converter.inventory.convert")
	if e != nil || contract.GetConverter().PackageHash != first.Digest {
		t.Fatal("rollback contract", e)
	}
	raw, _ := json.Marshal(map[string]any{"input_ref": f.inputRef})
	call := &toolbrokerv1.ToolCall{RunId: f.env.run, ToolCallId: "before-revoke", ToolName: contract.ToolName, ArgsJson: string(raw), AutonomyMode: commonv1.AutonomyMode(3)}
	p, e := f.auth.Prepare(f.f.ctx, call, contract)
	if e != nil {
		t.Fatal(e)
	}
	g, e := f.auth.Authorize(f.f.ctx, call, contract, p.Binding.ActionID, "", "")
	if e != nil {
		t.Fatal(e)
	}
	if _, e = f.store.Revoke(f.f.ctx, first.Digest, 4); e == nil {
		t.Fatal("model revoked owner capability")
	}
	revoked, e := f.store.Revoke(f.f.actor("owner-revoke-converter"), first.Digest, 4)
	if e != nil || revoked.Status != "revoked" || revoked.Generation != 5 {
		t.Fatal("owner revoke", e)
	}
	if e = f.auth.Claim(f.f.ctx, g.ID); e == nil {
		t.Fatal("saved grant survived owner revoke")
	}
	effect, e := f.journal.GetEffect(f.f.ctx, p.EffectID)
	if e != nil || effect.State != actionauth.StateAllowed {
		t.Fatal("denied saved grant crossed executing boundary", e)
	}
	items, e := f.store.List(f.f.actor("converter-owner-view"))
	if e != nil || len(items) != 2 {
		t.Fatal("version audit", e)
	}
	var channel, delivery string
	if e = f.f.pool.QueryRow(f.f.ctx, `SELECT source_channel,delivery_id FROM converter_events WHERE kind='owner_revoke'`).Scan(&channel, &delivery); e != nil || channel != "web" || delivery != "owner-revoke-converter" {
		t.Fatal("owner revoke lacks proof provenance", e)
	}
}
