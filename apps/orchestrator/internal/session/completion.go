package session

import (
	"context"
	"github.com/butler/butler/internal/ownership"
	"github.com/jackc/pgx/v5"
)

// LockCompletionTx follows the ownership guard and precedes run/task locks.
// The transcript revision trigger also locks this session row.
func (r *PostgresRepository) LockCompletionTx(ctx context.Context, tx pgx.Tx, key string) (string, error) {
	token, ok := ownership.TokenFromContext(ctx)
	if !ok || token.SessionKey != key {
		return "", ownership.ErrStale
	}
	var channel string
	err := tx.QueryRow(ctx, `SELECT channel FROM sessions WHERE session_key=$1 FOR UPDATE`, key).Scan(&channel)
	return channel, err
}
