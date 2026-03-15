package runtime

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	runtimev1 "github.com/butler/butler/internal/gen/runtime/v1"
	toolbrokerv1 "github.com/butler/butler/internal/gen/toolbroker/v1"
)

func TestExecutePerformsHTTPRequest(t *testing.T) {
	t.Parallel()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			t.Fatalf("expected POST, got %s", r.Method)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"ok":true}`))
	}))
	defer server.Close()

	runtimeServer := NewServer(server.Client(), nil)
	resp, err := runtimeServer.Execute(context.Background(), &runtimev1.ExecuteRequest{
		Context:  &runtimev1.ExecutionContext{RunId: "run-1", ToolCallId: "tool-1"},
		ToolCall: &toolbrokerv1.ToolCall{ToolCallId: "tool-1", RunId: "run-1", ToolName: "http.request", ArgsJson: `{"method":"POST","url":"` + server.URL + `"}`},
		Contract: &toolbrokerv1.ToolContract{ToolName: "http.request", AllowedDomains: []string{"127.0.0.1"}},
	})
	if err != nil {
		t.Fatalf("Execute returned error: %v", err)
	}
	if resp.GetResult().GetStatus() != "completed" {
		t.Fatalf("expected completed result, got %+v", resp.GetResult())
	}
	var payload map[string]any
	if err := json.Unmarshal([]byte(resp.GetResult().GetResultJson()), &payload); err != nil {
		t.Fatalf("decode result json: %v", err)
	}
	if int(payload["status_code"].(float64)) != http.StatusOK {
		t.Fatalf("unexpected status code payload: %+v", payload)
	}
	if payload["body"].(string) == "" {
		t.Fatalf("expected response body payload")
	}
}

func TestExecuteAppliesResolvedCredentialAuth(t *testing.T) {
	t.Parallel()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if got := r.Header.Get("Authorization"); got != "Bearer secret-token" {
			t.Fatalf("expected resolved auth header, got %q", got)
		}
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"ok":true}`))
	}))
	defer server.Close()

	runtimeServer := NewServer(server.Client(), nil)
	resp, err := runtimeServer.Execute(context.Background(), &runtimev1.ExecuteRequest{
		Context:             &runtimev1.ExecutionContext{RunId: "run-1", ToolCallId: "tool-1"},
		ToolCall:            &toolbrokerv1.ToolCall{ToolCallId: "tool-1", RunId: "run-1", ToolName: "http.request", ArgsJson: `{"method":"GET","url":"` + server.URL + `","auth":{"type":"credential_ref","alias":"github","field":"token"}}`},
		Contract:            &toolbrokerv1.ToolContract{ToolName: "http.request", AllowedDomains: []string{"127.0.0.1"}},
		ResolvedCredentials: []*runtimev1.ResolvedCredential{{Alias: "github", Field: "token", Value: "secret-token"}},
	})
	if err != nil {
		t.Fatalf("Execute returned error: %v", err)
	}
	if resp.GetResult().GetStatus() != "completed" {
		t.Fatalf("expected completed result, got %+v", resp.GetResult())
	}
}

func TestExecuteFailsWhenResolvedCredentialMissing(t *testing.T) {
	t.Parallel()

	runtimeServer := NewServer(nil, nil)
	resp, err := runtimeServer.Execute(context.Background(), &runtimev1.ExecuteRequest{
		ToolCall: &toolbrokerv1.ToolCall{ToolCallId: "tool-1", RunId: "run-1", ToolName: "http.request", ArgsJson: `{"method":"GET","url":"https://example.com","auth":{"type":"credential_ref","alias":"github","field":"token"}}`},
		Contract: &toolbrokerv1.ToolContract{ToolName: "http.request", AllowedDomains: []string{"example.com"}},
	})
	if err != nil {
		t.Fatalf("Execute returned error: %v", err)
	}
	if resp.GetResult().GetError().GetErrorClass().String() != "ERROR_CLASS_CREDENTIAL_ERROR" {
		t.Fatalf("expected credential error, got %+v", resp.GetResult().GetError())
	}
}

func TestExecuteWithEmptyAllowlistPermitsAll(t *testing.T) {
	t.Parallel()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{}`))
	}))
	defer server.Close()

	runtimeServer := NewServer(server.Client(), nil)
	resp, err := runtimeServer.Execute(context.Background(), &runtimev1.ExecuteRequest{
		Context:  &runtimev1.ExecutionContext{RunId: "run-1", ToolCallId: "tool-1"},
		ToolCall: &toolbrokerv1.ToolCall{ToolCallId: "tool-1", RunId: "run-1", ToolName: "http.request", ArgsJson: `{"method":"GET","url":"` + server.URL + `"}`},
		// Empty AllowedDomains = unrestricted; operators must populate the list to restrict.
		Contract: &toolbrokerv1.ToolContract{ToolName: "http.request", AllowedDomains: []string{}},
	})
	if err != nil {
		t.Fatalf("Execute returned error: %v", err)
	}
	if resp.GetResult().GetStatus() != "completed" {
		t.Fatalf("expected completed when allowlist is empty, got %+v", resp.GetResult())
	}
}

func TestExecuteDeniesDisallowedDomain(t *testing.T) {
	t.Parallel()

	runtimeServer := NewServer(nil, nil)
	resp, err := runtimeServer.Execute(context.Background(), &runtimev1.ExecuteRequest{
		ToolCall: &toolbrokerv1.ToolCall{ToolCallId: "tool-1", RunId: "run-1", ToolName: "http.request", ArgsJson: `{"method":"GET","url":"https://example.com"}`},
		Contract: &toolbrokerv1.ToolContract{ToolName: "http.request", AllowedDomains: []string{"api.example.org"}},
	})
	if err != nil {
		t.Fatalf("Execute returned error: %v", err)
	}
	if resp.GetResult().GetStatus() != "failed" {
		t.Fatalf("expected failed result, got %+v", resp.GetResult())
	}
	if resp.GetResult().GetError().GetMessage() == "" {
		t.Fatal("expected policy error message")
	}
}

func TestDomainAllowed(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name    string
		host    string
		allowed []string
		want    bool
	}{
		{name: "exact match", host: "example.com", allowed: []string{"example.com"}, want: true},
		{name: "subdomain match", host: "sub.example.com", allowed: []string{"example.com"}, want: true},
		{name: "no match", host: "other.com", allowed: []string{"example.com"}, want: false},
		{name: "empty host denied", host: "", allowed: []string{"example.com"}, want: false},
		{name: "empty allowlist permits all", host: "anything.io", allowed: []string{}, want: true},
		{name: "nil allowlist permits all", host: "anything.io", allowed: nil, want: true},
		{name: "wildcard entry permits all", host: "anything.io", allowed: []string{"*"}, want: true},
	}
	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			got := domainAllowed(tc.host, tc.allowed)
			if got != tc.want {
				t.Fatalf("domainAllowed(%q, %v) = %v, want %v", tc.host, tc.allowed, got, tc.want)
			}
		})
	}
}
