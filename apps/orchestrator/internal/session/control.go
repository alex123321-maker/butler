package session

import (
	"context"

	"github.com/jackc/pgx/v5"
)

// RevokeForControlTx shares the session ownership row with irreversible dispatch.
// A committed owner control wins over every later claim of the old fence.
func (m *PostgresLeaseManager) RevokeForControlTx(ctx context.Context, tx pgx.Tx, taskIDs []string) error {
	_, err := tx.Exec(ctx, `UPDATE session_ownership o SET lease_id='',fence_epoch=fence_epoch+1,expires_at=clock_timestamp() FROM runs r WHERE r.run_id=o.run_id AND r.task_id=ANY($1)`, taskIDs)
	return err
}
