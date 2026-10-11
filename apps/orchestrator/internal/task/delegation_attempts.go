package task

import (
	"context"
	"time"

	"github.com/butler/butler/internal/delegation"
	"github.com/butler/butler/internal/owneridentity"
	"github.com/butler/butler/internal/ownership"
	"github.com/jackc/pgx/v5"
)

// This adapter observes owning records and current parent source access. It
// cannot reuse failed child execution authority, collect a final or mutate a
// delegation state. Ordinary lifecycle checks continue requiring an active child.
type DelegationAttemptObserver interface {
	ObserveAttemptsTx(context.Context, pgx.Tx, Record, Record, delegation.Contract) (delegation.AttemptReport, error)
}

func (m *DelegationManager) ObserveFailedAttempts(ctx context.Context, parentID, childID string) (delegation.AttemptReport, error) {
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	if _, worker := ownership.TokenFromContext(ctx); worker {
		if _, actor := owneridentity.FromContext(ctx); actor {
			return delegation.AttemptReport{}, ErrUnauthorized
		}
	}
	observer, ok := m.guard.(DelegationAttemptObserver)
	if !ok {
		return delegation.AttemptReport{}, delegation.ErrDenied
	}
	tx, _, parent, err := m.begin(ctx, parentID)
	if err != nil {
		return delegation.AttemptReport{}, err
	}
	defer delegationRollback(tx)
	if err = delegationCallerTx(ctx, tx, parent); err != nil {
		return delegation.AttemptReport{}, err
	}
	child, err := m.store.GetTx(ctx, tx, childID)
	if err != nil {
		return delegation.AttemptReport{}, err
	}
	v, err := m.loadTx(ctx, tx, childID)
	if err != nil {
		return delegation.AttemptReport{}, err
	}
	c := v.Contract
	// Admit creates the child at constraints revision1. Contract constraints
	// pin the parent instead; a parent's revision2 is not a child's revision2.
	if child.ParentTaskID != parent.TaskID || child.OwnerID != parent.OwnerID || c.TaskID != child.TaskID || c.ParentTaskID != parent.TaskID || c.ParentRevision != parent.Revision || c.ConstraintsRevision != parent.ConstraintsRevision || child.ConstraintsRevision != 1 || child.Status != "blocked" {
		return delegation.AttemptReport{}, ErrConflict
	}
	var live bool
	// Holding the tree/session admission barrier prevents a new child attempt
	// racing this observation. A still-live failed worker is never borrowed.
	if err = tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM session_ownership WHERE session_key=$1 AND lease_id<>'' AND expires_at>clock_timestamp()) OR EXISTS(SELECT 1 FROM runs WHERE task_id=$2 AND current_state NOT IN ('completed','failed','cancelled','timed_out'))`, child.SessionKey, child.TaskID).Scan(&live); err != nil || live {
		return delegation.AttemptReport{}, ErrConflict
	}
	report, err := observer.ObserveAttemptsTx(ctx, tx, parent, child, c)
	if err != nil {
		return delegation.AttemptReport{}, err
	}
	if report.TaskID != child.TaskID || report.ParentTaskID != parent.TaskID || report.TaskRevision != child.Revision || report.ConstraintsRevision != child.ConstraintsRevision || report.ContractRevision != c.Revision || len(report.Attempts) == 0 {
		return delegation.AttemptReport{}, ErrConflict
	}
	if err = delegationCallerTx(ctx, tx, parent); err != nil {
		return delegation.AttemptReport{}, err
	}
	if err = tx.Commit(ctx); err != nil {
		return delegation.AttemptReport{}, err
	}
	return report, nil
}
