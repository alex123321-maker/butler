package transcript_test

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/butler/butler/internal/memory/transcript"
	"github.com/butler/butler/internal/ownership"
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

	transcriptStore := transcript.NewStore(store.Pool())
	msg, err := transcriptStore.AppendMessage(ctx, transcript.Message{SessionKey: sessionKey, RunID: runID, Role: "user", Content: "hello password=supersecret", MetadataJSON: `{"token":"abc"}`})
	if err != nil {
		t.Fatalf("AppendMessage returned error: %v", err)
	}
	call, err := transcriptStore.AppendToolCall(ctx, transcript.ToolCall{RunID: runID, ToolName: "browser.navigate", Status: "completed", RuntimeTarget: "browser", ArgsJSON: `{"authorization":"Bearer abc"}`, ResultJSON: `{"cookie":"session=abc123"}`, ErrorJSON: `{"connection_string":"postgres://user:pass@localhost/db"}`})
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

	// A resumed worker may still receive delayed model/tool output from an older
	// ownership epoch. Neither output may enter the durable transcript.
	if _, err := store.Pool().Exec(ctx, `INSERT INTO session_ownership(session_key,lease_id,run_id,owner_id,fence_epoch,expires_at) VALUES($1,'transcript-lease',$2,'worker',1,clock_timestamp()+interval '1 minute')`, sessionKey, runID); err != nil {
		t.Fatal(err)
	}
	fenced := ownership.WithToken(ctx, ownership.NewAuthority(store.Pool()), ownership.Token{SessionKey: sessionKey, RunID: runID, LeaseID: "transcript-lease", FenceEpoch: 1})
	if _, err := transcriptStore.AppendMessage(fenced, transcript.Message{SessionKey: sessionKey, RunID: "foreign-run", Role: "assistant", Content: "wrong run"}); !errors.Is(err, ownership.ErrStale) {
		t.Fatalf("foreign run transcript accepted: %v", err)
	}
	if _, err := transcriptStore.AppendToolCall(fenced, transcript.ToolCall{RunID: "foreign-run", ToolName: "web.fetch"}); !errors.Is(err, ownership.ErrStale) {
		t.Fatalf("foreign run tool accepted: %v", err)
	}
	if _, err := transcriptStore.AppendMessage(fenced, transcript.Message{SessionKey: sessionKey, RunID: runID, Role: "assistant", Content: "current epoch"}); err != nil {
		t.Fatalf("current fenced write: %v", err)
	}
	if _, err := store.Pool().Exec(ctx, `UPDATE session_ownership SET fence_epoch=2,lease_id='new-transcript-lease' WHERE session_key=$1`, sessionKey); err != nil {
		t.Fatal(err)
	}
	if _, err := transcriptStore.AppendMessage(fenced, transcript.Message{SessionKey: sessionKey, RunID: runID, Role: "assistant", Content: "late stale output"}); !errors.Is(err, ownership.ErrStale) {
		t.Fatalf("stale model output accepted: %v", err)
	}
	if _, err := transcriptStore.AppendToolCall(fenced, transcript.ToolCall{RunID: runID, ToolName: "web.fetch", Status: "completed"}); !errors.Is(err, ownership.ErrStale) {
		t.Fatalf("stale tool output accepted: %v", err)
	}
	final, err := transcriptStore.GetRunTranscript(ctx, runID)
	if err != nil || len(final.Messages) != 2 || len(final.ToolCalls) != 1 {
		t.Fatalf("stale output changed durable transcript: messages=%d tools=%d err=%v", len(final.Messages), len(final.ToolCalls), err)
	}
}
