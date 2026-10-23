package budget

import (
	"context"
	"time"

	"github.com/butler/butler/internal/owneridentity"
)

// AdmitOwnerQuery is an explicit authenticated HTTP search, not a model run or
// background job. Reserve through the existing ledger before releasing the
// query to the pinned local transport. Even with owner/day disabled, the fixed
// twelve-attempt minute allowance remains shared across tabs and processes.
func (c *Controller) AdmitOwnerQuery(ctx context.Context, target LocalTarget, o LocalOperation, access LocalAccess) (context.Context, func(), error) {
	actor, ok := owneridentity.FromContext(ctx)
	if !ok || actor.Channel != "web" || o.Kind != "query" || o.Items != 1 || o.InputBytes > 2048 || o.Timeout > 30*time.Second || access == nil {
		return nil, nil, ErrInvalid
	}
	charge, err := c.localQuote(target, o)
	if err != nil {
		return nil, nil, err
	}
	id, err := NewOperationID()
	if err != nil {
		return nil, nil, err
	}
	tx, err := c.pool.Begin(ctx)
	if err != nil {
		return nil, nil, err
	}
	defer tx.Rollback(ctx)
	if err = access(ctx, tx); err != nil {
		return nil, nil, err
	}
	var now time.Time
	if err = tx.QueryRow(ctx, `SELECT clock_timestamp()`).Scan(&now); err != nil {
		return nil, nil, err
	}
	minute := now.UTC().Truncate(time.Minute)
	admitted, err := c.ReserveAuxiliaryTx(ctx, tx, AuxiliaryIdentity{
		OwnerID: actor.OwnerID, JobID: "owner-search:" + id, OperationID: "owner-search:" + id,
		Limits: charge, Deadline: now.Add(30 * time.Second), InputBytes: charge.ProviderInputBytes,
		Tokens: charge.Tokens, ActiveMillis: charge.ActiveMillis,
		PolicyID:       "owner-search:" + minute.Format(time.RFC3339),
		PolicyLimits:   Units{ModelTurns: 12, Tokens: 12 * 4096, ActiveMillis: 12 * 30000, ProviderInputBytes: 12 * 2048},
		PolicyDeadline: minute.Add(90 * time.Second),
	})
	if err != nil {
		return nil, nil, err
	}
	if err = access(ctx, tx); err != nil {
		return nil, nil, err
	}
	if err = tx.Commit(ctx); err != nil {
		return nil, nil, err
	}
	call, cancel := context.WithDeadline(ctx, admitted.Deadline)
	return call, cancel, nil
}
