package runtime

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	runtimev1 "github.com/butler/butler/internal/gen/runtime/v1"
	toolbrokerv1 "github.com/butler/butler/internal/gen/toolbroker/v1"
	"github.com/butler/butler/internal/security/egress"
	"github.com/butler/butler/internal/security/egress/egresstest"
)

type fakeProvider struct {
	name   string
	result FetchResult
	err    error
}

func (f fakeProvider) Name() string { return f.name }

func (f fakeProvider) Fetch(context.Context, string, bool) (FetchResult, error) {
	if f.err != nil {
		return FetchResult{}, f.err
	}
	return f.result, nil
}

func TestProviderChainFallsBackToNextProvider(t *testing.T) {
	t.Parallel()

	chain := NewProviderChainWithPolicy(egress.Policy{Resolver: egresstest.Resolver{}},
		fakeProvider{name: "self_hosted_primary", err: fmt.Errorf("boom")},
		fakeProvider{name: "plain_http_fallback", result: FetchResult{Provider: "plain_http_fallback", RequestedURL: "https://example.com", FinalURL: "https://example.com", StatusCode: 200, ContentText: "hello"}},
	)
	result, err := chain.Fetch(context.Background(), "https://example.com", false)
	if err != nil {
		t.Fatalf("Fetch returned error: %v", err)
	}
	if result.Provider != "plain_http_fallback" {
		t.Fatalf("expected plain_http_fallback provider, got %q", result.Provider)
	}
}

func TestExecuteWebFetchReturnsNormalizedPayload(t *testing.T) {
	t.Parallel()

	server := NewServer(NewProviderChainWithPolicy(egress.Policy{Resolver: egresstest.Resolver{}}, fakeProvider{
		name: "self_hosted_primary",
		result: FetchResult{
			Provider:     "self_hosted_primary",
			RequestedURL: "https://example.com",
			FinalURL:     "https://example.com/final",
			StatusCode:   200,
			MIMEType:     "text/plain",
			ContentText:  "hello world",
			Metadata:     map[string]any{"cache_hit": false},
		},
	}), nil)

	resp, err := server.Execute(context.Background(), &runtimev1.ExecuteRequest{
		ToolCall: &toolbrokerv1.ToolCall{ToolCallId: "tc-1", RunId: "run-1", ToolName: "web.fetch", ArgsJson: `{"url":"https://example.com"}`},
	})
	if err != nil {
		t.Fatalf("Execute returned error: %v", err)
	}
	if resp.GetResult().GetStatus() != "completed" {
		t.Fatalf("expected completed, got %+v", resp.GetResult())
	}
	var payload map[string]any
	if err := json.Unmarshal([]byte(resp.GetResult().GetResultJson()), &payload); err != nil {
		t.Fatalf("decode result json: %v", err)
	}
	if payload["provider"] != "self_hosted_primary" {
		t.Fatalf("unexpected provider payload: %v", payload["provider"])
	}
}

func TestExecuteWebExtractFetchesAndExtractsText(t *testing.T) {
	t.Parallel()

	httpServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "text/html")
		_, _ = w.Write([]byte(`<html><body><h1>Hello</h1><p>World</p></body></html>`))
	}))
	defer httpServer.Close()

	server := NewServer(NewProviderChainWithPolicy(egress.Policy{Resolver: egresstest.Resolver{}}, NewPlainHTTPProvider(egresstest.Client(httpServer), true)), nil)
	resp, err := server.Execute(context.Background(), &runtimev1.ExecuteRequest{
		ToolCall: &toolbrokerv1.ToolCall{ToolCallId: "tc-2", RunId: "run-1", ToolName: "web.extract", ArgsJson: `{"url":"` + egresstest.Origin + `"}`},
	})
	if err != nil {
		t.Fatalf("Execute returned error: %v", err)
	}
	if resp.GetResult().GetStatus() != "completed" {
		t.Fatalf("expected completed, got %+v", resp.GetResult())
	}
	var payload map[string]any
	if err := json.Unmarshal([]byte(resp.GetResult().GetResultJson()), &payload); err != nil {
		t.Fatalf("decode result json: %v", err)
	}
	if payload["content_text"] == "" {
		t.Fatal("expected extracted content_text")
	}
}
