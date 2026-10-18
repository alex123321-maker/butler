package catalog

import (
	"context"
	"time"

	"github.com/butler/butler/internal/ownership"
	"github.com/jackc/pgx/v5"
)

// Ordinary context observations are immutable per run. A later read cannot
// silently refresh an already prepared run after a decision or freshness change.
func bindTemporalRunTx(ctx context.Context, tx pgx.Tx, facts []AcceptedFact, q TemporalQuery) error {
	token, ok := ownership.TokenFromContext(ctx)
	if !ok || len(facts) == 0 {
		return nil
	}
	batch, err := tx.Begin(ctx)
	if err != nil {
		return err
	}
	defer batch.Rollback(ctx)
	instants := []string{"", ""}
	for i, t := range []*time.Time{q.ValidAt, q.KnownAt} {
		if t != nil {
			instants[i] = t.UTC().Format(time.RFC3339Nano)
		}
	}
	queryKey := hash(instants)
	for _, f := range facts {
		p := f.Pin
		_, err = batch.Exec(ctx, `INSERT INTO memory_temporal_run_pins(run_id,claim_id,head_revision,active_id,temporal_revision,original_source_id,source_revision,access_revision,content_hash,metadata_hash,freshness_status,current_usable,valid_at,known_at,query_key) VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14,$15) ON CONFLICT DO NOTHING`, token.RunID, p.ClaimID, p.HeadRevision, p.ActiveID, p.TemporalRevision, p.Source.ID, p.Source.Revision, p.Source.AccessRevision, p.Source.Hash, p.MetadataHash, p.FreshnessStatus, p.CurrentUsable, q.ValidAt, q.KnownAt, queryKey)
		if err != nil {
			return err
		}
		var same bool
		err = batch.QueryRow(ctx, `SELECT head_revision=$3 AND active_id=$4 AND temporal_revision=$5 AND original_source_id=$6 AND source_revision=$7 AND access_revision=$8 AND content_hash=$9 AND metadata_hash=$10 AND freshness_status=$11 AND current_usable=$12 AND valid_at IS NOT DISTINCT FROM $13::timestamptz AND known_at IS NOT DISTINCT FROM $14::timestamptz FROM memory_temporal_run_pins WHERE run_id=$1 AND claim_id=$2 AND query_key=$15`, token.RunID, p.ClaimID, p.HeadRevision, p.ActiveID, p.TemporalRevision, p.Source.ID, p.Source.Revision, p.Source.AccessRevision, p.Source.Hash, p.MetadataHash, p.FreshnessStatus, p.CurrentUsable, q.ValidAt, q.KnownAt, queryKey).Scan(&same)
		if err != nil {
			return err
		}
		if !same {
			return ErrStale
		}
	}
	var current bool
	err = batch.QueryRow(ctx, `SELECT count(*)<=$2 AND butler_memory_temporal_run_current($1) FROM memory_temporal_run_pins WHERE run_id=$1`, token.RunID, MaxTemporalFacts).Scan(&current)
	if err != nil {
		return err
	}
	if !current {
		return ErrStale
	}
	return batch.Commit(ctx)
}

func checkTemporalRunTx(ctx context.Context, tx pgx.Tx, runID string) error {
	_, err := tx.Exec(ctx, `SELECT h.owner_id FROM memory_fact_heads h WHERE EXISTS(SELECT 1 FROM memory_claims c JOIN memory_temporal_run_pins p ON p.claim_id=c.claim_id WHERE p.run_id=$1 AND c.owner_id=h.owner_id AND c.scope_type=h.scope_type AND c.scope_id=h.scope_id AND c.fact_key=h.fact_key) ORDER BY h.owner_id,h.scope_type,h.scope_id,h.fact_key FOR SHARE OF h`, runID)
	if err != nil {
		return err
	}
	var current bool
	if err = tx.QueryRow(ctx, `SELECT butler_memory_temporal_run_current($1)`, runID).Scan(&current); err != nil {
		return err
	}
	if !current {
		return ErrStale
	}
	return nil
}
