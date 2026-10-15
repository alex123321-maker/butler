package catalog

import (
	"context"
	"github.com/jackc/pgx/v5"
)

// A copied transcript has complete dynamic original-source privacy dependencies.
// Lock original and ordinary source rows together in sorted order before reading
// any body; corrections/deletion cannot race a publication on another source.
func checkSummarySourcesTx(ctx context.Context, tx pgx.Tx, ids []string) error {
	if len(ids) == 0 {
		return nil
	}
	_, err := tx.Exec(ctx, `WITH RECURSIVE roots AS(SELECT unnest($1::text[]) AS id UNION SELECT d.parent_source_id FROM memory_source_dependencies d JOIN roots r ON r.id=d.child_source_id),all_ids AS(SELECT id FROM roots UNION SELECT d.original_source_id FROM memory_summary_dependencies d JOIN roots r ON r.id=d.snapshot_source_id)SELECT s.source_id FROM memory_sources s JOIN all_ids a ON a.id=s.source_id ORDER BY s.source_id FOR SHARE OF s`, ids)
	if err != nil {
		return err
	}
	var current bool
	if err = tx.QueryRow(ctx, `SELECT butler_memory_summary_sources_current($1)`, ids).Scan(&current); err != nil {
		return err
	}
	if !current {
		return ErrStale
	}
	return nil
}
