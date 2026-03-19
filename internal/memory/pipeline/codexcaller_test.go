package pipeline

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/butler/butler/internal/providerauth"
)

type codexTestAuth struct{}

func (codexTestAuth) ResolveOpenAICodex(context.Context) (providerauth.OpenAICodexAuth, error) {
	return providerauth.OpenAICodexAuth{AccessToken: "codex-token", AccountID: "acc_123", ExpiresAt: time.Now().Add(time.Hour)}, nil
}

func TestOpenAICodexCallerComplete(t *testing.T) {
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
		w.Header().Set("Content-Type", "text/event-stream")
		_, _ = fmt.Fprint(w, "data: {\"type\":\"response.output_text.delta\",\"delta\":\"{\\\"session_summary\\\":\\\"Tomsk pizza context\\\"}\"}\n\n")
		_, _ = fmt.Fprint(w, "data: {\"type\":\"response.completed\",\"response\":{\"status\":\"completed\",\"output_text\":\"{\\\"session_summary\\\":\\\"Tomsk pizza context\\\"}\"}}\n\n")
	}))
	defer server.Close()

	caller, err := NewOpenAICodexCaller(OpenAICodexCallerConfig{Model: "gpt-5.1-codex", BaseURL: server.URL, AuthSource: codexTestAuth{}}, server.Client())
	if err != nil {
		t.Fatalf("NewOpenAICodexCaller returned error: %v", err)
	}
	result, err := caller.Complete(context.Background(), "system", "user")
	if err != nil {
		t.Fatalf("Complete returned error: %v", err)
	}
	if result != `{"session_summary":"Tomsk pizza context"}` {
		t.Fatalf("unexpected result %q", result)
	}
}
