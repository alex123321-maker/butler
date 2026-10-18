package transport

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
)

// A nil snapshot preserves definitions. A non-nil snapshot replaces them,
// including an explicitly empty set. Revision identifies exact ordered content.
type ToolSetSnapshot struct {
	Revision    string           `json:"revision"`
	Definitions []ToolDefinition `json:"definitions"`
}

func NewToolSetSnapshot(defs []ToolDefinition) *ToolSetSnapshot {
	copyDefs := append([]ToolDefinition{}, defs...)
	b, _ := json.Marshal(copyDefs)
	h := sha256.Sum256(b)
	return &ToolSetSnapshot{Revision: hex.EncodeToString(h[:]), Definitions: copyDefs}
}
func (s *ToolSetSnapshot) Validate() error {
	if s == nil {
		return nil
	}
	if s.Revision != NewToolSetSnapshot(s.Definitions).Revision {
		return fmt.Errorf("tool set revision does not match definitions")
	}
	seen := map[string]bool{}
	for _, def := range s.Definitions {
		if def.Name == "" || seen[def.Name] || !json.Valid([]byte(def.SchemaJSON)) {
			return fmt.Errorf("invalid or duplicate tool definition")
		}
		seen[def.Name] = true
	}
	return nil
}
