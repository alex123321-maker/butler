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
