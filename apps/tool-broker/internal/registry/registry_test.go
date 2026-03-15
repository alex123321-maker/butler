package registry

import (
	"os"
	"path/filepath"
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
