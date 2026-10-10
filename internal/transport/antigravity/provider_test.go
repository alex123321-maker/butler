package antigravity

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/butler/butler/internal/transport"
)

const emptyInit = `{"event":"init","init":{"tools":[],"agent":"butler-model"}}` + "\n"
const answerResult = `{"event":"result","result":{"status":"SUCCESS","structured_output":{"kind":"answer","answer":"Привет 👋","tool_name":"","arguments":{}},"usage":{"input_tokens":11,"output_tokens":5,"total_tokens":16}}}` + "\n"

func TestReadResultRejectsUnsafeOrIncompleteCLI(t *testing.T) {
	cases := map[string]string{
		"enabled builtin":             `{"event":"init","init":{"tools":["run_command"],"agent":"butler-model"}}`,
		"implicit toolkit":            `{"event":"init","init":{"agent":"butler-model"}}`,
		"wrong agent":                 `{"event":"init","init":{"tools":[],"agent":"default"}}`,
		"missing init":                answerResult,
		"duplicate init":              emptyInit + emptyInit + answerResult,
		"tool step":                   emptyInit + `{"event":"step_update","step_update":{"step_type":"tool"}}`,
		"delegation":                  emptyInit + `{"event":"step_update","step_update":{"step_type":"agent_response","subagent_info":{"subagents":[]}}}`,
		"missing result":              emptyInit,
		"duplicate result":            emptyInit + answerResult + answerResult,
		"malformed JSON":              emptyInit + "not-json",
		"unsupported event":           emptyInit + `{"event":"control_request"}`,
		"free text instead of schema": emptyInit + `{"event":"result","result":{"status":"SUCCESS","response":"hello"}}`,
		"schema unknown fields":       emptyInit + `{"event":"result","result":{"status":"SUCCESS","structured_output":{"kind":"answer","answer":"x","tool_name":"","arguments":{},"authorized":true}}}`,
		"tool not supplied":           emptyInit + `{"event":"result","result":{"status":"SUCCESS","structured_output":{"kind":"tool_call","answer":"","tool_name":"shell.execute","arguments":{}}}}`,
		"missing object arguments":    emptyInit + `{"event":"result","result":{"status":"SUCCESS","structured_output":{"kind":"tool_call","answer":"","tool_name":"web.fetch","arguments":null}}}`,
	}
	for name, stream := range cases {
		t.Run(name, func(t *testing.T) {
			if _, _, err := readResult(strings.NewReader(stream), []transport.ToolDefinition{{Name: "web.fetch"}}); err == nil {
				t.Fatal("unsafe CLI response accepted")
			}
		})
	}
}

func TestReadResultPreservesAnswerAndUsage(t *testing.T) {
	choice, usage, err := readResult(strings.NewReader(emptyInit+answerResult), nil)
	if err != nil {
		t.Fatal(err)
	}
	if choice.Answer != "Привет 👋" || usage.InputTokens != 11 || usage.TotalTokens != 16 {
		t.Fatalf("incorrect normalized answer: %#v %#v", choice, usage)
	}
}

func TestCLIErrorsNeverExposeRawDiagnostics(t *testing.T) {
	for _, raw := range []string{"authentication failed access_token=secret-sentinel", "quota exhausted refresh_token=secret-sentinel", "unknown error cookie=secret-sentinel"} {
		encoded, _ := json.Marshal(map[string]any{"event": "result", "result": map[string]string{"status": "ERROR", "error": raw}})
		_, _, err := readResult(strings.NewReader(string(encoded)), nil)
		if err == nil || strings.Contains(err.Error(), "secret-sentinel") {
			t.Fatalf("unsafe normalized error: %v", err)
		}
	}
}

func TestCLIEnvironmentExcludesServiceCredentials(t *testing.T) {
	t.Setenv("BUTLER_POSTGRES_URL", "postgres-secret-sentinel")
	t.Setenv("GEMINI_API_KEY", "gemini-secret-sentinel")
	t.Setenv("BUTLER_OPENAI_API_KEY", "openai-secret-sentinel")
	for _, value := range cliEnvironment() {
		if strings.Contains(value, "secret-sentinel") {
			t.Fatalf("credential inherited by CLI: %s", strings.SplitN(value, "=", 2)[0])
		}
	}
}

func TestProviderProposalIsExternalAndCorrelated(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("subprocess contract test runs on the Linux lab")
	}
	profile := restrictedTestProfile(t)
	toolResult := `{"event":"result","result":{"status":"SUCCESS","structured_output":{"kind":"tool_call","answer":"","tool_name":"web.fetch","arguments":{"url":"https://example.com"}}}}` + "\n"
	binary := fakeCLI(t, emptyInit+toolResult)
	p, _ := NewProvider(Config{BinaryPath: binary, ProfileDir: profile, WorkspaceRoot: t.TempDir(), Timeout: time.Second})
	stream, err := p.StartRun(context.Background(), transport.StartRunRequest{Context: transport.TransportRunContext{RunID: "run-1", SessionKey: "session-1", ProviderName: Name, ModelName: "gemini-3.8-flash-tiered"}, InputItems: []transport.InputItem{{Role: "user", Content: "fetch it"}}, ToolDefinitions: []transport.ToolDefinition{{Name: "web.fetch", SchemaJSON: `{"type":"object"}`}}})
	if err != nil {
		t.Fatal(err)
	}
	var proposal *transport.ToolCallRequest
	for event := range stream {
		if event.TransportError != nil {
			t.Fatal(event.TransportError)
		}
		if event.ToolCall != nil {
			proposal = event.ToolCall
		}
	}
	if proposal == nil || proposal.ToolName != "web.fetch" || proposal.ToolCallRef != "agy-run-1-1" {
		t.Fatalf("missing proposal: %#v", proposal)
	}
	if _, err := p.SubmitToolResult(context.Background(), transport.SubmitToolResultRequest{RunID: "run-1", ToolCallRef: "foreign", ToolResultJSON: `{}`}); err == nil {
		t.Fatal("foreign tool result accepted")
	}
	if err := os.WriteFile(binary, []byte("#!/bin/sh\ncat >/dev/null\ncat <<'BUTLER_RESULT'\n"+emptyInit+answerResult+"BUTLER_RESULT\n"), 0700); err != nil {
		t.Fatal(err)
	}
	stream, err = p.SubmitToolResult(context.Background(), transport.SubmitToolResultRequest{RunID: "run-1", ToolCallRef: proposal.ToolCallRef, ToolResultJSON: `{"status":"success"}`})
	if err != nil {
		t.Fatal(err)
	}
	final := ""
	for event := range stream {
		if event.TransportError != nil {
			t.Fatal(event.TransportError)
		}
		if event.AssistantFinal != nil {
			final = event.AssistantFinal.Content
		}
	}
	if final != "Привет 👋" {
		t.Fatalf("missing final answer: %q", final)
	}
	p.mu.Lock()
	remaining := len(p.runs)
	p.mu.Unlock()
	if remaining != 0 {
		t.Fatal("terminal run state retained")
	}
}

func TestProviderCancellationStopsCLI(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("subprocess contract test runs on the Linux lab")
	}
	profile := restrictedTestProfile(t)
	path := filepath.Join(t.TempDir(), "agy")
	if err := os.WriteFile(path, []byte("#!/bin/sh\ncat >/dev/null\nexec sleep 30\n"), 0700); err != nil {
		t.Fatal(err)
	}
	p, _ := NewProvider(Config{BinaryPath: path, ProfileDir: profile, WorkspaceRoot: t.TempDir()})
	stream, err := p.StartRun(context.Background(), transport.StartRunRequest{Context: transport.TransportRunContext{RunID: "cancel", SessionKey: "s", ProviderName: Name, ModelName: "gemini"}})
	if err != nil {
		t.Fatal(err)
	}
	<-stream
	if _, err := p.CancelRun(context.Background(), transport.CancelRunRequest{RunID: "cancel"}); err != nil {
		t.Fatal(err)
	}
	done := make(chan struct{})
	go func() {
		for range stream {
		}
		close(done)
	}()
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("CLI cancellation did not complete")
	}
}

func TestProfileRejectsPermissivePolicyAndGlobalCustomizations(t *testing.T) {
	profile := restrictedTestProfile(t)
	if err := validateProfile(profile); err != nil {
		t.Fatal(err)
	}
	loginPolicy := strings.TrimSuffix(strings.TrimSpace(string(isolationSettings)), "}") + `,"trustedWorkspaces":["/work"],"model":"gemini-3.8-flash-tiered"}`
	settingsPathForLogin := filepath.Join(profile, ".gemini", "antigravity-cli", "settings.json")
	if err := os.WriteFile(settingsPathForLogin, []byte(loginPolicy), 0600); err != nil {
		t.Fatal(err)
	}
	if err := validateProfile(profile); err != nil {
		t.Fatalf("safe login metadata: %v", err)
	}
	if err := os.WriteFile(settingsPathForLogin, []byte(strings.Replace(loginPolicy, `"/work"`, `"/"`, 1)), 0600); err != nil {
		t.Fatal(err)
	}
	if err := validateProfile(profile); err == nil {
		t.Fatal("arbitrary trusted host workspace accepted")
	}
	if err := os.WriteFile(settingsPathForLogin, isolationSettings, 0600); err != nil {
		t.Fatal(err)
	}
	settingsPath := filepath.Join(profile, ".gemini", "antigravity-cli", "settings.json")
	canonical := `{"toolPermission":"strict","enableTerminalSandbox":true,"permissions":{"deny":["command(*)","unsandboxed(*)","read_file(*)","write_file(*)","read_url(*)","execute_url(*)","mcp(*)"]}}`
	if err := os.WriteFile(settingsPath, []byte(canonical), 0600); err != nil {
		t.Fatal(err)
	}
	if err := validateProfile(profile); err != nil {
		t.Fatalf("official CLI settings canonicalization rejected: %v", err)
	}
	if err := os.WriteFile(settingsPath, []byte(strings.Replace(canonical, `"unsandboxed(*)",`, "", 1)), 0600); err != nil {
		t.Fatal(err)
	}
	if err := validateProfile(profile); err == nil {
		t.Fatal("missing outside-sandbox execution restriction accepted")
	}
	if err := os.WriteFile(settingsPath, []byte(`{"toolPermission":"always-proceed"}`), 0600); err != nil {
		t.Fatal(err)
	}
	if err := validateProfile(profile); err == nil {
		t.Fatal("permissive profile accepted")
	}
	if err := os.WriteFile(settingsPath, isolationSettings, 0600); err != nil {
		t.Fatal(err)
	}
	customDir := filepath.Join(profile, ".gemini", "config")
	if err := os.MkdirAll(customDir, 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(customDir, "mcp_config.json"), nil, 0600); err != nil {
		t.Fatal(err)
	}
	if err := validateProfile(profile); err != nil {
		t.Fatalf("CLI empty MCP metadata rejected: %v", err)
	}
	if err := os.WriteFile(filepath.Join(customDir, "mcp_config.json"), []byte(`{"mcpServers":{"shell":{"command":"sh"}}}`), 0600); err != nil {
		t.Fatal(err)
	}
	if err := validateProfile(profile); err == nil {
		t.Fatal("configured global MCP server accepted")
	}
	if err := os.WriteFile(filepath.Join(customDir, "mcp_config.json"), nil, 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(customDir, "hooks.json"), []byte(`{}`), 0600); err != nil {
		t.Fatal(err)
	}
	if err := validateProfile(profile); err == nil {
		t.Fatal("global hooks accepted")
	}
}

func restrictedTestProfile(t *testing.T) string {
	t.Helper()
	profile := t.TempDir()
	t.Setenv("HOME", profile)
	if err := os.Chmod(profile, 0700); err != nil {
		t.Fatal(err)
	}
	settingsDir := filepath.Join(profile, ".gemini", "antigravity-cli")
	if err := os.MkdirAll(settingsDir, 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(settingsDir, "settings.json"), isolationSettings, 0600); err != nil {
		t.Fatal(err)
	}
	return profile
}

func fakeCLI(t *testing.T, events string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "agy")
	if err := os.WriteFile(path, []byte("#!/bin/sh\ncat >/dev/null\ncat <<'BUTLER_RESULT'\n"+events+"BUTLER_RESULT\n"), 0700); err != nil {
		t.Fatal(err)
	}
	return path
}
