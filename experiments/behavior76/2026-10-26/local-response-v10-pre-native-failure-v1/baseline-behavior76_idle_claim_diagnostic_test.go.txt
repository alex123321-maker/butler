package app

import (
	"context"
	"errors"
	"os"
	"sync/atomic"
	"testing"
	"time"

	"github.com/butler/butler/apps/orchestrator/internal/input"
	flow "github.com/butler/butler/apps/orchestrator/internal/orchestrator"
	"github.com/butler/butler/apps/orchestrator/internal/run"
	"github.com/butler/butler/apps/orchestrator/internal/session"
	"github.com/butler/butler/internal/ingress"
)

type behavior76IdleNoWorkExecutor struct{ calls atomic.Int32 }

func (e *behavior76IdleNoWorkExecutor) Execute(context.Context, ingress.InputEvent) (*flow.ExecutionResult, error) {
	e.calls.Add(1)
	return nil, errors.New("public idle diagnostic unexpectedly executed work")
}

type behavior76IdleClaimDiagnosticResult struct {
	contextCancelled bool
	admissionClosed  bool
}

// Explicit opt-in diagnosis of the OLD harness shutdown schedule. PASS here
// means the bug was reproduced; it is excluded from acceptance/Public selectors.
// No queued Input, native provider, auth factory or hidden corpus is involved.
func TestBehavior76IdleClaimCancellationLegacyDiagnosticIntegration(t *testing.T) {
	if os.Getenv("BUTLER_BEHAVIOR76_IDLE_CLAIM_DIAGNOSTIC") != "reproduce-cancelled-idle-claim-v1" {
		t.Skip("explicit public old-harness diagnostic only")
	}
	if os.Getenv("BUTLER_BEHAVIOR76_NATIVE_ENABLE") != "disabled" {
		t.Fatal("native must remain disabled")
	}
	parent, cancel := context.WithTimeout(t.Context(), 20*time.Second)
	defer cancel()
	var global atomic.Bool
	f := newBehavior76ResponseFixture(t, parent, &global)
	if f.scope.check(parent) != nil || global.Load() {
		t.Fatal("ordinary live fixture reconciliation was not healthy before stop")
	}
	var queued int
	if f.pool.QueryRow(parent, `SELECT count(*) FROM accepted_inputs`).Scan(&queued) != nil || queued != 0 {
		t.Fatal("diagnostic requires an empty real input table")
	}
	entered := make(chan struct{}, 2)
	release := make(chan struct{})
	results := make(chan behavior76IdleClaimDiagnosticResult, 2)
	admit := func(ctx context.Context) (func(), error) {
		entered <- struct{}{}
		select {
		case <-release:
		case <-parent.Done():
			return nil, parent.Err()
		}
		// Deliberately retain the old schedule: an idle worker was stopped while
		// this callback waited. The real unchanged scope sees its cancelled ctx.
		cancelled := errors.Is(ctx.Err(), context.Canceled)
		unlock, err := f.scope.claimAdmission(ctx)
		results <- behavior76IdleClaimDiagnosticResult{contextCancelled: cancelled, admissionClosed: errors.Is(err, input.ErrClaimAdmissionClosed)}
		return unlock, err
	}
	executor := &behavior76IdleNoWorkExecutor{}
	worker, err := input.NewServiceWithClaimAdmission(input.NewStore(f.pool), session.NewPostgresRepository(f.pool), executor, run.NewService(run.NewPostgresRepository(f.pool), nil), nil, admit)
	if err != nil {
		t.Fatal("ordinary claim-fenced input service unavailable")
	}
	live, stop := context.WithCancel(parent)
	done := make(chan error, 1)
	go func() { done <- worker.Run(live) }()
	for range 2 {
		select {
		case <-entered:
		case <-parent.Done():
			stop()
			t.Fatal("both ordinary idle claim workers did not enter")
		}
	}
	if global.Load() {
		t.Fatal("shared global changed before ordinary worker stop")
	}
	stop() // Same normal context cancellation as old evaluate's deferred stop.
	close(release)
	select {
	case runErr := <-done:
		if runErr != nil {
			t.Fatal("unexpected non-nil worker lifecycle result")
		}
	case <-parent.Done():
		t.Fatal("ordinary idle workers were not joined")
	}
	for range 2 {
		select {
		case result := <-results:
			if !result.contextCancelled || !result.admissionClosed {
				t.Fatal("diagnostic did not reach cancelled scope admission")
			}
		default:
			t.Fatal("actual admission result was not captured")
		}
	}
	if !global.Load() || executor.calls.Load() != 0 {
		t.Fatal("old idle cancellation did not poison shared global without work")
	}
	leave, err := f.scope.enterEvaluation(parent)
	if leave != nil {
		leave()
	}
	if !errors.Is(err, errBehavior76Native) {
		t.Fatal("next live evaluation did not observe poisoned shared global")
	}
	if f.pool.QueryRow(parent, `SELECT count(*) FROM accepted_inputs`).Scan(&queued) != nil || queued != 0 {
		t.Fatal("idle diagnostic unexpectedly changed input population")
	}
	t.Log("CAUSAL_IDLE_REPRO live_reconciliation_before_stop=true callbacks=2 callback_context=context.Canceled callback_return=ErrClaimAdmissionClosed ordinary_worker_Run=nil sharedGLOBAL_after_stop=true next_enterEvaluation=errBehavior76Native queued_inputs=0 executor_calls=0 native_auth_calls=0 original_hidden_failure_subtype_still_unknown=true")
}
