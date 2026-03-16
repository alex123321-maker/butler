package openaicodex

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"reflect"
	"testing"
	"time"

	"github.com/butler/butler/internal/providerauth"
	"github.com/butler/butler/internal/transport"
)

type staticAuth struct{}

func (staticAuth) ResolveOpenAICodex(context.Context) (providerauth.OpenAICodexAuth, error) {
	return providerauth.OpenAICodexAuth{AccessToken: "codex-token", AccountID: "acc_123", ExpiresAt: time.Now().Add(time.Hour)}, nil
}

func TestStartRunAppliesCodexAuthAndStreamsEvents(t *testing.T) {
	t.Parallel()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/codex/responses" {
			t.Fatalf("unexpected path %s", r.URL.Path)
		}
		if got := r.Header.Get("Authorization"); got != "Bearer codex-token" {
			t.Fatalf("unexpected authorization %q", got)
		}
		if got := r.Header.Get("chatgpt-account-id"); got != "acc_123" {
			t.Fatalf("unexpected account id %q", got)
		}
		if got := r.Header.Get("conversation_id"); got != "telegram:chat:1" {
			t.Fatalf("unexpected conversation id %q", got)
		}
		w.Header().Set("Content-Type", "text/event-stream")
		_, _ = fmt.Fprint(w, "data: {\"type\":\"response.created\",\"response\":{\"id\":\"resp_1\"}}\n\n")
		_, _ = fmt.Fprint(w, "data: {\"type\":\"response.output_text.delta\",\"delta\":\"Hello\",\"sequence_number\":1}\n\n")
		_, _ = fmt.Fprint(w, "data: {\"type\":\"response.completed\",\"response\":{\"id\":\"resp_1\",\"status\":\"completed\",\"output_text\":\"Hello\",\"usage\":{\"input_tokens\":2,\"output_tokens\":3,\"total_tokens\":5}}}\n\n")
	}))
	defer server.Close()

	provider, err := NewProvider(Config{Model: "gpt-5.1-codex", BaseURL: server.URL, Timeout: 5 * time.Second, AuthSource: staticAuth{}}, server.Client())
	if err != nil {
		t.Fatalf("NewProvider returned error: %v", err)
	}
	stream, err := provider.StartRun(context.Background(), transport.StartRunRequest{Context: transport.TransportRunContext{RunID: "run-1", SessionKey: "telegram:chat:1", ProviderName: providerName, ModelName: "gpt-5.1-codex"}, InputItems: []transport.InputItem{{Role: "user", Content: "hello"}}, StreamingEnabled: true})
	if err != nil {
		t.Fatalf("StartRun returned error: %v", err)
	}
	var eventTypes []transport.EventType
	for event := range stream {
		eventTypes = append(eventTypes, event.EventType)
	}
	want := []transport.EventType{transport.EventTypeRunStarted, transport.EventTypeProviderSessionBound, transport.EventTypeAssistantDelta, transport.EventTypeAssistantFinal, transport.EventTypeRunCompleted}
	if !reflect.DeepEqual(eventTypes, want) {
		t.Fatalf("unexpected event types: got %v want %v", eventTypes, want)
	}
}

func TestSubmitToolResultUsesPreviousResponseID(t *testing.T) {
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
		_, _ = fmt.Fprint(w, "data: {\"type\":\"response.completed\",\"response\":{\"id\":\"resp_2\",\"status\":\"completed\",\"output_text\":\"done\"}}\n\n")
	}))
	defer server.Close()

	provider, err := NewProvider(Config{Model: "gpt-5.1-codex", BaseURL: server.URL, Timeout: 5 * time.Second, AuthSource: staticAuth{}}, server.Client())
	if err != nil {
		t.Fatalf("NewProvider returned error: %v", err)
	}
	stream, err := provider.SubmitToolResult(context.Background(), transport.SubmitToolResultRequest{RunID: "run-1", ProviderSessionRef: &transport.ProviderSessionRef{ProviderName: providerName, SessionRef: "telegram:chat:1", ResponseRef: "resp_prev"}, ToolCallRef: "call_1", ToolResultJSON: `{"ok":true}`})
	if err != nil {
		t.Fatalf("SubmitToolResult returned error: %v", err)
	}
	for range stream {
	}
	payload := <-requestBody
	if payload["previous_response_id"] != "resp_prev" {
		t.Fatalf("expected previous_response_id to be forwarded, got %+v", payload)
	}
}
