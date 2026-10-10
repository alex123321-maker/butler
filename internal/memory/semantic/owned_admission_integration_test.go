package semantic

import (
	"context"
	"encoding/json"
	"errors"
	"path/filepath"
	"strconv"
	"testing"
	"time"

	"github.com/butler/butler/internal/actionauth"
	"github.com/butler/butler/internal/budget"
	pb "github.com/butler/butler/internal/gen/localmodels/v1"
	"github.com/butler/butler/internal/memory/catalog"
	"github.com/butler/butler/internal/memory/jobs"
	"github.com/butler/butler/internal/memory/memorytest"
	"github.com/butler/butler/internal/owneridentity"
	"github.com/butler/butler/internal/ownership"
	"github.com/butler/butler/internal/security/content"
	"github.com/jackc/pgx/v5/pgxpool"
)

type ownedFixture struct {
	pool        *pgxpool.Pool
	catalog     *catalog.Store
	budgets     *budget.Controller
	guard       *CatalogGuard
	target      budget.LocalTarget
	owner, live context.Context
	source      catalog.Source
	chunk       int64
}

func ownedMemory(t *testing.T, turns int64) ownedFixture {
	t.Helper()
	f := ownedFixture{pool: memorytest.Database(t)}
	f.owner = owneridentity.WithActor(context.Background(), owneridentity.Actor{OwnerID: "owner", Channel: "web", DeliveryID: "owned-fixture"})
	manifest := catalog.NewFileManifest(filepath.Join(t.TempDir(), "deletions"))
	if e := manifest.Initialize(); e != nil {
		t.Fatal(e)
	}
	f.catalog = catalog.New(f.pool, manifest)
	var e error
	f.source, e = f.catalog.Put(f.owner, catalog.PutSource{ExternalKey: "document", Kind: "document", ScopeType: "user", ScopeID: "owner", Title: "Исходник", Body: "Проверяемые данные"})
	if e != nil {
		t.Fatal(e)
	}
	_, e = f.catalog.PublishFragments(f.owner, catalog.SourceRef{ID: f.source.ID}, []catalog.Fragment{{Key: "part", Title: "Исходник", Text: "Проверяемые данные", Locator: json.RawMessage(`{"path":"fixture.txt"}`)}})
	if e != nil {
		t.Fatal(e)
	}
	if e = f.pool.QueryRow(f.owner, `SELECT object_id FROM memory_object_sources WHERE source_id=$1 AND memory_type='chunk'`, f.source.ID).Scan(&f.chunk); e != nil {
		t.Fatal(e)
	}
	_, e = f.pool.Exec(f.owner, `INSERT INTO sessions(session_key,user_id,channel) VALUES('session','owner','web');
INSERT INTO tasks(task_id,session_key,owner_id,goal) VALUES('root','session','owner','private search');
INSERT INTO tasks(task_id,parent_task_id,session_key,owner_id,goal) VALUES('child','root','session','owner','private search child');
INSERT INTO runs(run_id,session_key,input_event_id,status,autonomy_mode,current_state,model_provider,started_at,updated_at,task_id,task_revision) VALUES('run','session','input','running','mode_1','model_running','local',clock_timestamp(),clock_timestamp(),'root',1);
INSERT INTO session_ownership(session_key,run_id,lease_id,fence_epoch,expires_at) VALUES('session','run','lease',1,clock_timestamp()+INTERVAL '1 hour');`)
	if e != nil {
		t.Fatal(e)
	}
	p := budget.DefaultPolicy()
	p.Task.ModelTurns = turns
	p.Models["local:google/embeddinggemma-2"] = budget.ModelPrice{Basis: "local_no_billing"}
	p.Models["local:Qwen/Qwen3-Reranker-0.6B"] = budget.ModelPrice{Basis: "local_no_billing"}
	f.budgets = budget.NewController(f.pool, p)
	f.budgets.SetAuthorityResolver(actionauth.ResolveBudgetAuthority)
	f.guard, e = NewCatalogGuard(f.catalog, "private-retrieval")
	if e != nil {
		t.Fatal(e)
	}
	f.target, e = budget.NewLocalTarget("local-models:9090", "operator-pinned-service")
	if e != nil {
		t.Fatal(e)
	}
	f.live = ownership.WithToken(context.Background(), nil, ownership.Token{SessionKey: "session", RunID: "run", LeaseID: "lease", FenceEpoch: 1})
	return f
}
func liveAdapter(t *testing.T, f ownedFixture) (*Adapter, *fakeModel) {
	t.Helper()
	gate, e := NewLiveAdmission(f.budgets, f.guard, f.target)
	if e != nil {
		t.Fatal(e)
	}
	m := &fakeModel{space: testSpace(768)}
	return &Adapter{Backend: m, Space: m.space, Admission: gate}, m
}
func query(a *Adapter, ctx context.Context) error {
	_, _, e := a.Embeddings(ctx, Scope{"user", "owner"}, []*pb.TextInput{{Id: "query", Text: "Что содержится в исходнике?"}}, pb.EmbeddingTask_EMBEDDING_TASK_QUERY)
	return e
}

func assignLocalMandate(t *testing.T, f ownedFixture) {
	t.Helper()
	now := time.Now().UTC().Truncate(time.Microsecond)
	m := actionauth.Mandate{ID: "local-mandate", OwnerID: "owner", ScopeKind: "owner_goal", GoalID: "local-goal", GoalRevision: 1, Revision: 1, Status: "active", Purpose: "private-retrieval", Tools: []string{"web.fetch"}, ResourceScopes: []actionauth.ResourceScope{{Origin: "https://example.invalid", PathPrefix: "/fixture"}}, Effects: []actionauth.Effect{actionauth.EffectRead}, ExpiresAt: now.Add(time.Hour), BudgetDeadline: now.Add(time.Hour), Budget: actionauth.MandateBudget{ModelTurns: 4, Tokens: 30000, ActiveMillis: 300000, ProviderInputBytes: 30000}, Privacy: "local_only", Intervention: []string{"budget_exhausted"}, Reporting: "exceptions"}
	if e := actionauth.ValidateOwnerMandate(m, now); e != nil {
		t.Fatal(e)
	}
	goal := actionauth.GoalPolicy{ID: m.GoalID, OwnerID: m.OwnerID, Kind: "objective", Status: "active", Revision: 1, Purpose: m.Purpose, ExpiresAt: m.ExpiresAt}
	raw, _ := json.Marshal(goal)
	if _, e := f.pool.Exec(f.owner, `INSERT INTO owner_goals(goal_id,owner_id,kind,status,revision,expires_at,goal_data,source_data) VALUES($1,$2,'objective','active',1,$3,$4,'{}');`, goal.ID, goal.OwnerID, goal.ExpiresAt, raw); e != nil {
		t.Fatal(e)
	}
	if _, e := f.pool.Exec(f.owner, `INSERT INTO owner_purpose_policies(owner_id,purpose,revision) VALUES('owner','private-retrieval',0)`); e != nil {
		t.Fatal(e)
	}
	raw, _ = json.Marshal(m)
	if _, e := f.pool.Exec(f.owner, `INSERT INTO action_mandates(mandate_id,owner_id,task_id,revision,mandate_data,expires_at,scope_kind,goal_id,goal_revision,status) VALUES($1,'owner','',1,$2,$3,'owner_goal',$4,1,'active')`, m.ID, raw, m.ExpiresAt, m.GoalID); e != nil {
		t.Fatal(e)
	}
	if _, e := f.pool.Exec(f.owner, `INSERT INTO task_authority_bindings(task_id,owner_id,goal_id,goal_revision,mandate_id,mandate_revision,purpose,initiative_reason,observation_ref) VALUES('root','owner','local-goal',1,'local-mandate',1,'private-retrieval','owner-request','fixture')`); e != nil {
		t.Fatal(e)
	}
}

func TestCurrentLocalOnlyAuthorityAndImmediateRevocationIntegration(t *testing.T) {
	f := ownedMemory(t, 8)
	assignLocalMandate(t, f)
	a, m := liveAdapter(t, f)
	if e := query(a, f.live); e != nil {
		t.Fatal("local_only denied internal service", e)
	}
	if _, e := f.budgets.AdmitModel(f.live, "run", "", "local", "google/embeddinggemma-2", 32); e == nil {
		t.Fatal("local pricing relabeled cloud privacy")
	}
	if _, e := f.pool.Exec(f.owner, `UPDATE owner_goals SET status='paused' WHERE goal_id='local-goal'`); e != nil {
		t.Fatal(e)
	}
	if e := query(a, f.live); e == nil || m.calls != 1 {
		t.Fatal("paused goal admitted before sweep", e, m.calls)
	}
	scope, e := f.budgets.Ledger.Get(f.owner, budget.ScopeID("mandate", "owner", "local-mandate"))
	if e != nil || scope.Used.ModelTurns != 1 {
		t.Fatal("current mandate allowance", scope, e)
	}
}

func TestOwnedLiveAdmissionSharesRootAfterRestartChildIntegration(t *testing.T) {
	f := ownedMemory(t, 2)
	a, m := liveAdapter(t, f)
	if e := query(a, context.Background()); e == nil || m.calls != 0 {
		t.Fatal("untrusted caller invoked model", e)
	}
	if e := query(a, f.live); e != nil {
		t.Fatal(e)
	}
	_, e := f.pool.Exec(f.owner, `INSERT INTO runs(run_id,session_key,input_event_id,status,autonomy_mode,current_state,model_provider,started_at,updated_at,task_id,task_revision) VALUES('rerun','session','input2','running','mode_1','model_running','local',clock_timestamp(),clock_timestamp(),'child',1);UPDATE session_ownership SET run_id='rerun',fence_epoch=2 WHERE session_key='session'`)
	if e != nil {
		t.Fatal(e)
	}
	ctx := ownership.WithToken(context.Background(), nil, ownership.Token{SessionKey: "session", RunID: "rerun", LeaseID: "lease", FenceEpoch: 2})
	restarted := budget.NewController(f.pool, f.budgets.Policy)
	restarted.SetAuthorityResolver(actionauth.ResolveBudgetAuthority)
	f.budgets = restarted
	a, m = liveAdapter(t, f)
	if e = query(a, ctx); e != nil {
		t.Fatal(e)
	}
	e = query(a, ctx)
	var exhausted *budget.ExhaustedError
	if !errors.As(e, &exhausted) || exhausted.Kind != "task" || m.calls != 1 {
		t.Fatal("child/restart reset allowance", e, m.calls)
	}
	var reservations int
	if e = f.pool.QueryRow(ctx, `SELECT count(*) FROM execution_budget_reservations WHERE operation_id LIKE 'local:%'`).Scan(&reservations); e != nil || reservations != 2 {
		t.Fatal(reservations, e)
	}
	root, e := restarted.Ledger.Get(ctx, budget.ScopeID("task", "owner", "root"))
	if e != nil || root.Used.ModelTurns != 2 || root.Used.Tokens != 8192 {
		t.Fatal(root, e)
	}
}

func TestOwnedLiveDeniedStateCostSourceAndCancellationIntegration(t *testing.T) {
	f := ownedMemory(t, 8)
	a, m := liveAdapter(t, f)
	if _, e := budget.NewLocalTarget("https://local-no-billing.example", "identity"); e == nil {
		t.Fatal("price became locality")
	}
	saved := f.budgets.Policy.Models["local:google/embeddinggemma-2"]
	delete(f.budgets.Policy.Models, "local:google/embeddinggemma-2")
	if e := query(a, f.live); !errors.Is(e, budget.ErrUnknownCost) || m.calls != 0 {
		t.Fatal("missing operator pricing", e)
	}
	f.budgets.Policy.Models["local:google/embeddinggemma-2"] = saved
	for _, state := range []string{"finalizing", "completed", "cancelled", "failed"} {
		_, e := f.pool.Exec(f.owner, `UPDATE runs SET current_state=$1 WHERE run_id='run'`, state)
		if e != nil {
			t.Fatal(e)
		}
		if e = query(a, f.live); e == nil {
			t.Fatal("dead run released", state)
		}
	}
	if _, e := f.pool.Exec(f.owner, `UPDATE runs SET current_state='model_running' WHERE run_id='run'`); e != nil {
		t.Fatal(e)
	}
	snapshot, e := scanSource(f.pool.QueryRow(f.owner, `SELECT `+sourceColumns+` FROM memory_chunks WHERE id=$1`, f.chunk))
	if e != nil {
		t.Fatal(e)
	}
	op := Operation{ID: "actual", Kind: "rerank", Scope: Scope{"user", "owner"}, Items: 1, InputBytes: 32, Timeout: time.Second, Origin: content.Derive(content.MemoryDerived), ChunkIDs: []int64{f.chunk}, Sources: []SourceStamp{stamp(snapshot)}}
	gate := a.Admission.(*LiveAdmission)
	callCtx, release, e := gate.Admit(f.live, op)
	if e != nil {
		t.Fatal(e)
	}
	defer release()
	if _, e = f.catalog.Delete(f.owner, f.source.ID, "revoke", f.source.Revision); e != nil {
		t.Fatal(e)
	}
	select {
	case <-callCtx.Done():
	case <-time.After(2 * time.Second):
		t.Fatal("revoked source did not cancel inference")
	}
	if _, _, e = gate.Admit(f.live, op); e == nil {
		t.Fatal("deleted source released")
	}
	store := NewStore(f.pool)
	store.Guard = f.guard
	if _, e = store.LiveSource(f.live, Scope{"user", "owner"}, f.chunk); e == nil {
		t.Fatal("deleted chunk returned")
	}
}

func TestOwnedJobRebuildPinsSourcesChargesEveryAttemptIntegration(t *testing.T) {
	f := ownedMemory(t, 8)
	s := jobs.New(f.pool, f.catalog, f.budgets, &jobs.LocalTarget{Endpoint: f.target.Endpoint(), Identity: f.target.Identity()})
	r := jobs.Request{ID: "owned-semantic-index", Kind: "index", Purpose: "private-retrieval", TaskID: "root", OriginalRunID: "run", Sources: []catalog.SourceRef{{ID: f.source.ID, Revision: f.source.Revision, AccessRevision: f.source.AccessRevision, Hash: f.source.Hash}}, Allowance: budget.Units{ModelTurns: 3, Tokens: 20000, ActiveMillis: 180000, ProviderInputBytes: 20000}, Deadline: time.Now().Add(time.Hour)}
	if _, e := s.Enqueue(f.owner, r); e != nil {
		t.Fatal(e)
	}
	j, ok, e := s.Claim(context.Background())
	if e != nil || !ok {
		t.Fatal(e)
	}
	gate, e := NewJobAdmission(s, j, f.guard, f.target, budget.ModelPrice{Basis: "local_no_billing"})
	if e != nil {
		t.Fatal(e)
	}
	ctx := gate.Context(context.Background())
	store := NewStore(f.pool)
	store.Guard = f.guard
	g, e := store.Create(ctx, testSpace(768), Scope{"user", "owner"})
	if e != nil {
		t.Fatal(e)
	}
	m := &fakeModel{space: g.Space}
	a := &Adapter{Backend: m, Space: g.Space, Admission: gate}
	if _, e = Rebuild(ctx, store, a, g.ID, 10); e != nil {
		t.Fatal(e)
	}
	if m.calls != 1 {
		t.Fatal("missing actual index", m.calls)
	}
	// The run may finish, but the independent current job/task proof remains.
	if _, e = f.pool.Exec(f.owner, `UPDATE runs SET current_state='completed' WHERE run_id='run';UPDATE session_ownership SET expires_at=clock_timestamp()-INTERVAL '1 second'`); e != nil {
		t.Fatal(e)
	}
	texts := []*pb.TextInput{{Id: strconv.FormatInt(f.chunk, 10), Text: "Проверяемые данные", Title: "Исходник"}}
	snapshot, e := scanSource(f.pool.QueryRow(f.owner, `SELECT `+sourceColumns+` FROM memory_chunks WHERE id=$1`, f.chunk))
	if e != nil {
		t.Fatal(e)
	}
	ctx = withSourceStamps(ctx, []Source{snapshot})
	if _, _, e = a.Embeddings(ctx, Scope{"user", "owner"}, texts, pb.EmbeddingTask_EMBEDDING_TASK_DOCUMENT); e != nil {
		t.Fatal("job borrowed old lease", e)
	}
	var count int
	if e = f.pool.QueryRow(ctx, `SELECT count(*) FROM execution_budget_reservations WHERE operation_id LIKE 'aux:%'`).Scan(&count); e != nil || count != 2 {
		t.Fatal(count, e)
	}
	// Provenance text cannot invent permission to a foreign object.
	forged := Source{ID: f.chunk + 9999, ProvenanceJSON: `{"catalog":{"source_id":"allowed"}}`}
	if e = f.guard.CheckSources(ctx, []Source{forged}); e == nil {
		t.Fatal("citation granted access")
	}
	if _, e = f.pool.Exec(f.owner, `UPDATE tasks SET revision=revision+1 WHERE task_id='root'`); e != nil {
		t.Fatal(e)
	}
	if _, _, e = a.Embeddings(ctx, Scope{"user", "owner"}, texts, pb.EmbeddingTask_EMBEDDING_TASK_DOCUMENT); e == nil || m.calls != 2 {
		t.Fatal("task amend released", e, m.calls)
	}
}

func TestOwnedSourceSnapshotRejectsCorrectionBeforeAndAfterInferenceIntegration(t *testing.T) {
	f := ownedMemory(t, 8)
	s := NewStore(f.pool)
	s.Guard = f.guard
	src, e := s.LiveSource(f.live, Scope{"user", "owner"}, f.chunk)
	if e != nil {
		t.Fatal(e)
	}
	a, m := liveAdapter(t, f)
	op := Operation{ID: "rank", Kind: "rerank", Scope: src.Scope, Items: 1, InputBytes: 32, Timeout: time.Second, Origin: src.MemoryOrigin(), ChunkIDs: []int64{src.ID}, Sources: []SourceStamp{stamp(src)}}
	if _, e = f.pool.Exec(f.owner, `UPDATE memory_chunks SET content='Исправлено',updated_at=clock_timestamp() WHERE id=$1`, src.ID); e != nil {
		t.Fatal(e)
	}
	if _, _, e = a.Admission.Admit(f.live, op); !errors.Is(e, ErrStale) {
		t.Fatal("old stamped input released", e)
	}
	// Source metadata cannot inject a secret or an authority from its refs.
	op.Sources = nil
	if _, _, e = a.Admission.Admit(f.live, op); !errors.Is(e, ErrAdmission) {
		t.Fatal("identity-free IPC", e)
	}
	// A result generated before task revocation is rejected synchronously,
	// independent of the periodic cancellation monitor's next tick.
	m.mutate = func(*pb.InferResponse) {
		if _, e := f.pool.Exec(f.owner, `UPDATE tasks SET revision=revision+1 WHERE task_id='root'`); e != nil {
			t.Error(e)
		}
	}
	if e = query(a, f.live); e == nil || m.calls != 1 {
		t.Fatal("late revoked result consumed", e, m.calls)
	}
}

func TestOwnedIndexRejectsChangedChunkOutputWithoutRefundIntegration(t *testing.T) {
	f := ownedMemory(t, 8)
	js := jobs.New(f.pool, f.catalog, f.budgets, &jobs.LocalTarget{Endpoint: f.target.Endpoint(), Identity: f.target.Identity()})
	r := jobs.Request{ID: "source-stamp-index", Kind: "index", Purpose: "private-retrieval", TaskID: "root", Sources: []catalog.SourceRef{{ID: f.source.ID, Revision: f.source.Revision, AccessRevision: f.source.AccessRevision, Hash: f.source.Hash}}, Allowance: budget.Units{ModelTurns: 2, Tokens: 10000, ActiveMillis: 120000, ProviderInputBytes: 20000}, Deadline: time.Now().Add(time.Hour)}
	if _, e := js.Enqueue(f.owner, r); e != nil {
		t.Fatal(e)
	}
	j, ok, e := js.Claim(context.Background())
	if e != nil || !ok {
		t.Fatal(e)
	}
	gate, e := NewJobAdmission(js, j, f.guard, f.target, budget.ModelPrice{Basis: "local_no_billing"})
	if e != nil {
		t.Fatal(e)
	}
	ctx := gate.Context(context.Background())
	store := NewStore(f.pool)
	store.Guard = f.guard
	g, e := store.Create(ctx, testSpace(768), Scope{"user", "owner"})
	if e != nil {
		t.Fatal(e)
	}
	m := &fakeModel{space: g.Space, mutate: func(*pb.InferResponse) {
		if _, e := f.pool.Exec(f.owner, `UPDATE memory_chunks SET content='Исправлено после вычисления',updated_at=clock_timestamp() WHERE id=$1`, f.chunk); e != nil {
			t.Error(e)
		}
	}}
	if _, e = Rebuild(ctx, store, &Adapter{Backend: m, Space: g.Space, Admission: gate}, g.ID, 10); e != ErrStale {
		t.Fatal("late old chunk output accepted", e)
	}
	var entries, reservations int
	if e = f.pool.QueryRow(f.owner, `SELECT count(*) FROM semantic_index_entries`).Scan(&entries); e != nil {
		t.Fatal(e)
	}
	if e = f.pool.QueryRow(f.owner, `SELECT count(*) FROM execution_budget_reservations`).Scan(&reservations); e != nil {
		t.Fatal(e)
	}
	if entries != 0 || reservations != 1 || m.calls != 1 {
		t.Fatal("late output published or refunded", entries, reservations, m.calls)
	}
}
