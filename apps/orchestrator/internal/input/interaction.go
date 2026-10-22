package input

import (
	"context"
	"encoding/json"
	"github.com/butler/butler/internal/ownership"
	"github.com/jackc/pgx/v5"
)

// SuspendInteractionTx joins the publication transaction. The claim may remain
// running until its worker exits; ready is a monotonic later checkpoint.
func (s *Store) SuspendInteractionTx(ctx context.Context, tx pgx.Tx, runID, id string) error {
	r, ok := ctx.Value(claimKey{}).(Record)
	tok, owned := ownership.TokenFromContext(ctx)
	if !ok || !owned || tok.RunID != runID || r.RunID != "" && r.RunID != runID {
		return ErrClaimLost
	}
	marker, _ := json.Marshal(map[string]any{"version": 1, "kind": "communication_interaction", "interaction_id": id})
	tag, e := tx.Exec(ctx, `UPDATE accepted_inputs SET checkpoint='awaiting_interaction',checkpoint_json=checkpoint_json||jsonb_build_object('interaction',$4::jsonb),updated_at=clock_timestamp() WHERE input_id=$1 AND claim_id=$2 AND run_id=$3 AND status='running' AND checkpoint='tool_dispatching' AND claim_expires_at>clock_timestamp() AND expires_at>clock_timestamp()`, r.InputID, r.ClaimID, runID, marker)
	if e == nil && tag.RowsAffected() != 1 {
		return ErrClaimLost
	}
	return e
}

// ReadyInteractionTx preserves an early response until the previous worker's
// claim is released. Waiting time does not refresh spent execution budgets.
func (s *Store) ReadyInteractionTx(ctx context.Context, tx pgx.Tx, inputID, id string, paused bool) error {
	tag, e := tx.Exec(ctx, `UPDATE accepted_inputs SET checkpoint='interaction_ready',status=CASE WHEN $3 THEN 'blocked' WHEN status='running' AND claim_expires_at>clock_timestamp() THEN 'running' ELSE 'queued' END,expires_at=clock_timestamp()+INTERVAL '10 minutes',not_before=clock_timestamp(),updated_at=clock_timestamp() WHERE input_id=$1 AND checkpoint='awaiting_interaction' AND checkpoint_json->'interaction'->>'interaction_id'=$2 AND status IN ('running','waiting_for_user','blocked')`, inputID, id, paused)
	if e == nil && tag.RowsAffected() != 1 {
		return ErrClaimLost
	}
	return e
}

func (s *Store) CheckInteractionClaimTx(ctx context.Context, tx pgx.Tx, runID, id string) error {
	r, ok := ctx.Value(claimKey{}).(Record)
	if !ok {
		return ErrClaimLost
	}
	var valid bool
	e := tx.QueryRow(ctx, `SELECT TRUE FROM accepted_inputs WHERE input_id=$1 AND claim_id=$2 AND run_id=$3 AND checkpoint='interaction_ready' AND checkpoint_json->'interaction'->>'interaction_id'=$4 AND status='running' AND claim_expires_at>clock_timestamp() AND expires_at>clock_timestamp()`, r.InputID, r.ClaimID, runID, id).Scan(&valid)
	if e != nil {
		return ErrClaimLost
	}
	return nil
}

// SaveInteractionPreparation retains answer DATA for subsequent approval/model
// recovery without consuming or replacing the monotonic interaction-ready marker.
func (s *Store) SaveInteractionPreparation(ctx context.Context, runID, id, payload string) error {
	r, ok := ctx.Value(claimKey{}).(Record)
	tok, owned := ownership.TokenFromContext(ctx)
	if !ok || !owned || tok.RunID != runID {
		return ErrClaimLost
	}
	ctx, release, err := ownership.Begin(ctx)
	if err != nil {
		return err
	}
	defer release()
	tag, err := s.pool.Exec(ctx, `UPDATE accepted_inputs SET checkpoint_json=jsonb_set(checkpoint_json,'{prepared}',$4::jsonb),updated_at=clock_timestamp() WHERE input_id=$1 AND claim_id=$2 AND run_id=$3 AND checkpoint='interaction_ready' AND checkpoint_json->'interaction'->>'interaction_id'=$5 AND status='running' AND claim_expires_at>clock_timestamp() AND expires_at>clock_timestamp()`, r.InputID, r.ClaimID, runID, payload, id)
	if err == nil && tag.RowsAffected() != 1 {
		return ErrClaimLost
	}
	return err
}
