package catalog

import (
	"context"
	"encoding/json"

	"github.com/jackc/pgx/v5"
)

// CreateStagedDerivedTx retains the original complete proof and constructor
// receipt. Its fresh row is invisible outside this transaction until quarantine
// and the owning result checkpoint commit together.
func (s *Store) CreateStagedDerivedTx(ctx context.Context, tx pgx.Tx, parents []SourceRef, purpose, title, body string, locator json.RawMessage) (Source, error) {
	src, err := s.CreateDerivedTx(ctx, tx, parents, purpose, title, body, locator)
	if err != nil {
		return Source{}, err
	}
	tag, err := tx.Exec(ctx, `UPDATE memory_sources SET status='quarantined' WHERE source_id=$1 AND head_revision=1 AND access_revision=1 AND status='active'`, src.ID)
	if err != nil {
		return Source{}, err
	}
	if tag.RowsAffected() != 1 {
		return Source{}, ErrStale
	}
	src.Status = "quarantined"
	return src, nil
}
