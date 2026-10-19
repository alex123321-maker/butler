package antigravity

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/butler/butler/internal/toolcatalog"
	"github.com/butler/butler/internal/transport"
)

func TestToolDiscoveryOwnedSchemasFitNativeDialect(t *testing.T) {
	defs := []transport.ToolDefinition{}
	for _, c := range toolcatalog.Contracts() {
		defs = append(defs, transport.ToolDefinition{Name: c.ToolName, Description: c.Description, SchemaJSON: c.InputSchemaJson})
	}
	if _, _, err := convertTools(defs); err != nil {
		t.Fatal("fixed discovery schema incompatible with native transport", err)
	}
}

func TestToolSetLoadsOnNextWireRequestWithStableSignedAliases(t *testing.T) {
	var count atomic.Int32
	p := newFixture(t, func(req *http.Request) (*http.Response, error) {
		var body requestEnvelope
		if err := json.NewDecoder(req.Body).Decode(&body); err != nil {
			t.Error(err)
		}
		defs := body.Tools[0].FunctionDeclarations
		switch count.Add(1) {
		case 1:
			if len(defs) != 1 {
				t.Error("initial schemas leaked")
			}
			return response(frame(`{"functionCall":{"id":"load","name":"butler_tool_0","args":{}},"thoughtSignature":"load-signature"}`, "STOP")), nil
		case 2:
			if len(defs) != 2 || defs[0].Name != "butler_tool_1" || defs[1].Name != "butler_tool_0" {
				t.Errorf("aliases shifted: %+v", defs)
			}
			encoded, _ := json.Marshal(body.Contents)
			if !strings.Contains(string(encoded), "load-signature") {
				t.Error("signed history lost")
			}
			return response(frame(`{"functionCall":{"id":"use","name":"butler_tool_1","args":{"url":"https://example.com"}},"thoughtSignature":"use-signature"}`, "STOP")), nil
		default:
			if len(defs) != 1 || defs[0].Name != "butler_tool_0" {
				t.Error("removed schema retained")
			}
			return response(frame(`{"text":"done"}`, "STOP")), nil
		}
	})
	req := request("dynamic-tools")
	load := transport.ToolDefinition{Name: "tools.load", Description: "load", SchemaJSON: `{"type":"object"}`}
	use := transport.ToolDefinition{Name: "browser.read", SchemaJSON: `{"type":"object","properties":{"url":{"type":"string"}},"required":["url"]}`}
	req.ToolDefinitions = []transport.ToolDefinition{load}
	stream, err := p.StartRun(context.Background(), req)
	if err != nil {
		t.Fatal(err)
	}
	for step := 0; step < 2; step++ {
		events := collect(t, stream)
		if err := findError(events); err != nil {
			t.Fatal(err)
		}
		var call *transport.ToolCallRequest
		var ref *transport.ProviderSessionRef
		for _, ev := range events {
			if ev.ToolCall != nil {
				call = ev.ToolCall
			}
			if ev.ProviderSessionRef != nil {
				ref = ev.ProviderSessionRef
			}
		}
		if call == nil {
			t.Fatal("missing native proposal")
		}
		defs := []transport.ToolDefinition{use, load}
		if step == 1 {
			defs = []transport.ToolDefinition{load}
		}
		stream, err = p.SubmitToolResult(context.Background(), transport.SubmitToolResultRequest{RunID: req.Context.RunID, ProviderSessionRef: ref, ToolCallRef: call.ToolCallRef, ToolResultJSON: `{"ok":true}`, ToolSet: transport.NewToolSetSnapshot(defs)})
		if err != nil {
			t.Fatal(err)
		}
	}
	if err := findError(collect(t, stream)); err != nil {
		t.Fatal(err)
	}
	if count.Load() != 3 {
		t.Fatal("unexpected requests")
	}
}
