package githubcopilot

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"

	"github.com/butler/butler/internal/transport"
)

func TestToolSetReplacementOnCopilotWire(t *testing.T) {
	var calls atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var body struct {
			Tools []json.RawMessage `json:"tools"`
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Error(err)
		}
		w.Header().Set("Content-Type", "text/event-stream")
		if calls.Add(1) == 1 {
			if len(body.Tools) != 1 {
				t.Error("initial schemas leaked")
			}
			fmt.Fprint(w, "data: {\"choices\":[{\"delta\":{\"tool_calls\":[{\"index\":0,\"id\":\"load\",\"type\":\"function\",\"function\":{\"name\":\"tools.load\",\"arguments\":\"{}\"}}]}}]}\n\ndata: {\"choices\":[{\"finish_reason\":\"tool_calls\"}]}\n\n")
		} else {
			if len(body.Tools) != 2 {
				t.Error("loaded schema absent")
			}
			fmt.Fprint(w, "data: {\"choices\":[{\"delta\":{\"content\":\"done\"}}]}\n\ndata: {\"choices\":[{\"finish_reason\":\"stop\"}]}\n\n")
		}
	}))
	defer server.Close()
	p, err := NewProvider(Config{Model: "fixture", AuthSource: staticCopilotAuth{baseURL: server.URL}}, server.Client())
	if err != nil {
		t.Fatal(err)
	}
	defs := []transport.ToolDefinition{{Name: "tools.load", SchemaJSON: `{"type":"object"}`}}
	stream, err := p.StartRun(context.Background(), transport.StartRunRequest{Context: transport.TransportRunContext{RunID: "run", SessionKey: "session", ProviderName: providerName, ModelName: "fixture"}, InputItems: []transport.InputItem{{Role: "user", Content: "work"}}, ToolDefinitions: defs})
	if err != nil {
		t.Fatal(err)
	}
	call := readSingleToolCall(t, stream)
	defs = append(defs, transport.ToolDefinition{Name: "doctor.check_system", SchemaJSON: `{"type":"object"}`})
	stream, err = p.SubmitToolResult(context.Background(), transport.SubmitToolResultRequest{RunID: "run", ProviderSessionRef: &transport.ProviderSessionRef{SessionRef: "session"}, ToolCallRef: call.ToolCallRef, ToolResultJSON: `{}`, ToolSet: transport.NewToolSetSnapshot(defs)})
	if err != nil {
		t.Fatal(err)
	}
	for ev := range stream {
		if ev.TransportError != nil {
			t.Fatal(ev.TransportError)
		}
	}
	if calls.Load() != 2 {
		t.Fatal("unexpected wire calls")
	}
}
