package session

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	runservice "github.com/butler/butler/apps/orchestrator/internal/run"
	runv1 "github.com/butler/butler/internal/gen/run/v1"
	sessionv1 "github.com/butler/butler/internal/gen/session/v1"
	postgresstore "github.com/butler/butler/internal/storage/postgres"
)

func TestSessionPersistenceIntegration(t *testing.T) {
	dsn := os.Getenv("BUTLER_TEST_POSTGRES_URL")
	if dsn == "" {
		t.Skip("BUTLER_TEST_POSTGRES_URL is not set")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()

	store, err := postgresstore.Open(ctx, postgresstore.Config{
		URL:             dsn,
		MaxConns:        4,
		MinConns:        1,
		MaxConnLifetime: time.Minute,
	}, nil)
	if err != nil {
		t.Fatalf("Open returned error: %v", err)
	}
	defer store.Close()

	migrationsDir := filepath.Clean(filepath.Join("..", "..", "..", "..", "migrations"))
	if err := store.RunMigrations(ctx, migrationsDir); err != nil {
		t.Fatalf("RunMigrations returned error: %v", err)
	}

	server := NewServer(NewPostgresRepository(store.Pool()), nil, nil, time.Minute, nil)
	sessionKey := "integration:chat:session-1"

	_, _ = store.Pool().Exec(ctx, `DELETE FROM sessions WHERE session_key = $1`, sessionKey)
	defer func() {
		_, _ = store.Pool().Exec(context.Background(), `DELETE FROM sessions WHERE session_key = $1`, sessionKey)
	}()

	createResp, err := server.CreateSession(ctx, &sessionv1.CreateSessionRequest{
		SessionKey:   sessionKey,
		UserId:       "integration-user",
		Channel:      "integration",
		MetadataJson: `{"source":"integration-test"}`,
	})
	if err != nil {
		t.Fatalf("CreateSession returned error: %v", err)
	}
	if !createResp.GetCreated() {
		t.Fatal("expected created=true")
	}

	getResp, err := server.GetSession(ctx, &sessionv1.GetSessionRequest{SessionKey: sessionKey})
	if err != nil {
		t.Fatalf("GetSession returned error: %v", err)
	}
	if getResp.GetSession().GetSessionKey() != sessionKey {
		t.Fatalf("unexpected session key %q", getResp.GetSession().GetSessionKey())
	}

	var persistedUserID string
	if err := store.Pool().QueryRow(ctx, `SELECT user_id FROM sessions WHERE session_key = $1`, sessionKey).Scan(&persistedUserID); err != nil {
		t.Fatalf("failed to query persisted session: %v", err)
	}
	if persistedUserID != "integration-user" {
		t.Fatalf("unexpected persisted user_id %q", persistedUserID)
	}
}

func TestCreateRunDeduplicatesByIdempotencyKeyIntegration(t *testing.T) {
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

	migrationsDir := filepath.Clean(filepath.Join("..", "..", "..", "..", "migrations"))
	if err := store.RunMigrations(ctx, migrationsDir); err != nil {
		t.Fatalf("RunMigrations returned error: %v", err)
	}

	sessionKey := "integration:dedupe:session"
	_, _ = store.Pool().Exec(ctx, `DELETE FROM runs WHERE session_key = $1`, sessionKey)
	_, _ = store.Pool().Exec(ctx, `DELETE FROM sessions WHERE session_key = $1`, sessionKey)
	defer func() {
		_, _ = store.Pool().Exec(context.Background(), `DELETE FROM runs WHERE session_key = $1`, sessionKey)
		_, _ = store.Pool().Exec(context.Background(), `DELETE FROM sessions WHERE session_key = $1`, sessionKey)
	}()

	if _, err := store.Pool().Exec(ctx, `INSERT INTO sessions (session_key, user_id, channel, metadata) VALUES ($1, $2, $3, '{}'::jsonb)`, sessionKey, "integration-user", "integration"); err != nil {
		t.Fatalf("failed to seed session: %v", err)
	}

	runs := runservice.NewService(runservice.NewPostgresRepository(store.Pool()), nil)
	server := NewServer(NewPostgresRepository(store.Pool()), nil, runs, time.Minute, nil)
	request := &sessionv1.CreateRunRequest{
		SessionKey:    sessionKey,
		InputEvent:    &runv1.InputEvent{EventId: "event-1", IdempotencyKey: "duplicate-key"},
		AutonomyMode:  1,
		ModelProvider: "openai",
	}

	first, err := server.CreateRun(ctx, request)
	if err != nil {
		t.Fatalf("first CreateRun returned error: %v", err)
	}
	second, err := server.CreateRun(ctx, request)
	if err != nil {
		t.Fatalf("second CreateRun returned error: %v", err)
	}
	if first.GetRun().GetRunId() != second.GetRun().GetRunId() {
		t.Fatalf("expected duplicate create to resolve to same run, got %q and %q", first.GetRun().GetRunId(), second.GetRun().GetRunId())
	}

	var count int
	if err := store.Pool().QueryRow(ctx, `SELECT COUNT(*) FROM runs WHERE session_key = $1 AND idempotency_key = $2`, sessionKey, "duplicate-key").Scan(&count); err != nil {
		t.Fatalf("failed to count runs: %v", err)
	}
	if count != 1 {
		t.Fatalf("expected one deduplicated run, got %d", count)
	}
}
