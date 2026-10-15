package app

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
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
		if errors.Is(err, catalog.ErrStale) {
			return readcontrol.Empty("stale"), nil
		}
		if errors.Is(err, catalog.ErrDenied) || errors.Is(err, catalog.ErrDeleted) || errors.Is(err, catalog.ErrRestore) {
			return readcontrol.Empty("denied"), nil
		}
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
		var canonical any
		decoder := json.NewDecoder(bytes.NewReader(raw))
		decoder.UseNumber()
		if err = decoder.Decode(&canonical); err != nil {
			return out, err
		}
		raw, err = json.Marshal(canonical)
		if err != nil {
			return out, err
		}
		if len(raw) > 4096 || !temporalFactValueSafe(canonical) {
			out.Truncated = true
			continue
		}
		item := readcontrol.Item{Source: readcontrol.SourceRef{ID: f.Pin.Source.ID, Revision: f.Pin.Source.Revision, AccessRevision: f.Pin.Source.AccessRevision, Hash: f.Pin.Source.Hash}, Kind: "accepted_fact_snapshot", ScopeType: f.Claim.ScopeType, Title: "Owner-accepted temporal DATA", Text: string(raw), Locator: json.RawMessage(`{}`), Freshness: "current_revision_unverified_fact", Conflict: f.Conflict, Temporal: []readcontrol.TemporalEvidence{temporalEvidence(f)}}
		if f.Claim.EventTime != nil {
			item.EventTime = f.Claim.EventTime.UTC().Format(time.RFC3339Nano)
		}
		out.Items = append(out.Items, item)
	}
	if len(out.Items) > 0 {
		out.Status = "ok"
	}
	return out, nil
}

// Check strings/keys without passing precise accepted JSON numbers through a
// float64 sanitizer. Omitted credential-bearing records stay explicitly partial.
func temporalFactValueSafe(value any) bool {
	switch v := value.(type) {
	case string:
		return sanitize.Text(v) == v
	case map[string]any:
		for key, child := range v {
			if sanitize.Text(key) != key {
				return false
			}
			probe, _ := json.Marshal(map[string]string{key: "temporal_value_probe"})
			if sanitize.JSON(string(probe)) != string(probe) || !temporalFactValueSafe(child) {
				return false
			}
		}
	case []any:
		for _, child := range v {
			if !temporalFactValueSafe(child) {
				return false
			}
		}
	}
	return true
}
