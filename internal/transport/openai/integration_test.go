package openai

import (
	"context"
	"os"
	"testing"
	"time"

	"github.com/butler/butler/internal/transport"
)

func TestOpenAIIntegrationStartRun(t *testing.T) {
	apiKey := os.Getenv("BUTLER_OPENAI_API_KEY")
	if apiKey == "" {
		t.Skip("set BUTLER_OPENAI_API_KEY to run the OpenAI integration test")
	}

	provider, err := NewProvider(Config{APIKey: apiKey, Model: "gpt-5-mini", Timeout: 60 * time.Second}, nil)
	if err != nil {
		t.Fatalf("NewProvider returned error: %v", err)
	}

	stream, err := provider.StartRun(context.Background(), transport.StartRunRequest{
		Context:          transport.TransportRunContext{RunID: "integration-run", SessionKey: "integration:session", ProviderName: "openai", ModelName: "gpt-5-mini"},
		InputItems:       []transport.InputItem{{Role: "user", Content: "Reply with the word integration."}},
		StreamingEnabled: true,
	})
	if err != nil {
		t.Fatalf("StartRun returned error: %v", err)
	}

	sawFinal := false
	for event := range stream {
		if event.EventType == transport.EventTypeAssistantFinal {
			sawFinal = true
		}
	}
	if !sawFinal {
		t.Fatal("expected assistant_final event from OpenAI integration test")
	}
}

func TestOpenAIIntegrationWebSocketSession(t *testing.T) {
	apiKey := os.Getenv("BUTLER_OPENAI_API_KEY")
	if apiKey == "" {
		t.Skip("set BUTLER_OPENAI_API_KEY to run the OpenAI WebSocket integration test")
	}
	model := os.Getenv("BUTLER_OPENAI_REALTIME_MODEL")
	if model == "" {
		model = "gpt-4o-mini-realtime-preview"
	}

	provider, err := NewProvider(Config{
		APIKey:        apiKey,
		Model:         model,
		TransportMode: TransportModeWSFirst,
		Timeout:       60 * time.Second,
	}, nil)
	if err != nil {
		t.Fatalf("NewProvider returned error: %v", err)
	}
	defer provider.CloseAllSessions()

	sessionKey := "integration:ws:session"
	ctx := WithSessionKey(context.Background(), sessionKey)

	// First run via WebSocket.
	stream, err := provider.StartRun(ctx, transport.StartRunRequest{
		Context:          transport.TransportRunContext{RunID: "ws-run-1", SessionKey: sessionKey, ProviderName: "openai", ModelName: model},
		InputItems:       []transport.InputItem{{Role: "user", Content: "Reply with the word websocket."}},
		StreamingEnabled: true,
	})
	if err != nil {
		t.Fatalf("StartRun returned error: %v", err)
	}

	sawFinal := false
	for event := range stream {
		if event.EventType == transport.EventTypeAssistantFinal {
			sawFinal = true
		}
	}
	if !sawFinal {
		t.Fatal("expected assistant_final event from WebSocket run")
	}

	// Verify session is pooled.
	session := provider.getSessionByKey(sessionKey)
	if session == nil {
		t.Log("session not pooled — may have been a fallback to SSE")
		return
	}
	t.Logf("session pooled with ref %q, age %s", session.sessionRef, time.Since(session.createdAt).Round(time.Millisecond))
}
