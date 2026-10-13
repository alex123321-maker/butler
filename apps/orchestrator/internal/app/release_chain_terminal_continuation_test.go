package app

import (
	"context"
	"github.com/jackc/pgx/v5/pgxpool"
)

// A terminal durable INPUT, not a stale model_running run, ends the fixture
// wait. No live request deadline, state transition or permission is changed.
func releaseChainTerminalContinuation(ctx context.Context, pool *pgxpool.Pool, taskID string) (bool, error) {
	var failed bool
	err := pool.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM accepted_inputs WHERE task_id=$1 AND source='converter-return' AND status IN('blocked','failed','cancelled'))`, taskID).Scan(&failed)
	return failed, err
}
