package catalog

import (
	"context"
	"encoding/json"
)

type FragmentView struct {
	ID      int64           `json:"chunk_id"`
	Key     string          `json:"fragment_key"`
	Ordinal int             `json:"ordinal"`
	Title   string          `json:"title"`
	Text    string          `json:"text"`
	Locator json.RawMessage `json:"locator"`
}
type Inspection struct {
	Source        Source            `json:"source"`
	Revisions     []json.RawMessage `json:"revisions"`
	Fragments     []FragmentView    `json:"fragments"`
	Assets        []json.RawMessage `json:"assets"`
	RevisionLimit int               `json:"revision_limit"`
}

// Inspect keeps original revision/citation data behind authenticated owner
// scope proof. Tombstones remain visible, with no deleted text or asset bytes.
func (s *Store) Inspect(ctx context.Context, id string) (Inspection, error) {
	own, e := owner(ctx)
	if e != nil {
		return Inspection{}, e
	}
	if e = s.CheckRestore(ctx); e != nil {
		return Inspection{}, e
	}
	tx, e := s.pool.Begin(ctx)
	if e != nil {
		return Inspection{}, e
	}
	defer tx.Rollback(ctx)
	var typ, scope string
	if e = tx.QueryRow(ctx, `SELECT scope_type,scope_id FROM memory_sources WHERE source_id=$1 AND owner_id=$2`, id, own).Scan(&typ, &scope); e != nil {
		return Inspection{}, ErrDenied
	}
	if e = checkOwnerScopeTx(ctx, tx, own, typ, scope); e != nil {
		return Inspection{}, e
	}
	src, e := getSourceTx(ctx, tx, id, false)
	if e != nil || src.OwnerID != own {
		return Inspection{}, ErrDenied
	}
	v := Inspection{Source: src, Revisions: []json.RawMessage{}, Fragments: []FragmentView{}, Assets: []json.RawMessage{}, RevisionLimit: 5000}
	// Owner inspection can review quarantined/deleted history, but never makes
	// it eligible for model retrieval or index publication.
	rows, e := tx.Query(ctx, `SELECT to_jsonb(r) FROM memory_source_revisions r WHERE source_id=$1 ORDER BY revision DESC LIMIT 5000`, id)
	if e != nil {
		return v, e
	}
	for rows.Next() {
		var b json.RawMessage
		if e = rows.Scan(&b); e != nil {
			rows.Close()
			return v, e
		}
		v.Revisions = append(v.Revisions, b)
	}
	e = rows.Err()
	rows.Close()
	if e != nil {
		return v, e
	}
	rows, e = tx.Query(ctx, `SELECT o.object_id,o.fragment_key,o.fragment_ordinal,c.title,c.content,o.locator FROM memory_object_sources o JOIN memory_chunks c ON c.id=o.object_id AND o.memory_type='chunk' WHERE o.source_id=$1 AND o.source_revision=$2 ORDER BY o.fragment_ordinal,o.object_id`, id, src.Revision)
	if e != nil {
		return v, e
	}
	for rows.Next() {
		var f FragmentView
		if e = rows.Scan(&f.ID, &f.Key, &f.Ordinal, &f.Title, &f.Text, &f.Locator); e != nil {
			rows.Close()
			return v, e
		}
		v.Fragments = append(v.Fragments, f)
	}
	e = rows.Err()
	rows.Close()
	if e != nil {
		return v, e
	}
	rows, e = tx.Query(ctx, `SELECT to_jsonb(a)-'original_bytes' FROM memory_source_assets a WHERE source_id=$1 ORDER BY source_revision DESC LIMIT 5000`, id)
	if e != nil {
		return v, e
	}
	for rows.Next() {
		var b json.RawMessage
		if e = rows.Scan(&b); e != nil {
			rows.Close()
			return v, e
		}
		v.Assets = append(v.Assets, b)
	}
	e = rows.Err()
	rows.Close()
	if e != nil {
		return v, e
	}
	return v, tx.Commit(ctx)
}
