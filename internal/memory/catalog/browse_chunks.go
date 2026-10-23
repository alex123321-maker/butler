package catalog

import (
	"context"
	"time"

	"github.com/butler/butler/internal/memory/sanitize"
)

// ChunkPreviewRef is a server-derived exact fragment identity, never supplied by
// the public search request. Resolve it back to current durable source truth.
type ChunkPreviewRef struct {
	ID       int64
	Revision time.Time
}

func (s *Store) BrowseChunks(ctx context.Context, chunks []ChunkPreviewRef) ([]BrowseItem, error) {
	own, err := owner(ctx)
	if err != nil {
		return nil, err
	}
	if len(chunks) > 4 {
		return nil, ErrInvalid
	}
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback(ctx)
	refs := make([]SourceRef, len(chunks))
	dates := make([]time.Time, len(chunks))
	for i, chunk := range chunks {
		if chunk.ID < 1 || chunk.Revision.IsZero() {
			return nil, ErrInvalid
		}
		err = tx.QueryRow(ctx, `SELECT s.source_id,o.source_revision,s.created_at FROM memory_object_sources o JOIN memory_sources s USING(source_id) WHERE o.memory_type='chunk' AND o.object_id=$1 AND s.owner_id=$2 AND s.head_revision=o.source_revision`, chunk.ID, own).Scan(&refs[i].ID, &refs[i].Revision, &dates[i])
		if err != nil {
			return nil, ErrStale
		}
	}
	sources, err := s.AccessTx(ctx, tx, refs, "memory_read")
	if err != nil {
		return nil, err
	}
	byID := map[string]Source{}
	for _, src := range sources {
		byID[src.ID] = src
	}
	out := []BrowseItem{}
	seen := map[string]bool{}
	for i, chunk := range chunks {
		src, ok := byID[refs[i].ID]
		if !ok {
			return nil, ErrDenied
		}
		var body string
		err = tx.QueryRow(ctx, `SELECT c.content FROM memory_chunks c JOIN memory_object_sources o ON o.object_id=c.id AND o.memory_type='chunk' WHERE c.id=$1 AND c.updated_at=$2 AND o.source_id=$3 AND o.source_revision=$4 AND c.status='active' AND c.effective_status='active' AND NOT c.suppressed AND c.confirmation_state IN ('confirmed','auto_confirmed') AND (c.expires_at IS NULL OR c.expires_at>clock_timestamp()) FOR SHARE OF c`, chunk.ID, chunk.Revision, src.ID, src.Revision).Scan(&body)
		if err != nil {
			return nil, ErrStale
		}
		if sanitize.Text(body) != body {
			return nil, ErrDenied
		}
		if seen[src.ID] {
			continue
		}
		seen[src.ID] = true
		src.Body = body
		item := previewSource(src, dates[i])
		item.Match = "semantic"
		out = append(out, item)
	}
	if err = tx.Commit(ctx); err != nil {
		return nil, err
	}
	return out, nil
}
