package input

import (
	"context"
	"encoding/json"
	"errors"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/butler/butler/apps/orchestrator/internal/task"
	"github.com/butler/butler/internal/budget"
	runv1 "github.com/butler/butler/internal/gen/run/v1"
	"github.com/butler/butler/internal/ingress"
	"github.com/butler/butler/internal/memory/memorytest"
	"github.com/butler/butler/internal/ownership"
	"github.com/jackc/pgx/v5/pgxpool"
)

type modelRetryStoreFixture struct {
	pool       *pgxpool.Pool
	store      *Store
	controller *budget.Controller
	ctx        context.Context
	claim      Record
	taskID, op string
}

func newModelRetryStoreFixture(t *testing.T) *modelRetryStoreFixture {
	t.Helper()
	pool := memorytest.Database(t)
	ctx := context.Background()
	if _, err := pool.Exec(ctx, `INSERT INTO sessions(session_key,user_id,channel) VALUES('retry-session','owner','web')`); err != nil {
		t.Fatal(err)
	}
	job, err := task.NewStore(pool).Create(ctx, task.CreateRequest{SessionKey: "retry-session", OwnerID: "owner", Goal: "one retry", CompletionCriteria: []string{"known result"}})
	if err != nil {
		t.Fatal(err)
	}
	store := NewStore(pool)
	event := ingress.InputEvent{EventID: "retry-input", IdempotencyKey: "retry-input", SessionKey: "retry-session", Source: "web", EventType: runv1.InputEventType_INPUT_EVENT_TYPE_USER_MESSAGE, PayloadJSON: `{"text":"one retry","user_id":"owner"}`, CreatedAt: time.Now().UTC()}
	if _, _, err = store.Accept(ctx, event, "owner", job.TaskID); err != nil {
		t.Fatal(err)
	}
	claim, err := store.Claim(ctx, time.Minute)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = pool.Exec(ctx, `INSERT INTO runs(run_id,session_key,task_id,task_revision,input_event_id,status,autonomy_mode,current_state,model_provider,lease_id,started_at,updated_at) VALUES('retry-run','retry-session',$1,1,'retry-input','model_running','mode_1','model_running','fixture','retry-lease',clock_timestamp(),clock_timestamp())`, job.TaskID); err != nil {
		t.Fatal(err)
	}
	if _, err = pool.Exec(ctx, `INSERT INTO session_ownership(session_key,run_id,lease_id,fence_epoch,expires_at) VALUES('retry-session','retry-run','retry-lease',1,clock_timestamp()+interval '1 hour')`); err != nil {
		t.Fatal(err)
	}
	ctx = ownership.WithToken(WithClaim(ctx, claim), ownership.NewAuthority(pool), ownership.Token{SessionKey: "retry-session", RunID: "retry-run", LeaseID: "retry-lease", FenceEpoch: 1})
	policy := budget.DefaultPolicy()
	policy.Models["fixture:fixture"] = budget.ModelPrice{Basis: "local_no_billing"}
	policy.OutputTokens = 8
	controller := budget.NewController(pool, policy)
	a, err := controller.AdmitModel(ctx, "retry-run", "", "fixture", "fixture", 64)
	if err != nil {
		t.Fatal(err)
	}
	if err = store.SaveCheckpoint(ctx, "retry-run", "model_budget_reserved", `{"operation_id":"`+a.OperationID+`","deadline_origin":"model_turn"}`); err != nil {
		t.Fatal(err)
	}
	return &modelRetryStoreFixture{pool: pool, store: store, controller: controller, ctx: ctx, claim: claim, taskID: job.TaskID, op: a.OperationID}
}

func TestModelRetryDurableCountBackoffAndStartIntegration(t *testing.T) {
	f := newModelRetryStoreFixture(t)
	var admitted atomic.Int32
	var workers sync.WaitGroup
	for i := 0; i < 8; i++ {
		workers.Add(1)
		go func() {
			defer workers.Done()
			if f.store.ClaimModelRetry(f.ctx, "retry-run", f.op) == nil {
				admitted.Add(1)
			}
		}()
	}
	workers.Wait()
	if admitted.Load() != 1 {
		t.Fatal("durable retry count raced or refunded", admitted.Load())
	}
	var delay bool
	if f.pool.QueryRow(context.Background(), `SELECT not_before>clock_timestamp() AND (checkpoint_json->'model_retry'->>'not_before')::timestamptz>clock_timestamp() FROM accepted_inputs WHERE input_id=$1`, f.claim.InputID).Scan(&delay) != nil || !delay {
		t.Fatal("retry did not persist asynchronous backoff")
	}
	if f.store.ValidateModelRetry(f.ctx, "retry-run", f.op) == nil {
		t.Fatal("retry ignored DB not_before")
	}
	if err := f.store.Finish(context.Background(), f.claim, "queued", "model_retry_pending", "one bounded retry", 0); err != nil {
		t.Fatal(err)
	}
	if _, err := f.store.Claim(context.Background(), time.Minute); !errors.Is(err, ErrNotFound) {
		t.Fatal("worker claimed backoff early")
	}
	time.Sleep(2100 * time.Millisecond)
	claim, err := NewStore(f.pool).Claim(context.Background(), time.Minute)
	if err != nil || claim.Attempts != 2 {
		t.Fatal("restart lost receipt/backoff/attempt bound", err)
	}
	ctx := ownership.WithToken(WithClaim(context.Background(), claim), ownership.NewAuthority(f.pool), ownership.Token{SessionKey: "retry-session", RunID: "retry-run", LeaseID: "retry-lease", FenceEpoch: 1})
	if err = NewStore(f.pool).ValidateModelRetry(ctx, "retry-run", f.op); err != nil {
		t.Fatal("matching resumed private claim denied", err)
	}
	a, err := f.controller.AdmitModel(ctx, "retry-run", "", "fixture", "fixture", 64)
	if err != nil {
		t.Fatal(err)
	}
	if err = f.store.MarkModelRetryStarted(ctx, "retry-run", a.OperationID); err != nil {
		t.Fatal("fresh admission did not consume claimed retry", err)
	}
	if err = f.store.SaveCheckpoint(ctx, "retry-run", "prepared", `{"replacement":true}`); err != nil {
		t.Fatal(err)
	}
	var marker string
	var n, unknown int
	if f.pool.QueryRow(context.Background(), `SELECT checkpoint_json->'model_retry'->>'state' FROM accepted_inputs WHERE input_id=$1`, claim.InputID).Scan(&marker) != nil || marker != "started" || f.pool.QueryRow(context.Background(), `SELECT count(*),count(*) FILTER(WHERE actual_input_tokens IS NULL AND actual_output_tokens IS NULL) FROM execution_model_attempts WHERE run_id='retry-run'`).Scan(&n, &unknown) != nil || n != 2 || unknown != 2 {
		t.Fatal("retry counter or two spent NULL observations changed")
	}
	if f.store.ClaimModelRetry(ctx, "retry-run", a.OperationID) == nil || f.store.ValidateModelRetry(ctx, "retry-run", f.op) == nil {
		t.Fatal("started retry reopened or count reset after prepared replacement")
	}
}

func TestModelRetryClaimDenialMatrixIntegration(t *testing.T) {
	for _, name := range []string{"wrong_operation", "scope_deadline", "stopped_task", "stale_revision", "terminal_run", "input_claim_lost", "input_expired", "unfinished_tool", "unknown_tool"} {
		t.Run(name, func(t *testing.T) {
			f := newModelRetryStoreFixture(t)
			op := f.op
			var query string
			switch name {
			case "wrong_operation":
				op = "operation-another"
			case "scope_deadline":
				query = `UPDATE accepted_inputs SET checkpoint_json=jsonb_set(checkpoint_json,'{checkpoint_data,deadline_origin}','"scope_or_authority"')`
			case "stopped_task":
				query = `UPDATE tasks SET status='cancelled'`
			case "stale_revision":
				query = `UPDATE tasks SET revision=revision+1`
			case "terminal_run":
				query = `UPDATE runs SET current_state='failed'`
			case "input_claim_lost":
				query = `UPDATE accepted_inputs SET claim_expires_at=clock_timestamp()-interval '1 second'`
			case "input_expired":
				query = `UPDATE accepted_inputs SET expires_at=clock_timestamp()-interval '1 second'`
			case "unfinished_tool":
				query = `INSERT INTO tool_calls(tool_call_id,run_id,tool_name,args,status,runtime_target,started_at) VALUES('unfinished','retry-run','http.request','{}','executing','fixture',clock_timestamp())`
			case "unknown_tool":
				query = `INSERT INTO tool_calls(tool_call_id,run_id,tool_name,args,status,runtime_target,started_at,finished_at,result,error) VALUES('unknown','retry-run','http.request','{}','effect_unknown','fixture',clock_timestamp(),clock_timestamp(),'{}','{}')`
			}
			if query != "" {
				if _, err := f.pool.Exec(context.Background(), query); err != nil {
					t.Fatal(err)
				}
			}
			if f.store.ClaimModelRetry(f.ctx, "retry-run", op) == nil {
				t.Fatal("unsafe state admitted model retry")
			}
			var raw string
			if f.pool.QueryRow(context.Background(), `SELECT checkpoint_json::text FROM accepted_inputs WHERE input_id=$1`, f.claim.InputID).Scan(&raw) != nil {
				t.Fatal("receipt lost")
			}
			var body map[string]json.RawMessage
			_ = json.Unmarshal([]byte(raw), &body)
			if _, ok := body["model_retry"]; ok {
				t.Fatal("denied state persisted a retry claim")
			}
		})
	}
}

func TestAuthorityRetryStoreSharesCountAndRejectsUnsafeStateIntegration(t *testing.T) {
	for _, name := range []string{"shared_count", "expired_deadline", "after_acceptance_deadline", "superseded_lease", "claim_exhausted", "unknown_effect", "nominal_count_spent"} {
		t.Run(name, func(t *testing.T) {
			f := newModelRetryStoreFixture(t)
			deadline := time.Now().Add(30 * time.Second)
			var query string
			switch name {
			case "expired_deadline":
				deadline = time.Now().Add(-time.Second)
			case "after_acceptance_deadline":
				deadline = f.claim.ExpiresAt.Add(time.Hour)
			case "superseded_lease":
				query = `UPDATE session_ownership SET fence_epoch=fence_epoch+1`
			case "claim_exhausted":
				query = `UPDATE accepted_inputs SET attempts=max_attempts`
			case "unknown_effect":
				query = `INSERT INTO tool_calls(tool_call_id,run_id,tool_name,args,status,runtime_target,started_at,finished_at,result,error) VALUES('uncertain','retry-run','http.request','{}','effect_unknown','fixture',clock_timestamp(),clock_timestamp(),'{}','{}')`
			case "nominal_count_spent":
				if err := f.store.ClaimModelRetry(f.ctx, "retry-run", f.op); err != nil {
					t.Fatal(err)
				}
			}
			if query != "" {
				if _, err := f.pool.Exec(context.Background(), query); err != nil {
					t.Fatal(err)
				}
			}
			err := f.store.ClaimAuthorityRetry(f.ctx, "retry-run", f.op, deadline)
			if name != "shared_count" {
				if err == nil {
					t.Fatal("unsafe state admitted authority continuation")
				}
				return
			}
			if err != nil {
				t.Fatal("live private claim denied", err)
			}
			if f.store.ClaimModelRetry(f.ctx, "retry-run", f.op) == nil || f.store.ClaimAuthorityRetry(f.ctx, "retry-run", f.op, deadline) == nil {
				t.Fatal("nominal and authority retries received separate allowances")
			}
			var saved time.Time
			if f.pool.QueryRow(context.Background(), `SELECT (checkpoint_json->'model_retry'->>'execution_deadline')::timestamptz FROM accepted_inputs WHERE input_id=$1`, f.claim.InputID).Scan(&saved) != nil || !saved.Equal(deadline.UTC().Truncate(time.Microsecond)) {
				t.Fatal("original deadline not persisted")
			}
		})
	}
}
