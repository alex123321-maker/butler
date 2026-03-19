package approval

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	postgresstore "github.com/butler/butler/internal/storage/postgres"
)

func TestApprovalRepositoryIntegration_CreateResolveAndEvents(t *testing.T) {
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

	sessionKey := "approval:integration:session"
	runID := "approval-integration-run"
	_, _ = store.Pool().Exec(ctx, `DELETE FROM approval_events WHERE run_id = $1`, runID)
	_, _ = store.Pool().Exec(ctx, `DELETE FROM approvals WHERE run_id = $1`, runID)
	_, _ = store.Pool().Exec(ctx, `DELETE FROM runs WHERE run_id = $1`, runID)
	_, _ = store.Pool().Exec(ctx, `DELETE FROM sessions WHERE session_key = $1`, sessionKey)
	defer func() {
		_, _ = store.Pool().Exec(context.Background(), `DELETE FROM approval_events WHERE run_id = $1`, runID)
		_, _ = store.Pool().Exec(context.Background(), `DELETE FROM approvals WHERE run_id = $1`, runID)
		_, _ = store.Pool().Exec(context.Background(), `DELETE FROM runs WHERE run_id = $1`, runID)
		_, _ = store.Pool().Exec(context.Background(), `DELETE FROM sessions WHERE session_key = $1`, sessionKey)
	}()

	if _, err := store.Pool().Exec(ctx, `INSERT INTO sessions (session_key, user_id, channel, metadata) VALUES ($1, $2, $3, '{}'::jsonb)`, sessionKey, "approval-user", "telegram"); err != nil {
		t.Fatalf("seed session failed: %v", err)
	}
	now := time.Now().UTC()
	if _, err := store.Pool().Exec(ctx, `INSERT INTO runs (run_id, session_key, input_event_id, status, autonomy_mode, current_state, model_provider, started_at, updated_at) VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$8)`, runID, sessionKey, "event-1", "awaiting_approval", "mode_1", "awaiting_approval", "openai", now); err != nil {
		t.Fatalf("seed run failed: %v", err)
	}

	repo := NewPostgresRepository(store.Pool())
	rec, err := repo.CreateApproval(ctx, CreateParams{
		ApprovalID:   "approval-int-1",
		RunID:        runID,
		SessionKey:   sessionKey,
		ToolCallID:   "tool-int-1",
		RequestedVia: RequestedViaTelegram,
		ToolName:     "http.request",
		ArgsJSON:     `{"url":"https://example.com"}`,
		RequestedAt:  now,
	})
	if err != nil {
		t.Fatalf("CreateApproval returned error: %v", err)
	}
	if rec.Status != StatusPending {
		t.Fatalf("expected pending status, got %q", rec.Status)
	}

	updated, err := repo.ResolveApproval(ctx, ResolveParams{
		ApprovalID:       rec.ApprovalID,
		ExpectedStatus:   StatusPending,
		Status:           StatusApproved,
		ResolvedVia:      ResolvedViaTelegram,
		ResolvedBy:       "telegram_user:1",
		ResolutionReason: "approved",
		ResolvedAt:       now.Add(time.Minute),
	})
	if err != nil {
		t.Fatalf("ResolveApproval returned error: %v", err)
	}
	if updated.Status != StatusApproved {
		t.Fatalf("expected approved status, got %q", updated.Status)
	}

	if err := repo.InsertEvent(ctx, Event{
		ApprovalID:   rec.ApprovalID,
		RunID:        runID,
		SessionKey:   sessionKey,
		EventType:    "approval_resolved",
		StatusBefore: StatusPending,
		StatusAfter:  StatusApproved,
		ActorType:    "telegram",
		ActorID:      "telegram_user:1",
		Reason:       "approved",
		MetadataJSON: `{"source":"integration"}`,
		CreatedAt:    now.Add(time.Minute),
	}); err != nil {
		t.Fatalf("InsertEvent returned error: %v", err)
	}

	var eventCount int
	if err := store.Pool().QueryRow(ctx, `SELECT COUNT(1) FROM approval_events WHERE approval_id = $1`, rec.ApprovalID).Scan(&eventCount); err != nil {
		t.Fatalf("count approval events failed: %v", err)
	}
	if eventCount == 0 {
		t.Fatal("expected at least one approval event")
	}
}
