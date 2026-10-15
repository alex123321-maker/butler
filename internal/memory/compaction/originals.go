package compaction

import (
	"context"
	"github.com/butler/butler/internal/memory/catalog"
	"github.com/jackc/pgx/v5"
)

// pinOriginalsTx captures complete current privacy dependencies, including
// non-message ancestors. A snapshot cannot launder a project/other-task source
// into a session policy or inherit Secret data through a historical quotation.
func pinOriginalsTx(ctx context.Context, tx pgx.Tx, id, session, owner, task string, revision int64) error {
	var current, last int64
	if err := tx.QueryRow(ctx, `SELECT source_revision,last_sequence FROM transcript_heads WHERE session_key=$1`, session).Scan(&current, &last); err != nil {
		return err
	}
	if current != revision {
		return catalog.ErrStale
	}
	rows, err := tx.Query(ctx, `WITH RECURSIVE originals AS(
 SELECT 'message:'||message_id AS id FROM messages WHERE session_key=$1
 UNION SELECT d.parent_source_id FROM memory_source_dependencies d JOIN originals o ON o.id=d.child_source_id
)SELECT s.source_id,s.owner_id,s.scope_type,s.scope_id,s.status,s.head_revision,s.access_revision,r.content_hash,COALESCE(r.origin->>'sensitivity','private')
FROM originals o JOIN memory_sources s ON s.source_id=o.id JOIN memory_source_revisions r ON r.source_id=s.source_id AND r.revision=s.head_revision ORDER BY s.source_id LIMIT 2001 FOR SHARE OF s`, session)
	if err != nil {
		return err
	}
	type dependency struct {
		id               string
		revision, access int64
		hash             string
	}
	var deps []dependency
	for rows.Next() {
		var d dependency
		var own, typ, scope, status, sensitivity string
		if err = rows.Scan(&d.id, &own, &typ, &scope, &status, &d.revision, &d.access, &d.hash, &sensitivity); err != nil {
			rows.Close()
			return err
		}
		if own != owner || status != "active" || sensitivity == "secret" || !(typ == "session" && scope == session || typ == "user" && scope == owner || task != "" && typ == "task" && scope == task) {
			rows.Close()
			return catalog.ErrStale
		}
		deps = append(deps, d)
		if len(deps) > 2000 {
			rows.Close()
			return errSourceTooLarge
		}
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		return err
	}
	if len(deps) == 0 {
		return catalog.ErrStale
	}
	if _, err = tx.Exec(ctx, `INSERT INTO memory_summary_sources(source_id,session_key,source_revision,through_sequence)VALUES($1,$2,$3,$4) ON CONFLICT(source_id)DO UPDATE SET source_revision=EXCLUDED.source_revision,through_sequence=EXCLUDED.through_sequence WHERE memory_summary_sources.session_key=EXCLUDED.session_key`, id, session, revision, last); err != nil {
		return err
	}
	if _, err = tx.Exec(ctx, `DELETE FROM memory_summary_dependencies WHERE snapshot_source_id=$1`, id); err != nil {
		return err
	}
	for _, d := range deps {
		if _, err = tx.Exec(ctx, `INSERT INTO memory_summary_dependencies(snapshot_source_id,original_source_id,source_revision,access_revision,content_hash)VALUES($1,$2,$3,$4,$5)`, id, d.id, d.revision, d.access, d.hash); err != nil {
			return err
		}
	}
	return nil
}
