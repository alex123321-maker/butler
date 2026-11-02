package app

import (
	"context"
	"fmt"
	"time"

	"github.com/butler/butler/internal/generatedtools"
	"github.com/butler/butler/internal/memory/catalog"
	"github.com/butler/butler/internal/ownership"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// Source/current task authority precedes converter lifecycle at every provider
// release/event/recovery. No private Broker RPC runs while PG locks are held.
type converterSources struct {
	*catalog.Store
	pool *pgxpool.Pool
}

func (converterSources) MemorySourceCheckTimeout() time.Duration { return 2 * time.Second }

// This classifies capacity and read-only observation finalization, never
// authority or source proof. Returned Inputs and pinned usages stay compound.
// Both routes execute all the same current guards below. A stale classification
// can fail closed; it cannot authorize a cached source or completed worker.
func converterRunParticipates(ctx context.Context, db interface {
	QueryRow(context.Context, string, ...any) pgx.Row
}, id string) (bool, error) {
	var related bool
	err := db.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM accepted_inputs i WHERE i.run_id=r.run_id AND i.source='converter-return') OR EXISTS(SELECT 1 FROM converter_usages u WHERE u.run_id=r.run_id) OR EXISTS(SELECT 1 FROM converter_developer_snapshots a WHERE a.task_id=r.task_id OR a.parent_task_id=r.task_id) OR EXISTS(SELECT 1 FROM memory_run_sources p JOIN converter_versions v ON v.source_ref_json->>'source_id'=p.source_id JOIN runs created ON created.run_id=v.created_by_run JOIN converter_developer_snapshots a ON a.task_id=created.task_id WHERE p.run_id=r.run_id) FROM runs r WHERE r.run_id=$1`, id).Scan(&related)
	return related, err
}

func (s converterSources) CheckRunSources(ctx context.Context, id string) error {
	t, ok := ownership.TokenFromContext(ctx)
	if !ok || t.RunID != id {
		return generatedtools.ErrAccess
	}
	pool := s.pool
	observerOnly := false
	observations := s.Store.RunObservationPool(ctx)
	if observations != s.pool {
		// Classify using business capacity when it is available. A saturated
		// business pool must not prevent an unrelated token-only observation.
		probe := observations
		stat := s.pool.Stat()
		if stat.IdleConns() > 0 || stat.TotalConns() < s.pool.Config().MaxConns {
			probe = s.pool
		}
		related, err := converterRunParticipates(ctx, probe, id)
		if err != nil {
			return err
		}
		if !related {
			pool = observations
			observerOnly = true
		}
	}
	tx, e := pool.Begin(ctx)
	if e != nil {
		return e
	}
	defer func() {
		if tx != nil {
			_ = tx.Rollback(ctx)
		}
	}()
	if _, e = s.Store.AccessTx(ctx, tx, nil, "memory_read"); e != nil {
		return e
	}
	if pool != s.pool {
		// Repeat under AccessTx's current ownership/run/task locks. A converter
		// role accepted since classification must move to business capacity;
		// never carry that earlier read forward as authorization.
		related, err := converterRunParticipates(ctx, tx, id)
		if err != nil {
			return err
		}
		if related {
			observerOnly = false
			if e = tx.Rollback(ctx); e != nil {
				return e
			}
			tx, e = s.pool.Begin(ctx)
			if e != nil {
				return e
			}
			if _, e = s.Store.AccessTx(ctx, tx, nil, "memory_read"); e != nil {
				return e
			}
		}
	}
	if e = s.Store.CheckRunSourcesTx(ctx, tx, id); e != nil {
		return e
	}
	if e = generatedtools.CheckContinuationRunTx(ctx, tx, s.Store, id); e != nil {
		return fmt.Errorf("converter continuation: %w", e)
	}
	if e = generatedtools.CheckDeveloperLifecycleTx(ctx, tx, s.Store, id); e != nil {
		return fmt.Errorf("converter child lifecycle: %w", e)
	}
	if e = generatedtools.CheckDeveloperReturnLifecycleTx(ctx, tx, s.Store, id); e != nil {
		return fmt.Errorf("converter return lifecycle: %w", e)
	}
	if e = generatedtools.CheckPinnedLifecycleTx(ctx, tx, id); e != nil {
		return fmt.Errorf("converter catalog lifecycle: %w", e)
	}
	// Unrelated token-only observations select no new roots and all lifecycle
	// guards above are reads. Keep every current proof and row lock, but release
	// them without adding a COMMIT/WAL-sync dependency to each 250ms observation.
	// Compound converter/owner/checker contexts retain their owning commit.
	if observerOnly {
		return tx.Rollback(ctx)
	}
	return tx.Commit(ctx)
}
