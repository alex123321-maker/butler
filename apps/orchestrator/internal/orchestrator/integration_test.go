package orchestrator

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	runservice "github.com/butler/butler/apps/orchestrator/internal/run"
	"github.com/butler/butler/apps/orchestrator/internal/session"
	runv1 "github.com/butler/butler/internal/gen/run/v1"
	"github.com/butler/butler/internal/ingress"
	"github.com/butler/butler/internal/memory/transcript"
	"github.com/butler/butler/internal/memory/working"
	postgresstore "github.com/butler/butler/internal/storage/postgres"
	redisstore "github.com/butler/butler/internal/storage/redis"
	"github.com/butler/butler/internal/transport"
)

type workingIntegrationAdapter struct{ store *working.Store }

type transientWorkingIntegrationAdapter struct{ store *working.TransientStore }

func (a workingIntegrationAdapter) Get(ctx context.Context, sessionKey string) (WorkingMemorySnapshot, error) {
	snapshot, err := a.store.Get(ctx, sessionKey)
	if err != nil {
		if err == working.ErrSnapshotNotFound {
			return WorkingMemorySnapshot{}, ErrWorkingMemoryNotFound
		}
		return WorkingMemorySnapshot{}, err
	}
	return WorkingMemorySnapshot{SessionKey: snapshot.SessionKey, RunID: snapshot.RunID, Goal: snapshot.Goal, EntitiesJSON: snapshot.EntitiesJSON, PendingStepsJSON: snapshot.PendingStepsJSON, ScratchJSON: snapshot.ScratchJSON, Status: snapshot.Status}, nil
}

func (a workingIntegrationAdapter) Save(ctx context.Context, snapshot WorkingMemorySnapshot) (WorkingMemorySnapshot, error) {
	saved, err := a.store.Save(ctx, working.Snapshot{SessionKey: snapshot.SessionKey, RunID: snapshot.RunID, Goal: snapshot.Goal, EntitiesJSON: snapshot.EntitiesJSON, PendingStepsJSON: snapshot.PendingStepsJSON, ScratchJSON: snapshot.ScratchJSON, Status: snapshot.Status})
	if err != nil {
		return WorkingMemorySnapshot{}, err
	}
	return WorkingMemorySnapshot{SessionKey: saved.SessionKey, RunID: saved.RunID, Goal: saved.Goal, EntitiesJSON: saved.EntitiesJSON, PendingStepsJSON: saved.PendingStepsJSON, ScratchJSON: saved.ScratchJSON, Status: saved.Status}, nil
}

func (a workingIntegrationAdapter) Clear(ctx context.Context, sessionKey string) error {
	err := a.store.Clear(ctx, sessionKey)
	if err == working.ErrSnapshotNotFound {
		return ErrWorkingMemoryNotFound
	}
	return err
}

func (a transientWorkingIntegrationAdapter) Get(ctx context.Context, sessionKey, runID string) (TransientWorkingState, error) {
	state, err := a.store.Get(ctx, sessionKey, runID)
	if err != nil {
		if err == working.ErrTransientStateNotFound {
			return TransientWorkingState{}, ErrTransientWorkingStateNotFound
		}
		return TransientWorkingState{}, err
	}
	return TransientWorkingState{SessionKey: state.SessionKey, RunID: state.RunID, Status: state.Status, ScratchJSON: state.ScratchJSON, UpdatedAt: state.UpdatedAt}, nil
}

func (a transientWorkingIntegrationAdapter) Save(ctx context.Context, state TransientWorkingState, ttl time.Duration) (TransientWorkingState, error) {
	saved, err := a.store.Save(ctx, working.TransientState{SessionKey: state.SessionKey, RunID: state.RunID, Status: state.Status, ScratchJSON: state.ScratchJSON, UpdatedAt: state.UpdatedAt}, ttl)
	if err != nil {
		return TransientWorkingState{}, err
	}
	return TransientWorkingState{SessionKey: saved.SessionKey, RunID: saved.RunID, Status: saved.Status, ScratchJSON: saved.ScratchJSON, UpdatedAt: saved.UpdatedAt}, nil
}

func (a transientWorkingIntegrationAdapter) Clear(ctx context.Context, sessionKey, runID string) error {
	err := a.store.Clear(ctx, sessionKey, runID)
	if err == working.ErrTransientStateNotFound {
		return ErrTransientWorkingStateNotFound
	}
	return err
}

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
	_, _ = postgres.Pool().Exec(ctx, `DELETE FROM memory_working WHERE session_key = $1`, sessionKey)
	_, _ = postgres.Pool().Exec(ctx, `DELETE FROM runs WHERE session_key = $1`, sessionKey)
	_, _ = postgres.Pool().Exec(ctx, `DELETE FROM sessions WHERE session_key = $1`, sessionKey)
	defer func() {
		_, _ = postgres.Pool().Exec(context.Background(), `DELETE FROM messages WHERE session_key = $1`, sessionKey)
		_, _ = postgres.Pool().Exec(context.Background(), `DELETE FROM memory_working WHERE session_key = $1`, sessionKey)
		_, _ = postgres.Pool().Exec(context.Background(), `DELETE FROM runs WHERE session_key = $1`, sessionKey)
		_, _ = postgres.Pool().Exec(context.Background(), `DELETE FROM sessions WHERE session_key = $1`, sessionKey)
	}()

	service := NewService(
		session.NewPostgresRepository(postgres.Pool()),
		session.NewRedisLeaseManager(redis.Client(), nil),
		runservice.NewService(runservice.NewPostgresRepository(postgres.Pool()), nil),
		transcript.NewStore(postgres.Pool()),
		&mockProvider{startEvents: []transport.TransportEvent{
			transport.NewRunStartedEvent("", "openai", transport.CapabilitySnapshot{SupportsStreaming: true}, &transport.ProviderSessionRef{ProviderName: "openai", ResponseRef: "resp_integration"}),
			transport.NewAssistantDeltaEvent("", "openai", transport.AssistantDelta{Content: "Hel", SequenceNo: 1}),
			transport.NewAssistantFinalEvent("", "openai", transport.AssistantFinal{Content: "Hello integration", FinishReason: "completed"}),
			transport.NewTerminalEvent("", transport.EventTypeRunCompleted, "openai"),
		}},
		Config{ProviderName: "openai", ModelName: "gpt-5-mini", OwnerID: "integration-test", LeaseTTL: 60, WorkingStore: workingIntegrationAdapter{store: working.NewStore(postgres.Pool())}, TransientStore: transientWorkingIntegrationAdapter{store: working.NewTransientStore(redis.Client())}, TransientTTL: 2 * time.Second},
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
	var providerSessionRef string
	if err := postgres.Pool().QueryRow(ctx, `SELECT current_state, COALESCE(provider_session_ref, '') FROM runs WHERE run_id = $1`, result.RunID).Scan(&state, &providerSessionRef); err != nil {
		t.Fatalf("query run state: %v", err)
	}
	if state != "completed" {
		t.Fatalf("expected completed run state, got %q", state)
	}
	if providerSessionRef == "" {
		t.Fatal("expected provider_session_ref to be persisted")
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

	var workingCount int
	if err := postgres.Pool().QueryRow(ctx, `SELECT COUNT(*) FROM memory_working WHERE session_key = $1`, sessionKey).Scan(&workingCount); err != nil {
		t.Fatalf("query working memory count: %v", err)
	}
	if workingCount != 0 {
		t.Fatalf("expected completed run to clear working memory, got count %d", workingCount)
	}

	var metadataJSON string
	if err := postgres.Pool().QueryRow(ctx, `SELECT metadata_json::text FROM runs WHERE run_id = $1`, result.RunID).Scan(&metadataJSON); err != nil {
		t.Fatalf("query run metadata: %v", err)
	}
	if !strings.Contains(metadataJSON, "memory_bundle") || !strings.Contains(metadataJSON, "working") {
		t.Fatalf("expected working memory bundle in run metadata, got %s", metadataJSON)
	}
	transientStore := working.NewTransientStore(redis.Client())
	if _, err := transientStore.Get(ctx, sessionKey, result.RunID); err != working.ErrTransientStateNotFound {
		t.Fatalf("expected transient working state to be cleared, got %v", err)
	}
}
