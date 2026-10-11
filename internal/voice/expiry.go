package voice

import (
	"context"
	"github.com/butler/butler/internal/memory/catalog"
	"github.com/jackc/pgx/v5"
	"time"
)

// The caller has already locked and validated the complete catalog graph.
// Apply the voice lifetime to every audio ancestor of presented text.
func sourceVoiceDeadlineTx(ctx context.Context, tx pgx.Tx, source catalog.SourceRef) (*time.Time, error) {
	rows, e := tx.Query(ctx, `WITH RECURSIVE parents(id) AS (SELECT $1::text UNION SELECT d.parent_source_id FROM memory_source_dependencies d JOIN parents p ON p.id=d.child_source_id) SELECT c.expires_at,c.expires_at>clock_timestamp() AND c.state NOT IN ('deleted','cancelled') FROM voice_clips c JOIN parents p ON p.id=c.source_ref->>'source_id' ORDER BY c.clip_id FOR SHARE OF c`, source.ID)
	if e != nil {
		return nil, e
	}
	defer rows.Close()
	var earliest *time.Time
	for rows.Next() {
		var until time.Time
		var current bool
		if e = rows.Scan(&until, &current); e != nil {
			return nil, e
		}
		if !current {
			return nil, ErrDenied
		}
		if earliest == nil || until.Before(*earliest) {
			v := until
			earliest = &v
		}
	}
	return earliest, rows.Err()
}

func (s *Store) presentationDeadline(ctx context.Context, source catalog.SourceRef) (*time.Time, error) {
	tx, e := s.Catalog.Pool().Begin(ctx)
	if e != nil {
		return nil, e
	}
	defer tx.Rollback(ctx)
	if _, e = s.Catalog.AccessTx(ctx, tx, []catalog.SourceRef{source}, "voice_tts"); e != nil {
		return nil, e
	}
	until, e := sourceVoiceDeadlineTx(ctx, tx, source)
	if e != nil {
		return nil, e
	}
	return until, tx.Commit(ctx)
}
