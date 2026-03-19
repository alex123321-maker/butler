package orchestrator

import (
	"context"
	"fmt"
	"strings"

	promptmgmt "github.com/butler/butler/apps/orchestrator/internal/prompt"
	toolbrokerv1 "github.com/butler/butler/internal/gen/toolbroker/v1"
	memoryservice "github.com/butler/butler/internal/memory/service"
)

type PromptPreviewRequest struct {
	SessionKey  string
	UserID      string
	UserMessage string
}

type PromptPreviewResult struct {
	Config promptmgmt.ConfigState
	promptmgmt.Assembly
}

func (s *Service) GetPromptConfig(ctx context.Context) (promptmgmt.ConfigState, error) {
	if s.config.PromptManager == nil {
		return promptmgmt.ConfigState{}, fmt.Errorf("prompt manager is not configured")
	}
	return s.config.PromptManager.Get(ctx)
}

func (s *Service) UpdatePromptConfig(ctx context.Context, req promptmgmt.UpdateRequest) (promptmgmt.ConfigState, error) {
	if s.config.PromptManager == nil {
		return promptmgmt.ConfigState{}, fmt.Errorf("prompt manager is not configured")
	}
	return s.config.PromptManager.Update(ctx, req)
}

func (s *Service) PreviewPrompt(ctx context.Context, req PromptPreviewRequest) (PromptPreviewResult, error) {
	if s.config.PromptManager == nil || s.config.PromptAssembler == nil {
		return PromptPreviewResult{}, fmt.Errorf("prompt preview is not configured")
	}
	state, err := s.config.PromptManager.Get(ctx)
	if err != nil {
		return PromptPreviewResult{}, err
	}
	bundle, err := s.config.MemoryBundles.BuildBundle(ctx, memoryservice.BundleRequest{
		SessionKey:   strings.TrimSpace(req.SessionKey),
		UserID:       previewUserID(req),
		UserMessage:  strings.TrimSpace(req.UserMessage),
		IncludeQuery: strings.TrimSpace(req.UserMessage) != "",
	})
	if err != nil {
		return PromptPreviewResult{}, err
	}
	assembly := s.config.PromptAssembler.Assemble(state, promptmgmt.Context{
		SessionSummary: stringFromAny(bundle.Items["session_summary"]),
		Working:        bundle.Working,
		Profile:        sliceOfMaps(bundle.Items["profile"]),
		Episodes:       sliceOfMaps(bundle.Items["episodes"]),
		Chunks:         sliceOfMaps(bundle.Items["chunks"]),
		ToolSummary:    toolSummaryFromContracts(previewToolContracts(ctx, s.config.ToolCatalog)),
	})
	return PromptPreviewResult{Config: state, Assembly: assembly}, nil
}

func previewToolContracts(ctx context.Context, catalog ToolCatalog) []*toolbrokerv1.ToolContract {
	if catalog == nil {
		return nil
	}
	contracts, err := catalog.ListTools(ctx)
	if err != nil {
		return nil
	}
	return contracts
}

func previewUserID(req PromptPreviewRequest) string {
	if trimmed := strings.TrimSpace(req.UserID); trimmed != "" {
		return trimmed
	}
	return strings.TrimSpace(req.SessionKey)
}
