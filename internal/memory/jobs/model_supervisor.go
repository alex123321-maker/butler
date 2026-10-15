package jobs

import (
	"context"

	"github.com/jackc/pgx/v5"
)

// checkModelSupervisor observes already-admitted authority without rereading
// source bodies or taking the job's publication lock. The complete immutable
// manifest remains current. Full access/release/publication still use AccessTx.
// A storage timeout still cancels: the 200ms proof deadline is not relaxed.
func (s *Store) checkModelSupervisor(ctx context.Context, j Job, observePhase ...func(string)) error {
	if s == nil || s.catalog == nil {
		return ErrClaim
	}
	ctx = s.Context(ctx, j)
	phase := func(name string) {
		if len(observePhase) == 1 && observePhase[0] != nil {
			observePhase[0](name)
		}
	}
	phase("supervisor_begin")
	tx, err := s.pool.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)
	phase("supervisor_restore")
	if err = s.catalog.CheckRestoreTx(ctx, tx); err != nil {
		return err
	}
	phase("supervisor_policy")
	p, err := s.checkModelPolicyTx(ctx, tx, j)
	if err != nil {
		return err
	}
	if p.Privacy != "cloud_allowed" {
		return ErrClaim
	}
	phase("supervisor_claim")
	if _, err = (&claimChecker{s, j}).checkSnapshot(ctx, tx, false); err != nil {
		return err
	}
	// Copied summary inputs retain their dynamic original transcript graph.
	// The migration-072 privacy predicate is required, including after restore.
	ids := make([]string, len(j.Sources))
	for i, pin := range j.Sources {
		ids[i] = pin.ID
	}
	var current, busy bool
	phase("supervisor_sources")
	err = tx.QueryRow(ctx, `WITH pins AS MATERIALIZED (
 SELECT * FROM jsonb_to_recordset($1::jsonb) AS p(source_id text,revision bigint,access_revision bigint,content_hash text)
)
SELECT butler_memory_summary_sources_current($5) AND NOT EXISTS (
 SELECT 1 FROM pins p LEFT JOIN memory_sources s ON s.source_id=p.source_id
 LEFT JOIN memory_source_revisions r ON r.source_id=p.source_id AND r.revision=s.head_revision
 WHERE s.owner_id IS DISTINCT FROM $2 OR s.status IS DISTINCT FROM 'active'
 OR s.head_revision IS DISTINCT FROM p.revision OR s.access_revision IS DISTINCT FROM p.access_revision
 OR r.content_hash IS DISTINCT FROM p.content_hash
 OR r.origin->>'sensitivity'='secret'
 OR NOT CASE s.scope_type
  WHEN 'user' THEN s.scope_id=$2
  WHEN 'task' THEN $3<>'' AND s.scope_id=$3
  WHEN 'session' THEN EXISTS(SELECT 1 FROM runs WHERE run_id=$4 AND session_key=s.scope_id AND COALESCE(task_id,'')=$3)
   AND NOT EXISTS(SELECT 1 FROM tasks WHERE session_key=s.scope_id AND task_id<>$3)
  ELSE FALSE END
) AND NOT EXISTS (
 SELECT 1 FROM memory_source_dependencies d JOIN pins child ON child.source_id=d.child_source_id
 LEFT JOIN pins parent ON parent.source_id=d.parent_source_id
 WHERE parent.source_id IS NULL OR parent.revision<>d.parent_revision
), EXISTS(SELECT 1 FROM session_ownership WHERE expires_at>clock_timestamp())`, jsonBytes(j.Sources), j.OwnerID, j.TaskID, j.OriginalRunID, ids).Scan(&current, &busy)
	if err != nil {
		return err
	}
	if !current {
		return ErrClaim
	}
	if busy {
		return ErrWaiting
	}
	// Observation creates no durable state. Release its verification locks with
	// rollback, avoiding a synchronous WAL commit for tuple-lock records. Full
	// network/publication transactions keep their mandatory durable commits.
	phase("supervisor_rollback")
	return tx.Rollback(ctx)
}
