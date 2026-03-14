package run

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	commonv1 "github.com/butler/butler/internal/gen/common/v1"
	runv1 "github.com/butler/butler/internal/gen/run/v1"
	sessionv1 "github.com/butler/butler/internal/gen/session/v1"
	postgresstore "github.com/butler/butler/internal/storage/postgres"
)

func TestRunLifecycleIntegration(t *testing.T) {
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

	sessionKey := "integration:run:session"
	_, _ = store.Pool().Exec(ctx, `DELETE FROM runs WHERE session_key = $1`, sessionKey)
	_, _ = store.Pool().Exec(ctx, `DELETE FROM sessions WHERE session_key = $1`, sessionKey)
	defer func() {
		_, _ = store.Pool().Exec(context.Background(), `DELETE FROM runs WHERE session_key = $1`, sessionKey)
		_, _ = store.Pool().Exec(context.Background(), `DELETE FROM sessions WHERE session_key = $1`, sessionKey)
	}()

	if _, err := store.Pool().Exec(ctx, `INSERT INTO sessions (session_key, user_id, channel, metadata) VALUES ($1, $2, $3, '{}'::jsonb)`, sessionKey, "integration-user", "integration"); err != nil {
		t.Fatalf("failed to seed session: %v", err)
	}

	service := NewService(NewPostgresRepository(store.Pool()), nil)
	created, err := service.CreateRun(ctx, &sessionv1.CreateRunRequest{
		SessionKey:    sessionKey,
		InputEvent:    &runv1.InputEvent{EventId: "event-1", EventType: runv1.InputEventType_INPUT_EVENT_TYPE_USER_MESSAGE, IdempotencyKey: "event-1-key"},
		AutonomyMode:  commonv1.AutonomyMode_AUTONOMY_MODE_1,
		ModelProvider: "openai",
		MetadataJson:  `{"origin":"integration"}`,
	})
	if err != nil {
		t.Fatalf("CreateRun returned error: %v", err)
	}

	path := []struct{ from, to commonv1.RunState }{
		{commonv1.RunState_RUN_STATE_CREATED, commonv1.RunState_RUN_STATE_QUEUED},
		{commonv1.RunState_RUN_STATE_QUEUED, commonv1.RunState_RUN_STATE_ACQUIRED},
		{commonv1.RunState_RUN_STATE_ACQUIRED, commonv1.RunState_RUN_STATE_PREPARING},
		{commonv1.RunState_RUN_STATE_PREPARING, commonv1.RunState_RUN_STATE_MODEL_RUNNING},
		{commonv1.RunState_RUN_STATE_MODEL_RUNNING, commonv1.RunState_RUN_STATE_FINALIZING},
		{commonv1.RunState_RUN_STATE_FINALIZING, commonv1.RunState_RUN_STATE_COMPLETED},
	}

	current := created
	for _, step := range path {
		current, err = service.TransitionRun(ctx, &sessionv1.UpdateRunStateRequest{RunId: current.GetRunId(), FromState: step.from, ToState: step.to})
		if err != nil {
			t.Fatalf("TransitionRun %v -> %v returned error: %v", step.from, step.to, err)
		}
	}

	fetched, err := service.GetRun(ctx, current.GetRunId())
	if err != nil {
		t.Fatalf("GetRun returned error: %v", err)
	}
	if fetched.GetCurrentState() != commonv1.RunState_RUN_STATE_COMPLETED {
		t.Fatalf("expected completed state, got %v", fetched.GetCurrentState())
	}
	if fetched.GetFinishedAt() == "" {
		t.Fatal("expected finished_at after completed transition")
	}
	if fetched.GetMetadataJson() != `{"origin":"integration"}` {
		t.Fatalf("expected metadata_json to round-trip, got %q", fetched.GetMetadataJson())
	}

	var state string
	var metadata string
	if err := store.Pool().QueryRow(ctx, `SELECT current_state, metadata_json::text FROM runs WHERE run_id = $1`, current.GetRunId()).Scan(&state, &metadata); err != nil {
		t.Fatalf("failed to query run state: %v", err)
	}
	if state != "completed" {
		t.Fatalf("expected persisted completed state, got %q", state)
	}
	if metadata != `{"origin": "integration"}` && metadata != `{"origin":"integration"}` {
		t.Fatalf("expected persisted metadata_json, got %q", metadata)
	}
}
