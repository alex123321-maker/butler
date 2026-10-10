package catalog

import (
	"context"
	"encoding/json"
)

type FragmentResult struct {
	Inserted  int `json:"inserted"`
	Updated   int `json:"updated"`
	Unchanged int `json:"unchanged"`
	Deleted   int `json:"deleted"`
}

// PublishFragments is a current-source replacement. Stable semantic keys retain
// chunk IDs for unchanged text. Removed fragments physically delete vectors.
func (s *Store) PublishFragments(ctx context.Context, ref SourceRef, fragments []Fragment) (FragmentResult, error) {
	var result FragmentResult
	if len(fragments) > 2000 {
		return result, ErrInvalid
	}
	seen := map[string]bool{}
	for _, f := range fragments {
		if !bounded(f.Key, 512) || seen[f.Key] || len(f.Text) > 32*1024 || !json.Valid(f.Locator) {
			return result, ErrInvalid
		}
		seen[f.Key] = true
	}
	if e := s.CheckRestore(ctx); e != nil {
		return result, e
	}
	tx, e := s.pool.Begin(ctx)
	if e != nil {
		return result, e
	}
	defer tx.Rollback(ctx)
	sources, e := s.AccessTx(context.WithValue(ctx, writeSourcesKey{}, true), tx, []SourceRef{ref}, "memory_ingest")
	if e != nil {
		return result, e
	}
	src := sources[0]
	rows, e := tx.Query(ctx, `SELECT object_id,fragment_key,fragment_hash FROM memory_object_sources WHERE source_id=$1 AND memory_type='chunk' ORDER BY object_id`, src.ID)
	if e != nil {
		return result, e
	}
	type old struct {
		id  int64
		sum string
	}
	existing := map[string]old{}
	for rows.Next() {
		var id int64
		var key, sum string
		if e = rows.Scan(&id, &key, &sum); e != nil {
			rows.Close()
			return result, e
		}
		existing[key] = old{id, sum}
	}
	e = rows.Err()
	rows.Close()
	if e != nil {
		return result, e
	}
	for ordinal, f := range fragments {
		sum := hash([]string{f.Title, f.Text})
		old, exists := existing[f.Key]
		var id int64
		origin := mustJSON(map[string]any{"kind": "document", "sensitivity": src.Origin.Sensitivity, "source_refs": []string{src.ID}, "parent_refs": append(src.Origin.SourceRefs, src.Origin.ParentRefs...), "locator": f.Locator, "catalog": map[string]any{"source_id": src.ID, "revision": src.Revision, "access_revision": src.AccessRevision, "fragment_key": f.Key}})
		if exists {
			id = old.id
			if sum == old.sum {
				result.Unchanged++
			} else {
				result.Updated++
			}
			if _, e = tx.Exec(ctx, `UPDATE memory_chunks SET title=$2,content=$3,summary=$2,embedding=CASE WHEN content<>$3 OR title<>$2 THEN NULL ELSE embedding END,provenance=$4,updated_at=clock_timestamp() WHERE id=$1`, id, f.Title, f.Text, origin); e != nil {
				return result, e
			}
		} else {
			result.Inserted++
			if e = tx.QueryRow(ctx, `INSERT INTO memory_chunks(memory_type,scope_type,scope_id,title,content,summary,source_type,source_id,provenance,confirmation_state) VALUES('chunk',$1,$2,$3,$4,$3,'catalog',$5,$6,'confirmed') RETURNING id`, src.ScopeType, src.ScopeID, f.Title, f.Text, src.ID, origin).Scan(&id); e != nil {
				return result, e
			}
		}
		if _, e = tx.Exec(ctx, `UPDATE memory_object_sources SET source_revision=$3,fragment_key=$4,fragment_hash=$5,locator=$6,fragment_ordinal=$7 WHERE memory_type='chunk' AND object_id=$1 AND source_id=$2`, id, src.ID, src.Revision, f.Key, sum, f.Locator, ordinal); e != nil {
			return result, e
		}
		delete(existing, f.Key)
	}
	for _, old := range existing {
		if _, e = tx.Exec(ctx, `DELETE FROM memory_chunks WHERE id=$1`, old.id); e != nil {
			return result, e
		}
		if _, e = tx.Exec(ctx, `DELETE FROM memory_object_sources WHERE memory_type='chunk' AND object_id=$1`, old.id); e != nil {
			return result, e
		}
		result.Deleted++
	}
	return result, tx.Commit(ctx)
}

type Citation struct {
	ChunkID   int64      `json:"chunk_id"`
	Fragment  Fragment   `json:"fragment"`
	Source    Source     `json:"source"`
	Neighbors []Fragment `json:"neighbors"`
}

func (s *Store) Citation(ctx context.Context, chunkID int64) (Citation, error) {
	var c Citation
	c.ChunkID = chunkID
	if e := s.CheckRestore(ctx); e != nil {
		return c, e
	}
	tx, e := s.pool.Begin(ctx)
	if e != nil {
		return c, e
	}
	defer tx.Rollback(ctx)
	if e = s.CheckObjectsTx(ctx, tx, []ObjectRef{{"chunk", chunkID}}, "memory_read"); e != nil {
		return c, e
	}
	var sid string
	if e = tx.QueryRow(ctx, `SELECT o.source_id,o.fragment_key,p.title,p.content,o.locator FROM memory_object_sources o JOIN memory_chunks p ON p.id=o.object_id WHERE o.memory_type='chunk' AND o.object_id=$1`, chunkID).Scan(&sid, &c.Fragment.Key, &c.Fragment.Title, &c.Fragment.Text, &c.Fragment.Locator); e != nil {
		return c, e
	}
	c.Source, e = getSourceTx(ctx, tx, sid, false)
	if e != nil {
		return c, e
	}
	rows, e := tx.Query(ctx, `SELECT o.fragment_key,p.title,p.content,o.locator FROM memory_object_sources o JOIN memory_chunks p ON p.id=o.object_id WHERE o.memory_type='chunk' AND o.source_id=$1 AND o.object_id<>$2 ORDER BY abs(o.fragment_ordinal-(SELECT fragment_ordinal FROM memory_object_sources WHERE memory_type='chunk' AND object_id=$2)),o.fragment_ordinal LIMIT 2`, sid, chunkID)
	if e != nil {
		return c, e
	}
	for rows.Next() {
		var f Fragment
		if e = rows.Scan(&f.Key, &f.Title, &f.Text, &f.Locator); e != nil {
			rows.Close()
			return c, e
		}
		c.Neighbors = append(c.Neighbors, f)
	}
	e = rows.Err()
	rows.Close()
	if e != nil {
		return c, e
	}
	return c, tx.Commit(ctx)
}
