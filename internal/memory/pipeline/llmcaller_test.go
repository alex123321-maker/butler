package pipeline

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestOpenAICallerComplete(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			t.Errorf("expected POST, got %s", r.Method)
		}
		if r.URL.Path != "/chat/completions" {
			t.Errorf("expected /chat/completions, got %s", r.URL.Path)
		}
		if auth := r.Header.Get("Authorization"); auth != "Bearer test-key" {
			t.Errorf("expected Bearer test-key, got %s", auth)
		}

		var req chatCompletionRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			t.Fatalf("decode request: %v", err)
		}
		if req.Model != "gpt-4o-mini" {
			t.Errorf("expected model gpt-4o-mini, got %s", req.Model)
		}
		if len(req.Messages) != 2 {
			t.Fatalf("expected 2 messages, got %d", len(req.Messages))
		}
		if req.Messages[0].Role != "system" {
			t.Errorf("expected system role, got %s", req.Messages[0].Role)
		}
		if req.Messages[1].Role != "user" {
			t.Errorf("expected user role, got %s", req.Messages[1].Role)
		}

		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(chatCompletionResponse{
			Choices: []chatChoice{
				{Message: chatMessage{Role: "assistant", Content: `{"profile_updates":[],"episodes":[],"session_summary":"test summary"}`}},
			},
		})
	}))
	defer server.Close()

	caller, err := NewOpenAICaller(OpenAICallerConfig{
		APIKey:  "test-key",
		Model:   "gpt-4o-mini",
		BaseURL: server.URL,
	}, server.Client())
	if err != nil {
		t.Fatalf("new caller: %v", err)
	}

	result, err := caller.Complete(context.Background(), "system prompt", "user prompt")
	if err != nil {
		t.Fatalf("complete: %v", err)
	}
	if result == "" {
		t.Fatal("expected non-empty result")
	}
}

func TestOpenAICallerHandlesAPIError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
		_ = json.NewEncoder(w).Encode(chatCompletionResponse{
			Error: &chatError{Message: "server error", Type: "server_error"},
		})
	}))
	defer server.Close()

	caller, err := NewOpenAICaller(OpenAICallerConfig{
		APIKey:  "test-key",
		BaseURL: server.URL,
	}, server.Client())
	if err != nil {
		t.Fatalf("new caller: %v", err)
	}

	_, err = caller.Complete(context.Background(), "system", "user")
	if err == nil {
		t.Fatal("expected error for API error response")
	}
}

func TestOpenAICallerRequiresAPIKey(t *testing.T) {
	_, err := NewOpenAICaller(OpenAICallerConfig{APIKey: ""}, nil)
	if err == nil {
		t.Fatal("expected error for empty API key")
	}
}

func TestOpenAICallerDefaultsModel(t *testing.T) {
	caller, err := NewOpenAICaller(OpenAICallerConfig{APIKey: "test-key"}, nil)
	if err != nil {
		t.Fatalf("new caller: %v", err)
	}
	if caller.model != "gpt-4o-mini" {
		t.Errorf("expected default model gpt-4o-mini, got %s", caller.model)
	}
}
