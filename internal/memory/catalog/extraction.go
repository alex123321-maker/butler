package catalog

import (
	"context"
	"encoding/json"
	"math"

	"github.com/jackc/pgx/v5"
)

// ProposeTx publishes neutral candidates under the caller's current owner or
// durable-job proof in the SAME transaction as its extraction checkpoint.
// Exact retries retain the original decision; changed retry bodies are rejected.
func (s *Store) ProposeTx(ctx context.Context, tx pgx.Tx, p ClaimRequest) (Claim, error) {
	if s == nil || tx == nil || !bounded(p.Key, 512) || !bounded(p.DedupKey, 128) || !json.Valid(p.Value) || len(p.Value) > 32*1024 || len(p.Summary) > 8192 || math.IsNaN(p.Confidence) || math.IsInf(p.Confidence, 0) || p.Confidence < 0 || p.Confidence > 1 {
		return Claim{}, ErrInvalid
	}
	if e := s.CheckRestoreTx(ctx, tx); e != nil {
		return Claim{}, e
	}
	sources, e := s.AccessTx(ctx, tx, []SourceRef{{ID: p.SourceID, Revision: p.SourceRevision}}, "memory_extract")
	if e != nil {
		return Claim{}, e
	}
	src := sources[0]
	sum := hash([]any{p.Key, json.RawMessage(p.Value), p.Summary, p.Confidence})
	id, e := randomID("claim_")
	if e != nil {
		return Claim{}, e
	}
	tag, e := tx.Exec(ctx, `INSERT INTO memory_extraction_dedup(owner_id,source_id,source_revision,dedup_key,request_hash,claim_id) VALUES($1,$2,$3,$4,$5,$6) ON CONFLICT DO NOTHING`, src.OwnerID, src.ID, src.Revision, p.DedupKey, sum, id)
	if e != nil {
		return Claim{}, e
	}
	var priorHash, priorID string
	if e = tx.QueryRow(ctx, `SELECT request_hash,claim_id FROM memory_extraction_dedup WHERE owner_id=$1 AND source_id=$2 AND source_revision=$3 AND dedup_key=$4 FOR UPDATE`, src.OwnerID, src.ID, src.Revision, p.DedupKey).Scan(&priorHash, &priorID); e != nil {
		return Claim{}, e
	}
	if priorHash != sum {
		return Claim{}, ErrStale
	}
	// INSERT/FOR UPDATE can wait past a run lease or independent job deadline.
	// Do not return even an existing candidate to an expired extraction worker.
	if e = s.finalizeAccessTx(ctx, tx, []SourceRef{{ID: p.SourceID, Revision: p.SourceRevision}}, "memory_extract"); e != nil {
		return Claim{}, e
	}
	if tag.RowsAffected() == 0 {
		return getClaim(ctx, tx, priorID, false)
	}
	c := Claim{ID: id, OwnerID: src.OwnerID, ScopeType: src.ScopeType, ScopeID: src.ScopeID, Key: p.Key, SourceID: src.ID, SourceRevision: src.Revision, Value: p.Value, Summary: p.Summary, Status: "candidate", Confidence: p.Confidence, EventTime: src.EventTime}
	_, e = tx.Exec(ctx, `INSERT INTO memory_claims(claim_id,owner_id,scope_type,scope_id,fact_key,source_id,source_revision,value_json,summary,extraction_confidence,event_time) VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11)`, c.ID, c.OwnerID, c.ScopeType, c.ScopeID, c.Key, c.SourceID, c.SourceRevision, c.Value, c.Summary, c.Confidence, c.EventTime)
	return c, e
}
