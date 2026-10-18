package catalog

import (
	"context"
	"time"

	"github.com/butler/butler/internal/actionauth"
	"github.com/jackc/pgx/v5"
)

// CheckCapturedRunGrantsTx checks only metadata for an already captured run
// graph. It is not an access grant or a completion/ownership guard: its caller
// must first prove its own operation, exact task/constraints revisions and call
// CheckRunSourcesTx. It returns no body and never adds or repairs source pins.
// Final publication can therefore repeat current scope/grant proof without
// allowing generic AccessTx on a finalizing run.
func (s *Store) CheckCapturedRunGrantsTx(ctx context.Context, tx pgx.Tx, run string) error {
	var owner, session, taskID string
	var revision int64
	var delegated bool
	if err := tx.QueryRow(ctx, `SELECT s.user_id,r.session_key,COALESCE(r.task_id,''),COALESCE(r.task_revision,0) FROM runs r JOIN sessions s USING(session_key) WHERE r.run_id=$1 FOR SHARE OF r,s`, run).Scan(&owner, &session, &taskID, &revision); err != nil || owner == "" || session == "" {
		return ErrDenied
	}
	if taskID != "" {
		var current bool
		if err := tx.QueryRow(ctx, `SELECT owner_id=$2 AND session_key=$3 AND revision=$4 AND constraints_revision>0 AND status='active',EXISTS(SELECT 1 FROM task_delegations d WHERE d.task_id=t.task_id) FROM tasks t WHERE task_id=$1 FOR SHARE OF t`, taskID, owner, session, revision).Scan(&current, &delegated); err != nil || !current {
			return ErrDenied
		}
	}
	rows, err := tx.Query(ctx, `SELECT source_id,source_revision,access_revision,content_hash FROM memory_run_sources WHERE run_id=$1 ORDER BY source_id LIMIT 2001`, run)
	if err != nil {
		return err
	}
	refs := []SourceRef{}
	pins := map[string]SourceRef{}
	for rows.Next() {
		var p SourceRef
		if err = rows.Scan(&p.ID, &p.Revision, &p.AccessRevision, &p.Hash); err != nil {
			rows.Close()
			return err
		}
		refs, pins[p.ID] = append(refs, p), p
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		return err
	}
	if len(refs) > 2000 {
		return ErrDenied
	}
	graph, err := s.CanonicalSourcesTx(ctx, tx, refs)
	if err != nil {
		return err
	}
	for _, ref := range graph {
		pin, ok := pins[ref.ID]
		if !ok || pin.Revision != ref.Revision {
			return ErrDenied
		}
		var src Source
		var digest string
		err = tx.QueryRow(ctx, `SELECT s.source_id,s.owner_id,s.scope_type,s.scope_id,s.head_revision,s.access_revision,s.status,r.content_hash FROM memory_sources s JOIN memory_source_revisions r ON r.source_id=s.source_id AND r.revision=s.head_revision WHERE s.source_id=$1 FOR SHARE OF s`, ref.ID).Scan(&src.ID, &src.OwnerID, &src.ScopeType, &src.ScopeID, &src.Revision, &src.AccessRevision, &src.Status, &digest)
		if err != nil || src.OwnerID != owner || src.Status != "active" || src.Revision != pin.Revision || src.AccessRevision != pin.AccessRevision || digest != pin.Hash {
			return ErrDenied
		}
		if src.ScopeType == "session" && src.ScopeID == session || src.ScopeType == "task" && src.ScopeID == taskID || !delegated && src.ScopeType == "user" && src.ScopeID == owner {
			continue
		}
		if taskID == "" {
			return ErrDenied
		}
		if err = checkInheritedPermissionTx(ctx, tx, taskID, owner, src, "memory_read", 0); err != nil {
			return err
		}
	}
	// Grant and source locks can wait. Check expiring owner/delegation authority
	// on fresh database time only after those locks, without loading any body.
	if taskID != "" {
		var now time.Time
		if err = tx.QueryRow(ctx, `SELECT clock_timestamp()`).Scan(&now); err != nil {
			return err
		}
		if _, err = actionauth.CheckTaskAuthorityTx(ctx, tx, taskID, owner, nil, now); err != nil {
			return err
		}
	}
	return nil
}
