package orchestrator

import (
	"strings"
	"testing"

	toolbrokerv1 "github.com/butler/butler/internal/gen/toolbroker/v1"
	"github.com/butler/butler/internal/security/content"
	"github.com/butler/butler/internal/transport"
)

func TestSkillProseHasOnlyCurrentContextAndSourceCopies(t *testing.T) {
	const private = "private-reviewed-procedure-marker"
	live := preparedRun{InputItems: []transport.InputItem{content.CurrentOwnerMessage("owner request", "input:1"), content.Data(map[string]string{"procedure": private}, content.Origin{Kind: content.Document, SourceRefs: []string{"skill:exact-digest"}, Sensitivity: content.Private}), content.Data(map[string]string{"description": private}, content.Origin{Kind: content.Document, SourceRefs: []string{"skill-index:run"}, Sensitivity: content.Private})}}
	persisted := durableSkillPreparation(live)
	if strings.Contains(mustMarshalJSON(persisted), private) || !strings.Contains(mustMarshalJSON(live), private) || persisted.InputItems[0].Content != live.InputItems[0].Content {
		t.Fatal("checkpoint retained procedure prose or changed the live/owner context")
	}
	for _, item := range persisted.InputItems[1:] {
		if content.ItemOrigin(item).IsInstruction() || !strings.Contains(item.Content, "reload_current_data") {
			t.Fatal("reference became instruction authority")
		}
	}
	for _, tool := range []string{"skills.load", "skills.search"} {
		result := &toolbrokerv1.ToolResult{ToolName: tool, Status: "completed", ResultJson: `{"digest":"exact-digest","manifest":{"description":"` + private + `"},"skills":[{"digest":"exact-digest","description":"` + private + `"}]}`, ActionId: "action", EffectState: "effect_confirmed", EvidenceRefs: []string{"evidence"}}
		stored := durableSkillObservation(result)
		if strings.Contains(stored.ResultJson, private) || !strings.Contains(result.ResultJson, private) || stored.ActionId != result.ActionId || stored.EffectState != result.EffectState || len(stored.EvidenceRefs) != 1 {
			t.Fatal("observation copied prose, changed live data or discarded effect evidence")
		}
	}
}
