package orchestrator

import (
	"encoding/json"
	"github.com/butler/butler/internal/transport"
	"strings"
)

// Human-readable preview for existing retrieval assertions. Production keeps
// each data item separate from the first trusted instruction.
func fixtureContextText(items []transport.InputItem) string {
	parts := []string{}
	for _, item := range items {
		var envelope struct {
			Data struct {
				Content string `json:"content"`
			} `json:"data"`
		}
		if json.Unmarshal([]byte(item.Content), &envelope) == nil && envelope.Data.Content != "" {
			parts = append(parts, envelope.Data.Content)
		} else {
			parts = append(parts, item.Content)
		}
	}
	return strings.Join(parts, "\n")
}
