package orchestrator

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	runservice "github.com/butler/butler/apps/orchestrator/internal/run"
	"github.com/butler/butler/apps/orchestrator/internal/session"
	runv1 "github.com/butler/butler/internal/gen/run/v1"
	"github.com/butler/butler/internal/ingress"
	"github.com/butler/butler/internal/memory/transcript"
	postgresstore "github.com/butler/butler/internal/storage/postgres"
	redisstore "github.com/butler/butler/internal/storage/redis"
	"github.com/butler/butler/internal/transport"
)

func TestExecuteIntegrationPersistsRunAndTranscript(t *testing.T) {
	postgresURL := os.Getenv("BUTLER_TEST_POSTGRES_URL")
	redisURL := os.Getenv("BUTLER_TEST_REDIS_URL")
	if postgresURL == "" || redisURL == "" {
		t.Skip("set BUTLER_TEST_POSTGRES_URL and BUTLER_TEST_REDIS_URL to run integration tests")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()

	postgres, err := postgresstore.Open(ctx, postgresstore.Config{URL: postgresURL, MaxConns: 4, MinConns: 1, MaxConnLifetime: time.Minute}, nil)
	if err != nil {
		t.Fatalf("open postgres: %v", err)
	}
	defer postgres.Close()

	migrationsDir := filepath.Clean(filepath.Join("..", "..", "..", "..", "migrations"))
	if err := postgres.RunMigrations(ctx, migrationsDir); err != nil {
		t.Fatalf("run migrations: %v", err)
	}

	redis, err := redisstore.Open(ctx, redisstore.Config{URL: redisURL}, nil)
	if err != nil {
		t.Fatalf("open redis: %v", err)
	}
	defer func() { _ = redis.Close() }()

	sessionKey := "integration:orchestrator:session"
	_, _ = postgres.Pool().Exec(ctx, `DELETE FROM messages WHERE session_key = $1`, sessionKey)
	_, _ = postgres.Pool().Exec(ctx, `DELETE FROM runs WHERE session_key = $1`, sessionKey)
	_, _ = postgres.Pool().Exec(ctx, `DELETE FROM sessions WHERE session_key = $1`, sessionKey)
	defer func() {
		_, _ = postgres.Pool().Exec(context.Background(), `DELETE FROM messages WHERE session_key = $1`, sessionKey)
		_, _ = postgres.Pool().Exec(context.Background(), `DELETE FROM runs WHERE session_key = $1`, sessionKey)
		_, _ = postgres.Pool().Exec(context.Background(), `DELETE FROM sessions WHERE session_key = $1`, sessionKey)
	}()

	service := NewService(
		session.NewPostgresRepository(postgres.Pool()),
		session.NewRedisLeaseManager(redis.Client(), nil),
		runservice.NewService(runservice.NewPostgresRepository(postgres.Pool()), nil),
		transcript.NewStore(postgres.Pool()),
		&mockProvider{events: []transport.TransportEvent{
			transport.NewAssistantDeltaEvent("", "openai", transport.AssistantDelta{Content: "Hel", SequenceNo: 1}),
			transport.NewAssistantFinalEvent("", "openai", transport.AssistantFinal{Content: "Hello integration", FinishReason: "completed"}),
			transport.NewTerminalEvent("", transport.EventTypeRunCompleted, "openai"),
		}},
		Config{ProviderName: "openai", ModelName: "gpt-5-mini", OwnerID: "integration-test", LeaseTTL: 60},
		nil,
	)

	result, err := service.Execute(ctx, ingress.InputEvent{
		EventID:        "integration-event-1",
		EventType:      runv1.InputEventType_INPUT_EVENT_TYPE_USER_MESSAGE,
		SessionKey:     sessionKey,
		Source:         "integration",
		PayloadJSON:    `{"text":"hello integration","user_id":"integration-user"}`,
		CreatedAt:      time.Now().UTC(),
		IdempotencyKey: "integration-event-1",
	})
	if err != nil {
		t.Fatalf("Execute returned error: %v", err)
	}

	var state string
	if err := postgres.Pool().QueryRow(ctx, `SELECT current_state FROM runs WHERE run_id = $1`, result.RunID).Scan(&state); err != nil {
		t.Fatalf("query run state: %v", err)
	}
	if state != "completed" {
		t.Fatalf("expected completed run state, got %q", state)
	}

	transcriptRows, err := transcript.NewStore(postgres.Pool()).GetRunTranscript(ctx, result.RunID)
	if err != nil {
		t.Fatalf("GetRunTranscript returned error: %v", err)
	}
	if len(transcriptRows.Messages) != 2 {
		t.Fatalf("expected two transcript messages, got %d", len(transcriptRows.Messages))
	}
	if transcriptRows.Messages[0].Role != "user" || transcriptRows.Messages[1].Role != "assistant" {
		t.Fatalf("unexpected transcript roles: %+v", transcriptRows.Messages)
	}
	if transcriptRows.Messages[1].Content != "Hello integration" {
		t.Fatalf("unexpected assistant transcript content %q", transcriptRows.Messages[1].Content)
	}
}
