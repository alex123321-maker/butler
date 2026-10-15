package catalog

import (
	"context"
	"encoding/json"

	"github.com/butler/butler/internal/memory/sanitize"
	"github.com/jackc/pgx/v5"
)

// RefreshOriginalTx is a trusted ingest adapter for an already authorized,
// unparented source. It retains identity, scope, access and origin; it cannot
// impersonate an owner or replace a derivative's original-source graph.
func (s *Store) RefreshOriginalTx(ctx context.Context, tx pgx.Tx, ref SourceRef, purpose, body string, locator json.RawMessage) (Source, error) {
	if tx == nil || len(body) > 1024*1024 || sanitize.Text(body) != body || !json.Valid(locator) {
		return Source{}, ErrInvalid
	}
	sources, err := s.AccessTx(ctx, tx, []SourceRef{ref}, purpose)
	if err != nil || len(sources) != 1 {
		return Source{}, ErrDenied
	}
	src := sources[0]
	var parents bool
	if err = tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM memory_source_dependencies WHERE child_source_id=$1)`, src.ID).Scan(&parents); err != nil {
		return Source{}, err
	}
	if parents || src.Kind != "document" {
		return Source{}, ErrInvalid
	}
	if src.Body == body && string(src.Locator) == string(locator) {
		return src, nil
	}
	sum := hash([]any{src.Title, body, locator, src.Author, src.EventTime, src.Origin})
	if _, err = tx.Exec(ctx, `INSERT INTO memory_source_revisions(source_id,revision,content_hash,title,body,locator,origin,author,event_time) VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9)`, src.ID, src.Revision+1, sum, src.Title, body, locator, mustJSON(src.Origin), src.Author, src.EventTime); err != nil {
		return Source{}, err
	}
	tag, err := tx.Exec(ctx, `UPDATE memory_sources SET head_revision=$2,updated_at=clock_timestamp() WHERE source_id=$1 AND head_revision=$3 AND access_revision=$4 AND status='active'`, src.ID, src.Revision+1, src.Revision, src.AccessRevision)
	if err != nil {
		return Source{}, err
	}
	if tag.RowsAffected() != 1 {
		return Source{}, ErrStale
	}
	return getSourceTx(ctx, tx, src.ID, false)
}
