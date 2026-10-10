package task

import (
	"context"

	"github.com/butler/butler/internal/ownership"
)

// BlockRun records an execution obstacle without changing the owner's semantic
// task revision or overriding a concurrent pause/cancellation/edit.
func (s *Store) BlockRun(ctx context.Context, runID, reason string) error {
	ctx, release, err := ownership.Begin(ctx)
	if err != nil {
		return err
	}
	defer release()
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)
	var id *string
	if err = tx.QueryRow(ctx, `SELECT task_id FROM runs WHERE run_id=$1 FOR SHARE`, runID).Scan(&id); err != nil {
		return err
	}
	if id == nil {
		return nil
	}
	if err = guardTaskWrite(ctx, tx, *id); err != nil {
		return err
	}
	_, err = tx.Exec(ctx, `UPDATE tasks SET status='blocked',working_json=jsonb_set(working_json,'{scratch}',COALESCE(working_json->'scratch','{}'::jsonb)||jsonb_build_object('block_reason',$2::text)),working_revision=working_revision+1,updated_at=clock_timestamp() WHERE status IN ('active','waiting_for_user') AND EXISTS(SELECT 1 FROM runs WHERE run_id=$1 AND runs.task_id=tasks.task_id AND runs.task_revision=tasks.revision)`, runID, reason)
	if err != nil {
		return err
	}
	return tx.Commit(ctx)
}
