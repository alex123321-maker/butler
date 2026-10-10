package input

import (
	"context"
	"strings"
	"testing"

	"github.com/butler/butler/apps/orchestrator/internal/completion"
	flow "github.com/butler/butler/apps/orchestrator/internal/orchestrator"
	"github.com/butler/butler/apps/orchestrator/internal/session"
	"github.com/butler/butler/internal/budget"
	"github.com/butler/butler/internal/transport"
)

type budgetFixtureProvider struct {
	lifecycleProvider
	idle   bool
	starts int
}

func (*budgetFixtureProvider) InferenceLimits() (int64, int64) { return 20, 8 }
func (p *budgetFixtureProvider) StartRun(ctx context.Context, r transport.StartRunRequest) (transport.EventStream, error) {
	p.starts++
	if p.idle {
		stream := make(chan transport.TransportEvent)
		go func() { <-ctx.Done(); close(stream) }()
		return stream, nil
	}
	return p.lifecycleProvider.StartRun(ctx, r)
}
func TestRuntimeBudgetStopRetainsContinuationWithoutProviderOrFinalResultIntegration(t *testing.T) {
	f := newLifecycleFixture(t)
	policy := budget.DefaultPolicy()
	policy.Task.ModelTurns = 0
	policy.Models["fixture:fixture"] = budget.ModelPrice{Basis: "local_no_billing"}
	provider := &budgetFixtureProvider{}
	executor := flow.NewService(f.sessions, session.NewPostgresLeaseManager(f.pool), f.runs, f.transcript, provider, flow.Config{Completion: completion.New(f.pool), ProviderName: "fixture", ModelName: "fixture", OwnerID: "worker", LeaseTTL: 30, Tasks: f.tasks, Checkpoints: f.store, Budgets: budget.NewController(f.pool, policy)}, nil)
	worker := NewService(f.store, f.sessions, executor, f.runs, nil)
	worker.process(f.ctx, f.claim(t))
	receipt := f.assertReceipt(t, "blocked")
	if receipt.ErrorCode != "budget_model_turns_task" || provider.starts != 0 || f.tools.calls != 0 {
		t.Fatal("budget stop invoked inference/effect", receipt.ErrorCode, provider.starts)
	}
	stage, raw, err := f.store.LoadCheckpoint(f.ctx, receipt.RunID)
	if err != nil || stage != "execution_stopped" || !strings.Contains(raw, "continuation_json") || !strings.Contains(raw, "model_started") {
		t.Fatal("stop lost continuation", stage, raw, err)
	}
	var completed bool
	if err = f.pool.QueryRow(f.ctx, `SELECT EXISTS(SELECT 1 FROM run_final_results WHERE run_id=$1)`, receipt.RunID).Scan(&completed); err != nil || completed {
		t.Fatal("exhaustion saved a fake final result", completed, err)
	}
}
func TestRuntimeBudgetDeadlineCancelsIdleInferenceIntegration(t *testing.T) {
	f := newLifecycleFixture(t)
	policy := budget.DefaultPolicy()
	policy.ModelMillis = 40
	policy.Models["fixture:fixture"] = budget.ModelPrice{Basis: "local_no_billing"}
	provider := &budgetFixtureProvider{idle: true}
	executor := flow.NewService(f.sessions, session.NewPostgresLeaseManager(f.pool), f.runs, f.transcript, provider, flow.Config{Completion: completion.New(f.pool), ProviderName: "fixture", ModelName: "fixture", OwnerID: "worker", LeaseTTL: 30, Tasks: f.tasks, Checkpoints: f.store, Budgets: budget.NewController(f.pool, policy)}, nil)
	worker := NewService(f.store, f.sessions, executor, f.runs, nil)
	worker.process(f.ctx, f.claim(t))
	receipt := f.assertReceipt(t, "blocked")
	if receipt.ErrorCode != "execution_budget_deadline" || provider.starts != 1 || provider.cancellations != 1 {
		t.Fatal("idle inference did not stop honestly", receipt.ErrorCode, provider.starts, provider.cancellations)
	}
}
