package app

import (
	"context"
	"encoding/json"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/butler/butler/apps/orchestrator/internal/approval"
	"github.com/butler/butler/apps/orchestrator/internal/completion"
	"github.com/butler/butler/apps/orchestrator/internal/goal"
	durableinput "github.com/butler/butler/apps/orchestrator/internal/input"
	"github.com/butler/butler/apps/orchestrator/internal/run"
	"github.com/butler/butler/apps/orchestrator/internal/session"
	"github.com/butler/butler/apps/orchestrator/internal/task"
	"github.com/butler/butler/internal/actionauth"
	"github.com/butler/butler/internal/budget"
	"github.com/butler/butler/internal/delegation"
	"github.com/butler/butler/internal/memory/catalog"
	"github.com/butler/butler/internal/memory/memorytest"
	"github.com/butler/butler/internal/memory/transcript"
	"github.com/butler/butler/internal/owneridentity"
	"github.com/butler/butler/internal/ownership"
	"github.com/butler/butler/internal/security/content"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// This explicit capability reader fixture locks current PG metadata/permission
// rows; it is not a production Broker registry/owner capability implementation.
// All Catalog, manager, owner, journal, completion and budget stores are real.
type delegationPGCapabilities struct{}

func (delegationPGCapabilities) ReadDelegationCapabilitiesTx(ctx context.Context, tx pgx.Tx, parent task.Record, childID string) (DelegationCapabilities, error) {
	var c DelegationCapabilities
	var raw []byte
	err := tx.QueryRow(ctx, `SELECT snapshot FROM fixture_parent_capabilities WHERE task_id=$1 FOR SHARE`, parent.TaskID).Scan(&raw)
	if err != nil {
		return c, err
	}
	err = json.Unmarshal(raw, &c)
	if err == nil && childID != "" {
		var valid bool
		err = tx.QueryRow(ctx, `SELECT parent_task_id=$2 AND parent_revision=$3 AND parent_constraints_revision=$4 FROM task_delegations WHERE task_id=$1`, childID, parent.TaskID, parent.Revision, parent.ConstraintsRevision).Scan(&valid)
		if err == nil && !valid {
			err = delegation.ErrDenied
		}
	}
	return c, err
}

type delegationGuardFixture struct {
	ctx, owner context.Context
	pool       *pgxpool.Pool
	sources    *catalog.Store
	store      *task.Store
	manager    *task.DelegationManager
	guard      *DelegationGuards
	parent     task.Record
	caps       DelegationCapabilities
	leases     *session.PostgresLeaseManager
	budgets    *budget.Controller
}

func newDelegationGuardFixture(t *testing.T) *delegationGuardFixture {
	return newDelegationGuardFixtureWithManifest(t, filepath.Join(t.TempDir(), "deletions.log"))
}

func newDelegationGuardFixtureWithManifest(t *testing.T, path string) *delegationGuardFixture {
	t.Helper()
	pool := memorytest.Database(t)
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	t.Cleanup(cancel)
	owner := owneridentity.WithActor(ctx, owneridentity.Actor{OwnerID: "guard-owner", Channel: "web", DeliveryID: "guard-fixture"})
	manifest := catalog.NewFileManifest(path)
	if err := manifest.Initialize(); err != nil {
		t.Fatal(err)
	}
	sources := catalog.New(pool, manifest)
	if _, err := pool.Exec(ctx, `INSERT INTO sessions(session_key,user_id,channel) VALUES('guard-root','guard-owner','web'); CREATE TABLE fixture_parent_capabilities(task_id text PRIMARY KEY,snapshot jsonb)`); err != nil {
		t.Fatal(err)
	}
	policy := budget.DefaultPolicy()
	bc := budget.NewController(pool, policy)
	bc.SetAuthorityResolver(actionauth.ResolveBudgetAuthority)
	store := task.NewStore(pool)
	store.SetBudgets(bc, "")
	parent, err := store.Create(ctx, task.CreateRequest{SessionKey: "guard-root", OwnerID: "guard-owner", Goal: "bounded objective", CompletionCriteria: []string{"independent evidence"}})
	if err != nil {
		t.Fatal(err)
	}
	caps := DelegationCapabilities{TaskID: parent.TaskID, OwnerID: parent.OwnerID, TaskRevision: parent.Revision, ConstraintsRevision: parent.ConstraintsRevision, Permissions: []delegation.Permission{{Tool: "http.request", ContractHash: validationHash, Resources: []string{"https://fixture.example/read"}, Effects: []string{"read"}}}, Budget: delegation.Limits{ModelTurns: 8, Tokens: 1000000, ExternalCalls: 8, Children: 4, ActiveMillis: 1000000, EgressBytes: 33554432, ProviderInputBytes: 33554432}, Deadline: time.Now().Add(time.Hour).UTC().Truncate(time.Microsecond)}
	guard, err := NewDelegationGuards(sources, delegationPGCapabilities{})
	if err != nil {
		t.Fatal(err)
	}
	limits := task.DefaultDelegationLimits()
	limits.MaxParallel = 2
	manager, err := task.NewDelegationManager(store, session.NewPostgresRepository(pool), durableinput.NewStore(pool), guard, limits)
	if err != nil {
		t.Fatal(err)
	}
	f := &delegationGuardFixture{ctx, owner, pool, sources, store, manager, guard, parent, caps, session.NewPostgresLeaseManager(pool), bc}
	f.saveCaps(t)
	return f
}
func (f *delegationGuardFixture) saveCaps(t *testing.T) {
	t.Helper()
	raw, _ := json.Marshal(f.caps)
	if _, err := f.pool.Exec(f.ctx, `INSERT INTO fixture_parent_capabilities VALUES($1,$2) ON CONFLICT(task_id) DO UPDATE SET snapshot=EXCLUDED.snapshot`, f.parent.TaskID, raw); err != nil {
		t.Fatal(err)
	}
}
func guardSource(s catalog.Source) delegation.SourceRef {
	return delegation.SourceRef{ID: s.ID, Revision: s.Revision, AccessRevision: s.AccessRevision, Hash: s.Hash}
}
func (f *delegationGuardFixture) source(t *testing.T, key, scope, id, body string, parents []catalog.SourceRef) delegation.SourceRef {
	t.Helper()
	s, err := f.sources.Put(f.owner, catalog.PutSource{ExternalKey: key, Kind: "model_candidate", ScopeType: scope, ScopeID: id, Body: body, Title: "Bounded DATA", Parents: parents})
	if err != nil {
		t.Fatal(err)
	}
	return guardSource(s)
}
func (f *delegationGuardFixture) proposal(t *testing.T, key string) task.DelegationRequest {
	t.Helper()
	return task.DelegationRequest{ParentTaskID: f.parent.TaskID, ParentRevision: f.parent.Revision, ConstraintsRevision: f.parent.ConstraintsRevision, ContractSource: f.source(t, key, "user", f.parent.OwnerID, "bounded contract DATA", nil), Permissions: append([]delegation.Permission(nil), f.caps.Permissions...), Budget: f.caps.Budget, Deadline: time.Now().Add(10 * time.Minute).UTC().Truncate(time.Microsecond)}
}
func (f *delegationGuardFixture) admit(t *testing.T, key string) (delegation.Contract, task.Record) {
	t.Helper()
	r, err := f.manager.Admit(f.owner, f.proposal(t, key))
	if err != nil {
		t.Fatal(err)
	}
	child, err := f.store.Get(f.ctx, r.Contract.TaskID)
	if err != nil {
		t.Fatal(err)
	}
	return r.Contract, child
}
func (f *delegationGuardFixture) worker(t *testing.T, child task.Record, id string) context.Context {
	ctx := f.leasedWorker(t, child, id)
	if _, err := f.manager.MarkExecuting(ctx, child.TaskID); err != nil {
		t.Fatal(err)
	}
	return ctx
}

func (f *delegationGuardFixture) leasedWorker(t *testing.T, child task.Record, id string) context.Context {
	t.Helper()
	if _, err := f.pool.Exec(f.ctx, `INSERT INTO runs(run_id,session_key,input_event_id,status,current_state,autonomy_mode,model_provider,started_at,updated_at,task_id,task_revision) VALUES($1,$2,'fixture','model_running','model_running','mode_1','fixture',clock_timestamp(),clock_timestamp(),$3,$4)`, id, child.SessionKey, child.TaskID, child.Revision); err != nil {
		t.Fatal(err)
	}
	l, err := f.leases.AcquireLease(f.ctx, session.AcquireLeaseParams{SessionKey: child.SessionKey, RunID: id, OwnerID: "guard-worker", TTL: time.Minute})
	if err != nil {
		t.Fatal(err)
	}
	if _, err = f.pool.Exec(f.ctx, `UPDATE runs SET lease_id=$2 WHERE run_id=$1`, id, l.LeaseID); err != nil {
		t.Fatal(err)
	}
	ctx := ownership.WithToken(f.ctx, f.leases.Authority(), ownership.Token{SessionKey: child.SessionKey, RunID: id, LeaseID: l.LeaseID, FenceEpoch: l.FenceEpoch})
	return ctx
}
func (f *delegationGuardFixture) complete(t *testing.T, c delegation.Contract, child task.Record, unknown, partial bool) (task.Result, delegation.Report, []catalog.SourceRef) {
	t.Helper()
	runID := "child-run:" + child.TaskID
	ctx := f.worker(t, child, runID)
	tx, err := f.pool.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = f.sources.AccessTx(ctx, tx, refsForDelegation(c), "memory_read"); err != nil {
		t.Fatal(err)
	}
	if err = tx.Commit(ctx); err != nil {
		t.Fatal(err)
	}
	journal := actionauth.NewPostgresStore(f.pool)
	journal.SetBudgets(f.budgets)
	journal.SetRunDataGuard(f.sources.CheckRunSourcesTx)
	token, _ := ownership.TokenFromContext(ctx)
	binding := actionauth.ActionBinding{ActionID: "action:" + child.TaskID, ActionHash: validationHash, RunID: runID, ToolCallID: "call:" + child.TaskID, TaskID: child.TaskID, TaskRevision: child.Revision, ConstraintsRevision: child.ConstraintsRevision, OwnerID: child.OwnerID, ToolName: "http.request", ContractHash: validationHash, ArgsHash: validationHash, Resource: "https://fixture.example/read", Effect: actionauth.EffectRead}
	if err = journal.PutIntent(ctx, actionauth.Intent{Binding: binding, EffectID: binding.ActionID, EffectFingerprint: validationHash, EffectPolicy: actionauth.EffectPolicy{RetrySafety: "read_only", VerificationMethod: "read_result"}, ExpiresAt: time.Now().Add(time.Minute)}); err != nil {
		t.Fatal(err)
	}
	grant, err := journal.PutGrant(ctx, actionauth.Grant{ID: "grant:" + child.TaskID, Binding: binding, LeaseID: token.LeaseID, LeaseEpoch: token.FenceEpoch, ExpiresAt: time.Now().Add(time.Minute)})
	if err != nil {
		t.Fatal(err)
	}
	if err = journal.ClaimGrant(ctx, grant.ID); err != nil {
		t.Fatal(err)
	}
	verdict := actionauth.StateConfirmed
	outcome := "dispatch_completed"
	if unknown {
		verdict = actionauth.StateUnknown
		outcome = "unknown"
	}
	now := time.Now().UTC()
	ev := actionauth.EffectEvidence{EvidenceID: "evidence:" + child.TaskID, ActionID: binding.ActionID, SourceActionID: binding.ActionID, SourceRunID: runID, SourceToolCallID: binding.ToolCallID, Source: "runtime_response", Method: "read_result", Verdict: verdict, ResultDigest: validationHash, ObservedAt: now, RecordedAt: now}
	if _, err = journal.RecordDispatchOutcome(ctx, grant.ID, outcome, ev); err != nil {
		t.Fatal(err)
	}
	// A synthetic independently recorded runtime observation exercises the real
	// journal boundary; no HTTP recipient or actual model execution is claimed.
	if _, err = f.pool.Exec(ctx, `INSERT INTO tool_calls(tool_call_id,run_id,tool_name,args,status,runtime_target,started_at,finished_at,result) VALUES($1,$2,'http.request','{}','completed','fixture',NOW(),NOW(),'{}')`, binding.ToolCallID, runID); err != nil {
		t.Fatal(err)
	}
	if _, err = f.pool.Exec(ctx, `UPDATE runs SET status='finalizing',current_state='finalizing' WHERE run_id=$1`, runID); err != nil {
		t.Fatal(err)
	}
	coordinator := completion.New(f.pool)
	coordinator.DataGuard = f.sources.CheckRunSourcesTx
	coordinator.Evidence = effectEvidence{journal: journal}
	answer := "final"
	if partial {
		answer = "partial"
	}
	final, err := coordinator.Save(ctx, completion.Request{RunID: runID, SessionKey: child.SessionKey, TaskID: child.TaskID, TaskRevision: child.Revision, ConstraintsRevision: child.ConstraintsRevision, Content: "Actual stored attempt", AnswerState: answer})
	if err != nil {
		t.Fatal(err)
	}
	results, err := f.store.Results(f.ctx, child.TaskID)
	if err != nil || len(results) != 1 {
		t.Fatal(err, results)
	}
	result := results[0]
	r := delegation.Report{TaskID: child.TaskID, TaskRevision: child.Revision, ContractRevision: 1, ResultID: result.ResultID, Actions: []delegation.ActionObservation{{ActionID: binding.ActionID, EffectID: binding.ActionID, State: string(verdict), Evidence: []delegation.EvidenceRef{{Kind: "action", ID: ev.EvidenceID, Hash: ev.ResultDigest}}}}, Evidence: []delegation.EvidenceRef{{Kind: "result", ID: result.ResultID, Revision: child.Revision, Hash: final.ContentHash}, {Kind: "artifact", ID: final.ArtifactID, Hash: final.ContentHash}}, Uncertainty: []delegation.Reason{{Kind: "verification_missing"}}}
	if unknown {
		r.Uncertainty = append(r.Uncertainty, delegation.Reason{Kind: "effect_unknown"})
	}
	if partial {
		r.Blockers = []delegation.Reason{{Kind: "execution_failed"}}
	}
	rows, err := f.pool.Query(f.ctx, `SELECT reservation_id FROM execution_budget_reservations WHERE request_json->'scope_ids' ? $1 ORDER BY reservation_id`, budget.ScopeID("delegation", child.OwnerID, child.TaskID))
	if err != nil {
		t.Fatal(err)
	}
	for rows.Next() {
		var id string
		if err = rows.Scan(&id); err != nil {
			t.Fatal(err)
		}
		r.Budget.ReservationIDs = append(r.Budget.ReservationIDs, id)
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		t.Fatal(err)
	}
	var raw []byte
	if err = f.pool.QueryRow(f.ctx, `SELECT used_json FROM execution_budget_scopes WHERE scope_id=$1`, budget.ScopeID("delegation", child.OwnerID, child.TaskID)).Scan(&raw); err != nil {
		t.Fatal(err)
	}
	if err = json.Unmarshal(raw, &r.Budget.Used); err != nil {
		t.Fatal(err)
	}
	var source catalog.SourceRef
	if err = f.pool.QueryRow(f.ctx, `SELECT s.source_id,s.head_revision,s.access_revision,r.content_hash FROM memory_sources s JOIN memory_source_revisions r ON r.source_id=s.source_id AND r.revision=s.head_revision WHERE s.source_id=$1`, "message:"+final.MessageID).Scan(&source.ID, &source.Revision, &source.AccessRevision, &source.Hash); err != nil {
		t.Fatal(err)
	}
	return result, r, append([]catalog.SourceRef{source}, refsForDelegation(c)...)
}
func (f *delegationGuardFixture) report(t *testing.T, key string, child task.Record, r delegation.Report, parents []catalog.SourceRef) delegation.SourceRef {
	t.Helper()
	raw, _ := json.Marshal(r)
	return f.source(t, key, "task", child.TaskID, string(raw), parents)
}

func TestDelegationGuardsExactCapabilitiesIntegration(t *testing.T) {
	f := newDelegationGuardFixture(t)
	if _, err := NewDelegationGuards(nil, delegationPGCapabilities{}); err == nil {
		t.Fatal("missing Catalog accepted")
	}
	if _, err := NewDelegationGuards(f.sources, nil); err == nil {
		t.Fatal("missing permission reader accepted")
	}
	base := f.proposal(t, "exact-capability")
	changes := []func(*task.DelegationRequest){
		func(r *task.DelegationRequest) { r.Permissions[0].Tool = "http.other" }, func(r *task.DelegationRequest) { r.Permissions[0].ContractHash = strings.Repeat("b", 64) },
		func(r *task.DelegationRequest) {
			r.Permissions[0].Resources = []string{"https://fixture.example/other"}
		}, func(r *task.DelegationRequest) { r.Permissions[0].Resources = []string{"*"} },
		func(r *task.DelegationRequest) { r.Permissions[0].Effects = []string{"commit"} }, func(r *task.DelegationRequest) {
			r.Permissions[0].Credentials = []delegation.CredentialRef{{Alias: "other", Field: "password"}}
		},
		func(r *task.DelegationRequest) { r.Budget.ModelTurns++ }, func(r *task.DelegationRequest) { r.Deadline = f.caps.Deadline.Add(time.Second) },
	}
	for i, change := range changes {
		raw, _ := json.Marshal(base)
		var r task.DelegationRequest
		_ = json.Unmarshal(raw, &r)
		change(&r)
		if _, err := f.manager.Admit(f.owner, r); err == nil {
			t.Fatalf("widened permission %d accepted", i)
		}
	}
	var children, receipts int
	if err := f.pool.QueryRow(f.ctx, `SELECT count(*) FROM task_delegations`).Scan(&children); err != nil {
		t.Fatal(err)
	}
	if err := f.pool.QueryRow(f.ctx, `SELECT count(*) FROM execution_budget_reservations`).Scan(&receipts); err != nil {
		t.Fatal(err)
	}
	if children != 0 || receipts != 0 {
		t.Fatal("denial published child or spent budget", children, receipts)
	}
	if _, err := f.manager.Admit(f.owner, base); err != nil {
		t.Fatal(err)
	}
}

func TestDelegationGuardsCurrentOriginalGrantIntegration(t *testing.T) {
	f := newDelegationGuardFixture(t)
	r := f.proposal(t, "scoped-contract")
	selected := f.source(t, "project-data", "project", "guard-project", "selected project DATA", nil)
	r.Sources = []delegation.SourceRef{selected}
	if _, err := f.manager.Admit(f.owner, r); err == nil {
		t.Fatal("owner admission bypassed parent grant")
	}
	if _, err := f.pool.Exec(f.ctx, `INSERT INTO memory_scope_grants(task_id,source_id,task_revision,access_revision,purpose,owner_id) VALUES($1,$2,$3,$4,'memory_read',$5)`, f.parent.TaskID, selected.ID, f.parent.Revision, selected.AccessRevision, f.parent.OwnerID); err != nil {
		t.Fatal(err)
	}
	child, err := f.manager.Admit(f.owner, r)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = f.pool.Exec(f.ctx, `DELETE FROM memory_scope_grants WHERE task_id=$1 AND source_id=$2`, f.parent.TaskID, selected.ID); err != nil {
		t.Fatal(err)
	}
	live, err := f.store.Get(f.ctx, child.Contract.TaskID)
	if err != nil {
		t.Fatal(err)
	}
	tx, err := f.pool.Begin(f.ctx)
	if err != nil {
		t.Fatal(err)
	}
	err = f.guard.CheckLifecycleTx(f.ctx, tx, f.parent, live, child.Contract)
	_ = tx.Rollback(f.ctx)
	if err == nil {
		t.Fatal("copied child grant survived original revoke")
	}
	var copied int
	if err = f.pool.QueryRow(f.ctx, `SELECT count(*) FROM memory_scope_grants WHERE task_id=$1 AND source_id=$2`, child.Contract.TaskID, selected.ID).Scan(&copied); err != nil || copied != 1 {
		t.Fatal("test did not retain copied grant", copied, err)
	}
}

func TestDelegationGuardsActualReportEvidenceIntegration(t *testing.T) {
	f := newDelegationGuardFixture(t)
	c, child := f.admit(t, "report-contract")
	result, r, parents := f.complete(t, c, child, false, false)
	valid := f.report(t, "valid-report", child, r, parents)
	if _, err := f.manager.Collect(f.ctx, f.parent.TaskID, child.TaskID, result.ResultID, valid); err != nil {
		t.Fatal(err)
	}
	for i, change := range []func(*delegation.Report){
		func(r *delegation.Report) { r.Actions = nil }, func(r *delegation.Report) { r.Actions[0].State = "effect_unknown" }, func(r *delegation.Report) { r.Actions[0].Evidence[0].Hash = "" },
		func(r *delegation.Report) { r.Budget.Used.ExternalCalls = 0 }, func(r *delegation.Report) { r.Budget.ReservationIDs = []string{"forged-reservation"} }, func(r *delegation.Report) { r.Uncertainty = nil },
		func(r *delegation.Report) { r.Evidence[0].Hash = "" }, func(r *delegation.Report) { r.ResultID = "different-result" },
	} {
		raw, _ := json.Marshal(r)
		var forged delegation.Report
		_ = json.Unmarshal(raw, &forged)
		change(&forged)
		ref := f.report(t, string(rune('a'+i))+"-forged", child, forged, parents)
		tx, err := f.pool.Begin(f.ctx)
		if err != nil {
			t.Fatal(err)
		}
		err = f.guard.CheckReportTx(f.ctx, tx, f.parent, child, result, ref)
		_ = tx.Rollback(f.ctx)
		if err == nil {
			t.Fatalf("forged report %d accepted", i)
		}
	}
	d := task.DelegationDecision{Accept: true, Report: valid, Evidence: []delegation.SourceRef{{ID: parents[0].ID, Revision: parents[0].Revision, AccessRevision: parents[0].AccessRevision, Hash: parents[0].Hash}}}
	if _, err := f.manager.Decide(f.owner, f.parent.TaskID, child.TaskID, d); err == nil {
		t.Fatal("model result became independent verification")
	}
	d.Accept = false
	if _, err := f.manager.Decide(f.owner, f.parent.TaskID, child.TaskID, d); err != nil {
		t.Fatal(err)
	}
}

func TestDelegationGuardsOwnerVerifiedExactAcceptanceIntegration(t *testing.T) {
	f := newDelegationGuardFixture(t)
	c, child := f.admit(t, "owner-contract")
	result, r, parents := f.complete(t, c, child, false, false)
	ref := delegation.SourceRef{ID: parents[0].ID, Revision: parents[0].Revision, AccessRevision: parents[0].AccessRevision, Hash: parents[0].Hash}
	verified, err := f.store.SaveVerifiedResult(f.owner, task.Result{TaskID: child.TaskID, TaskRevision: child.Revision, RunID: result.RunID, Summary: "Owner confirmed actual evidence", EvidenceRefs: []string{DelegationSourceEvidence(ref), DelegationRecordEvidence(delegation.EvidenceRef{Kind: "action", ID: r.Actions[0].ActionID, Hash: validationHash}), DelegationRecordEvidence(r.Actions[0].Evidence[0])}, VerifiedCriteria: child.CompletionCriteria, Verifier: "owner"})
	if err != nil {
		t.Fatal(err)
	}
	r.ResultID = verified.ResultID
	r.Evidence[0].ID = verified.ResultID
	r.Uncertainty = nil
	report := f.report(t, "owner-report", child, r, parents)
	if _, err = f.manager.Collect(f.ctx, f.parent.TaskID, child.TaskID, verified.ResultID, report); err != nil {
		t.Fatal(err)
	}
	if _, err = f.manager.Decide(f.owner, f.parent.TaskID, child.TaskID, task.DelegationDecision{Accept: true, Report: report, Evidence: []delegation.SourceRef{ref}}); err != nil {
		t.Fatal(err)
	}
	parent, err := f.store.Get(f.ctx, f.parent.TaskID)
	if err != nil || parent.Status != "active" {
		t.Fatal("acceptance completed parent", parent.Status, err)
	}
	live, err := f.store.Get(f.ctx, child.TaskID)
	if err != nil || live.Status != "active" {
		t.Fatal("acceptance completed child", live.Status, err)
	}
}

func TestDelegationGuardsPartialUnknownAndPrivacyIntegration(t *testing.T) {
	f := newDelegationGuardFixture(t)
	c, child := f.admit(t, "unknown-contract")
	result, r, parents := f.complete(t, c, child, true, true)
	valid := f.report(t, "unknown-report", child, r, parents)
	if _, err := f.manager.Collect(f.ctx, f.parent.TaskID, child.TaskID, result.ResultID, valid); err != nil {
		t.Fatal(err)
	}
	tx, err := f.pool.Begin(f.ctx)
	if err != nil {
		t.Fatal(err)
	}
	forged := result
	forged.Verdict = "verified"
	forged.Verifier = "owner"
	err = f.guard.CheckReportTx(f.ctx, tx, f.parent, child, forged, valid)
	_ = tx.Rollback(f.ctx)
	if err == nil {
		t.Fatal("forged owner result accepted")
	}
	if _, err = f.sources.Delete(f.owner, c.ContractSource.ID, "delete", c.ContractSource.Revision); err != nil {
		t.Fatal(err)
	}
	if _, err = f.manager.Collect(f.ctx, f.parent.TaskID, child.TaskID, result.ResultID, valid); err == nil {
		t.Fatal("deleted original graph survived result read")
	}
}

// A partial assistant final is different from a stopped attempt. The latter
// has no current completed result to collect, and must retain its observations
// without acquiring fresh execution or acceptance authority from the report.
func TestDelegationGuardsBlockedAttemptsRetainHistoryIntegration(t *testing.T) {
	t.Run("unknown blocks without changing revision", func(t *testing.T) {
		f := newDelegationGuardFixture(t)
		c, child := f.admit(t, "blocked-unknown-contract")
		result, report, parents := f.complete(t, c, child, true, true)
		ref := f.report(t, "blocked-unknown-report", child, report, parents)
		var token ownership.Token
		err := f.pool.QueryRow(f.ctx, `SELECT session_key,run_id,lease_id,fence_epoch FROM session_ownership WHERE session_key=$1`, child.SessionKey).Scan(&token.SessionKey, &token.RunID, &token.LeaseID, &token.FenceEpoch)
		if err != nil {
			t.Fatal(err)
		}
		owned := ownership.WithToken(f.ctx, f.leases.Authority(), token)
		// Real BlockRun is also used by the ordinary effect_unknown path. This
		// fixture first persisted a final to prove historical evidence survives.
		if err = f.store.BlockRun(owned, result.RunID, "effect_unknown"); err != nil {
			t.Fatal(err)
		}
		blocked, err := f.store.Get(f.ctx, child.TaskID)
		if err != nil || blocked.Status != "blocked" || blocked.Revision != child.Revision {
			t.Fatal("blocked outcome changed semantic revision", blocked, err)
		}
		if _, err = f.manager.Collect(f.ctx, f.parent.TaskID, child.TaskID, result.ResultID, ref); err == nil {
			t.Fatal("blocked task gained authority from a historical report")
		}
		var finals int
		var state string
		if err = f.pool.QueryRow(f.ctx, `SELECT count(*) FROM run_final_results WHERE run_id=$1`, result.RunID).Scan(&finals); err != nil || finals != 1 {
			t.Fatal("lost historical final", finals, err)
		}
		if err = f.pool.QueryRow(f.ctx, `SELECT state FROM action_effects WHERE action_id=$1`, report.Actions[0].EffectID).Scan(&state); err != nil || state != string(actionauth.StateUnknown) {
			t.Fatal("lost unknown effect", state, err)
		}
	})
	for _, state := range []string{"failed", "timed_out"} {
		t.Run(state+" has no completed final", func(t *testing.T) {
			f := newDelegationGuardFixture(t)
			c, child := f.admit(t, "blocked-"+state+"-contract")
			runID := "stopped:" + child.TaskID
			owned := f.worker(t, child, runID)
			// SaveRunOutcome is the actual ordinary failure finalizer. It keeps
			// an unverified attempt note and bumps the blocked task revision.
			if err := f.store.SaveRunOutcome(owned, runID, state, "attempt failed before a final"); err != nil {
				t.Fatal(err)
			}
			blocked, err := f.store.Get(f.ctx, child.TaskID)
			if err != nil || blocked.Status != "blocked" || blocked.Revision != child.Revision+1 {
				t.Fatal("incorrect failure task outcome", blocked, err)
			}
			results, err := f.store.Results(f.ctx, child.TaskID)
			if err != nil || len(results) != 1 || results[0].Verdict != "unverified" || results[0].TaskRevision != child.Revision {
				t.Fatal("lost failed attempt observation", results, err)
			}
			var finals int
			if err = f.pool.QueryRow(f.ctx, `SELECT count(*) FROM run_final_results WHERE run_id=$1`, runID).Scan(&finals); err != nil || finals != 0 {
				t.Fatal("invented final result", finals, err)
			}
			r := delegation.Report{TaskID: child.TaskID, TaskRevision: child.Revision, ContractRevision: c.Revision, ResultID: results[0].ResultID, Blockers: []delegation.Reason{{Kind: "execution_failed"}}}
			ref := f.report(t, "blocked-"+state+"-report", child, r, refsForDelegation(c))
			if _, err = f.manager.Collect(f.ctx, f.parent.TaskID, child.TaskID, results[0].ResultID, ref); err == nil {
				t.Fatal("failed attempt became a completed delegated result")
			}
		})
	}
}

func TestDelegationGuardsCurrentStopAndFiniteWaitIntegration(t *testing.T) {
	f := newDelegationGuardFixture(t)
	r := f.proposal(t, "finite-wait")
	r.Deadline = time.Now().Add(150 * time.Millisecond).UTC().Truncate(time.Microsecond)
	tx, err := f.pool.Begin(f.ctx)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = tx.Exec(f.ctx, `SELECT task_id FROM fixture_parent_capabilities WHERE task_id=$1 FOR UPDATE`, f.parent.TaskID); err != nil {
		t.Fatal(err)
	}
	done := make(chan error, 1)
	go func() { _, err := f.manager.Admit(f.owner, r); done <- err }()
	time.Sleep(250 * time.Millisecond)
	if err = tx.Commit(f.ctx); err != nil {
		t.Fatal(err)
	}
	select {
	case err = <-done:
		if err == nil {
			t.Fatal("expired proposal admitted after authority wait")
		}
	case <-time.After(5 * time.Second):
		t.Fatal("pool2 authority wait deadlock")
	}
	controls := task.NewController(f.store, f.leases.Authority(), f.leases, run.NewPostgresRepository(f.pool), approval.NewPostgresRepository(f.pool), durableinput.NewStore(f.pool), transcript.NewStore(f.pool))
	if _, err = controls.Apply(f.owner, task.ControlRequest{TaskID: f.parent.TaskID, ExpectedRevision: f.parent.Revision, Kind: "stop", Instruction: "stop current tree"}); err != nil {
		t.Fatal(err)
	}
	if _, err = f.manager.Admit(f.owner, f.proposal(t, "stopped-contract")); err == nil {
		t.Fatal("stopped parent admitted")
	}
}

type delegationSignalledCapabilities struct {
	once    sync.Once
	waiting chan struct{}
}

func (r *delegationSignalledCapabilities) ReadDelegationCapabilitiesTx(ctx context.Context, tx pgx.Tx, parent task.Record, childID string) (DelegationCapabilities, error) {
	r.once.Do(func() { close(r.waiting) })
	return (delegationPGCapabilities{}).ReadDelegationCapabilitiesTx(ctx, tx, parent, childID)
}

func TestDelegationGuardsCollectStopOrderingIntegration(t *testing.T) {
	f := newDelegationGuardFixture(t)
	c, child := f.admit(t, "stop-contract")
	result, r, parents := f.complete(t, c, child, false, false)
	report := f.report(t, "stop-report", child, r, parents)
	blocker, err := f.pool.Begin(f.ctx)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = blocker.Exec(f.ctx, `SELECT task_id FROM fixture_parent_capabilities WHERE task_id=$1 FOR UPDATE`, f.parent.TaskID); err != nil {
		t.Fatal(err)
	}
	reader := &delegationSignalledCapabilities{waiting: make(chan struct{})}
	f.guard.permissions = reader
	collect := make(chan error, 1)
	go func() {
		_, err := f.manager.Collect(f.ctx, f.parent.TaskID, child.TaskID, result.ResultID, report)
		collect <- err
	}()
	select {
	case <-reader.waiting:
	case <-time.After(3 * time.Second):
		t.Fatal("collection did not reach current metadata wait")
	}
	controls := task.NewController(f.store, f.leases.Authority(), f.leases, run.NewPostgresRepository(f.pool), approval.NewPostgresRepository(f.pool), durableinput.NewStore(f.pool), transcript.NewStore(f.pool))
	stop := make(chan error, 1)
	go func() {
		_, err := controls.Apply(f.owner, task.ControlRequest{TaskID: f.parent.TaskID, ExpectedRevision: f.parent.Revision, Kind: "stop", Instruction: "stop after independent report observation"})
		stop <- err
	}()
	if err = blocker.Commit(f.ctx); err != nil {
		t.Fatal(err)
	}
	for _, done := range []chan error{collect, stop} {
		select {
		case err = <-done:
			if err != nil {
				t.Fatal(err)
			}
		case <-time.After(5 * time.Second):
			t.Fatal("pool2 collect/stop deadlock")
		}
	}
	if _, err = f.manager.Collect(f.ctx, f.parent.TaskID, child.TaskID, result.ResultID, report); err == nil {
		t.Fatal("late collection after stop accepted")
	}
	var state, retained string
	if err = f.pool.QueryRow(f.ctx, `SELECT state,result_id FROM task_delegations WHERE task_id=$1`, child.TaskID).Scan(&state, &retained); err != nil || state != "result_ready" || retained != result.ResultID {
		t.Fatal("stop lost prior truthful result", state, retained, err)
	}
	var taskState string
	if err = f.pool.QueryRow(f.ctx, `SELECT status FROM tasks WHERE task_id=$1`, child.TaskID).Scan(&taskState); err != nil || taskState != "cancelled" {
		t.Fatal("stop missed child task", taskState, err)
	}
	var effect string
	if err = f.pool.QueryRow(f.ctx, `SELECT state FROM action_effects WHERE action_id=$1`, r.Actions[0].EffectID).Scan(&effect); err != nil || effect != "effect_confirmed" {
		t.Fatal("stop rewrote historical effect", effect, err)
	}
}

func TestDelegationGuardsReportScopeAncestryAndSecretIntegration(t *testing.T) {
	f := newDelegationGuardFixture(t)
	r := f.proposal(t, "secret-contract")
	s, err := f.sources.Put(f.owner, catalog.PutSource{ExternalKey: "secret-selected", Kind: "document", ScopeType: "user", ScopeID: f.parent.OwnerID, Body: "bounded sensitive fixture", Origin: content.Origin{Kind: content.Document, Sensitivity: content.Secret}})
	if err != nil {
		t.Fatal(err)
	}
	r.Sources = []delegation.SourceRef{guardSource(s)}
	if _, err = f.manager.Admit(f.owner, r); err == nil {
		t.Fatal("Secret source admitted")
	}
	c, child := f.admit(t, "ancestry-contract")
	result, report, parents := f.complete(t, c, child, false, false)
	// The actual final source already retains all original contract ancestors;
	// a single final root is valid. Missing that root or the entire graph is not.
	for i, p := range [][]catalog.SourceRef{nil, refsForDelegation(c)} {
		ref := f.report(t, "missing-ancestry:"+string(rune('a'+i)), child, report, p)
		tx, err := f.pool.Begin(f.ctx)
		if err != nil {
			t.Fatal(err)
		}
		err = f.guard.CheckReportTx(f.ctx, tx, f.parent, child, result, ref)
		_ = tx.Rollback(f.ctx)
		if err == nil {
			t.Fatal("missing contract or actual final-source edge accepted")
		}
	}
	extra := f.source(t, "unselected-report-data", "user", f.parent.OwnerID, "unselected owner DATA", nil)
	ref := f.report(t, "widened-report-graph", child, report, append(append([]catalog.SourceRef(nil), parents...), catalogRefs([]delegation.SourceRef{extra})...))
	tx, err := f.pool.Begin(f.ctx)
	if err != nil {
		t.Fatal(err)
	}
	err = f.guard.CheckReportTx(f.ctx, tx, f.parent, child, result, ref)
	_ = tx.Rollback(f.ctx)
	if err == nil {
		t.Fatal("report added unselected owner context")
	}
	body, _ := json.Marshal(report)
	foreign := f.source(t, "user-report", "user", f.parent.OwnerID, string(body), parents)
	tx, err = f.pool.Begin(f.ctx)
	if err != nil {
		t.Fatal(err)
	}
	err = f.guard.CheckReportTx(f.ctx, tx, f.parent, child, result, foreign)
	_ = tx.Rollback(f.ctx)
	if err == nil {
		t.Fatal("report body outside child scope accepted")
	}
}

func TestDelegationGuardsActualGoalMandateIntegration(t *testing.T) {
	f := newDelegationGuardFixture(t)
	m := goal.New(f.pool, f.store, nil)
	objective, err := m.PutGoal(f.owner, goal.GoalRequest{Goal: actionauth.GoalPolicy{Kind: "objective", Status: "active", Title: "Bounded actual owner goal", Purpose: "guard_purpose", ExpiresAt: time.Now().Add(24 * time.Hour)}})
	if err != nil {
		t.Fatal(err)
	}
	a := f.caps.Budget
	mandate, err := m.PutMandate(owneridentity.WithActor(f.ctx, owneridentity.Actor{OwnerID: f.parent.OwnerID, Channel: "web", DeliveryID: "guard-mandate"}), goal.MandateRequest{Mandate: actionauth.Mandate{GoalID: objective.SubjectID, GoalRevision: objective.Revision, Status: "active", Purpose: "guard_purpose", Tools: []string{"http.request"}, ResourceScopes: []actionauth.ResourceScope{{Origin: "https://fixture.example", PathPrefix: "/read"}}, Effects: []actionauth.Effect{actionauth.EffectRead}, Privacy: "local_only", Budget: actionauth.MandateBudget{ModelTurns: a.ModelTurns, Tokens: a.Tokens, ExternalCalls: a.ExternalCalls, Children: a.Children, ActiveMillis: a.ActiveMillis, EgressBytes: a.EgressBytes, ProviderInputBytes: a.ProviderInputBytes}, ExpiresAt: time.Now().Add(2 * time.Hour), Intervention: []string{"scope_change", "effect_unknown", "budget_exhausted"}, Reporting: "exceptions"}})
	if err != nil {
		t.Fatal(err)
	}
	parent, err := m.CreateOwnerTask(owneridentity.WithActor(f.ctx, owneridentity.Actor{OwnerID: f.parent.OwnerID, Channel: "web", DeliveryID: "guard-owner-task"}), goal.TaskRequest{SessionKey: f.parent.SessionKey, GoalID: objective.SubjectID, MandateID: mandate.SubjectID, Purpose: "guard_purpose", Goal: "Scoped owner task", CompletionCriteria: []string{"actual evidence"}, InitiativeReason: "explicit owner work", ObservationRef: "fixture:owner"})
	if err != nil {
		t.Fatal(err)
	}
	f.parent = parent
	f.caps.TaskID = parent.TaskID
	f.saveCaps(t)
	if _, err = f.manager.Admit(f.owner, f.proposal(t, "assigned-contract")); err != nil {
		t.Fatal(err)
	}
	f.caps.Permissions[0].Resources = []string{"https://fixture.example/other"}
	f.saveCaps(t)
	if _, err = f.manager.Admit(f.owner, f.proposal(t, "outside-mandate-contract")); err == nil {
		t.Fatal("metadata reader widened actual owner mandate")
	}
}

type delegationPhaseCapabilities struct {
	mu       sync.Mutex
	children []string
}

func (r *delegationPhaseCapabilities) ReadDelegationCapabilitiesTx(ctx context.Context, tx pgx.Tx, parent task.Record, childID string) (DelegationCapabilities, error) {
	r.mu.Lock()
	r.children = append(r.children, childID)
	r.mu.Unlock()
	return (delegationPGCapabilities{}).ReadDelegationCapabilitiesTx(ctx, tx, parent, childID)
}
func TestDelegationGuardsExplicitPhaseAndParentCrashIntegration(t *testing.T) {
	f := newDelegationGuardFixture(t)
	request := f.proposal(t, "phase-contract")
	reader := &delegationPhaseCapabilities{}
	f.guard.permissions = reader
	if _, err := f.pool.Exec(f.ctx, `INSERT INTO runs(run_id,session_key,input_event_id,status,current_state,autonomy_mode,model_provider,started_at,updated_at,task_id,task_revision) VALUES('parent-proof','guard-root','fixture','model_running','model_running','mode_1','fixture',clock_timestamp(),clock_timestamp(),$1,$2)`, f.parent.TaskID, f.parent.Revision); err != nil {
		t.Fatal(err)
	}
	lease, err := f.leases.AcquireLease(f.ctx, session.AcquireLeaseParams{SessionKey: f.parent.SessionKey, RunID: "parent-proof", OwnerID: "parent-worker", TTL: time.Minute})
	if err != nil {
		t.Fatal(err)
	}
	owned := ownership.WithToken(f.ctx, f.leases.Authority(), ownership.Token{SessionKey: f.parent.SessionKey, RunID: "parent-proof", LeaseID: lease.LeaseID, FenceEpoch: lease.FenceEpoch})
	accepted, err := f.manager.Admit(owned, request)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = f.leases.ReleaseLease(f.ctx, lease.LeaseID); err != nil {
		t.Fatal(err)
	}
	child, err := f.store.Get(f.ctx, accepted.Contract.TaskID)
	if err != nil {
		t.Fatal(err)
	}
	tx, err := f.pool.Begin(f.ctx)
	if err != nil {
		t.Fatal(err)
	}
	err = f.guard.CheckLifecycleTx(f.ctx, tx, f.parent, child, accepted.Contract)
	_ = tx.Rollback(f.ctx)
	if err != nil {
		t.Fatal("durable lifecycle borrowed ended parent worker", err)
	}
	reader.mu.Lock()
	defer reader.mu.Unlock()
	proposal, lifecycle := 0, 0
	for _, id := range reader.children {
		if id == "" {
			proposal++
		} else if id == child.TaskID {
			lifecycle++
		} else {
			t.Fatal("wrong lifecycle child", id)
		}
	}
	if proposal != 2 || lifecycle < 2 {
		t.Fatal("phase identity missing", reader.children)
	}
}
