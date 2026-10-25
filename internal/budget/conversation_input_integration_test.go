package budget

import (
	"context"
	"testing"
)

func TestConversationInputScopeIsDurableAndDoesNotSpendAnotherRequestIntegration(t *testing.T) {
	pool := controllerDB(t)
	ctx := context.Background()
	_, err := pool.Exec(ctx, `INSERT INTO runs(run_id,session_key,task_revision) VALUES('first','session',0),('second','session',0),('retry','session',0),('legacy','session',0);
INSERT INTO accepted_inputs(input_id,run_id,owner_id,session_key,communication_ingress_epoch,context_epoch) VALUES('input-a','first','owner','session',1,1),('input-b','second','owner','session',1,1),('input-old','legacy','owner','session',1,0);`)
	if err != nil {
		t.Fatal(err)
	}
	p := testPolicy()
	p.Conversation.ModelTurns = 1
	p.Day.ModelTurns = 100
	controller := NewController(pool, p)
	scope := func(run string) ScopeSpec {
		t.Helper()
		tx, e := pool.Begin(ctx)
		if e != nil {
			t.Fatal(e)
		}
		defer tx.Rollback(ctx)
		_, specs, e := controller.scopeSpecsTx(ctx, tx, run, "", "")
		if e != nil {
			t.Fatal(e)
		}
		for _, v := range specs {
			if v.Kind == "conversation" {
				return v
			}
		}
		t.Fatal("no scope")
		return ScopeSpec{}
	}
	a, b, old := scope("first"), scope("second"), scope("legacy")
	if a.SubjectID != "input:input-a" || b.SubjectID != "input:input-b" || old.SubjectID == a.SubjectID || old.SubjectID == b.SubjectID {
		t.Fatal(a, b, old)
	}
	_, _, err = controller.Ledger.Reserve(ctx, Request{OwnerID: "owner", OperationID: "one", ScopeIDs: []string{ScopeID(a.Kind, a.OwnerID, a.SubjectID)}, Charge: Units{ModelTurns: 1}, CostKnown: true})
	// Scope creation is normally done by admission. Materialize through its
	// owning API here, then test exhaustion and immutable policy on restart.
	if err == nil {
		t.Fatal("missing scope unexpectedly existed")
	}
	if _, err = controller.Ledger.Ensure(ctx, a); err != nil {
		t.Fatal(err)
	}
	if _, err = controller.Ledger.Ensure(ctx, b); err != nil {
		t.Fatal(err)
	}
	if _, _, err = controller.Ledger.Reserve(ctx, Request{OwnerID: "owner", OperationID: "one", ScopeIDs: []string{ScopeID(a.Kind, a.OwnerID, a.SubjectID)}, Charge: Units{ModelTurns: 1}, CostKnown: true}); err != nil {
		t.Fatal(err)
	}
	_, _, err = controller.Ledger.Reserve(ctx, Request{OwnerID: "owner", OperationID: "again", ScopeIDs: []string{ScopeID(a.Kind, a.OwnerID, a.SubjectID)}, Charge: Units{ModelTurns: 1}, CostKnown: true})
	assertBudgetDimension(t, err, "conversation", "model_turns")
	if _, err = pool.Exec(ctx, `UPDATE accepted_inputs SET run_id='retry' WHERE input_id='input-a'`); err != nil {
		t.Fatal(err)
	}
	controller = NewController(pool, p)
	if retry := scope("retry"); retry != a {
		t.Fatal("restart or rerun refilled input scope", retry, a)
	}
	if _, _, err = controller.Ledger.Reserve(ctx, Request{OwnerID: "owner", OperationID: "independent", ScopeIDs: []string{ScopeID(b.Kind, b.OwnerID, b.SubjectID)}, Charge: Units{ModelTurns: 1}, CostKnown: true}); err != nil {
		t.Fatal("another request inherited exhaustion", err)
	}
}
