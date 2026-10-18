package input

import (
	"context"
	"github.com/jackc/pgx/v5"
)

// InvalidateForScheduleTx is called only after the owning schedule control has
// changed its revision under an actual Broker dispatch. Terminal receipts and
// uncertain effect checkpoints remain history. Removing obsolete inputs from
// session ordering prevents an old blocked occurrence starving a fresh revision.
func (s *Store) InvalidateForScheduleTx(ctx context.Context, tx pgx.Tx, taskIDs []string) error {
	_, err := tx.Exec(ctx, `UPDATE accepted_inputs SET status='cancelled',claim_id='',claim_expires_at=NULL,error_code='schedule_changed',error_message='owning schedule revision invalidated this occurrence',updated_at=clock_timestamp() WHERE task_id=ANY($1) AND status NOT IN ('completed','failed','expired','cancelled')`, taskIDs)
	return err
}
