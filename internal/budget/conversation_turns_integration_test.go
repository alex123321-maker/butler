package budget

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/butler/butler/internal/security/content"
	"github.com/jackc/pgx/v5"
)

func TestConversationModelTurnOptOutPreservesAccountingAcrossRestartIntegration(t *testing.T) {
	pool := ownerDayBudgetDB(t)
	ctx := context.Background()
	if _, err := pool.Exec(ctx, `INSERT INTO runs(run_id,session_key,task_revision) VALUES('conversation','session',0); UPDATE session_ownership SET run_id='conversation'`); err != nil {
		t.Fatal(err)
	}
	p := testPolicy()
	p.DisableOwnerDay = true
	p.Conversation.ModelTurns = 1
	p.Models["local:google/embeddinggemma-2"] = ModelPrice{Basis: "local_no_billing"}
	c := NewController(pool, p)
	first, err := c.AdmitModel(modelCtx("conversation"), "conversation", "", "fixture", "fixture", 20)
	if err != nil {
		t.Fatal(err)
	}
	var id string
	if err = pool.QueryRow(ctx, `SELECT scope_id FROM execution_budget_scopes WHERE kind='conversation'`).Scan(&id); err != nil {
		t.Fatal(err)
	}
	before, err := c.Ledger.Get(ctx, id)
	if err != nil {
		t.Fatal(err)
	}
	var retained string
	const history = `SELECT jsonb_build_object('reservation',(SELECT to_jsonb(r) FROM execution_budget_reservations r WHERE reservation_id=$1),'attempt',(SELECT to_jsonb(a) FROM execution_model_attempts a WHERE reservation_id=$1))::text`
	if err = pool.QueryRow(ctx, history, first.ReservationID).Scan(&retained); err != nil {
		t.Fatal(err)
	}
	_, err = c.AdmitModel(modelCtx("conversation"), "conversation", "", "fixture", "fixture", 20)
	assertBudgetDimension(t, err, "conversation", "model_turns")
	p.DisableConversationModelTurns = true
	c = NewController(pool, p)
	if _, err = c.AdmitModel(modelCtx("conversation"), "conversation", "", "fixture", "fixture", 20); err != nil {
		t.Fatal("existing exhausted scope", err)
	}
	target, _ := NewLocalTarget("local-models:9090", "pinned-fixture")
	_, release, err := c.AdmitLocalModel(modelCtx("conversation"), "conversation", target, LocalOperation{Kind: "query", Items: 1, InputBytes: 16, Timeout: time.Second, Origin: content.Derive(content.MemoryDerived)}, func(context.Context, pgx.Tx) error { return nil })
	if err != nil {
		t.Fatal("local call after opt-out", err)
	}
	release()
	after, err := c.Ledger.Get(ctx, id)
	if err != nil || after.ScopeSpec != before.ScopeSpec || after.Used.ModelTurns != 3 {
		t.Fatal("policy reset or missing charge", after, err)
	}
	var original string
	if err = pool.QueryRow(ctx, history, first.ReservationID).Scan(&original); err != nil || original != retained {
		t.Fatal("old attempt/reservation changed", err)
	}
	// Local calls still have their independent per-run auxiliary ceiling.
	_, _, err = c.AdmitLocalModel(modelCtx("conversation"), "conversation", target, LocalOperation{Kind: "query", Items: 1, InputBytes: 16, Timeout: time.Second, Origin: content.Derive(content.MemoryDerived)}, func(context.Context, pgx.Tx) error { return nil })
	assertBudgetDimension(t, err, "auxiliary", "model_turns")
	snapshot, err := c.Ledger.SnapshotOwner(ctx, "owner")
	if err != nil {
		t.Fatal(err)
	}
	// Re-enable with the same stored policy. Spending is retained and denies.
	p.DisableConversationModelTurns = false
	c = NewController(pool, p)
	_, err = c.AdmitModel(modelCtx("conversation"), "conversation", "", "fixture", "fixture", 20)
	assertBudgetDimension(t, err, "conversation", "model_turns")
	next, err := c.Ledger.SnapshotOwner(ctx, "owner")
	if err != nil {
		t.Fatal(err)
	}
	a, _ := json.Marshal(snapshot)
	b, _ := json.Marshal(next)
	if string(a) != string(b) {
		t.Fatal("denied reservation changed ledger")
	}
}
