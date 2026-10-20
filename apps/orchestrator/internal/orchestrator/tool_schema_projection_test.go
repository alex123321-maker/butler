package orchestrator

import (
	"encoding/json"
	"strings"
	"testing"

	toolbrokerv1 "github.com/butler/butler/internal/gen/toolbroker/v1"
	"github.com/butler/butler/internal/memory/adaptive"
	"github.com/butler/butler/internal/memory/readcontrol"
	"github.com/butler/butler/internal/toolcatalog"
)

func TestToolSchemaProjectionRespectsOwningAdaptiveComposition(t *testing.T) {
	contract := readcontrol.Contract(readcontrol.Search)
	ref := toolcatalog.Ref(contract)
	for _, ready := range []bool{false, true} {
		s := &Service{}
		if ready {
			s.config.AdaptiveRetrieval = &adaptive.Store{}
		}
		defs, err := s.modelToolDefinitions([]*toolbrokerv1.ToolContract{contract})
		if err != nil || len(defs) != 1 {
			t.Fatal("search projection failed", err)
		}
		var schema struct {
			Properties map[string]json.RawMessage `json:"properties"`
		}
		if json.Unmarshal([]byte(defs[0].SchemaJSON), &schema) != nil {
			t.Fatal("invalid native schema")
		}
		var strategy struct {
			Enum []string `json:"enum"`
		}
		_ = json.Unmarshal(schema.Properties["strategy"], &strategy)
		if strings.Contains(strings.Join(strategy.Enum, ","), "semantic_hybrid") != ready || (schema.Properties["event_from"] != nil) != ready || (schema.Properties["event_until"] != nil) != ready {
			t.Fatal("advertised unavailable adaptive options", ready, strategy.Enum)
		}
		if !ready && (len(strategy.Enum) != 1 || strategy.Enum[0] != "catalog_exact") {
			t.Fatal("exact search missing")
		}
		if toolcatalog.Ref(contract) != ref || !strings.Contains(contract.InputSchemaJson, "semantic_hybrid") {
			t.Fatal("native projection changed canonical authority/ref")
		}
	}
}

func TestToolSchemaProjectionSupportsIndependentLocalSemanticSearch(t *testing.T) {
	s := &Service{config: Config{SemanticSearchEnabled: true}}
	defs, e := s.modelToolDefinitions([]*toolbrokerv1.ToolContract{readcontrol.Contract(readcontrol.Search)})
	if e != nil || len(defs) != 1 {
		t.Fatal(e)
	}
	if !strings.Contains(defs[0].SchemaJSON, "semantic_hybrid") || strings.Contains(defs[0].SchemaJSON, "event_from") {
		t.Fatal("semantic composition enabled unqualified temporal/adaptive options")
	}
}
