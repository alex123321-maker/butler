package orchestrator

import (
	"context"
	"github.com/butler/butler/internal/communicationcontrol"
	sessionv1 "github.com/butler/butler/internal/gen/session/v1"
	"github.com/butler/butler/internal/transport"
)

func (s *Service) communicationSourceFeedback(ctx context.Context, current *sessionv1.RunRecord, r *transport.ToolCallRequest) (string, error) {
	if !communicationcontrol.IsTool(r.ToolName) {
		return "", nil
	}
	a, err := communicationcontrol.DecodeArgs(r.ToolName, r.ArgsJSON)
	if err != nil || len(a.SourceRefs) == 0 {
		return "", nil
	} // Schema remains independently checked.
	checker, ok := s.config.Communication.(interface {
		CheckPublicationSources(context.Context, string, []string) (bool, error)
	})
	if !ok {
		return "", nil
	} // Legacy compositions retain their dispatched checks.
	selected, err := checker.CheckPublicationSources(ctx, current.GetRunId(), a.SourceRefs)
	if err != nil {
		return "", err
	}
	if selected {
		return "", nil
	}
	return "No message was sent. source_refs must contain only exact Catalog source_ref values selected in this run. Never infer message references from an event/Input/turn ID. Browser and protected login observations are not Catalog references: leave source_refs empty for them and retain supported facts and the page URL in text. Correct this draft without repeating login or dropping unsupported claims into an uncited answer.", nil
}
