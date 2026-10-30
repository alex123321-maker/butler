package openai

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/butler/butler/internal/transport"
)

func TestInstructionsSurviveToolCompletionAndEndWithRun(t *testing.T) {
	p, err := NewProvider(Config{Model: "fixture", TransportMode: TransportModeSSEOnly}, nil)
	if err != nil {
		t.Fatal(err)
	}
	start, err := p.startRunBody(transport.StartRunRequest{Context: transport.TransportRunContext{RunID: "run", ModelName: "fixture"}, InputItems: []transport.InputItem{{Role: "system", Content: "follow owner policy"}, {Role: "user", Content: "status"}}})
	if err != nil {
		t.Fatal(err)
	}
	var payload map[string]any
	if err := json.Unmarshal(start, &payload); err != nil {
		t.Fatal(err)
	}
	if payload["instructions"] != "follow owner policy" || len(payload["input"].([]any)) != 1 {
		t.Fatal("start instruction normalization failed")
	}
	state := &streamState{}
	_, _, _ = p.normalizePayload("run", "response.output_item.done", map[string]any{"item": map[string]any{"type": "function_call", "name": "single_tab.status", "call_id": "call", "arguments": "{}"}}, state)
	_, _, _ = p.normalizePayload("run", "response.completed", map[string]any{"response": map[string]any{"status": "completed"}}, state)
	resume, err := p.submitToolResultBody(transport.SubmitToolResultRequest{RunID: "run", ToolCallRef: "call", ToolResultJSON: `{}`})
	if err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal(resume, &payload); err != nil {
		t.Fatal(err)
	}
	if payload["instructions"] != "follow owner policy" {
		t.Fatal("tool response lost instructions")
	}
	if p.instructionsFor("other") != "" {
		t.Fatal("instructions leaked across runs")
	}
	_, _, _ = p.normalizePayload("run", "response.completed", map[string]any{"response": map[string]any{"status": "completed"}}, &streamState{})
	if p.instructionsFor("run") == "" {
		t.Fatal("final lost bounded correction context")
	}
	p.setInstructions("run", "old")
	if _, err := p.CancelRun(context.Background(), transport.CancelRunRequest{RunID: "run"}); err != nil {
		t.Fatal(err)
	}
	if p.instructionsFor("run") != "" {
		t.Fatal("cancel retained instructions")
	}
}
