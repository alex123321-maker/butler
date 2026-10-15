package app

import (
	"context"
	"encoding/json"
	"time"

	"github.com/butler/butler/internal/memory/catalog"
	"github.com/butler/butler/internal/memory/factcontrol"
	"github.com/butler/butler/internal/memory/readcontrol"
	"github.com/butler/butler/internal/memory/sanitize"
	"github.com/jackc/pgx/v5"
)

func (s *MemoryReads) readTemporalFactsTx(ctx context.Context, tx pgx.Tx, a factcontrol.Args) (readcontrol.Result, error) {
	out := readcontrol.Empty("empty")
	q := catalog.TemporalQuery{}
	for _, entry := range []struct {
		raw    string
		target **time.Time
	}{{a.ValidAt, &q.ValidAt}, {a.KnownAt, &q.KnownAt}} {
		if entry.raw != "" {
			v, err := time.Parse(time.RFC3339Nano, entry.raw)
			if err != nil {
				return out, factcontrol.ErrDenied
			}
			v = v.UTC()
			*entry.target = &v
		}
	}
	refs := make([]catalog.SourceRef, len(a.Sources))
	for i, r := range a.Sources {
		refs[i] = catalog.SourceRef{ID: r.ID, Revision: r.Revision, AccessRevision: r.AccessRevision, Hash: r.Hash}
	}
	page, err := s.sources.ReadAcceptedFactsForSourcesTx(ctx, tx, refs, q)
	if err != nil {
		return out, err
	}
	out.UnknownValidity = page.UnknownValidity
	out.NotKnownAtObservation = page.NotKnownAtObservation
	out.Truncated = page.Truncated
	for _, f := range page.Items {
		if len(out.Items) >= a.Limit {
			out.Truncated = true
			break
		}
		value := struct {
			Key         string          `json:"key"`
			Value       json.RawMessage `json:"value"`
			Summary     string          `json:"summary"`
			ClaimStatus string          `json:"claim_status"`
		}{f.Claim.Key, f.Claim.Value, f.Claim.Summary, f.Claim.Status}
		raw, err := json.Marshal(value)
		if err != nil {
			return out, err
		}
		if len(raw) > 4096 || sanitize.JSON(string(raw)) != string(raw) {
			out.Truncated = true
			continue
		}
		out.Items = append(out.Items, readcontrol.Item{Source: readcontrol.SourceRef{ID: f.Pin.Source.ID, Revision: f.Pin.Source.Revision, AccessRevision: f.Pin.Source.AccessRevision, Hash: f.Pin.Source.Hash}, Kind: "accepted_fact_snapshot", ScopeType: f.Claim.ScopeType, Title: "Owner-accepted temporal DATA", Text: string(raw), Locator: json.RawMessage(`{}`), Freshness: "current_revision_unverified_fact", Temporal: []readcontrol.TemporalEvidence{temporalEvidence(f)}})
	}
	if len(out.Items) > 0 {
		out.Status = "ok"
	}
	return out, nil
}
