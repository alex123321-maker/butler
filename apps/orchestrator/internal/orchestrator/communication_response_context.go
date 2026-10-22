package orchestrator

import (
	"encoding/json"
	"github.com/butler/butler/internal/memory/sanitize"
	"slices"

	"github.com/butler/butler/internal/security/content"
)

func interactionResponsePreparation(prepared, refreshed preparedRun, id, answer string) preparedRun {
	sourceRef := "message:response:" + id
	prepared.InputItems = nil
	for _, item := range refreshed.InputItems {
		origin := content.ItemOrigin(item)
		if origin.Kind == content.ToolOutput && (slices.Contains(origin.SourceRefs, id) || slices.Contains(origin.SourceRefs, sourceRef)) {
			continue
		}
		prepared.InputItems = append(prepared.InputItems, item)
	}
	prepared.InputItems = append(prepared.InputItems, content.Data(map[string]any{"kind": "interaction_response", "data": json.RawMessage(sanitize.JSON(answer))}, content.Origin{Kind: content.ToolOutput, SourceRefs: []string{sourceRef}, Sensitivity: content.Private}))
	prepared.ToolDefs = refreshed.ToolDefs
	return prepared
}
