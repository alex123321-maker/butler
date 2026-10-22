package budget

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"testing"

	"github.com/jackc/pgx/v5/pgxpool"
)

func conversationLoopDB(t *testing.T) *pgxpool.Pool {
	t.Helper()
	pool := controllerDB(t)
	_, err := pool.Exec(context.Background(), `ALTER TABLE runs ADD COLUMN input_event_id TEXT NOT NULL DEFAULT '';
INSERT INTO runs(run_id,session_key,input_event_id) VALUES('conversation-a','session','owner-input-a'),('conversation-b','session','owner-input-b'),('conversation-resumed','session','owner-input-a')`)
	if err != nil {
		t.Fatal(err)
	}
	return pool
}

func conversationLoopCall(run, effect string) ToolIdentity {
	return ToolIdentity{RunID: run, EffectID: effect, ToolName: "tools.search", ArgsHash: "same-communication-category", ContractHash: "catalog-v1", Effect: "read"}
}

func observeConversationCall(t *testing.T, pool *pgxpool.Pool, c *Controller, i ToolIdentity) {
	t.Helper()
	ctx := context.Background()
	binding, _ := json.Marshal(map[string]string{"owner_id": "owner", "task_id": "", "run_id": i.RunID, "effect": "read"})
	tx, err := pool.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Rollback(ctx)
	if _, err = tx.Exec(ctx, `INSERT INTO action_effects(action_id,fingerprint,initial_binding,policy,state) VALUES($1,$1,$2,'{}','effect_confirmed')`, i.EffectID, binding); err != nil {
		t.Fatal(err)
	}
	if err = c.ReserveToolTx(ctx, tx, i); err != nil {
		t.Fatal("independent Input or bounded call rejected", i.RunID, err)
	}
	if err = c.ObserveToolTx(ctx, tx, i, "success", "same-permitted-catalog"); err != nil {
		t.Fatal(err)
	}
	if err = tx.Commit(ctx); err != nil {
		t.Fatal(err)
	}
}

func checkConversationCall(t *testing.T, pool *pgxpool.Pool, c *Controller, i ToolIdentity) error {
	t.Helper()
	tx, err := pool.Begin(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Rollback(context.Background())
	return c.CheckToolTx(context.Background(), tx, i)
}

func assertConversationLoopStopped(t *testing.T, err error, reason string) {
	t.Helper()
	var loop *LoopError
	if !errors.As(err, &loop) || loop.Reason != reason {
		t.Fatal("expected preserved loop/effect protection", reason, err)
	}
}

func TestCommunicationConversationLoopIndependentInputsKeepHistoryAndLimitsIntegration(t *testing.T) {
	pool := conversationLoopDB(t)
	ctx := context.Background()
	p := testPolicy()
	p.Conversation.ExternalCalls = int64(p.RepeatLimit + 2)
	p.Conversation.ModelTurns = 1
	c := NewController(pool, p)
	if _, err := pool.Exec(ctx, `UPDATE session_ownership SET run_id='conversation-a'`); err != nil {
		t.Fatal(err)
	}
	if _, err := c.AdmitModel(modelCtx("conversation-a"), "conversation-a", "", "fixture", "fixture", 20); err != nil {
		t.Fatal(err)
	}
	var modelHistory, policies string
	if err := pool.QueryRow(ctx, `SELECT (SELECT string_agg(row_to_json(a)::text,'' ORDER BY operation_id) FROM execution_model_attempts a),(SELECT string_agg(jsonb_build_array(scope_id,limits_json,deadline)::text,'' ORDER BY scope_id) FROM execution_budget_scopes)`).Scan(&modelHistory, &policies); err != nil {
		t.Fatal(err)
	}
	// These are the old-format observations, written by the same owning method
	// before and after the fix. Each new Input asks for the identical catalog.
	for n := int64(0); n < p.RepeatLimit+2; n++ {
		run := fmt.Sprintf("independent-%d", n)
		if _, err := pool.Exec(ctx, `INSERT INTO runs(run_id,session_key,input_event_id) VALUES($1,'session',$1)`, run); err != nil {
			t.Fatal(err)
		}
		observeConversationCall(t, pool, c, conversationLoopCall(run, "effect-"+run))
	}
	var history string
	if err := pool.QueryRow(ctx, `SELECT string_agg(row_to_json(x)::text,'' ORDER BY effect_id) FROM execution_loop_observations x`).Scan(&history); err != nil {
		t.Fatal(err)
	}
	var scopes, calls int64
	if err := pool.QueryRow(ctx, `SELECT count(*),min((used_json->>'external_calls')::int) FROM execution_budget_scopes`).Scan(&scopes, &calls); err != nil || scopes != 2 || calls != p.RepeatLimit+2 {
		t.Fatal("new Input split or reset conversation/day spending", scopes, calls, err)
	}
	// New owner input changes repetition scope, never the finite budget scope.
	tx, err := pool.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	err = NewController(pool, p).ReserveToolTx(ctx, tx, conversationLoopCall("conversation-b", "over-budget"))
	tx.Rollback(ctx)
	var exhausted *ExhaustedError
	if !errors.As(err, &exhausted) || exhausted.Kind != "conversation" || exhausted.Dimension != "external_calls" {
		t.Fatal("new Input replenished finite conversation allowance", err)
	}
	var after string
	if err = pool.QueryRow(ctx, `SELECT string_agg(row_to_json(x)::text,'' ORDER BY effect_id) FROM execution_loop_observations x`).Scan(&after); err != nil || history != after {
		t.Fatal("loop history rewritten or removed", err)
	}
	if _, err = pool.Exec(ctx, `UPDATE session_ownership SET run_id='conversation-b'`); err != nil {
		t.Fatal(err)
	}
	_, err = NewController(pool, p).AdmitModel(modelCtx("conversation-b"), "conversation-b", "", "fixture", "fixture", 20)
	if !errors.As(err, &exhausted) || exhausted.Kind != "conversation" || exhausted.Dimension != "model_turns" {
		t.Fatal("new Input replenished finite model turns", err)
	}
	var afterModel, afterPolicies string
	if err = pool.QueryRow(ctx, `SELECT (SELECT string_agg(row_to_json(a)::text,'' ORDER BY operation_id) FROM execution_model_attempts a),(SELECT string_agg(jsonb_build_array(scope_id,limits_json,deadline)::text,'' ORDER BY scope_id) FROM execution_budget_scopes)`).Scan(&afterModel, &afterPolicies); err != nil || afterModel != modelHistory || afterPolicies != policies {
		t.Fatal("historical attempt/NULL usage or immutable policies changed", err)
	}
}

func TestCommunicationConversationLoopSameInputSurvivesResumeAndRestartIntegration(t *testing.T) {
	pool := conversationLoopDB(t)
	p := testPolicy()
	for n := int64(0); n < p.RepeatLimit; n++ {
		observeConversationCall(t, pool, NewController(pool, p), conversationLoopCall("conversation-a", fmt.Sprintf("repeat-%d", n)))
	}
	for _, run := range []string{"conversation-a", "conversation-resumed"} {
		t.Run(run, func(t *testing.T) {
			assertConversationLoopStopped(t, checkConversationCall(t, pool, NewController(pool, p), conversationLoopCall(run, "next")), "repeated_action_without_progress")
		})
	}
	if err := checkConversationCall(t, pool, NewController(pool, p), conversationLoopCall("conversation-b", "independent")); err != nil {
		t.Fatal("different Input inherited previous completed request's repetitions", err)
	}
}

func TestCommunicationConversationLoopNewInputCannotEvadeUnknownEffectIntegration(t *testing.T) {
	for _, state := range []string{"executing", "effect_unknown"} {
		t.Run(state, func(t *testing.T) {
			pool := conversationLoopDB(t)
			ctx := context.Background()
			_, err := pool.Exec(ctx, `INSERT INTO action_effects(action_id,fingerprint,initial_binding,policy,state) VALUES('uncertain','f','{"owner_id":"owner","task_id":"","run_id":"conversation-a","effect":"commit"}','{}',$1)`, state)
			if err != nil {
				t.Fatal(err)
			}
			c := NewController(pool, testPolicy())
			i := conversationLoopCall("conversation-b", "new-input")
			if err = checkConversationCall(t, pool, c, i); err != nil {
				t.Fatal("independent authorized read prevented reconciliation", err)
			}
			i.Effect, i.ToolName = "commit", "different.write"
			assertConversationLoopStopped(t, checkConversationCall(t, pool, c, i), "external_effect_unknown")
			var saved string
			if err = pool.QueryRow(ctx, `SELECT state FROM action_effects WHERE action_id='uncertain'`).Scan(&saved); err != nil || saved != state {
				t.Fatal("new Input reconciled unknown write", saved, err)
			}
		})
	}
}

func TestCommunicationConversationLoopMissingProvenanceRemainsConservativeIntegration(t *testing.T) {
	pool := conversationLoopDB(t)
	ctx := context.Background()
	p := testPolicy()
	c := NewController(pool, p)
	// Legacy observations without a surviving exact effect/run cannot safely be
	// attributed to another Input. Preserve their prior restrictive meaning.
	for n := int64(0); n < p.RepeatLimit; n++ {
		tx, err := pool.Begin(ctx)
		if err != nil {
			t.Fatal(err)
		}
		err = c.ObserveToolTx(ctx, tx, conversationLoopCall("conversation-a", fmt.Sprintf("legacy-%d", n)), "success", "same-permitted-catalog")
		if err != nil {
			t.Fatal(err)
		}
		if err = tx.Commit(ctx); err != nil {
			t.Fatal(err)
		}
	}
	assertConversationLoopStopped(t, checkConversationCall(t, pool, NewController(pool, p), conversationLoopCall("conversation-b", "cannot-prove")), "repeated_action_without_progress")
}

func TestCommunicationConversationLoopTaskAncestryCannotBecomeInputScopeIntegration(t *testing.T) {
	pool := conversationLoopDB(t)
	ctx := context.Background()
	p := testPolicy()
	i := conversationLoopCall("run", "")
	i.TaskRevision, i.ConstraintsRevision = 1, 1
	for n := int64(0); n < p.RepeatLimit; n++ {
		i.EffectID = fmt.Sprintf("task-repeat-%d", n)
		observeConversationCall(t, pool, NewController(pool, p), i)
	}
	if _, err := pool.Exec(ctx, `INSERT INTO runs(run_id,session_key,task_id,task_revision,input_event_id) VALUES('child-run','session','child',1,'different-input'); INSERT INTO tasks(task_id,parent_task_id,owner_id,session_key) VALUES('broken-task','missing-parent','owner','session');INSERT INTO runs(run_id,session_key,task_id,input_event_id) VALUES('broken-run','session','broken-task','different-input')`); err != nil {
		t.Fatal(err)
	}
	i.RunID, i.EffectID = "child-run", "child-next"
	assertConversationLoopStopped(t, checkConversationCall(t, pool, NewController(pool, p), i), "repeated_action_without_progress")
	i.RunID = "broken-run"
	if err := checkConversationCall(t, pool, NewController(pool, p), i); err == nil {
		t.Fatal("broken task ancestry fell back to an independent Input")
	}
}
