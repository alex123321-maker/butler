package catalog

import (
	"context"
	"time"

	"github.com/butler/butler/internal/actionauth"
	"github.com/butler/butler/internal/delegation"
	"github.com/butler/butler/internal/owneridentity"
	"github.com/butler/butler/internal/ownership"
	"github.com/jackc/pgx/v5"
)

// finalizeAccessTx follows AccessTx in the same transaction. Its ownership,
// run, task, authority and source rows are already locked. Repeat time-sensitive
// checks after waits without borrowing a connection or reversing lock order.
func (s *Store) finalizeAccessTx(ctx context.Context, tx pgx.Tx, refs []SourceRef, purpose string) error {
	if _, ok := owneridentity.FromContext(ctx); ok {
		return nil
	}
	if c, ok := ctx.Value(checkerKey{}).(AccessChecker); ok {
		return c.FinalizeMemoryAccessTx(ctx, tx, refs, purpose)
	}
	tok, ok := ownership.TokenFromContext(ctx)
	if !ok {
		return ErrDenied
	}
	var valid bool
	if tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM session_ownership o JOIN runs r ON r.run_id=o.run_id WHERE o.session_key=$1 AND o.run_id=$2 AND o.lease_id=$3 AND o.fence_epoch=$4 AND o.expires_at>clock_timestamp() AND r.session_key=o.session_key AND r.current_state NOT IN ('completed','failed','cancelled','timed_out','finalizing') AND r.status NOT IN ('completed','failed','cancelled','timed_out','finalizing'))`, tok.SessionKey, tok.RunID, tok.LeaseID, tok.FenceEpoch).Scan(&valid) != nil || !valid {
		return ErrDenied
	}
	var taskID, own string
	if tx.QueryRow(ctx, `SELECT COALESCE(r.task_id,''),s.user_id FROM runs r JOIN sessions s USING(session_key) WHERE r.run_id=$1`, tok.RunID).Scan(&taskID, &own) != nil {
		return ErrDenied
	}
	if taskID != "" {
		var now time.Time
		if tx.QueryRow(ctx, `SELECT clock_timestamp()`).Scan(&now) != nil {
			return ErrDenied
		}
		if _, err := actionauth.CheckTaskAuthorityTx(ctx, tx, taskID, own, nil, now); err != nil {
			return err
		}
		bounded, err := delegation.LoadTx(ctx, tx, taskID)
		if err != nil {
			return err
		}
		if bounded != nil {
			return s.CheckDelegationSourcesTx(ctx, tx, taskID, delegationRoots(bounded), "memory_read")
		}
	}
	return nil
}
