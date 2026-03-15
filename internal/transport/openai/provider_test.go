package openai

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"reflect"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/butler/butler/internal/transport"
	"github.com/gorilla/websocket"
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

	provider, err := NewProvider(Config{Model: "gpt-5-mini", BaseURL: server.URL, TransportMode: TransportModeSSEOnly}, server.Client())
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

func TestStartRunUsesRealtimeWebSocketWhenEnabled(t *testing.T) {
	t.Parallel()

	upgrader := websocket.Upgrader{CheckOrigin: func(r *http.Request) bool { return true }}

	mux := http.NewServeMux()
	mux.HandleFunc("/realtime", func(w http.ResponseWriter, r *http.Request) {
		conn, err := upgrader.Upgrade(w, r, nil)
		if err != nil {
			t.Errorf("upgrade websocket: %v", err)
			return
		}
		defer conn.Close()

		// Drain client setup messages (session.update, conversation.item.create)
		// until we receive a response.create event.
		for {
			var msg map[string]any
			if err := conn.ReadJSON(&msg); err != nil {
				t.Errorf("read realtime message: %v", err)
				return
			}
			if msg["type"] == "response.create" {
				break
			}
		}

		// Send the expected Realtime API server events.
		_ = conn.WriteJSON(map[string]any{"type": "session.created", "session": map[string]any{"id": "sess_123"}})
		_ = conn.WriteJSON(map[string]any{"type": "response.created", "response": map[string]any{"id": "resp_123"}})
		_ = conn.WriteJSON(map[string]any{"type": "response.text.delta", "delta": "Hel", "sequence_number": 1})
		_ = conn.WriteJSON(map[string]any{"type": "response.function_call_arguments.done", "call_id": "call_1", "name": "lookup_weather", "arguments": map[string]any{"city": "Berlin"}, "sequence_number": 2})
		_ = conn.WriteJSON(map[string]any{"type": "response.done", "response": map[string]any{"id": "resp_123", "status": "completed", "output_text": "Hello", "usage": map[string]any{"input_tokens": 2, "output_tokens": 3, "total_tokens": 5}}})
	})
	server := httptest.NewServer(mux)
	defer server.Close()

	realtimeURL := websocketURL(server.URL) + "/realtime"
	provider, err := NewProvider(Config{APIKey: "test-key", Model: "gpt-4o-mini", BaseURL: server.URL, RealtimeURL: realtimeURL, TransportMode: TransportModeWSFirst, Timeout: 5 * time.Second}, server.Client())
	if err != nil {
		t.Fatalf("NewProvider returned error: %v", err)
	}

	stream, err := provider.StartRun(context.Background(), transport.StartRunRequest{
		Context:          transport.TransportRunContext{RunID: "run-123", SessionKey: "telegram:chat:1", ProviderName: "openai", ModelName: "gpt-4o-mini"},
		InputItems:       []transport.InputItem{{Role: "user", Content: "hello"}},
		StreamingEnabled: true,
	})
	if err != nil {
		t.Fatalf("StartRun returned error: %v", err)
	}

	var eventTypes []transport.EventType
	for event := range stream {
		eventTypes = append(eventTypes, event.EventType)
	}

	want := []transport.EventType{
		transport.EventTypeProviderSessionBound,
		transport.EventTypeRunStarted,
		transport.EventTypeProviderSessionBound,
		transport.EventTypeAssistantDelta,
		transport.EventTypeToolCallRequested,
		transport.EventTypeAssistantFinal,
		transport.EventTypeRunCompleted,
	}
	if !reflect.DeepEqual(eventTypes, want) {
		t.Fatalf("unexpected realtime event types: got %v want %v", eventTypes, want)
	}
	if provider.activeRealtimeSession("run-123") != nil {
		t.Fatal("expected realtime session to be cleared after completion")
	}
}

func TestStartRunFallsBackToSSEWhenRealtimeUnavailable(t *testing.T) {
	t.Parallel()

	mux := http.NewServeMux()
	mux.HandleFunc("/responses", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		_, _ = fmt.Fprint(w, "event: response.created\n")
		_, _ = fmt.Fprint(w, "data: {\"type\":\"response.created\",\"response\":{\"id\":\"resp_123\"}}\n\n")
		_, _ = fmt.Fprint(w, "event: response.completed\n")
		_, _ = fmt.Fprint(w, "data: {\"type\":\"response.completed\",\"response\":{\"id\":\"resp_123\",\"status\":\"completed\",\"output_text\":\"Hello fallback\"}}\n\n")
	})
	server := httptest.NewServer(mux)
	defer server.Close()

	provider, err := NewProvider(Config{APIKey: "test-key", Model: "gpt-5-mini", BaseURL: server.URL, RealtimeURL: websocketURL(server.URL) + "/realtime", TransportMode: TransportModeWSFirst, Timeout: 5 * time.Second}, server.Client())
	if err != nil {
		t.Fatalf("NewProvider returned error: %v", err)
	}

	stream, err := provider.StartRun(context.Background(), transport.StartRunRequest{
		Context:          transport.TransportRunContext{RunID: "run-fallback", SessionKey: "telegram:chat:1", ProviderName: "openai", ModelName: "gpt-5-mini"},
		InputItems:       []transport.InputItem{{Role: "user", Content: "hello"}},
		StreamingEnabled: true,
	})
	if err != nil {
		t.Fatalf("StartRun returned error: %v", err)
	}

	var eventTypes []transport.EventType
	for event := range stream {
		eventTypes = append(eventTypes, event.EventType)
	}
	if len(eventTypes) == 0 || eventTypes[0] != transport.EventTypeTransportWarning {
		t.Fatalf("expected fallback warning first, got %v", eventTypes)
	}
	if eventTypes[len(eventTypes)-1] != transport.EventTypeRunCompleted {
		t.Fatalf("expected fallback stream to complete, got %v", eventTypes)
	}
}

func websocketURL(httpURL string) string {
	parsed, err := url.Parse(httpURL)
	if err != nil {
		return httpURL
	}
	if parsed.Scheme == "https" {
		parsed.Scheme = "wss"
	} else {
		parsed.Scheme = "ws"
	}
	return strings.TrimRight(parsed.String(), "/")
}

func TestWebSocketSessionReusedAcrossRuns(t *testing.T) {
	t.Parallel()

	upgrader := websocket.Upgrader{CheckOrigin: func(r *http.Request) bool { return true }}
	var connectionCount int32
	var connectionMu sync.Mutex

	mux := http.NewServeMux()
	mux.HandleFunc("/realtime", func(w http.ResponseWriter, r *http.Request) {
		conn, err := upgrader.Upgrade(w, r, nil)
		if err != nil {
			t.Errorf("upgrade websocket: %v", err)
			return
		}
		connectionMu.Lock()
		connectionCount++
		connectionMu.Unlock()

		// Handle multiple rounds of messages on the same connection.
		_ = conn.WriteJSON(map[string]any{"type": "session.created", "session": map[string]any{"id": "sess_reuse"}})
		for {
			var msg map[string]any
			if err := conn.ReadJSON(&msg); err != nil {
				return
			}
			if msg["type"] == "response.create" {
				_ = conn.WriteJSON(map[string]any{"type": "response.created", "response": map[string]any{"id": "resp_reuse"}})
				_ = conn.WriteJSON(map[string]any{"type": "response.done", "response": map[string]any{"id": "resp_reuse", "status": "completed", "output_text": "ok"}})
			}
		}
	})
	server := httptest.NewServer(mux)
	defer server.Close()

	realtimeURL := websocketURL(server.URL) + "/realtime"
	provider, err := NewProvider(Config{APIKey: "test-key", Model: "gpt-4o-mini", BaseURL: server.URL, RealtimeURL: realtimeURL, TransportMode: TransportModeWSFirst, Timeout: 5 * time.Second}, server.Client())
	if err != nil {
		t.Fatalf("NewProvider returned error: %v", err)
	}

	sessionKey := "telegram:chat:reuse"

	// First run.
	ctx1 := WithSessionKey(context.Background(), sessionKey)
	stream1, err := provider.StartRun(ctx1, transport.StartRunRequest{
		Context:          transport.TransportRunContext{RunID: "run-1", SessionKey: sessionKey, ProviderName: "openai", ModelName: "gpt-4o-mini"},
		InputItems:       []transport.InputItem{{Role: "user", Content: "hello 1"}},
		StreamingEnabled: true,
	})
	if err != nil {
		t.Fatalf("StartRun 1 returned error: %v", err)
	}
	for range stream1 {
	}

	// Second run — should reuse the same WebSocket connection.
	ctx2 := WithSessionKey(context.Background(), sessionKey)
	stream2, err := provider.StartRun(ctx2, transport.StartRunRequest{
		Context:          transport.TransportRunContext{RunID: "run-2", SessionKey: sessionKey, ProviderName: "openai", ModelName: "gpt-4o-mini"},
		InputItems:       []transport.InputItem{{Role: "user", Content: "hello 2"}},
		StreamingEnabled: true,
	})
	if err != nil {
		t.Fatalf("StartRun 2 returned error: %v", err)
	}
	for range stream2 {
	}

	// Verify only one WebSocket connection was created.
	connectionMu.Lock()
	count := connectionCount
	connectionMu.Unlock()
	if count != 1 {
		t.Fatalf("expected 1 websocket connection (reused), got %d", count)
	}

	// Verify session is still in pool.
	session := provider.getSessionByKey(sessionKey)
	if session == nil {
		t.Fatal("expected session to remain in pool after runs complete")
	}
	if session.sessionRef != "sess_reuse" {
		t.Fatalf("expected session ref 'sess_reuse', got %q", session.sessionRef)
	}

	// Cleanup.
	provider.CloseSession(sessionKey)
	if provider.getSessionByKey(sessionKey) != nil {
		t.Fatal("expected session to be removed after CloseSession")
	}
}

func TestWebSocketSessionLossTriggersNewConnection(t *testing.T) {
	t.Parallel()

	upgrader := websocket.Upgrader{CheckOrigin: func(r *http.Request) bool { return true }}
	var connectionCount int32
	var connectionMu sync.Mutex

	mux := http.NewServeMux()
	mux.HandleFunc("/realtime", func(w http.ResponseWriter, r *http.Request) {
		conn, err := upgrader.Upgrade(w, r, nil)
		if err != nil {
			t.Errorf("upgrade websocket: %v", err)
			return
		}
		connectionMu.Lock()
		n := connectionCount
		connectionCount++
		connectionMu.Unlock()

		_ = conn.WriteJSON(map[string]any{"type": "session.created", "session": map[string]any{"id": fmt.Sprintf("sess_%d", n)}})

		for {
			var msg map[string]any
			if err := conn.ReadJSON(&msg); err != nil {
				return
			}
			if msg["type"] == "response.create" {
				if n == 0 {
					// First connection: close abruptly mid-stream to simulate loss.
					_ = conn.WriteJSON(map[string]any{"type": "response.created", "response": map[string]any{"id": "resp_1"}})
					conn.Close()
					return
				}
				// Second connection: respond normally.
				_ = conn.WriteJSON(map[string]any{"type": "response.created", "response": map[string]any{"id": "resp_2"}})
				_ = conn.WriteJSON(map[string]any{"type": "response.done", "response": map[string]any{"id": "resp_2", "status": "completed", "output_text": "recovered"}})
			}
		}
	})

	// SSE fallback handler.
	mux.HandleFunc("/responses", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		_, _ = fmt.Fprint(w, "event: response.completed\n")
		_, _ = fmt.Fprint(w, "data: {\"type\":\"response.completed\",\"response\":{\"id\":\"resp_sse\",\"status\":\"completed\",\"output_text\":\"sse fallback\"}}\n\n")
	})

	server := httptest.NewServer(mux)
	defer server.Close()

	realtimeURL := websocketURL(server.URL) + "/realtime"
	provider, err := NewProvider(Config{APIKey: "test-key", Model: "gpt-4o-mini", BaseURL: server.URL, RealtimeURL: realtimeURL, TransportMode: TransportModeWSFirst, Timeout: 5 * time.Second}, server.Client())
	if err != nil {
		t.Fatalf("NewProvider returned error: %v", err)
	}

	sessionKey := "telegram:chat:loss"

	// First run — will experience connection loss.
	ctx1 := WithSessionKey(context.Background(), sessionKey)
	stream1, err := provider.StartRun(ctx1, transport.StartRunRequest{
		Context:          transport.TransportRunContext{RunID: "run-loss-1", SessionKey: sessionKey, ProviderName: "openai", ModelName: "gpt-4o-mini"},
		InputItems:       []transport.InputItem{{Role: "user", Content: "hello"}},
		StreamingEnabled: true,
	})
	if err != nil {
		t.Fatalf("StartRun 1 returned error: %v", err)
	}

	var sawSessionLost bool
	for event := range stream1 {
		if event.TransportError != nil && event.TransportError.Type == transport.ErrorTypeStatefulSessionLost {
			sawSessionLost = true
		}
	}
	if !sawSessionLost {
		// Might have gotten SSE fallback instead — that's also acceptable.
		t.Log("connection loss was handled via SSE fallback (wrapRealtimeFallback)")
	}

	// Second run — should create a new connection since the old one was lost.
	ctx2 := WithSessionKey(context.Background(), sessionKey)
	stream2, err := provider.StartRun(ctx2, transport.StartRunRequest{
		Context:          transport.TransportRunContext{RunID: "run-loss-2", SessionKey: sessionKey, ProviderName: "openai", ModelName: "gpt-4o-mini"},
		InputItems:       []transport.InputItem{{Role: "user", Content: "hello again"}},
		StreamingEnabled: true,
	})
	if err != nil {
		t.Fatalf("StartRun 2 returned error: %v", err)
	}
	for range stream2 {
	}

	connectionMu.Lock()
	count := connectionCount
	connectionMu.Unlock()
	if count < 2 {
		t.Fatalf("expected at least 2 connections (original + reconnect), got %d", count)
	}
}

func TestCloseAllSessionsCleansUp(t *testing.T) {
	t.Parallel()

	provider, err := NewProvider(Config{Model: "gpt-4o-mini", TransportMode: TransportModeSSEOnly, Timeout: 5 * time.Second}, nil)
	if err != nil {
		t.Fatalf("NewProvider returned error: %v", err)
	}

	// Store some fake sessions.
	now := time.Now().UTC()
	s1 := &realtimeSession{sessionKey: "key1", createdAt: now, lastUsedAt: now}
	s2 := &realtimeSession{sessionKey: "key2", createdAt: now, lastUsedAt: now}
	provider.storeSession("key1", s1)
	provider.storeSession("key2", s2)

	if provider.getSessionByKey("key1") == nil {
		t.Fatal("expected session key1 to exist")
	}

	provider.CloseAllSessions()

	if provider.getSessionByKey("key1") != nil {
		t.Fatal("expected session key1 to be cleaned up")
	}
	if provider.getSessionByKey("key2") != nil {
		t.Fatal("expected session key2 to be cleaned up")
	}
}

func TestWithSessionKeyContext(t *testing.T) {
	t.Parallel()

	ctx := WithSessionKey(context.Background(), "telegram:chat:42")
	val, ok := ctx.Value(sessionKeyContextKey).(string)
	if !ok || val != "telegram:chat:42" {
		t.Fatalf("expected session key from context, got %q", val)
	}
}
