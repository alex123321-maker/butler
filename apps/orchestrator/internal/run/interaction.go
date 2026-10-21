package run

import (
	"context"
	"github.com/butler/butler/internal/ownership"
	"github.com/jackc/pgx/v5"
)

// The existing coarse waiting run state is reused with a distinct durable Input
// marker. No approvals record or action grant is created by a preference widget.
func SuspendInteractionTx(ctx context.Context, tx pgx.Tx, id string) error {
	t, ok := ownership.TokenFromContext(ctx)
	if !ok || t.RunID != id {
		return ownership.ErrStale
	}
	tag, e := tx.Exec(ctx, `UPDATE runs SET current_state='awaiting_approval',status='awaiting_approval',updated_at=clock_timestamp() WHERE run_id=$1 AND session_key=$2 AND lease_id=$3 AND current_state='tool_running'`, id, t.SessionKey, t.LeaseID)
	if e == nil && tag.RowsAffected() != 1 {
		return ownership.ErrStale
	}
	if e != nil {
		return e
	}
	_, e = tx.Exec(ctx, `INSERT INTO run_state_transitions(run_id,from_state,to_state,triggered_by,metadata_json,transitioned_at) VALUES($1,'tool_running','awaiting_approval','communication_interaction','{"kind":"preference_wait"}',clock_timestamp())`, id)
	return e
}
