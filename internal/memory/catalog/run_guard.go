package catalog

import (
	"context"
	"github.com/butler/butler/internal/delegation"
	"github.com/butler/butler/internal/ownership"

	"github.com/jackc/pgx/v5"
)

// CheckRunSources checks a current owned run before a model request/stream
// observation. It never reuses a completed run as auxiliary authority.
func (s *Store) CheckRunSources(ctx context.Context, runID string) error {
	token, ok := ownership.TokenFromContext(ctx)
	if !ok || token.RunID != runID {
		return ErrDenied
	}
	tx, e := s.pool.Begin(ctx)
	if e != nil {
		return e
	}
	defer tx.Rollback(ctx)
	if _, e = s.AccessTx(ctx, tx, nil, "memory_read"); e != nil {
		return e
	}
	if e = s.CheckRunSourcesTx(ctx, tx, runID); e != nil {
		return e
	}
	return tx.Commit(ctx)
}

// CheckRunSourcesTx is wired by the owning app at the irreversible dispatch
// boundary, after run/task/goal locks and before budget/grant CAS. Publication
// triggers call the same PG guard. No additional pool connection is acquired.
func (s *Store) CheckRunSourcesTx(ctx context.Context, tx pgx.Tx, runID string) error {
	if e := s.CheckRestoreTx(ctx, tx); e != nil {
		return e
	}
	var taskID string
	if e := tx.QueryRow(ctx, `SELECT COALESCE(task_id,'') FROM runs WHERE run_id=$1 FOR SHARE`, runID).Scan(&taskID); e != nil {
		return e
	}
	if taskID != "" {
		bounded, e := delegation.LoadTx(ctx, tx, taskID)
		if e != nil {
			return e
		}
		if bounded != nil {
			if e = s.CheckDelegationSourcesTx(ctx, tx, taskID, delegationRoots(bounded), "memory_read"); e != nil {
				return e
			}
		}
	}
	_, e := tx.Exec(ctx, `SELECT butler_memory_check_run_sources($1)`, runID)
	if e == nil {
		var ids []string
		e = tx.QueryRow(ctx, `SELECT COALESCE(array_agg(source_id),'{}'::text[]) FROM memory_run_sources WHERE run_id=$1`, runID).Scan(&ids)
		if e == nil {
			e = checkSummarySourcesTx(ctx, tx, ids)
		}
	}
	return e
}
