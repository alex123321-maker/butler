package antigravity

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/butler/butler/internal/transport"
)

func TestHighStructuredGenerationUsesFrozenAllowance(t *testing.T) {
	schema := json.RawMessage(`{"type":"object","properties":{"ok":{"type":"boolean"}},"required":["ok"],"additionalProperties":false}`)
	want := string(schema)
	p, err := NewProvider(Config{Model: FlashHighModel, AuthSource: cloudAuthSource{project: "discovered-project"}, MaximumOutputTokens: 32768, JSONResponseSchema: schema})
	if err != nil {
		t.Fatal(err)
	}
	// Mutating the constructor input must not mutate the trusted wire schema.
	schema[0] = 'x'
	var calls atomic.Int32
	fixture := newFixture(t, func(req *http.Request) (*http.Response, error) {
		calls.Add(1)
		var wire cloudRequestEnvelope
		if json.NewDecoder(req.Body).Decode(&wire) != nil {
			t.Fatal("invalid generation wire")
		}
		g := wire.Request.GenerationConfig
		if req.URL.String() != cloudEndpoint+":streamGenerateContent?alt=sse" || wire.Model != FlashHighModel ||
			g.MaxOutputTokens != 32768 || g.ThinkingConfig == nil || g.ThinkingConfig.ThinkingBudget != -1 ||
			g.ResponseMIMEType != "application/json" || string(g.ResponseJSONSchema) != want || len(wire.Request.Tools) != 0 {
			t.Error("frozen High thinking/schema/ceiling changed")
		}
		return response(cloudFrame(`{"text":"{\"ok\":true}"}`, "STOP")), nil
	})
	p.client = fixture.client
	in, out := p.InferenceLimits()
	if in != 1048576 || out != 32768 {
		t.Fatal("kernel reservation does not include the actual generated ceiling")
	}
	r := request("json-observation")
	r.Context.ModelName, r.ToolDefinitions = FlashHighModel, nil
	stream, err := p.StartRun(context.Background(), r)
	if err != nil || findError(collect(t, stream)) != nil || calls.Load() != 1 {
		t.Fatal("structured High lifecycle denied", err)
	}
	for name, mutate := range map[string]func(*transport.StartRunRequest){
		"foreign_model":  func(r *transport.StartRunRequest) { r.Context.ModelName = DefaultModel },
		"tool_authority": func(r *transport.StartRunRequest) { r.ToolDefinitions = request("template").ToolDefinitions },
		"map_override":   func(r *transport.StartRunRequest) { r.TransportOptions = map[string]any{"maxOutputTokens": 1} },
		"json_override":  func(r *transport.StartRunRequest) { r.TransportOptionsJSON = `{"responseMimeType":"text/plain"}` },
	} {
		t.Run(name, func(t *testing.T) {
			r := request("denied-" + name)
			r.Context.ModelName, r.ToolDefinitions = FlashHighModel, nil
			mutate(&r)
			if _, err := p.StartRun(context.Background(), r); err == nil || calls.Load() != 1 {
				t.Fatal("untrusted generation change reached network")
			}
		})
	}
}

func TestConfiguredGenerationRejectsUnsupportedSchema(t *testing.T) {
	for name, cfg := range map[string]Config{
		"negative":           {MaximumOutputTokens: -1},
		"oversized":          {MaximumOutputTokens: 32769},
		"tiered_ceiling":     {Model: DefaultModel, MaximumOutputTokens: 32768},
		"tiered_schema":      {Model: DefaultModel, JSONResponseSchema: json.RawMessage(`{"type":"object"}`)},
		"duplicate":          {JSONResponseSchema: json.RawMessage(`{"type":"object","type":"object"}`)},
		"array":              {JSONResponseSchema: json.RawMessage(`{"type":"array"}`)},
		"external_reference": {JSONResponseSchema: json.RawMessage(`{"type":"object","properties":{"x":{"$ref":"https://example.invalid/private"}}}`)},
		"local_reference":    {JSONResponseSchema: json.RawMessage(`{"type":"object","$ref":"#/$defs/x"}`)},
		"too_large":          {JSONResponseSchema: json.RawMessage(`{"type":"object","description":"` + strings.Repeat("x", 16384) + `"}`)},
	} {
		t.Run(name, func(t *testing.T) {
			if cfg.Model == "" {
				cfg.Model = FlashHighModel
			}
			cfg.AuthSource = cloudAuthSource{project: "discovered-project"}
			if _, err := NewProvider(cfg); err == nil {
				t.Fatal("invalid trusted generation configuration accepted")
			}
		})
	}
}

func TestHighStructuredRejectionDoesNotRetryOrFallback(t *testing.T) {
	p, err := NewProvider(Config{Model: FlashHighModel, AuthSource: cloudAuthSource{project: "discovered-project"}, MaximumOutputTokens: 32768, JSONResponseSchema: json.RawMessage(`{"type":"object"}`)})
	if err != nil {
		t.Fatal(err)
	}
	var calls atomic.Int32
	fixture := newFixture(t, func(*http.Request) (*http.Response, error) {
		calls.Add(1)
		return &http.Response{StatusCode: 400, Body: http.NoBody, Header: make(http.Header)}, nil
	})
	p.client = fixture.client
	r := request("reject-structured")
	r.Context.ModelName, r.ToolDefinitions = FlashHighModel, nil
	stream, err := p.StartRun(context.Background(), r)
	if err != nil {
		t.Fatal(err)
	}
	events := collect(t, stream)
	if calls.Load() != 1 || findError(events) == nil {
		t.Fatal("rejected schema changed request or retried")
	}
}
