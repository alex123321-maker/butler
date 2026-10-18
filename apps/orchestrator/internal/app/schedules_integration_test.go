package app

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/butler/butler/apps/orchestrator/internal/completion"
	durableinput "github.com/butler/butler/apps/orchestrator/internal/input"
	flow "github.com/butler/butler/apps/orchestrator/internal/orchestrator"
	runservice "github.com/butler/butler/apps/orchestrator/internal/run"
	"github.com/butler/butler/apps/orchestrator/internal/scheduled"
	"github.com/butler/butler/apps/orchestrator/internal/session"
	"github.com/butler/butler/internal/actionauth"
	"github.com/butler/butler/internal/budget"
	commonv1 "github.com/butler/butler/internal/gen/common/v1"
	"github.com/butler/butler/internal/memory/transcript"
	"github.com/butler/butler/internal/schedulecontrol"
	"github.com/butler/butler/internal/transport"
	"github.com/jackc/pgx/v5/pgxpool"
)

func scheduleFixtureSpec(mode string) schedulecontrol.Spec {
	return schedulecontrol.Spec{Title: "Check tomorrow", Mode: mode, Instruction: "Проверь состояние проекта", CompletionCriteria: []string{"Saved source evidence"}, Timing: schedulecontrol.Timing{Kind: "once", TimeZone: "Asia/Tomsk", AfterSeconds: 60, MissedPolicy: "coalesce", GraceSeconds: 60}, ExpiresAt: time.Now().Add(10 * time.Minute).UTC().Truncate(time.Microsecond), MaxOccurrences: 1}
}

// This is an ordinary Core/Input execution with a scripted zero-network model.
// It proves integration and result delivery, not live model planning quality.
type scheduleFinalProvider struct {
	*initiativeProjectScriptedProvider
	pool   *pgxpool.Pool
	taskID string
}

func (p *scheduleFinalProvider) StartRun(ctx context.Context, r transport.StartRunRequest) (transport.EventStream, error) {
	var id string
	if p.pool.QueryRow(ctx, `SELECT task_id FROM runs WHERE run_id=$1`, r.Context.RunID).Scan(&id) != nil || id != p.taskID {
		return nil, errors.New("scheduled model entered foreign task")
	}
	p.entries.Add(1)
	return initiativeProjectStream(transport.NewAssistantFinalEvent(r.Context.RunID, p.Name(), transport.AssistantFinal{Content: "Scheduled project check prepared", FinishReason: "stop", Usage: &transport.Usage{InputTokens: 10, OutputTokens: 5, TotalTokens: 15}}), transport.NewTerminalEvent(r.Context.RunID, transport.EventTypeRunCompleted, p.Name())), nil
}

func TestScheduleBrokerProcessAndOrdinaryExecutionIntegration(t *testing.T) {
	if os.Getenv("BUTLER_SCHEDULES_APP_BINARY") == "" {
		t.Skip("compiled scheduling Broker required")
	}
	for _, lost := range []bool{false, true} {
		t.Run(map[bool]string{false: "ordinary_run", true: "lost_ack"}[lost], func(t *testing.T) {
			f := newNamedToolControlDispatchFixture(t, nil, false, false, "schedule.create", true)
			inputs := durableinput.NewStore(f.f.pool)
			checkpoint, _ := json.Marshal(map[string]string{"tool_call_id": f.call.ToolCallId, "action_id": f.prepared.Binding.ActionID, "effect_id": f.prepared.EffectID, "authorization_id": f.grant.ID})
			if err := inputs.SaveCheckpoint(f.owned, f.call.RunId, "tool_dispatching", string(checkpoint)); err != nil {
				t.Fatal(err)
			}
			f.f.budgets.Policy.Models["fixture:fixture"] = budget.ModelPrice{Basis: "subscription_no_overage"}
			store := &scheduled.Store{Pool: f.f.pool, Tasks: f.f.store, Inputs: inputs, Budgets: f.f.budgets, Allowance: budget.ModelAllowance{Provider: "fixture", Model: "fixture", InputTokens: 1024, OutputTokens: 8192}, SourceGuard: f.f.sources.CheckRunSourcesTx}
			const token = "guarded-request-private-process-fixture-token"
			h, err := schedulecontrol.NewHandler(token, &ScheduleControl{Store: store})
			if err != nil {
				t.Fatal(err)
			}
			var hits atomic.Int32
			actual := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				hits.Add(1)
				if !lost {
					h.ServeHTTP(w, r)
					return
				}
				record := httptest.NewRecorder()
				h.ServeHTTP(record, r)
				if record.Code != 200 {
					w.WriteHeader(record.Code)
					return
				}
				conn, _, e := w.(http.Hijacker).Hijack()
				if e == nil {
					_ = conn.Close()
				}
			})
			client := controlBrokerProcess(t, f, actual, schedulecontrol.Path, "BUTLER_SCHEDULES_HARNESS=1")
			catalog, err := client.ListToolsForRun(f.f.ctx, f.call.RunId)
			if err != nil || len(catalog) != 5 {
				t.Fatal("schedule contracts unavailable to agent", len(catalog), err)
			}
			result, err := client.ExecuteAuthorizedToolCall(f.f.ctx, f.call, f.grant.ID)
			if err != nil {
				t.Fatal(err)
			}
			var scheduleID string
			if err = f.f.pool.QueryRow(f.f.ctx, `SELECT schedule_id FROM scheduled_jobs`).Scan(&scheduleID); err != nil {
				t.Fatal("actual schedule not persisted", err)
			}
			if lost {
				if result.GetEffectState() != string(actionauth.StateUnknown) || hits.Load() != 1 {
					t.Fatal("unknown scheduling ACK was confirmed", result.GetEffectState(), hits.Load())
				}
				_, _ = client.ExecuteAuthorizedToolCall(f.f.ctx, f.call, f.grant.ID)
				if hits.Load() != 1 {
					t.Fatal("uncertain create replayed")
				}
				return
			}
			if result.GetStatus() != "completed" || result.GetEffectState() != string(actionauth.StateConfirmed) {
				t.Fatal("actual Broker scheduling failed", result.GetStatus(), result.GetEffectState(), result.GetError())
			}
			if _, err = f.f.pool.Exec(f.f.ctx, `UPDATE accepted_inputs SET status='completed',claim_id='',claim_expires_at=NULL WHERE input_id=$1;`, f.inputClaim.InputID); err != nil {
				t.Fatal(err)
			}
			if _, err = f.f.pool.Exec(f.f.ctx, `UPDATE runs SET current_state='completed',status='completed' WHERE run_id=$1`, f.call.RunId); err != nil {
				t.Fatal(err)
			}
			if _, err = f.f.leases.ReleaseLease(f.f.ctx, f.grant.LeaseID); err != nil {
				t.Fatal(err)
			}
			past := time.Now().Add(-time.Second).UTC().Truncate(time.Microsecond)
			if _, err = f.f.pool.Exec(f.f.ctx, `UPDATE scheduled_jobs SET next_at=$2,spec_json=jsonb_set(spec_json,'{timing,at}',to_jsonb($2::timestamptz)) WHERE schedule_id=$1`, scheduleID, past); err != nil {
				t.Fatal(err)
			}
			if err = store.Sweep(f.f.ctx); err != nil {
				t.Fatal(err)
			}
			var taskID, inputID string
			if err = f.f.pool.QueryRow(f.f.ctx, `SELECT task_id,input_id FROM scheduled_occurrences WHERE schedule_id=$1`, scheduleID).Scan(&taskID, &inputID); err != nil {
				t.Fatal(err)
			}
			sessions := session.NewPostgresRepository(f.f.pool)
			runs := runservice.NewService(runservice.NewPostgresRepository(f.f.pool), nil)
			provider := &scheduleFinalProvider{initiativeProjectScriptedProvider: &initiativeProjectScriptedProvider{}, pool: f.f.pool, taskID: taskID}
			results := completion.New(f.f.pool)
			results.DataGuard = f.f.sources.CheckRunSourcesTx
			executor := flow.NewService(sessions, f.f.leases, runs, transcript.NewStore(f.f.pool), provider, flow.Config{ProviderName: "fixture", ModelName: "fixture", OwnerID: "schedule-worker", LeaseTTL: 60, AutonomyMode: commonv1.AutonomyMode_AUTONOMY_MODE_1, Tasks: f.f.store, Checkpoints: inputs, Completion: results, Budgets: f.f.budgets, MemorySources: f.f.sources}, nil)
			worker := durableinput.NewService(inputs, sessions, executor, runs, nil)
			live, stop := context.WithCancel(f.f.ctx)
			done := make(chan error, 1)
			go func() { done <- worker.Run(live) }()
			defer func() { stop(); <-done }()
			deadline := time.Now().Add(15 * time.Second)
			for time.Now().Before(deadline) {
				record, e := inputs.Get(f.f.ctx, inputID)
				if e != nil {
					t.Fatal(e)
				}
				if record.Status == "completed" {
					final, e := results.GetByRun(f.f.ctx, record.RunID)
					if e != nil || final.AnswerState != "final" || final.Verdict != "unverified" {
						t.Fatal("scheduled result falsely verified or missing", e)
					}
					notices, e := results.Outbox.ListByRun(f.f.ctx, record.RunID)
					if e != nil || len(notices) != 1 || notices[0].Status != "result_ready" || provider.entries.Load() != 1 {
						t.Fatal("ordinary final/delivery", e, notices, provider.entries.Load())
					}
					return
				}
				if record.Status == "failed" || record.Status == "blocked" || record.Status == "expired" {
					t.Fatal("scheduled execution stopped", record.Status, record.ErrorCode)
				}
				time.Sleep(20 * time.Millisecond)
			}
			t.Fatal("ordinary scheduled input did not complete")
		})
	}
}
func TestScheduleActualDispatchedControlIntegration(t *testing.T) {
	for _, scenario := range []string{"created", "conversation", "replayed_receipt", "unclaimed", "wrong_effect", "wrong_args", "wrong_hash", "lost_input_claim", "wrong_input_slot", "expired_lease", "stale_task", "owner_forgery"} {
		t.Run(scenario, func(t *testing.T) {
			f := newNamedToolControlDispatchFixture(t, nil, false, false, "schedule.create", scenario == "conversation")
			inputs := durableinput.NewStore(f.f.pool)
			checkpoint, _ := json.Marshal(map[string]string{"tool_call_id": f.call.ToolCallId, "action_id": f.prepared.Binding.ActionID, "effect_id": f.prepared.EffectID, "authorization_id": f.grant.ID})
			if err := inputs.SaveCheckpoint(f.owned, f.call.RunId, "tool_dispatching", string(checkpoint)); err != nil {
				t.Fatal(err)
			}
			store := &scheduled.Store{Pool: f.f.pool, Tasks: f.f.store, Inputs: inputs, Budgets: f.f.budgets, Allowance: budget.ModelAllowance{Provider: "test", Model: "test", InputTokens: 1024, OutputTokens: 8192}, SourceGuard: f.f.sources.CheckRunSourcesTx}
			control := &ScheduleControl{Store: store}
			if err := store.CheckReady(f.f.ctx); err != nil {
				t.Fatal(err)
			}
			e := schedulecontrol.Envelope{RunID: f.envelope.RunID, ToolCallID: f.envelope.ToolCallID, EffectID: f.envelope.EffectID, ArgsJSON: f.envelope.ArgsJSON, ContractHash: f.envelope.ContractHash, Tool: f.call.ToolName}
			if scenario != "unclaimed" {
				f.claim(t)
			}
			var err error
			switch scenario {
			case "wrong_effect":
				e.EffectID = "wrong"
			case "wrong_args":
				e.ArgsJSON = strings.Replace(e.ArgsJSON, "Проверь состояние проекта", "Другое поручение", 1)
			case "wrong_hash":
				e.ContractHash = strings.Repeat("0", 64)
			case "lost_input_claim":
				_, err = f.f.pool.Exec(f.f.ctx, `UPDATE accepted_inputs SET claim_id='other-worker'`)
			case "wrong_input_slot":
				_, err = f.f.pool.Exec(f.f.ctx, `UPDATE accepted_inputs SET checkpoint_json=jsonb_set(checkpoint_json,'{checkpoint_data,effect_id}','"other"')`)
			case "expired_lease":
				_, err = f.f.pool.Exec(f.f.ctx, `UPDATE session_ownership SET expires_at=clock_timestamp()-interval '1 second'`)
			case "stale_task":
				_, err = f.f.pool.Exec(f.f.ctx, `UPDATE tasks SET constraints_revision=constraints_revision+1 WHERE task_id=$1`, f.f.parent.TaskID)
			}
			if err != nil {
				t.Fatal(err)
			}
			var receipt schedulecontrol.Receipt
			if scenario == "owner_forgery" {
				receipt, err = control.ExecuteDispatched(f.f.owner, e)
			} else {
				const token = "schedule-control-integration-private-token"
				h, hErr := schedulecontrol.NewHandler(token, control)
				if hErr != nil {
					t.Fatal(hErr)
				}
				server := httptest.NewServer(h)
				defer server.Close()
				client, cErr := schedulecontrol.NewClient(server.URL+schedulecontrol.Path, token)
				if cErr != nil {
					t.Fatal(cErr)
				}
				receipt, err = client.ExecuteDispatched(f.f.ctx, e)
				if scenario == "replayed_receipt" && err == nil {
					again, e2 := client.ExecuteDispatched(f.f.ctx, e)
					if e2 != nil || again.Schedule.ID != receipt.Schedule.ID {
						t.Fatal("receipt replay duplicated schedule", e2)
					}
				}
			}
			var n int
			if queryErr := f.f.pool.QueryRow(f.f.ctx, `SELECT count(*) FROM scheduled_jobs`).Scan(&n); queryErr != nil {
				t.Fatal(queryErr)
			}
			if scenario == "created" || scenario == "conversation" || scenario == "replayed_receipt" {
				if err != nil || receipt.Schedule == nil || n != 1 {
					t.Fatal("exact schedule publication", n, err)
				}
			} else if err == nil || n != 0 {
				t.Fatal("forged/stale schedule admitted", scenario, n, err)
			}
		})
	}
}
