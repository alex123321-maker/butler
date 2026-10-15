package antigravity

import (
	"encoding/json"
	"testing"

	"github.com/butler/butler/internal/memory/readcontrol"
	"github.com/butler/butler/internal/transport"
)

// Canonical Broker contracts must survive the actual native declaration stage,
// before auth resolution, HTTP or spending can produce an upstream observation.
func TestMemoryReadCanonicalNativeSchemaCompatibility(t *testing.T) {
	for _, contract := range readcontrol.Contracts() {
		t.Run(contract.ToolName, func(t *testing.T) {
			declarations, definitions, err := convertTools([]transport.ToolDefinition{{Name: contract.ToolName, Description: contract.Description, SchemaJSON: contract.InputSchemaJson}})
			if err != nil {
				t.Fatalf("canonical tool rejected before HTTP at convertTools: %v", err)
			}
			if len(declarations) != 1 || len(definitions) != 1 {
				t.Fatal("canonical tool disappeared during declaration conversion")
			}
		})
	}
}

func TestNativeToolBoundsLoweringKeepsOriginalContract(t *testing.T) {
	original := `{"type":"object","properties":{"minimum":{"type":"integer","minimum":1,"maximum":10},"nested":{"type":"array","items":{"anyOf":[{"type":"string","minLength":1,"maxLength":32},{"type":"number","minimum":-2.5}]}}},"required":["minimum"],"additionalProperties":false}`
	declarations, definitions, err := convertTools([]transport.ToolDefinition{{Name: "test.bounds", SchemaJSON: original}})
	if err != nil {
		t.Fatal(err)
	}
	if definitions["butler_tool_0"].SchemaJSON != original {
		t.Fatal("logical Broker schema changed")
	}
	var wire map[string]any
	if err = json.Unmarshal(declarations[0].Parameters, &wire); err != nil {
		t.Fatal(err)
	}
	props := wire["properties"].(map[string]any)
	if props["minimum"] == nil || wire["additionalProperties"] != false {
		t.Fatal("real property or object validation lost")
	}
	var check func(map[string]any)
	check = func(schema map[string]any) {
		for _, key := range []string{"minLength", "maxLength", "minimum", "maximum"} {
			if _, ok := schema[key]; ok {
				t.Fatal("bound annotation reached native declaration", key)
			}
		}
		if p, ok := schema["properties"].(map[string]any); ok {
			for _, v := range p {
				check(v.(map[string]any))
			}
		}
		if v, ok := schema["items"].(map[string]any); ok {
			check(v)
		}
		for _, k := range []string{"oneOf", "anyOf"} {
			if vs, ok := schema[k].([]any); ok {
				for _, v := range vs {
					check(v.(map[string]any))
				}
			}
		}
	}
	check(wire)
	if validateArguments(original, json.RawMessage(`{"minimum":2,"unexpected":true}`)) == nil {
		t.Fatal("original structural argument check weakened")
	}
	if _, err := readcontrol.DecodeArgs(readcontrol.Search, `{"query":"calibration","page":10}`); err == nil {
		t.Fatal("owning Broker bounds weakened")
	}
}

func TestNativeToolBoundAnnotationsAndUnknownKeywordsFailClosed(t *testing.T) {
	for _, schema := range []string{
		`{"type":"string","minLength":-1}`, `{"type":"string","maxLength":1.5}`, `{"type":"string","maxLength":"10"}`,
		`{"type":"string","maxLength":9223372036854775808}`, `{"type":"number","minimum":"1"}`, `{"type":"number","maximum":1e999}`,
		`{"type":"string","pattern":".*"}`, `{"type":"number","exclusiveMinimum":1}`, `{"type":"object","$ref":"file:///secret"}`,
	} {
		if _, err := nativeToolSchema(schema); err == nil {
			t.Fatal("unsupported/invalid native tool annotation admitted", schema)
		}
	}
}
