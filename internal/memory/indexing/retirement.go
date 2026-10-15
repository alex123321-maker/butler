package indexing

import (
	"context"

	"github.com/butler/butler/internal/memory/catalog"
	"github.com/butler/butler/internal/memory/semantic"
)

// Retire is private owner maintenance. It requires no Jobs controller, model,
// backend, live task grant or source content; it cannot create or activate an
// index. Retirement and vector removal commit together with current owner proof.
func (r *Runtime) Retire(ctx context.Context, q semantic.RetireRequest) (semantic.Retirement, error) {
	if r == nil || r.Catalog == nil {
		return semantic.Retirement{}, catalog.ErrInvalid
	}
	tx, err := r.Catalog.Pool().Begin(ctx)
	if err != nil {
		return semantic.Retirement{}, err
	}
	defer tx.Rollback(ctx)
	if err = r.Catalog.CheckRestoreTx(ctx, tx); err != nil {
		return semantic.Retirement{}, err
	}
	out, err := semantic.NewStore(r.Catalog.Pool()).RetireTx(ctx, tx, q)
	if err != nil {
		return semantic.Retirement{}, err
	}
	return out, tx.Commit(ctx)
}
