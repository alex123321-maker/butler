package runtime

import (
	"context"
	"encoding/json"
	"errors"
	"testing"

	runtimev1 "github.com/butler/butler/internal/gen/runtime/v1"
	toolbrokerv1 "github.com/butler/butler/internal/gen/toolbroker/v1"
)

func TestExecuteNavigateWithAllowedDomain(t *testing.T) {
	t.Parallel()

	runner := &stubRunner{result: Result{FinalURL: "https://example.com", Title: "Example"}}
	server := NewServer(runner, nil)
	resp, err := server.Execute(context.Background(), &runtimev1.ExecuteRequest{
		Context:  &runtimev1.ExecutionContext{RunId: "run-1", ToolCallId: "tool-1"},
		ToolCall: &toolbrokerv1.ToolCall{ToolCallId: "tool-1", RunId: "run-1", ToolName: "browser.navigate", ArgsJson: `{"url":"https://example.com"}`},
		Contract: &toolbrokerv1.ToolContract{ToolName: "browser.navigate", AllowedDomains: []string{"example.com"}},
	})
	if err != nil {
		t.Fatalf("Execute returned error: %v", err)
	}
	if resp.GetResult().GetStatus() != "completed" {
		t.Fatalf("expected completed result, got %+v", resp.GetResult())
	}
	if len(runner.requests) != 1 || runner.requests[0].ToolName != "browser.navigate" {
		t.Fatalf("unexpected runner requests: %+v", runner.requests)
	}
	var payload map[string]any
	if err := json.Unmarshal([]byte(resp.GetResult().GetResultJson()), &payload); err != nil {
		t.Fatalf("decode result json: %v", err)
	}
	if payload["title"].(string) != "Example" {
		t.Fatalf("unexpected payload: %+v", payload)
	}
}

func TestExecuteSnapshotDeniedForDisallowedDomain(t *testing.T) {
	t.Parallel()

	server := NewServer(&stubRunner{}, nil)
	resp, err := server.Execute(context.Background(), &runtimev1.ExecuteRequest{
		ToolCall: &toolbrokerv1.ToolCall{ToolCallId: "tool-1", RunId: "run-1", ToolName: "browser.snapshot", ArgsJson: `{"url":"https://example.com"}`},
		Contract: &toolbrokerv1.ToolContract{ToolName: "browser.snapshot", AllowedDomains: []string{"butler.local"}},
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

func TestExecuteReturnsRunnerFailure(t *testing.T) {
	t.Parallel()

	server := NewServer(&stubRunner{err: errors.New("playwright failed")}, nil)
	resp, err := server.Execute(context.Background(), &runtimev1.ExecuteRequest{
		ToolCall: &toolbrokerv1.ToolCall{ToolCallId: "tool-1", RunId: "run-1", ToolName: "browser.navigate", ArgsJson: `{"url":"https://example.com"}`},
		Contract: &toolbrokerv1.ToolContract{ToolName: "browser.navigate", AllowedDomains: []string{"example.com"}},
	})
	if err != nil {
		t.Fatalf("Execute returned error: %v", err)
	}
	if resp.GetResult().GetStatus() != "failed" {
		t.Fatalf("expected failed result, got %+v", resp.GetResult())
	}
}

type stubRunner struct {
	requests []Request
	result   Result
	err      error
}

func (s *stubRunner) Run(_ context.Context, req Request) (Result, error) {
	s.requests = append(s.requests, req)
	if s.err != nil {
		return Result{}, s.err
	}
	return s.result, nil
}
