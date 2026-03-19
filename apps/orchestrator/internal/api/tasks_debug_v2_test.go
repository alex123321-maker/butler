package api

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/butler/butler/apps/orchestrator/internal/run"
	"github.com/butler/butler/internal/memory/transcript"
)

func TestTaskDebugServer_ReturnsRawRunAndTranscript(t *testing.T) {
	t.Parallel()

	now := time.Date(2026, 3, 19, 18, 0, 0, 0, time.UTC)
	server := NewTasksDebugServer(
		&fakeRunReader{record: run.Record{RunID: "run-1", SessionKey: "telegram:chat:1", Status: "completed", CurrentState: "completed", ModelProvider: "openai", MetadataJSON: `{"debug":true}`, StartedAt: now, UpdatedAt: now}},
		&fakeTranscriptStore{tx: transcript.Transcript{Messages: []transcript.Message{{MessageID: "m1", RunID: "run-1", Role: "user", Content: "hello", CreatedAt: now}}, ToolCalls: []transcript.ToolCall{{ToolCallID: "t1", RunID: "run-1", ToolName: "http.request", ArgsJSON: `{"url":"https://example.com"}`, Status: "completed", StartedAt: now, ResultJSON: `{"ok":true}`}}}},
	)

	request := httptest.NewRequest(http.MethodGet, "/api/v2/tasks/run-1/debug", nil)
	recorder := httptest.NewRecorder()
	server.HandleGetTaskDebug("/api/v2/tasks/").ServeHTTP(recorder, request)

	if recorder.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", recorder.Code)
	}
	var payload map[string]any
	if err := json.Unmarshal(recorder.Body.Bytes(), &payload); err != nil {
		t.Fatalf("decode response failed: %v", err)
	}
	if payload["run"].(map[string]any)["run_id"] != "run-1" {
		t.Fatalf("unexpected run payload: %v", payload["run"])
	}
	if len(payload["transcript"].(map[string]any)["messages"].([]any)) != 1 {
		t.Fatalf("expected one transcript message")
	}
}
