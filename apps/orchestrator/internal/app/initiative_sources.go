package app

import (
	"context"

	"github.com/butler/butler/internal/memory/catalog"
	"github.com/jackc/pgx/v5/pgxpool"
)

// Recorded checks outlive their worker lease. This read-only check validates
// current source/transcript pins and the independent deletion anchor; it does
// not borrow old ownership or admit inference, tools, publication or a task.
// The observer separately filters the journal by exact owner/project/revision.
func checkRecordedInitiativeSources(ctx context.Context, pool *pgxpool.Pool, sources *catalog.Store, runID string) error {
	tx, err := pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)
	if err = sources.CheckRunSourcesTx(ctx, tx, runID); err != nil {
		return err
	}
	return tx.Commit(ctx)
}
