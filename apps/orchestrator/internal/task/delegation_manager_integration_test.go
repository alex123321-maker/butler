package task_test

import (
	"context"
	"encoding/json"
	"errors"
	"path/filepath"
	"sort"
	"strings"
	"testing"
	"time"

	"github.com/butler/butler/apps/orchestrator/internal/approval"
	durableinput "github.com/butler/butler/apps/orchestrator/internal/input"
	"github.com/butler/butler/apps/orchestrator/internal/run"
	"github.com/butler/butler/apps/orchestrator/internal/session"
	"github.com/butler/butler/apps/orchestrator/internal/task"
	"github.com/butler/butler/internal/budget"
	"github.com/butler/butler/internal/delegation"
	"github.com/butler/butler/internal/memory/catalog"
	"github.com/butler/butler/internal/memory/memorytest"
	"github.com/butler/butler/internal/memory/transcript"
	"github.com/butler/butler/internal/owneridentity"
	"github.com/butler/butler/internal/ownership"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// Explicit policy fixture checks real current Catalog snapshots in this TX.
// Root separately owns production transitive grant/Broker/context composition.
type delegationFixtureGuard struct {
	catalog       *catalog.Store
	denyBind      bool
	afterBind     func()
	lifecycleWait func(context.Context, pgx.Tx) error
}

func (g *delegationFixtureGuard) sources(ctx context.Context, tx pgx.Tx, owner string, refs []delegation.SourceRef) error {
	if err := g.catalog.CheckRestoreTx(ctx, tx); err != nil {
		return err
	}
	refs = append([]delegation.SourceRef(nil), refs...)
	sort.Slice(refs, func(i, j int) bool { return refs[i].ID < refs[j].ID })
	for _, s := range refs {
		var valid bool
		err := tx.QueryRow(ctx, `SELECT s.owner_id=$2 AND s.head_revision=$3 AND s.access_revision=$4 AND s.status='active' AND r.content_hash=$5 FROM memory_sources s JOIN memory_source_revisions r ON r.source_id=s.source_id AND r.revision=s.head_revision WHERE s.source_id=$1 FOR SHARE OF s,r`, s.ID, owner, s.Revision, s.AccessRevision, s.Hash).Scan(&valid)
		if err != nil || !valid {
			return delegation.ErrDenied
		}
	}
	return nil
}
func (g *delegationFixtureGuard) CheckProposalTx(ctx context.Context, tx pgx.Tx, parent task.Record, r task.DelegationRequest) error {
	for _, p := range r.Permissions {
		if p.Tool != "http.request" || p.ContractHash != strings.Repeat("a", 64) || len(p.Credentials) != 0 {
			return delegation.ErrDenied
		}
		for _, resource := range p.Resources {
			if resource != "https://fixture.example/read" {
				return delegation.ErrDenied
			}
		}
		for _, effect := range p.Effects {
			if effect != "read" {
				return delegation.ErrDenied
			}
		}
	}
	return g.sources(ctx, tx, parent.OwnerID, append(append([]delegation.SourceRef(nil), r.Sources...), r.ContractSource))
}
func (g *delegationFixtureGuard) BindContractSourcesTx(ctx context.Context, tx pgx.Tx, parent, child task.Record, c delegation.Contract) error {
	if err := g.sources(ctx, tx, parent.OwnerID, append(append([]delegation.SourceRef(nil), c.Sources...), c.ContractSource)); err != nil {
		return err
	}
	if _, err := tx.Exec(ctx, `INSERT INTO fixture_delegation_sources(task_id,source_id) VALUES($1,$2)`, child.TaskID, c.ContractSource.ID); err != nil {
		return err
	}
	if g.afterBind != nil {
		g.afterBind()
	}
	if g.denyBind {
		return delegation.ErrDenied
	}
	return nil
}
func (g *delegationFixtureGuard) CheckLifecycleTx(ctx context.Context, tx pgx.Tx, parent, child task.Record, c delegation.Contract) error {
	if err := g.sources(ctx, tx, parent.OwnerID, append(append([]delegation.SourceRef(nil), c.Sources...), c.ContractSource)); err != nil {
		return err
	}
	if g.lifecycleWait != nil {
		return g.lifecycleWait(ctx, tx)
	}
	return nil
}
func (g *delegationFixtureGuard) CheckReportTx(ctx context.Context, tx pgx.Tx, parent, child task.Record, result task.Result, ref delegation.SourceRef) error {
	if err := g.sources(ctx, tx, parent.OwnerID, []delegation.SourceRef{ref}); err != nil {
		return err
	}
	var body string
	if err := tx.QueryRow(ctx, `SELECT body FROM memory_source_revisions WHERE source_id=$1 AND revision=$2`, ref.ID, ref.Revision).Scan(&body); err != nil {
		return err
	}
	r, err := delegation.ParseReport([]byte(body))
	if err != nil {
		return err
	}
	if r.TaskID != child.TaskID || r.TaskRevision != child.Revision || r.ResultID != result.ResultID {
		return delegation.ErrDenied
	}
	return nil
}
func (g *delegationFixtureGuard) CheckDecisionTx(ctx context.Context, tx pgx.Tx, parent, child task.Record, result task.Result, d task.DelegationDecision) error {
	if err := g.CheckReportTx(ctx, tx, parent, child, result, d.Report); err != nil {
		return err
	}
	if err := g.sources(ctx, tx, parent.OwnerID, d.Evidence); err != nil {
		return err
	}
	if d.Accept && (result.Verdict != "verified" || result.Verifier != "owner") {
		return delegation.ErrDenied
	}
	return nil
}

type delegationFixture struct {
	ctx, owner context.Context
	pool       *pgxpool.Pool
	store      *task.Store
	manager    *task.DelegationManager
	guard      *delegationFixtureGuard
	catalog    *catalog.Store
	leases     *session.PostgresLeaseManager
	root       task.Record
	budget     *budget.Controller
}

func newDelegationFixture(t *testing.T, parallel int, children int64) *delegationFixture {
	t.Helper()
	pool := memorytest.Database(t)
	ctx, cancel := context.WithTimeout(context.Background(), 45*time.Second)
	t.Cleanup(cancel)
	owner := owneridentity.WithActor(ctx, owneridentity.Actor{OwnerID: "delegation-owner", Channel: "web", DeliveryID: "fixture"})
	manifest := catalog.NewFileManifest(filepath.Join(t.TempDir(), "deletions.log"))
	if err := manifest.Initialize(); err != nil {
		t.Fatal(err)
	}
	catalogStore := catalog.New(pool, manifest)
	if _, err := pool.Exec(ctx, `INSERT INTO sessions(session_key,user_id,channel) VALUES('delegation-root','delegation-owner','web');CREATE TABLE fixture_delegation_sources(task_id text,source_id text)`); err != nil {
		t.Fatal(err)
	}
	policy := budget.DefaultPolicy()
	policy.Task.Children = children
	bc := budget.NewController(pool, policy)
	store := task.NewStore(pool)
	store.SetBudgets(bc, "")
	root, err := store.Create(ctx, task.CreateRequest{SessionKey: "delegation-root", OwnerID: "delegation-owner", Goal: "independent research", CompletionCriteria: []string{"objective evidence"}})
	if err != nil {
		t.Fatal(err)
	}
	guard := &delegationFixtureGuard{catalog: catalogStore}
	limits := task.DefaultDelegationLimits()
	limits.MaxParallel = parallel
	manager, err := task.NewDelegationManager(store, session.NewPostgresRepository(pool), durableinput.NewStore(pool), guard, limits)
	if err != nil {
		t.Fatal(err)
	}
	return &delegationFixture{ctx, owner, pool, store, manager, guard, catalogStore, session.NewPostgresLeaseManager(pool), root, bc}
}
func (f *delegationFixture) source(t *testing.T, key, body string) delegation.SourceRef {
	t.Helper()
	s, err := f.catalog.Put(f.owner, catalog.PutSource{ExternalKey: key, Kind: "document", ScopeType: "user", ScopeID: "delegation-owner", Title: "Fixture data", Body: body})
	if err != nil {
		t.Fatal(err)
	}
	return delegation.SourceRef{ID: s.ID, Revision: s.Revision, AccessRevision: s.AccessRevision, Hash: s.Hash}
}
func (f *delegationFixture) proposal(t *testing.T, parent task.Record, key string) task.DelegationRequest {
	t.Helper()
	return task.DelegationRequest{ParentTaskID: parent.TaskID, ParentRevision: parent.Revision, ConstraintsRevision: parent.ConstraintsRevision, ContractSource: f.source(t, key, "bounded objective and criteria"), Permissions: []delegation.Permission{{Tool: "http.request", ContractHash: strings.Repeat("a", 64), Resources: []string{"https://fixture.example/read"}, Effects: []string{"read"}}}, Budget: delegation.Limits{ModelTurns: 4, Tokens: 1000, Children: 2, ActiveMillis: 10000}, Deadline: time.Now().Add(10 * time.Minute)}
}
func (f *delegationFixture) admit(t *testing.T, key string) task.DelegationRecord {
	t.Helper()
	r, err := f.manager.Admit(f.owner, f.proposal(t, f.root, key))
	if err != nil {
		t.Fatal(err)
	}
	return r
}
func (f *delegationFixture) result(t *testing.T, c task.DelegationRecord, verified bool) (task.Result, delegation.SourceRef) {
	t.Helper()
	child, err := f.store.Get(f.ctx, c.Contract.TaskID)
	if err != nil {
		t.Fatal(err)
	}
	runID := "fixture-run:" + child.TaskID
	if _, err = f.pool.Exec(f.ctx, `INSERT INTO runs(run_id,session_key,input_event_id,status,current_state,autonomy_mode,model_provider,started_at,updated_at,task_id,task_revision) VALUES($1,$2,$1,'model_running','model_running','mode_1','fixture',clock_timestamp(),clock_timestamp(),$3,$4)`, runID, child.SessionKey, child.TaskID, child.Revision); err != nil {
		t.Fatal(err)
	}
	l, err := f.leases.AcquireLease(f.ctx, session.AcquireLeaseParams{SessionKey: child.SessionKey, RunID: runID, OwnerID: "fixture-worker", TTL: time.Minute})
	if err != nil {
		t.Fatal(err)
	}
	owned := ownership.WithToken(f.ctx, f.leases.Authority(), ownership.Token{SessionKey: child.SessionKey, RunID: runID, LeaseID: l.LeaseID, FenceEpoch: l.FenceEpoch})
	if _, err = f.manager.MarkExecuting(owned, child.TaskID); err != nil {
		t.Fatal(err)
	}
	if _, err = f.pool.Exec(f.ctx, `UPDATE runs SET status='completed',current_state='completed' WHERE run_id=$1`, runID); err != nil {
		t.Fatal(err)
	}
	var result task.Result
	if verified {
		result, err = f.store.SaveVerifiedResult(f.owner, task.Result{TaskID: child.TaskID, TaskRevision: child.Revision, RunID: runID, Summary: "fixture verified observation", EvidenceRefs: []string{"fixture-observation"}, VerifiedCriteria: child.CompletionCriteria, Verifier: "owner"})
	} else {
		err = f.store.SaveRunOutcome(f.ctx, runID, "completed", "unverified fixture output")
		results, e := f.store.Results(f.ctx, child.TaskID)
		if e != nil {
			t.Fatal(e)
		}
		if len(results) != 1 {
			t.Fatal("result missing")
		}
		result = results[0]
	}
	if err != nil {
		t.Fatal(err)
	}
	report := delegation.Report{TaskID: child.TaskID, TaskRevision: child.Revision, ContractRevision: 1, ResultID: result.ResultID, Evidence: []delegation.EvidenceRef{{Kind: "result", ID: result.ResultID}}}
	body, _ := json.Marshal(report)
	return result, f.source(t, "report:"+child.TaskID, string(body))
}

func TestDelegationSingleExecutorRestartAndResultDecisionIntegration(t *testing.T) {
	f := newDelegationFixture(t, 1, 8)
	request := f.proposal(t, f.root, "first")
	a, err := f.manager.Admit(f.owner, request)
	if err != nil {
		t.Fatal(err)
	}
	b := f.admit(t, "second")
	if a.Contract.State != "queued" || b.Contract.State != "waiting" || a.Contract.InputID == "" || b.Contract.InputID != "" {
		t.Fatal("single executor bound", a.Contract.State, b.Contract.State)
	}
	repeat, err := f.manager.Admit(f.owner, request)
	if err != nil || repeat.Contract.TaskID != a.Contract.TaskID {
		t.Fatal("durable duplicate changed identity", err)
	}
	request.Budget.Tokens--
	if _, err = f.manager.Admit(f.owner, request); !errors.Is(err, task.ErrConflict) {
		t.Fatal("changed proposal replay", err)
	}
	child, _ := f.store.Get(f.ctx, a.Contract.TaskID)
	if child.SessionKey == f.root.SessionKey || len(child.Working.Plan) != 0 {
		t.Fatal("child inherited parent identity/state")
	}
	receipt, err := durableinput.NewStore(f.pool).Get(f.ctx, a.Contract.InputID)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(receipt.Event.PayloadJSON, "bounded objective") || strings.Contains(receipt.Event.PayloadJSON, "user_id") || receipt.MaxAttempts != 3 || !receipt.ExpiresAt.Equal(a.Contract.Deadline) {
		t.Fatal("raw context or unbounded receipt", receipt.Event.PayloadJSON)
	}
	tx, err := f.pool.Begin(f.ctx)
	if err != nil {
		t.Fatal(err)
	}
	_, err = durableinput.NewStore(f.pool).AcceptBoundedTx(f.ctx, tx, delegation.InputRequest{OwnerID: child.OwnerID, SessionKey: child.SessionKey, TaskID: child.TaskID, EventID: "delegation:" + child.TaskID, ContractSource: a.Contract.ContractSource, Deadline: a.Contract.Deadline.Add(time.Minute), MaxAttempts: 3})
	_ = tx.Rollback(f.ctx)
	if err == nil {
		t.Fatal("retry extended immutable child receipt deadline")
	}
	r, report := f.result(t, a, true)
	if _, err = f.manager.Collect(f.ctx, f.root.TaskID, child.TaskID, r.ResultID, report); err != nil {
		t.Fatal(err)
	}
	if _, err = f.manager.Decide(f.owner, f.root.TaskID, child.TaskID, task.DelegationDecision{Accept: true, Report: report, Evidence: []delegation.SourceRef{report}}); err != nil {
		t.Fatal(err)
	}
	current, _ := f.store.Get(f.ctx, child.TaskID)
	if current.Status != "active" {
		t.Fatal("delegation acceptance invented task completion")
	}
	saved, err := f.manager.Get(f.ctx, child.TaskID)
	if err != nil || saved.Contract.State != "accepted" || saved.ResultID != r.ResultID || saved.Decision == nil || saved.Report == nil || *saved.Report != report {
		t.Fatal("saved metadata projection", saved, err)
	}
	// A new manager has no process-local parent/session state. Maintenance uses
	// current task authority rather than resurrecting a completed parent lease.
	m, err := task.NewDelegationManager(f.store, session.NewPostgresRepository(f.pool), durableinput.NewStore(f.pool), f.guard, task.DefaultDelegationLimits())
	if err != nil {
		t.Fatal(err)
	}
	advanced, err := m.Advance(f.ctx, f.root.TaskID)
	if err != nil || len(advanced) != 1 || advanced[0].Contract.State != "queued" {
		t.Fatal("restart lost waiting work", advanced, err)
	}
}

func TestDelegationBoundedParallelBudgetAndAdmissionRollbackIntegration(t *testing.T) {
	f := newDelegationFixture(t, 2, 2)
	requests := []task.DelegationRequest{f.proposal(t, f.root, "a"), f.proposal(t, f.root, "b")}
	start := make(chan struct{})
	out := make(chan error, 2)
	for _, r := range requests {
		go func(r task.DelegationRequest) { <-start; _, err := f.manager.Admit(f.owner, r); out <- err }(r)
	}
	close(start)
	for range 2 {
		if err := <-out; err != nil {
			t.Fatal(err)
		}
	}
	if _, err := f.manager.Admit(f.owner, f.proposal(t, f.root, "third")); err == nil {
		t.Fatal("shared root child allowance reset")
	}
	var branches, inputs int
	if err := f.pool.QueryRow(f.ctx, `SELECT count(*) FROM task_delegations WHERE state='queued'`).Scan(&branches); err != nil {
		t.Fatal(err)
	}
	if err := f.pool.QueryRow(f.ctx, `SELECT count(*) FROM accepted_inputs WHERE source='delegation'`).Scan(&inputs); err != nil || branches != 2 || inputs != 2 {
		t.Fatal("failed budget admission leaked durable child", branches, inputs, err)
	}
	used, err := f.budget.Ledger.Get(f.ctx, budget.ScopeID("task", "delegation-owner", f.root.TaskID))
	if err != nil || used.Used.Children != 2 {
		t.Fatal("actual root spend", used.Used, err)
	}
	// Force denial AFTER task insertion, child reservation and bound source write.
	g := newDelegationFixture(t, 1, 8)
	g.guard.denyBind = true
	if _, err = g.manager.Admit(g.owner, g.proposal(t, g.root, "rolled-back")); err == nil {
		t.Fatal("late source bind denial ignored")
	}
	for _, table := range []string{"task_delegations", "fixture_delegation_sources", "accepted_inputs", "execution_budget_reservations"} {
		var n int
		if err = g.pool.QueryRow(g.ctx, `SELECT count(*) FROM `+table).Scan(&n); err != nil || n != 0 {
			t.Fatal("outer rollback leaked", table, n, err)
		}
	}
	var count int
	if err = g.pool.QueryRow(g.ctx, `SELECT count(*) FROM sessions`).Scan(&count); err != nil || count != 1 {
		t.Fatal("orphan child session", count, err)
	}
}

func TestDelegationTotalBranchCapAndMissingAuthorityIntegration(t *testing.T) {
	f := newDelegationFixture(t, 1, 8)
	limits := task.DefaultDelegationLimits()
	limits.MaxChildren = 2
	m, err := task.NewDelegationManager(f.store, session.NewPostgresRepository(f.pool), durableinput.NewStore(f.pool), f.guard, limits)
	if err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"one", "two"} {
		if _, err = m.Admit(f.owner, f.proposal(t, f.root, name)); err != nil {
			t.Fatal(err)
		}
	}
	if _, err = m.Admit(f.owner, f.proposal(t, f.root, "three")); err == nil {
		t.Fatal("tree cap bypassed independent of remaining budget")
	}
	if _, err = m.Admit(f.ctx, f.proposal(t, f.root, "anonymous")); err == nil {
		t.Fatal("proposal created owner authority from IDs")
	}
	if _, err = task.NewDelegationManager(f.store, session.NewPostgresRepository(f.pool), durableinput.NewStore(f.pool), nil, limits); err == nil {
		t.Fatal("nil policy enabled manager")
	}
	used, err := f.budget.Ledger.Get(f.ctx, budget.ScopeID("task", "delegation-owner", f.root.TaskID))
	if err != nil || used.Used.Children != 2 {
		t.Fatal("cap denial charged/leaked another child", used.Used, err)
	}
}

func TestDelegationWorkerLeaseExpiresDuringAuthorityWaitIntegration(t *testing.T) {
	f := newDelegationFixture(t, 1, 8)
	c := f.admit(t, "lease-wait")
	child, err := f.store.Get(f.ctx, c.Contract.TaskID)
	if err != nil {
		t.Fatal(err)
	}
	runID := "waiting-worker:" + child.TaskID
	if _, err = f.pool.Exec(f.ctx, `INSERT INTO runs(run_id,session_key,input_event_id,status,current_state,autonomy_mode,model_provider,started_at,updated_at,task_id,task_revision) VALUES($1,$2,$1,'model_running','model_running','mode_1','fixture',clock_timestamp(),clock_timestamp(),$3,$4)`, runID, child.SessionKey, child.TaskID, child.Revision); err != nil {
		t.Fatal(err)
	}
	l, err := f.leases.AcquireLease(f.ctx, session.AcquireLeaseParams{SessionKey: child.SessionKey, RunID: runID, OwnerID: "fixture-worker", TTL: time.Minute})
	if err != nil {
		t.Fatal(err)
	}
	owned := ownership.WithToken(f.ctx, f.leases.Authority(), ownership.Token{SessionKey: child.SessionKey, RunID: runID, LeaseID: l.LeaseID, FenceEpoch: l.FenceEpoch})
	waited := false
	f.guard.lifecycleWait = func(ctx context.Context, tx pgx.Tx) error {
		// The first private token check already passed. Emulate time spent waiting
		// for source/authority validation before the authority write.
		waited = true
		if _, err := tx.Exec(ctx, `UPDATE session_ownership SET expires_at=clock_timestamp()+interval '40 milliseconds' WHERE session_key=$1`, child.SessionKey); err != nil {
			return err
		}
		_, err := tx.Exec(ctx, `SELECT pg_sleep(0.08)`)
		return err
	}
	if _, err = f.manager.MarkExecuting(owned, child.TaskID); !errors.Is(err, ownership.ErrStale) || !waited {
		t.Fatal("late worker advanced after lease expiry", waited, err)
	}
	f.guard.lifecycleWait = nil
	saved, err := f.manager.Get(f.ctx, child.TaskID)
	if err != nil || saved.Contract.State != "queued" {
		t.Fatal("denied worker changed durable lifecycle", saved.Contract.State, err)
	}
}

func TestDelegationDependencyCyclesConflictsAndSourceRevocationIntegration(t *testing.T) {
	f := newDelegationFixture(t, 1, 8)
	a := f.admit(t, "active")
	b := f.admit(t, "waiting-b")
	c := f.admit(t, "waiting-c")
	if _, err := f.manager.SetDependencies(f.owner, f.root.TaskID, b.Contract.TaskID, []string{c.Contract.TaskID}); err != nil {
		t.Fatal(err)
	}
	if _, err := f.manager.SetDependencies(f.owner, f.root.TaskID, c.Contract.TaskID, []string{b.Contract.TaskID}); !errors.Is(err, task.ErrConflict) {
		t.Fatal("cycle accepted", err)
	}
	r, report := f.result(t, a, false)
	if _, err := f.manager.Collect(f.ctx, f.root.TaskID, a.Contract.TaskID, r.ResultID, report); err != nil {
		t.Fatal(err)
	}
	d := task.DelegationDecision{Accept: true, Report: report, Evidence: []delegation.SourceRef{report}}
	if _, err := f.manager.Decide(f.owner, f.root.TaskID, a.Contract.TaskID, d); err == nil {
		t.Fatal("model agreement became verification")
	}
	d.Accept = false
	if _, err := f.manager.Decide(f.owner, f.root.TaskID, a.Contract.TaskID, d); err != nil {
		t.Fatal(err)
	}
	if _, err := f.manager.SetDependencies(f.owner, f.root.TaskID, c.Contract.TaskID, []string{a.Contract.TaskID}); err != nil {
		t.Fatal(err)
	}
	states, err := f.manager.Advance(f.ctx, f.root.TaskID)
	if err != nil {
		t.Fatal(err)
	}
	var blocked bool
	for _, v := range states {
		if v.Contract.TaskID == c.Contract.TaskID && v.Contract.State == "blocked" {
			blocked = true
		}
	}
	if !blocked {
		t.Fatal("rejected dependency silently resumed")
	}
	// Current source hash/access revision is checked before maintenance publishes
	// even a source-only receipt, without borrowing any ended executor lease.
	if _, err = f.pool.Exec(f.ctx, `UPDATE memory_sources SET access_revision=access_revision+1 WHERE source_id=$1`, b.Contract.ContractSource.ID); err != nil {
		t.Fatal(err)
	}
	if _, err = f.manager.Advance(f.ctx, f.root.TaskID); err == nil {
		t.Fatal("stale source contract resumed")
	}
}

func TestDelegationStaleParentSubsetDepthAndStopRaceIntegration(t *testing.T) {
	f := newDelegationFixture(t, 2, 8)
	a := f.admit(t, "branch-one")
	child, _ := f.store.Get(f.ctx, a.Contract.TaskID)
	proposal := f.proposal(t, child, "expanded")
	proposal.Permissions[0].Effects = []string{"commit"}
	if _, err := f.manager.Admit(f.owner, proposal); err == nil {
		t.Fatal("descendant broadened permission")
	}
	proposal = f.proposal(t, child, "too-large")
	proposal.Budget.Tokens = a.Contract.Budget.Tokens + 1
	if _, err := f.manager.Admit(f.owner, proposal); err == nil {
		t.Fatal("descendant broadened budget")
	}
	proposal = f.proposal(t, child, "narrow-child")
	proposal.Deadline = a.Contract.Deadline
	proposal.Budget = a.Contract.Budget
	b, err := f.manager.Admit(f.owner, proposal)
	if err != nil {
		t.Fatal(err)
	}
	grandchild, _ := f.store.Get(f.ctx, b.Contract.TaskID)
	third := f.proposal(t, grandchild, "third-depth")
	third.Deadline = b.Contract.Deadline
	third.Budget = b.Contract.Budget
	c, err := f.manager.Admit(f.owner, third)
	if err != nil {
		t.Fatal(err)
	}
	deep, _ := f.store.Get(f.ctx, c.Contract.TaskID)
	released, report := f.result(t, a, true)
	if _, err = f.manager.Collect(f.ctx, f.root.TaskID, a.Contract.TaskID, released.ResultID, report); err != nil {
		t.Fatal(err)
	}
	if _, err = f.manager.Advance(f.ctx, grandchild.TaskID); err != nil {
		t.Fatal(err)
	}
	fourth := f.proposal(t, deep, "fourth-depth")
	fourth.Deadline = c.Contract.Deadline
	fourth.Budget = c.Contract.Budget
	if _, err = f.manager.Admit(f.owner, fourth); err == nil {
		t.Fatal("unbounded delegation depth")
	}
	stale := f.proposal(t, f.root, "stale-parent")
	stale.ParentRevision++
	if _, err = f.manager.Admit(f.owner, stale); !errors.Is(err, task.ErrConflict) {
		t.Fatal("stale proposal accepted", err)
	}
	controls := task.NewController(f.store, f.leases.Authority(), f.leases, run.NewPostgresRepository(f.pool), approval.NewPostgresRepository(f.pool), durableinput.NewStore(f.pool), transcript.NewStore(f.pool))
	start := make(chan struct{})
	errorsOut := make(chan error, 2)
	request := f.proposal(t, f.root, "race-child")
	go func() { <-start; _, err := f.manager.Admit(f.owner, request); errorsOut <- err }()
	go func() {
		<-start
		_, err := controls.Apply(f.owner, task.ControlRequest{TaskID: f.root.TaskID, ExpectedRevision: f.root.Revision, Kind: "stop", Instruction: "stop bounded tree"})
		errorsOut <- err
	}()
	close(start)
	var stopSucceeded bool
	for range 2 {
		if err := <-errorsOut; err == nil {
			stopSucceeded = true
		}
	}
	if !stopSucceeded {
		t.Fatal("stop/admit neither progressed")
	}
	var live int
	if err = f.pool.QueryRow(f.ctx, `SELECT count(*) FROM tasks WHERE status<>'cancelled'`).Scan(&live); err != nil || live != 0 {
		t.Fatal("stop missed cross-session child", live, err)
	}
	if _, err = f.manager.Advance(f.ctx, f.root.TaskID); err == nil {
		t.Fatal("cancelled parent resumed descendants")
	}
}
