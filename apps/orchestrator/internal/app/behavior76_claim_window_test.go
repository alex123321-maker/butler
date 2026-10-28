package app

import (
	"context"
	"errors"
	"sync/atomic"
	"testing"
	"time"

	"github.com/butler/butler/apps/orchestrator/internal/input"
	"github.com/butler/butler/apps/orchestrator/internal/run"
	"github.com/butler/butler/apps/orchestrator/internal/session"
)

// Ordinary successful shutdown, with both idle claim workers at the admission
// boundary. The separately archived baseline diagnoses the old cancellation
// schedule; this test neither injects a storage fault nor invokes a provider.
func TestBehavior76LocalResponseClaimWindowIntegration(t *testing.T) {
	ctx, cancel := context.WithTimeout(t.Context(), 45*time.Second)
	defer cancel()
	var global atomic.Bool
	f := newBehavior76ResponseFixture(t, ctx, &global)
	window := &behavior76ClaimWindow{scope: f.scope}
	entered := make(chan struct{}, 2)
	release := make(chan struct{})
	results := make(chan bool, 2)
	admit := func(live context.Context) (func(), error) {
		entered <- struct{}{}
		select {
		case <-release:
		case <-ctx.Done():
			return nil, ctx.Err()
		}
		unlock, err := window.admit(live)
		results <- live.Err() == nil && errors.Is(err, input.ErrClaimAdmissionClosed)
		return unlock, err
	}
	executor := &behavior76IdleNoWorkExecutor{}
	worker, err := input.NewServiceWithClaimAdmission(input.NewStore(f.pool), session.NewPostgresRepository(f.pool), executor, run.NewService(run.NewPostgresRepository(f.pool), nil), nil, admit)
	if err != nil {
		t.Fatal("ordinary fenced input service unavailable")
	}
	live, stop := context.WithCancel(ctx)
	defer stop()
	done := make(chan error, 1)
	go func() { done <- worker.Run(live) }()
	for range 2 {
		select {
		case <-entered:
		case <-ctx.Done():
			t.Fatal("ordinary idle claim workers did not reach admission")
		}
	}
	window.close()
	close(release)
	if err := worker.WaitClaimWorkers(ctx); err != nil || live.Err() != nil {
		t.Fatal("actual claim workers did not join under the original live context")
	}
	stop()
	select {
	case err := <-done:
		if err != nil {
			t.Fatal("ordinary input Run did not stop normally")
		}
	case <-ctx.Done():
		t.Fatal("ordinary input Run did not join")
	}
	for range 2 {
		if !<-results {
			t.Fatal("closed idle claim window did not deny with live context")
		}
	}
	if global.Load() || executor.calls.Load() != 0 || f.scope.check(ctx) != nil {
		t.Fatal("ordinary idle close changed fixture safety or executed work")
	}
	// The same fixture must retain ordinary admission for subsequent owner turns.
	episode := behavior76DevelopmentEpisode()
	episode.Initial.Tasks[0].ExecutionPolicy = "normal"
	f.seed(episode)
	provider := &behavior76SetupProvider{initiativeProjectScriptedProvider: &initiativeProjectScriptedProvider{}, fixture: f}
	for _, step := range episode.Steps[:2] {
		outcome, err := f.evaluate(provider, step)
		if err != nil || outcome.State != "completed" || outcome.OwnerRunID == "" || global.Load() {
			t.Fatal("ordinary next owner turn failed after prior window closed")
		}
	}
	if f.scope.closed || f.scope.localStopped() {
		t.Fatal("per-evaluation window closed the whole fixture")
	}
}
