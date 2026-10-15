package orchestrator

import (
	"context"
	"errors"
	"github.com/butler/butler/internal/budget"
	"github.com/butler/butler/internal/memory/adaptive"
	"github.com/butler/butler/internal/memory/readcontrol"
)

type adaptivePlanKey struct{}

func adaptiveContext(ctx context.Context, p preparedRun) context.Context {
	return context.WithValue(ctx, adaptivePlanKey{}, p.AdaptivePlan != nil && p.AdaptivePlan.SearchEnabled())
}

func (s *Service) guardAdaptiveProposal(ctx context.Context, runID, tool, raw string) error {
	if s.config.AdaptiveRetrieval == nil || !readcontrol.IsTool(tool) {
		return nil
	}
	err := s.config.AdaptiveRetrieval.CheckProposal(ctx, runID, tool, raw)
	if errors.Is(err, adaptive.ErrBound) {
		return &budget.LoopError{Reason: "memory_retrieval_bound"}
	}
	if err != nil {
		return &budget.LoopError{Reason: "memory_retrieval_plan_denied"}
	}
	return nil
}
