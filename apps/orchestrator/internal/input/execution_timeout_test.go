package input

import (
	"context"
	"errors"
	"testing"
	"time"

	flow "github.com/butler/butler/apps/orchestrator/internal/orchestrator"
	"github.com/butler/butler/internal/ingress"
)

func TestExecutionTimeoutStartupBound(t *testing.T) {
	for _, limit := range []time.Duration{0, -time.Second, 40*time.Minute + time.Nanosecond} {
		if s, err := NewServiceWithExecutionTimeout(nil, nil, nil, nil, nil, limit); err == nil || s != nil {
			t.Fatal("invalid startup bound admitted", limit)
		}
	}
	s, err := NewServiceWithExecutionTimeout(nil, nil, nil, nil, nil, 40*time.Minute)
	if err != nil || s.executionTimeout != 40*time.Minute || NewService(nil, nil, nil, nil, nil).executionTimeout != 10*time.Minute {
		t.Fatal("explicit startup profile changed the ordinary default")
	}
}

type deadlineObservingExecutor struct {
	deadline time.Time
	err      error
}

func (e *deadlineObservingExecutor) Execute(ctx context.Context, _ ingress.InputEvent) (*flow.ExecutionResult, error) {
	e.deadline, _ = ctx.Deadline()
	<-ctx.Done()
	e.err = ctx.Err()
	return nil, e.err
}

func TestExecutionTimeoutCurrentWorkerIntegration(t *testing.T) {
	for _, parentBound := range []bool{false, true} {
		name := "service_bound"
		if parentBound {
			name = "parent_bound"
		}
		t.Run(name, func(t *testing.T) {
			f := newLifecycleFixture(t)
			claim := f.claim(t)
			executor := &deadlineObservingExecutor{}
			limit := 150 * time.Millisecond
			if parentBound {
				limit = 5 * time.Second
			}
			worker, err := NewServiceWithExecutionTimeout(f.store, f.sessions, executor, f.runs, nil, limit)
			if err != nil {
				t.Fatal(err)
			}
			parent := f.ctx
			var expected time.Time
			if parentBound {
				var cancel context.CancelFunc
				parent, cancel = context.WithTimeout(parent, 100*time.Millisecond)
				defer cancel()
				expected, _ = parent.Deadline()
			}
			worker.process(parent, claim)
			if !errors.Is(executor.err, context.DeadlineExceeded) || executor.deadline.IsZero() {
				t.Fatal("current claimed execution lost its finite deadline")
			}
			if parentBound {
				if !executor.deadline.Equal(expected) {
					t.Fatal("startup profile extended the caller deadline")
				}
			} else {
				saved, err := f.store.Get(f.ctx, claim.InputID)
				if err != nil || saved.Status != "blocked" || saved.ErrorCode != "execution_deadline_exceeded" {
					t.Fatal("execution expiry lost its durable category")
				}
			}
		})
	}
}
