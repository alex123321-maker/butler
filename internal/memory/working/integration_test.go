package working

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	postgresstore "github.com/butler/butler/internal/storage/postgres"
)

func TestWorkingMemoryStoreIntegration(t *testing.T) {
	dsn := os.Getenv("BUTLER_TEST_POSTGRES_URL")
	if dsn == "" {
		t.Skip("BUTLER_TEST_POSTGRES_URL is not set")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()

	store, err := postgresstore.Open(ctx, postgresstore.Config{URL: dsn, MaxConns: 4, MinConns: 1, MaxConnLifetime: time.Minute}, nil)
	if err != nil {
		t.Fatalf("Open returned error: %v", err)
	}
	defer store.Close()

	migrationsDir := filepath.Clean(filepath.Join("..", "..", "..", "migrations"))
	if err := store.RunMigrations(ctx, migrationsDir); err != nil {
		t.Fatalf("RunMigrations returned error: %v", err)
	}

	sessionKey := "integration:working-memory"
	_, _ = store.Pool().Exec(ctx, `DELETE FROM memory_working WHERE session_key = $1`, sessionKey)
	_, _ = store.Pool().Exec(ctx, `DELETE FROM runs WHERE session_key = $1`, sessionKey)
	_, _ = store.Pool().Exec(ctx, `DELETE FROM sessions WHERE session_key = $1`, sessionKey)
	defer func() {
		_, _ = store.Pool().Exec(context.Background(), `DELETE FROM memory_working WHERE session_key = $1`, sessionKey)
		_, _ = store.Pool().Exec(context.Background(), `DELETE FROM runs WHERE session_key = $1`, sessionKey)
		_, _ = store.Pool().Exec(context.Background(), `DELETE FROM sessions WHERE session_key = $1`, sessionKey)
	}()

	if _, err := store.Pool().Exec(ctx, `INSERT INTO sessions (session_key, user_id, channel, metadata) VALUES ($1, $2, $3, '{}'::jsonb)`, sessionKey, "integration-user", "integration"); err != nil {
		t.Fatalf("failed to seed session: %v", err)
	}
	if _, err := store.Pool().Exec(ctx, `INSERT INTO runs (run_id, session_key, input_event_id, status, autonomy_mode, current_state, model_provider, started_at, updated_at, metadata_json) VALUES ($1, $2, $3, $4, $5, $6, $7, NOW(), NOW(), '{}'::jsonb)`, "run-working-1", sessionKey, "event-1", "created", "mode_1", "created", "openai"); err != nil {
		t.Fatalf("failed to seed run: %v", err)
	}

	workingStore := NewStore(store.Pool())
	saved, err := workingStore.Save(ctx, Snapshot{
		MemoryType:       "working",
		SessionKey:       sessionKey,
		RunID:            "run-working-1",
		Goal:             "Verify working memory persistence",
		EntitiesJSON:     `{"task":"working-memory"}`,
		PendingStepsJSON: `["check persistence"]`,
		ScratchJSON:      `{"attempt":1}`,
		Status:           "active",
		SourceType:       "run",
		SourceID:         "run-working-1",
	})
	if err != nil {
		t.Fatalf("Save returned error: %v", err)
	}
	if saved.ID == 0 {
		t.Fatal("expected persisted snapshot id")
	}

	loaded, err := workingStore.Get(ctx, sessionKey)
	if err != nil {
		t.Fatalf("Get returned error: %v", err)
	}
	if loaded.Goal != "Verify working memory persistence" {
		t.Fatalf("unexpected goal %q", loaded.Goal)
	}
	if loaded.MemoryType != "working" || loaded.SourceType != "run" || loaded.SourceID != "run-working-1" || loaded.ProvenanceJSON == "" {
		t.Fatalf("expected provenance-aware working snapshot, got %+v", loaded)
	}

	updated, err := workingStore.Save(ctx, Snapshot{SessionKey: sessionKey, Goal: "Updated goal", Status: "completed"})
	if err != nil {
		t.Fatalf("Save update returned error: %v", err)
	}
	if updated.Goal != "Updated goal" || updated.Status != "completed" {
		t.Fatalf("unexpected updated snapshot %+v", updated)
	}

	if err := workingStore.Clear(ctx, sessionKey); err != nil {
		t.Fatalf("Clear returned error: %v", err)
	}
	if _, err := workingStore.Get(ctx, sessionKey); err != ErrSnapshotNotFound {
		t.Fatalf("expected ErrSnapshotNotFound after clear, got %v", err)
	}
}
