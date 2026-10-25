package orchestrator

import (
	"context"
	"encoding/json"
	"errors"
	"time"

	commonv1 "github.com/butler/butler/internal/gen/common/v1"
	sessionv1 "github.com/butler/butler/internal/gen/session/v1"
	"github.com/butler/butler/internal/ownership"
)

type authorityRetryStore interface {
	ClaimAuthorityRetry(context.Context, string, string, time.Time) error
}

// Use the still-live Input parent, never WithoutCancel/Background. The stopped
// stream stays cancelled. Fresh proofs only permit recording a continuation;
// the next worker acquires a new lease/claim and independently revalidates it.
func (s *Service) recoverAuthorityTimeout(parent, stopped context.Context, current *sessionv1.RunRecord, err error) error {
	var cause *ownedCancellation
	if !errors.As(err, &cause) || err != cause || !cause.observationTimeout || context.Cause(stopped) != cause || parent.Err() != nil || current == nil {
		return err
	}
	if state := current.GetCurrentState(); state != commonv1.RunState_RUN_STATE_MODEL_RUNNING && state != commonv1.RunState_RUN_STATE_AWAITING_MODEL_RESUME {
		return err
	}
	deadline, ok := parent.Deadline()
	if !ok || time.Until(deadline) <= 2*time.Second {
		return err
	}
	token, ok := ownership.TokenFromContext(parent)
	if !ok || token.RunID != current.GetRunId() || token.SessionKey != current.GetSessionKey() {
		return err
	}
	store, ok := s.config.Checkpoints.(authorityRetryStore)
	if !ok || s.config.MemorySources == nil || s.config.Budgets == nil || s.transcript == nil {
		return err
	}
	ctx, cancel := context.WithTimeout(parent, 5*time.Second)
	defer cancel()
	if s.config.Tasks != nil {
		guard, ok := s.config.Tasks.(interface {
			ValidateRunAuthority(context.Context, string) error
		})
		if !ok || guard.ValidateRunAuthority(ctx, current.GetRunId()) != nil {
			return err
		}
	}
	if s.config.MemorySources.CheckRunSources(ctx, current.GetRunId()) != nil {
		return err
	}
	if _, e := s.modelRetryRecordedInputs(ctx, current, nil); e != nil {
		return err
	}
	stage, raw, e := s.config.Checkpoints.LoadCheckpoint(ctx, current.GetRunId())
	var checkpoint struct {
		Data struct {
			Operation string `json:"operation_id"`
		} `json:"checkpoint_data"`
	}
	if e != nil || stage != "model_budget_reserved" || json.Unmarshal([]byte(raw), &checkpoint) != nil || checkpoint.Data.Operation == "" {
		return err
	}
	if store.ClaimAuthorityRetry(ctx, current.GetRunId(), checkpoint.Data.Operation, deadline) != nil {
		return err
	}
	_ = s.discardProviderContext(ctx, current.GetRunId())
	s.log.Info("authority observation retry queued", "run_id", current.GetRunId())
	return ErrModelRetryPending
}
