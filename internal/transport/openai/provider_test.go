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

	provider, err := NewProvider(Config{APIKey: "test-key", Model: "gpt-5-mini", BaseURL: server.URL, Timeout: 5 * time.Second}, server.Client())
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
		transport.EventTypeAssistantFinal,
		transport.EventTypeRunCompleted,
	}
	if !reflect.DeepEqual(eventTypes, want) {
		t.Fatalf("unexpected event types: got %v want %v", eventTypes, want)
	}
	if !sawToolCall {
		t.Fatalf("expected normalized tool call event")
	}
	if finalContent != "Hello" {
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

	provider, err := NewProvider(Config{Model: "gpt-5-mini", BaseURL: server.URL}, server.Client())
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

	provider, err := NewProvider(Config{Model: "gpt-5-mini", BaseURL: server.URL}, server.Client())
	if err != nil {
		t.Fatalf("NewProvider returned error: %v", err)
	}

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

func TestCancelRunUsesResponseRefWhenPresent(t *testing.T) {
	t.Parallel()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/responses/resp_123/cancel" {
			t.Fatalf("unexpected cancel path %s", r.URL.Path)
		}
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	provider, err := NewProvider(Config{Model: "gpt-5-mini", BaseURL: server.URL}, server.Client())
	if err != nil {
		t.Fatalf("NewProvider returned error: %v", err)
	}

	event, err := provider.CancelRun(context.Background(), transport.CancelRunRequest{RunID: "run-123", ProviderSessionRef: &transport.ProviderSessionRef{ResponseRef: "resp_123"}})
	if err != nil {
		t.Fatalf("CancelRun returned error: %v", err)
	}
	if event.EventType != transport.EventTypeRunCancelled {
		t.Fatalf("expected run_cancelled event, got %s", event.EventType)
	}
}
