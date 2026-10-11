package app

import (
	"context"

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

func (s converterSources) CheckRunSources(ctx context.Context, id string) error {
	t, ok := ownership.TokenFromContext(ctx)
	if !ok || t.RunID != id {
		return generatedtools.ErrAccess
	}
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
	if e = generatedtools.CheckPinnedLifecycleTx(ctx, tx, id); e != nil {
		return e
	}
	return tx.Commit(ctx)
}
