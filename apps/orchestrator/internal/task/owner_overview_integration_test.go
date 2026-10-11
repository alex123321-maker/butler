package task

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/butler/butler/internal/budget"
	"github.com/butler/butler/internal/memory/memorytest"
	"github.com/butler/butler/internal/owneridentity"
)

func TestOwnerOverviewSeparatesSharedReservationsUnknownUsageAndEffectsIntegration(t *testing.T) {
	pool := memorytest.Database(t)
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	owner := owneridentity.WithActor(ctx, owneridentity.Actor{OwnerID: "owner", Channel: "web", DeliveryID: "overview"})
	if _, err := pool.Exec(ctx, `INSERT INTO sessions(session_key,user_id,channel) VALUES('overview','owner','web')`); err != nil {
		t.Fatal(err)
	}
	store := NewStore(pool)
	root, err := store.Create(ctx, CreateRequest{SessionKey: "overview", OwnerID: "owner", Goal: "Verified owner result", CompletionCriteria: []string{"objective evidence"}, Source: map[string]any{"private": "PRIVATE_OVERVIEW_MARKER"}})
	if err != nil {
		t.Fatal(err)
	}
	child, err := store.Create(ctx, CreateRequest{SessionKey: "overview", OwnerID: "owner", ParentTaskID: root.TaskID, Goal: "Independent child", CompletionCriteria: []string{"child evidence"}})
	if err != nil {
		t.Fatal(err)
	}
	ledger := budget.NewLedger(pool)
	limits := budget.Units{ModelTurns: 5, Tokens: 100}
	scope, err := ledger.Ensure(ctx, budget.ScopeSpec{Kind: "task", OwnerID: "owner", SubjectID: root.TaskID, Limits: limits, Deadline: time.Now().Add(time.Hour)})
	if err != nil {
		t.Fatal(err)
	}
	for index, taskID := range []string{root.TaskID, child.TaskID, child.TaskID} {
		runID := []string{"overview-root", "overview-child-known", "overview-child-partial"}[index]
		reservation, _, err := ledger.Reserve(ctx, budget.Request{OwnerID: "owner", OperationID: runID, ScopeIDs: []string{scope.ID}, Charge: budget.Units{ModelTurns: 1, Tokens: 10}, CostKnown: true})
		if err != nil {
			t.Fatal(err)
		}
		if _, err = pool.Exec(ctx, `INSERT INTO runs(run_id,session_key,input_event_id,status,current_state,autonomy_mode,model_provider,started_at,updated_at,task_id,task_revision,error_message) VALUES($1,'overview',$1,'completed','completed','mode_1','fixture',clock_timestamp(),clock_timestamp(),$2,1,'PRIVATE_OVERVIEW_MARKER')`, runID, taskID); err != nil {
			t.Fatal(err)
		}
		var input, output *int64
		if index == 1 {
			in, out := int64(12), int64(4)
			input, output = &in, &out
		}
		if index == 2 {
			in := int64(8)
			input = &in
		}
		if _, err = pool.Exec(ctx, `INSERT INTO execution_model_attempts(operation_id,run_id,reservation_id,provider,model,input_allowance,output_allowance,actual_input_tokens,actual_output_tokens) VALUES($1,$1,$2,'fixture','fixture',10,10,$3,$4)`, runID, reservation.ID, input, output); err != nil {
			t.Fatal(err)
		}
	}
	binding := map[string]any{"action_id": "overview-action", "tool_name": "http.request", "task_id": root.TaskID, "task_revision": 1, "owner_id": "owner", "effect": "commit", "contract_hash": strings.Repeat("a", 64), "args": map[string]any{"private": "PRIVATE_OVERVIEW_MARKER"}}
	raw, _ := json.Marshal(binding)
	if _, err = pool.Exec(ctx, `INSERT INTO action_effects(action_id,fingerprint,initial_binding,policy,state) VALUES('overview-effect',$1,$2,'{}','effect_unknown')`, strings.Repeat("a", 64), raw); err != nil {
		t.Fatal(err)
	}
	if _, err = pool.Exec(ctx, `INSERT INTO action_intents(action_id,run_id,tool_call_id,action_hash,binding,requires_approval,expires_at,effect_id) VALUES('overview-action','overview-root','overview-call',$1,$2,true,clock_timestamp()+INTERVAL '1 hour','overview-effect')`, strings.Repeat("a", 64), raw); err != nil {
		t.Fatal(err)
	}
	v, err := store.OwnerOverview(owner, root.TaskID)
	if err != nil {
		t.Fatal(err)
	}
	if len(v.Children) != 1 || v.Children[0].Status != "active" || v.Children[0].CompletionResultID != "" {
		t.Fatal("completed model attempt was presented as verified child completion")
	}
	if len(v.Actions) != 1 || v.Actions[0].Effect != "commit" || v.Actions[0].EffectState != "effect_unknown" {
		t.Fatal("action consequence/effect were conflated")
	}
	if v.Budget.ModelAttempts != 3 || v.Budget.KnownInputTokens != 20 || v.Budget.KnownOutputTokens != 4 || v.Budget.UnknownInputAttempts != 1 || v.Budget.UnknownOutputAttempts != 2 || v.Budget.Reserved.ModelTurns != 3 || v.Budget.Reserved.Tokens != 30 {
		t.Fatal("reservations and incomplete actual usage were conflated")
	}
	childView, err := NewStore(pool).OwnerOverview(owner, child.TaskID)
	if err != nil || childView.Budget.RootTaskID != root.TaskID || childView.Budget.ModelAttempts != 3 || childView.Budget.Reserved.ModelTurns != 3 {
		t.Fatal("child read reset or detached the shared root budget")
	}
	encoded, _ := json.Marshal(v)
	if strings.Contains(string(encoded), "PRIVATE_OVERVIEW_MARKER") {
		t.Fatal("source, arguments or raw error escaped overview")
	}
	if _, err = store.OwnerOverview(ctx, root.TaskID); !errors.Is(err, ErrUnauthorized) {
		t.Fatal("missing actor accepted")
	}
	foreign := owneridentity.WithActor(ctx, owneridentity.Actor{OwnerID: "foreign", Channel: "web", DeliveryID: "foreign"})
	if _, err = store.OwnerOverview(foreign, root.TaskID); !errors.Is(err, ErrUnauthorized) {
		t.Fatal("foreign owner accepted")
	}
}

func TestOwnerOverviewMissingBudgetIsUnavailableIntegration(t *testing.T) {
	pool := memorytest.Database(t)
	ctx := owneridentity.WithActor(context.Background(), owneridentity.Actor{OwnerID: "owner", Channel: "telegram", DeliveryID: "overview"})
	if _, err := pool.Exec(ctx, `INSERT INTO sessions(session_key,user_id,channel) VALUES('overview-empty','owner','telegram')`); err != nil {
		t.Fatal(err)
	}
	store := NewStore(pool)
	r, err := store.Create(ctx, CreateRequest{SessionKey: "overview-empty", OwnerID: "owner", Goal: "No attempts yet", CompletionCriteria: []string{"evidence"}})
	if err != nil {
		t.Fatal(err)
	}
	v, err := store.OwnerOverview(ctx, r.TaskID)
	if err != nil || v.Budget.Limits != nil || v.Budget.Reserved != nil || v.Budget.ModelAttempts != 0 || v.Children == nil || v.Actions == nil {
		t.Fatal("absent budget was presented as a zero limit")
	}
}
