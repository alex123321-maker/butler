package orchestrator

import (
	"encoding/json"
	"strings"

	toolbrokerv1 "github.com/butler/butler/internal/gen/toolbroker/v1"
	"github.com/butler/butler/internal/security/content"
	"github.com/butler/butler/internal/skills"
	"github.com/butler/butler/internal/transport"
	"google.golang.org/protobuf/proto"
)

// Live model context may contain currently authorized procedural DATA. Durable
// copies retain refs only, so deletion does not leave an untracked checkpoint
// or observation copy. Recovery reloads text through the current owned catalog.
func durableSkillPreparation(p preparedRun) preparedRun {
	p.InputItems = append([]transport.InputItem(nil), p.InputItems...)
	for i, item := range p.InputItems {
		origin := content.ItemOrigin(item)
		if origin.IsInstruction() {
			continue
		}
		for _, ref := range origin.SourceRefs {
			if strings.HasPrefix(ref, "skill:") || strings.HasPrefix(ref, "skill-index:") {
				p.InputItems[i] = content.Data(map[string]any{"catalog_reference": ref, "reload_current_data": true}, origin)
				break
			}
		}
	}
	return p
}

func durableSkillObservation(result *toolbrokerv1.ToolResult) *toolbrokerv1.ToolResult {
	if result == nil || result.GetStatus() != "completed" || result.GetToolName() != "skills.load" && result.GetToolName() != "skills.search" {
		return result
	}
	copy := proto.Clone(result).(*toolbrokerv1.ToolResult)
	var value struct {
		Digest string              `json:"digest"`
		Skills []skills.Descriptor `json:"skills"`
	}
	if json.Unmarshal([]byte(result.GetResultJson()), &value) != nil {
		copy.ResultJson = `{"catalog_data_unavailable":true}`
		return copy
	}
	refs := []string{}
	for _, descriptor := range value.Skills {
		refs = append(refs, descriptor.Digest)
	}
	copy.ResultJson = mustMarshalJSON(map[string]any{"digest": value.Digest, "skill_digests": refs, "instruction_authority": false, "reload_current_data": true})
	return copy
}
