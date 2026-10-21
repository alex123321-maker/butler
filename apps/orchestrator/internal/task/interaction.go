package task

import (
	"context"
	"github.com/jackc/pgx/v5"
)

// WaitInteractionTx is an owning task hook used inside publication's transaction.
func WaitInteractionTx(ctx context.Context, tx pgx.Tx, id string, revision, constraints int64) error {
	if id == "" {
		return nil
	}
	if err := guardTaskWrite(ctx, tx, id); err != nil {
		return err
	}
	tag, e := tx.Exec(ctx, `UPDATE tasks SET status='waiting_for_user',updated_at=clock_timestamp() WHERE task_id=$1 AND revision=$2 AND constraints_revision=$3 AND status='active'`, id, revision, constraints)
	if e == nil && tag.RowsAffected() != 1 {
		return ErrConflict
	}
	return e
}

// ResumeInteractionTx never grants authority or changes the owner's revisions.
func ResumeInteractionTx(ctx context.Context, tx pgx.Tx, id string, revision, constraints int64) error {
	if id == "" {
		return nil
	}
	if err := guardTaskWrite(ctx, tx, id); err != nil {
		return err
	}
	tag, e := tx.Exec(ctx, `UPDATE tasks SET status='active',updated_at=clock_timestamp() WHERE task_id=$1 AND revision=$2 AND constraints_revision=$3 AND status IN ('waiting_for_user','active')`, id, revision, constraints)
	if e == nil && tag.RowsAffected() != 1 {
		return ErrConflict
	}
	return e
}

// CloseInteractionWaitTx is the maintenance counterpart to a timed-out question.
// It can only block the exact existing wait, never activate or complete a task.
func CloseInteractionWaitTx(ctx context.Context, tx pgx.Tx, id string, revision, constraints int64) error {
	if id == "" {
		return nil
	}
	_, e := tx.Exec(ctx, `UPDATE tasks SET status='blocked',updated_at=clock_timestamp() WHERE task_id=$1 AND revision=$2 AND constraints_revision=$3 AND status='waiting_for_user'`, id, revision, constraints)
	return e
}
