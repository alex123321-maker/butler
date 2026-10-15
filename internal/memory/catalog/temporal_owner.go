package catalog

import (
	"context"
	"errors"
	"time"

	"github.com/jackc/pgx/v5"
)

func validTemporalUpdate(p TemporalUpdate) bool {
	if !bounded(p.ClaimID, 1024) || p.ExpectedRevision < 0 || p.ExpectedHeadRevision < 0 {
		return false
	}
	if p.Validity != ValidityUnknown && p.Validity != ValidityInterval || p.Validity == ValidityUnknown && (p.ValidFrom != nil || p.ValidTo != nil) {
		return false
	}
	if p.ValidFrom != nil && p.ValidFrom.IsZero() || p.ValidTo != nil && p.ValidTo.IsZero() || p.ValidFrom != nil && p.ValidTo != nil && !p.ValidFrom.Before(*p.ValidTo) {
		return false
	}
	switch p.Class {
	case FreshnessUnknown, FreshnessStablePreference, FreshnessWorkStatus, FreshnessTechnicalFact, FreshnessCompletedEpisode:
		return true
	}
	return false
}

// SetTemporal appends an authenticated interpretation. It never changes the
// owner head, claim value, source revision or operational projection lifecycle.
func (s *Store) SetTemporal(ctx context.Context, p TemporalUpdate) (TemporalMetadata, error) {
	own, err := owner(ctx)
	if err != nil || !validTemporalUpdate(p) {
		if err != nil {
			return TemporalMetadata{}, err
		}
		return TemporalMetadata{}, ErrInvalid
	}
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return TemporalMetadata{}, err
	}
	defer tx.Rollback(ctx)
	c, err := getClaim(ctx, tx, p.ClaimID, false)
	if err != nil || c.OwnerID != own {
		return TemporalMetadata{}, ErrDenied
	}
	refs := []SourceRef{{ID: c.SourceID, Revision: c.SourceRevision}}
	if _, err = s.AccessTx(ctx, tx, refs, "memory_accept"); err != nil {
		return TemporalMetadata{}, err
	}
	var head int64
	if err = tx.QueryRow(ctx, `SELECT revision FROM memory_fact_heads WHERE owner_id=$1 AND scope_type=$2 AND scope_id=$3 AND fact_key=$4 FOR UPDATE`, own, c.ScopeType, c.ScopeID, c.Key).Scan(&head); errors.Is(err, pgx.ErrNoRows) {
		// Candidate metadata needs the same key lock as its later owner decision.
		if _, err = tx.Exec(ctx, `INSERT INTO memory_fact_heads(owner_id,scope_type,scope_id,fact_key) VALUES($1,$2,$3,$4) ON CONFLICT DO NOTHING`, own, c.ScopeType, c.ScopeID, c.Key); err != nil {
			return TemporalMetadata{}, err
		}
		err = tx.QueryRow(ctx, `SELECT revision FROM memory_fact_heads WHERE owner_id=$1 AND scope_type=$2 AND scope_id=$3 AND fact_key=$4 FOR UPDATE`, own, c.ScopeType, c.ScopeID, c.Key).Scan(&head)
	}
	if err != nil {
		return TemporalMetadata{}, err
	}
	if head != p.ExpectedHeadRevision {
		return TemporalMetadata{}, ErrStale
	}
	c, err = getClaim(ctx, tx, p.ClaimID, true)
	if err != nil || c.Status == "deleted" || c.Status == "rejected" {
		return TemporalMetadata{}, ErrStale
	}
	if _, err = tx.Exec(ctx, `INSERT INTO memory_claim_temporal_heads(claim_id) VALUES($1) ON CONFLICT DO NOTHING`, c.ID); err != nil {
		return TemporalMetadata{}, err
	}
	var revision int64
	if err = tx.QueryRow(ctx, `SELECT revision FROM memory_claim_temporal_heads WHERE claim_id=$1 FOR UPDATE`, c.ID).Scan(&revision); err != nil {
		return TemporalMetadata{}, err
	}
	if revision != p.ExpectedRevision {
		return TemporalMetadata{}, ErrStale
	}
	var now time.Time
	if err = tx.QueryRow(ctx, `SELECT clock_timestamp()`).Scan(&now); err != nil {
		return TemporalMetadata{}, err
	}
	var verified *time.Time
	basis := "unknown"
	if p.OwnerVerifiedNow {
		verified = &now
		basis = "owner_statement"
	}
	if _, err = tx.Exec(ctx, `INSERT INTO memory_claim_temporal_revisions(claim_id,revision,validity,valid_from,valid_to,freshness_class,verified_at,verification_basis,recorded_at,recorded_by) VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9,$10)`, c.ID, revision+1, p.Validity, p.ValidFrom, p.ValidTo, p.Class, verified, basis, now, own); err != nil {
		return TemporalMetadata{}, err
	}
	if _, err = tx.Exec(ctx, `UPDATE memory_claim_temporal_heads SET revision=$2 WHERE claim_id=$1`, c.ID, revision+1); err != nil {
		return TemporalMetadata{}, err
	}
	m, _, err := loadTemporalMetadataTx(ctx, tx, c, TemporalQuery{})
	if err != nil {
		return TemporalMetadata{}, err
	}
	if err = s.finalizeAccessTx(ctx, tx, refs, "memory_accept"); err != nil {
		return TemporalMetadata{}, err
	}
	return m, tx.Commit(ctx)
}
