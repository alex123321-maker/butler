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
	_, _, _ = p.normalizePayload("run", "response.function_call_arguments.done", map[string]any{"name": "single_tab.status", "call_id": "call", "arguments": "{}"}, state)
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
	if p.instructionsFor("run") != "" {
		t.Fatal("final retained instructions")
	}
	p.setInstructions("run", "old")
	if _, err := p.CancelRun(context.Background(), transport.CancelRunRequest{RunID: "run"}); err != nil {
		t.Fatal(err)
	}
	if p.instructionsFor("run") != "" {
		t.Fatal("cancel retained instructions")
	}
}

func TestRealtimeInstructionContinuityClearsPreviousSessionPolicy(t *testing.T) {
	p, _ := NewProvider(Config{Model: "fixture"}, nil)
	start, err := p.startRunRealtimeMessages(transport.StartRunRequest{Context: transport.TransportRunContext{RunID: "run"}, InputItems: []transport.InputItem{{Role: "system", Content: "current policy"}, {Role: "user", Content: "status"}}})
	if err != nil {
		t.Fatal(err)
	}
	if start[0]["session"].(map[string]any)["instructions"] != "current policy" {
		t.Fatal("initial policy missing")
	}
	resume, _ := p.submitToolResultRealtimeMessages(transport.SubmitToolResultRequest{RunID: "run", ToolCallRef: "call", ToolResultJSON: `{}`})
	if resume[0]["session"].(map[string]any)["instructions"] != "current policy" {
		t.Fatal("resume policy missing")
	}
	continued, _ := p.continueRunRealtimeMessages(transport.ContinueRunRequest{RunID: "run", InputItems: []transport.InputItem{{Role: "system", Content: "new policy"}}})
	if continued[0]["session"].(map[string]any)["instructions"] != "new policy" {
		t.Fatal("updated policy missing")
	}
	next, _ := p.startRunRealtimeMessages(transport.StartRunRequest{Context: transport.TransportRunContext{RunID: "next"}, InputItems: []transport.InputItem{{Role: "user", Content: "hello"}}})
	if next[0]["session"].(map[string]any)["instructions"] != "" {
		t.Fatal("new run inherited old policy")
	}
}
