package catalog

import (
	"context"
	"time"

	"github.com/jackc/pgx/v5"
)

// Delete records the content-free anchor before committing tombstones. A crash
// between them causes fail-closed restore reconciliation, never resurrection.
// PostgreSQL advisory serialization also protects the shared manifest against
// simultaneous orchestrator instances on the same authoritative database.
func (s *Store) Delete(ctx context.Context, sourceID, action string, expectedRevision int64) (Deletion, error) {
	if action != "delete" && action != "revoke" {
		return Deletion{}, ErrInvalid
	}
	own, e := owner(ctx)
	if e != nil {
		return Deletion{}, e
	}
	return s.deleteSource(ctx, sourceID, action, expectedRevision, own)
}

// DeleteExpiredMobileObservation applies only durable owner-configured device
// retention. It never manufactures an owner actor, grant or model authorization.
func (s *Store) DeleteExpiredMobileObservation(ctx context.Context, sourceID string, expectedRevision int64) (Deletion, error) {
	return s.deleteSource(ctx, sourceID, "archive_expire", expectedRevision, "")
}

func (s *Store) deleteSource(ctx context.Context, sourceID, action string, expectedRevision int64, own string) (Deletion, error) {
	if action != "delete" && action != "revoke" && action != "archive_expire" {
		return Deletion{}, ErrInvalid
	}
	if e := s.CheckRestore(ctx); e != nil {
		return Deletion{}, e
	}
	tx, e := s.pool.Begin(ctx)
	if e != nil {
		return Deletion{}, e
	}
	defer tx.Rollback(ctx)
	if _, e = tx.Exec(ctx, `SELECT pg_advisory_xact_lock(226046)`); e != nil {
		return Deletion{}, e
	}
	if own == "" {
		// Lock current device policy until commit; an ordinary source, foreign
		// source or notification whose retention was extended cannot be erased.
		if e = tx.QueryRow(ctx, `SELECT d.owner_id FROM mobile_events e JOIN mobile_devices d USING(device_id) WHERE e.source_id=$1 AND e.received_at<clock_timestamp()-make_interval(days=>d.retention_days) FOR SHARE OF d,e`, sourceID).Scan(&own); e != nil {
			return Deletion{}, ErrDenied
		}
	}
	var actual string
	var rev int64
	if tx.QueryRow(ctx, `SELECT owner_id,head_revision FROM memory_sources WHERE source_id=$1`, sourceID).Scan(&actual, &rev) != nil || actual != own {
		return Deletion{}, ErrDenied
	}
	if rev != expectedRevision {
		return Deletion{}, ErrStale
	}
	if action == "archive_expire" {
		var pending bool
		if e = tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM mobile_messages m JOIN memory_sources n ON n.source_id=m.result_source_id WHERE m.source_id=$1 AND m.state='remembered' AND n.status='active' AND NOT EXISTS(SELECT 1 FROM memory_retained_mobile_evidence e WHERE e.source_id=n.source_id))`, sourceID).Scan(&pending); e != nil {
			return Deletion{}, e
		}
		if pending {
			return Deletion{}, ErrDenied
		}
	}
	records, e := s.manifest.Load(ctx)
	if e != nil {
		return Deletion{}, e
	}
	id, e := randomID("deletion_")
	if e != nil {
		return Deletion{}, e
	}
	d := Deletion{ID: id, SourceID: sourceID, OwnerID: own, Action: action, Watermark: int64(len(records) + 1)}
	r := ManifestRecord{ID: id, SourceID: sourceID, OwnerHash: hash(own), Action: action, Sequence: d.Watermark, Time: time.Now().UTC()}
	if e = s.manifest.Append(ctx, r); e != nil {
		return Deletion{}, e
	}
	if e = s.applyDeletionTx(ctx, tx, r, own); e != nil {
		return Deletion{}, e
	}
	return d, tx.Commit(ctx)
}

// Reconcile is an operator-owned startup/restore action. No model or ordinary
// HTTP body can replace the anchor, lower its watermark, or opt out of replay.
func (s *Store) Reconcile(ctx context.Context) error {
	if s == nil || s.manifest == nil {
		return ErrRestore
	}
	records, e := s.manifest.Load(ctx)
	if e != nil {
		return e
	}
	tx, e := s.pool.Begin(ctx)
	if e != nil {
		return e
	}
	defer tx.Rollback(ctx)
	if _, e = tx.Exec(ctx, `SELECT pg_advisory_xact_lock(226046)`); e != nil {
		return e
	}
	var current int64
	if e = tx.QueryRow(ctx, `SELECT watermark FROM memory_restore_state WHERE singleton FOR UPDATE`).Scan(&current); e != nil {
		return e
	}
	if current > int64(len(records)) {
		return ErrRestore
	}
	for _, r := range records {
		if r.Sequence <= current {
			continue
		}
		var own string
		e = tx.QueryRow(ctx, `SELECT owner_id FROM memory_sources WHERE source_id=$1`, r.SourceID).Scan(&own)
		if e == pgx.ErrNoRows {
			own = ""
		} else if e != nil {
			return e
		}
		if own != "" && hash(own) != r.OwnerHash {
			return ErrDenied
		}
		if e = s.applyDeletionTx(ctx, tx, r, own); e != nil {
			return e
		}
	}
	return tx.Commit(ctx)
}
func (s *Store) applyDeletionTx(ctx context.Context, tx pgx.Tx, r ManifestRecord, own string) error {
	// Transcript append/finalization lock sessions/heads before publication source
	// rows. Take the same order before redacting messages or covered summaries.
	if _, e := tx.Exec(ctx, `SELECT session_key FROM sessions ORDER BY session_key FOR UPDATE`); e != nil {
		return e
	}
	if _, e := tx.Exec(ctx, `SELECT task_id FROM tasks WHERE owner_id=$1 ORDER BY task_id FOR UPDATE`, own); e != nil {
		return e
	}
	if _, e := tx.Exec(ctx, `SELECT session_key FROM transcript_heads ORDER BY session_key FOR UPDATE`); e != nil {
		return e
	}
	if _, e := tx.Exec(ctx, `INSERT INTO memory_deletions(deletion_id,source_id,owner_id,action,watermark) OVERRIDING SYSTEM VALUE VALUES($1,$2,$3,$4,$5) ON CONFLICT(deletion_id) DO NOTHING`, r.ID, r.SourceID, own, r.Action, r.Sequence); e != nil {
		return e
	}
	if _, e := tx.Exec(ctx, `SELECT set_config('butler.memory_redaction',$1,TRUE)`, r.ID); e != nil {
		return e
	}
	if _, e := tx.Exec(ctx, `DELETE FROM memory_working WHERE session_key IN(SELECT session_key FROM sessions WHERE user_id=$1)`, own); e != nil {
		return e
	}
	if _, e := tx.Exec(ctx, `UPDATE tasks SET working_json=jsonb_build_object('plan','[]'::jsonb,'pending_actions','[]'::jsonb,'scratch',jsonb_build_object('privacy_redacted',TRUE,'deletion_id',$2::TEXT)),working_revision=working_revision+1,updated_at=clock_timestamp() WHERE owner_id=$1`, own, r.ID); e != nil {
		return e
	}
	// Old extraction rows can lack original-message edges. Conservatively purge
	// this owner's quarantined legacy data rather than leave an untraceable copy.
	// Metadata-only witnesses are privacy ancestors without becoming readable
	// body parents. Follow those edges at every depth along with ordinary copied
	// source dependencies; witness deletion must physically erase its R copies.
	rows, e := tx.Query(ctx, `WITH RECURSIVE privacy_edges(parent_source_id,child_source_id) AS NOT MATERIALIZED(
 SELECT parent_source_id,child_source_id FROM memory_source_dependencies
 UNION ALL SELECT original_source_id,source_id FROM memory_experience_reconciled_attempts
 UNION ALL SELECT original_source_id,source_id FROM memory_retained_mobile_evidence WHERE $3<>'archive_expire'
),affected AS(
 SELECT source_id FROM memory_sources WHERE source_id=$1 OR source_id IN(SELECT snapshot_source_id FROM memory_summary_dependencies WHERE original_source_id=$1) OR source_id IN(SELECT x.source_id FROM memory_summary_sources x WHERE x.session_key IN(SELECT scope_id FROM memory_sources WHERE source_id=$1 AND scope_type='session')) OR (owner_id=$2 AND source_kind='legacy' AND status='quarantined')
 UNION SELECT d.child_source_id FROM privacy_edges d JOIN affected a ON a.source_id=d.parent_source_id
)SELECT s.source_id FROM memory_sources s JOIN affected a USING(source_id) ORDER BY s.source_id FOR UPDATE OF s`, r.SourceID, own, r.Action)
	if e != nil {
		return e
	}
	var ids []string
	for rows.Next() {
		var id string
		if e = rows.Scan(&id); e != nil {
			rows.Close()
			return e
		}
		ids = append(ids, id)
	}
	e = rows.Err()
	rows.Close()
	if e != nil {
		return e
	}
	status := "deleted"
	if r.Action == "revoke" {
		status = "revoked"
	}
	if e = redactMessagesTx(ctx, tx, ids, r.ID); e != nil {
		return e
	}
	if e = redactPinnedRunsTx(ctx, tx, ids, r.ID); e != nil {
		return e
	}
	if _, e = tx.Exec(ctx, `UPDATE memory_aux_jobs SET status='cancelled',claim_id='',claim_until=NULL,error_code='source_deleted',updated_at=clock_timestamp() WHERE owner_id=$1 AND status IN ('pending','running') AND EXISTS(SELECT 1 FROM jsonb_array_elements(sources) x WHERE x->>'source_id'=ANY($2))`, own, ids); e != nil {
		return e
	}
	for _, table := range []string{"memory_profile", "memory_episodes", "memory_chunks"} {
		if _, e = tx.Exec(ctx, `DELETE FROM `+table+` p WHERE NOT EXISTS(SELECT 1 FROM memory_object_sources o WHERE o.object_id=p.id AND o.memory_type=p.memory_type) AND ((p.scope_type='user' AND p.scope_id=$1) OR (p.scope_type='session' AND EXISTS(SELECT 1 FROM sessions s WHERE s.session_key=p.scope_id AND s.user_id=$1)))`, own); e != nil {
			return e
		}
	}
	for _, id := range ids {
		if _, e = tx.Exec(ctx, `DELETE FROM memory_retained_mobile_evidence WHERE source_id=$1`, id); e != nil {
			return e
		}
		// Purge vectors/chunks before source text. Retain only audit IDs and hashes.
		for _, pair := range []struct{ typ, table string }{{"profile", "memory_profile"}, {"episodic", "memory_episodes"}, {"chunk", "memory_chunks"}} {
			if _, e = tx.Exec(ctx, `DELETE FROM `+pair.table+` WHERE id IN(SELECT object_id FROM memory_object_sources WHERE memory_type=$1 AND source_id=$2)`, pair.typ, id); e != nil {
				return e
			}
		}
		if _, e = tx.Exec(ctx, `DELETE FROM memory_object_sources WHERE source_id=$1`, id); e != nil {
			return e
		}
		if _, e = tx.Exec(ctx, `UPDATE memory_claims SET status='deleted',value_json='{}',summary='',profile_id=NULL WHERE source_id=$1`, id); e != nil {
			return e
		}
		if _, e = tx.Exec(ctx, `UPDATE memory_fact_heads SET active_claim_id='',revision=revision+1 WHERE active_claim_id IN(SELECT claim_id FROM memory_claims WHERE source_id=$1)`, id); e != nil {
			return e
		}
		if _, e = tx.Exec(ctx, `UPDATE memory_source_revisions SET body='',title='',locator='{}',origin=jsonb_build_object('kind','memory_derived','sensitivity','private','source_refs',jsonb_build_array(source_id)),author='' WHERE source_id=$1`, id); e != nil {
			return e
		}
		if _, e = tx.Exec(ctx, `UPDATE memory_sources SET status=$2,access_revision=access_revision+1,updated_at=clock_timestamp() WHERE source_id=$1`, id, status); e != nil {
			return e
		}
		if _, e = tx.Exec(ctx, `DELETE FROM memory_scope_grants WHERE source_id=$1`, id); e != nil {
			return e
		}
	}
	// A legacy summary may lack fine-grained source refs. Conservative invalidation
	// is preferable to retaining any potentially deleted derived text.
	if _, e = tx.Exec(ctx, `UPDATE transcript_heads SET source_revision=source_revision+1; DELETE FROM session_summaries; UPDATE sessions SET summary=''`); e != nil {
		return e
	}
	_, e = tx.Exec(ctx, `UPDATE memory_restore_state SET watermark=$1 WHERE singleton`, r.Sequence)
	return e
}
