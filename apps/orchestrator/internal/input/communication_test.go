package input

import (
	"context"
	"testing"

	flow "github.com/butler/butler/apps/orchestrator/internal/orchestrator"
	"github.com/butler/butler/internal/ingress"
	"github.com/jackc/pgx/v5/pgxpool"
)

type communicationReadFailure struct {
	pool           *pgxpool.Pool
	inputID, stage string
}

func (f communicationReadFailure) Execute(ctx context.Context, _ ingress.InputEvent) (*flow.ExecutionResult, error) {
	if _, e := f.pool.Exec(ctx, `UPDATE accepted_inputs SET checkpoint=$2 WHERE input_id=$1`, f.inputID, f.stage); e != nil {
		return nil, e
	}
	return nil, flow.ErrCommunicationStatePending
}

func TestCommunicationStateReadRetryPreservesReadyAndNoExpiryWaitIntegration(t *testing.T) {
	for _, stage := range []string{"tool_dispatching", "awaiting_interaction", "interaction_ready"} {
		t.Run(stage, func(t *testing.T) {
			f := newLifecycleFixture(t)
			s := NewService(f.store, f.sessions, communicationReadFailure{pool: f.pool, inputID: f.receipt.InputID, stage: stage}, f.runs, nil)
			s.process(f.ctx, f.claim(t))
			want := "queued"
			if stage == "awaiting_interaction" {
				want = "waiting_for_user"
			}
			r, e := f.store.Get(f.ctx, f.receipt.InputID)
			if e != nil || r.Status != want || r.Checkpoint != stage || r.ErrorCode != "communication_state_pending" {
				t.Fatal("temporary read lost wait/readiness", r, e)
			}
		})
	}
}
