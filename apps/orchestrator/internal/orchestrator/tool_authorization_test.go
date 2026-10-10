package orchestrator

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	runv1 "github.com/butler/butler/internal/gen/run/v1"
	toolbrokerv1 "github.com/butler/butler/internal/gen/toolbroker/v1"
	"github.com/butler/butler/internal/ingress"
	"github.com/butler/butler/internal/transport"
)

type unavailableApprovalChecker struct{}

func (unavailableApprovalChecker) RequiresApproval(context.Context, string) (bool, error) {
	return false, errors.New("policy service unavailable")
}

func TestToolAuthorizationFailsClosed(t *testing.T) {
	for _, tc := range []struct {
		name    string
		checker ApprovalChecker
		want    string
	}{
		{"missing policy", nil, "approval checker is not configured"},
		{"failed policy", unavailableApprovalChecker{}, "policy service unavailable"},
		{"missing approval gate", &mockApprovalChecker{toolsRequiringApproval: map[string]bool{"http.request": true}}, "approval gate is not configured"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			for _, batch := range []bool{false, true} {
				tools := &mockToolExecutor{result: &toolbrokerv1.ToolResult{Status: "completed"}}
				call := transport.ToolCallRequest{ToolCallRef: "call-1", ToolName: "http.request", ArgsJSON: `{"url":"https://example.com"}`}
				event := transport.NewToolCallRequestedEvent("", "openai", call)
				if batch {
					event.EventType = transport.EventTypeToolCallBatchRequested
					event.ToolCall = nil
					event.ToolCallBatch = &transport.ToolCallBatch{ToolCalls: []transport.ToolCallRequest{call}}
				}
				provider := &mockProvider{startEvents: []transport.TransportEvent{event}}
				service := NewService(&memorySessionRepo{}, &memoryLeaseManager{}, newMemoryRunManager(), &memoryTranscriptStore{}, provider, Config{Tools: tools, ApprovalChecker: tc.checker}, nil)
				_, err := service.Execute(context.Background(), ingress.InputEvent{EventID: "event-1", EventType: runv1.InputEventType_INPUT_EVENT_TYPE_USER_MESSAGE, SessionKey: "web:1", Source: "web", PayloadJSON: `{"text":"fetch"}`, CreatedAt: time.Now().UTC(), IdempotencyKey: "event-1"})
				if err == nil || !strings.Contains(err.Error(), tc.want) {
					t.Fatalf("batch=%v: expected %q, got %v", batch, tc.want, err)
				}
				if len(tools.calls) != 0 || len(provider.submittedResults) != 0 {
					t.Fatalf("batch=%v: unauthorized tool executed or resumed model", batch)
				}
			}
		})
	}
}
