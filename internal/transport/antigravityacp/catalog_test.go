package antigravityacp

import (
	"encoding/json"
	"os"
	"testing"

	"github.com/butler/butler/internal/memory/factcontrol"
	"github.com/butler/butler/internal/memory/readcontrol"
	"github.com/butler/butler/internal/transport"
)

func TestACPAcceptsCanonicalButlerToolCatalog(t *testing.T) {
	raw, err := os.ReadFile("../../../configs/tools.json")
	if err != nil {
		t.Fatal(err)
	}
	var catalog struct {
		Tools []struct {
			Name   string `json:"tool_name"`
			Schema string `json:"input_schema_json"`
		} `json:"tools"`
	}
	if err = json.Unmarshal(raw, &catalog); err != nil || len(catalog.Tools) == 0 {
		t.Fatal("missing canonical catalog", err)
	}
	for _, tool := range catalog.Tools {
		t.Run(tool.Name, func(t *testing.T) {
			if err := validateTools([]transport.ToolDefinition{{Name: tool.Name, SchemaJSON: tool.Schema}}); err != nil {
				t.Fatal("canonical tool rejected", err)
			}
		})
	}
	for _, contract := range readcontrol.Contracts() {
		if err := validateTools([]transport.ToolDefinition{{Name: contract.ToolName, SchemaJSON: contract.InputSchemaJson}}); err != nil {
			t.Fatal(contract.ToolName, err)
		}
	}
	for _, contract := range []struct{ ToolName, InputSchemaJson string }{{factcontrol.Contract().ToolName, factcontrol.Contract().InputSchemaJson}} {
		if err := validateTools([]transport.ToolDefinition{{Name: contract.ToolName, SchemaJSON: contract.InputSchemaJson}}); err != nil {
			t.Fatal(contract.ToolName, err)
		}
	}
}
