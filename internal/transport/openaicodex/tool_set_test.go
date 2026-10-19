package openaicodex

import (
	"encoding/json"
	"testing"

	"github.com/butler/butler/internal/transport"
)

func TestToolSetReplacementPreservesCollisionAliasesAndEmptySet(t *testing.T) {
	p := &Provider{}
	a := transport.ToolDefinition{Name: "a.b", SchemaJSON: `{"type":"object"}`}
	b := transport.ToolDefinition{Name: "a_b", SchemaJSON: `{"type":"object"}`}
	p.rememberTools("run", []transport.ToolDefinition{a})
	p.rememberTools("run", []transport.ToolDefinition{b, a})
	payload := map[string]any{}
	p.attachCurrentTools(payload, "run")
	tools := payload["tools"].([]map[string]any)
	if tools[0]["name"] != "a_b_2" || tools[1]["name"] != "a_b" || p.resolveToolName("run", "a_b") != "a.b" {
		t.Fatal("old alias rebound", tools)
	}
	p.rememberTools("run", []transport.ToolDefinition{b})
	if p.resolveToolName("run", "a_b") != "a.b" {
		t.Fatal("historical alias discarded")
	}
	req := transport.SubmitToolResultRequest{RunID: "run", ToolCallRef: "call", ToolResultJSON: `{}`, ToolSet: transport.NewToolSetSnapshot(nil)}
	body, err := p.submitToolResultBody(req)
	if err != nil {
		t.Fatal(err)
	}
	var wire map[string]json.RawMessage
	if json.Unmarshal(body, &wire) != nil || string(wire["tools"]) != "[]" {
		t.Fatalf("explicit empty tools absent: %s", body)
	}
}
