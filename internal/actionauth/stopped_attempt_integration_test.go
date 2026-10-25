package actionauth

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/butler/butler/internal/budget"
)

func TestStoppedAttemptIsolationDeniesActualGrantClaimIntegration(t *testing.T) {
	s, store, reader, call, contract, receiver, _ := receiptFixture(t)
	ctx := context.Background()
	raw, err := os.ReadFile(filepath.Join("..", "..", "migrations", "037_execution_budgets.up.sql"))
	if err != nil {
		t.Fatal(err)
	}
	if _, err = store.pool.Exec(ctx, string(raw)); err != nil {
		t.Fatal(err)
	}
	if _, err = store.pool.Exec(ctx, `CREATE TABLE sessions(session_key TEXT PRIMARY KEY,user_id TEXT);INSERT INTO sessions VALUES('web:owner','owner');ALTER TABLE runs ADD COLUMN session_key TEXT DEFAULT 'web:owner';ALTER TABLE tasks ADD COLUMN parent_task_id TEXT,ADD COLUMN session_key TEXT DEFAULT 'web:owner',ADD COLUMN created_at TIMESTAMPTZ DEFAULT clock_timestamp(); INSERT INTO runs(run_id,current_state,status) VALUES('old-stopped','failed','failed'); INSERT INTO input_stopped_attempts VALUES('old-input','old-stopped','owner','web:owner','uncertain')`); err != nil {
		t.Fatal(err)
	}
	store.SetBudgets(budget.NewController(store.pool, budget.DefaultPolicy()))
	g := logicalGrant(t, s, reader, call, contract, "isolated-scope")
	err = store.ClaimGrant(ctx, g.ID)
	var loop *budget.LoopError
	if !errors.As(err, &loop) || loop.Reason != "stopped_attempt_uncertain" {
		t.Fatal("receiving claim bypassed isolated uncertainty", err)
	}
	grant, err := store.GetGrant(ctx, g.ID)
	if err != nil || grant.State != "authorized" || receiver.effects.Load() != 0 {
		t.Fatal("denied claim dispatched or spent grant", err)
	}
	var reservations int
	if err = store.pool.QueryRow(ctx, `SELECT count(*) FROM execution_budget_reservations`).Scan(&reservations); err != nil || reservations != 0 {
		t.Fatal("denial partially charged or dispatched", err)
	}
}
