package githubcopilot

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"

	"github.com/butler/butler/internal/providerauth"
	"github.com/butler/butler/internal/transport"
)

type staticCopilotAuth struct{ baseURL string }

func (a staticCopilotAuth) ResolveGitHubCopilot(context.Context) (providerauth.GitHubCopilotAuth, error) {
	return providerauth.GitHubCopilotAuth{AccessToken: "copilot-token", BaseURL: a.baseURL, ExpiresAt: time.Now().Add(time.Hour)}, nil
}

func TestSequentialToolCallsQueueLocallyBeforeResume(t *testing.T) {
	t.Parallel()
	var requests atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		current := requests.Add(1)
		w.Header().Set("Content-Type", "text/event-stream")
		switch current {
		case 1:
			_, _ = fmt.Fprint(w, "data: {\"choices\":[{\"delta\":{\"tool_calls\":[{\"index\":0,\"id\":\"call_1\",\"type\":\"function\",\"function\":{\"name\":\"first\",\"arguments\":\"{\\\"value\\\":1}\"}}]}}]}\n\n")
			_, _ = fmt.Fprint(w, "data: {\"choices\":[{\"delta\":{\"tool_calls\":[{\"index\":1,\"id\":\"call_2\",\"type\":\"function\",\"function\":{\"name\":\"second\",\"arguments\":\"{\\\"value\\\":2}\"}}]}}]}\n\n")
			_, _ = fmt.Fprint(w, "data: {\"choices\":[{\"finish_reason\":\"tool_calls\"}]}\n\n")
		case 2:
			_, _ = fmt.Fprint(w, "data: {\"choices\":[{\"delta\":{\"content\":\"done\"}}],\"usage\":{\"prompt_tokens\":2,\"completion_tokens\":1,\"total_tokens\":3}}\n\n")
			_, _ = fmt.Fprint(w, "data: {\"choices\":[{\"finish_reason\":\"stop\"}]}\n\n")
		default:
			t.Fatalf("unexpected request count %d", current)
		}
	}))
	defer server.Close()

	provider, err := NewProvider(Config{Model: "gpt-4o", Timeout: 5 * time.Second, AuthSource: staticCopilotAuth{baseURL: server.URL}}, server.Client())
	if err != nil {
		t.Fatalf("NewProvider returned error: %v", err)
	}
	startStream, err := provider.StartRun(context.Background(), transport.StartRunRequest{Context: transport.TransportRunContext{RunID: "run-1", SessionKey: "telegram:chat:1", ProviderName: providerName, ModelName: "gpt-4o"}, InputItems: []transport.InputItem{{Role: "user", Content: "hello"}}, ToolDefinitions: []transport.ToolDefinition{{Name: "first", SchemaJSON: `{"type":"object"}`}, {Name: "second", SchemaJSON: `{"type":"object"}`}}, StreamingEnabled: true})
	if err != nil {
		t.Fatalf("StartRun returned error: %v", err)
	}
	firstToolCall := readSingleToolCall(t, startStream)
	if firstToolCall.ToolName != "first" {
		t.Fatalf("expected first tool call, got %+v", firstToolCall)
	}
	if requests.Load() != 1 {
		t.Fatalf("expected one upstream request after first tool call, got %d", requests.Load())
	}

	secondStream, err := provider.SubmitToolResult(context.Background(), transport.SubmitToolResultRequest{RunID: "run-1", ProviderSessionRef: &transport.ProviderSessionRef{ProviderName: providerName, SessionRef: "telegram:chat:1"}, ToolCallRef: "call_1", ToolResultJSON: `{"ok":true}`})
	if err != nil {
		t.Fatalf("SubmitToolResult(first) returned error: %v", err)
	}
	secondToolCall := readSingleToolCall(t, secondStream)
	if secondToolCall.ToolName != "second" {
		t.Fatalf("expected second tool call, got %+v", secondToolCall)
	}
	if requests.Load() != 1 {
		t.Fatalf("expected second tool call to be emitted locally, got %d upstream requests", requests.Load())
	}

	finalStream, err := provider.SubmitToolResult(context.Background(), transport.SubmitToolResultRequest{RunID: "run-1", ProviderSessionRef: &transport.ProviderSessionRef{ProviderName: providerName, SessionRef: "telegram:chat:1"}, ToolCallRef: "call_2", ToolResultJSON: `{"ok":true}`})
	if err != nil {
		t.Fatalf("SubmitToolResult(second) returned error: %v", err)
	}
	final := readAssistantFinal(t, finalStream)
	if final != "done" {
		t.Fatalf("expected final assistant response, got %q", final)
	}
	if requests.Load() != 2 {
		t.Fatalf("expected final resume to trigger second upstream request, got %d", requests.Load())
	}
}

func readSingleToolCall(t *testing.T, stream transport.EventStream) transport.ToolCallRequest {
	t.Helper()
	for event := range stream {
		if event.ToolCall != nil {
			return *event.ToolCall
		}
	}
	t.Fatal("expected tool call event")
	return transport.ToolCallRequest{}
}

func readAssistantFinal(t *testing.T, stream transport.EventStream) string {
	t.Helper()
	for event := range stream {
		if event.AssistantFinal != nil {
			return event.AssistantFinal.Content
		}
	}
	t.Fatal("expected assistant final event")
	return ""
}
