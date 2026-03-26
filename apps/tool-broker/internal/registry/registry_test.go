package registry

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	toolbrokerv1 "github.com/butler/butler/internal/gen/toolbroker/v1"
)

func TestLoadAndListTools(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	path := filepath.Join(dir, "tools.json")
	if err := os.WriteFile(path, []byte(`{"tools":[{"tool_name":"browser.navigate","tool_class":"browser","input_schema_json":"{\"type\":\"object\",\"required\":[\"url\"],\"properties\":{\"url\":{\"type\":\"string\"}},\"additionalProperties\":false}","status":"enabled"},{"tool_name":"browser.snapshot","tool_class":"browser","input_schema_json":"{\"type\":\"object\"}","status":"disabled"}]}`), 0o600); err != nil {
		t.Fatalf("write registry file: %v", err)
	}

	registry, err := Load(path, "tool-browser:9090")
	if err != nil {
		t.Fatalf("Load returned error: %v", err)
	}
	tools := registry.List("browser", false)
	if len(tools) != 1 || tools[0].GetToolName() != "browser.navigate" {
		t.Fatalf("unexpected enabled tool list: %+v", tools)
	}
	allTools := registry.List("browser", true)
	if len(allTools) != 2 {
		t.Fatalf("expected disabled tools when requested, got %d", len(allTools))
	}
	if allTools[0].GetRuntimeTarget() == "" {
		t.Fatal("expected default runtime target to be applied")
	}
}

func TestValidateToolCall(t *testing.T) {
	t.Parallel()

	registry := &Registry{toolsByName: map[string]*toolbrokerv1.ToolContract{
		"browser.navigate": {
			ToolName:        "browser.navigate",
			Status:          "enabled",
			InputSchemaJson: `{"type":"object","required":["url"],"properties":{"url":{"type":"string"}},"additionalProperties":false}`,
		},
	}}

	valid, contract, err := registry.Validate(&toolbrokerv1.ToolCall{ToolName: "browser.navigate", ArgsJson: `{"url":"https://example.com"}`})
	if !valid || err != nil || contract == nil {
		t.Fatalf("expected valid tool call, got valid=%t contract=%v err=%v", valid, contract, err)
	}

	valid, _, err = registry.Validate(&toolbrokerv1.ToolCall{ToolName: "browser.navigate", ArgsJson: `{"unexpected":true}`})
	if valid || err == nil {
		t.Fatal("expected invalid tool call for schema mismatch")
	}
	if err.GetMessage() == "" {
		t.Fatal("expected validation error message")
	}
}

func TestValidateToolCallRejectsCredentialRefsWhenUnsupported(t *testing.T) {
	t.Parallel()

	registry := &Registry{toolsByName: map[string]*toolbrokerv1.ToolContract{
		"browser.navigate": {
			ToolName:               "browser.navigate",
			Status:                 "enabled",
			InputSchemaJson:        `{"type":"object","required":["url"],"properties":{"url":{"type":"string"}},"additionalProperties":false}`,
			SupportsCredentialRefs: false,
		},
	}}

	valid, _, err := registry.Validate(&toolbrokerv1.ToolCall{ToolName: "browser.navigate", ArgsJson: `{"url":"https://example.com"}`, CredentialRefs: []*toolbrokerv1.CredentialRef{{Type: "credential_ref", Alias: "github", Field: "token"}}})
	if valid || err == nil {
		t.Fatal("expected credential refs to be rejected for unsupported tool")
	}
}

func TestValidateToolCallSupportsOneOf(t *testing.T) {
	t.Parallel()

	registry := &Registry{toolsByName: map[string]*toolbrokerv1.ToolContract{
		"single_tab.press_keys": {
			ToolName:        "single_tab.press_keys",
			Status:          "enabled",
			InputSchemaJson: `{"type":"object","required":["session_id","keys"],"properties":{"session_id":{"type":"string"},"keys":{"oneOf":[{"type":"string"},{"type":"array","items":{"type":"string"}}]}},"additionalProperties":false}`,
		},
	}}

	valid, _, err := registry.Validate(&toolbrokerv1.ToolCall{
		ToolName: "single_tab.press_keys",
		ArgsJson: `{"session_id":"single-tab-1","keys":"Enter"}`,
	})
	if !valid || err != nil {
		t.Fatalf("expected string keys payload to validate, got valid=%t err=%v", valid, err)
	}

	valid, _, err = registry.Validate(&toolbrokerv1.ToolCall{
		ToolName: "single_tab.press_keys",
		ArgsJson: `{"session_id":"single-tab-1","keys":["Control","Enter"]}`,
	})
	if !valid || err != nil {
		t.Fatalf("expected array keys payload to validate, got valid=%t err=%v", valid, err)
	}
}

func TestLoadActualProjectRegistry(t *testing.T) {
	t.Parallel()

	reg, err := Load(filepath.Clean(filepath.Join("..", "..", "..", "..", "configs", "tools.json")), "local")
	if err != nil {
		t.Fatalf("Load returned error: %v", err)
	}

	webFetch, ok := reg.Get("web.fetch")
	if !ok {
		t.Fatal("expected web.fetch contract in project registry")
	}
	if webFetch.GetRuntimeTarget() != "tool-webfetch:9090" {
		t.Fatalf("unexpected web.fetch runtime target %q", webFetch.GetRuntimeTarget())
	}

	singleTabBind, ok := reg.Get("single_tab.bind")
	if !ok {
		t.Fatal("expected single_tab.bind contract in project registry")
	}
	if singleTabBind.GetStatus() != "active" {
		t.Fatalf("expected single_tab.bind to be active, got %q", singleTabBind.GetStatus())
	}
	if singleTabBind.GetRequiresApproval() {
		t.Fatal("expected single_tab.bind to avoid generic pre-tool approval")
	}
	if !strings.Contains(singleTabBind.GetInputSchemaJson(), "wait_timeout_ms") {
		t.Fatalf("expected single_tab.bind schema to expose wait_timeout_ms, got %q", singleTabBind.GetInputSchemaJson())
	}

	singleTabPressKeys, ok := reg.Get("single_tab.press_keys")
	if !ok {
		t.Fatal("expected single_tab.press_keys contract in project registry")
	}
	if singleTabPressKeys.GetRuntimeTarget() != "tool-browser-local:9090" {
		t.Fatalf("unexpected single_tab.press_keys runtime target %q", singleTabPressKeys.GetRuntimeTarget())
	}
}
