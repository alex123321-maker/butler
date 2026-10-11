package skills

import (
	"context"
	"github.com/butler/butler/internal/actionauth"
	toolbrokerv1 "github.com/butler/butler/internal/gen/toolbroker/v1"
	"github.com/butler/butler/internal/memory/catalog"
	"github.com/butler/butler/internal/memory/memorytest"
	"github.com/butler/butler/internal/owneridentity"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"google.golang.org/protobuf/proto"
	"path/filepath"
	"sync"
	"testing"
	"time"
)

type skillEnvironment struct {
	context  actionauth.ContextSnapshot
	contract *fixtureAuthority
}

func (e *skillEnvironment) GetActionContext(context.Context, string) (actionauth.ContextSnapshot, error) {
	return e.context, nil
}
func (e *skillEnvironment) ContractsForRun(context.Context, string) ([]*toolbrokerv1.ToolContract, error) {
	return []*toolbrokerv1.ToolContract{e.contract.contract}, nil
}
func (f *fixtureAuthority) GetSkillContractTx(ctx context.Context, _ pgx.Tx, name string) (*toolbrokerv1.ToolContract, error) {
	return f.GetSkillContract(ctx, name)
}
func skillDB(t *testing.T) *pgxpool.Pool {
	t.Helper()
	pool := memorytest.Database(t)
	ctx := context.Background()
	if _, err := pool.Exec(ctx, `INSERT INTO sessions(session_key,user_id,channel) VALUES('web:owner','owner','web');INSERT INTO tasks(task_id,owner_id,session_key,goal) VALUES('task','owner','web:owner','Use checked procedure');INSERT INTO runs(run_id,session_key,input_event_id,status,autonomy_mode,current_state,model_provider,started_at,updated_at,task_id,task_revision) VALUES('run','web:owner','input','running','mode_1','model_running','fixture',clock_timestamp(),clock_timestamp(),'task',1);INSERT INTO session_ownership(session_key,run_id,owner_id,lease_id,fence_epoch,expires_at) VALUES('web:owner','run','owner','lease',1,clock_timestamp()+interval '1 hour')`); err != nil {
		t.Fatal(err)
	}
	return pool
}
func skillFixture(t *testing.T) (*Store, Manifest, *fixtureAuthority, *skillEnvironment, context.Context) {
	t.Helper()
	m, a := fixtureManifest(t)
	env := &skillEnvironment{context: actionauth.ContextSnapshot{RunID: "run", RunState: "model_running", SessionKey: "web:owner", OwnerID: "owner", TaskID: "task", TaskStatus: "active", TaskRevision: 1, PinnedTaskRevision: 1, ConstraintsRevision: 1, LeaseID: "lease", LeaseEpoch: 1, LeaseExpiresAt: time.Now().Add(time.Hour)}, contract: a}
	s := NewStore(skillDB(t), "owner", a, a, env)
	ctx := owneridentity.WithActor(context.Background(), owneridentity.Actor{OwnerID: "owner", Channel: "web", DeliveryID: "owner-delivery"})
	manifest := catalog.NewFileManifest(filepath.Join(t.TempDir(), "deletions"))
	if err := manifest.Initialize(); err != nil {
		t.Fatal(err)
	}
	s.SetSources(catalog.New(s.pool, manifest))
	parent, err := s.sources.Put(ctx, catalog.PutSource{Kind: "document", ScopeType: "user", ScopeID: "owner", ExternalKey: "checked-result", Body: "Source proof fixture"})
	if err != nil {
		t.Fatal(err)
	}
	a.parents = []catalog.SourceRef{{ID: parent.ID, Revision: parent.Revision, AccessRevision: parent.AccessRevision, Hash: parent.Hash}}
	return s, m, a, env, ctx
}
func activeFixture(t *testing.T, s *Store, m Manifest, ctx context.Context) Record {
	t.Helper()
	p := packageOf(t, m)
	r, err := s.Stage(ctx, p)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = s.Test(ctx, p.Digest); err != nil {
		t.Fatal(err)
	}
	r, err = s.Activate(ctx, p.Digest, r.Generation)
	if err != nil {
		t.Fatal(err)
	}
	return r
}
func TestSkillLifecycleIntegration(t *testing.T) {
	s, m, a, _, ctx := skillFixture(t)
	r := activeFixture(t, s, m, ctx)
	if _, err := s.Stage(context.Background(), r.Package); err != ErrAccess {
		t.Fatal("unauthenticated stage accepted")
	}
	index, err := s.Index(ctx, "run", "check", 12)
	if err != nil || len(index) != 1 {
		t.Fatalf("index: %v %+v", err, index)
	}
	if _, err = s.Load(ctx, "run", r.Digest, 8192); err != nil {
		t.Fatal(err)
	}
	// A bad new candidate never overwrites the working version.
	n := m
	n.Version = "1.0.1"
	n.Description = "Candidate wording"
	candidate, err := s.Stage(ctx, packageOf(t, n))
	if err != nil {
		t.Fatal(err)
	}
	a.invalid = true
	if _, err = s.Test(ctx, candidate.Digest); err != ErrEvidence {
		t.Fatalf("bad proof: %v", err)
	}
	if _, err = s.Activate(ctx, candidate.Digest, r.Generation); err != ErrEvidence {
		t.Fatalf("bad activation: %v", err)
	}
	current, _ := s.Get(ctx, r.Digest)
	if !current.Active {
		t.Fatal("bad candidate replaced working skill")
	}
	a.invalid = false
	if _, err = s.Test(ctx, candidate.Digest); err != nil {
		t.Fatal(err)
	}
	updated, err := s.Activate(ctx, candidate.Digest, r.Generation)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = s.Pinned(ctx, "run"); err != nil {
		t.Fatalf("prior pinned procedure lost on update: %v", err)
	}
	if _, err = s.Activate(ctx, r.Digest, updated.Generation); err != nil {
		t.Fatalf("rollback: %v", err)
	}
	old, _ := s.Get(ctx, r.Digest)
	revoked, err := s.Disable(ctx, r.Digest, old.Generation, true)
	if err != nil || revoked.Status != "revoked" {
		t.Fatal(err)
	}
	if _, err = s.Pinned(ctx, "run"); err != ErrUnavailable {
		t.Fatalf("revoked procedure recovered: %v", err)
	}
	tx, _ := s.pool.Begin(ctx)
	defer tx.Rollback(ctx)
	if err = CheckPinnedLifecycleTx(ctx, tx, "run"); err != ErrUnavailable {
		t.Fatalf("final result guard allowed revoked procedure: %v", err)
	}
	_ = tx.Rollback(ctx)
	if _, err = s.Activate(ctx, r.Digest, revoked.Generation); err == nil {
		t.Fatal("permanent revoke rolled back")
	}
}
func TestSkillPinsConcurrentIntegration(t *testing.T) {
	s, m, a, _, ctx := skillFixture(t)
	r := activeFixture(t, s, m, ctx)
	var wg sync.WaitGroup
	errs := make(chan error, 16)
	for range 16 {
		wg.Add(1)
		go func() { defer wg.Done(); _, err := s.Load(ctx, "run", r.Digest, 8192); errs <- err }()
	}
	wg.Wait()
	close(errs)
	for err := range errs {
		if err != nil {
			t.Fatal(err)
		}
	}
	var count int
	if err := s.pool.QueryRow(ctx, `SELECT count(*) FROM skill_usages WHERE run_id='run'`).Scan(&count); err != nil || count != 1 {
		t.Fatalf("duplicate usage: %d %v", count, err)
	}
	a.contract.ContractRevision = "changed"
	if _, err := s.Pinned(ctx, "run"); err != ErrConflict {
		t.Fatalf("changed interface replayed: %v", err)
	}
	tx, _ := s.pool.Begin(ctx)
	defer tx.Rollback(ctx)
	if err := s.CheckRunTx(ctx, tx, "run", "owner"); err != ErrConflict {
		t.Fatalf("changed interface dispatched: %v", err)
	}
}

type changedTransactionContract struct{ *fixtureAuthority }

func (f changedTransactionContract) GetSkillContractTx(context.Context, pgx.Tx, string) (*toolbrokerv1.ToolContract, error) {
	c := proto.Clone(f.contract).(*toolbrokerv1.ToolContract)
	c.ContractRevision = "current-owned-revision-changed"
	return c, nil
}

func TestActivationRechecksOwnedCurrentContractIntegration(t *testing.T) {
	s, m, authority, _, ctx := skillFixture(t)
	original := activeFixture(t, s, m, ctx)
	m.Version = "1.0.1"
	m.Description = "Accepted wording with original dependency"
	candidate, err := s.Stage(ctx, packageOf(t, m))
	if err != nil {
		t.Fatal(err)
	}
	if _, err = s.Test(ctx, candidate.Digest); err != nil {
		t.Fatal(err)
	}
	// The read API still reports the previously validated contract; the owned
	// transaction observes the current interface at irreversible activation.
	s.contracts = changedTransactionContract{authority}
	if _, err = s.Activate(ctx, candidate.Digest, original.Generation); err != ErrConflict {
		t.Fatalf("stale preflight activated incompatible version: %v", err)
	}
	current, err := s.Get(ctx, original.Digest)
	if err != nil || !current.Active || current.Generation != original.Generation {
		t.Fatal("failed activation changed working procedure", err)
	}
}
func TestSkillLeaseAndTaskRevocationIntegration(t *testing.T) {
	s, m, _, env, ctx := skillFixture(t)
	r := activeFixture(t, s, m, ctx)
	if _, err := s.Load(ctx, "run", r.Digest, 8192); err != nil {
		t.Fatal(err)
	}
	if _, err := s.pool.Exec(ctx, `UPDATE session_ownership SET fence_epoch=2`); err != nil {
		t.Fatal(err)
	}
	if _, err := s.Load(ctx, "run", r.Digest, 8192); err != ErrAccess {
		t.Fatalf("old fence accepted: %v", err)
	}
	env.context.LeaseEpoch = 2
	if _, err := s.pool.Exec(ctx, `UPDATE tasks SET constraints_revision=2`); err != nil {
		t.Fatal(err)
	}
	if _, err := s.Load(ctx, "run", r.Digest, 8192); err != ErrAccess {
		t.Fatalf("stale constraints accepted: %v", err)
	}
	if _, err := s.pool.Exec(ctx, `UPDATE skill_versions SET dependencies_json='[]'`); err == nil {
		t.Fatal("immutable procedure changed")
	}
}
