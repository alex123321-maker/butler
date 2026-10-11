package generatedtools

import (
	"context"

	"github.com/jackc/pgx/v5/pgxpool"
)

// CheckDeveloperReady checks the owned accepted-role and reference-only return
// schema before an operator enables the optional developer bridge. It grants no
// caller authority and does not replace the per-call current lifecycle guards.
func CheckDeveloperReady(ctx context.Context, pool *pgxpool.Pool) error {
	if pool == nil {
		return ErrAccess
	}
	var immutable bool
	err := pool.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM pg_trigger WHERE tgrelid='converter_developer_snapshots'::regclass AND tgname='converter_developer_immutable' AND NOT tgisinternal AND tgenabled='O') AND EXISTS(SELECT 1 FROM pg_trigger WHERE tgrelid='converter_developer_returns'::regclass AND tgname='converter_return_immutable' AND NOT tgisinternal AND tgenabled='O') AND NOT EXISTS(SELECT 1 FROM converter_developer_snapshots a LEFT JOIN converter_developer_returns r ON r.child_task_id=a.task_id WHERE a.snapshot_hash IS NULL OR (r.child_task_id IS NOT NULL AND r.projection_hash IS NULL))`).Scan(&immutable)
	if err != nil || !immutable {
		return ErrAccess
	}
	return nil
}
