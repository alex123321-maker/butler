package app

import (
	"context"
	"time"

	"github.com/butler/butler/internal/memory/catalog"
	"github.com/butler/butler/internal/memory/readcontrol"
	"github.com/jackc/pgx/v5"
)

func temporalInstant(t *time.Time) *string {
	if t == nil {
		return nil
	}
	v := t.UTC().Format(time.RFC3339Nano)
	return &v
}
func temporalEvidence(f catalog.AcceptedFact) readcontrol.TemporalEvidence {
	m := f.Temporal
	accepted := ""
	if m.AcceptedAt != nil {
		accepted = *temporalInstant(m.AcceptedAt)
	}
	return readcontrol.TemporalEvidence{ClaimID: f.Claim.ID, Validity: string(m.Validity), ValidFrom: temporalInstant(m.ValidFrom), ValidTo: temporalInstant(m.ValidTo), RecordedAt: temporalInstant(m.RecordedAt), SourceRecordedAt: m.SourceRecordedAt.UTC().Format(time.RFC3339Nano), AcceptedAt: accepted, FreshnessClass: string(m.Class), VerifiedAt: temporalInstant(m.VerifiedAt), Freshness: readcontrol.TemporalFreshness{Status: f.Freshness.Status, RevalidateAfter: temporalInstant(f.Freshness.RevalidateAfter), RequiresRevalidation: f.Freshness.RequiresRevalidation, CurrentUsable: f.Freshness.CurrentUsable}, Historical: f.Historical, TemporalRevision: m.Revision}
}

// The ordinary reader owns exact source bytes. This optional enrichment runs
// before adaptive ObserveTx so the same final JSON is hashed by Core and Broker.
// It never adds a source, claim value, scope, read receipt or owner authority.
func (s *MemoryReads) enrichTemporalReadTx(ctx context.Context, tx pgx.Tx, result *readcontrol.Result) error {
	refs := []catalog.SourceRef{}
	seen := map[string]bool{}
	for _, item := range result.Items {
		if !seen[item.Source.ID] {
			seen[item.Source.ID] = true
			refs = append(refs, catalog.SourceRef{ID: item.Source.ID, Revision: item.Source.Revision, AccessRevision: item.Source.AccessRevision, Hash: item.Source.Hash})
		}
	}
	page, err := s.sources.ReadAcceptedFactsForSourcesTx(ctx, tx, refs, catalog.TemporalQuery{})
	if err != nil {
		return err
	}
	bySource := map[string][]readcontrol.TemporalEvidence{}
	for _, f := range page.Items {
		bySource[f.Claim.SourceID] = append(bySource[f.Claim.SourceID], temporalEvidence(f))
	}
	result.UnknownValidity = page.UnknownValidity
	result.NotKnownAtObservation = page.NotKnownAtObservation
	result.Truncated = result.Truncated || page.Truncated
	count := 0
	for i := range result.Items {
		evidence := bySource[result.Items[i].Source.ID]
		if count+len(evidence) > catalog.MaxTemporalFacts {
			result.Truncated = true
			continue
		}
		result.Items[i].Temporal = evidence
		count += len(evidence)
	}
	return nil
}
