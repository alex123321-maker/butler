package runtime

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"testing"

	runtimev1 "github.com/butler/butler/internal/gen/runtime/v1"
	toolbrokerv1 "github.com/butler/butler/internal/gen/toolbroker/v1"
)

// ---------------------------------------------------------------------------
// Navigate / Snapshot (existing)
// ---------------------------------------------------------------------------

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

func TestExecuteNavigateWithEmptyAllowlistPermitsAll(t *testing.T) {
	t.Parallel()

	runner := &stubRunner{result: Result{FinalURL: "https://example.com", Title: "Open"}}
	server := NewServer(runner, nil)
	resp, err := server.Execute(context.Background(), &runtimev1.ExecuteRequest{
		Context:  &runtimev1.ExecutionContext{RunId: "run-1", ToolCallId: "tool-1"},
		ToolCall: &toolbrokerv1.ToolCall{ToolCallId: "tool-1", RunId: "run-1", ToolName: "browser.navigate", ArgsJson: `{"url":"https://example.com"}`},
		Contract: &toolbrokerv1.ToolContract{ToolName: "browser.navigate", AllowedDomains: []string{}},
	})
	if err != nil {
		t.Fatalf("Execute returned error: %v", err)
	}
	if resp.GetResult().GetStatus() != "completed" {
		t.Fatalf("expected completed when allowlist is empty, got %+v", resp.GetResult())
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

func TestResolveBrowserScriptPathUsesConfiguredPath(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	path := filepath.Join(dir, "browser_runtime.mjs")
	if err := os.WriteFile(path, []byte("export {}\n"), 0o600); err != nil {
		t.Fatalf("WriteFile returned error: %v", err)
	}
	resolved, err := resolveBrowserScriptPath(path)
	if err != nil {
		t.Fatalf("resolveBrowserScriptPath returned error: %v", err)
	}
	if resolved != path {
		t.Fatalf("expected resolved path %q, got %q", path, resolved)
	}
}

func TestResolveBrowserScriptPathFailsWhenMissing(t *testing.T) {
	t.Parallel()
	_, err := resolveBrowserScriptPath(filepath.Join(t.TempDir(), "missing.mjs"))
	if err == nil {
		t.Fatal("expected missing script path to fail")
	}
}

// ---------------------------------------------------------------------------
// browser.click
// ---------------------------------------------------------------------------

func TestExecuteClick(t *testing.T) {
	t.Parallel()

	runner := &stubRunner{result: Result{OK: true, Title: "After Click"}}
	server := NewServer(runner, nil)
	resp, err := server.Execute(context.Background(), &runtimev1.ExecuteRequest{
		Context:  &runtimev1.ExecutionContext{RunId: "run-1", ToolCallId: "tc-1"},
		ToolCall: &toolbrokerv1.ToolCall{ToolCallId: "tc-1", RunId: "run-1", ToolName: "browser.click", ArgsJson: `{"selector":"#submit"}`},
		Contract: &toolbrokerv1.ToolContract{ToolName: "browser.click"},
	})
	if err != nil {
		t.Fatalf("Execute returned error: %v", err)
	}
	if resp.GetResult().GetStatus() != "completed" {
		t.Fatalf("expected completed, got %+v", resp.GetResult())
	}
	if len(runner.requests) != 1 || runner.requests[0].Selector != "#submit" {
		t.Fatalf("unexpected runner requests: %+v", runner.requests)
	}
	var payload map[string]any
	if err := json.Unmarshal([]byte(resp.GetResult().GetResultJson()), &payload); err != nil {
		t.Fatalf("decode result json: %v", err)
	}
	if payload["ok"] != true {
		t.Fatalf("expected ok=true, got %+v", payload)
	}
}

func TestExecuteClickMissingSelectorFails(t *testing.T) {
	t.Parallel()

	server := NewServer(&stubRunner{}, nil)
	resp, err := server.Execute(context.Background(), &runtimev1.ExecuteRequest{
		ToolCall: &toolbrokerv1.ToolCall{ToolCallId: "tc-1", RunId: "run-1", ToolName: "browser.click", ArgsJson: `{}`},
		Contract: &toolbrokerv1.ToolContract{ToolName: "browser.click"},
	})
	if err != nil {
		t.Fatalf("Execute returned error: %v", err)
	}
	if resp.GetResult().GetStatus() != "failed" {
		t.Fatalf("expected failed, got %+v", resp.GetResult())
	}
}

// ---------------------------------------------------------------------------
// browser.fill
// ---------------------------------------------------------------------------

func TestExecuteFill(t *testing.T) {
	t.Parallel()

	runner := &stubRunner{result: Result{OK: true}}
	server := NewServer(runner, nil)
	resp, err := server.Execute(context.Background(), &runtimev1.ExecuteRequest{
		Context:  &runtimev1.ExecutionContext{RunId: "run-1", ToolCallId: "tc-1"},
		ToolCall: &toolbrokerv1.ToolCall{ToolCallId: "tc-1", RunId: "run-1", ToolName: "browser.fill", ArgsJson: `{"selector":"#email","value":"test@example.com"}`},
		Contract: &toolbrokerv1.ToolContract{ToolName: "browser.fill"},
	})
	if err != nil {
		t.Fatalf("Execute returned error: %v", err)
	}
	if resp.GetResult().GetStatus() != "completed" {
		t.Fatalf("expected completed, got %+v", resp.GetResult())
	}
	if runner.requests[0].Value != "test@example.com" {
		t.Fatalf("expected value 'test@example.com', got %q", runner.requests[0].Value)
	}
}

func TestExecuteFillWithCredentialRef(t *testing.T) {
	t.Parallel()

	runner := &stubRunner{result: Result{OK: true}}
	server := NewServer(runner, nil)
	resp, err := server.Execute(context.Background(), &runtimev1.ExecuteRequest{
		Context:  &runtimev1.ExecutionContext{RunId: "run-1", ToolCallId: "tc-1"},
		ToolCall: &toolbrokerv1.ToolCall{ToolCallId: "tc-1", RunId: "run-1", ToolName: "browser.fill", ArgsJson: `{"selector":"#password","value":"placeholder"}`},
		Contract: &toolbrokerv1.ToolContract{ToolName: "browser.fill"},
		ResolvedCredentials: []*runtimev1.ResolvedCredential{
			{Alias: "my-login", Field: "password", Value: "s3cret!"},
		},
	})
	if err != nil {
		t.Fatalf("Execute returned error: %v", err)
	}
	if resp.GetResult().GetStatus() != "completed" {
		t.Fatalf("expected completed, got %+v", resp.GetResult())
	}
	// The resolved credential value should replace the placeholder.
	if runner.requests[0].Value != "s3cret!" {
		t.Fatalf("expected resolved credential value 's3cret!', got %q", runner.requests[0].Value)
	}
}

// ---------------------------------------------------------------------------
// browser.type
// ---------------------------------------------------------------------------

func TestExecuteType(t *testing.T) {
	t.Parallel()

	runner := &stubRunner{result: Result{OK: true}}
	server := NewServer(runner, nil)
	resp, err := server.Execute(context.Background(), &runtimev1.ExecuteRequest{
		Context:  &runtimev1.ExecutionContext{RunId: "run-1", ToolCallId: "tc-1"},
		ToolCall: &toolbrokerv1.ToolCall{ToolCallId: "tc-1", RunId: "run-1", ToolName: "browser.type", ArgsJson: `{"selector":"#search","text":"hello world"}`},
		Contract: &toolbrokerv1.ToolContract{ToolName: "browser.type"},
	})
	if err != nil {
		t.Fatalf("Execute returned error: %v", err)
	}
	if resp.GetResult().GetStatus() != "completed" {
		t.Fatalf("expected completed, got %+v", resp.GetResult())
	}
	if runner.requests[0].Text != "hello world" {
		t.Fatalf("expected text 'hello world', got %q", runner.requests[0].Text)
	}
}

func TestExecuteTypeWithCredentialRef(t *testing.T) {
	t.Parallel()

	runner := &stubRunner{result: Result{OK: true}}
	server := NewServer(runner, nil)
	resp, err := server.Execute(context.Background(), &runtimev1.ExecuteRequest{
		Context:  &runtimev1.ExecutionContext{RunId: "run-1", ToolCallId: "tc-1"},
		ToolCall: &toolbrokerv1.ToolCall{ToolCallId: "tc-1", RunId: "run-1", ToolName: "browser.type", ArgsJson: `{"selector":"#otp","text":"placeholder"}`},
		Contract: &toolbrokerv1.ToolContract{ToolName: "browser.type"},
		ResolvedCredentials: []*runtimev1.ResolvedCredential{
			{Alias: "my-login", Field: "otp", Value: "123456"},
		},
	})
	if err != nil {
		t.Fatalf("Execute returned error: %v", err)
	}
	if resp.GetResult().GetStatus() != "completed" {
		t.Fatalf("expected completed, got %+v", resp.GetResult())
	}
	if runner.requests[0].Text != "123456" {
		t.Fatalf("expected resolved credential value '123456', got %q", runner.requests[0].Text)
	}
}

// ---------------------------------------------------------------------------
// browser.wait_for
// ---------------------------------------------------------------------------

func TestExecuteWaitFor(t *testing.T) {
	t.Parallel()

	runner := &stubRunner{result: Result{Matched: true}}
	server := NewServer(runner, nil)
	resp, err := server.Execute(context.Background(), &runtimev1.ExecuteRequest{
		Context:  &runtimev1.ExecutionContext{RunId: "run-1", ToolCallId: "tc-1"},
		ToolCall: &toolbrokerv1.ToolCall{ToolCallId: "tc-1", RunId: "run-1", ToolName: "browser.wait_for", ArgsJson: `{"selector":".loaded","timeout":3000}`},
		Contract: &toolbrokerv1.ToolContract{ToolName: "browser.wait_for"},
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
	if payload["matched"] != true {
		t.Fatalf("expected matched=true, got %+v", payload)
	}
}

// ---------------------------------------------------------------------------
// browser.extract_text
// ---------------------------------------------------------------------------

func TestExecuteExtractText(t *testing.T) {
	t.Parallel()

	runner := &stubRunner{result: Result{Text: "Hello World"}}
	server := NewServer(runner, nil)
	resp, err := server.Execute(context.Background(), &runtimev1.ExecuteRequest{
		Context:  &runtimev1.ExecutionContext{RunId: "run-1", ToolCallId: "tc-1"},
		ToolCall: &toolbrokerv1.ToolCall{ToolCallId: "tc-1", RunId: "run-1", ToolName: "browser.extract_text", ArgsJson: `{"selector":"#content"}`},
		Contract: &toolbrokerv1.ToolContract{ToolName: "browser.extract_text"},
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
	if payload["text"].(string) != "Hello World" {
		t.Fatalf("expected text 'Hello World', got %+v", payload)
	}
}

func TestExecuteExtractTextMissingSelectorFails(t *testing.T) {
	t.Parallel()

	server := NewServer(&stubRunner{}, nil)
	resp, err := server.Execute(context.Background(), &runtimev1.ExecuteRequest{
		ToolCall: &toolbrokerv1.ToolCall{ToolCallId: "tc-1", RunId: "run-1", ToolName: "browser.extract_text", ArgsJson: `{}`},
		Contract: &toolbrokerv1.ToolContract{ToolName: "browser.extract_text"},
	})
	if err != nil {
		t.Fatalf("Execute returned error: %v", err)
	}
	if resp.GetResult().GetStatus() != "failed" {
		t.Fatalf("expected failed, got %+v", resp.GetResult())
	}
}

// ---------------------------------------------------------------------------
// browser.set_cookie
// ---------------------------------------------------------------------------

func TestExecuteSetCookie(t *testing.T) {
	t.Parallel()

	runner := &stubRunner{result: Result{OK: true}}
	server := NewServer(runner, nil)
	resp, err := server.Execute(context.Background(), &runtimev1.ExecuteRequest{
		Context:  &runtimev1.ExecutionContext{RunId: "run-1", ToolCallId: "tc-1"},
		ToolCall: &toolbrokerv1.ToolCall{ToolCallId: "tc-1", RunId: "run-1", ToolName: "browser.set_cookie", ArgsJson: `{"url":"https://example.com","cookie":{"name":"sid","value":"abc123","domain":"example.com"}}`},
		Contract: &toolbrokerv1.ToolContract{ToolName: "browser.set_cookie", AllowedDomains: []string{"example.com"}},
	})
	if err != nil {
		t.Fatalf("Execute returned error: %v", err)
	}
	if resp.GetResult().GetStatus() != "completed" {
		t.Fatalf("expected completed, got %+v", resp.GetResult())
	}
}

func TestExecuteSetCookieMissingFieldsFails(t *testing.T) {
	t.Parallel()

	server := NewServer(&stubRunner{}, nil)
	resp, err := server.Execute(context.Background(), &runtimev1.ExecuteRequest{
		ToolCall: &toolbrokerv1.ToolCall{ToolCallId: "tc-1", RunId: "run-1", ToolName: "browser.set_cookie", ArgsJson: `{"url":"https://example.com"}`},
		Contract: &toolbrokerv1.ToolContract{ToolName: "browser.set_cookie", AllowedDomains: []string{"example.com"}},
	})
	if err != nil {
		t.Fatalf("Execute returned error: %v", err)
	}
	if resp.GetResult().GetStatus() != "failed" {
		t.Fatalf("expected failed, got %+v", resp.GetResult())
	}
}

// ---------------------------------------------------------------------------
// browser.restore_storage_state
// ---------------------------------------------------------------------------

func TestExecuteRestoreStorageState(t *testing.T) {
	t.Parallel()

	runner := &stubRunner{result: Result{OK: true}}
	server := NewServer(runner, nil)
	resp, err := server.Execute(context.Background(), &runtimev1.ExecuteRequest{
		Context:  &runtimev1.ExecutionContext{RunId: "run-1", ToolCallId: "tc-1"},
		ToolCall: &toolbrokerv1.ToolCall{ToolCallId: "tc-1", RunId: "run-1", ToolName: "browser.restore_storage_state", ArgsJson: `{"storage_state":{"cookies":[{"name":"sid","value":"abc","domain":"example.com"}]}}`},
		Contract: &toolbrokerv1.ToolContract{ToolName: "browser.restore_storage_state"},
	})
	if err != nil {
		t.Fatalf("Execute returned error: %v", err)
	}
	if resp.GetResult().GetStatus() != "completed" {
		t.Fatalf("expected completed, got %+v", resp.GetResult())
	}
}

func TestExecuteRestoreStorageStateMissingStateFails(t *testing.T) {
	t.Parallel()

	server := NewServer(&stubRunner{}, nil)
	resp, err := server.Execute(context.Background(), &runtimev1.ExecuteRequest{
		ToolCall: &toolbrokerv1.ToolCall{ToolCallId: "tc-1", RunId: "run-1", ToolName: "browser.restore_storage_state", ArgsJson: `{}`},
		Contract: &toolbrokerv1.ToolContract{ToolName: "browser.restore_storage_state"},
	})
	if err != nil {
		t.Fatalf("Execute returned error: %v", err)
	}
	if resp.GetResult().GetStatus() != "failed" {
		t.Fatalf("expected failed, got %+v", resp.GetResult())
	}
}

// ---------------------------------------------------------------------------
// Unsupported tool
// ---------------------------------------------------------------------------

func TestExecuteUnsupportedToolFails(t *testing.T) {
	t.Parallel()

	server := NewServer(&stubRunner{}, nil)
	resp, err := server.Execute(context.Background(), &runtimev1.ExecuteRequest{
		ToolCall: &toolbrokerv1.ToolCall{ToolCallId: "tc-1", RunId: "run-1", ToolName: "browser.unknown", ArgsJson: `{}`},
		Contract: &toolbrokerv1.ToolContract{ToolName: "browser.unknown"},
	})
	if err != nil {
		t.Fatalf("Execute returned error: %v", err)
	}
	if resp.GetResult().GetStatus() != "failed" {
		t.Fatalf("expected failed, got %+v", resp.GetResult())
	}
}

// ---------------------------------------------------------------------------
// Integration-style: navigate → fill → click
// ---------------------------------------------------------------------------

func TestIntegrationNavigateFillClick(t *testing.T) {
	t.Parallel()

	// Simulate a three-step browser interaction using the stub runner.
	// This validates the server routes each tool correctly and passes
	// the expected arguments through.
	runner := &stubRunner{}
	server := NewServer(runner, nil)

	contract := &toolbrokerv1.ToolContract{ToolName: "browser.navigate", AllowedDomains: []string{}}

	// Step 1: Navigate.
	runner.result = Result{FinalURL: "https://app.example.com/login", Title: "Login"}
	resp, err := server.Execute(context.Background(), &runtimev1.ExecuteRequest{
		Context:  &runtimev1.ExecutionContext{RunId: "run-1", ToolCallId: "tc-1"},
		ToolCall: &toolbrokerv1.ToolCall{ToolCallId: "tc-1", RunId: "run-1", ToolName: "browser.navigate", ArgsJson: `{"url":"https://app.example.com/login"}`},
		Contract: contract,
	})
	if err != nil || resp.GetResult().GetStatus() != "completed" {
		t.Fatalf("navigate step failed: err=%v resp=%+v", err, resp.GetResult())
	}

	// Step 2: Fill email field with credential_ref.
	runner.result = Result{OK: true}
	resp, err = server.Execute(context.Background(), &runtimev1.ExecuteRequest{
		Context:  &runtimev1.ExecutionContext{RunId: "run-1", ToolCallId: "tc-2"},
		ToolCall: &toolbrokerv1.ToolCall{ToolCallId: "tc-2", RunId: "run-1", ToolName: "browser.fill", ArgsJson: `{"selector":"#password","value":"placeholder"}`},
		Contract: &toolbrokerv1.ToolContract{ToolName: "browser.fill"},
		ResolvedCredentials: []*runtimev1.ResolvedCredential{
			{Alias: "app-login", Field: "password", Value: "real-password"},
		},
	})
	if err != nil || resp.GetResult().GetStatus() != "completed" {
		t.Fatalf("fill step failed: err=%v resp=%+v", err, resp.GetResult())
	}
	if runner.requests[1].Value != "real-password" {
		t.Fatalf("fill step: expected resolved password, got %q", runner.requests[1].Value)
	}

	// Step 3: Click submit.
	runner.result = Result{OK: true, Title: "Dashboard"}
	resp, err = server.Execute(context.Background(), &runtimev1.ExecuteRequest{
		Context:  &runtimev1.ExecutionContext{RunId: "run-1", ToolCallId: "tc-3"},
		ToolCall: &toolbrokerv1.ToolCall{ToolCallId: "tc-3", RunId: "run-1", ToolName: "browser.click", ArgsJson: `{"selector":"#submit"}`},
		Contract: &toolbrokerv1.ToolContract{ToolName: "browser.click"},
	})
	if err != nil || resp.GetResult().GetStatus() != "completed" {
		t.Fatalf("click step failed: err=%v resp=%+v", err, resp.GetResult())
	}

	if len(runner.requests) != 3 {
		t.Fatalf("expected 3 runner requests, got %d", len(runner.requests))
	}
}

// ---------------------------------------------------------------------------
// Helpers
// ---------------------------------------------------------------------------

func TestValidateSelectorRequired(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name    string
		req     Request
		wantErr bool
	}{
		{name: "click with selector", req: Request{ToolName: "browser.click", Selector: "#btn"}, wantErr: false},
		{name: "click without selector", req: Request{ToolName: "browser.click"}, wantErr: true},
		{name: "fill with selector", req: Request{ToolName: "browser.fill", Selector: "input"}, wantErr: false},
		{name: "fill without selector", req: Request{ToolName: "browser.fill"}, wantErr: true},
		{name: "type with selector", req: Request{ToolName: "browser.type", Selector: "input"}, wantErr: false},
		{name: "type without selector", req: Request{ToolName: "browser.type"}, wantErr: true},
		{name: "wait_for with selector", req: Request{ToolName: "browser.wait_for", Selector: ".done"}, wantErr: false},
		{name: "wait_for without selector", req: Request{ToolName: "browser.wait_for"}, wantErr: true},
		{name: "extract_text with selector", req: Request{ToolName: "browser.extract_text", Selector: "p"}, wantErr: false},
		{name: "extract_text without selector", req: Request{ToolName: "browser.extract_text"}, wantErr: true},
		{name: "navigate (no selector needed)", req: Request{ToolName: "browser.navigate"}, wantErr: false},
	}
	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			err := validateSelectorRequired(tc.req)
			if (err != nil) != tc.wantErr {
				t.Fatalf("validateSelectorRequired(%+v) error=%v, wantErr=%v", tc.req, err, tc.wantErr)
			}
		})
	}
}

func TestBuildResultJSON(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name     string
		toolName string
		result   Result
		wantKey  string
	}{
		{name: "navigate includes final_url", toolName: "browser.navigate", result: Result{FinalURL: "https://x.com", Title: "X"}, wantKey: "final_url"},
		{name: "click includes ok", toolName: "browser.click", result: Result{OK: true}, wantKey: "ok"},
		{name: "wait_for includes matched", toolName: "browser.wait_for", result: Result{Matched: true}, wantKey: "matched"},
		{name: "extract_text includes text", toolName: "browser.extract_text", result: Result{Text: "hi"}, wantKey: "text"},
	}
	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			raw := buildResultJSON(tc.toolName, tc.result)
			var payload map[string]any
			if err := json.Unmarshal([]byte(raw), &payload); err != nil {
				t.Fatalf("buildResultJSON decode: %v", err)
			}
			if _, ok := payload[tc.wantKey]; !ok {
				t.Fatalf("expected key %q in result, got %+v", tc.wantKey, payload)
			}
		})
	}
}

// ---------------------------------------------------------------------------
// Stub
// ---------------------------------------------------------------------------

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
