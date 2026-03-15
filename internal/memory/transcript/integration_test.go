package transcript

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	postgresstore "github.com/butler/butler/internal/storage/postgres"
)

func TestTranscriptStoreIntegration(t *testing.T) {
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

	sessionKey := "transcript:session:1"
	runID := "transcript-run-1"
	_, _ = store.Pool().Exec(ctx, `DELETE FROM tool_calls WHERE run_id = $1`, runID)
	_, _ = store.Pool().Exec(ctx, `DELETE FROM messages WHERE session_key = $1`, sessionKey)
	_, _ = store.Pool().Exec(ctx, `DELETE FROM runs WHERE run_id = $1`, runID)
	_, _ = store.Pool().Exec(ctx, `DELETE FROM sessions WHERE session_key = $1`, sessionKey)
	defer func() {
		_, _ = store.Pool().Exec(context.Background(), `DELETE FROM tool_calls WHERE run_id = $1`, runID)
		_, _ = store.Pool().Exec(context.Background(), `DELETE FROM messages WHERE session_key = $1`, sessionKey)
		_, _ = store.Pool().Exec(context.Background(), `DELETE FROM runs WHERE run_id = $1`, runID)
		_, _ = store.Pool().Exec(context.Background(), `DELETE FROM sessions WHERE session_key = $1`, sessionKey)
	}()

	if _, err := store.Pool().Exec(ctx, `INSERT INTO sessions (session_key, user_id, channel, metadata) VALUES ($1, $2, $3, '{}'::jsonb)`, sessionKey, "user-1", "integration"); err != nil {
		t.Fatalf("seed session: %v", err)
	}
	if _, err := store.Pool().Exec(ctx, `INSERT INTO runs (run_id, session_key, input_event_id, status, autonomy_mode, current_state, model_provider, started_at, updated_at) VALUES ($1, $2, $3, $4, $5, $6, $7, NOW(), NOW())`, runID, sessionKey, "event-1", "created", "mode_1", "created", "openai"); err != nil {
		t.Fatalf("seed run: %v", err)
	}

	transcriptStore := NewStore(store.Pool())
	msg, err := transcriptStore.AppendMessage(ctx, Message{SessionKey: sessionKey, RunID: runID, Role: "user", Content: "hello password=supersecret", MetadataJSON: `{"token":"abc"}`})
	if err != nil {
		t.Fatalf("AppendMessage returned error: %v", err)
	}
	call, err := transcriptStore.AppendToolCall(ctx, ToolCall{RunID: runID, ToolName: "browser.navigate", Status: "completed", RuntimeTarget: "browser", ArgsJSON: `{"authorization":"Bearer abc"}`, ResultJSON: `{"cookie":"session=abc123"}`, ErrorJSON: `{"connection_string":"postgres://user:pass@localhost/db"}`})
	if err != nil {
		t.Fatalf("AppendToolCall returned error: %v", err)
	}

	full, err := transcriptStore.GetTranscript(ctx, sessionKey)
	if err != nil {
		t.Fatalf("GetTranscript returned error: %v", err)
	}
	if len(full.Messages) != 1 || full.Messages[0].MessageID != msg.MessageID {
		t.Fatalf("unexpected transcript messages: %+v", full.Messages)
	}
	if len(full.ToolCalls) != 1 || full.ToolCalls[0].ToolCallID != call.ToolCallID {
		t.Fatalf("unexpected transcript tool calls: %+v", full.ToolCalls)
	}

	runTranscript, err := transcriptStore.GetRunTranscript(ctx, runID)
	if err != nil {
		t.Fatalf("GetRunTranscript returned error: %v", err)
	}
	if len(runTranscript.Messages) != 1 || len(runTranscript.ToolCalls) != 1 {
		t.Fatalf("unexpected run transcript: %+v", runTranscript)
	}
	if runTranscript.Messages[0].Content != "hello password=supersecret" || runTranscript.ToolCalls[0].ArgsJSON == "" {
		t.Fatalf("expected transcript store to preserve raw transcript for source-of-truth audit, got %+v", runTranscript)
	}
}
