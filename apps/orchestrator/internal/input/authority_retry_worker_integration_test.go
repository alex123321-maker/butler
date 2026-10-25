package input

import (
	"context"
	"encoding/json"
	"sync/atomic"
	"testing"
	"time"

	"github.com/butler/butler/apps/orchestrator/internal/completion"
	flow "github.com/butler/butler/apps/orchestrator/internal/orchestrator"
	"github.com/butler/butler/apps/orchestrator/internal/session"
	"github.com/butler/butler/internal/budget"
	"github.com/butler/butler/internal/transport"
)

// Only the watchdog's observation stalls. PostgreSQL ownership, claims and
// business mutations remain real and available for the subsequent fresh proof.
type stalledObservationLease struct {
	*session.PostgresLeaseManager
	ready *atomic.Bool
}

func (l *stalledObservationLease) GetLease(ctx context.Context, id string) (session.LeaseRecord, error) {
	if l.ready.Swap(false) {
		<-ctx.Done()
		return session.LeaseRecord{}, ctx.Err()
	}
	return l.PostgresLeaseManager.GetLease(ctx, id)
}

type authorityRetryProvider struct {
	*modelRetryWorkerProvider
	ready *atomic.Bool
	fault bool
}

func (p *authorityRetryProvider) StartRun(ctx context.Context, r transport.StartRunRequest) (transport.EventStream, error) {
	stream, err := p.modelRetryWorkerProvider.StartRun(ctx, r)
	if p.fault && !p.tool && err == nil {
		p.ready.Store(true)
	}
	return stream, err
}

func (p *authorityRetryProvider) SubmitToolResult(ctx context.Context, r transport.SubmitToolResultRequest) (transport.EventStream, error) {
	if p.fault {
		transport.ObserveProviderStage(ctx, transport.ProviderStageHTTP)
		p.submits++
		p.ready.Store(true)
		return make(chan transport.TransportEvent), nil
	}
	return p.modelRetryWorkerProvider.SubmitToolResult(ctx, r)
}

func (f *modelRetryWorkerFixture) authorityWorker(fault bool) {
	ready := &atomic.Bool{}
	p := &authorityRetryProvider{modelRetryWorkerProvider: f.provider, ready: ready, fault: fault}
	f.provider.hold = fault && !f.provider.tool
	leases := &stalledObservationLease{PostgresLeaseManager: session.NewPostgresLeaseManager(f.pool), ready: ready}
	executor := flow.NewService(f.sessionRepo, leases, f.runs, f.transcript, p, flow.Config{Completion: completion.New(f.pool), ProviderName: "fixture", ModelName: "fixture", OwnerID: "retry-worker", LeaseTTL: 30, Tasks: f.tasks, Checkpoints: NewStore(f.pool), Tools: f.tools, ActionAuthorizer: f.tools, ToolCatalog: f.tools, MemorySources: f.sources, Budgets: budget.NewController(f.pool, f.policy)}, nil)
	f.worker = NewService(NewStore(f.pool), f.sessionRepo, executor, f.runs, nil)
}

func TestAuthorityRetryWorkerSameInputRecoveryIntegration(t *testing.T) {
	for _, name := range []string{"initial", "conversation", "after_confirmed_tool", "equivalent_write", "second_timeout", "source_revoked", "task_stopped", "budget_exhausted", "original_deadline"} {
		t.Run(name, func(t *testing.T) {
			tool := name == "after_confirmed_tool" || name == "equivalent_write"
			f := newModelRetryWorkerFixtureScope(t, tool, name == "equivalent_write", name == "conversation")
			if name == "budget_exhausted" {
				f.policy.Task.ModelTurns = 1
			}
			f.authorityWorker(true)
			if name == "original_deadline" {
				f.worker.executionTimeout = 3300 * time.Millisecond
			}
			started := time.Now()
			f.step(t)
			r := f.result(t)
			var saved struct {
				Retry modelRetryRecord `json:"model_retry"`
			}
			if json.Unmarshal([]byte(r.CheckpointJSON), &saved) != nil || r.Status != "queued" || r.Checkpoint != "model_retry_pending" || saved.Retry.Reason != "authority_observation_timeout" || saved.Retry.ExecutionDeadline.IsZero() || saved.Retry.Count != 1 {
				t.Fatal("watchdog timeout did not durably queue the same input", r.Status, r.Checkpoint, r.ErrorMessage)
			}
			if f.provider.active || f.provider.activeCancellations != 1 || time.Since(started) > 3*time.Second {
				t.Fatal("750ms observation deadline failed to cancel active inference promptly")
			}
			var released bool
			if f.pool.QueryRow(context.Background(), `SELECT lease_id='' FROM session_ownership WHERE session_key=$1`, r.SessionKey).Scan(&released) != nil || !released {
				t.Fatal("backoff retained session lease")
			}
			if name == "original_deadline" && saved.Retry.ExecutionDeadline.After(started.Add(3500*time.Millisecond)) {
				t.Fatal("retry extended original execution deadline")
			}
			if name == "source_revoked" {
				if _, err := f.pool.Exec(context.Background(), `UPDATE model_retry_test_current_source SET active=false`); err != nil {
					t.Fatal(err)
				}
			}
			if name == "task_stopped" {
				// Store CAS and Resume must independently reject the changed task.
				if _, err := f.pool.Exec(context.Background(), `UPDATE tasks SET status='cancelled',revision=revision+1 WHERE task_id=$1`, r.TaskID); err != nil {
					t.Fatal(err)
				}
			}
			time.Sleep(2100 * time.Millisecond)
			if name == "original_deadline" {
				time.Sleep(400 * time.Millisecond)
			}
			// Construct a new worker/lease manager/store; only durable state survives.
			f.authorityWorker(name == "second_timeout")
			f.step(t)
			next := f.result(t)
			want := "completed"
			if name != "initial" && name != "conversation" && name != "after_confirmed_tool" {
				want = "blocked"
			}
			if next.InputID != r.InputID || next.RunID != r.RunID || next.Attempts != 2 || next.Status != want {
				t.Fatal("recovery changed identity, reopened work or required manual ingress", next.Status, next.ErrorCode, next.ErrorMessage)
			}
			calls, unknown := 2, 1
			if tool {
				calls = 3
			}
			if name == "second_timeout" {
				unknown = 2
			}
			if name == "source_revoked" || name == "task_stopped" || name == "budget_exhausted" || name == "original_deadline" {
				calls = 1
			}
			var models, nulls int
			if f.pool.QueryRow(context.Background(), `SELECT count(*),count(*) FILTER(WHERE actual_input_tokens IS NULL AND actual_output_tokens IS NULL) FROM execution_model_attempts WHERE run_id=$1`, r.RunID).Scan(&models, &nulls) != nil || models != calls || nulls != unknown {
				t.Fatal("retry reset spent reservations or invented usage", models, nulls)
			}
			if calls > 1 && (len(f.provider.calls) != 2 || f.provider.calls[1].Context.ProviderSessionRef != nil) {
				t.Fatal("recovery reused provider context")
			}
			if tool && (f.tools.calls != 1 || f.tools.prepares != 1) {
				t.Fatal("confirmed external action prepared/dispatched twice")
			}
			if name == "second_timeout" && next.ErrorCode != "execution_authority_observation_timeout" {
				t.Fatal("exhausted retry lost finite timeout diagnosis", next.ErrorCode)
			}
			var finals int
			if f.pool.QueryRow(context.Background(), `SELECT count(*) FROM messages WHERE run_id=$1 AND role='assistant'`, r.RunID).Scan(&finals) != nil || finals != map[bool]int{true: 1, false: 0}[want == "completed"] {
				t.Fatal("retry duplicated or lost final result", finals)
			}
		})
	}
}
