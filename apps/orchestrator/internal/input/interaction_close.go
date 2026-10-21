package input

import (
	"context"
	"github.com/jackc/pgx/v5"
)

func (s *Store) CloseInteractionTx(ctx context.Context, tx pgx.Tx, inputID, id, reason string) error {
	_, e := tx.Exec(ctx, `UPDATE accepted_inputs SET status='blocked',claim_id='',claim_expires_at=NULL,error_code=$3,updated_at=clock_timestamp() WHERE input_id=$1 AND checkpoint='awaiting_interaction' AND checkpoint_json->'interaction'->>'interaction_id'=$2 AND status IN ('running','waiting_for_user','blocked')`, inputID, id, "interaction_"+reason)
	return e
}
