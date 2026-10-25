package orchestrator

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/butler/butler/internal/budget"
	commonv1 "github.com/butler/butler/internal/gen/common/v1"
	sessionv1 "github.com/butler/butler/internal/gen/session/v1"
	"github.com/butler/butler/internal/memory/transcript"
	"github.com/butler/butler/internal/ownership"
	"github.com/butler/butler/internal/transport"
)

type authorityRetryUnitStore struct {
	modelRetryUnitStore
	deadline time.Time
}

func (*authorityRetryUnitStore) LoadCheckpoint(context.Context, string) (string, string, error) {
	return "model_budget_reserved", `{"checkpoint_data":{"operation_id":"owned-operation"}}`, nil
}

func (s *authorityRetryUnitStore) ClaimAuthorityRetry(ctx context.Context, run, op string, deadline time.Time) error {
	if ctx.Err() != nil || run != "run" || op != "owned-operation" {
		return errors.New("invalid live continuation")
	}
	s.deadline = deadline
	return s.ClaimModelRetry(ctx, run, op)
}

func TestAuthorityRetryPrivateCauseAndLiveParent(t *testing.T) {
	for _, name := range []string{"genuine", "provider_text", "other_owner_cause", "task_authority", "parent_cancel", "parent_deadline", "missing_deadline", "terminal", "independent_failure", "unfinished_effect"} {
		t.Run(name, func(t *testing.T) {
			store := &authorityRetryUnitStore{}
			receipts := &memoryTranscriptStore{}
			s := newFixtureService(nil, nil, nil, receipts, &mockProvider{}, Config{Checkpoints: store, MemorySources: modelRetryUnitSources{}, Budgets: budget.NewController(nil, budget.DefaultPolicy())}, nil)
			parent, cancel := context.WithTimeout(t.Context(), time.Minute)
			defer cancel()
			if name == "missing_deadline" {
				parent = context.Background()
			}
			parent = ownership.WithToken(parent, nil, ownership.Token{RunID: "run", SessionKey: "session"})
			cause := &ownedCancellation{reason: "execution_authority_observation_timeout", failure: ownership.ErrStale, observationTimeout: true}
			stopped, stop := context.WithCancelCause(parent)
			stop(cause)
			var failure error = cause
			current := &sessionv1.RunRecord{RunId: "run", SessionKey: "session", CurrentState: commonv1.RunState_RUN_STATE_MODEL_RUNNING}
			switch name {
			case "provider_text":
				failure = errors.New(cause.Error())
			case "other_owner_cause":
				failure = &ownedCancellation{reason: cause.reason, observationTimeout: true}
			case "task_authority":
				cause.observationTimeout = false
			case "parent_cancel":
				cancel()
			case "parent_deadline":
				parent, cancel = context.WithTimeout(parent, time.Nanosecond)
				defer cancel()
				<-parent.Done()
			case "terminal":
				current.CurrentState = commonv1.RunState_RUN_STATE_COMPLETED
			case "independent_failure":
				failure = errors.Join(cause, &transport.Error{Type: transport.ErrorTypeProviderUnavailable})
				if AuthorityObservationTimeout(failure) {
					t.Fatal("joined independent failure lost its own diagnosis")
				}
			case "unfinished_effect":
				receipts.toolCalls = append(receipts.toolCalls, transcript.ToolCall{RunID: "run", ToolCallID: "unfinished"})
			}
			got := s.recoverAuthorityTimeout(parent, stopped, current, failure)
			if name == "genuine" {
				deadline, _ := parent.Deadline()
				if !errors.Is(got, ErrModelRetryPending) || !store.deadline.Equal(deadline) || store.claims != 1 {
					t.Fatal("private observation timeout failed to queue bounded continuation", got)
				}
				if s.recoverAuthorityTimeout(parent, stopped, current, failure) != failure || store.claims != 2 {
					t.Fatal("spent retry was reopened")
				}
			} else if got != failure || store.claims != 0 {
				t.Fatal("untrusted cause or unsafe state consumed retry", got)
			}
		})
	}
}
