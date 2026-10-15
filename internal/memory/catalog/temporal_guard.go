package catalog

import (
	"context"

	"github.com/jackc/pgx/v5"
)

// Original source locks precede these owner-head locks. Metadata updates take
// the same head FOR UPDATE, including when no temporal revision existed yet.
// A default-off feature flag cannot disable already-bound derivative privacy.
func checkTemporalSourcesTx(ctx context.Context, tx pgx.Tx, ids []string) error {
	if len(ids) == 0 {
		return nil
	}
	_, err := tx.Exec(ctx, `WITH RECURSIVE roots AS(SELECT unnest($1::text[]) AS id UNION SELECT d.parent_source_id FROM memory_source_dependencies d JOIN roots r ON r.id=d.child_source_id) SELECT h.owner_id FROM memory_fact_heads h WHERE EXISTS(SELECT 1 FROM memory_temporal_source_pins p JOIN roots r ON r.id=p.derived_source_id JOIN memory_claims c ON c.claim_id=p.claim_id WHERE h.owner_id=c.owner_id AND h.scope_type=c.scope_type AND h.scope_id=c.scope_id AND h.fact_key=c.fact_key) ORDER BY h.owner_id,h.scope_type,h.scope_id,h.fact_key FOR SHARE OF h`, ids)
	if err != nil {
		return err
	}
	var current bool
	if err = tx.QueryRow(ctx, `SELECT butler_memory_temporal_sources_current($1)`, ids).Scan(&current); err != nil {
		return err
	}
	if !current {
		return ErrStale
	}
	return nil
}
