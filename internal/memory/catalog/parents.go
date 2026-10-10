package catalog

import (
	"context"
	"sort"

	"github.com/jackc/pgx/v5"
)

// CanonicalSourcesTx expands the authoritative dependency graph, never citation
// JSON. Callers lock authority first, then these sorted source rows. Conflicting
// parent revisions are stale evidence, not a last-write-wins choice.
func (s *Store) CanonicalSourcesTx(ctx context.Context, tx pgx.Tx, refs []SourceRef) ([]SourceRef, error) {
	byID := map[string]SourceRef{}
	var ids []string
	for _, r := range refs {
		if prior, ok := byID[r.ID]; ok && prior.Revision != 0 && r.Revision != 0 && prior.Revision != r.Revision {
			return nil, ErrStale
		}
		byID[r.ID] = r
		ids = append(ids, r.ID)
	}
	rows, e := tx.Query(ctx, `WITH RECURSIVE parents AS(SELECT parent_source_id,parent_revision FROM memory_source_dependencies WHERE child_source_id=ANY($1) UNION SELECT d.parent_source_id,d.parent_revision FROM memory_source_dependencies d JOIN parents p ON d.child_source_id=p.parent_source_id) SELECT parent_source_id,parent_revision FROM parents ORDER BY parent_source_id`, ids)
	if e != nil {
		return nil, e
	}
	for rows.Next() {
		var r SourceRef
		if e = rows.Scan(&r.ID, &r.Revision); e != nil {
			rows.Close()
			return nil, e
		}
		if prior, ok := byID[r.ID]; ok {
			if prior.Revision != 0 && prior.Revision != r.Revision {
				rows.Close()
				return nil, ErrStale
			}
			if prior.Revision == 0 {
				prior.Revision = r.Revision
				byID[r.ID] = prior
			}
		} else {
			byID[r.ID] = r
		}
		if len(byID) > 2000 {
			rows.Close()
			return nil, ErrInvalid
		}
	}
	e = rows.Err()
	rows.Close()
	if e != nil {
		return nil, e
	}
	ordered := make([]SourceRef, 0, len(byID))
	for _, r := range byID {
		ordered = append(ordered, r)
	}
	sort.Slice(ordered, func(i, j int) bool { return ordered[i].ID < ordered[j].ID })
	return ordered, nil
}
