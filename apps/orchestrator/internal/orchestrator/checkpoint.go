package orchestrator

import "context"

// CheckpointStore is durable execution progress owned by the accepted input.
// Transport refs alone never authorize replay of an external effect.
type CheckpointStore interface {
	SaveCheckpoint(context.Context, string, string, string) error
	LoadCheckpoint(context.Context, string) (string, string, error)
}

func (s *Service) checkpoint(ctx context.Context, runID, stage string, payload any) error {
	if s.config.Checkpoints == nil {
		return nil
	}
	if prepared, ok := payload.(preparedRun); ok {
		payload = durableSkillPreparation(prepared)
	}
	if err := s.config.Checkpoints.SaveCheckpoint(ctx, runID, stage, mustMarshalJSON(payload)); err != nil {
		return err
	}
	if stage == "tool_dispatching" && s.config.ApprovalService != nil {
		if data, ok := payload.(map[string]any); ok {
			if toolID, ok := data["tool_call_id"].(string); ok {
				return s.config.ApprovalService.MarkActionCheckpoint(ctx, runID, toolID, "dispatching")
			}
		}
	}
	if stage == "tool_result_recorded" && s.config.ApprovalService != nil {
		if data, ok := payload.(map[string]any); ok {
			if toolID, ok := data["tool_call_id"].(string); ok {
				state := "completed"
				if data["status"] == "effect_unknown" {
					state = "blocked"
				}
				return s.config.ApprovalService.MarkActionCheckpoint(ctx, runID, toolID, state)
			}
		}
	}
	return nil
}
