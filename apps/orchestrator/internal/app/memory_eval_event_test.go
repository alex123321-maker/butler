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
	HTTPStatus   int                       `json:"http_status,omitempty"`
	ErrorReason  string                    `json:"error_reason,omitempty"`
}

func memoryEvalTrace(event transport.TransportEvent) memoryEvalEvent {
	reason := ""
	if event.TransportError != nil && event.TransportError.Type == transport.ErrorTypeProviderProtocolError {
		// Exact literals owned by the transport, never a substring or raw text.
		switch event.TransportError.Message {
		case "Antigravity rejected the model request":
			reason = "upstream_request_rejected"
		case "Antigravity returned a non-SSE response":
			reason = "non_sse_response"
		case "Antigravity stream ended without a complete terminal response", "stream ended before a terminal candidate":
			reason = "incomplete_stream"
		case "invalid or conflicting observed model version":
			reason = "model_version_conflict"
		case "model response was blocked or failed":
			reason = "model_response_blocked"
		case "model returned no usable text or function":
			reason = "unusable_model_output"
		}
	}
	if fixed := transport.DiagnosticReason(event.TransportError); fixed != "" {
		reason = fixed
	}
	return memoryEvalEvent{ID: event.EventID, Run: event.RunID, Provider: event.ProviderName, Time: event.Timestamp, Type: string(event.EventType), Version: event.ProviderModelVersion, Usage: event.Usage, Final: event.AssistantFinal, ToolProposed: event.ToolCall != nil || event.ToolCallBatch != nil, ErrorCode: transport.DiagnosticCode(event.TransportError), HTTPStatus: transport.DiagnosticHTTPStatus(event.TransportError), ErrorReason: reason}
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
	for _, status := range []int{400, 999} {
		e := transport.TransportEvent{TransportError: &transport.Error{Type: transport.ErrorTypeProviderProtocolError, HTTPStatus: status, Message: "Antigravity rejected the model request", ProviderDetails: map[string]any{"PRIVATE": "SECRET"}}}
		trace := memoryEvalTrace(e)
		if trace.ErrorReason != "upstream_request_rejected" || (status == 400 && trace.HTTPStatus != 400) || (status == 999 && trace.HTTPStatus != 0) {
			t.Fatal("safe response classification differs")
		}
	}
}
