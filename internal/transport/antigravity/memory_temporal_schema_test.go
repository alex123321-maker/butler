package antigravity

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"strings"
	"testing"

	"github.com/butler/butler/internal/memory/factcontrol"
	"github.com/butler/butler/internal/transport"
)

func TestTemporalFactCanonicalNativeSchemaCompatibility(t *testing.T) {
	c := factcontrol.Contract()
	_, definitions, err := convertTools([]transport.ToolDefinition{{Name: c.ToolName, Description: c.Description, SchemaJSON: c.InputSchemaJson}})
	if err != nil {
		t.Fatal("canonical memory.facts rejected before auth/HTTP", err)
	}
	if definitions["butler_tool_0"].SchemaJSON != c.InputSchemaJson {
		t.Fatal("canonical argument/authority contract changed")
	}
}

func TestTemporalNativeFactProposalEnforcesOriginalArrayBeforeAction(t *testing.T) {
	for _, n := range []int{0, 1, 8, 9} {
		t.Run(fmt.Sprint(n), func(t *testing.T) {
			refs := make([]map[string]any, n)
			for i := range refs {
				refs[i] = map[string]any{"source_id": fmt.Sprintf("source-%d", i), "revision": 1, "access_revision": 1, "content_hash": strings.Repeat("a", 64)}
			}
			args, _ := json.Marshal(map[string]any{"sources": refs})
			fakeHTTP := 0
			p := newFixture(t, func(r *http.Request) (*http.Response, error) {
				fakeHTTP++
				var request cloudRequestEnvelope
				if json.NewDecoder(r.Body).Decode(&request) != nil || request.Model != FlashHighModel {
					t.Fatal("controlled native request malformed")
				}
				return response(cloudFrame(`{"functionCall":{"name":"butler_tool_0","args":`+string(args)+`},"thoughtSignature":"controlled-signature"}`, "STOP")), nil
			})
			p.config.AuthSource = cloudAuthSource{project: "controlled-project"}
			c := factcontrol.Contract()
			r := request("temporal-array-" + fmt.Sprint(n))
			r.Context.ModelName = FlashHighModel
			r.ToolDefinitions = []transport.ToolDefinition{{Name: c.ToolName, Description: c.Description, SchemaJSON: c.InputSchemaJson}}
			stream, err := p.StartRun(context.Background(), r)
			if err != nil {
				t.Fatal("native declaration stopped before controlled transport", err)
			}
			events := collect(t, stream)
			proposals := 0
			for _, event := range events {
				if event.ToolCall != nil {
					proposals++
					if event.ToolCall.ToolName != c.ToolName || validateArguments(c.InputSchemaJson, json.RawMessage(event.ToolCall.ArgsJSON)) != nil {
						t.Fatal("original argument contract changed")
					}
				}
			}
			if fakeHTTP != 1 {
				t.Fatal("controlled native transport repeated request")
			}
			if n == 1 || n == 8 {
				if proposals != 1 || findError(events) != nil {
					t.Fatal("valid canonical facts proposal lost")
				}
			} else {
				if proposals != 0 || findError(events) == nil {
					t.Fatal("out-of-bound proposal escaped adapter")
				}
			}
		})
	}
}

func TestTemporalNativeArrayBoundsRetainExactLocalArguments(t *testing.T) {
	c := factcontrol.Contract()
	declarations, definitions, err := convertTools([]transport.ToolDefinition{{Name: c.ToolName, SchemaJSON: c.InputSchemaJson}})
	if err != nil || definitions["butler_tool_0"].SchemaJSON != c.InputSchemaJson {
		t.Fatal("original canonical contract lost", err)
	}
	var wire map[string]any
	if json.Unmarshal(declarations[0].Parameters, &wire) != nil {
		t.Fatal("native declaration invalid")
	}
	sources := wire["properties"].(map[string]any)["sources"].(map[string]any)
	if sources["minItems"] != nil || sources["maxItems"] != nil || sources["items"] == nil {
		t.Fatal("native lowering changed array shape")
	}
	for _, n := range []int{0, 1, 8, 9} {
		t.Run(fmt.Sprint(n), func(t *testing.T) {
			refs := make([]map[string]any, n)
			for i := range refs {
				refs[i] = map[string]any{"source_id": fmt.Sprintf("source-%d", i), "revision": 1, "access_revision": 1, "content_hash": strings.Repeat("a", 64)}
			}
			raw, _ := json.Marshal(map[string]any{"sources": refs})
			err := validateArguments(c.InputSchemaJson, raw)
			if (err == nil) != (n >= 1 && n <= 8) {
				t.Fatal("exact original cardinality not enforced before proposal", n, err)
			}
			if _, decoderErr := factcontrol.DecodeArgs(string(raw)); (decoderErr == nil) != (err == nil) {
				t.Fatal("transport and owning bounded decoder disagree")
			}
		})
	}
}

func TestTemporalNativeNestedArrayBoundsAndKeywordPropertyNames(t *testing.T) {
	schema := `{"type":"object","properties":{"minItems":{"type":"string"},"outer":{"type":"array","minItems":1,"maxItems":2,"items":{"anyOf":[{"type":"array","minItems":1,"maxItems":2,"items":{"type":"string"}},{"type":"boolean"}]}}},"required":["minItems","outer"],"additionalProperties":false}`
	wire, err := nativeToolSchema(schema)
	if err != nil {
		t.Fatal(err)
	}
	var decoded map[string]any
	if json.Unmarshal(wire, &decoded) != nil {
		t.Fatal("wire invalid")
	}
	if decoded["properties"].(map[string]any)["minItems"] == nil {
		t.Fatal("real property erased")
	}
	for _, raw := range []string{`{"minItems":"literal","outer":[["a"]]}`, `{"minItems":"literal","outer":[true,["a","b"]]}`} {
		if err := validateArguments(schema, json.RawMessage(raw)); err != nil {
			t.Fatal("valid nested array refused", err)
		}
	}
	for _, raw := range []string{`{"minItems":"literal","outer":[]}`, `{"minItems":"literal","outer":[[]]}`, `{"minItems":"literal","outer":[["a","b","c"]]}`, `{"minItems":"literal","outer":[true,true,true]}`} {
		if validateArguments(schema, json.RawMessage(raw)) == nil {
			t.Fatal("nested original cardinality widened", raw)
		}
	}
}

func TestTemporalNativeMalformedArrayBoundsFailClosed(t *testing.T) {
	for _, schema := range []string{
		`{"type":"array","minItems":-1}`, `{"type":"array","maxItems":1.5}`, `{"type":"array","minItems":"1"}`, `{"type":"array","minItems":true}`, `{"type":"array","maxItems":[]}`, `{"type":"array","maxItems":{}}`,
		`{"type":"array","minItems":NaN}`, `{"type":"array","maxItems":1e999}`, `{"type":"array","maxItems":9223372036854775808}`, `{"type":"array","minItems":2,"maxItems":1}`, `{"type":"object","maxItems":2}`, `{"minItems":1}`, `{"type":["array","null"],"maxItems":2}`, `{"type":"array","uniqueItems":true}`,
	} {
		if _, err := nativeToolSchema(schema); err == nil {
			t.Fatal("malformed/unsupported array bound admitted", schema)
		}
	}
	if validateSchema(`{"type":"array","minItems":1,"maxItems":8,"items":{"type":"string"}}`) == nil {
		t.Fatal("general schema dialect silently changed")
	}
}

// Optional exact failed-request capture is synthetic and contains no credentials.
// convertInputs/convertTools cannot perform auth or HTTP. This proves the precise
// local phase without replaying an original input against the provider.
func TestTemporalFailedV1RequestPreHTTPDiagnostic(t *testing.T) {
	path := os.Getenv("BUTLER_TEMPORAL_FAILED_V1_REQUEST")
	if path == "" {
		t.Skip("explicit immutable failed-request diagnostic fixture required")
	}
	raw, err := os.ReadFile(path)
	var captured struct {
		Start transport.StartRunRequest `json:"start"`
	}
	if err != nil || json.Unmarshal(raw, &captured) != nil {
		t.Fatal("captured original unavailable")
	}
	r := captured.Start
	if err := r.Validate(); err != nil {
		t.Fatal("actual request validation phase", err)
	}
	if r.Context.ProviderName != Name || r.Context.ModelName != FlashHighModel || r.Context.ProviderSessionRef != nil || len(r.TransportOptions) != 0 || r.TransportOptionsJSON != "" {
		t.Fatal("actual context phase differs")
	}
	if _, _, err := convertInputs(r.InputItems); err != nil {
		t.Fatal("actual input conversion phase", err)
	}
	for _, tool := range r.ToolDefinitions {
		if _, err := nativeToolSchema(tool.SchemaJSON); err != nil {
			t.Errorf("actual tool=%s native-declaration validation: %v", tool.Name, err)
		}
	}
}
