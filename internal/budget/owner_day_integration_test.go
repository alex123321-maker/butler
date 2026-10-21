package budget

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/butler/butler/internal/ownership"
	"github.com/butler/butler/internal/security/content"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

func ownerDayBudgetDB(t *testing.T) *pgxpool.Pool {
	t.Helper()
	pool := controllerDB(t)
	// The minimal controller fixture predates local/auxiliary scopes. Include
	// migration047's scope kind and the real run timestamp used by local queries.
	_, err := pool.Exec(context.Background(), `ALTER TABLE execution_budget_scopes DROP CONSTRAINT execution_budget_scopes_kind_check;
ALTER TABLE execution_budget_scopes ADD CONSTRAINT execution_budget_scopes_kind_check CHECK(kind IN ('task','mandate','owner_day','conversation','auxiliary'));
ALTER TABLE runs ADD COLUMN started_at TIMESTAMPTZ NOT NULL DEFAULT clock_timestamp()`)
	if err != nil {
		t.Fatal(err)
	}
	return pool
}

func ownerDayBudgetTx(t *testing.T, pool *pgxpool.Pool) pgx.Tx {
	t.Helper()
	tx, err := pool.Begin(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = tx.Rollback(context.Background()) })
	return tx
}

func TestOwnerDayOptOutPreservesSpentHistoryAndOtherLimitsIntegration(t *testing.T) {
	pool := ownerDayBudgetDB(t)
	ctx := context.Background()
	p := testPolicy()
	p.Day.ModelTurns = 1
	p.Task.ModelTurns = 3
	p.Task.ExternalCalls = 1
	p.Task.Children = 1
	p.Conversation.ModelTurns = 1
	p.Models["local:google/embeddinggemma-2"] = ModelPrice{Basis: "local_no_billing"}
	c := NewController(pool, p)
	if _, err := c.AdmitModel(modelCtx("run"), "run", "", "fixture", "fixture", 20); err != nil {
		t.Fatal(err)
	}
	var dayID string
	if err := pool.QueryRow(ctx, `SELECT scope_id FROM execution_budget_scopes WHERE kind='owner_day'`).Scan(&dayID); err != nil {
		t.Fatal(err)
	}
	before, err := c.Ledger.Get(ctx, dayID)
	if err != nil {
		t.Fatal(err)
	}
	p.DisableOwnerDay = true
	c = NewController(pool, p) // Restart with the explicit operator policy.
	c.SetAuthorityResolver(func(context.Context, pgx.Tx, string, string) (MandateScope, error) { return MandateScope{}, nil })
	tx := ownerDayBudgetTx(t, pool)
	err = c.CheckScheduledRootAdmissionTx(ctx, tx, "owner", nil, ModelAllowance{Provider: "fixture", Model: "fixture", InputTokens: 20, OutputTokens: 8})
	tx.Rollback(ctx)
	if err != nil {
		t.Fatal("exhausted historical day blocked scheduling", err)
	}
	if _, err = c.AdmitModel(modelCtx("run"), "run", "", "fixture", "fixture", 20); err != nil {
		t.Fatal("exhausted historical day blocked model", err)
	}
	target, _ := NewLocalTarget("local-models:9090", "pinned-fixture")
	_, release, err := c.AdmitLocalModel(modelCtx("run"), "run", target, LocalOperation{Kind: "query", Items: 1, InputBytes: 16, Timeout: time.Second, Origin: content.Derive(content.MemoryDerived)}, func(context.Context, pgx.Tx) error { return nil })
	if err != nil {
		t.Fatal("exhausted historical day blocked local query", err)
	}
	release()
	_, err = c.AdmitModel(modelCtx("run"), "run", "", "fixture", "fixture", 20)
	assertBudgetDimension(t, err, "task", "model_turns")
	for n := range 2 {
		tx = ownerDayBudgetTx(t, pool)
		err = c.ReserveToolTx(ctx, tx, ToolIdentity{RunID: "run", EffectID: []string{"first", "second"}[n], ToolName: "http.request", Effect: "read"})
		if n == 0 {
			if err != nil {
				t.Fatal(err)
			}
			if err = tx.Commit(ctx); err != nil {
				t.Fatal(err)
			}
		} else {
			tx.Rollback(ctx)
			assertBudgetDimension(t, err, "task", "external_calls")
		}
	}
	for n := range 2 {
		tx = ownerDayBudgetTx(t, pool)
		err = c.ReserveChildTx(ctx, tx, "root", "", []string{"child-one", "child-two"}[n])
		if n == 0 {
			if err != nil {
				t.Fatal(err)
			}
			if err = tx.Commit(ctx); err != nil {
				t.Fatal(err)
			}
		} else {
			tx.Rollback(ctx)
			assertBudgetDimension(t, err, "task", "children")
		}
	}
	// A conversation still has its own finite scope, independent of owner/day.
	if _, err = pool.Exec(ctx, `INSERT INTO runs(run_id,session_key,task_revision) VALUES('conversation','session',0); UPDATE session_ownership SET run_id='conversation'`); err != nil {
		t.Fatal(err)
	}
	if _, err = c.AdmitModel(modelCtx("conversation"), "conversation", "", "fixture", "fixture", 20); err != nil {
		t.Fatal(err)
	}
	_, err = c.AdmitModel(modelCtx("conversation"), "conversation", "", "fixture", "fixture", 20)
	assertBudgetDimension(t, err, "conversation", "model_turns")
	after, err := c.Ledger.Get(ctx, dayID)
	if err != nil || after != before {
		t.Fatal("daily history changed while disabled", err)
	}
	var links int
	if err = pool.QueryRow(ctx, `SELECT count(*) FROM execution_budget_reservations WHERE request_json->'scope_ids' ? $1`, dayID).Scan(&links); err != nil || links != 1 {
		t.Fatal("new spending attached to disabled owner/day", links, err)
	}
	// Re-enabling preserves the original exhausted scope; it cannot refill it.
	p.DisableOwnerDay = false
	if _, err = pool.Exec(ctx, `INSERT INTO sessions VALUES('fresh','owner'); INSERT INTO runs(run_id,session_key,task_revision) VALUES('fresh','fresh',0); INSERT INTO session_ownership VALUES('fresh','fresh','lease',1,clock_timestamp()+interval '1 hour')`); err != nil {
		t.Fatal(err)
	}
	freshCtx := ownership.WithToken(ctx, nil, ownership.Token{SessionKey: "fresh", RunID: "fresh", LeaseID: "lease", FenceEpoch: 1})
	_, err = NewController(pool, p).AdmitModel(freshCtx, "fresh", "", "fixture", "fixture", 20)
	assertBudgetDimension(t, err, "owner_day", "model_turns")
}

func TestOwnerDayOptOutKeepsAuxiliaryAndMandateBoundsIntegration(t *testing.T) {
	pool := ownerDayBudgetDB(t)
	ctx := context.Background()
	p := testPolicy()
	p.DisableOwnerDay = true
	p.Day = Units{} // Disabled daily zeros must not deny any path.
	c := NewController(pool, p)
	deadline := time.Now().Add(time.Hour).UTC().Truncate(time.Microsecond)
	i := AuxiliaryIdentity{OwnerID: "owner", JobID: "independent-job", OperationID: "job-one", Limits: Units{ModelTurns: 1, Tokens: 100, ActiveMillis: 1000, ProviderInputBytes: 100}, Deadline: deadline, InputBytes: 10, Tokens: 20, ActiveMillis: 100}
	tx := ownerDayBudgetTx(t, pool)
	if _, err := c.ReserveAuxiliaryTx(ctx, tx, i); err != nil {
		t.Fatal("disabled day blocked independent job", err)
	}
	if err := tx.Commit(ctx); err != nil {
		t.Fatal(err)
	}
	i.OperationID = "job-two"
	tx = ownerDayBudgetTx(t, pool)
	_, err := c.ReserveAuxiliaryTx(ctx, tx, i)
	tx.Rollback(ctx)
	assertBudgetDimension(t, err, "auxiliary", "model_turns")
	m := MandateScope{ID: "assigned", OwnerID: "owner", Privacy: "cloud_allowed", Limits: p.Mandate, Deadline: deadline}
	m.Limits.ModelTurns = 1
	c.SetAuthorityResolver(func(context.Context, pgx.Tx, string, string) (MandateScope, error) { return m, nil })
	if _, err = c.AdmitModel(modelCtx("run"), "run", "", "fixture", "fixture", 20); err != nil {
		t.Fatal(err)
	}
	_, err = c.AdmitModel(modelCtx("run"), "run", "", "fixture", "fixture", 20)
	assertBudgetDimension(t, err, "mandate", "model_turns")
	tx = ownerDayBudgetTx(t, pool)
	err = c.CheckRootAdmissionTx(ctx, tx, "owner", m, ModelAllowance{Provider: "fixture", Model: "fixture", InputTokens: 20, OutputTokens: 8})
	tx.Rollback(ctx)
	assertBudgetDimension(t, err, "mandate", "model_turns")
	var days int
	if err = pool.QueryRow(ctx, `SELECT count(*) FROM execution_budget_scopes WHERE kind='owner_day'`).Scan(&days); err != nil || days != 0 {
		t.Fatal("disabled policy created daily scopes", days, err)
	}
}

func assertBudgetDimension(t *testing.T, err error, kind, dimension string) {
	t.Helper()
	var exhausted *ExhaustedError
	if !errors.As(err, &exhausted) || exhausted.Kind != kind || exhausted.Dimension != dimension {
		t.Fatalf("expected %s/%s exhaustion, got %v", kind, dimension, err)
	}
}
