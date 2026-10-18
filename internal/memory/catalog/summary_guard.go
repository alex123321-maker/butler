package catalog

import (
	"context"
	"github.com/jackc/pgx/v5"
)

// A copied transcript has complete dynamic original-source privacy dependencies.
// Lock original and ordinary source rows together in sorted order before reading
// any body; corrections/deletion cannot race a publication on another source.
func lockSummarySourcesTx(ctx context.Context, tx pgx.Tx, ids []string) error {
	if len(ids) == 0 {
		return nil
	}
	_, err := tx.Exec(ctx, `WITH RECURSIVE roots AS(SELECT unnest($1::text[]) AS id UNION SELECT d.parent_source_id FROM memory_source_dependencies d JOIN roots r ON r.id=d.child_source_id),all_ids AS(SELECT id FROM roots UNION SELECT d.original_source_id FROM memory_summary_dependencies d JOIN roots r ON r.id=d.snapshot_source_id)SELECT s.source_id FROM memory_sources s JOIN all_ids a ON a.id=s.source_id ORDER BY s.source_id FOR SHARE OF s`, ids)
	if err != nil {
		return err
	}
	// Exact copied tool rows belong to experience observations, including when a
	// descendant is read. Lock only the immutable captured set; ordinary future
	// calls and global transcript watermarks do not revoke unrelated experience.
	_, err = tx.Exec(ctx, `WITH RECURSIVE roots AS(SELECT unnest($1::text[]) AS id UNION SELECT d.parent_source_id FROM memory_source_dependencies d JOIN roots r ON r.id=d.child_source_id)SELECT t.tool_call_id FROM tool_calls t JOIN memory_experience_tool_pins p USING(tool_call_id) JOIN roots r ON r.id=p.observation_source_id ORDER BY t.tool_call_id FOR SHARE OF t`, ids)
	if err != nil {
		return err
	}
	// Exact stopped-input evidence has a content-free metadata head. This shared
	// fence orders publication against an input writer without locking its large
	// checkpoint row or borrowing execution authority from the stopped attempt.
	_, err = tx.Exec(ctx, `SELECT butler_lock_memory_terminal_attempts($1)`, ids)
	if err != nil {
		return err
	}
	return nil
}

func checkSummarySourcesTx(ctx context.Context, tx pgx.Tx, ids []string) error {
	if len(ids) == 0 {
		return nil
	}
	if err := lockSummarySourcesTx(ctx, tx, ids); err != nil {
		return err
	}
	var current bool
	if err := tx.QueryRow(ctx, `SELECT butler_memory_summary_sources_current($1) AND butler_memory_experience_sources_current($1)`, ids).Scan(&current); err != nil {
		return err
	}
	if !current {
		return ErrStale
	}
	return nil
}

// SnapshotForRefreshTx returns data only to a trusted current policy checker.
// It permits primary-message/tool corrections, never a changed original access,
// Secret sensitivity, graph, or non-message ancestor's corrected content.
func (s *Store) SnapshotForRefreshTx(ctx context.Context, tx pgx.Tx, ref SourceRef) (Source, bool, error) {
	c, ok := ctx.Value(checkerKey{}).(AccessChecker)
	if !ok {
		return Source{}, false, ErrDenied
	}
	own, err := c.CheckMemoryAccessTx(ctx, tx, []SourceRef{{ID: ref.ID, AccessRevision: ref.AccessRevision}}, "memory_job_create")
	if err != nil {
		return Source{}, false, err
	}
	if err = lockSummarySourcesTx(ctx, tx, []string{ref.ID}); err != nil {
		return Source{}, false, err
	}
	src, err := getSourceTx(ctx, tx, ref.ID, false)
	if err != nil || src.OwnerID != own || src.AccessRevision != ref.AccessRevision || src.Status != "active" || src.ScopeType != "session" {
		return Source{}, false, ErrDenied
	}
	var current, allowed bool
	if err = tx.QueryRow(ctx, `SELECT butler_memory_summary_sources_current(ARRAY[$1::text]),butler_memory_summary_refresh_allowed($1)`, src.ID).Scan(&current, &allowed); err != nil {
		return Source{}, false, err
	}
	if !current && !allowed {
		return Source{}, false, ErrStale
	}
	return src, !current, nil
}
