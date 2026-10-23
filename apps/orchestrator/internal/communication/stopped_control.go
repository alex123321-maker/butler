package communication

import (
	"context"
	"errors"

	"github.com/butler/butler/apps/orchestrator/internal/input"
	"github.com/butler/butler/apps/orchestrator/internal/run"
	"github.com/butler/butler/internal/owneridentity"
)

type CloseFailedRequest struct {
	BindingID                 string   `json:"binding_id"`
	BindingVersion            int64    `json:"binding_version"`
	ExpectedCheckpoint        string   `json:"expected_checkpoint"`
	ExpectedErrorCode         string   `json:"expected_error_code"`
	AcknowledgeUnknownEffects []string `json:"acknowledge_unknown_effects,omitempty"`
}

// CloseFailed requires a current owner binding. It cannot resume work, grant an
// action, clear a budget or convert a failed attempt into a successful result.
func (s *Store) CloseFailed(ctx context.Context, id string, q CloseFailedRequest) error {
	a, ok := owneridentity.FromContext(ctx)
	if !ok || a.Channel != "web" || a.DeliveryID == "" || len(a.DeliveryID) > 512 || s == nil || s.Inputs == nil || id == "" || len(id) > 160 || q.BindingID == "" || q.BindingVersion < 1 || q.ExpectedCheckpoint == "" || q.ExpectedErrorCode == "" {
		return ErrDenied
	}
	tx, err := s.Pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(context.Background())
	r, changed, err := s.Inputs.CloseStoppedTx(ctx, tx, id, q.ExpectedCheckpoint, q.ExpectedErrorCode, q.AcknowledgeUnknownEffects)
	if errors.Is(err, input.ErrControlConflict) {
		return ErrConflict
	}
	if err != nil {
		return ErrDenied
	}
	var yes bool
	if err = tx.QueryRow(ctx, `SELECT TRUE FROM communication_admissions a JOIN sessions s ON s.session_key=a.session_key AND s.user_id=a.owner_id JOIN communication_bindings b ON b.owner_id=a.owner_id WHERE a.input_id=$1 AND a.owner_id=$2 AND b.binding_id=$3 AND b.version=$4 AND b.channel='web' AND b.recipient=$5 AND b.enabled AND b.verified FOR SHARE OF b,s`, id, a.OwnerID, q.BindingID, q.BindingVersion, a.SessionKey).Scan(&yes); err != nil {
		return ErrDenied
	}
	if !changed {
		if err = tx.QueryRow(ctx, `SELECT TRUE FROM communication_events WHERE owner_id=$1 AND session_key=$2 AND aggregate_id=$3 AND event_type='input.closed_failed'`, a.OwnerID, r.SessionKey, id).Scan(&yes); err != nil {
			return ErrConflict
		}
		return tx.Commit(ctx)
	}
	if err = run.FailStoppedInputTx(ctx, tx, r.RunID, id, q.AcknowledgeUnknownEffects); err != nil {
		return err
	}
	if _, err = tx.Exec(ctx, `UPDATE communication_admissions SET obligation_state='closed_with_failure' WHERE input_id=$1 AND obligation_state IN ('open','waiting_for_user')`, id); err != nil {
		return err
	}
	if err = s.eventTx(ctx, tx, a.OwnerID, r.SessionKey, id, "input.closed_failed"); err != nil {
		return err
	}
	if len(q.AcknowledgeUnknownEffects) > 0 {
		if err = s.eventTx(ctx, tx, a.OwnerID, r.SessionKey, id, "input.uncertainty_retained"); err != nil {
			return err
		}
		for _, effectID := range q.AcknowledgeUnknownEffects {
			if err = s.eventTx(ctx, tx, a.OwnerID, r.SessionKey, effectID, "effect.uncertainty_retained"); err != nil {
				return err
			}
		}
	}
	return tx.Commit(ctx)
}
