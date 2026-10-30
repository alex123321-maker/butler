package openai

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/butler/butler/internal/transport"
	"github.com/gorilla/websocket"
)

func lunaRequest(run string) transport.StartRunRequest {
	return transport.StartRunRequest{Context: transport.TransportRunContext{RunID: run, SessionKey: "session", ProviderName: "openai", ModelName: "gpt-6-luna"}, InputItems: []transport.InputItem{{Role: "system", Content: "owner policy"}, {Role: "user", Content: "inspect"}}, ToolDefinitions: []transport.ToolDefinition{{Name: "memory.search", SchemaJSON: `{"type":"object","properties":{"q":{"type":"string"}},"required":[]}`}}, StreamingEnabled: true}
}
func completed(id string, output []any) map[string]any {
	return map[string]any{"type": "response.completed", "response": map[string]any{"id": id, "model": "gpt-6-luna", "status": "completed", "output": output, "usage": map[string]any{"input_tokens": float64(12), "output_tokens": float64(8), "total_tokens": float64(20), "input_tokens_details": map[string]any{"cached_tokens": float64(4)}, "output_tokens_details": map[string]any{"reasoning_tokens": float64(5)}}}}
}
func textOutput(text string) []any {
	return []any{map[string]any{"type": "message", "content": []any{map[string]any{"type": "output_text", "text": text}}}}
}
func collect(t *testing.T, s transport.EventStream) []transport.TransportEvent {
	t.Helper()
	var out []transport.TransportEvent
	for e := range s {
		out = append(out, e)
	}
	return out
}

func TestResponsesWebSocketToolRoundTrip(t *testing.T) {
	var connections atomic.Int32
	var dispatches atomic.Int32
	requests := make(chan map[string]any, 2)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/responses" || r.URL.RawQuery != "" || r.Header.Get("OpenAI-Beta") != "" {
			t.Error("not a Responses connection")
		}
		conn, err := (&websocket.Upgrader{}).Upgrade(w, r, nil)
		if err != nil {
			t.Error(err)
			return
		}
		defer conn.Close()
		connections.Add(1)
		for turn := 0; turn < 2; turn++ {
			var q map[string]any
			if conn.ReadJSON(&q) != nil {
				return
			}
			requests <- q
			if q["type"] != "response.create" || q["stream"] != nil || q["model"] != "gpt-6-luna" || q["max_output_tokens"] != float64(8192) {
				t.Error("incorrect WS envelope")
			}
			if turn == 0 {
				call := map[string]any{"type": "function_call", "id": "item1", "call_id": "call1", "name": toolAlias("memory.search"), "arguments": `{"q":"lesson"}`}
				_ = conn.WriteJSON(map[string]any{"type": "response.output_item.done", "item": call})
				_ = conn.WriteJSON(completed("resp1", []any{call}))
			} else {
				_ = conn.WriteJSON(completed("resp2", textOutput("done")))
			}
		}
	}))
	defer server.Close()
	p, err := NewProvider(Config{Model: "gpt-6-luna", BaseURL: server.URL, ReasoningEffort: "high"}, server.Client())
	if err != nil {
		t.Fatal(err)
	}
	defer p.CloseAllSessions()
	ctx := transport.WithInferenceDispatch(context.Background(), func(context.Context) error { dispatches.Add(1); return nil })
	stream, err := p.StartRun(ctx, lunaRequest("run"))
	if err != nil {
		t.Fatal(err)
	}
	events := collect(t, stream)
	if len(events) != 2 || events[0].ToolCall == nil || events[0].ToolCall.ToolName != "memory.search" || events[0].Usage == nil || events[0].Usage.TotalTokens != 20 {
		t.Fatalf("tool/usage missing: %+v", events)
	}
	first := <-requests
	if first["instructions"] != "owner policy" || first["reasoning"].(map[string]any)["effort"] != "high" {
		t.Fatal("policy/profile missing")
	}
	tools := first["tools"].([]any)
	if tools[0].(map[string]any)["strict"] != false {
		t.Fatal("optional fields strengthened")
	}
	stream, err = p.SubmitToolResult(ctx, transport.SubmitToolResultRequest{RunID: "run", ToolCallRef: "call1", ToolResultJSON: `{"items":[]}`, ProviderSessionRef: events[0].ProviderSessionRef, ToolSet: transport.NewToolSetSnapshot(nil)})
	if err != nil {
		t.Fatal(err)
	}
	events = collect(t, stream)
	q := <-requests
	if q["previous_response_id"] != "resp1" || q["instructions"] != "owner policy" || len(q["tools"].([]any)) != 0 {
		t.Fatal("continuation state changed")
	}
	item := q["input"].([]any)[0].(map[string]any)
	if item["output"] != `{"items":[]}` || item["call_id"] != "call1" {
		t.Fatal("tool output not encoded as a string")
	}
	if events[0].AssistantFinal == nil || events[0].AssistantFinal.Content != "done" || *events[0].Usage.CachedInputTokens != 4 || *events[0].Usage.OutputReasoningTokens != 5 || events[0].Usage.ThinkingTokens != nil {
		t.Fatal("final accounting mismatch")
	}
	if connections.Load() != 1 || dispatches.Load() != 2 {
		t.Fatal("socket reuse/one dispatch violated")
	}
}

func TestResponsesNeverReplayAfterDispatch(t *testing.T) {
	var posts atomic.Int32
	var creates atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPost {
			posts.Add(1)
			return
		}
		c, err := (&websocket.Upgrader{}).Upgrade(w, r, nil)
		if err != nil {
			return
		}
		defer c.Close()
		var q map[string]any
		if c.ReadJSON(&q) == nil {
			creates.Add(1)
		}
	}))
	defer server.Close()
	p, _ := NewProvider(Config{Model: "gpt-6-luna", BaseURL: server.URL}, server.Client())
	defer p.CloseAllSessions()
	stream, err := p.StartRun(context.Background(), lunaRequest("run"))
	if err != nil {
		t.Fatal(err)
	}
	events := collect(t, stream)
	if creates.Load() != 1 || posts.Load() != 0 || len(events) != 1 || events[0].TransportError == nil {
		t.Fatal("lost response replayed or hidden")
	}
}

func TestResponsesFallbackOnlyBeforeGeneration(t *testing.T) {
	for _, status := range []int{404, 401, 403, 429, 500} {
		t.Run(fmt.Sprint(status), func(t *testing.T) {
			var posts atomic.Int32
			var marks atomic.Int32
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.Method == http.MethodGet {
					w.WriteHeader(status)
					return
				}
				posts.Add(1)
				w.Header().Set("Content-Type", "text/event-stream")
				raw, _ := json.Marshal(completed("resp", textOutput("done")))
				fmt.Fprintf(w, "data: %s\n\n", raw)
			}))
			defer server.Close()
			p, _ := NewProvider(Config{Model: "gpt-6-luna", BaseURL: server.URL}, server.Client())
			defer p.CloseAllSessions()
			ctx := transport.WithInferenceDispatch(context.Background(), func(context.Context) error { marks.Add(1); return nil })
			s, err := p.StartRun(ctx, lunaRequest("run"))
			if status == 404 {
				if err != nil {
					t.Fatal(err)
				}
				events := collect(t, s)
				if len(events) != 3 || events[0].EventType != transport.EventTypeTransportWarning || posts.Load() != 1 || marks.Load() != 1 {
					t.Fatal("unsupported handshake did not fall back exactly once")
				}
			} else if err == nil || posts.Load() != 0 || marks.Load() != 0 {
				t.Fatal("denied handshake caused generation")
			}
		})
	}
}

func TestResponsesIncompleteNeverReleasesTool(t *testing.T) {
	p, _ := NewProvider(Config{}, nil)
	defer p.CloseAllSessions()
	state := &streamState{}
	_, _, err := p.normalizePayload("run", "response.output_item.done", map[string]any{"item": map[string]any{"type": "function_call", "name": "tool", "call_id": "c", "arguments": "{}"}}, state)
	if err != nil {
		t.Fatal(err)
	}
	end := completed("resp", nil)
	end["type"] = "response.incomplete"
	end["response"].(map[string]any)["status"] = "incomplete"
	events, stop, err := p.normalizePayload("run", "response.incomplete", end, state)
	if err != nil || !stop || len(events) != 1 || events[0].ToolCall != nil || events[0].TransportError == nil || events[0].Usage.TotalTokens != 20 {
		t.Fatal("incomplete response released tool or lost cost")
	}
}

func TestResponsesCancellationUnblocksSocket(t *testing.T) {
	ready := make(chan struct{})
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		c, err := (&websocket.Upgrader{}).Upgrade(w, r, nil)
		if err != nil {
			return
		}
		defer c.Close()
		var q map[string]any
		if c.ReadJSON(&q) != nil {
			return
		}
		close(ready)
		_, _, _ = c.ReadMessage()
	}))
	defer server.Close()
	p, _ := NewProvider(Config{Model: "gpt-6-luna", BaseURL: server.URL}, server.Client())
	defer p.CloseAllSessions()
	s, err := p.StartRun(context.Background(), lunaRequest("run"))
	if err != nil {
		t.Fatal(err)
	}
	<-ready
	if _, err = p.CancelRun(context.Background(), transport.CancelRunRequest{RunID: "run", ProviderSessionRef: &transport.ProviderSessionRef{ResponseRef: "not-a-background-response"}}); err != nil {
		t.Fatal(err)
	}
	done := make(chan struct{})
	go func() {
		for range s {
		}
		close(done)
	}()
	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("cancel failed to unblock")
	}
}

func TestResponsesUsageValidationAndAliases(t *testing.T) {
	for _, raw := range []string{`{"input_tokens":1,"output_tokens":2,"total_tokens":3}`, `{"input_tokens":-1,"output_tokens":2,"total_tokens":1}`, `{"input_tokens":1.5,"output_tokens":2,"total_tokens":3}`, `{"input_tokens":1,"total_tokens":1}`, `{"input_tokens":1,"output_tokens":2,"total_tokens":4}`, `{"input_tokens":1,"output_tokens":2,"total_tokens":3,"output_tokens_details":{"reasoning_tokens":3}}`} {
		var m map[string]any
		_ = json.Unmarshal([]byte(raw), &m)
		u, err := parseUsage(m)
		if strings.Contains(raw, `"input_tokens":1,"output_tokens":2,"total_tokens":3}`) {
			if err != nil || u.TotalTokens != 3 {
				t.Fatal("valid usage denied")
			}
		} else if err == nil {
			t.Fatal("invalid usage accepted")
		}
	}
	names := []string{"memory.search", "memory_search", "butler_literal", strings.Repeat("x", 100), "skills.load"}
	seen := map[string]bool{}
	for _, n := range names {
		alias := toolAlias(n)
		if len(alias) > 64 || seen[alias] {
			t.Fatal("alias collision")
		}
		seen[alias] = true
	}
}

func TestResponsesAuthorityAndOptionsCannotChangeOwnership(t *testing.T) {
	var calls atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) { calls.Add(1) }))
	defer server.Close()
	p, _ := NewProvider(Config{Model: "gpt-6-luna", BaseURL: server.URL, ReasoningEffort: "high"}, server.Client())
	defer p.CloseAllSessions()
	q := lunaRequest("run")
	q.TransportOptions = map[string]any{"model": "other", "input": "injected", "instructions": "injected", "tools": []any{}, "reasoning": map[string]any{"effort": "none"}}
	raw, err := p.startRunBody(q)
	if err != nil {
		t.Fatal(err)
	}
	var m map[string]any
	_ = json.Unmarshal(raw, &m)
	if m["model"] != "gpt-6-luna" || m["instructions"] != "owner policy" || m["reasoning"].(map[string]any)["effort"] != "high" {
		t.Fatal("options replaced ownership/profile")
	}
	ctx := transport.WithInferenceAuthority(context.Background(), func(context.Context) error { return fmt.Errorf("denied") })
	if _, err = p.StartRun(ctx, q); err == nil || calls.Load() != 0 {
		t.Fatal("denied authority reached network")
	}
}

func TestResponsesBoundsWholePrefixAndPreservesHistoricalAliases(t *testing.T) {
	p, _ := NewProvider(Config{Model: "gpt-6-luna", MaximumInputTokens: 1000}, nil)
	defer p.CloseAllSessions()
	p.prefixTokens["run"] = map[string]int64{"resp": 990}
	if p.checkContextAllowance("run", []byte(`{"previous_response_id":"resp","input":"x"}`)) == nil {
		t.Fatal("incremental request ignored complete prefix")
	}
	if p.checkContextAllowance("other", []byte(`{"previous_response_id":"resp"}`)) == nil {
		t.Fatal("cross-run prefix accepted")
	}
	p.rememberTools("run", []transport.ToolDefinition{{Name: "memory.search"}})
	p.rememberTools("run", nil)
	if p.resolveToolName("run", toolAlias("memory.search")) != "memory.search" {
		t.Fatal("loaded-set replacement lost historical alias")
	}
	for _, n := range []any{8193, 1.5, 0, -1} {
		if _, err := p.encodeRequest(map[string]any{"max_output_tokens": n}); err == nil {
			t.Fatal("invalid output cap accepted")
		}
	}
}
