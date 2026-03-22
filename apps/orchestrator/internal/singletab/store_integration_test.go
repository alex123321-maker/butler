package singletab

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	postgresstore "github.com/butler/butler/internal/storage/postgres"
)

func TestPostgresRepository_CreateActivateAndLoadSingleTabSession(t *testing.T) {
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

	sessionKey := "singletab:integration:session"
	runID := "singletab-integration-run"
	singleTabSessionID := "single-tab-session-1"

	_, _ = store.Pool().Exec(ctx, `DELETE FROM single_tab_sessions WHERE single_tab_session_id = $1`, singleTabSessionID)
	_, _ = store.Pool().Exec(ctx, `DELETE FROM runs WHERE run_id = $1`, runID)
	_, _ = store.Pool().Exec(ctx, `DELETE FROM sessions WHERE session_key = $1`, sessionKey)
	defer func() {
		_, _ = store.Pool().Exec(context.Background(), `DELETE FROM single_tab_sessions WHERE single_tab_session_id = $1`, singleTabSessionID)
		_, _ = store.Pool().Exec(context.Background(), `DELETE FROM runs WHERE run_id = $1`, runID)
		_, _ = store.Pool().Exec(context.Background(), `DELETE FROM sessions WHERE session_key = $1`, sessionKey)
	}()

	if _, err := store.Pool().Exec(ctx, `INSERT INTO sessions (session_key, user_id, channel, metadata) VALUES ($1, $2, $3, '{}'::jsonb)`, sessionKey, "single-tab-user", "web"); err != nil {
		t.Fatalf("seed session failed: %v", err)
	}
	now := time.Now().UTC()
	if _, err := store.Pool().Exec(ctx, `INSERT INTO runs (run_id, session_key, input_event_id, status, autonomy_mode, current_state, model_provider, started_at, updated_at) VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$8)`, runID, sessionKey, "event-1", "awaiting_approval", "mode_1", "awaiting_approval", "openai", now); err != nil {
		t.Fatalf("seed run failed: %v", err)
	}

	repo := NewPostgresRepository(store.Pool())
	created, err := repo.CreateSession(ctx, CreateParams{
		SingleTabSessionID: singleTabSessionID,
		SessionKey:         sessionKey,
		CreatedByRunID:     runID,
		Status:             StatusPendingApproval,
		BoundTabRef:        "browser-1:tab-42",
		BrowserInstanceID:  "browser-1",
		HostID:             "host-a",
		CreatedAt:          now,
	})
	if err != nil {
		t.Fatalf("CreateSession returned error: %v", err)
	}
	if created.Status != StatusPendingApproval {
		t.Fatalf("expected pending approval status, got %q", created.Status)
	}

	activatedAt := now.Add(time.Minute)
	updated, err := repo.UpdateSessionStatus(ctx, UpdateStatusParams{
		SingleTabSessionID: singleTabSessionID,
		Status:             StatusActive,
		StatusReason:       "approved in web ui",
		SelectedVia:        "web",
		SelectedBy:         "user:web",
		CurrentURL:         "https://example.com",
		CurrentTitle:       "Example",
		ActivatedAt:        &activatedAt,
		LastSeenAt:         &activatedAt,
		UpdatedAt:          activatedAt,
	})
	if err != nil {
		t.Fatalf("UpdateSessionStatus returned error: %v", err)
	}
	if updated.Status != StatusActive {
		t.Fatalf("expected active status, got %q", updated.Status)
	}
	if updated.SelectedVia != "web" {
		t.Fatalf("expected selected_via web, got %q", updated.SelectedVia)
	}

	active, err := repo.GetActiveSessionBySessionKey(ctx, sessionKey)
	if err != nil {
		t.Fatalf("GetActiveSessionBySessionKey returned error: %v", err)
	}
	if active.SingleTabSessionID != singleTabSessionID {
		t.Fatalf("expected single_tab_session_id %q, got %q", singleTabSessionID, active.SingleTabSessionID)
	}
	if active.CurrentURL != "https://example.com" {
		t.Fatalf("expected current_url to be updated, got %q", active.CurrentURL)
	}
}
