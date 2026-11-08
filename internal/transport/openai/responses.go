package openai

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"regexp"
	"strings"

	"github.com/butler/butler/internal/transport"
)

const lunaModel = "gpt-6-luna"

// Responses caps the entire generation, including billed reasoning tokens.
func (p *Provider) InferenceLimits() (int64, int64) {
	if p.config.Model == lunaModel {
		return 922000, 8192
	}
	return 0, 0
}

func (p *Provider) responsesBody(payload map[string]any) ([]byte, error) {
	if p.config.Model == lunaModel {
		if payload["model"] != lunaModel {
			return nil, fmt.Errorf("model differs from configured OpenAI allowance")
		}
		_, output := p.InferenceLimits()
		payload["max_output_tokens"] = output
		payload["parallel_tool_calls"] = false
		if p.config.ReasoningEffort != "" {
			payload["reasoning"] = map[string]string{"effort": p.config.ReasoningEffort}
		}
	}
	return json.Marshal(payload)
}

var invalidWireChar = regexp.MustCompile("[^a-zA-Z0-9_-]")

// Preserve valid native names. Make namespaced Butler methods recognizable on
// the wire and disambiguate from valid names and other normalized spellings.
func wireToolNames(defs []transport.ToolDefinition) map[string]string {
	names := make(map[string]string, len(defs))
	used := make(map[string]bool, len(defs))
	for _, d := range defs {
		if len(d.Name) > 0 && len(d.Name) <= 64 && !invalidWireChar.MatchString(d.Name) {
			names[d.Name], used[d.Name] = d.Name, true
		}
	}
	for _, d := range defs {
		if names[d.Name] != "" {
			continue
		}
		base := invalidWireChar.ReplaceAllString(d.Name, "_")
		if base == "" {
			base = "tool"
		}
		name := base
		if len(name) > 64 || used[name] {
			prefix := base[:min(46, len(base))]
			for index := 0; ; index++ {
				sum := sha256.Sum256([]byte(fmt.Sprintf("%s/%d", d.Name, index)))
				name = prefix + "_" + hex.EncodeToString(sum[:8])
				if !used[name] {
					break
				}
			}
		}
		names[d.Name], used[name] = name, true
	}
	return names
}

// Responses can announce the same call twice. Only its completed response
// carries authoritative usage, response identity and the complete call list.
func (s *streamState) collectCall(item map[string]any, seq int) {
	id, name := stringValue(item["call_id"]), stringValue(item["name"])
	if id == "" || name == "" {
		return
	}
	call := transport.ToolCallRequest{ToolCallRef: id, ToolName: name, ArgsJSON: coalesceJSONString(item["arguments"], "{}"), SequenceNo: seq}
	for i, old := range s.pendingCalls {
		if old.ToolCallRef == id {
			s.pendingCalls[i] = call
			return
		}
	}
	s.pendingCalls = append(s.pendingCalls, call)
}

func (p *Provider) finishResponses(run string, payload map[string]any, state *streamState) ([]transport.TransportEvent, bool, error) {
	resp := mapValue(payload["response"])
	if ref := providerSessionFromPayload(payload); ref != nil {
		state.providerSession = ref
	}
	status := stringValue(resp["status"])
	output, _ := resp["output"].([]any)
	for i, entry := range output {
		item := mapValue(entry)
		if stringValue(item["type"]) == "function_call" {
			state.collectCall(item, i)
		}
	}
	usage := usageFromMap(mapValue(resp["usage"]))
	model := stringValue(resp["model"])
	effort := stringValue(mapValue(resp["reasoning"])["effort"])
	observed := func(event transport.TransportEvent) transport.TransportEvent {
		event.ProviderSessionRef = state.providerSession
		event.Usage, event.ProviderReasoningEffort = usage, effort
		if model != "" {
			event.ProviderModelVersion = &model
		}
		return event
	}
	var events []transport.TransportEvent
	if len(state.pendingCalls) > 0 && status != "incomplete" {
		defs, _ := p.currentTools(run)
		aliases := wireToolNames(defs)
		for i, call := range state.pendingCalls {
			for logical, wire := range aliases {
				if wire == call.ToolName {
					state.pendingCalls[i].ToolName = logical
					break
				}
			}
		}
		if len(state.pendingCalls) == 1 {
			events = append(events, observed(transport.NewToolCallRequestedEvent(run, providerName, state.pendingCalls[0])))
		} else {
			batch := transport.ToolCallBatch{ToolCalls: state.pendingCalls, BatchRef: stringValue(resp["id"])}
			events = append(events, observed(transport.NewToolCallBatchRequestedEvent(run, providerName, batch)))
		}
	} else {
		p.clearInstructions(run)
	}
	text := strings.TrimSpace(extractOutputText(resp))
	if text == "" {
		text = strings.TrimSpace(state.finalText.String())
	}
	final := observed(transport.NewAssistantFinalEvent(run, providerName, transport.AssistantFinal{Content: text, FinishReason: status, Usage: usage}))
	end := transport.NewTerminalEvent(run, transport.EventTypeRunCompleted, providerName)
	end.ProviderSessionRef = state.providerSession
	return append(events, final, end), true, nil
}

func detailCount(payload map[string]any, group, name string) *int {
	value, ok := mapValue(payload[group])[name].(float64)
	if !ok {
		return nil
	}
	count := int(value)
	return &count
}
