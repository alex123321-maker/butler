package input

import (
	"context"
	"encoding/json"
	"github.com/jackc/pgx/v5"
)

// HoldInteractionControlTx pauses a newly admitted owner-resume Input when its
// existing question has not been answered. No cancelled old worker is revived.
func (s *Store) HoldInteractionControlTx(ctx context.Context, tx pgx.Tx, inputID, id string) error {
	marker, _ := json.Marshal(map[string]any{"version": 1, "kind": "communication_interaction", "interaction_id": id})
	tag, e := tx.Exec(ctx, `UPDATE accepted_inputs SET status='waiting_for_user',checkpoint='awaiting_interaction',checkpoint_json=checkpoint_json||jsonb_build_object('interaction',$2::jsonb),updated_at=clock_timestamp() WHERE input_id=$1 AND status='queued' AND run_id IS NULL`, inputID, marker)
	if e == nil && tag.RowsAffected() != 1 {
		return ErrClaimLost
	}
	return e
}
