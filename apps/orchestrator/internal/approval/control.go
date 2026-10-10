package approval

import (
	"context"

	"github.com/jackc/pgx/v5"
)

// Lock the same approval/action/run rows as ResolveDurable, before task rows.
func (r *PostgresRepository) LockForControlTx(ctx context.Context, tx pgx.Tx, taskIDs []string) error {
	rows, err := tx.Query(ctx, `SELECT d.approval_id FROM approval_actions d JOIN approvals a USING(approval_id) JOIN runs r ON r.run_id=a.run_id WHERE r.task_id=ANY($1) ORDER BY d.approval_id FOR UPDATE OF d,a,r`, taskIDs)
	if err != nil {
		return err
	}
	defer rows.Close()
	for rows.Next() {
	}
	return rows.Err()
}
func (r *PostgresRepository) InvalidateForControlTx(ctx context.Context, tx pgx.Tx, taskIDs []string, controlID, owner string) error {
	_, err := tx.Exec(ctx, `WITH invalidated AS (UPDATE approvals a SET status='expired',resolved_at=clock_timestamp(),resolved_via='system',resolution_reason='task changed by owner',updated_at=clock_timestamp() FROM runs r WHERE r.run_id=a.run_id AND r.task_id=ANY($1) AND a.status='pending' RETURNING a.approval_id,a.run_id,a.session_key) INSERT INTO approval_events(approval_id,run_id,session_key,event_type,status_before,status_after,actor_type,actor_id,reason,metadata_json) SELECT approval_id,run_id,session_key,'owner_control','pending','expired','operator',$3,'task changed by owner',jsonb_build_object('control_id',$2::text) FROM invalidated`, taskIDs, controlID, owner)
	if err != nil {
		return err
	}
	// Executing/completed action observations are never rewritten by cancellation.
	_, err = tx.Exec(ctx, `UPDATE approval_actions d SET state='blocked',updated_at=clock_timestamp() FROM runs r,approvals a WHERE a.approval_id=d.approval_id AND r.run_id=a.run_id AND r.task_id=ANY($1) AND d.state IN ('waiting','ready')`, taskIDs)
	return err
}
