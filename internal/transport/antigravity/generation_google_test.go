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

const googleObservationSchema = `{"type":"OBJECT","properties":{"values":{"type":"ARRAY","items":{"type":"STRING"},"minItems":"0","maxItems":"8"},"source_id":{"type":"STRING","enum":["public_a","public_b"],"nullable":true},"checked":{"type":"BOOLEAN"},"count":{"type":"INTEGER"}},"required":["values","source_id","checked","count"]}`

func TestHighGoogleResponseSchemaUsesExclusiveFrozenNativeDialect(t *testing.T) {
	schema := json.RawMessage(googleObservationSchema)
	p, err := NewProvider(Config{Model: FlashHighModel, AuthSource: cloudAuthSource{project: "discovered-project"}, MaximumOutputTokens: 32768, GoogleResponseSchema: schema})
	if err != nil {
		t.Fatal(err)
	}
	schema[0] = 'x'
	var calls atomic.Int32
	fixture := newFixture(t, func(req *http.Request) (*http.Response, error) {
		calls.Add(1)
		var wire cloudRequestEnvelope
		if json.NewDecoder(req.Body).Decode(&wire) != nil {
			t.Fatal("invalid native dialect wire")
		}
		g := wire.Request.GenerationConfig
		if req.URL.String() != cloudEndpoint+":streamGenerateContent?alt=sse" || wire.Model != FlashHighModel || g.MaxOutputTokens != 32768 ||
			g.ThinkingConfig == nil || g.ThinkingConfig.ThinkingBudget != -1 || g.ResponseMIMEType != "application/json" ||
			string(g.ResponseSchema) != googleObservationSchema || len(g.ResponseJSONSchema) != 0 || len(wire.Request.Tools) != 0 || len(wire.Request.ToolConfig) != 0 {
			t.Error("native schema changed model/effort/allowance or mixed dialects/tools")
		}
		return response(cloudFrame(`{"text":"{\"values\":[],\"source_id\":null,\"checked\":true,\"count\":0}"}`, "STOP")), nil
	})
	p.client = fixture.client
	in, out := p.InferenceLimits()
	if in != 1048576 || out != 32768 {
		t.Fatal("configured complete ceiling does not match kernel reservation")
	}
	r := request("native-json-observation")
	r.Context.ModelName, r.ToolDefinitions = FlashHighModel, nil
	stream, err := p.StartRun(context.Background(), r)
	if err != nil || findError(collect(t, stream)) != nil || calls.Load() != 1 {
		t.Fatal("native dialect synthetic lifecycle denied", err)
	}
	for name, mutate := range map[string]func(*transport.StartRunRequest){
		"foreign_model": func(r *transport.StartRunRequest) { r.Context.ModelName = DefaultModel },
		"tools":         func(r *transport.StartRunRequest) { r.ToolDefinitions = request("template").ToolDefinitions },
		"map_override": func(r *transport.StartRunRequest) {
			r.TransportOptions = map[string]any{"responseSchema": map[string]any{"type": "OBJECT"}}
		},
		"json_override": func(r *transport.StartRunRequest) {
			r.TransportOptionsJSON = `{"responseJsonSchema":{"type":"object"}}`
		},
	} {
		t.Run(name, func(t *testing.T) {
			r := request("native-denied-" + name)
			r.Context.ModelName, r.ToolDefinitions = FlashHighModel, nil
			mutate(&r)
			if _, err := p.StartRun(context.Background(), r); err == nil || calls.Load() != 1 {
				t.Fatal("untrusted native schema configuration reached network")
			}
		})
	}
}

func TestGoogleResponseSchemaRejectsUnsupportedConstruction(t *testing.T) {
	property := func(text string) string { return `{"type":"OBJECT","properties":{"value":` + text + `}}` }
	deep := `{"type":"STRING"}`
	for i := 0; i < 9; i++ {
		deep = property(deep)
	}
	wide := map[string]any{}
	for i := 0; i < 32; i++ {
		wide["field_"+strings.Repeat("a", i+1)] = json.RawMessage(property(`{"type":"OBJECT","properties":{"x":{"type":"STRING"},"y":{"type":"STRING"},"z":{"type":"STRING"},"a":{"type":"STRING"},"b":{"type":"STRING"},"c":{"type":"STRING"}}}`))
	}
	wideRaw, _ := json.Marshal(map[string]any{"type": "OBJECT", "properties": wide})
	for name, raw := range map[string]string{
		"duplicate_key":             `{"type":"OBJECT","type":"OBJECT","properties":{"x":{"type":"STRING"}}}`,
		"wrong_root":                `{"type":"ARRAY","items":{"type":"STRING"},"maxItems":"1"}`,
		"lowercase_type":            property(`{"type":"string"}`),
		"reference":                 property(`{"type":"STRING","$ref":"https://example.invalid/schema"}`),
		"annotation":                property(`{"type":"STRING","description":"untrusted instruction"}`),
		"union":                     property(`{"anyOf":[{"type":"STRING"},{"type":"INTEGER"}]}`),
		"extra_keyword":             `{"type":"OBJECT","properties":{"x":{"type":"STRING"}},"additionalProperties":false}`,
		"empty_properties":          `{"type":"OBJECT","properties":{}}`,
		"foreign_required":          `{"type":"OBJECT","properties":{"x":{"type":"STRING"}},"required":["y"]}`,
		"duplicate_required":        `{"type":"OBJECT","properties":{"x":{"type":"STRING"}},"required":["x","x"]}`,
		"invalid_nullable":          property(`{"type":"STRING","nullable":"true"}`),
		"duplicate_enum":            property(`{"type":"STRING","enum":["a","a"]}`),
		"numeric_enum":              property(`{"type":"STRING","enum":[1]}`),
		"missing_array_bound":       property(`{"type":"ARRAY","items":{"type":"STRING"}}`),
		"numeric_array_bound":       property(`{"type":"ARRAY","items":{"type":"STRING"},"maxItems":8}`),
		"noncanonical_array_bound":  property(`{"type":"ARRAY","items":{"type":"STRING"},"maxItems":"08"}`),
		"oversized_array_bound":     property(`{"type":"ARRAY","items":{"type":"STRING"},"maxItems":"129"}`),
		"contradictory_array_bound": property(`{"type":"ARRAY","items":{"type":"STRING"},"minItems":"2","maxItems":"1"}`),
		"oversized_schema":          property(`{"type":"STRING","enum":["` + strings.Repeat("x", 16384) + `"]}`),
		"deep":                      deep,
		"too_many_nodes":            string(wideRaw),
	} {
		t.Run(name, func(t *testing.T) {
			if _, err := NewProvider(Config{Model: FlashHighModel, AuthSource: cloudAuthSource{project: "discovered-project"}, MaximumOutputTokens: 32768, GoogleResponseSchema: json.RawMessage(raw)}); err == nil {
				t.Fatal("unsupported trusted native schema accepted")
			}
		})
	}
	for name, cfg := range map[string]Config{
		"foreign_model":  {Model: DefaultModel, GoogleResponseSchema: json.RawMessage(googleObservationSchema)},
		"mixed_dialects": {Model: FlashHighModel, JSONResponseSchema: json.RawMessage(`{"type":"object"}`), GoogleResponseSchema: json.RawMessage(googleObservationSchema)},
	} {
		t.Run(name, func(t *testing.T) {
			cfg.AuthSource = cloudAuthSource{project: "discovered-project"}
			if _, err := NewProvider(cfg); err == nil {
				t.Fatal("foreign or mixed response dialect configured")
			}
		})
	}
}

func TestGoogleResponseSchemaDoesNotRetryFallbackOrRepair(t *testing.T) {
	for _, status := range []int{http.StatusBadRequest, http.StatusOK} {
		t.Run(http.StatusText(status), func(t *testing.T) {
			p, err := NewProvider(Config{Model: FlashHighModel, AuthSource: cloudAuthSource{project: "discovered-project"}, MaximumOutputTokens: 32768, GoogleResponseSchema: json.RawMessage(googleObservationSchema)})
			if err != nil {
				t.Fatal(err)
			}
			var calls atomic.Int32
			const fenced = "```json\n{\"checked\":true}\n```"
			fixture := newFixture(t, func(*http.Request) (*http.Response, error) {
				calls.Add(1)
				if status == http.StatusBadRequest {
					return &http.Response{StatusCode: status, Body: http.NoBody, Header: make(http.Header)}, nil
				}
				part, _ := json.Marshal(map[string]any{"text": fenced})
				return response(cloudFrame(string(part), "STOP")), nil
			})
			p.client = fixture.client
			r := request("native-no-fallback")
			r.Context.ModelName, r.ToolDefinitions = FlashHighModel, nil
			stream, err := p.StartRun(context.Background(), r)
			if err != nil {
				t.Fatal(err)
			}
			events := collect(t, stream)
			if calls.Load() != 1 || status == http.StatusBadRequest && findError(events) == nil {
				t.Fatal("rejection retried or fell back")
			}
			if status == http.StatusOK {
				finals := 0
				for _, event := range events {
					if event.AssistantFinal != nil {
						finals++
						if event.AssistantFinal.Content != fenced {
							t.Fatal("native schema dialect repaired the provider's original answer")
						}
					}
				}
				if finals != 1 || findError(events) != nil {
					t.Fatal("synthetic original final was lost")
				}
			}
		})
	}
}
