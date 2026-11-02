package antigravity

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/butler/butler/internal/toolcatalog"
	"github.com/butler/butler/internal/transport"
)

func TestCatalogLoadWireRetainsVisibleLimitsAndExactLocalContract(t *testing.T) {
	c := toolcatalog.Contract(toolcatalog.Load)
	declarations, definitions, err := convertTools([]transport.ToolDefinition{{Name: c.ToolName, Description: c.Description, SchemaJSON: c.InputSchemaJson}})
	if err != nil {
		t.Fatal(err)
	}
	var wire map[string]any
	if json.Unmarshal(declarations[0].Parameters, &wire) != nil {
		t.Fatal("invalid native schema")
	}
	refs := wire["properties"].(map[string]any)["tool_refs"].(map[string]any)
	if refs["description"] != "Butler argument limits: minItems=1; maxItems=8." || refs["minItems"] != nil || refs["maxItems"] != nil {
		t.Fatal("load batch bounds disappeared from model-facing schema")
	}
	if definitions["butler_tool_0"].SchemaJSON != c.InputSchemaJson {
		t.Fatal("canonical contract changed")
	}
	for _, n := range []int{0, 1, 8, 9} {
		items := make([]string, n)
		for i := range items {
			items[i] = "test.method@" + strings.Repeat("a", 64)
		}
		args, _ := json.Marshal(map[string]any{"tool_refs": items})
		part := marshalPart(map[string]any{"functionCall": map[string]any{"name": "butler_tool_0", "args": json.RawMessage(args)}})
		call, err := validateFunctionCalls([]json.RawMessage{part}, definitions)
		if n == 0 || n == 9 {
			if call != nil || transport.DiagnosticReason(err) != "native_arguments_array_bounds" {
				t.Fatal("invalid batch escaped or diagnosis lost")
			}
		} else if err != nil || call == nil || string(call.Args) != string(args) {
			t.Fatal("valid batch changed or rejected")
		}
	}
}

func TestNestedBoundsHintsPreserveDescriptionsAndResponseDialect(t *testing.T) {
	original := `{"type":"object","properties":{"minimum":{"type":"integer","description":"Existing guidance.","minimum":1,"maximum":8},"values":{"type":"array","items":{"anyOf":[{"type":"string","minLength":2,"maxLength":20},{"type":"number","minimum":-2.5}]}}}}`
	wire, err := nativeToolSchema(original)
	if err != nil {
		t.Fatal(err)
	}
	var schema map[string]any
	if json.Unmarshal(wire, &schema) != nil {
		t.Fatal("invalid native schema")
	}
	properties := schema["properties"].(map[string]any)
	if properties["minimum"].(map[string]any)["description"] != "Existing guidance.\nButler argument limits: minimum=1; maximum=8." {
		t.Fatal("description or property name overwritten")
	}
	variants := properties["values"].(map[string]any)["items"].(map[string]any)["anyOf"].([]any)
	if variants[0].(map[string]any)["description"] != "Butler argument limits: minLength=2; maxLength=20." || variants[1].(map[string]any)["description"] != "Butler argument limits: minimum=-2.5." {
		t.Fatal("nested/union limits lost")
	}
	if validateSchema(original) == nil {
		t.Fatal("response schema dialect widened")
	}
}
