package catalog

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"sort"
	"time"

	"github.com/butler/butler/internal/owneridentity"
	"github.com/butler/butler/internal/ownership"
	"github.com/jackc/pgx/v5"
)

const MaxTemporalFacts = 64

type derivedCreation struct {
	xid                       string
	source                    SourceRef
	owner, scopeType, scopeID string
}

func loadTemporalMetadataTx(ctx context.Context, tx pgx.Tx, c Claim, q TemporalQuery) (TemporalMetadata, int64, error) {
	m := TemporalMetadata{ClaimID: c.ID, Validity: ValidityUnknown, Class: FreshnessUnknown, VerificationBasis: "unknown"}
	var head int64
	if err := tx.QueryRow(ctx, `SELECT v.extracted_at,c.accepted_at,COALESCE(h.revision,0) FROM memory_claims c JOIN memory_source_revisions v ON v.source_id=c.source_id AND v.revision=c.source_revision LEFT JOIN memory_claim_temporal_heads h ON h.claim_id=c.claim_id WHERE c.claim_id=$1`, c.ID).Scan(&m.SourceRecordedAt, &m.AcceptedAt, &head); err != nil {
		return m, 0, err
	}
	var maximum int64
	if err := tx.QueryRow(ctx, `SELECT COALESCE(MAX(revision),0) FROM memory_claim_temporal_revisions WHERE claim_id=$1`, c.ID).Scan(&maximum); err != nil {
		return m, 0, err
	}
	if maximum != head {
		return m, 0, ErrStale
	}
	err := tx.QueryRow(ctx, `SELECT revision,validity,valid_from,valid_to,recorded_at,freshness_class,verified_at,verification_basis,verification_action_id,verification_evidence_id,verification_resource_hash FROM memory_claim_temporal_revisions WHERE claim_id=$1 AND ($2::timestamptz IS NULL OR recorded_at<=$2) ORDER BY revision DESC LIMIT 1`, c.ID, q.KnownAt).Scan(&m.Revision, &m.Validity, &m.ValidFrom, &m.ValidTo, &m.RecordedAt, &m.Class, &m.VerifiedAt, &m.VerificationBasis, &m.VerificationActionID, &m.VerificationEvidenceID, &m.VerificationResourceHash)
	if errors.Is(err, pgx.ErrNoRows) {
		err = nil
	}
	return m, head, err
}

func (s *Store) ReadAcceptedFactsTx(ctx context.Context, tx pgx.Tx, claimIDs []string, q TemporalQuery) (AcceptedFactPage, error) {
	// A foreground run can originate from an authenticated owner request. Once
	// owned, its reads must use the run's current scope/fence and persist source
	// pins; an ambient owner actor cannot replace that narrower execution proof.
	ctx = temporalReadContext(ctx)
	out := AcceptedFactPage{Items: []AcceptedFact{}}
	if tx == nil || len(claimIDs) > MaxTemporalFacts || q.ValidAt != nil && q.ValidAt.IsZero() || q.KnownAt != nil && q.KnownAt.IsZero() {
		return out, ErrInvalid
	}
	ids := append([]string(nil), claimIDs...)
	sort.Strings(ids)
	for i, id := range ids {
		if !bounded(id, 1024) || i > 0 && id == ids[i-1] {
			return out, ErrInvalid
		}
	}
	refs := []SourceRef{}
	claims := make([]Claim, 0, len(ids))
	for _, id := range ids {
		c, err := getClaim(ctx, tx, id, false)
		if err != nil {
			return out, ErrDenied
		}
		claims = append(claims, c)
		refs = append(refs, SourceRef{ID: c.SourceID, Revision: c.SourceRevision})
	}
	sources, err := s.AccessTx(ctx, tx, refs, "memory_read")
	if err != nil {
		return out, err
	}
	byID := map[string]Source{}
	for _, src := range sources {
		byID[src.ID] = src
	}
	// Sorted source graph locks precede decision/head locks, as in owner Decide.
	if _, err = tx.Exec(ctx, `SELECT h.owner_id FROM memory_fact_heads h WHERE EXISTS(SELECT 1 FROM memory_claims c WHERE c.claim_id=ANY($1) AND c.owner_id=h.owner_id AND c.scope_type=h.scope_type AND c.scope_id=h.scope_id AND c.fact_key=h.fact_key) ORDER BY h.owner_id,h.scope_type,h.scope_id,h.fact_key FOR SHARE OF h`, ids); err != nil {
		return out, err
	}
	if _, err = tx.Exec(ctx, `SELECT claim_id FROM memory_claims WHERE claim_id=ANY($1) ORDER BY claim_id FOR SHARE`, ids); err != nil {
		return out, err
	}
	var now time.Time
	if err = tx.QueryRow(ctx, `SELECT clock_timestamp()`).Scan(&now); err != nil {
		return out, err
	}
	if q.KnownAt != nil && q.KnownAt.After(now) {
		return out, ErrInvalid
	}
	for _, routed := range claims {
		c, err := getClaim(ctx, tx, routed.ID, false)
		if err != nil {
			return out, err
		}
		if c.Status != "confirmed" && c.Status != "superseded" {
			continue
		}
		m, temporalHead, err := loadTemporalMetadataTx(ctx, tx, c, q)
		if err != nil {
			return out, err
		}
		if m.AcceptedAt == nil {
			continue
		}
		var head int64
		var active string
		if err = tx.QueryRow(ctx, `SELECT revision,active_claim_id FROM memory_fact_heads WHERE owner_id=$1 AND scope_type=$2 AND scope_id=$3 AND fact_key=$4`, c.OwnerID, c.ScopeType, c.ScopeID, c.Key).Scan(&head, &active); err != nil {
			return out, err
		}
		if q.KnownAt != nil && (m.SourceRecordedAt.After(*q.KnownAt) || c.ExtractedAt.After(*q.KnownAt) || m.AcceptedAt.After(*q.KnownAt)) {
			out.NotKnownAtObservation++
			continue
		}
		if q.ValidAt == nil {
			selected := active
			if q.KnownAt != nil {
				err = tx.QueryRow(ctx, `SELECT claim_id FROM memory_fact_decision_history WHERE owner_id=$1 AND scope_type=$2 AND scope_id=$3 AND fact_key=$4 AND recorded_at<=$5 ORDER BY head_revision DESC LIMIT 1`, c.OwnerID, c.ScopeType, c.ScopeID, c.Key, q.KnownAt).Scan(&selected)
				if errors.Is(err, pgx.ErrNoRows) {
					out.NotKnownAtObservation++
					continue
				}
				if err != nil {
					return out, err
				}
			}
			if selected != c.ID {
				continue
			}
		} else {
			if m.Validity == ValidityUnknown {
				out.UnknownValidity++
				continue
			}
			if m.ValidFrom != nil && q.ValidAt.Before(*m.ValidFrom) || m.ValidTo != nil && !q.ValidAt.Before(*m.ValidTo) {
				continue
			}
		}
		var freshJSON []byte
		var metadataHash string
		if err = tx.QueryRow(ctx, `SELECT butler_memory_temporal_freshness($1,$2),butler_memory_temporal_metadata_hash($1,$2)`, c.ID, q.KnownAt).Scan(&freshJSON, &metadataHash); err != nil {
			return out, err
		}
		var fresh Freshness
		if err = json.Unmarshal(freshJSON, &fresh); err != nil {
			return out, err
		}
		src, ok := byID[c.SourceID]
		if !ok {
			return out, ErrDenied
		}
		pin := AcceptedFactPin{ClaimID: c.ID, HeadRevision: head, ActiveID: active, TemporalRevision: temporalHead, Source: SourceRef{src.ID, src.Revision, src.AccessRevision, src.Hash}, MetadataHash: metadataHash, FreshnessStatus: fresh.Status, CurrentUsable: fresh.CurrentUsable}
		out.Items = append(out.Items, AcceptedFact{Claim: c, Temporal: m, Freshness: fresh, Pin: pin, Historical: q.ValidAt != nil || q.KnownAt != nil})
	}
	// Report overlapping, different selected accepted values without choosing a
	// winner by upload time or mutating the existing owner's current decision.
	for i := range out.Items {
		for j := range out.Items {
			a, b := out.Items[i].Claim, out.Items[j].Claim
			if i != j && a.OwnerID == b.OwnerID && a.ScopeType == b.ScopeType && a.ScopeID == b.ScopeID && a.Key == b.Key && !bytes.Equal(a.Value, b.Value) {
				out.Items[i].Conflict = true
			}
		}
	}
	if err = bindTemporalRunTx(ctx, tx, out.Items, q); err != nil {
		return AcceptedFactPage{}, err
	}
	if err = s.finalizeAccessTx(ctx, tx, refs, "memory_read"); err != nil {
		return AcceptedFactPage{}, err
	}
	return out, nil
}

func temporalReadContext(ctx context.Context) context.Context {
	if _, owned := ownership.TokenFromContext(ctx); owned {
		return owneridentity.WithActor(ctx, owneridentity.Actor{})
	}
	return ctx
}

func (s *Store) CheckAcceptedFactPinsTx(ctx context.Context, tx pgx.Tx, pins []AcceptedFactPin, q TemporalQuery) error {
	ids := make([]string, len(pins))
	expected := map[string]AcceptedFactPin{}
	for i, p := range pins {
		ids[i] = p.ClaimID
		if _, duplicate := expected[p.ClaimID]; duplicate {
			return ErrInvalid
		}
		expected[p.ClaimID] = p
	}
	page, err := s.ReadAcceptedFactsTx(ctx, tx, ids, q)
	if err != nil {
		return err
	}
	if len(page.Items) != len(pins) || page.Truncated || page.UnknownValidity > 0 || page.NotKnownAtObservation > 0 {
		return ErrStale
	}
	for _, f := range page.Items {
		if f.Pin != expected[f.Claim.ID] {
			return ErrStale
		}
	}
	return nil
}

func (s *Store) BindAcceptedFactPinsTx(ctx context.Context, tx pgx.Tx, derivedSourceID string, pins []AcceptedFactPin, q TemporalQuery) error {
	if !bounded(derivedSourceID, 1024) || len(pins) == 0 {
		return ErrInvalid
	}
	// Route every pin through the child's complete declared source graph before
	// taking any accepted-fact head locks. No later read may add another source.
	for _, p := range pins {
		var parent bool
		if err := tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM memory_source_dependencies WHERE child_source_id=$1 AND parent_source_id=$2 AND parent_revision=$3)`, derivedSourceID, p.Source.ID, p.Source.Revision).Scan(&parent); err != nil {
			return err
		}
		if !parent {
			return ErrStale
		}
	}
	if _, err := s.AccessTx(ctx, tx, []SourceRef{{ID: derivedSourceID}}, "memory_read"); err != nil {
		return err
	}
	if err := s.CheckAcceptedFactPinsTx(ctx, tx, pins, q); err != nil {
		return err
	}
	return bindTemporalPinsTx(ctx, tx, derivedSourceID, pins, q)
}

// BindNewAcceptedFactPinsTx handles only a freshly created immutable derivative
// in this exact transaction. It proves the original parents using the existing
// caller/job authority; the new child never becomes a caller-selectable read or
// owner identity. Quarantine remains enforced by ordinary access until activation.
func (s *Store) BindNewAcceptedFactPinsTx(ctx context.Context, tx pgx.Tx, derived Source, parents []SourceRef, pins []AcceptedFactPin, q TemporalQuery) error {
	if tx == nil || len(parents) == 0 || len(parents) > 64 || len(pins) == 0 || derived.Kind != "model_candidate" || derived.Revision != 1 || derived.AccessRevision != 1 || (derived.Status != "active" && derived.Status != "quarantined") {
		return ErrInvalid
	}
	for _, p := range parents {
		if p.ID == "" || p.Revision < 1 || p.AccessRevision < 1 || len(p.Hash) != 64 {
			return ErrInvalid
		}
	}
	if derived.creation == nil || derived.creation.source != (SourceRef{derived.ID, derived.Revision, derived.AccessRevision, derived.Hash}) || derived.creation.owner != derived.OwnerID || derived.creation.scopeType != derived.ScopeType || derived.creation.scopeID != derived.ScopeID {
		return ErrDenied
	}
	for _, p := range pins {
		found := false
		for _, parent := range parents {
			if parent == p.Source {
				found = true
				break
			}
		}
		if !found {
			return ErrStale
		}
	}
	current, err := s.AccessTx(ctx, tx, parents, "memory_read")
	if err != nil {
		return err
	}
	byID := map[string]Source{}
	for _, src := range current {
		byID[src.ID] = src
	}
	anchor, ok := byID[parents[0].ID]
	if !ok || derived.OwnerID != anchor.OwnerID || derived.ScopeType != anchor.ScopeType || derived.ScopeID != anchor.ScopeID {
		return ErrDenied
	}
	if err = s.CheckAcceptedFactPinsTx(ctx, tx, pins, q); err != nil {
		return err
	}
	// A constructor-issued receipt binds the top transaction across savepoints.
	// Comparing row xmin to a top xid would reject legitimate subtransactions.
	var fresh bool
	err = tx.QueryRow(ctx, `SELECT pg_current_xact_id()::text=$7 AND s.created_at>=transaction_timestamp() AND v.extracted_at>=transaction_timestamp() AND s.source_kind='model_candidate' AND s.external_key='derived:'||s.source_id AND s.head_revision=1 AND s.access_revision=1 AND s.status=$2 AND s.owner_id=$3 AND s.scope_type=$4 AND s.scope_id=$5 AND v.content_hash=$6 FROM memory_sources s JOIN memory_source_revisions v ON v.source_id=s.source_id AND v.revision=1 WHERE s.source_id=$1`, derived.ID, derived.Status, derived.OwnerID, derived.ScopeType, derived.ScopeID, derived.Hash, derived.creation.xid).Scan(&fresh)
	if err != nil {
		return err
	}
	if !fresh {
		return ErrDenied
	}
	var exact bool
	err = tx.QueryRow(ctx, `WITH p AS(SELECT * FROM jsonb_to_recordset($2::jsonb) AS x(source_id text,revision bigint,access_revision bigint,content_hash text)),d AS(SELECT parent_source_id,parent_revision FROM memory_source_dependencies WHERE child_source_id=$1) SELECT (SELECT count(*) FROM d)=(SELECT count(*) FROM p) AND NOT EXISTS(SELECT 1 FROM d LEFT JOIN p ON p.source_id=d.parent_source_id AND p.revision=d.parent_revision WHERE p.source_id IS NULL)`, derived.ID, mustJSON(parents)).Scan(&exact)
	if err != nil {
		return err
	}
	if !exact {
		return ErrStale
	}
	return bindTemporalPinsTx(ctx, tx, derived.ID, pins, q)
}

func bindTemporalPinsTx(ctx context.Context, tx pgx.Tx, derivedSourceID string, pins []AcceptedFactPin, q TemporalQuery) error {
	// A rejected later pin must not leave a partially protected derivative when
	// the caller handles the error and commits its outer transaction.
	batch, err := tx.Begin(ctx)
	if err != nil {
		return err
	}
	defer batch.Rollback(ctx)
	tx = batch
	for _, p := range pins {
		if _, err := tx.Exec(ctx, `INSERT INTO memory_temporal_source_pins(derived_source_id,claim_id,head_revision,active_id,temporal_revision,original_source_id,source_revision,access_revision,content_hash,metadata_hash,freshness_status,current_usable,valid_at,known_at) VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14) ON CONFLICT DO NOTHING`, derivedSourceID, p.ClaimID, p.HeadRevision, p.ActiveID, p.TemporalRevision, p.Source.ID, p.Source.Revision, p.Source.AccessRevision, p.Source.Hash, p.MetadataHash, p.FreshnessStatus, p.CurrentUsable, q.ValidAt, q.KnownAt); err != nil {
			return err
		}
		var same bool
		if err := tx.QueryRow(ctx, `SELECT head_revision=$3 AND active_id=$4 AND temporal_revision=$5 AND original_source_id=$6 AND source_revision=$7 AND access_revision=$8 AND content_hash=$9 AND metadata_hash=$10 AND freshness_status=$11 AND current_usable=$12 AND valid_at IS NOT DISTINCT FROM $13::timestamptz AND known_at IS NOT DISTINCT FROM $14::timestamptz FROM memory_temporal_source_pins WHERE derived_source_id=$1 AND claim_id=$2`, derivedSourceID, p.ClaimID, p.HeadRevision, p.ActiveID, p.TemporalRevision, p.Source.ID, p.Source.Revision, p.Source.AccessRevision, p.Source.Hash, p.MetadataHash, p.FreshnessStatus, p.CurrentUsable, q.ValidAt, q.KnownAt).Scan(&same); err != nil {
			return err
		}
		if !same {
			return ErrStale
		}
	}
	var current bool
	if err := tx.QueryRow(ctx, `SELECT butler_memory_temporal_sources_current(ARRAY[$1::text])`, derivedSourceID).Scan(&current); err != nil {
		return err
	}
	if !current {
		return ErrStale
	}
	return batch.Commit(ctx)
}
