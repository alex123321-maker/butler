package activity

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	postgresstore "github.com/butler/butler/internal/storage/postgres"
)

func TestActivityRepositoryIntegration_CreateAndList(t *testing.T) {
	dsn := os.Getenv("BUTLER_TEST_POSTGRES_URL")
	if dsn == "" {
		t.Skip("BUTLER_TEST_POSTGRES_URL is not set")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 25*time.Second)
	defer cancel()

	store, err := postgresstore.Open(ctx, postgresstore.Config{URL: dsn, MaxConns: 4, MinConns: 1, MaxConnLifetime: time.Minute}, nil)
	if err != nil {
		t.Fatalf("Open returned error: %v", err)
	}
	defer store.Close()

	migrationsDir := filepath.Clean(filepath.Join("..", "..", "..", "..", "migrations"))
	if err := store.RunMigrations(ctx, migrationsDir); err != nil {
		t.Fatalf("RunMigrations returned error: %v", err)
	}

	sessionKey := "activity:integration:session"
	runID := "activity-integration-run"
	_, _ = store.Pool().Exec(ctx, `DELETE FROM task_activity WHERE run_id = $1`, runID)
	_, _ = store.Pool().Exec(ctx, `DELETE FROM runs WHERE run_id = $1`, runID)
	_, _ = store.Pool().Exec(ctx, `DELETE FROM sessions WHERE session_key = $1`, sessionKey)
	defer func() {
		_, _ = store.Pool().Exec(context.Background(), `DELETE FROM task_activity WHERE run_id = $1`, runID)
		_, _ = store.Pool().Exec(context.Background(), `DELETE FROM runs WHERE run_id = $1`, runID)
		_, _ = store.Pool().Exec(context.Background(), `DELETE FROM sessions WHERE session_key = $1`, sessionKey)
	}()

	if _, err := store.Pool().Exec(ctx, `INSERT INTO sessions (session_key, user_id, channel, metadata) VALUES ($1,$2,$3,'{}'::jsonb)`, sessionKey, "activity-user", "telegram"); err != nil {
		t.Fatalf("seed session failed: %v", err)
	}
	now := time.Now().UTC()
	if _, err := store.Pool().Exec(ctx, `INSERT INTO runs (run_id, session_key, input_event_id, status, autonomy_mode, current_state, model_provider, started_at, updated_at) VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$8)`, runID, sessionKey, "event-activity-1", "queued", "mode_1", "queued", "openai", now); err != nil {
		t.Fatalf("seed run failed: %v", err)
	}

	repo := NewPostgresRepository(store.Pool())
	created, err := repo.CreateActivity(ctx, CreateParams{RunID: runID, SessionKey: sessionKey, ActivityType: TypeTaskReceived, Title: "Task received", Summary: "Task context prepared", DetailsJSON: `{"source":"telegram"}`, ActorType: "system", Severity: SeverityInfo, CreatedAt: now})
	if err != nil {
		t.Fatalf("CreateActivity returned error: %v", err)
	}
	if created.ActivityID == 0 {
		t.Fatal("expected non-zero activity id")
	}
	items, err := repo.ListActivities(ctx, ListParams{RunID: runID, Limit: 20, Offset: 0})
	if err != nil {
		t.Fatalf("ListActivities returned error: %v", err)
	}
	if len(items) != 1 {
		t.Fatalf("expected one activity item, got %d", len(items))
	}
}
