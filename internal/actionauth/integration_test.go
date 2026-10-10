package actionauth

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

func actionDatabase(t *testing.T) *pgxpool.Pool {
	t.Helper()
	dsn := os.Getenv("BUTLER_TEST_ACTIONAUTH_POSTGRES_URL")
	if dsn == "" {
		t.Skip("isolated action authorization PostgreSQL integration requires BUTLER_TEST_ACTIONAUTH_POSTGRES_URL")
	}
	ctx := context.Background()
	cfg, err := pgxpool.ParseConfig(dsn)
	if err != nil {
		t.Fatal("invalid test database configuration")
	}
	if !strings.HasSuffix(cfg.ConnConfig.Database, "_test") {
		t.Fatal("action authorization tests require a separate *_test database")
	}
	admin, err := pgxpool.NewWithConfig(ctx, cfg)
	if err != nil {
		t.Fatal(err)
	}
	schema := "actionauth_" + randomID()
	if _, err := admin.Exec(ctx, "CREATE SCHEMA "+schema); err != nil {
		t.Fatal(err)
	}
	cfg.ConnConfig.RuntimeParams["search_path"] = schema
	pool, err := pgxpool.NewWithConfig(ctx, cfg)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { pool.Close(); _, _ = admin.Exec(ctx, "DROP SCHEMA "+schema+" CASCADE"); admin.Close() })
	data, err := os.ReadFile(filepath.Join("..", "..", "migrations", "031_action_authorization.up.sql"))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, string(data)); err != nil {
		t.Fatal(err)
	}
	data, err = os.ReadFile(filepath.Join("..", "..", "migrations", "035_external_effect_journal.up.sql"))
	if err != nil {
		t.Fatal(err)
	}
	if _, err = pool.Exec(ctx, string(data)); err != nil {
		t.Fatal(err)
	}
	// Narrow dispatch-authority fixtures use the production column names. The
	// journal migration and every fault/CAS operation execute against real PG.
	if _, err = pool.Exec(ctx, `CREATE TABLE session_ownership(session_key TEXT PRIMARY KEY,run_id TEXT NOT NULL,lease_id TEXT NOT NULL,fence_epoch BIGINT NOT NULL,expires_at TIMESTAMPTZ NOT NULL);
 CREATE TABLE runs(run_id TEXT PRIMARY KEY,task_id TEXT,task_revision BIGINT);
 CREATE TABLE tasks(task_id TEXT PRIMARY KEY,owner_id TEXT NOT NULL,status TEXT NOT NULL,revision BIGINT NOT NULL,constraints_revision BIGINT NOT NULL,execution_policy TEXT NOT NULL DEFAULT 'normal');
 INSERT INTO tasks(task_id,owner_id,status,revision,constraints_revision) VALUES('task','owner','active',7,2);
 INSERT INTO runs VALUES('run','task',7);
 INSERT INTO session_ownership VALUES('web:owner','run','lease',4,clock_timestamp()+INTERVAL '1 hour');`); err != nil {
		t.Fatal(err)
	}
	return pool
}
func TestPostgresDispatchCASAndUnknownOutcome(t *testing.T) {
	pool := actionDatabase(t)
	store := NewPostgresStore(pool)
	ctx := context.Background()
	s, _, reader, _, call, contract := fixture(t)
	s.store = store
	g := prepareApproved(t, s, reader, call, contract)
	var wins atomic.Int32
	var wg sync.WaitGroup
	for i := 0; i < 24; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if store.ClaimGrant(ctx, g.ID) == nil {
				wins.Add(1)
			}
		}()
	}
	wg.Wait()
	if wins.Load() != 1 {
		t.Fatalf("dispatch claims won=%d", wins.Load())
	}
	if err := store.FinishGrant(ctx, g.ID, "unknown"); err != nil {
		t.Fatal(err)
	}
	if _, err := s.Authorize(ctx, call, contract, g.Binding.ActionID, "approval", ""); err == nil {
		t.Fatal("unknown external outcome replay authorized")
	}
	stored, err := store.GetGrant(ctx, g.ID)
	if err != nil || stored.State != "unknown" {
		t.Fatal("unknown outcome lost")
	}
}
func TestPostgresRefreshFenceAndRevokeMandate(t *testing.T) {
	pool := actionDatabase(t)
	store := NewPostgresStore(pool)
	ctx := context.Background()
	s, _, reader, _, call, contract := fixture(t)
	s.store = store
	reader.current.MandateID = "mandate"
	m := Mandate{ID: "mandate", OwnerID: "owner", TaskID: "task", TaskRevision: 7, ConstraintsRevision: 2, Revision: 1, Tools: []string{"http.request"}, Resources: []string{"https://example.com"}, Effects: []Effect{EffectCommit}, ExpiresAt: time.Now().Add(time.Hour)}
	if err := store.PutMandate(ctx, m); err != nil {
		t.Fatal(err)
	}
	p, err := s.Prepare(ctx, call, contract)
	if err != nil {
		t.Fatal(err)
	}
	g, err := s.Authorize(ctx, call, contract, p.Binding.ActionID, "", m.ID)
	if err != nil {
		t.Fatal(err)
	}
	reader.current.LeaseID = "replacement"
	reader.current.LeaseEpoch++
	if _, err := s.Verify(ctx, call, contract, g.ID); err == nil {
		t.Fatal("old worker fence accepted")
	}
	fresh, err := s.Authorize(ctx, call, contract, p.Binding.ActionID, "", m.ID)
	if err != nil {
		t.Fatal(err)
	}
	if fresh.ID == g.ID {
		t.Fatal("dispatch grant was not refreshed")
	}
	if _, err := store.GetGrant(ctx, g.ID); err == nil {
		t.Fatal("old refreshed grant still reachable")
	}
	if err := store.RevokeMandate(ctx, m.ID, m.OwnerID, m.Revision); err != nil {
		t.Fatal(err)
	}
	if err := store.ClaimGrant(ctx, fresh.ID); err == nil {
		t.Fatal("revoked mandate dispatched in CAS")
	}
	if _, err := s.Verify(ctx, call, contract, fresh.ID); err == nil {
		t.Fatal("revoked mandate verified")
	}
}
func TestPostgresIntentIdentityCannotChange(t *testing.T) {
	store := NewPostgresStore(actionDatabase(t))
	ctx := context.Background()
	s, _, _, _, call, contract := fixture(t)
	s.store = store
	if _, err := s.Prepare(ctx, call, contract); err != nil {
		t.Fatal(err)
	}
	call.ArgsJson = `{"url":"https://example.com/different","method":"POST"}`
	if _, err := s.Prepare(ctx, call, contract); err == nil {
		t.Fatal("same tool call identity replaced prepared intent")
	}
}
