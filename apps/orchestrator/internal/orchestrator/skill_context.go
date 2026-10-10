package orchestrator

import (
	"context"
	"github.com/butler/butler/internal/security/content"
	"github.com/butler/butler/internal/skills"
)

type SkillCatalog interface {
	Index(context.Context, string) ([]skills.Descriptor, error)
	Pinned(context.Context, string) ([]skills.Package, error)
}

func (s *Service) attachSkillContext(ctx context.Context, p *preparedRun, recovered bool) error {
	if s.config.Skills == nil || p == nil || p.CurrentRunID == "" {
		return nil
	}
	if !recovered && p.Task == nil {
		return nil
	}
	if !recovered {
		index, err := s.config.Skills.Index(ctx, p.CurrentRunID)
		if err != nil {
			return err
		}
		if len(index) > 0 {
			p.InputItems = append(p.InputItems, content.Data(map[string]any{"skill_descriptions": index}, content.Origin{Kind: content.Document, Sensitivity: content.Private, SourceRefs: []string{"skill-index:" + p.CurrentRunID}}))
		}
	}
	if recovered {
		packages, err := s.config.Skills.Pinned(ctx, p.CurrentRunID)
		if err != nil {
			return err
		}
		for _, pack := range packages {
			p.InputItems = append(p.InputItems, content.Data(map[string]any{"procedure": pack.Manifest, "digest": pack.Digest}, content.Origin{Kind: content.Document, Sensitivity: content.Private, SourceRefs: []string{"skill:" + pack.Digest}}))
		}
	}
	return nil
}
