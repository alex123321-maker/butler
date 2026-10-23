package service

import (
	"encoding/json"
	"fmt"
	"strings"
)

// FormatProfileEntries preserves the scope and provenance of selected profile
// evidence. It does not resolve conflicts, establish applicability or grant
// instruction authority. Callers must admit or omit records whole.
func FormatProfileEntries(entries []map[string]any) string {
	lines := make([]string, 0, len(entries))
	for _, entry := range entries {
		key, _ := entry["key"].(string)
		summary, _ := entry["summary"].(string)
		key, summary = strings.TrimSpace(key), strings.TrimSpace(summary)
		if key == "" || summary == "" {
			continue
		}
		scopeType, _ := entry["scope_type"].(string)
		scopeID, _ := entry["scope_id"].(string)
		metadata := "scope unknown"
		if scopeType != "" && scopeID != "" {
			metadata = fmt.Sprintf("scope_type=%q; scope_id=%q", scopeType, scopeID)
		}
		// A durable JSON round trip changes []string into []any. Accept only
		// string handles in either representation; never invent provenance.
		refs := profileSourceRefs(entry["source_refs"])
		if len(refs) > 0 {
			encoded, _ := json.Marshal(refs)
			metadata += "; sources=" + string(encoded)
		}
		lines = append(lines, "- "+key+": "+summary+" ("+metadata+"; stored evidence)")
	}
	return strings.Join(lines, "\n")
}

func profileSourceRefs(value any) []string {
	switch refs := value.(type) {
	case []string:
		return refs
	case []any:
		result := make([]string, 0, len(refs))
		for _, ref := range refs {
			if text, ok := ref.(string); ok && text != "" {
				result = append(result, text)
			}
		}
		return result
	default:
		return nil
	}
}
