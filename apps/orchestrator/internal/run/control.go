package run

import (
	"context"
	"encoding/json"
	"github.com/butler/butler/internal/owneridentity"
	"github.com/butler/butler/internal/ownership"

	"github.com/jackc/pgx/v5"
)

// FailStoppedInputTx joins the authenticated Input failure-close transaction.
// It produces a failed transition, with no final result or success/effect claim.
func FailStoppedInputTx(ctx context.Context, tx pgx.Tx, id, inputID string, unknownEffects []string) error {
	a, ok := owneridentity.FromContext(ctx)
	if !ok || a.OwnerID == "" || a.DeliveryID == "" {
		return ownership.ErrStale
	}
	acknowledged, err := json.Marshal(unknownEffects)
	if err != nil {
		return err
	}
	_, err = tx.Exec(ctx, `WITH previous AS(SELECT r.run_id,r.current_state FROM runs r JOIN accepted_inputs i ON i.run_id=r.run_id WHERE r.run_id=$1 AND i.input_id=$2 AND i.owner_id=$3 AND i.task_id IS NULL AND i.status='failed' AND r.current_state NOT IN ('completed','failed','cancelled','timed_out') FOR UPDATE OF r), changed AS(UPDATE runs r SET status='failed',current_state='failed',finished_at=clock_timestamp(),updated_at=clock_timestamp() FROM previous p WHERE r.run_id=p.run_id RETURNING r.run_id,p.current_state) INSERT INTO run_state_transitions(run_id,from_state,to_state,triggered_by,metadata_json,transitioned_at) SELECT run_id,current_state,'failed','owner',jsonb_build_object('kind','close_stopped_input','input_id',$2::text,'control_id',$4::text,'unknown_effect_ids',$5::jsonb,'effect_reconciled',false),clock_timestamp() FROM changed`, id, inputID, a.OwnerID, a.DeliveryID, string(acknowledged))
	return err
}

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
