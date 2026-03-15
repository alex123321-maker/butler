package runtime

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	runtimev1 "github.com/butler/butler/internal/gen/runtime/v1"
	toolbrokerv1 "github.com/butler/butler/internal/gen/toolbroker/v1"
)

// ---------------------------------------------------------------------------
// http.request (existing tests)
// ---------------------------------------------------------------------------

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

// ---------------------------------------------------------------------------
// http.download
// ---------------------------------------------------------------------------

func TestExecuteDownload(t *testing.T) {
	t.Parallel()

	body := strings.Repeat("x", 1024)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/octet-stream")
		_, _ = w.Write([]byte(body))
	}))
	defer server.Close()

	runtimeServer := NewServer(server.Client(), nil)
	resp, err := runtimeServer.Execute(context.Background(), &runtimev1.ExecuteRequest{
		Context:  &runtimev1.ExecutionContext{RunId: "run-1", ToolCallId: "tc-1"},
		ToolCall: &toolbrokerv1.ToolCall{ToolCallId: "tc-1", RunId: "run-1", ToolName: "http.download", ArgsJson: `{"url":"` + server.URL + `"}`},
		Contract: &toolbrokerv1.ToolContract{ToolName: "http.download", AllowedDomains: []string{}},
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
	if int(payload["size"].(float64)) != 1024 {
		t.Fatalf("expected size 1024, got %v", payload["size"])
	}
	if payload["truncated"].(bool) != false {
		t.Fatal("expected truncated=false")
	}
}

func TestExecuteDownloadEnforcesMaxSize(t *testing.T) {
	t.Parallel()

	body := strings.Repeat("a", 2048)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(body))
	}))
	defer server.Close()

	runtimeServer := NewServer(server.Client(), nil)
	resp, err := runtimeServer.Execute(context.Background(), &runtimev1.ExecuteRequest{
		Context:  &runtimev1.ExecutionContext{RunId: "run-1", ToolCallId: "tc-1"},
		ToolCall: &toolbrokerv1.ToolCall{ToolCallId: "tc-1", RunId: "run-1", ToolName: "http.download", ArgsJson: `{"url":"` + server.URL + `","max_size":1024}`},
		Contract: &toolbrokerv1.ToolContract{ToolName: "http.download", AllowedDomains: []string{}},
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
	if int(payload["size"].(float64)) != 1024 {
		t.Fatalf("expected truncated size 1024, got %v", payload["size"])
	}
	if payload["truncated"].(bool) != true {
		t.Fatal("expected truncated=true")
	}
}

func TestExecuteDownloadDeniesDisallowedDomain(t *testing.T) {
	t.Parallel()

	runtimeServer := NewServer(nil, nil)
	resp, err := runtimeServer.Execute(context.Background(), &runtimev1.ExecuteRequest{
		ToolCall: &toolbrokerv1.ToolCall{ToolCallId: "tc-1", RunId: "run-1", ToolName: "http.download", ArgsJson: `{"url":"https://evil.com/file"}`},
		Contract: &toolbrokerv1.ToolContract{ToolName: "http.download", AllowedDomains: []string{"safe.com"}},
	})
	if err != nil {
		t.Fatalf("Execute returned error: %v", err)
	}
	if resp.GetResult().GetStatus() != "failed" {
		t.Fatalf("expected failed, got %+v", resp.GetResult())
	}
}

func TestExecuteDownloadInvalidURL(t *testing.T) {
	t.Parallel()

	runtimeServer := NewServer(nil, nil)
	resp, err := runtimeServer.Execute(context.Background(), &runtimev1.ExecuteRequest{
		ToolCall: &toolbrokerv1.ToolCall{ToolCallId: "tc-1", RunId: "run-1", ToolName: "http.download", ArgsJson: `{"url":"not-a-url"}`},
		Contract: &toolbrokerv1.ToolContract{ToolName: "http.download"},
	})
	if err != nil {
		t.Fatalf("Execute returned error: %v", err)
	}
	if resp.GetResult().GetStatus() != "failed" {
		t.Fatalf("expected failed, got %+v", resp.GetResult())
	}
}

// ---------------------------------------------------------------------------
// http.parse_html
// ---------------------------------------------------------------------------

func TestExecuteParseHTMLFromURL(t *testing.T) {
	t.Parallel()

	htmlPage := `<html><body><h1>Title</h1><p class="content">Hello World</p><p class="content">Goodbye</p></body></html>`
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "text/html")
		_, _ = w.Write([]byte(htmlPage))
	}))
	defer server.Close()

	runtimeServer := NewServer(server.Client(), nil)
	resp, err := runtimeServer.Execute(context.Background(), &runtimev1.ExecuteRequest{
		Context:  &runtimev1.ExecutionContext{RunId: "run-1", ToolCallId: "tc-1"},
		ToolCall: &toolbrokerv1.ToolCall{ToolCallId: "tc-1", RunId: "run-1", ToolName: "http.parse_html", ArgsJson: `{"url":"` + server.URL + `","selector":"p.content"}`},
		Contract: &toolbrokerv1.ToolContract{ToolName: "http.parse_html", AllowedDomains: []string{}},
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
	if int(payload["count"].(float64)) != 2 {
		t.Fatalf("expected 2 matches, got %v", payload["count"])
	}
	matches := payload["matches"].([]any)
	if matches[0].(string) != "Hello World" {
		t.Fatalf("expected first match 'Hello World', got %q", matches[0])
	}
}

func TestExecuteParseHTMLFromContent(t *testing.T) {
	t.Parallel()

	htmlContent := `<div id="main"><span>Inner Text</span></div>`
	argsJSON := fmt.Sprintf(`{"content":%q,"selector":"#main"}`, htmlContent)

	runtimeServer := NewServer(nil, nil)
	resp, err := runtimeServer.Execute(context.Background(), &runtimev1.ExecuteRequest{
		Context:  &runtimev1.ExecutionContext{RunId: "run-1", ToolCallId: "tc-1"},
		ToolCall: &toolbrokerv1.ToolCall{ToolCallId: "tc-1", RunId: "run-1", ToolName: "http.parse_html", ArgsJson: argsJSON},
		Contract: &toolbrokerv1.ToolContract{ToolName: "http.parse_html"},
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
	if int(payload["count"].(float64)) != 1 {
		t.Fatalf("expected 1 match, got %v", payload["count"])
	}
	matches := payload["matches"].([]any)
	if matches[0].(string) != "Inner Text" {
		t.Fatalf("expected 'Inner Text', got %q", matches[0])
	}
}

func TestExecuteParseHTMLByTag(t *testing.T) {
	t.Parallel()

	htmlContent := `<html><body><h1>First</h1><h1>Second</h1></body></html>`
	argsJSON := fmt.Sprintf(`{"content":%q,"selector":"h1"}`, htmlContent)

	runtimeServer := NewServer(nil, nil)
	resp, err := runtimeServer.Execute(context.Background(), &runtimev1.ExecuteRequest{
		Context:  &runtimev1.ExecutionContext{RunId: "run-1", ToolCallId: "tc-1"},
		ToolCall: &toolbrokerv1.ToolCall{ToolCallId: "tc-1", RunId: "run-1", ToolName: "http.parse_html", ArgsJson: argsJSON},
		Contract: &toolbrokerv1.ToolContract{ToolName: "http.parse_html"},
	})
	if err != nil {
		t.Fatalf("Execute returned error: %v", err)
	}
	var payload map[string]any
	_ = json.Unmarshal([]byte(resp.GetResult().GetResultJson()), &payload)
	if int(payload["count"].(float64)) != 2 {
		t.Fatalf("expected 2 h1 matches, got %v", payload["count"])
	}
}

func TestExecuteParseHTMLMissingSelectorFails(t *testing.T) {
	t.Parallel()

	runtimeServer := NewServer(nil, nil)
	resp, err := runtimeServer.Execute(context.Background(), &runtimev1.ExecuteRequest{
		ToolCall: &toolbrokerv1.ToolCall{ToolCallId: "tc-1", RunId: "run-1", ToolName: "http.parse_html", ArgsJson: `{"content":"<p>hi</p>"}`},
		Contract: &toolbrokerv1.ToolContract{ToolName: "http.parse_html"},
	})
	if err != nil {
		t.Fatalf("Execute returned error: %v", err)
	}
	if resp.GetResult().GetStatus() != "failed" {
		t.Fatalf("expected failed, got %+v", resp.GetResult())
	}
}

func TestExecuteParseHTMLMissingURLAndContentFails(t *testing.T) {
	t.Parallel()

	runtimeServer := NewServer(nil, nil)
	resp, err := runtimeServer.Execute(context.Background(), &runtimev1.ExecuteRequest{
		ToolCall: &toolbrokerv1.ToolCall{ToolCallId: "tc-1", RunId: "run-1", ToolName: "http.parse_html", ArgsJson: `{"selector":"p"}`},
		Contract: &toolbrokerv1.ToolContract{ToolName: "http.parse_html"},
	})
	if err != nil {
		t.Fatalf("Execute returned error: %v", err)
	}
	if resp.GetResult().GetStatus() != "failed" {
		t.Fatalf("expected failed, got %+v", resp.GetResult())
	}
}

func TestExecuteParseHTMLDeniesDisallowedDomain(t *testing.T) {
	t.Parallel()

	runtimeServer := NewServer(nil, nil)
	resp, err := runtimeServer.Execute(context.Background(), &runtimev1.ExecuteRequest{
		ToolCall: &toolbrokerv1.ToolCall{ToolCallId: "tc-1", RunId: "run-1", ToolName: "http.parse_html", ArgsJson: `{"url":"https://evil.com","selector":"p"}`},
		Contract: &toolbrokerv1.ToolContract{ToolName: "http.parse_html", AllowedDomains: []string{"safe.com"}},
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

	runtimeServer := NewServer(nil, nil)
	resp, err := runtimeServer.Execute(context.Background(), &runtimev1.ExecuteRequest{
		ToolCall: &toolbrokerv1.ToolCall{ToolCallId: "tc-1", RunId: "run-1", ToolName: "http.unknown", ArgsJson: `{}`},
		Contract: &toolbrokerv1.ToolContract{ToolName: "http.unknown"},
	})
	if err != nil {
		t.Fatalf("Execute returned error: %v", err)
	}
	if resp.GetResult().GetStatus() != "failed" {
		t.Fatalf("expected failed, got %+v", resp.GetResult())
	}
}

// ---------------------------------------------------------------------------
// Integration-style: request → download → parse_html
// ---------------------------------------------------------------------------

func TestIntegrationRequestDownloadParseHTML(t *testing.T) {
	t.Parallel()

	htmlPage := `<html><body><h1>API Docs</h1><p class="info">Version 2.0</p></body></html>`
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api":
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"status":"ok"}`))
		case "/page":
			w.Header().Set("Content-Type", "text/html")
			_, _ = w.Write([]byte(htmlPage))
		case "/file":
			w.Header().Set("Content-Type", "application/octet-stream")
			_, _ = w.Write([]byte("binary-content-here"))
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer server.Close()

	runtimeServer := NewServer(server.Client(), nil)
	contract := &toolbrokerv1.ToolContract{AllowedDomains: []string{}}

	// Step 1: http.request
	resp, err := runtimeServer.Execute(context.Background(), &runtimev1.ExecuteRequest{
		Context:  &runtimev1.ExecutionContext{RunId: "run-1", ToolCallId: "tc-1"},
		ToolCall: &toolbrokerv1.ToolCall{ToolCallId: "tc-1", RunId: "run-1", ToolName: "http.request", ArgsJson: `{"method":"GET","url":"` + server.URL + `/api"}`},
		Contract: contract,
	})
	if err != nil || resp.GetResult().GetStatus() != "completed" {
		t.Fatalf("request step failed: err=%v resp=%+v", err, resp.GetResult())
	}

	// Step 2: http.download
	resp, err = runtimeServer.Execute(context.Background(), &runtimev1.ExecuteRequest{
		Context:  &runtimev1.ExecutionContext{RunId: "run-1", ToolCallId: "tc-2"},
		ToolCall: &toolbrokerv1.ToolCall{ToolCallId: "tc-2", RunId: "run-1", ToolName: "http.download", ArgsJson: `{"url":"` + server.URL + `/file"}`},
		Contract: contract,
	})
	if err != nil || resp.GetResult().GetStatus() != "completed" {
		t.Fatalf("download step failed: err=%v resp=%+v", err, resp.GetResult())
	}
	var dlPayload map[string]any
	_ = json.Unmarshal([]byte(resp.GetResult().GetResultJson()), &dlPayload)
	if dlPayload["body"].(string) != "binary-content-here" {
		t.Fatalf("download body mismatch: %v", dlPayload["body"])
	}

	// Step 3: http.parse_html
	resp, err = runtimeServer.Execute(context.Background(), &runtimev1.ExecuteRequest{
		Context:  &runtimev1.ExecutionContext{RunId: "run-1", ToolCallId: "tc-3"},
		ToolCall: &toolbrokerv1.ToolCall{ToolCallId: "tc-3", RunId: "run-1", ToolName: "http.parse_html", ArgsJson: `{"url":"` + server.URL + `/page","selector":"p.info"}`},
		Contract: contract,
	})
	if err != nil || resp.GetResult().GetStatus() != "completed" {
		t.Fatalf("parse_html step failed: err=%v resp=%+v", err, resp.GetResult())
	}
	var parsePayload map[string]any
	_ = json.Unmarshal([]byte(resp.GetResult().GetResultJson()), &parsePayload)
	matches := parsePayload["matches"].([]any)
	if len(matches) != 1 || matches[0].(string) != "Version 2.0" {
		t.Fatalf("parse_html unexpected matches: %v", matches)
	}
}

// ---------------------------------------------------------------------------
// HTML parsing helpers
// ---------------------------------------------------------------------------

func TestParseSimpleSelector(t *testing.T) {
	t.Parallel()

	cases := []struct {
		sel       string
		wantTag   string
		wantID    string
		wantClass string
	}{
		{sel: "h1", wantTag: "h1"},
		{sel: "#main", wantID: "main"},
		{sel: ".info", wantClass: "info"},
		{sel: "p.content", wantTag: "p", wantClass: "content"},
		{sel: "div#header", wantTag: "div", wantID: "header"},
		{sel: "", wantTag: ""},
	}
	for _, tc := range cases {
		tc := tc
		t.Run(tc.sel, func(t *testing.T) {
			t.Parallel()
			tag, id, class := parseSimpleSelector(tc.sel)
			if tag != tc.wantTag || id != tc.wantID || class != tc.wantClass {
				t.Fatalf("parseSimpleSelector(%q) = (%q, %q, %q), want (%q, %q, %q)",
					tc.sel, tag, id, class, tc.wantTag, tc.wantID, tc.wantClass)
			}
		})
	}
}

func TestExtractTextBySelector(t *testing.T) {
	t.Parallel()

	htmlContent := `<html><body><div id="main"><p class="info">Hello</p><p class="info">World</p></div><p>Other</p></body></html>`

	cases := []struct {
		name     string
		selector string
		want     int
	}{
		{name: "by class", selector: "p.info", want: 2},
		{name: "by id", selector: "#main", want: 1},
		{name: "by tag", selector: "p", want: 3},
		{name: "no match", selector: "span", want: 0},
	}
	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			matches := extractTextBySelector(htmlContent, tc.selector)
			if len(matches) != tc.want {
				t.Fatalf("extractTextBySelector(%q) returned %d matches, want %d: %v",
					tc.selector, len(matches), tc.want, matches)
			}
		})
	}
}
