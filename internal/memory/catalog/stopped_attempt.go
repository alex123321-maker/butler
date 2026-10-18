package catalog

import (
	"context"
	"sort"

	"github.com/butler/butler/internal/owneridentity"
)

// StoppedAttemptParents identifies original primary message and used-source
// dependencies without pretending a failed attempt has an assistant final.
// The owning input adapter proves the stopped identity separately. This method
// confers no execution permission and returns no message/source body.
func (s *Store) StoppedAttemptParents(ctx context.Context, run, input string) ([]SourceRef, error) {
	a, ok := owneridentity.FromContext(ctx)
	if !ok || a.OwnerID == "" || a.DeliveryID == "" || run == "" || input == "" {
		return nil, ErrDenied
	}
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback(ctx)
	if err = s.CheckRestoreTx(ctx, tx); err != nil {
		return nil, err
	}
	rows, err := tx.Query(ctx, `WITH owned AS(SELECT r.run_id FROM runs r JOIN accepted_inputs i ON i.run_id=r.run_id JOIN tasks t ON t.task_id=r.task_id
WHERE r.run_id=$1 AND i.input_id=$2 AND i.owner_id=$3 AND t.owner_id=$3 AND i.task_id=t.task_id AND i.session_key=r.session_key
AND EXISTS(SELECT 1 FROM messages m WHERE m.run_id=r.run_id AND m.role='user'))
SELECT source_id,source_revision,access_revision,content_hash FROM memory_run_sources JOIN owned USING(run_id)
UNION SELECT s.source_id,s.head_revision,s.access_revision,v.content_hash FROM messages m JOIN memory_sources s ON s.source_id='message:'||m.message_id JOIN memory_source_revisions v ON v.source_id=s.source_id AND v.revision=s.head_revision
JOIN owned ON owned.run_id=m.run_id
WHERE m.run_id=$1 ORDER BY source_id LIMIT 65`, run, input, a.OwnerID)
	if err != nil {
		return nil, err
	}
	refs := []SourceRef{}
	for rows.Next() {
		var r SourceRef
		if err = rows.Scan(&r.ID, &r.Revision, &r.AccessRevision, &r.Hash); err != nil {
			rows.Close()
			return nil, err
		}
		refs = append(refs, r)
	}
	err = rows.Err()
	rows.Close()
	if err != nil || len(refs) == 0 || len(refs) > 64 {
		return nil, ErrDenied
	}
	refs, err = s.CanonicalSourcesTx(ctx, tx, refs)
	if err != nil || len(refs) > 64 {
		return nil, ErrDenied
	}
	if _, err = s.AccessTx(ctx, tx, refs, "memory_job_create"); err != nil {
		return nil, err
	}
	sort.Slice(refs, func(i, j int) bool { return refs[i].ID < refs[j].ID })
	return refs, tx.Commit(ctx)
}
