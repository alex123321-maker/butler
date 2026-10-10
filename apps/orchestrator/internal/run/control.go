package run

import (
	"context"

	"github.com/jackc/pgx/v5"
)

func (r *PostgresRepository) LockForControlTx(ctx context.Context, tx pgx.Tx, taskIDs []string) error {
	rows, err := tx.Query(ctx, `SELECT run_id FROM runs WHERE task_id=ANY($1) ORDER BY run_id FOR UPDATE`, taskIDs)
	if err != nil {
		return err
	}
	defer rows.Close()
	for rows.Next() {
	}
	return rows.Err()
}
func (r *PostgresRepository) CancelForControlTx(ctx context.Context, tx pgx.Tx, taskIDs []string, controlID string) error {
	_, err := tx.Exec(ctx, `WITH previous AS(SELECT run_id,current_state FROM runs WHERE task_id=ANY($1) AND current_state NOT IN ('completed','failed','cancelled','timed_out') FOR UPDATE),cancelled AS (UPDATE runs r SET status='cancelled',current_state='cancelled',finished_at=clock_timestamp(),updated_at=clock_timestamp(),error_type='owner_control',error_message=$2 FROM previous p WHERE r.run_id=p.run_id RETURNING r.run_id,p.current_state) INSERT INTO run_state_transitions(run_id,from_state,to_state,triggered_by,metadata_json,transitioned_at) SELECT run_id,current_state,'cancelled','owner',jsonb_build_object('control_id',$2::text),clock_timestamp() FROM cancelled`, taskIDs, controlID)
	return err
}
