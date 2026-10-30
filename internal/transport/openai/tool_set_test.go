package openai

import (
	"encoding/json"
	"testing"

	"github.com/butler/butler/internal/transport"
)

func TestToolSetReplacementInResponses(t *testing.T) {
	p, err := NewProvider(Config{Model: "fixture"}, nil)
	if err != nil {
		t.Fatal(err)
	}
	initial := []transport.ToolDefinition{{Name: "tools.search", SchemaJSON: `{"type":"object"}`}}
	if _, err = p.startRunBody(transport.StartRunRequest{Context: transport.TransportRunContext{RunID: "run"}, ToolDefinitions: initial}); err != nil {
		t.Fatal(err)
	}
	loaded := append(initial, transport.ToolDefinition{Name: "http.request", SchemaJSON: `{"type":"object"}`})
	req := transport.SubmitToolResultRequest{RunID: "run", ToolCallRef: "load", ToolResultJSON: `{"status":"ok"}`, ToolSet: transport.NewToolSetSnapshot(loaded)}
	body, err := p.submitToolResultBody(req)
	if err != nil {
		t.Fatal(err)
	}
	var payload struct {
		Tools []map[string]any `json:"tools"`
	}
	if json.Unmarshal(body, &payload) != nil || len(payload.Tools) != 2 {
		t.Fatalf("updated schemas absent: %s", body)
	}
	req.ToolSet = nil
	body, _ = p.submitToolResultBody(req)
	_ = json.Unmarshal(body, &payload)
	if len(payload.Tools) != 2 {
		t.Fatal("nil snapshot lost loaded tools")
	}
	req.ToolSet = transport.NewToolSetSnapshot(nil)
	if _, err = p.submitToolResultBody(req); err != nil {
		t.Fatal(err)
	}
	body, _ = p.continueRunBody(transport.ContinueRunRequest{RunID: "run"})
	_ = json.Unmarshal(body, &payload)
	if len(payload.Tools) != 0 {
		t.Fatal("empty schema set resurrected")
	}
}
