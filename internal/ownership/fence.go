// Package ownership carries a Butler session fencing token through execution.
// It contains no provider-side authority and no process-global lease state.
package ownership

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

var ErrStale = errors.New("execution ownership expired or superseded")

type Token struct {
	SessionKey string
	RunID      string
	LeaseID    string
	FenceEpoch int64
}

type Authority struct {
	pool      *pgxpool.Pool
	slots     chan struct{}
	mutations chan struct{}
}

// Guard uses a second pooled connection while a repository performs its write.
// Bound guards to half the pool so concurrent guards cannot exhaust all write
// connections. The production pool must have at least two connections.
func NewAuthority(pool *pgxpool.Pool) *Authority {
	n := int(pool.Config().MaxConns / 2)
	if n < 1 {
		n = 1
	}
	return &Authority{pool: pool, slots: make(chan struct{}, n), mutations: make(chan struct{}, 1)}
}

// BeginMutation waits outside the connection pool for local guarded writes.
// This prevents a lease UPDATE waiting on a row guard from taking the last
// connection needed by that guard's business write (including a pool of two).
// One authority instance is shared by all leases/worker tokens in this process.
func (a *Authority) BeginMutation(ctx context.Context) (func(), error) {
	select {
	case a.mutations <- struct{}{}:
	case <-ctx.Done():
		return nil, ctx.Err()
	}
	taken := 0
	for taken < cap(a.slots) {
		select {
		case a.slots <- struct{}{}:
			taken++
		case <-ctx.Done():
			for taken > 0 {
				<-a.slots
				taken--
			}
			<-a.mutations
			return nil, ctx.Err()
		}
	}
	return func() {
		for range cap(a.slots) {
			<-a.slots
		}
		<-a.mutations
	}, nil
}

type executionKey struct{}
type execution struct {
	token     Token
	authority *Authority
	guarded   bool
}

func WithToken(ctx context.Context, authority *Authority, token Token) context.Context {
	return context.WithValue(ctx, executionKey{}, execution{token: token, authority: authority})
}

func TokenFromContext(ctx context.Context) (Token, bool) {
	e, ok := ctx.Value(executionKey{}).(execution)
	return e.token, ok
}

// Begin holds a shared row lock until release. Lease takeover requires an
// exclusive lock on the same row, so a validated authority write completes
// before takeover; a superseded executor cannot start a later write.
// Controller writes without an execution token are outside worker authority.
func Begin(ctx context.Context) (context.Context, func(), error) {
	e, ok := ctx.Value(executionKey{}).(execution)
	if !ok || e.guarded {
		return ctx, func() {}, nil
	}
	if e.authority == nil || e.authority.pool.Config().MaxConns < 2 {
		return ctx, nil, fmt.Errorf("fencing requires a PostgreSQL pool with at least two connections")
	}
	select {
	case e.authority.slots <- struct{}{}:
	case <-ctx.Done():
		return ctx, nil, ctx.Err()
	}
	tx, err := e.authority.pool.Begin(ctx)
	if err != nil {
		<-e.authority.slots
		return ctx, nil, err
	}
	release := func() {
		cleanup, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		_ = tx.Rollback(cleanup)
		<-e.authority.slots
	}
	var valid bool
	err = tx.QueryRow(ctx, `SELECT lease_id=$2 AND run_id=$3 AND fence_epoch=$4 AND expires_at>clock_timestamp() FROM session_ownership WHERE session_key=$1 FOR SHARE`, e.token.SessionKey, e.token.LeaseID, e.token.RunID, e.token.FenceEpoch).Scan(&valid)
	if errors.Is(err, pgx.ErrNoRows) || (err == nil && !valid) {
		release()
		return ctx, nil, ErrStale
	}
	if err != nil {
		release()
		return ctx, nil, fmt.Errorf("validate fenced write: %w", err)
	}
	e.guarded = true
	return context.WithValue(ctx, executionKey{}, e), release, nil
}
