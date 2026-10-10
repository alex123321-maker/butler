package orchestrator

import (
	"context"
	"fmt"

	"github.com/butler/butler/internal/actionauth"
	toolbrokerv1 "github.com/butler/butler/internal/gen/toolbroker/v1"
)

type actionSlotStore interface {
	EnsureActionSlot(context.Context, string, string, string, string) (string, error)
}

func (s *Service) prepareActionSlot(ctx context.Context, call *toolbrokerv1.ToolCall) (context.Context, error) {
	recovered, _ := ctx.Value(recoveredApprovalKey{}).(approvalContinuation)
	ref := ""
	if recovered.ToolCall != nil && recovered.ToolCall.ToolCallRef == call.GetToolCallId() {
		ref = recovered.LogicalActionRef
	}
	if slots, ok := s.config.Checkpoints.(actionSlotStore); ok {
		hash, err := actionauth.ArgumentsHash(call.GetArgsJson())
		if err != nil {
			return ctx, err
		}
		persisted, err := slots.EnsureActionSlot(ctx, call.GetRunId(), call.GetToolCallId(), call.GetToolName(), hash)
		if err != nil {
			return ctx, err
		}
		if ref != "" && ref != persisted {
			return ctx, fmt.Errorf("continuation action-slot identity changed")
		}
		ref = persisted
	}
	return actionauth.WithLogicalReference(ctx, ref), nil
}
