package app

import (
	"context"
	"fmt"
	"time"

	"github.com/butler/butler/internal/generatedtools"
	"github.com/butler/butler/internal/memory/catalog"
	"github.com/butler/butler/internal/ownership"
	"github.com/jackc/pgx/v5/pgxpool"
)

// Source/current task authority precedes converter lifecycle at every provider
// release/event/recovery. No private Broker RPC runs while PG locks are held.
type converterSources struct {
	*catalog.Store
	pool *pgxpool.Pool
}

func (converterSources) MemorySourceCheckTimeout() time.Duration { return 2 * time.Second }

func (s converterSources) CheckRunSources(ctx context.Context, id string) error {
	t, ok := ownership.TokenFromContext(ctx)
	if !ok || t.RunID != id {
		return generatedtools.ErrAccess
	}
	// Returned-package validation can bind selected source pins and repeat full
	// converter/continuation guards. This compound foreground proof must not hold
	// the single connection reserved for bounded lease/authority observations.
	tx, e := s.pool.Begin(ctx)
	if e != nil {
		return e
	}
	defer tx.Rollback(ctx)
	if _, e = s.Store.AccessTx(ctx, tx, nil, "memory_read"); e != nil {
		return e
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
	return tx.Commit(ctx)
}
