package catalog

import (
	"context"
	"github.com/butler/butler/internal/delegation"
	"github.com/butler/butler/internal/owneridentity"
	"github.com/butler/butler/internal/ownership"

	"github.com/jackc/pgx/v5"
)

// CheckRunSources checks a current owned run before a model request/stream
// observation. It never reuses a completed run as auxiliary authority.
func (s *Store) CheckRunSources(ctx context.Context, runID string) (err error) {
	ctx, finish := beginRunSourceProbe(ctx, s)
	defer func() { finish(err) }()
	token, ok := ownership.TokenFromContext(ctx)
	if !ok || token.RunID != runID {
		return ErrDenied
	}
	runSourcePhase(ctx, "pool_begin")
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
	// The ordinary token-only observation passes no selected refs or auxiliary
	// checker, so AccessTx cannot bind memory_run_sources and all remaining
	// helpers are current SELECT/row-lock proofs. There is no business mutation
	// to make durable. Bounded rollback releases the same locks without adding
	// an observer COMMIT/WAL-sync requirement. HTTP dispatch and publication
	// callers of CheckRunSourcesTx retain their owning transaction commits.
	_, ownerActor := owneridentity.FromContext(ctx)
	_, auxiliary := ctx.Value(checkerKey{}).(AccessChecker)
	if !ownerActor && !auxiliary {
		runSourcePhase(ctx, "observer_rollback")
		return tx.Rollback(ctx)
	}
	// Preserve the existing commit semantics for trusted compound contexts;
	// their independently supplied checker is outside the token-only audit.
	runSourcePhase(ctx, "observer_commit")
	return tx.Commit(ctx)
}

// CheckRunSourcesTx is wired by the owning app at the irreversible dispatch
// boundary, after run/task/goal locks and before budget/grant CAS. Publication
// triggers call the same PG guard. No additional pool connection is acquired.
func (s *Store) CheckRunSourcesTx(ctx context.Context, tx pgx.Tx, runID string) error {
	runSourcePhase(ctx, "run_restore")
	if e := s.CheckRestoreTx(ctx, tx); e != nil {
		return e
	}
	var taskID string
	runSourcePhase(ctx, "run_current")
	if e := tx.QueryRow(ctx, `SELECT COALESCE(task_id,'') FROM runs WHERE run_id=$1 FOR SHARE`, runID).Scan(&taskID); e != nil {
		return e
	}
	if taskID != "" {
		runSourcePhase(ctx, "run_delegation")
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
	runSourcePhase(ctx, "run_source_pins")
	_, e := tx.Exec(ctx, `SELECT butler_memory_check_run_sources($1)`, runID)
	if e == nil {
		var ids []string
		e = tx.QueryRow(ctx, `SELECT COALESCE(array_agg(source_id),'{}'::text[]) FROM memory_run_sources WHERE run_id=$1`, runID).Scan(&ids)
		if e == nil {
			runSourcePhase(ctx, "run_summary_originals")
			e = checkSummarySourcesTx(ctx, tx, ids)
		}
		if e == nil {
			runSourcePhase(ctx, "run_temporal_sources")
			e = checkTemporalSourcesTx(ctx, tx, ids)
		}
		if e == nil {
			runSourcePhase(ctx, "run_temporal_pins")
			e = checkTemporalRunTx(ctx, tx, runID)
		}
	}
	return e
}
