package api

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/butler/butler/apps/orchestrator/internal/orchestrator"
	commonv1 "github.com/butler/butler/internal/gen/common/v1"
	orchestratorv1 "github.com/butler/butler/internal/gen/orchestrator/v1"
	runv1 "github.com/butler/butler/internal/gen/run/v1"
	"github.com/butler/butler/internal/ingress"
)

func TestSubmitEvent(t *testing.T) {
	t.Parallel()

	server := NewServer(fakeExecutor{result: &orchestrator.ExecutionResult{RunID: "run-123", SessionKey: "telegram:chat:1", CurrentState: commonv1.RunState_RUN_STATE_COMPLETED, AssistantResponse: "hello"}}, nil)

	resp, err := server.SubmitEvent(context.Background(), &orchestratorv1.SubmitEventRequest{Event: &runv1.InputEvent{
		EventId:        "event-1",
		EventType:      runv1.InputEventType_INPUT_EVENT_TYPE_USER_MESSAGE,
		SessionKey:     "telegram:chat:1",
		Source:         "telegram",
		PayloadJson:    `{"text":"hello"}`,
		CreatedAt:      "2026-03-15T00:00:00Z",
		IdempotencyKey: "event-1",
	}})
	if err != nil {
		t.Fatalf("SubmitEvent returned error: %v", err)
	}
	if resp.GetRunId() != "run-123" {
		t.Fatalf("expected run id, got %q", resp.GetRunId())
	}
	if resp.GetCurrentState() != commonv1.RunState_RUN_STATE_COMPLETED {
		t.Fatalf("expected completed state, got %v", resp.GetCurrentState())
	}
}

func TestHTTPHandler(t *testing.T) {
	t.Parallel()

	server := NewServer(fakeExecutor{result: &orchestrator.ExecutionResult{RunID: "run-123", SessionKey: "telegram:chat:1", CurrentState: commonv1.RunState_RUN_STATE_COMPLETED, AssistantResponse: "hello"}}, nil)
	body := `{"event_id":"event-1","event_type":"user_message","session_key":"telegram:chat:1","source":"telegram","payload":{"text":"hello"},"created_at":"2026-03-15T00:00:00Z","idempotency_key":"event-1"}`
	req := httptest.NewRequest(http.MethodPost, "/api/v1/events", strings.NewReader(body))
	rr := httptest.NewRecorder()

	server.HTTPHandler().ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200 OK, got %d", rr.Code)
	}
	var payload map[string]any
	if err := json.Unmarshal(rr.Body.Bytes(), &payload); err != nil {
		t.Fatalf("decode response body: %v", err)
	}
	if payload["run_id"] != "run-123" {
		t.Fatalf("expected run_id in response, got %v", payload["run_id"])
	}
}

type fakeExecutor struct {
	result *orchestrator.ExecutionResult
	err    error
}

func (f fakeExecutor) Execute(context.Context, ingress.InputEvent) (*orchestrator.ExecutionResult, error) {
	return f.result, f.err
}
