package app

import (
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/butler/butler/internal/transport"
)

// Only permitted output/measurement metadata is retained, never raw HTTP,
// credential envelopes, headers or transport error payloads.
type memoryEvalEvent struct {
	ID           string                    `json:"event_id"`
	Run          string                    `json:"run_id"`
	Provider     string                    `json:"provider"`
	Time         time.Time                 `json:"timestamp"`
	Type         string                    `json:"type"`
	Version      *string                   `json:"version"`
	Usage        *transport.Usage          `json:"usage"`
	Final        *transport.AssistantFinal `json:"final"`
	ToolProposed bool                      `json:"tool_proposed"`
	ErrorCode    string                    `json:"error_code,omitempty"`
}

func memoryEvalTrace(event transport.TransportEvent) memoryEvalEvent {
	return memoryEvalEvent{ID: event.EventID, Run: event.RunID, Provider: event.ProviderName, Time: event.Timestamp, Type: string(event.EventType), Version: event.ProviderModelVersion, Usage: event.Usage, Final: event.AssistantFinal, ToolProposed: event.ToolCall != nil || event.ToolCallBatch != nil, ErrorCode: transport.DiagnosticCode(event.TransportError)}
}

func TestMemoryEvalCollectorSafeErrorProjection(t *testing.T) {
	for _, kind := range []transport.ErrorType{transport.ErrorTypeProviderProtocolError, "PRIVATE_UNKNOWN_TYPE"} {
		e := transport.TransportEvent{TransportError: &transport.Error{Type: kind, Message: "PRIVATE_MESSAGE", ProviderCode: "PRIVATE_CODE", ProviderDetails: map[string]any{"PRIVATE_DETAIL": "PRIVATE_VALUE"}}}
		trace := memoryEvalTrace(e)
		raw, err := json.Marshal(trace)
		if err != nil || strings.Contains(string(raw), "PRIVATE_") || trace.ErrorCode == "" {
			t.Fatal("safe trace lost classification or exported private error material")
		}
		if kind == transport.ErrorTypeProviderProtocolError && trace.ErrorCode != string(kind) {
			t.Fatal("fixed error class lost")
		}
	}
}
