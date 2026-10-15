package catalog

import (
	"context"

	"github.com/jackc/pgx/v5"
)

// ReadAcceptedFacts preserves the same current source authority as ordinary
// reads. Supplying an observation date never recovers deleted or ungranted data.
func (s *Store) ReadAcceptedFacts(ctx context.Context, ids []string, q TemporalQuery) (AcceptedFactPage, error) {
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return AcceptedFactPage{}, err
	}
	defer tx.Rollback(ctx)
	page, err := s.ReadAcceptedFactsTx(ctx, tx, ids, q)
	if err != nil {
		return AcceptedFactPage{}, err
	}
	return page, tx.Commit(ctx)
}

// ReadAcceptedFactsForSourcesTx enriches only explicitly requested source
// revisions. Privacy ancestors are checked but never added as answer evidence.
func (s *Store) ReadAcceptedFactsForSourcesTx(ctx context.Context, tx pgx.Tx, refs []SourceRef, q TemporalQuery) (AcceptedFactPage, error) {
	ctx = temporalReadContext(ctx)
	if len(refs) > MaxTemporalFacts {
		return AcceptedFactPage{}, ErrInvalid
	}
	for _, ref := range refs {
		if !bounded(ref.ID, 1024) || ref.Revision < 1 || ref.AccessRevision < 1 || len(ref.Hash) != 64 {
			return AcceptedFactPage{}, ErrInvalid
		}
	}
	if _, err := s.AccessTx(ctx, tx, refs, "memory_read"); err != nil {
		return AcceptedFactPage{}, err
	}
	rows, err := tx.Query(ctx, `SELECT c.claim_id FROM memory_claims c JOIN jsonb_to_recordset($1::jsonb) AS r(source_id text,revision bigint) ON r.source_id=c.source_id AND r.revision=c.source_revision WHERE c.status IN ('confirmed','superseded') ORDER BY c.claim_id LIMIT $2`, mustJSON(refs), MaxTemporalFacts+1)
	if err != nil {
		return AcceptedFactPage{}, err
	}
	ids := []string{}
	for rows.Next() {
		var id string
		if err = rows.Scan(&id); err != nil {
			rows.Close()
			return AcceptedFactPage{}, err
		}
		ids = append(ids, id)
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		return AcceptedFactPage{}, err
	}
	if len(ids) > MaxTemporalFacts {
		return AcceptedFactPage{Items: []AcceptedFact{}, Truncated: true}, nil
	}
	return s.ReadAcceptedFactsTx(ctx, tx, ids, q)
}

// ObjectAcceptedFact returns the accepted interpretation behind one operational
// projection. Legacy rows remain unverified data and return no accepted fact.
func (s *Store) ObjectAcceptedFact(ctx context.Context, object ObjectRef) (*AcceptedFact, error) {
	if object.ID < 1 || object.Type != "profile" && object.Type != "episodic" {
		return nil, ErrInvalid
	}
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback(ctx)
	column := "profile_id"
	if object.Type == "episodic" {
		column = "episode_id"
	}
	var id string
	err = tx.QueryRow(ctx, `SELECT claim_id FROM memory_claims WHERE `+column+`=$1 AND status='confirmed'`, object.ID).Scan(&id)
	if err == pgx.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	page, err := s.ReadAcceptedFactsTx(ctx, tx, []string{id}, TemporalQuery{})
	if err != nil {
		return nil, err
	}
	if len(page.Items) != 1 {
		return nil, ErrStale
	}
	return &page.Items[0], tx.Commit(ctx)
}
