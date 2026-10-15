package catalog

import (
	"context"
	"encoding/json"

	"github.com/jackc/pgx/v5"
)

// ReadBoundTemporalObservationsTx follows only explicit semantic pins on the
// exact selected derivatives. Ordinary privacy ancestry alone adds no evidence.
func (s *Store) ReadBoundTemporalObservationsTx(ctx context.Context, tx pgx.Tx, refs []SourceRef) (TemporalObservationPage, error) {
	ctx = temporalReadContext(ctx)
	out := TemporalObservationPage{Items: []TemporalObservation{}}
	if len(refs) > MaxTemporalFacts {
		return out, ErrInvalid
	}
	for _, ref := range refs {
		if !bounded(ref.ID, 1024) || ref.Revision < 1 || ref.AccessRevision < 1 || len(ref.Hash) != 64 {
			return out, ErrInvalid
		}
	}
	if _, err := s.AccessTx(ctx, tx, refs, "memory_read"); err != nil {
		return out, err
	}
	rows, err := tx.Query(ctx, `SELECT p.derived_source_id,p.claim_id,p.original_source_id,p.source_revision,p.access_revision,p.content_hash,p.valid_at,p.known_at FROM memory_temporal_source_pins p JOIN jsonb_to_recordset($1::jsonb) AS r(source_id TEXT) ON r.source_id=p.derived_source_id ORDER BY p.derived_source_id,p.claim_id LIMIT $2`, mustJSON(refs), MaxTemporalFacts+1)
	if err != nil {
		return out, err
	}
	type route struct {
		o TemporalObservation
		q TemporalQuery
	}
	routes := []route{}
	for rows.Next() {
		var r route
		if err = rows.Scan(&r.o.OnSourceID, &r.o.ClaimID, &r.o.Source.ID, &r.o.Source.Revision, &r.o.Source.AccessRevision, &r.o.Source.Hash, &r.q.ValidAt, &r.q.KnownAt); err != nil {
			rows.Close()
			return out, err
		}
		routes = append(routes, r)
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		return out, err
	}
	if len(routes) > MaxTemporalFacts {
		out.Truncated = true
		routes = routes[:MaxTemporalFacts]
	}
	for _, r := range routes {
		metadata, _, err := loadTemporalMetadataTx(ctx, tx, Claim{ID: r.o.ClaimID}, r.q)
		if err != nil {
			return out, err
		}
		r.o.Temporal = metadata
		r.o.Historical = r.q.ValidAt != nil || r.q.KnownAt != nil
		var raw []byte
		if err = tx.QueryRow(ctx, `SELECT butler_memory_temporal_freshness($1,$2)`, r.o.ClaimID, r.q.KnownAt).Scan(&raw); err != nil {
			return out, err
		}
		if err = json.Unmarshal(raw, &r.o.Freshness); err != nil {
			return out, err
		}
		out.Items = append(out.Items, r.o)
	}
	ids := make([]string, len(refs))
	for i, r := range refs {
		ids[i] = r.ID
	}
	if err = checkTemporalSourcesTx(ctx, tx, ids); err != nil {
		return TemporalObservationPage{}, err
	}
	if err = s.finalizeAccessTx(ctx, tx, refs, "memory_read"); err != nil {
		return TemporalObservationPage{}, err
	}
	return out, nil
}
