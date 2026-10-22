package orchestrator

import (
	"encoding/json"
	"slices"

	"github.com/butler/butler/internal/security/content"
)

func interactionResponsePreparation(prepared, refreshed preparedRun, id, answer string) preparedRun {
	prepared.InputItems = nil
	for _, item := range refreshed.InputItems {
		origin := content.ItemOrigin(item)
		if origin.Kind == content.ToolOutput && slices.Contains(origin.SourceRefs, id) {
			continue
		}
		prepared.InputItems = append(prepared.InputItems, item)
	}
	prepared.InputItems = append(prepared.InputItems, content.Data(map[string]any{"kind": "interaction_response", "data": json.RawMessage(answer)}, content.Origin{Kind: content.ToolOutput, SourceRefs: []string{id}, Sensitivity: content.Private}))
	prepared.ToolDefs = refreshed.ToolDefs
	return prepared
}
