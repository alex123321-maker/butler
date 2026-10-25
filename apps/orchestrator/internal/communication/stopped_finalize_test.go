package communication_test

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/butler/butler/apps/orchestrator/internal/communication"
	"github.com/butler/butler/apps/orchestrator/internal/input"
	flow "github.com/butler/butler/apps/orchestrator/internal/orchestrator"
	"github.com/butler/butler/apps/orchestrator/internal/session"
	"github.com/butler/butler/internal/actionauth"
	"github.com/butler/butler/internal/budget"
	"github.com/butler/butler/internal/communicationcontrol"
	runv1 "github.com/butler/butler/internal/gen/run/v1"
	"github.com/butler/butler/internal/ingress"
	"github.com/butler/butler/internal/owneridentity"
	"github.com/butler/butler/internal/ownership"
)

func stoppedKernelFixture(t *testing.T, task, uncertain bool) *fixture {
	t.Helper()
	f, _ := stopResumeFixture(t, fixtureOptions{Task: task}, false, false)
	f.s.Inputs.StoppedFinalizer = f.s
	if _, err := f.pool.Exec(context.Background(), `WITH reserved AS(INSERT INTO execution_budget_reservations(reservation_id,owner_id,operation_id,fingerprint,request_json) VALUES('retained-stop-budget',$2,'retained-stop-operation','f','{}') RETURNING reservation_id) INSERT INTO execution_model_attempts(operation_id,run_id,reservation_id,provider,model,input_allowance,output_allowance) SELECT 'retained-stop-operation',$1,reservation_id,'fixture','fixture',32,8 FROM reserved`, f.p.RunID, f.p.OwnerID); err != nil {
		t.Fatal(err)
	}
	// The existing private canonical admission qualifies historic transport
	// epochs too. No public payload or mutation upgrades the old ingress.
	if _, err := f.pool.Exec(context.Background(), `UPDATE accepted_inputs SET updated_at=clock_timestamp()-interval '10 seconds' WHERE input_id=$1`, f.claim.InputID); err != nil {
		t.Fatal(err)
	}
	if uncertain {
		if _, err := f.pool.Exec(context.Background(), `INSERT INTO action_effects(action_id,fingerprint,initial_binding,policy,state) VALUES('old-uncertain','f',jsonb_build_object('run_id',$1::text,'owner_id',$2::text,'task_id',$3::text,'tool_call_id','old-call','tool_name','http.request','args_hash','old-args','effect','commit'),'{}','effect_unknown')`, f.p.RunID, f.p.OwnerID, f.p.TaskID); err != nil {
			t.Fatal(err)
		}
	}
	return f
}

func TestStoppedAttemptFreshOwnerReplyRequiresExactCurrentProofIntegration(t *testing.T) {
	for _, mode := range []string{"valid", "wrong_intent", "expired_claim", "exhausted_admission", "untracked_unknown", "missing_provenance", "duplicate_reply"} {
		t.Run(mode, func(t *testing.T) {
			f := stoppedKernelFixture(t, false, true)
			next := queueStoppedTelegram(t, f)
			ctx := context.Background()
			if err := f.s.Inputs.SweepStopped(ctx); err != nil {
				t.Fatal(err)
			}
			claim, err := f.s.Inputs.Claim(ctx, time.Minute)
			if err != nil || claim.InputID != next.InputID {
				t.Fatal(err)
			}
			if _, err = f.pool.Exec(ctx, `INSERT INTO runs(run_id,session_key,input_event_id,status,autonomy_mode,current_state,model_provider,started_at,updated_at) VALUES('reply-run',$1,'new-telegram-question','model_running','mode_2','model_running','fixture',clock_timestamp(),clock_timestamp())`, f.p.SessionKey); err != nil {
				t.Fatal(err)
			}
			lease, err := f.leases.AcquireLease(ctx, session.AcquireLeaseParams{SessionKey: f.p.SessionKey, RunID: "reply-run", OwnerID: "worker", TTL: time.Minute})
			if err != nil {
				t.Fatal(err)
			}
			if _, err = f.pool.Exec(ctx, `UPDATE runs SET lease_id=$1 WHERE run_id='reply-run'`, lease.LeaseID); err != nil {
				t.Fatal(err)
			}
			live := ownership.WithToken(input.WithClaim(ctx, claim), f.leases.Authority(), ownership.Token{SessionKey: lease.SessionKey, RunID: lease.RunID, LeaseID: lease.LeaseID, FenceEpoch: lease.FenceEpoch})
			if err = f.s.Inputs.SaveCheckpoint(live, "reply-run", "prepared", `{}`); err != nil {
				t.Fatal(err)
			}
			tx, err := f.pool.Begin(live)
			if err != nil {
				t.Fatal(err)
			}
			defer tx.Rollback(context.Background())
			if err = f.s.AdmitTx(live, tx, communication.Principal{OwnerID: f.p.OwnerID, SessionKey: f.p.SessionKey, RunID: "reply-run", InputID: next.InputID}, false, []string{"communication.message"}); err != nil {
				t.Fatal(err)
			}
			if err = tx.Commit(live); err != nil {
				t.Fatal(err)
			}
			args, _ := actionauth.ArgumentsHash(message)
			contract, _ := actionauth.ContractHash(communicationcontrol.Contract("communication.message"))
			binding, _ := json.Marshal(map[string]any{"owner_id": f.p.OwnerID, "tool_name": "communication.message", "resource": "tool:communication.message", "effect": "prepare", "args_hash": args, "contract_hash": contract, "credential_refs": []string{}, "communication_destinations_hash": strings.Repeat("a", 64)})
			if _, err = f.pool.Exec(ctx, `INSERT INTO action_effects(action_id,fingerprint,initial_binding,policy,state) VALUES('reply-effect','f',$1,'{}','prepared')`, binding); err != nil {
				t.Fatal(err)
			}
			if _, err = f.pool.Exec(ctx, `INSERT INTO action_intents(action_id,run_id,tool_call_id,action_hash,binding,requires_approval,expires_at,effect_id,effect_fingerprint) VALUES('reply-intent','reply-run','reply-call','f',$1,false,clock_timestamp()+interval '1 minute','reply-effect','f')`, binding); err != nil {
				t.Fatal(err)
			}
			switch mode {
			case "missing_provenance":
				_, err = f.pool.Exec(ctx, `INSERT INTO action_effects(action_id,fingerprint,initial_binding,policy,state) VALUES('untracked-effect','f',jsonb_build_object('run_id','missing-run','owner_id',$1::text,'effect','commit'),'{}','effect_unknown')`, f.p.OwnerID)
			case "expired_claim":
				_, err = f.pool.Exec(ctx, `UPDATE accepted_inputs SET claim_expires_at=clock_timestamp()-interval '1 second' WHERE input_id=$1`, next.InputID)
			case "exhausted_admission":
				_, err = f.pool.Exec(ctx, `UPDATE communication_admissions SET publication_count=20 WHERE input_id=$1`, next.InputID)
			case "untracked_unknown":
				_, err = f.pool.Exec(ctx, `WITH old AS(INSERT INTO runs(run_id,session_key,input_event_id,status,autonomy_mode,current_state,model_provider,started_at,updated_at) VALUES('untracked',$1,'old','failed','mode_2','failed','fixture',clock_timestamp(),clock_timestamp()) RETURNING run_id) INSERT INTO action_effects(action_id,fingerprint,initial_binding,policy,state) SELECT 'untracked-effect','f',jsonb_build_object('run_id',run_id,'owner_id',$2::text,'task_id','','effect','commit'),'{}','effect_unknown' FROM old`, f.p.SessionKey, f.p.OwnerID)
			case "duplicate_reply":
				_, err = f.pool.Exec(ctx, `UPDATE action_effects SET initial_binding=initial_binding||jsonb_build_object('tool_name','communication.message','args_hash',$1::text) WHERE action_id='old-uncertain'`, args)
			}
			if err != nil {
				t.Fatal(err)
			}
			i := budget.ToolIdentity{RunID: "reply-run", EffectID: "reply-effect", ToolName: "communication.message", ArgsHash: args, ContractHash: contract, Effect: "prepare", OwnerReply: true}
			if mode == "wrong_intent" {
				i.ArgsHash = "different"
			}
			tx, err = f.pool.Begin(ctx)
			if err != nil {
				t.Fatal(err)
			}
			defer tx.Rollback(ctx)
			err = budget.NewController(f.pool, budget.DefaultPolicy()).CheckToolTx(ctx, tx, i)
			if (err == nil) != (mode == "valid") {
				t.Fatal("fresh owner reply proof bypassed or rejected", mode, err)
			}
		})
	}
}

func queueStoppedTelegram(t *testing.T, f *fixture) input.Record {
	t.Helper()
	e := ingress.InputEvent{EventID: "new-telegram-question", IdempotencyKey: "new-telegram-question", SessionKey: f.p.SessionKey, Source: "telegram", EventType: runv1.InputEventType_INPUT_EVENT_TYPE_USER_MESSAGE, PayloadJSON: `{"text":"independent question"}`, CreatedAt: time.Now()}
	tx, err := f.pool.Begin(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Rollback(context.Background())
	r, _, err := f.s.Inputs.AcceptTx(context.Background(), tx, e, f.p.OwnerID, "")
	if err != nil {
		t.Fatal(err)
	}
	hash, _ := actionauth.ArgumentsHash(e.PayloadJSON)
	a := owneridentity.WithActor(context.Background(), owneridentity.Actor{OwnerID: f.p.OwnerID, Channel: "telegram", SessionKey: "telegram:verified", DeliveryID: e.EventID})
	if err = f.s.Inputs.MarkCommunicationTx(a, tx, r, input.CommunicationIngress{OwnerID: f.p.OwnerID, BindingID: f.bindings["telegram"], BindingVersion: 1, BotID: 77, DeliveryID: e.EventID, PayloadHash: hash, Kind: "telegram_message", ActorChannel: "telegram", ActorDeliveryID: e.EventID}); err != nil {
		t.Fatal(err)
	}
	if err = tx.Commit(context.Background()); err != nil {
		t.Fatal(err)
	}
	return r
}

func TestStoppedAttemptFinalizationReleasesQueueWithoutReplayingIntegration(t *testing.T) {
	for _, name := range []string{"conversation_known", "conversation_unknown", "already_failed_unknown", "task_unknown", "journal_allowed", "missing_receipt", "untracked_receipt", "live_lease", "live_claim", "pending_question", "grace", "completed_run", "journal_locked", "rollback"} {
		t.Run(name, func(t *testing.T) {
			uncertain := name == "conversation_unknown" || name == "task_unknown" || name == "already_failed_unknown"
			f := stoppedKernelFixture(t, name == "task_unknown", uncertain)
			next := queueStoppedTelegram(t, f)
			var query string
			switch name {
			case "already_failed_unknown":
				query = `UPDATE accepted_inputs SET status='failed' WHERE input_id=$1`
			case "untracked_receipt":
				uncertain = true
				if _, err := f.pool.Exec(context.Background(), `INSERT INTO tool_calls(tool_call_id,run_id,tool_name,args,status,runtime_target,started_at,finished_at,result,error) VALUES('legacy-call',$1,'http.request','{}','completed','fixture',clock_timestamp(),clock_timestamp(),'{}','{}')`, f.p.RunID); err != nil {
					t.Fatal(err)
				}
			case "journal_allowed":
				query = `UPDATE accepted_inputs SET checkpoint='tool_dispatching',checkpoint_json=jsonb_set(checkpoint_json,'{checkpoint_data}','{"tool_call_id":"old-call","effect_id":"old-uncertain"}') WHERE input_id=$1`
				if _, err := f.pool.Exec(context.Background(), `INSERT INTO action_effects(action_id,fingerprint,initial_binding,policy,state) VALUES('old-uncertain','f',jsonb_build_object('run_id',$1::text,'owner_id',$2::text,'tool_call_id','old-call','effect','prepare'),'{}','allowed')`, f.p.RunID, f.p.OwnerID); err != nil {
					t.Fatal(err)
				}
			case "missing_receipt":
				uncertain = true
				query = `UPDATE accepted_inputs SET checkpoint='tool_dispatching',checkpoint_json=jsonb_set(checkpoint_json,'{checkpoint_data}','{"tool_call_id":"lost-call","effect_id":"missing-effect"}') WHERE input_id=$1`
			case "live_lease":
				query = `UPDATE session_ownership SET lease_id='live',expires_at=clock_timestamp()+interval '1 minute' WHERE session_key=(SELECT session_key FROM accepted_inputs WHERE input_id=$1)`
			case "live_claim":
				query = `UPDATE accepted_inputs SET claim_id='live',claim_expires_at=clock_timestamp()+interval '1 minute' WHERE input_id=$1`
			case "pending_question":
				query = `UPDATE communication_interactions SET state='open',expires_at=NULL WHERE input_id=$1`
			case "grace":
				query = `UPDATE accepted_inputs SET updated_at=clock_timestamp() WHERE input_id=$1`
			case "completed_run":
				query = `UPDATE runs SET current_state='completed' WHERE run_id=(SELECT run_id FROM accepted_inputs WHERE input_id=$1)`
			case "rollback":
				query = `CREATE FUNCTION deny_stop() RETURNS trigger LANGUAGE plpgsql AS $$ BEGIN RAISE EXCEPTION 'test transaction failure'; END $$; CREATE TRIGGER deny_stop BEFORE UPDATE ON communication_admissions FOR EACH ROW EXECUTE FUNCTION deny_stop()`
			case "journal_locked":
				if _, err := f.pool.Exec(context.Background(), `INSERT INTO action_effects(action_id,fingerprint,initial_binding,policy,state) VALUES('locked','f',jsonb_build_object('run_id',$1::text,'owner_id',$2::text),'{}','allowed')`, f.p.RunID, f.p.OwnerID); err != nil {
					t.Fatal(err)
				}
			}
			if query != "" {
				var err error
				if name == "rollback" {
					_, err = f.pool.Exec(context.Background(), query)
				} else {
					_, err = f.pool.Exec(context.Background(), query, f.claim.InputID)
				}
				if err != nil {
					t.Fatal(err)
				}
			}
			var second input.Record
			if name == "rollback" {
				if _, err := f.pool.Exec(context.Background(), `INSERT INTO runs(run_id,session_key,input_event_id,status,autonomy_mode,current_state,model_provider,started_at,updated_at) VALUES('other-stopped-run',$1,'other-stopped','model_running','mode_2','model_running','fixture',clock_timestamp(),clock_timestamp())`, f.p.SessionKey); err != nil {
					t.Fatal(err)
				}
				var err error
				second, _, err = f.s.Inputs.Accept(context.Background(), ingress.InputEvent{EventID: "other-stopped", IdempotencyKey: "other-stopped", SessionKey: f.p.SessionKey, Source: "fixture", EventType: runv1.InputEventType_INPUT_EVENT_TYPE_USER_MESSAGE, PayloadJSON: `{"text":"independent stopped attempt"}`, CreatedAt: time.Now()}, f.p.OwnerID, "")
				if err != nil {
					t.Fatal(err)
				}
				if _, err = f.pool.Exec(context.Background(), `UPDATE accepted_inputs SET run_id='other-stopped-run',status='blocked',error_code='execution_stopped',communication_ingress_epoch=1,updated_at=clock_timestamp()-interval '9 seconds' WHERE input_id=$1`, second.InputID); err != nil {
					t.Fatal(err)
				}
			}
			if name == "journal_locked" {
				tx, err := f.pool.Begin(context.Background())
				if err != nil {
					t.Fatal(err)
				}
				defer tx.Rollback(context.Background())
				if _, err = tx.Exec(context.Background(), `SELECT action_id FROM action_effects WHERE action_id='locked' FOR UPDATE`); err != nil {
					t.Fatal(err)
				}
			}
			var before string
			if err := f.pool.QueryRow(context.Background(), `SELECT COALESCE(jsonb_agg(to_jsonb(e)),'[]')::text FROM action_effects e`).Scan(&before); err != nil {
				t.Fatal(err)
			}
			original, err := f.s.Inputs.Get(context.Background(), f.claim.InputID)
			if err != nil {
				t.Fatal(err)
			}
			err = f.s.Inputs.SweepStopped(context.Background())
			if (err != nil) != (name == "rollback") {
				t.Fatal("unexpected finalizer result", err)
			}
			r, err := f.s.Inputs.Get(context.Background(), f.claim.InputID)
			if err != nil {
				t.Fatal(err)
			}
			denied := name == "live_lease" || name == "live_claim" || name == "pending_question" || name == "grace" || name == "completed_run" || name == "journal_locked" || name == "rollback"
			if denied {
				if r.Status != "blocked" {
					t.Fatal("unsafe attempt finalized", r.Status)
				}
				if name == "rollback" {
					other, e := f.s.Inputs.Get(context.Background(), second.InputID)
					if e != nil || other.Status != "failed" {
						t.Fatal("one failed transaction blocked independent cleanup", e)
					}
					var partial int
					if e = f.pool.QueryRow(context.Background(), `SELECT count(*) FROM input_stopped_attempts WHERE input_id=$1`, r.InputID).Scan(&partial); e != nil || partial != 0 {
						t.Fatal("partial failure proof committed", e)
					}
				}
				return
			}
			if r.Status != "failed" || r.Checkpoint != original.Checkpoint || r.CheckpointJSON != original.CheckpointJSON || r.ErrorCode != original.ErrorCode || r.Attempts != original.Attempts {
				t.Fatal("failed attempt changed history", r)
			}
			var retained int
			if err = f.pool.QueryRow(context.Background(), `SELECT count(*) FROM execution_model_attempts WHERE operation_id='retained-stop-operation' AND actual_input_tokens IS NULL AND actual_output_tokens IS NULL`).Scan(&retained); err != nil || retained != 1 {
				t.Fatal("unknown usage was refunded or filled", err)
			}
			var class, after, runState string
			if err = f.pool.QueryRow(context.Background(), `SELECT classification FROM input_stopped_attempts WHERE input_id=$1`, r.InputID).Scan(&class); err != nil || (class == "uncertain") != uncertain {
				t.Fatal(class, err)
			}
			if err = f.pool.QueryRow(context.Background(), `SELECT COALESCE(jsonb_agg(to_jsonb(e)),'[]')::text FROM action_effects e`).Scan(&after); err != nil || after != before {
				t.Fatal("journal reconciled or execution restarted", err)
			}
			if err = f.pool.QueryRow(context.Background(), `SELECT current_state FROM runs WHERE run_id=$1`, r.RunID).Scan(&runState); err != nil || runState != "failed" {
				t.Fatal(runState, err)
			}
			if err = f.s.Inputs.SweepStopped(context.Background()); err != nil {
				t.Fatal(err)
			}
			var audits int
			if err = f.pool.QueryRow(context.Background(), `SELECT count(*) FROM communication_events WHERE aggregate_id=$1 AND event_type IN ('input.stopped_failed','input.stopped_uncertain')`, r.InputID).Scan(&audits); err != nil || audits != 1 {
				t.Fatal("duplicate audit", audits, err)
			}
			claim, err := f.s.Inputs.Claim(context.Background(), time.Minute)
			if err != nil || claim.InputID != next.InputID {
				t.Fatal("new message still blocked", claim.InputID, err)
			}
			// An arbitrary write from the newly admitted run is denied, including
			// when the uncertain attempt belonged to a different explicit Task.
			if _, err = f.pool.Exec(context.Background(), `INSERT INTO runs(run_id,session_key,input_event_id,status,autonomy_mode,current_state,model_provider,started_at,updated_at) VALUES('new-run',$1,'new-telegram-question','model_running','mode_2','model_running','fixture',clock_timestamp(),clock_timestamp())`, f.p.SessionKey); err != nil {
				t.Fatal(err)
			}
			tx, err := f.pool.Begin(context.Background())
			if err != nil {
				t.Fatal(err)
			}
			defer tx.Rollback(context.Background())
			b := budget.NewController(f.pool, budget.DefaultPolicy())
			err = b.CheckToolTx(context.Background(), tx, budget.ToolIdentity{RunID: "new-run", ToolName: "different.write", ArgsHash: "different", Effect: "commit"})
			var loop *budget.LoopError
			if uncertain && (!errors.As(err, &loop) || loop.Reason != "stopped_attempt_uncertain") {
				t.Fatal("new run evaded uncertainty", err)
			}
			if !uncertain && err != nil {
				t.Fatal("known failure blocked new work", err)
			}
		})
	}
}

type stoppedQueueExecutor struct {
	mu  sync.Mutex
	ids []string
}

func (e *stoppedQueueExecutor) Execute(_ context.Context, event ingress.InputEvent) (*flow.ExecutionResult, error) {
	e.mu.Lock()
	e.ids = append(e.ids, event.IdempotencyKey)
	e.mu.Unlock()
	return &flow.ExecutionResult{ReceiptStatus: "completed"}, nil
}

func TestStoppedAttemptWorkerRestartProcessesOriginalTelegramInputIntegration(t *testing.T) {
	f := stoppedKernelFixture(t, false, true)
	next := queueStoppedTelegram(t, f)
	store := input.NewStore(f.pool)
	store.CommunicationEpoch = 1
	store.StoppedFinalizer = f.s
	executor := &stoppedQueueExecutor{}
	worker := input.NewService(store, session.NewPostgresRepository(f.pool), executor, nil, nil)
	ctx, cancel := context.WithTimeout(context.Background(), 8*time.Second)
	defer cancel()
	done := make(chan error, 1)
	go func() { done <- worker.Run(ctx) }()
	for ctx.Err() == nil {
		r, err := store.Get(ctx, next.InputID)
		if err != nil {
			t.Fatal(err)
		}
		if r.Status == "completed" {
			break
		}
		time.Sleep(20 * time.Millisecond)
	}
	cancel()
	<-done
	executor.mu.Lock()
	defer executor.mu.Unlock()
	if len(executor.ids) != 1 || executor.ids[0] != next.InputID {
		raw, _ := json.Marshal(executor.ids)
		t.Fatal("worker required new ingress or replayed old work", string(raw))
	}
}

func TestStoppedAttemptConcurrentReviewAndSeparateReconciliationIntegration(t *testing.T) {
	f := stoppedKernelFixture(t, false, true)
	var workers sync.WaitGroup
	for n := 0; n < 8; n++ {
		workers.Add(1)
		go func() {
			defer workers.Done()
			if err := f.s.Inputs.SweepStopped(context.Background()); err != nil {
				t.Error(err)
			}
		}()
	}
	workers.Wait()
	var events int
	if err := f.pool.QueryRow(context.Background(), `SELECT count(*) FROM communication_events WHERE aggregate_id=$1 AND event_type='input.stopped_uncertain'`, f.claim.InputID).Scan(&events); err != nil || events != 1 {
		t.Fatal("concurrent review duplicated failure", events, err)
	}
	// Simulate a later owning-journal reconciliation; the finalizer itself has
	// no effect-write capability and cannot manufacture this evidence.
	if _, err := f.pool.Exec(context.Background(), `UPDATE action_effects SET state='effect_confirmed' WHERE action_id='old-uncertain'; UPDATE input_stopped_attempts SET checked_at=clock_timestamp()-interval '10 seconds'`); err != nil {
		t.Fatal(err)
	}
	if err := f.s.Inputs.SweepStopped(context.Background()); err != nil {
		t.Fatal(err)
	}
	var classification, state string
	if err := f.pool.QueryRow(context.Background(), `SELECT classification FROM input_stopped_attempts WHERE input_id=$1`, f.claim.InputID).Scan(&classification); err != nil || classification != "known" {
		t.Fatal("separate reconciliation did not release write isolation", classification, err)
	}
	if err := f.pool.QueryRow(context.Background(), `SELECT status FROM accepted_inputs WHERE input_id=$1`, f.claim.InputID).Scan(&state); err != nil || state != "failed" {
		t.Fatal("reconciliation reopened failed attempt", state, err)
	}
}
