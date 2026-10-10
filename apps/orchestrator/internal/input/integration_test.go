package input

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"

	flow "github.com/butler/butler/apps/orchestrator/internal/orchestrator"
	"github.com/butler/butler/apps/orchestrator/internal/run"
	"github.com/butler/butler/apps/orchestrator/internal/session"
	commonv1 "github.com/butler/butler/internal/gen/common/v1"
	runv1 "github.com/butler/butler/internal/gen/run/v1"
	"github.com/butler/butler/internal/ingress"
	postgresstore "github.com/butler/butler/internal/storage/postgres"
)

type testExecutor struct {
	mu      sync.Mutex
	seen    []string
	started chan string
	release chan struct{}
}

func (e *testExecutor) Execute(ctx context.Context, event ingress.InputEvent) (*flow.ExecutionResult, error) {
	e.mu.Lock()
	e.seen = append(e.seen, event.EventID)
	e.mu.Unlock()
	select {
	case e.started <- event.EventID:
	case <-ctx.Done():
		return nil, ctx.Err()
	}
	select {
	case <-e.release:
	case <-ctx.Done():
		return nil, ctx.Err()
	}
	return &flow.ExecutionResult{CurrentState: commonv1.RunState_RUN_STATE_COMPLETED}, nil
}
func TestDurableInputAcceptanceOrderingRecoveryIntegration(t *testing.T) {
	dsn := os.Getenv("BUTLER_TEST_POSTGRES_URL")
	if dsn == "" {
		t.Skip("BUTLER_TEST_POSTGRES_URL is not set")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	db, err := postgresstore.Open(ctx, postgresstore.Config{URL: dsn, MaxConns: 4, MinConns: 1}, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	if err = db.RunMigrations(ctx, filepath.Join("..", "..", "..", "..", "migrations")); err != nil {
		t.Fatal(err)
	}
	key := "integration:input:" + time.Now().UTC().Format("150405.000000000")
	defer db.Pool().Exec(context.Background(), `DELETE FROM sessions WHERE session_key LIKE $1`, key+"%")
	store := NewStore(db.Pool())
	repo := session.NewPostgresRepository(db.Pool())
	runs := run.NewService(run.NewPostgresRepository(db.Pool()), nil)
	executor := &testExecutor{started: make(chan string, 4), release: make(chan struct{})}
	service := NewService(store, repo, executor, runs, nil)
	event := func(id, sessionKey string) ingress.InputEvent {
		return ingress.InputEvent{EventID: id, IdempotencyKey: id, SessionKey: sessionKey, Source: "web", EventType: runv1.InputEventType_INPUT_EVENT_TYPE_USER_MESSAGE, PayloadJSON: `{"text":"hello","user_id":"owner"}`, CreatedAt: time.Now().UTC()}
	}
	client, disconnect := context.WithCancel(ctx)
	receipt, err := service.Execute(client, event("first", key))
	if err != nil {
		t.Fatal(err)
	}
	disconnect()
	if !receipt.Accepted || receipt.InputID == "" || receipt.TaskID != "" {
		t.Fatalf("receipt contract: %#v", receipt)
	}
	duplicate, err := service.Execute(ctx, event("first", key))
	if err != nil || duplicate.InputID != receipt.InputID {
		t.Fatalf("dedup: %#v %v", duplicate, err)
	}
	conflict := event("first", key)
	conflict.PayloadJSON = `{"text":"changed","user_id":"owner"}`
	if _, err = service.Execute(ctx, conflict); err == nil {
		t.Fatal("changed payload reused accepted identity")
	}
	second, err := service.Execute(ctx, event("second", key))
	if err != nil {
		t.Fatal(err)
	}
	independent, err := service.Execute(ctx, event("independent", key+":other"))
	if err != nil {
		t.Fatal(err)
	}
	workCtx, stop := context.WithCancel(ctx)
	done := make(chan error, 1)
	go func() { done <- service.Run(workCtx) }()
	starts := map[string]bool{}
	for range 2 {
		select {
		case id := <-executor.started:
			starts[id] = true
		case <-ctx.Done():
			t.Fatal("worker did not start")
		}
	}
	if !starts["first"] || !starts["independent"] || starts["second"] {
		t.Fatalf("FIFO/fair session claims: %v", starts)
	}
	close(executor.release)
	select {
	case id := <-executor.started:
		if id != "second" {
			t.Fatalf("expected second, got %s", id)
		}
	case <-ctx.Done():
		t.Fatal("second input starved")
	}
	wait := func(id, status string) {
		t.Helper()
		for {
			r, err := store.Get(ctx, id)
			if err != nil {
				t.Fatal(err)
			}
			if r.Status == status {
				return
			}
			select {
			case <-ctx.Done():
				t.Fatalf("input %s stayed %s, expected %s", id, r.Status, status)
			case <-time.After(10 * time.Millisecond):
			}
		}
	}
	wait(receipt.InputID, "completed")
	wait(second.InputID, "completed")
	wait(independent.InputID, "completed")
	stop()
	if err = <-done; err != nil {
		t.Fatal(err)
	}
	executor.mu.Lock()
	if len(executor.seen) != 3 {
		t.Fatalf("duplicate delivery executed twice: %v", executor.seen)
	}
	executor.mu.Unlock()
	// A process stops after accepting/claiming, before execution checkpoint.
	pending, err := service.Execute(ctx, event("restart", key))
	if err != nil {
		t.Fatal(err)
	}
	claim, err := store.Claim(ctx, time.Minute)
	if err != nil || claim.InputID != pending.InputID {
		t.Fatalf("claim restart: %#v %v", claim, err)
	}
	if _, err = db.Pool().Exec(ctx, `UPDATE accepted_inputs SET claim_expires_at=clock_timestamp()-INTERVAL '1 second' WHERE input_id=$1`, claim.InputID); err != nil {
		t.Fatal(err)
	}
	reclaimed, err := NewStore(db.Pool()).Claim(ctx, time.Minute)
	if err != nil || reclaimed.InputID != claim.InputID || reclaimed.ClaimID == claim.ClaimID {
		t.Fatalf("restart ownership: %#v %v", reclaimed, err)
	}
	if err = store.Finish(ctx, claim, "completed", "", "", 0); !errors.Is(err, ErrClaimLost) {
		t.Fatalf("stale queue finisher accepted: %v", err)
	}
	if err = store.Finish(ctx, reclaimed, "blocked", "effect_unknown", "manual reconciliation required", 0); err != nil {
		t.Fatal(err)
	}
	tail, err := service.Execute(ctx, event("after-blocked", key))
	if err != nil {
		t.Fatal(err)
	}
	if _, err = store.Claim(ctx, time.Minute); !errors.Is(err, ErrNotFound) {
		t.Fatalf("blocked head bypassed for %s: %v", tail.InputID, err)
	}
	// A new owner decision is a continuation, while crashes after it is claimed
	// consume the same bounded recovery allowance as other execution stages.
	retryReceipt, err := service.Execute(ctx, event("retry-limit", key+":retries"))
	if err != nil {
		t.Fatal(err)
	}
	retry, err := store.Claim(ctx, time.Minute)
	if err != nil || retry.InputID != retryReceipt.InputID || retry.Attempts != 1 {
		t.Fatalf("first retry claim: %#v %v", retry, err)
	}
	if _, err = db.Pool().Exec(ctx, `UPDATE accepted_inputs SET checkpoint='approval_ready' WHERE input_id=$1`, retry.InputID); err != nil {
		t.Fatal(err)
	}
	if err = store.Finish(ctx, retry, "waiting_for_user", "", "", 0); err != nil {
		t.Fatal(err)
	}
	retry, err = store.Claim(ctx, time.Minute)
	if err != nil || retry.Attempts != 1 {
		t.Fatalf("owner decision consumed retry: %#v %v", retry, err)
	}
	for expected := 2; expected <= retry.MaxAttempts+1; expected++ {
		if _, err = db.Pool().Exec(ctx, `UPDATE accepted_inputs SET claim_expires_at=clock_timestamp()-INTERVAL '1 second' WHERE input_id=$1`, retry.InputID); err != nil {
			t.Fatal(err)
		}
		retry, err = store.Claim(ctx, time.Minute)
		if err != nil || retry.Attempts != expected {
			t.Fatalf("crash retry %d: %#v %v", expected, retry, err)
		}
	}
	service.process(ctx, retry)
	finished, err := store.Get(ctx, retry.InputID)
	if err != nil || finished.Status != "failed" || finished.ErrorCode != "retry_exhausted" {
		t.Fatalf("retry bound ignored: %#v %v", finished, err)
	}
}
