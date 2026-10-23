package ownership

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// BeginTransaction is for owning writers whose work joins one transaction.
// It locks the live fencing row in that same transaction, leaving a connection
// available to the ownership watchdog even in the supported two-connection pool.
// Returned context is local to this transaction and must not escape or be used
// for a separate business write. Existing multi-repository guards use Begin.
func BeginTransaction(ctx context.Context, pool *pgxpool.Pool) (context.Context, pgx.Tx, error) {
	e, owned := ctx.Value(executionKey{}).(execution)
	if !owned {
		tx, err := pool.Begin(ctx)
		return ctx, tx, err
	}
	if e.authority == nil || e.authority.pool != pool || pool.Config().MaxConns < 2 {
		return ctx, nil, fmt.Errorf("fenced transaction requires its owning pool with at least two connections")
	}
	release := func() {}
	if !e.guarded {
		select {
		case e.authority.slots <- struct{}{}:
		case <-ctx.Done():
			return ctx, nil, ctx.Err()
		}
		var once sync.Once
		release = func() { once.Do(func() { <-e.authority.slots }) }
	}
	tx, err := pool.Begin(ctx)
	if err != nil {
		release()
		return ctx, nil, err
	}
	guarded := &fencedTransaction{Tx: tx, token: e.token, release: release}
	if err = guarded.validate(ctx); err != nil {
		guarded.cleanup()
		return ctx, nil, err
	}
	e.guarded = true
	return context.WithValue(ctx, executionKey{}, e), guarded, nil
}

type fencedTransaction struct {
	pgx.Tx
	token   Token
	release func()
	closed  bool
}

func (t *fencedTransaction) validate(ctx context.Context) error {
	var valid bool
	err := t.Tx.QueryRow(ctx, `SELECT lease_id=$2 AND run_id=$3 AND fence_epoch=$4 AND expires_at>clock_timestamp() FROM session_ownership WHERE session_key=$1 FOR SHARE`, t.token.SessionKey, t.token.LeaseID, t.token.RunID, t.token.FenceEpoch).Scan(&valid)
	if errors.Is(err, pgx.ErrNoRows) || err == nil && !valid {
		return ErrStale
	}
	if err != nil {
		return fmt.Errorf("validate fenced transaction: %w", err)
	}
	return nil
}
func (t *fencedTransaction) cleanup() {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	_ = t.Rollback(ctx)
}
func (t *fencedTransaction) Commit(ctx context.Context) error {
	if t.closed {
		return pgx.ErrTxClosed
	}
	if err := t.validate(ctx); err != nil {
		t.cleanup()
		return err
	}
	err := t.Tx.Commit(ctx)
	t.closed = true
	t.release()
	return err
}
func (t *fencedTransaction) Rollback(ctx context.Context) error {
	if t.closed {
		return pgx.ErrTxClosed
	}
	err := t.Tx.Rollback(ctx)
	t.closed = true
	t.release()
	return err
}
