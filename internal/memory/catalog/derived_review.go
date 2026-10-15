package catalog

import (
	"context"
	"sort"

	"github.com/butler/butler/internal/security/content"
	"github.com/jackc/pgx/v5"
)

// ReviewDerivedTx is an authenticated owner review, not ordinary model access.
// It may expose one quarantined immutable DATA derivative only while its entire
// original graph and temporal pins are current. Other derived IDs are metadata
// locks only, allowing a caller's CAS to retire an old stale choice atomically.
// The caller owns its view head; this method neither accepts a fact nor changes it.
func (s *Store) ReviewDerivedTx(ctx context.Context, tx pgx.Tx, ref SourceRef, otherDerivedIDs ...string) (Source, error) {
	own, err := owner(ctx)
	if err != nil {
		return Source{}, err
	}
	if tx == nil || ref.ID == "" || ref.Revision < 1 || len(ref.Hash) != 64 || len(otherDerivedIDs) > 1 {
		return Source{}, ErrInvalid
	}
	if err = s.CheckRestoreTx(ctx, tx); err != nil {
		return Source{}, err
	}
	graph, err := s.CanonicalSourcesTx(ctx, tx, []SourceRef{ref})
	if err != nil || len(graph) < 2 || len(graph) > 64 {
		return Source{}, ErrStale
	}
	ids := make([]string, 0, len(graph)+len(otherDerivedIDs))
	proofIDs := make([]string, 0, len(graph))
	seen := map[string]bool{}
	for _, r := range graph {
		ids = append(ids, r.ID)
		proofIDs = append(proofIDs, r.ID)
		seen[r.ID] = true
	}
	for _, id := range otherDerivedIDs {
		if id != "" && !seen[id] {
			ids = append(ids, id)
			seen[id] = true
		}
	}
	sort.Strings(ids)
	rows, err := tx.Query(ctx, `SELECT source_id FROM memory_sources WHERE source_id=ANY($1) ORDER BY source_id FOR UPDATE`, ids)
	if err != nil {
		return Source{}, err
	}
	count := 0
	for rows.Next() {
		var id string
		if err = rows.Scan(&id); err != nil {
			rows.Close()
			return Source{}, err
		}
		count++
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		return Source{}, err
	}
	if count != len(ids) {
		return Source{}, ErrDenied
	}
	for _, id := range otherDerivedIDs {
		if id == "" {
			continue
		}
		var kind, actual string
		if err = tx.QueryRow(ctx, `SELECT owner_id,source_kind FROM memory_sources WHERE source_id=$1`, id).Scan(&actual, &kind); err != nil {
			return Source{}, err
		}
		if actual != own || kind != "model_candidate" {
			return Source{}, ErrDenied
		}
	}
	parents := make([]SourceRef, 0, len(graph)-1)
	for _, r := range graph {
		if r.ID != ref.ID {
			parents = append(parents, r)
		}
	}
	if _, err = s.AccessTx(ctx, tx, parents, "memory_read"); err != nil {
		return Source{}, err
	}
	root, err := getSourceTx(ctx, tx, ref.ID, false)
	if err != nil {
		return Source{}, err
	}
	if root.OwnerID != own || root.Kind != "model_candidate" || root.Status != "active" && root.Status != "quarantined" || root.Origin.Sensitivity == content.Secret {
		return Source{}, ErrDenied
	}
	if root.Revision != ref.Revision || root.Hash != ref.Hash || ref.AccessRevision != 0 && root.AccessRevision != ref.AccessRevision {
		return Source{}, ErrStale
	}
	if err = checkSummarySourcesTx(ctx, tx, proofIDs); err != nil {
		return Source{}, err
	}
	if err = checkTemporalSourcesTx(ctx, tx, proofIDs); err != nil {
		return Source{}, err
	}
	return root, nil
}

// SetDerivedViewStateTx changes presentation eligibility only. Active selection
// requires current complete evidence; quarantine can retire an obsolete result
// without re-reading stale text. Tombstones are never changed or resurrected.
func (s *Store) SetDerivedViewStateTx(ctx context.Context, tx pgx.Tx, ref SourceRef, state string) (SourceRef, error) {
	own, err := owner(ctx)
	if err != nil {
		return SourceRef{}, err
	}
	if tx == nil || (state != "active" && state != "quarantined") || ref.Revision < 1 || len(ref.Hash) != 64 {
		return SourceRef{}, ErrInvalid
	}
	var src Source
	if state == "active" {
		src, err = s.ReviewDerivedTx(ctx, tx, ref)
	} else {
		if err = s.CheckRestoreTx(ctx, tx); err != nil {
			return SourceRef{}, err
		}
		src, err = getSourceTx(ctx, tx, ref.ID, true)
	}
	if err != nil {
		return SourceRef{}, err
	}
	if src.OwnerID != own || src.Kind != "model_candidate" || src.Revision != ref.Revision || src.Hash != ref.Hash {
		return SourceRef{}, ErrStale
	}
	if src.Status == "deleted" || src.Status == "revoked" {
		if state == "active" {
			return SourceRef{}, ErrDeleted
		}
		return SourceRef{src.ID, src.Revision, src.AccessRevision, src.Hash}, nil
	}
	if src.Status != state {
		if err = tx.QueryRow(ctx, `UPDATE memory_sources SET status=$2,access_revision=access_revision+1,updated_at=clock_timestamp() WHERE source_id=$1 RETURNING access_revision`, src.ID, state).Scan(&src.AccessRevision); err != nil {
			return SourceRef{}, err
		}
	}
	return SourceRef{src.ID, src.Revision, src.AccessRevision, src.Hash}, nil
}
