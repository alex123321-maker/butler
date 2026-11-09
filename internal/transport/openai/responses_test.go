package openai

import (
	"encoding/json"
	"testing"

	"github.com/butler/butler/internal/transport"
)

func TestLunaResponsesProfileAndToolTurn(t *testing.T) {
	p, err := NewProvider(Config{Model: lunaModel, ReasoningEffort: "high"}, nil)
	if err != nil {
		t.Fatal(err)
	}
	tools := []transport.ToolDefinition{{Name: "tools.search"}, {Name: "tools_search"}}
	body, err := p.startRunBody(transport.StartRunRequest{Context: transport.TransportRunContext{RunID: "run", ModelName: lunaModel}, ToolDefinitions: tools, TransportOptions: map[string]any{"max_output_tokens": 90000, "reasoning": map[string]any{"effort": "low"}}})
	var wire struct {
		Cap       int `json:"max_output_tokens"`
		Reasoning struct{ Effort string }
		Tools     []struct{ Name string }
	}
	if err != nil || json.Unmarshal(body, &wire) != nil || wire.Cap != 8192 || wire.Reasoning.Effort != "high" || p.prefersWebSocket() || wire.Tools[0].Name == wire.Tools[1].Name {
		t.Fatal("wrong native request or colliding tool aliases")
	}
	alias := wire.Tools[0].Name
	state := streamState{responses: true}
	call := map[string]any{"type": "function_call", "name": alias, "call_id": "call_1", "arguments": "{}"}
	for _, packet := range []struct {
		typ  string
		body map[string]any
	}{{"response.function_call_arguments.done", call}, {"response.output_item.done", map[string]any{"item": call}}} {
		events, _, err := p.normalizePayload("run", packet.typ, packet.body, &state)
		if err != nil || len(events) != 0 {
			t.Fatal("call dispatched before provider usage")
		}
	}
	var response map[string]any
	json.Unmarshal([]byte(`{"id":"resp_1","model":"gpt-6-luna","status":"completed","reasoning":{"effort":"high"},"usage":{"input_tokens":30,"output_tokens":12,"total_tokens":42,"input_tokens_details":{"cached_tokens":10,"cache_write_tokens":4},"output_tokens_details":{"reasoning_tokens":7}}}`), &response)
	events, done, err := p.normalizePayload("run", "response.completed", map[string]any{"response": response}, &state)
	if err != nil || !done || len(events) != 3 {
		t.Fatal("missing completed native events")
	}
	e := events[0]
	if e.ToolCall == nil || e.ToolCall.ToolName != "tools.search" || e.ToolCall.ToolCallRef != "call_1" || e.ProviderSessionRef.ResponseRef != "resp_1" || e.Usage == nil || e.Usage.Validate() != nil || *e.Usage.CachedInputTokens != 10 || *e.Usage.CacheWriteInputTokens != 4 || *e.Usage.ReasoningOutputTokens != 7 || *e.ProviderModelVersion != lunaModel || e.ProviderReasoningEffort != "high" {
		t.Fatalf("incorrect tool receipt: %+v", e)
	}
	b, err := p.submitToolResultBody(transport.SubmitToolResultRequest{RunID: "run", ToolCallRef: "call_1", ToolResultJSON: `{"ok":true}`, ProviderSessionRef: e.ProviderSessionRef})
	var continuation struct {
		Input     []struct{ Output string }
		Previous  string `json:"previous_response_id"`
		Reasoning struct{ Effort string }
	}
	if err != nil || json.Unmarshal(b, &continuation) != nil || continuation.Input[0].Output != `{"ok":true}` || continuation.Previous != "resp_1" || continuation.Reasoning.Effort != "high" {
		t.Fatal("continuation lost native string output, session or effort")
	}
	if _, e := NewProvider(Config{Model: lunaModel, ReasoningEffort: "typo"}, nil); e == nil {
		t.Fatal("invalid profile accepted")
	}
}

func TestTerraMediumNativeProfile(t *testing.T) {
	const model = "gpt-6.1-terra"
	p, err := NewProvider(Config{Model: model, ReasoningEffort: "medium"}, nil)
	if err != nil {
		t.Fatal(err)
	}
	b, err := p.responsesBody(map[string]any{"model": model})
	var wire struct {
		Reasoning struct{ Effort string }
		Cap       int `json:"max_output_tokens"`
	}
	if err != nil || json.Unmarshal(b, &wire) != nil || wire.Reasoning.Effort != "medium" || wire.Cap != 8192 || p.prefersWebSocket() {
		t.Fatal("missing bounded Terra medium Responses profile")
	}
}
