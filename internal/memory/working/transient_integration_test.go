package working

import (
	"context"
	"os"
	"testing"
	"time"

	redisstore "github.com/butler/butler/internal/storage/redis"
)

func TestTransientWorkingStoreIntegration(t *testing.T) {
	redisURL := os.Getenv("BUTLER_TEST_REDIS_URL")
	if redisURL == "" {
		t.Skip("BUTLER_TEST_REDIS_URL is not set")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()

	store, err := redisstore.Open(ctx, redisstore.Config{URL: redisURL}, nil)
	if err != nil {
		t.Fatalf("Open returned error: %v", err)
	}
	defer func() {
		if err := store.Close(); err != nil {
			t.Fatalf("Close returned error: %v", err)
		}
	}()

	transient := NewTransientStore(store.Client())
	sessionKey := "integration:transient:working"
	runID := "run-transient-1"
	_ = store.Client().Del(ctx, transientKey(sessionKey, runID)).Err()
	defer func() { _ = store.Client().Del(context.Background(), transientKey(sessionKey, runID)).Err() }()

	saved, err := transient.Save(ctx, TransientState{SessionKey: sessionKey, RunID: runID, Status: "tool_running", ScratchJSON: `{"step":"fetch"}`}, 2*time.Second)
	if err != nil {
		t.Fatalf("Save returned error: %v", err)
	}
	if saved.UpdatedAt == "" {
		t.Fatal("expected UpdatedAt to be populated")
	}

	loaded, err := transient.Get(ctx, sessionKey, runID)
	if err != nil {
		t.Fatalf("Get returned error: %v", err)
	}
	if loaded.Status != "tool_running" {
		t.Fatalf("unexpected transient status %q", loaded.Status)
	}

	ttl, err := transient.TTL(ctx, sessionKey, runID)
	if err != nil {
		t.Fatalf("TTL returned error: %v", err)
	}
	if ttl <= 0 || ttl > 2*time.Second {
		t.Fatalf("unexpected ttl %s", ttl)
	}

	time.Sleep(2200 * time.Millisecond)
	if _, err := transient.Get(ctx, sessionKey, runID); err != ErrTransientStateNotFound {
		t.Fatalf("expected ErrTransientStateNotFound after expiry, got %v", err)
	}

	saved, err = transient.Save(ctx, TransientState{SessionKey: sessionKey, RunID: runID, Status: "active", ScratchJSON: `{"step":"resume"}`}, 5*time.Second)
	if err != nil {
		t.Fatalf("Save after expiry returned error: %v", err)
	}
	if saved.Status != "active" {
		t.Fatalf("unexpected saved status %q", saved.Status)
	}
	if err := transient.Clear(ctx, sessionKey, runID); err != nil {
		t.Fatalf("Clear returned error: %v", err)
	}
	if _, err := transient.Get(ctx, sessionKey, runID); err != ErrTransientStateNotFound {
		t.Fatalf("expected ErrTransientStateNotFound after clear, got %v", err)
	}
}
