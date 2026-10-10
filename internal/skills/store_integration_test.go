package skills

import (
	"context"
	"fmt"
	"github.com/butler/butler/internal/actionauth"
	toolbrokerv1 "github.com/butler/butler/internal/gen/toolbroker/v1"
	"github.com/butler/butler/internal/owneridentity"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"os"
	"path/filepath"
	"strings"
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
	dsn := os.Getenv("BUTLER_TEST_POSTGRES_URL")
	if dsn == "" {
		t.Skip("requires isolated PostgreSQL")
	}
	cfg, err := pgxpool.ParseConfig(dsn)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasSuffix(cfg.ConnConfig.Database, "_test") {
		t.Fatal("requires isolated *_test DB")
	}
	ctx := context.Background()
	admin, err := pgxpool.NewWithConfig(ctx, cfg)
	if err != nil {
		t.Fatal(err)
	}
	schema := fmt.Sprintf("skills_%d", time.Now().UnixNano())
	if _, err = admin.Exec(ctx, "CREATE SCHEMA "+schema); err != nil {
		t.Fatal(err)
	}
	cfg.MaxConns = 2
	cfg.ConnConfig.RuntimeParams["search_path"] = schema
	pool, err := pgxpool.NewWithConfig(ctx, cfg)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		pool.Close()
		_, _ = admin.Exec(context.Background(), "DROP SCHEMA "+schema+" CASCADE")
		admin.Close()
	})
	if _, err = pool.Exec(ctx, `CREATE TABLE tasks(task_id text PRIMARY KEY,owner_id text NOT NULL,status text NOT NULL,revision bigint NOT NULL,constraints_revision bigint NOT NULL,execution_policy text NOT NULL DEFAULT 'normal');INSERT INTO tasks VALUES('task','owner','active',1,1,'normal');CREATE TABLE runs(run_id text PRIMARY KEY,task_id text,task_revision bigint);INSERT INTO runs VALUES('run','task',1);CREATE TABLE session_ownership(session_key text PRIMARY KEY,run_id text NOT NULL,lease_id text NOT NULL,fence_epoch bigint NOT NULL,expires_at timestamptz NOT NULL);INSERT INTO session_ownership VALUES('web:owner','run','lease',1,clock_timestamp()+interval '1 hour')`); err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"031_action_authorization.up.sql", "035_external_effect_journal.up.sql", "037_execution_budgets.up.sql", "038_owner_goals_mandates.up.sql", "038_owner_purpose_policies.up.sql", "050_reviewed_extensions.up.sql", "051_reviewed_skills.up.sql"} {
		raw, e := os.ReadFile(filepath.Join("..", "..", "migrations", name))
		if e != nil {
			t.Fatal(e)
		}
		if _, e = pool.Exec(ctx, string(raw)); e != nil {
			t.Fatal(e)
		}
	}
	return pool
}
func skillFixture(t *testing.T) (*Store, Manifest, *fixtureAuthority, *skillEnvironment, context.Context) {
	t.Helper()
	m, a := fixtureManifest(t)
	env := &skillEnvironment{context: actionauth.ContextSnapshot{RunID: "run", RunState: "model_running", SessionKey: "web:owner", OwnerID: "owner", TaskID: "task", TaskStatus: "active", TaskRevision: 1, PinnedTaskRevision: 1, ConstraintsRevision: 1, LeaseID: "lease", LeaseEpoch: 1, LeaseExpiresAt: time.Now().Add(time.Hour)}, contract: a}
	s := NewStore(skillDB(t), "owner", a, a, env)
	ctx := owneridentity.WithActor(context.Background(), owneridentity.Actor{OwnerID: "owner", Channel: "web", DeliveryID: "owner-delivery"})
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
	if _, err := s.pool.Exec(ctx, `UPDATE skill_versions SET manifest_text='{}'`); err == nil {
		t.Fatal("immutable procedure changed")
	}
}
