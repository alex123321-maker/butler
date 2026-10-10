package budget

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/butler/butler/internal/ownership"
	"github.com/jackc/pgx/v5/pgxpool"
)

func controllerDB(t *testing.T) *pgxpool.Pool {
	t.Helper()
	dsn := os.Getenv("BUTLER_TEST_POSTGRES_URL")
	if dsn == "" {
		t.Skip("requires isolated PG")
	}
	cfg, err := pgxpool.ParseConfig(dsn)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasSuffix(cfg.ConnConfig.Database, "_test") {
		t.Fatal("requires isolated *_test database")
	}
	ctx := context.Background()
	admin, err := pgxpool.NewWithConfig(ctx, cfg)
	if err != nil {
		t.Fatal(err)
	}
	schema := fmt.Sprintf("budget_%d", time.Now().UnixNano())
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
		admin.Exec(context.Background(), "DROP SCHEMA "+schema+" CASCADE")
		admin.Close()
	})
	for _, file := range []string{"031_action_authorization.up.sql", "035_external_effect_journal.up.sql", "037_execution_budgets.up.sql"} {
		raw, err := os.ReadFile(filepath.Join("..", "..", "migrations", file))
		if err != nil {
			t.Fatal(err)
		}
		if _, err = pool.Exec(ctx, string(raw)); err != nil {
			t.Fatal(err)
		}
	}
	_, err = pool.Exec(ctx, `CREATE TABLE sessions(session_key TEXT PRIMARY KEY,user_id TEXT NOT NULL);
CREATE TABLE tasks(task_id TEXT PRIMARY KEY,parent_task_id TEXT,owner_id TEXT NOT NULL,session_key TEXT NOT NULL,created_at TIMESTAMPTZ NOT NULL DEFAULT clock_timestamp(),revision BIGINT NOT NULL DEFAULT 1,constraints_revision BIGINT NOT NULL DEFAULT 1,status TEXT NOT NULL DEFAULT 'active');
CREATE TABLE runs(run_id TEXT PRIMARY KEY,session_key TEXT NOT NULL,task_id TEXT,task_revision BIGINT,current_state TEXT DEFAULT 'model_running');
CREATE TABLE session_ownership(session_key TEXT PRIMARY KEY,run_id TEXT NOT NULL,lease_id TEXT NOT NULL,fence_epoch BIGINT NOT NULL,expires_at TIMESTAMPTZ NOT NULL);
INSERT INTO sessions VALUES('session','owner');INSERT INTO tasks(task_id,owner_id,session_key) VALUES('root','owner','session');
INSERT INTO tasks(task_id,parent_task_id,owner_id,session_key) VALUES('child','root','owner','session');
INSERT INTO runs VALUES('run','session','root',1,'model_running');
INSERT INTO session_ownership VALUES('session','run','lease',1,clock_timestamp()+INTERVAL '1 hour');`)
	if err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(filepath.Join("..", "..", "migrations", "038_owner_goals_mandates.up.sql"))
	if err != nil {
		t.Fatal(err)
	}
	if _, err = pool.Exec(ctx, string(data)); err != nil {
		t.Fatal(err)
	}
	return pool
}
func modelCtx(run string) context.Context {
	return ownership.WithToken(context.Background(), nil, ownership.Token{SessionKey: "session", RunID: run, LeaseID: "lease", FenceEpoch: 1})
}
func testPolicy() Policy {
	p := DefaultPolicy()
	p.Models["fixture:fixture"] = ModelPrice{Basis: "local_no_billing"}
	p.OutputTokens = 8
	return p
}
func TestRuntimeAdmissionSurvivesNewRunAndChildWithoutResetIntegration(t *testing.T) {
	pool := controllerDB(t)
	ctx := context.Background()
	p := testPolicy()
	p.Task.ModelTurns = 2
	c := NewController(pool, p)
	first, err := c.AdmitModel(modelCtx("run"), "run", "", "fixture", "fixture", 20)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = c.AdmitModel(ctx, "run", "", "fixture", "fixture", 20); err == nil {
		t.Fatal("model-supplied run without trusted identity admitted")
	}
	if _, err = pool.Exec(ctx, `INSERT INTO runs VALUES('rerun','session','child',1,'model_running'); UPDATE session_ownership SET run_id='rerun' WHERE session_key='session'`); err != nil {
		t.Fatal(err)
	}
	restarted := NewController(pool, p)
	second, err := restarted.AdmitModel(modelCtx("rerun"), "rerun", "", "fixture", "fixture", 20)
	if err != nil {
		t.Fatal(err)
	}
	if second.OperationID == first.OperationID {
		t.Fatal("new inference reused an old reservation")
	}
	_, err = restarted.AdmitModel(modelCtx("rerun"), "rerun", "", "fixture", "fixture", 20)
	var exhausted *ExhaustedError
	if !errors.As(err, &exhausted) || exhausted.Kind != "task" || exhausted.Dimension != "model_turns" {
		t.Fatal("root allowance reset", err)
	}
	actualIn, actualOut := int64(4), int64(2)
	if err = c.ObserveModel(ctx, first.OperationID, &actualIn, &actualOut); err != nil {
		t.Fatal(err)
	}
	snapshot, err := c.Ledger.SnapshotOwner(ctx, "owner")
	if err != nil {
		t.Fatal(err)
	}
	if len(snapshot.Attempts) != 2 || snapshot.Attempts[0].ActualInput != nil {
		t.Fatal("unknown usage reported as zero", snapshot)
	}
	root, err := c.Ledger.Get(ctx, ScopeID("task", "owner", "root"))
	if err != nil || root.Used.ModelTurns != 2 || root.Used.Tokens != 56 {
		t.Fatal("observed usage refunded reservation", root, err)
	}
	if _, err = pool.Exec(ctx, `UPDATE tasks SET revision=2 WHERE task_id='child'`); err != nil {
		t.Fatal(err)
	}
	if _, err = c.AdmitModel(modelCtx("rerun"), "rerun", "", "fixture", "fixture", 20); !errors.Is(err, ErrConflict) {
		t.Fatal("amended task stale inference admitted", err)
	}
	down, err := os.ReadFile(filepath.Join("..", "..", "migrations", "037_execution_budgets.down.sql"))
	if err != nil {
		t.Fatal(err)
	}
	if _, err = pool.Exec(ctx, string(down)); err == nil {
		t.Fatal("rollback erased durable allowance history")
	}
	if _, err = c.Ledger.Get(ctx, root.ID); err != nil {
		t.Fatal("refused rollback still lost scope", err)
	}
}
func TestChildAndToolReservationsShareAtomicParentLimitsIntegration(t *testing.T) {
	pool := controllerDB(t)
	ctx := context.Background()
	p := testPolicy()
	p.Task.Children = 2
	c := NewController(pool, p)
	var wins atomic.Int32
	var wg sync.WaitGroup
	fail := make(chan error, 16)
	for n := range 16 {
		wg.Add(1)
		go func(n int) {
			defer wg.Done()
			tx, err := pool.Begin(ctx)
			if err != nil {
				fail <- err
				return
			}
			defer tx.Rollback(ctx)
			err = c.ReserveChildTx(ctx, tx, "child", "", fmt.Sprintf("child-%d", n))
			if err == nil {
				err = tx.Commit(ctx)
				if err == nil {
					wins.Add(1)
				}
			}
			var e *ExhaustedError
			if err != nil && !errors.As(err, &e) {
				fail <- err
			}
		}(n)
	}
	wg.Wait()
	close(fail)
	for err := range fail {
		t.Error(err)
	}
	if wins.Load() != 2 {
		t.Fatal("parallel child allowance escaped root", wins.Load())
	}
	// Replaying the permanent effect reserves once. Rolling back the same outer
	// claim transaction rolls back every budget change, without another connection.
	i := ToolIdentity{RunID: "run", EffectID: "effect", ToolName: "http.request", ArgsHash: "canonical", ContractHash: "v1", Effect: "read", TaskRevision: 1, ConstraintsRevision: 1}
	tx, _ := pool.Begin(ctx)
	if err := c.ReserveToolTx(ctx, tx, i); err != nil {
		t.Fatal(err)
	}
	tx.Rollback(ctx)
	scope, err := c.Ledger.Get(ctx, ScopeID("task", "owner", "root"))
	if err != nil || scope.Used.ExternalCalls != 0 {
		t.Fatal("rolled back claim spent budget", scope, err)
	}
	for range 2 {
		tx, _ = pool.Begin(ctx)
		if err = c.ReserveToolTx(ctx, tx, i); err != nil {
			t.Fatal(err)
		}
		if err = tx.Commit(ctx); err != nil {
			t.Fatal(err)
		}
	}
	scope, err = c.Ledger.Get(ctx, scope.ID)
	if err != nil || scope.Used.ExternalCalls != 1 {
		t.Fatal("effect replay charged again", scope, err)
	}
	deadline, err := c.ToolDeadline(ctx, "owner", i.EffectID)
	if err != nil || !deadline.After(time.Now()) || deadline.After(time.Now().Add(time.Duration(p.ToolMillis)*time.Millisecond)) {
		t.Fatal("reserved runtime deadline not bounded", deadline, err)
	}
}

func TestRuntimeAdmissionRejectsFinishedRunAndAggregateBytesIntegration(t *testing.T) {
	pool := controllerDB(t)
	ctx := context.Background()
	p := testPolicy()
	p.Task.ProviderInputBytes = p.ModelInputBytes
	c := NewController(pool, p)
	if _, err := pool.Exec(ctx, `UPDATE runs SET current_state='finalizing' WHERE run_id='run'`); err != nil {
		t.Fatal(err)
	}
	if _, err := c.AdmitModel(modelCtx("run"), "run", "", "fixture", "fixture", 20); !errors.Is(err, ErrConflict) {
		t.Fatal("frozen finalization admitted new inference", err)
	}
	if _, err := pool.Exec(ctx, `UPDATE runs SET current_state='model_running' WHERE run_id='run'`); err != nil {
		t.Fatal(err)
	}
	if _, err := c.AdmitModel(modelCtx("run"), "run", "", "fixture", "fixture", 20); err != nil {
		t.Fatal(err)
	}
	_, err := NewController(pool, p).AdmitModel(modelCtx("run"), "run", "", "fixture", "fixture", 20)
	var exhausted *ExhaustedError
	if !errors.As(err, &exhausted) || exhausted.Dimension != "provider_input_bytes" {
		t.Fatal("aggregate bytes reset on restart", err)
	}
}

func TestBudgetRollbackRefusesUnspentImmutablePolicyIntegration(t *testing.T) {
	pool := controllerDB(t)
	ctx := context.Background()
	ledger := NewLedger(pool)
	scope, err := ledger.Ensure(ctx, ScopeSpec{Kind: "task", OwnerID: "owner", SubjectID: "root", Deadline: time.Now().Add(time.Hour)})
	if err != nil {
		t.Fatal(err)
	}
	down, err := os.ReadFile(filepath.Join("..", "..", "migrations", "037_execution_budgets.down.sql"))
	if err != nil {
		t.Fatal(err)
	}
	if _, err = pool.Exec(ctx, string(down)); err == nil {
		t.Fatal("rollback erased a chosen zero-limit policy")
	}
	if _, err = ledger.Get(ctx, scope.ID); err != nil {
		t.Fatal("refused rollback lost policy", err)
	}
}

func TestBudgetRootPreflightDoesNotChargeOrResetAdmissionIntegration(t *testing.T) {
	pool := controllerDB(t)
	ctx := context.Background()
	p := testPolicy()
	p.Day.ModelTurns = 1
	c := NewController(pool, p)
	m := MandateScope{ID: "mandate", OwnerID: "owner", Privacy: "cloud_allowed", Limits: p.Mandate, Deadline: time.Now().Add(time.Hour).Truncate(time.Microsecond)}
	a := ModelAllowance{Provider: "fixture", Model: "fixture", InputTokens: 20, OutputTokens: 8}
	tx, _ := pool.Begin(ctx)
	if err := c.CheckRootAdmissionTx(ctx, tx, "owner", m, a); err != nil {
		t.Fatal(err)
	}
	if err := tx.Commit(ctx); err != nil {
		t.Fatal(err)
	}
	var count int
	if err := pool.QueryRow(ctx, `SELECT COUNT(*) FROM execution_budget_reservations`).Scan(&count); err != nil || count != 0 {
		t.Fatal("promotion spent inference", count, err)
	}
	var now time.Time
	if err := pool.QueryRow(ctx, `SELECT clock_timestamp()`).Scan(&now); err != nil {
		t.Fatal(err)
	}
	day := now.UTC().Truncate(24 * time.Hour)
	s, err := c.Ledger.Ensure(ctx, ScopeSpec{Kind: "owner_day", OwnerID: "owner", SubjectID: day.Format("2006-01-02"), Limits: p.Day, Deadline: day.Add(24 * time.Hour)})
	if err != nil {
		t.Fatal(err)
	}
	if _, _, err = c.Ledger.Reserve(ctx, Request{OwnerID: "owner", OperationID: "actual-attempt", ScopeIDs: []string{s.ID}, Charge: Units{ModelTurns: 1}, CostKnown: true}); err != nil {
		t.Fatal(err)
	}
	tx, _ = pool.Begin(ctx)
	err = NewController(pool, p).CheckRootAdmissionTx(ctx, tx, "owner", m, a)
	tx.Rollback(ctx)
	var exhausted *ExhaustedError
	if !errors.As(err, &exhausted) || exhausted.Dimension != "model_turns" || exhausted.Kind != "owner_day" {
		t.Fatal("promotion ignored current exhausted day", err)
	}
}
func TestNormalizedLoopPersistsAcrossRestartAndTypedErrorsIntegration(t *testing.T) {
	pool := controllerDB(t)
	ctx := context.Background()
	p := testPolicy()
	c := NewController(pool, p)
	i := ToolIdentity{RunID: "run", ToolName: "http.request", ArgsHash: "canonical-equivalent-JSON", ContractHash: "v1", Effect: "read", TaskRevision: 1, ConstraintsRevision: 1}
	for n := range 3 {
		tx, _ := pool.Begin(ctx)
		i.EffectID = fmt.Sprintf("loop-%d", n)
		if err := c.ReserveToolTx(ctx, tx, i); err != nil {
			t.Fatal(err)
		}
		if err := c.ObserveToolTx(ctx, tx, i, "ERROR_CLASS_TOOL_RUNTIME_ERROR", ""); err != nil {
			t.Fatal(err)
		}
		if err := tx.Commit(ctx); err != nil {
			t.Fatal(err)
		}
	}
	c = NewController(pool, p)
	tx, _ := pool.Begin(ctx)
	i.EffectID = "loop-next"
	err := c.ReserveToolTx(ctx, tx, i)
	tx.Rollback(ctx)
	var loop *LoopError
	if !errors.As(err, &loop) || loop.Reason != "repeated_action_without_progress" {
		t.Fatal("typed equivalent errors did not stop", err)
	}
	i.ConstraintsRevision++
	tx, _ = pool.Begin(ctx)
	err = c.ReserveToolTx(ctx, tx, i)
	tx.Rollback(ctx)
	if err != nil {
		t.Fatal("new constrained state rejected by old loop", err)
	}
	// A genuinely unknown write prevents tool switching in a new run. Its
	// reconciliation changes journal state; counters remain permanently charged.
	_, err = pool.Exec(ctx, `INSERT INTO action_effects(action_id,fingerprint,initial_binding,policy,state,idempotency_key) VALUES('uncertain','f','{"owner_id":"owner","task_id":"root","effect":"commit"}','{}','effect_unknown','key')`)
	if err != nil {
		t.Fatal(err)
	}
	i.Effect = "commit"
	i.ToolName = "different.tool"
	tx, _ = pool.Begin(ctx)
	err = c.ReserveToolTx(ctx, tx, i)
	tx.Rollback(ctx)
	if !errors.As(err, &loop) || loop.Reason != "external_effect_unknown" {
		t.Fatal("tool switching replayed unknown effect", err)
	}
}
