package orchestrator

import (
	"context"
	"errors"

	sessionv1 "github.com/butler/butler/internal/gen/session/v1"
	"github.com/butler/butler/internal/ownership"
)

// This is a bounded durable-state read retry, never permission for another
// model turn or tool dispatch. The owning Input retains the stored checkpoint.
var ErrCommunicationStatePending = errors.New("communication state verification pending")

// The single correction allowance has already been durably spent. An admission
// or stream failure must close a still-open obligation without another inference.
// Current ownership/source checks remain in BeforeFinal; a cancelled/stale
// execution cannot manufacture a system publication.
func (s *Service) failedCommunicationCorrection(ctx context.Context, current *sessionv1.RunRecord, cause error) (*ExecutionResult, error) {
	if ctx.Err() != nil || errors.Is(cause, context.Canceled) || errors.Is(cause, ownership.ErrStale) || errors.Is(cause, ErrCommunicationStatePending) {
		return nil, cause
	}
	decision, err := s.config.Communication.BeforeFinal(ctx, current.GetRunId(), "")
	if err != nil {
		return nil, err
	}
	if decision == "failed" {
		if blockErr := s.blockTaskRun(ctx, current.GetRunId(), "communication_result_missing"); blockErr != nil {
			return nil, blockErr.withPreceding(cause)
		}
		return &ExecutionResult{RunID: current.GetRunId(), SessionKey: current.GetSessionKey(), CurrentState: current.GetCurrentState(), ReceiptStatus: "blocked", ErrorCode: "communication_result_missing"}, nil
	}
	// A result may have been published before a broken terminal stream. Keep
	// its receipt, but never turn that stream failure into successful completion.
	return nil, cause
}
