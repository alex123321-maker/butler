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
	return s.config.Checkpoints.SaveCheckpoint(ctx, runID, stage, mustMarshalJSON(payload))
}
