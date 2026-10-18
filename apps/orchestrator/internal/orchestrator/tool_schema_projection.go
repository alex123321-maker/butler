package orchestrator

import (
	"encoding/json"
	"fmt"

	toolbrokerv1 "github.com/butler/butler/internal/gen/toolbroker/v1"
	"github.com/butler/butler/internal/memory/readcontrol"
	"github.com/butler/butler/internal/transport"
	"google.golang.org/protobuf/proto"
)

// Advertise only options composed in the owning read service. This narrows model
// presentation; canonical contracts/refs and every Broker authority check remain
// unchanged. In particular an unqualified adaptive reader is never enabled here.
func (s *Service) modelToolDefinitions(contracts []*toolbrokerv1.ToolContract) ([]transport.ToolDefinition, error) {
	defs := toolDefinitionsFromContracts(contracts)
	if s.config.AdaptiveRetrieval != nil {
		return defs, nil
	}
	ownedSearch := false
	for _, c := range contracts {
		if c != nil && c.ToolName == readcontrol.Search && proto.Equal(c, readcontrol.Contract(readcontrol.Search)) {
			ownedSearch = true
		}
	}
	if !ownedSearch {
		return defs, nil
	}
	for i := range defs {
		if defs[i].Name != readcontrol.Search {
			continue
		}
		var schema map[string]any
		if json.Unmarshal([]byte(defs[i].SchemaJSON), &schema) != nil {
			return nil, fmt.Errorf("owned memory search schema unavailable")
		}
		properties, ok := schema["properties"].(map[string]any)
		if !ok {
			return nil, fmt.Errorf("owned memory search properties unavailable")
		}
		properties["strategy"] = map[string]any{"type": "string", "enum": []string{"catalog_exact"}}
		delete(properties, "event_from")
		delete(properties, "event_until")
		encoded, err := json.Marshal(schema)
		if err != nil {
			return nil, fmt.Errorf("owned memory search projection unavailable")
		}
		defs[i].SchemaJSON = string(encoded)
	}
	return defs, nil
}
