package runtime

import (
	"context"
	"encoding/json"
	"errors"
	browserscripts "github.com/butler/butler/apps/tool-browser/scripts"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"
	"unicode/utf8"

	runtimev1 "github.com/butler/butler/internal/gen/runtime/v1"
	toolbrokerv1 "github.com/butler/butler/internal/gen/toolbroker/v1"
)

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
	result := s.result
	if result.FinalURL == "" {
		result.FinalURL = req.URL
		if result.FinalURL == "" {
			result.FinalURL = req.ExpectedURL
		}
	}
	if result.ObservationVersion == 0 {
		result.ObservationVersion = req.ObservationVersion + 1
	}
	return result, nil
}
func browserCall(tool, owner string, args map[string]any) *runtimev1.ExecuteRequest {
	contract := &toolbrokerv1.ToolContract{ToolName: tool, ContractRevision: "3", AllowedDomains: []string{"example.com"}}
	if tool == "browser.prepare_fill" || tool == "browser.prepare_type" {
		contract.ContractRevision = "2"
		contract.EffectContract = &toolbrokerv1.EffectContract{RetrySafety: "unsafe", VerificationMethod: "local_browser_preparation_v1"}
	}
	return &runtimev1.ExecuteRequest{
		Context:  &runtimev1.ExecutionContext{RunId: owner},
		ToolCall: &toolbrokerv1.ToolCall{ToolCallId: "call-1", RunId: owner, ToolName: tool, ArgsJson: mustJSON(args)},
		Contract: contract,
	}
}

func TestLocalPreparationRequiresPinnedContractBeforeRunner(t *testing.T) {
	runner := &stubRunner{result: Result{OK: true}}
	server := NewServer(runner, nil)
	obs := openTest(t, server)
	for _, tool := range []string{"browser.prepare_fill", "browser.prepare_type"} {
		args := actionArgs(obs)
		args["value"], args["text"] = "local", "local"
		req := browserCall(tool, "run-1", args)
		req.Contract.ContractRevision = "1"
		assertCode(t, executeTest(t, server, req), "contract_mismatch")
		req.Contract.ContractRevision = "2"
		req.Contract.EffectContract.VerificationMethod = "none"
		assertCode(t, executeTest(t, server, req), "contract_mismatch")
	}
	if len(runner.requests) != 1 {
		t.Fatal("invalid local contract reached helper")
	}
}
func executeTest(t *testing.T, server *Server, req *runtimev1.ExecuteRequest) *toolbrokerv1.ToolResult {
	t.Helper()
	response, err := server.Execute(context.Background(), req)
	if err != nil {
		t.Fatalf("execute: %v", err)
	}
	return response.GetResult()
}
func openTest(t *testing.T, server *Server) Result {
	t.Helper()
	result := executeTest(t, server, browserCall("browser.open", "run-1", map[string]any{"url": "https://example.com/form"}))
	if result.GetStatus() != "completed" {
		t.Fatalf("open: %+v", result)
	}
	var observation Result
	if err := json.Unmarshal([]byte(result.GetResultJson()), &observation); err != nil {
		t.Fatal(err)
	}
	return observation
}
func actionArgs(observation Result) map[string]any {
	return map[string]any{"browser_session_id": observation.SessionID, "observation_version": observation.ObservationVersion, "expected_url": observation.FinalURL, "selector": "#submit"}
}
func assertCode(t *testing.T, result *toolbrokerv1.ToolResult, code string) {
	t.Helper()
	if result.GetStatus() != "failed" || !strings.Contains(result.GetError().GetDetailsJson(), `"`+code+`"`) {
		t.Fatalf("wanted failure %s, got %+v", code, result)
	}
}

func TestReadIsIndependentAndSessionToolsRequireExplicitOpen(t *testing.T) {
	runner := &stubRunner{result: Result{OK: true, Text: "Example"}}
	server := NewServer(runner, nil)
	read := executeTest(t, server, browserCall("browser.read", "run-1", map[string]any{"url": "https://example.com"}))
	if read.GetStatus() != "completed" || len(server.sessions.sessions) != 0 {
		t.Fatalf("read created continuation: %+v", read)
	}
	for _, tool := range []string{"browser.navigate", "browser.snapshot", "browser.click"} {
		args := map[string]any{"url": "https://example.com", "selector": "#submit"}
		assertCode(t, executeTest(t, server, browserCall(tool, "run-1", args)), "session_lost")
	}
	if len(runner.requests) != 1 {
		t.Fatal("legacy continuation reached helper")
	}
}

func TestSessionGuardsBeforeEffect(t *testing.T) {
	cases := []struct {
		name, code string
		modify     func(*runtimev1.ExecuteRequest, *Server, Result)
	}{
		{"owner", "owner_mismatch", func(req *runtimev1.ExecuteRequest, _ *Server, _ Result) {
			req.ToolCall.RunId = "run-2"
			req.Context.RunId = "run-2"
		}},
		{"context owner", "owner_mismatch", func(req *runtimev1.ExecuteRequest, _ *Server, _ Result) { req.Context.RunId = "run-2" }},
		{"version", "stale_observation", func(req *runtimev1.ExecuteRequest, _ *Server, obs Result) {
			args := actionArgs(obs)
			args["observation_version"] = 999
			req.ToolCall.ArgsJson = mustJSON(args)
		}},
		{"page", "page_changed", func(req *runtimev1.ExecuteRequest, _ *Server, obs Result) {
			args := actionArgs(obs)
			args["expected_url"] = "https://example.com/other"
			req.ToolCall.ArgsJson = mustJSON(args)
		}},
		{"domain contract", "scope_denied", func(req *runtimev1.ExecuteRequest, _ *Server, _ Result) {
			req.Contract.AllowedDomains = []string{"other.com"}
		}},
		{"navigate domain cap", "scope_denied", func(req *runtimev1.ExecuteRequest, _ *Server, obs Result) {
			req.ToolCall.ToolName = "browser.navigate"
			req.Contract.ToolName = "browser.navigate"
			req.Contract.AllowedDomains = []string{"example.com"}
			args := actionArgs(obs)
			args["url"] = "https://other.com"
			req.ToolCall.ArgsJson = mustJSON(args)
		}},
		{"expired", "session_expired", func(_ *runtimev1.ExecuteRequest, server *Server, obs Result) {
			server.sessions.sessions[obs.SessionID].expires = time.Now().Add(-time.Second)
		}},
		{"generation lost", "session_lost", func(_ *runtimev1.ExecuteRequest, server *Server, _ Result) { server.sessions.invalidateAll() }},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			runner := &stubRunner{result: Result{OK: true}}
			server := NewServer(runner, nil)
			obs := openTest(t, server)
			req := browserCall("browser.click", "run-1", actionArgs(obs))
			tc.modify(req, server, obs)
			assertCode(t, executeTest(t, server, req), tc.code)
			for _, request := range runner.requests[1:] {
				if request.ToolName != "browser.close" {
					t.Fatal("guard allowed an effect")
				}
			}
		})
	}
}

func TestSnapshotDoesNotNavigateAndCloseReleasesCapacity(t *testing.T) {
	runner := &stubRunner{result: Result{OK: true, FinalURL: "https://example.com/form"}}
	server := NewServer(runner, nil)
	obs := openTest(t, server)
	snapshot := executeTest(t, server, browserCall("browser.snapshot", "run-1", map[string]any{"browser_session_id": obs.SessionID}))
	if snapshot.GetStatus() != "completed" || runner.requests[1].URL != "" {
		t.Fatalf("snapshot navigated: %+v", runner.requests)
	}
	closeResult := executeTest(t, server, browserCall("browser.close", "run-1", map[string]any{"browser_session_id": obs.SessionID}))
	if closeResult.GetStatus() != "completed" || len(server.sessions.sessions) != 0 {
		t.Fatal("close did not release session")
	}
	assertCode(t, executeTest(t, server, browserCall("browser.click", "run-1", actionArgs(obs))), "session_lost")
}

type blockingRunner struct {
	started chan struct{}
	release chan struct{}
}

func (r *blockingRunner) Run(_ context.Context, req Request) (Result, error) {
	if req.ToolName == "browser.click" {
		close(r.started)
		<-r.release
	}
	return Result{OK: true, FinalURL: "https://example.com/form", ObservationVersion: req.ObservationVersion + 1}, nil
}
func TestExclusiveControlRejectsConcurrentEffect(t *testing.T) {
	runner := &blockingRunner{started: make(chan struct{}), release: make(chan struct{})}
	server := NewServer(runner, nil)
	obs := openTest(t, server)
	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		_, _ = server.Execute(context.Background(), browserCall("browser.click", "run-1", actionArgs(obs)))
	}()
	<-runner.started
	assertCode(t, executeTest(t, server, browserCall("browser.click", "run-1", actionArgs(obs))), "session_busy")
	close(runner.release)
	wg.Wait()
}

func TestProcessFailureInvalidatesAllSessions(t *testing.T) {
	runner := &stubRunner{result: Result{OK: true}}
	server := NewServer(runner, nil)
	first := openTest(t, server)
	second := openTest(t, server)
	runner.err = errors.New("unsafe private diagnostic")
	result := executeTest(t, server, browserCall("browser.click", "run-1", actionArgs(first)))
	assertCode(t, result, "session_lost")
	if strings.Contains(result.GetError().GetMessage(), "unsafe") {
		t.Fatal("private helper error leaked")
	}
	assertCode(t, executeTest(t, server, browserCall("browser.click", "run-1", actionArgs(second))), "session_lost")
}

func TestCredentialArgumentsRequireMatchingBrokerResolution(t *testing.T) {
	args := `{"selector":"#password","value":{"type":"credential_ref","alias":"login","field":"password"}}`
	if _, err := decodeRequest("browser.fill", args, nil); err == nil {
		t.Fatal("unresolved reference accepted")
	}
	if _, err := decodeRequest("browser.fill", args, []*runtimev1.ResolvedCredential{{Alias: "other", Field: "password", Value: "wrong"}}); err == nil {
		t.Fatal("wrong reference matched")
	}
	req, err := decodeRequest("browser.fill", args, []*runtimev1.ResolvedCredential{{Alias: "login", Field: "password", Value: "private-test-value"}})
	if err != nil || req.Value != "private-test-value" || len(req.SecretValues) != 1 {
		t.Fatal("reference not applied privately")
	}
	for _, tool := range []string{"browser.set_cookie", "browser.restore_storage_state"} {
		if _, err := decodeRequest(tool, `{"cookie":{"name":"sid","value":"raw-secret","domain":"example.com"},"storage_state":{"cookies":[]}}`, nil); err == nil {
			t.Fatal("raw auth state accepted")
		}
	}
}

func TestAuthStateCannotExpandSessionOrigin(t *testing.T) {
	for _, req := range []Request{
		{Cookie: &Cookie{Domain: "other.com"}},
		{Cookie: &Cookie{Domain: "example.com", Secure: true}},
		{StorageState: &StorageState{Origins: []OriginStorage{{Origin: "https://other.com"}}}},
		{StorageState: &StorageState{Cookies: []Cookie{{Domain: "com"}}}},
	} {
		if err := validateAuthScope(req, "http://example.com"); err == nil {
			t.Fatalf("scope expanded: %+v", req)
		}
	}
}

func TestCookieScopeCheckedBeforeSecretInjection(t *testing.T) {
	runner := &stubRunner{result: Result{OK: true}}
	server := NewServer(runner, nil)
	obs := openTest(t, server)
	args := actionArgs(obs)
	delete(args, "selector")
	args["cookie"] = map[string]string{"type": "credential_ref", "alias": "cookie", "field": "cookie"}
	req := browserCall("browser.set_cookie", "run-1", args)
	req.ResolvedCredentials = []*runtimev1.ResolvedCredential{{Alias: "cookie", Field: "cookie", Value: `{"name":"sid","value":"private","domain":"other.com"}`}}
	assertCode(t, executeTest(t, server, req), "scope_denied")
	if len(runner.requests) != 1 {
		t.Fatal("wrong-domain secret reached helper")
	}
}

func TestSessionLimitAndDefaultPortOrigin(t *testing.T) {
	store := newSessionStore()
	for i := 0; i < maxBrowserSessions; i++ {
		if _, err := store.open("owner", "https://example.com:443"); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := store.open("owner", "https://example.com"); err == nil {
		t.Fatal("unbounded contexts accepted")
	}
	if originOf("https://example.com:443/path") != "https://example.com" || originOf("http://example.com:80") != "http://example.com" {
		t.Fatal("default port origin mismatch")
	}
}
func TestURLAndUnicodeValidation(t *testing.T) {
	for _, raw := range []string{"file:///etc/passwd", "data:text/plain,a", "https://user:secret@example.com", "example.com"} {
		if _, err := parseWebURL(raw); err == nil {
			t.Fatal("invalid URL accepted")
		}
	}
	value := truncate("Русский 🐈 текст", 9)
	if !utf8.ValidString(value) || utf8.RuneCountInString(value) != 9 {
		t.Fatal("invalid Unicode truncation")
	}
}
func TestResolveBrowserScriptPath(t *testing.T) {
	root := t.TempDir()
	path := filepath.Join(root, "browser_runtime.mjs")
	if err := os.WriteFile(path, []byte(browserscripts.Runtime), 0600); err != nil {
		t.Fatal(err)
	}
	if resolved, err := resolveBrowserScriptPathFrom(root, path); err != nil || resolved != path {
		t.Fatalf("path=%q err=%v", resolved, err)
	}
	if _, err := resolveBrowserScriptPathFrom(root, path+".missing"); err == nil {
		t.Fatal("missing script accepted")
	}
	if err := os.WriteFile(path, []byte("unreviewed extension update"), 0600); err != nil {
		t.Fatal(err)
	}
	if _, err := resolveBrowserScriptPathFrom(root, path); err == nil {
		t.Fatal("swapped extension revision was executed")
	}
	other := filepath.Join(t.TempDir(), "outside.mjs")
	if err := os.WriteFile(other, []byte(browserscripts.Runtime), 0600); err != nil {
		t.Fatal(err)
	}
	if _, err := resolveBrowserScriptPathFrom(root, other); err == nil {
		t.Fatal("external workspace file was read")
	}
}
