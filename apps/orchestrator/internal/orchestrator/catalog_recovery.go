package orchestrator

import (
	"context"
	"fmt"
	"strings"

	"github.com/butler/butler/internal/security/content"
	"github.com/butler/butler/internal/transport"
)

func (s *Service) refreshRecoveredToolContext(ctx context.Context, runID string, inputs []transport.InputItem, oldDefs []transport.ToolDefinition) (preparedRun, error) {
	p := preparedRun{CurrentRunID: runID, MemoryBundle: map[string]any{}}
	if s.config.ToolDiscoveryEnabled && s.config.ToolCatalog == nil {
		return preparedRun{}, fmt.Errorf("current discovery catalog unavailable")
	}
	if s.config.AdaptiveRetrieval != nil {
		plan, err := s.config.AdaptiveRetrieval.Plan(ctx, runID)
		if err != nil {
			return preparedRun{}, err
		}
		p.AdaptivePlan = &plan
	}
	for _, item := range inputs {
		packageData := false
		for _, ref := range content.ItemOrigin(item).SourceRefs {
			if strings.HasPrefix(ref, "extension-manifest:") || strings.HasPrefix(ref, "skill:") || strings.HasPrefix(ref, "skill-index:") || strings.HasPrefix(ref, "tool-catalog:") || ref == "kernel:tool-discovery-v1" || s.config.ToolDiscoveryEnabled && ref == "retrieval:tool_summary" {
				packageData = true
			}
		}
		if !packageData {
			p.InputItems = append(p.InputItems, item)
		}
	}
	if s.config.ToolCatalog == nil {
		// Compatibility for built-in-only fixtures; installed capabilities require
		// a live trusted scoped reader and cannot rely on old definitions.
		for _, def := range oldDefs {
			if !strings.HasPrefix(def.Name, "extension.") {
				p.ToolDefs = append(p.ToolDefs, def)
			}
		}
		if err := s.attachSkillContext(ctx, &p, true); err != nil {
			return preparedRun{}, err
		}
		return p, nil
	}
	if err := s.attachToolContext(ctx, &p); err != nil {
		return preparedRun{}, err
	}
	if err := s.attachSkillContext(ctx, &p, true); err != nil {
		return preparedRun{}, err
	}
	return p, nil
}
