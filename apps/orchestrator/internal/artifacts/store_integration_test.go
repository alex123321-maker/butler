package artifacts

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	postgresstore "github.com/butler/butler/internal/storage/postgres"
)

func TestArtifactsRepositoryIntegration_CreateAndList(t *testing.T) {
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

	sessionKey := "artifacts:integration:session"
	runID := "artifacts-integration-run"
	_, _ = store.Pool().Exec(ctx, `DELETE FROM artifacts WHERE run_id = $1`, runID)
	_, _ = store.Pool().Exec(ctx, `DELETE FROM runs WHERE run_id = $1`, runID)
	_, _ = store.Pool().Exec(ctx, `DELETE FROM sessions WHERE session_key = $1`, sessionKey)
	defer func() {
		_, _ = store.Pool().Exec(context.Background(), `DELETE FROM artifacts WHERE run_id = $1`, runID)
		_, _ = store.Pool().Exec(context.Background(), `DELETE FROM runs WHERE run_id = $1`, runID)
		_, _ = store.Pool().Exec(context.Background(), `DELETE FROM sessions WHERE session_key = $1`, sessionKey)
	}()

	if _, err := store.Pool().Exec(ctx, `INSERT INTO sessions (session_key, user_id, channel, metadata) VALUES ($1, $2, $3, '{}'::jsonb)`, sessionKey, "artifacts-user", "telegram"); err != nil {
		t.Fatalf("seed session failed: %v", err)
	}
	now := time.Now().UTC()
	if _, err := store.Pool().Exec(ctx, `INSERT INTO runs (run_id, session_key, input_event_id, status, autonomy_mode, current_state, model_provider, started_at, updated_at) VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$8)`, runID, sessionKey, "event-art-1", "completed", "mode_1", "completed", "openai", now); err != nil {
		t.Fatalf("seed run failed: %v", err)
	}

	repo := NewPostgresRepository(store.Pool())
	created, err := repo.CreateArtifact(ctx, CreateParams{
		ArtifactID:    "artifact-int-1",
		RunID:         runID,
		SessionKey:    sessionKey,
		ArtifactType:  TypeAssistantFinal,
		Title:         "Assistant final response",
		Summary:       "done",
		ContentText:   "done",
		ContentJSON:   `{"kind":"assistant_final"}`,
		ContentFormat: "text",
		SourceType:    "message",
		SourceRef:     runID,
		CreatedAt:     now,
	})
	if err != nil {
		t.Fatalf("CreateArtifact returned error: %v", err)
	}
	if created.ArtifactType != TypeAssistantFinal {
		t.Fatalf("expected assistant_final artifact, got %q", created.ArtifactType)
	}

	items, err := repo.ListArtifactsByRun(ctx, runID, 20)
	if err != nil {
		t.Fatalf("ListArtifactsByRun returned error: %v", err)
	}
	if len(items) != 1 {
		t.Fatalf("expected one artifact, got %d", len(items))
	}
}
