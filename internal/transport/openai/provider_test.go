package openai

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"reflect"
	"testing"
	"time"

	"github.com/butler/butler/internal/transport"
)

func TestStartRunStreamsNormalizedEvents(t *testing.T) {
	t.Parallel()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/responses" {
			t.Fatalf("unexpected path %s", r.URL.Path)
		}
		if got := r.Header.Get("Authorization"); got != "Bearer test-key" {
			t.Fatalf("unexpected auth header %q", got)
		}
		w.Header().Set("Content-Type", "text/event-stream")
		_, _ = fmt.Fprint(w, "event: response.created\n")
		_, _ = fmt.Fprint(w, "data: {\"type\":\"response.created\",\"response\":{\"id\":\"resp_123\"}}\n\n")
		_, _ = fmt.Fprint(w, "event: response.output_text.delta\n")
		_, _ = fmt.Fprint(w, "data: {\"type\":\"response.output_text.delta\",\"delta\":\"Hel\",\"sequence_number\":1}\n\n")
		_, _ = fmt.Fprint(w, "event: response.output_item.done\n")
		_, _ = fmt.Fprint(w, "data: {\"type\":\"response.output_item.done\",\"sequence_number\":2,\"item\":{\"type\":\"function_call\",\"call_id\":\"call_1\",\"name\":\"lookup_weather\",\"arguments\":\"{\\\"city\\\":\\\"Berlin\\\"}\"}}\n\n")
		_, _ = fmt.Fprint(w, "event: response.completed\n")
		_, _ = fmt.Fprint(w, "data: {\"type\":\"response.completed\",\"response\":{\"id\":\"resp_123\",\"status\":\"completed\",\"output_text\":\"Hello\",\"usage\":{\"input_tokens\":2,\"output_tokens\":3,\"total_tokens\":5}}}\n\n")
	}))
	defer server.Close()

	provider, err := NewProvider(Config{APIKey: "test-key", Model: "gpt-5-mini", BaseURL: server.URL, Timeout: 5 * time.Second, TransportMode: TransportModeSSEOnly}, server.Client())
	if err != nil {
		t.Fatalf("NewProvider returned error: %v", err)
	}

	stream, err := provider.StartRun(context.Background(), transport.StartRunRequest{
		Context:          transport.TransportRunContext{RunID: "run-123", SessionKey: "telegram:chat:1", ProviderName: "openai", ModelName: "gpt-5-mini"},
		InputItems:       []transport.InputItem{{Role: "user", Content: "hello"}},
		ToolDefinitions:  []transport.ToolDefinition{{Name: "lookup_weather", Description: "lookup weather", SchemaJSON: `{"type":"object"}`}},
		StreamingEnabled: true,
	})
	if err != nil {
		t.Fatalf("StartRun returned error: %v", err)
	}

	var eventTypes []transport.EventType
	var finalContent string
	var sawToolCall bool
	for event := range stream {
		eventTypes = append(eventTypes, event.EventType)
		if event.AssistantFinal != nil {
			finalContent = event.AssistantFinal.Content
		}
		if event.ToolCall != nil {
			sawToolCall = true
		}
	}

	want := []transport.EventType{
		transport.EventTypeRunStarted,
		transport.EventTypeProviderSessionBound,
		transport.EventTypeAssistantDelta,
		transport.EventTypeToolCallRequested,
		transport.EventTypeRunCompleted,
	}
	if !reflect.DeepEqual(eventTypes, want) {
		t.Fatalf("unexpected event types: got %v want %v", eventTypes, want)
	}
	if !sawToolCall {
		t.Fatalf("expected normalized tool call event")
	}
	if finalContent != "" {
		t.Fatalf("expected final content, got %q", finalContent)
	}
}

func TestStartRunMapsHTTPErrors(t *testing.T) {
	t.Parallel()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusTooManyRequests)
		_ = json.NewEncoder(w).Encode(map[string]any{"error": map[string]any{"message": "too many requests", "code": "rate_limit_exceeded"}})
	}))
	defer server.Close()

	provider, err := NewProvider(Config{Model: "gpt-5-mini", BaseURL: server.URL, TransportMode: TransportModeSSEOnly}, server.Client())
	if err != nil {
		t.Fatalf("NewProvider returned error: %v", err)
	}

	_, err = provider.StartRun(context.Background(), transport.StartRunRequest{
		Context:          transport.TransportRunContext{RunID: "run-123", SessionKey: "telegram:chat:1", ProviderName: "openai", ModelName: "gpt-5-mini"},
		StreamingEnabled: true,
	})
	if err == nil {
		t.Fatal("expected start run error")
	}
	normalized, ok := err.(*transport.Error)
	if !ok {
		t.Fatalf("expected normalized transport error, got %T", err)
	}
	if normalized.Type != transport.ErrorTypeRateLimited {
		t.Fatalf("expected rate limited error, got %s", normalized.Type)
	}
}

func TestSubmitToolResultUsesFunctionCallOutputInput(t *testing.T) {
	t.Parallel()

	requestBody := make(chan map[string]any, 1)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		defer r.Body.Close()
		var payload map[string]any
		if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
			t.Fatalf("decode request body: %v", err)
		}
		requestBody <- payload
		w.Header().Set("Content-Type", "text/event-stream")
		_, _ = fmt.Fprint(w, "event: response.completed\n")
		_, _ = fmt.Fprint(w, "data: {\"type\":\"response.completed\",\"response\":{\"id\":\"resp_456\",\"status\":\"completed\",\"output_text\":\"done\"}}\n\n")
	}))
	defer server.Close()

	provider, err := NewProvider(Config{Model: "gpt-5-mini", BaseURL: server.URL, TransportMode: TransportModeSSEOnly}, server.Client())
	if err != nil {
		t.Fatalf("NewProvider returned error: %v", err)
	}

	provider.prefixTokens["run-123"] = map[string]int64{"resp_123": 20}
	stream, err := provider.SubmitToolResult(context.Background(), transport.SubmitToolResultRequest{
		RunID:              "run-123",
		ToolCallRef:        "call_1",
		ToolResultJSON:     `{"status":"ok"}`,
		ProviderSessionRef: &transport.ProviderSessionRef{ResponseRef: "resp_123"},
	})
	if err != nil {
		t.Fatalf("SubmitToolResult returned error: %v", err)
	}
	for range stream {
	}

	payload := <-requestBody
	if payload["previous_response_id"] != "resp_123" {
		t.Fatalf("expected previous_response_id to be sent")
	}
	input, ok := payload["input"].([]any)
	if !ok || len(input) != 1 {
		t.Fatalf("expected one function_call_output item")
	}
	first, ok := input[0].(map[string]any)
	if !ok {
		t.Fatalf("expected input item object")
	}
	if first["type"] != "function_call_output" {
		t.Fatalf("expected function_call_output, got %v", first["type"])
	}
}

func TestEncodeInputItemsUsesOutputTextForAssistant(t *testing.T) {
	t.Parallel()

	items := []transport.InputItem{
		{Role: "user", Content: "hello"},
		{Role: "assistant", Content: "hi there"},
		{Role: "system", Content: "you are a bot"},
	}
	encoded := encodeInputItems(items)
	if len(encoded) != 3 {
		t.Fatalf("expected 3 encoded items, got %d", len(encoded))
	}

	// user → input_text
	userContent := encoded[0]["content"].([]map[string]any)
	if userContent[0]["type"] != "input_text" {
		t.Fatalf("expected input_text for user role, got %v", userContent[0]["type"])
	}

	// assistant → output_text
	assistantContent := encoded[1]["content"].([]map[string]any)
	if assistantContent[0]["type"] != "output_text" {
		t.Fatalf("expected output_text for assistant role, got %v", assistantContent[0]["type"])
	}

	// system → input_text
	systemContent := encoded[2]["content"].([]map[string]any)
	if systemContent[0]["type"] != "input_text" {
		t.Fatalf("expected input_text for system role, got %v", systemContent[0]["type"])
	}
}

func TestContentTypeForRole(t *testing.T) {
	t.Parallel()

	tests := []struct {
		role string
		want string
	}{
		{"user", "input_text"},
		{"assistant", "output_text"},
		{"system", "input_text"},
		{"Assistant", "output_text"},
		{"ASSISTANT", "output_text"},
		{"", "input_text"},
	}
	for _, tt := range tests {
		got := contentTypeForRole(tt.role)
		if got != tt.want {
			t.Errorf("contentTypeForRole(%q) = %q, want %q", tt.role, got, tt.want)
		}
	}
}
