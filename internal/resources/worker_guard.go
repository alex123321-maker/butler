package resources

import (
	"context"
	"errors"
	"time"

	"github.com/butler/butler/internal/actionauth"
	"github.com/butler/butler/internal/ownership"
	"github.com/jackc/pgx/v5"
)

// WorkerGuard checks durable Butler ownership, run and task/mandate/parent
// versions using the caller transaction, including a pool with two connections.
// Resource authority is additional to Broker authorization; it cannot authorize
// an effect, release a credential or reserve/refill an execution budget.
type WorkerGuard struct{}

func (WorkerGuard) CheckTx(ctx context.Context, tx pgx.Tx, b Binding) error {
	if !validBinding(b) {
		return ErrDenied
	}
	token, ok := ownership.TokenFromContext(ctx)
	if !ok || token.SessionKey != b.SessionKey || token.RunID != b.RunID || token.FenceEpoch != b.LeaseEpoch || token.LeaseID == "" {
		return ErrDenied
	}
	var valid bool
	err := tx.QueryRow(ctx, `SELECT run_id=$2 AND lease_id=$3 AND fence_epoch=$4 AND expires_at>clock_timestamp() AND $5::timestamptz<=expires_at AND $5::timestamptz>clock_timestamp() FROM session_ownership WHERE session_key=$1 FOR SHARE`, b.SessionKey, b.RunID, token.LeaseID, b.LeaseEpoch, b.Deadline).Scan(&valid)
	if errors.Is(err, pgx.ErrNoRows) || err == nil && !valid {
		return ErrDenied
	}
	if err != nil {
		return err
	}
	var owner string
	if err = tx.QueryRow(ctx, `SELECT user_id FROM sessions WHERE session_key=$1 FOR SHARE`, b.SessionKey).Scan(&owner); err != nil {
		return err
	}
	if owner != b.OwnerID {
		return ErrDenied
	}
	var taskID, session, state, status string
	var pinned int64
	if err = tx.QueryRow(ctx, `SELECT session_key,COALESCE(task_id,''),COALESCE(task_revision,0),current_state,status FROM runs WHERE run_id=$1 FOR SHARE`, b.RunID).Scan(&session, &taskID, &pinned, &state, &status); err != nil {
		return err
	}
	if session != b.SessionKey || taskID != b.TaskID || pinned != b.TaskRevision {
		return ErrDenied
	}
	for _, value := range []string{state, status} {
		switch value {
		case "", "completed", "failed", "cancelled", "timed_out", "finalizing":
			return ErrDenied
		}
	}
	var revision, constraints int64
	if err = tx.QueryRow(ctx, `SELECT owner_id,session_key,status,revision,constraints_revision FROM tasks WHERE task_id=$1 FOR SHARE`, b.TaskID).Scan(&owner, &session, &status, &revision, &constraints); err != nil {
		return err
	}
	if owner != b.OwnerID || session != b.SessionKey || status != "active" || revision != b.TaskRevision || constraints != b.ConstraintsRevision {
		return ErrDenied
	}
	var now time.Time
	if err = tx.QueryRow(ctx, `SELECT clock_timestamp()`).Scan(&now); err != nil {
		return err
	}
	_, err = actionauth.CheckTaskAuthorityTx(ctx, tx, b.TaskID, b.OwnerID, nil, now)
	return err
}
