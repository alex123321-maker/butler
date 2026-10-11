package catalog

import (
	"context"
	"sort"

	"github.com/butler/butler/internal/security/content"
)

// VerifiedResultParents is a content-free reader for the orchestrator's typed
// task verifier. The caller independently verifies owner/result/verdict and
// passes the actual immutable final message/run IDs from the result repository.
// This identifies evidence; it grants no publication or memory-read authority.
func (s *Store) VerifiedResultParents(ctx context.Context, owner, messageID, runID string) ([]SourceRef, error) {
	if owner == "" || messageID == "" || runID == "" {
		return nil, ErrInvalid
	}
	tx, e := s.pool.Begin(ctx)
	if e != nil {
		return nil, e
	}
	defer tx.Rollback(ctx)
	if e = s.CheckRestoreTx(ctx, tx); e != nil {
		return nil, e
	}
	// Unlocked identity routing is rechecked after the complete sorted graph is
	// known; do not acquire a result-root lock before an earlier parent lock.
	var root SourceRef
	if e = tx.QueryRow(ctx, `SELECT s.source_id,s.head_revision,s.access_revision,r.content_hash FROM memory_sources s JOIN memory_source_revisions r ON r.source_id=s.source_id AND r.revision=s.head_revision JOIN messages m ON m.message_id=$3 AND m.run_id=$4 WHERE s.source_id=$1 AND s.owner_id=$2 AND s.status='active' AND m.role='assistant'`, "message:"+messageID, owner, messageID, runID).Scan(&root.ID, &root.Revision, &root.AccessRevision, &root.Hash); e != nil {
		return nil, ErrDenied
	}
	refs, e := s.CanonicalSourcesTx(ctx, tx, []SourceRef{root})
	if e != nil {
		return nil, e
	}
	byID := map[string]SourceRef{}
	for _, r := range refs {
		byID[r.ID] = r
	}
	rows, e := tx.Query(ctx, `SELECT source_id,source_revision,access_revision,content_hash FROM memory_run_sources WHERE run_id=$1 ORDER BY source_id`, runID)
	if e != nil {
		return nil, e
	}
	for rows.Next() {
		var r SourceRef
		if e = rows.Scan(&r.ID, &r.Revision, &r.AccessRevision, &r.Hash); e != nil {
			rows.Close()
			return nil, e
		}
		p, ok := byID[r.ID]
		if !ok || p.Revision != r.Revision {
			rows.Close()
			return nil, ErrStale
		}
		byID[r.ID] = r
	}
	e = rows.Err()
	rows.Close()
	if e != nil {
		return nil, e
	}
	// Dependencies are expanded by the catalog rather than by citation strings.
	// The final-message registration trigger links every original run pin. A
	// missing edge is not silently replaced with a weaker citation dependency.
	for i, r := range refs {
		refs[i] = byID[r.ID]
	}
	sort.Slice(refs, func(i, j int) bool { return refs[i].ID < refs[j].ID })
	for i, r := range refs {
		src, e := getSourceTx(ctx, tx, r.ID, false)
		if e != nil || src.OwnerID != owner || src.Status != "active" || src.Origin.Sensitivity == content.Secret || r.Revision != 0 && r.Revision != src.Revision || r.AccessRevision != 0 && r.AccessRevision != src.AccessRevision || r.Hash != "" && r.Hash != src.Hash {
			return nil, ErrStale
		}
		refs[i] = SourceRef{src.ID, src.Revision, src.AccessRevision, src.Hash}
	}
	// Returning the root retains all transitive edges without flattening their
	// scopes or leaking content. Publication still requires fresh AccessTx.
	return []SourceRef{root}, tx.Commit(ctx)
}
