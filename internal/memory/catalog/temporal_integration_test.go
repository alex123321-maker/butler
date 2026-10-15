package catalog

import (
	"context"
	"encoding/json"
	"errors"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
)

func temporalRead(t *testing.T, s *Store, ctx context.Context, ids []string, q TemporalQuery) AcceptedFactPage {
	t.Helper()
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Rollback(ctx)
	page, err := s.ReadAcceptedFactsTx(ctx, tx, ids, q)
	if err != nil {
		t.Fatal(err)
	}
	return page
}

type temporalPinnedChecker struct{ parents []SourceRef }

func (p temporalPinnedChecker) PinnedMemorySources() []SourceRef {
	return append([]SourceRef(nil), p.parents...)
}
func (p temporalPinnedChecker) CheckMemoryAccessTx(_ context.Context, _ pgx.Tx, refs []SourceRef, _ string) (string, error) {
	for _, r := range refs {
		found := false
		for _, allowed := range p.parents {
			if r.ID == allowed.ID && (r.Revision == 0 || r.Revision == allowed.Revision) && (r.Hash == "" || r.Hash == allowed.Hash) && (r.AccessRevision == 0 || r.AccessRevision == allowed.AccessRevision) {
				found = true
				break
			}
		}
		if !found {
			return "", ErrDenied
		}
	}
	return "owner", nil
}
func (p temporalPinnedChecker) FinalizeMemoryAccessTx(ctx context.Context, tx pgx.Tx, refs []SourceRef, purpose string) error {
	_, err := p.CheckMemoryAccessTx(ctx, tx, refs, purpose)
	return err
}

func TestTemporalFreshDerivativeBindingDoesNotMintChildReadAuthorityIntegration(t *testing.T) {
	s, owner := fixture(t)
	src := put(t, s, owner, "accepted", "Пиши по-русски.")
	c := propose(t, s, owner, src, `"ru"`)
	c, err := s.Decide(owner, Decision{ClaimID: c.ID})
	if err != nil {
		t.Fatal(err)
	}
	_, err = s.SetTemporal(owner, TemporalUpdate{ClaimID: c.ID, ExpectedHeadRevision: 1, Validity: ValidityUnknown, Class: FreshnessStablePreference})
	if err != nil {
		t.Fatal(err)
	}
	facts := temporalRead(t, s, owner, []string{c.ID}, TemporalQuery{})
	pins := []AcceptedFactPin{facts.Items[0].Pin}
	parents := []SourceRef{ref(src)}
	ctx := WithAccessChecker(context.Background(), temporalPinnedChecker{parents})
	for _, mode := range []struct {
		status    string
		savepoint bool
	}{{"active", false}, {"quarantined", false}, {"active", true}, {"quarantined", true}} {
		name := mode.status
		if mode.savepoint {
			name += "_savepoint"
		}
		t.Run(name, func(t *testing.T) {
			tx, err := s.pool.Begin(ctx)
			if err != nil {
				t.Fatal(err)
			}
			defer tx.Rollback(ctx)
			creationTx := tx
			if mode.savepoint {
				creationTx, err = tx.Begin(ctx)
				if err != nil {
					t.Fatal(err)
				}
				defer creationTx.Rollback(ctx)
			}
			derived, err := s.CreateDerivedTx(ctx, creationTx, parents, "memory_read", "view", "Exact candidate DATA", json.RawMessage(`{}`))
			if err != nil {
				t.Fatal(err)
			}
			if mode.status == "quarantined" {
				if _, err = creationTx.Exec(ctx, `UPDATE memory_sources SET status='quarantined' WHERE source_id=$1`, derived.ID); err != nil {
					t.Fatal(err)
				}
				derived.Status = mode.status
			}
			if mode.savepoint {
				if err = creationTx.Commit(ctx); err != nil {
					t.Fatal(err)
				}
			}
			if err = s.BindAcceptedFactPinsTx(ctx, tx, derived.ID, pins, TemporalQuery{}); !errors.Is(err, ErrDenied) {
				t.Fatal("strict path expanded job manifest", err)
			}
			forged := derived
			forged.Hash = "ffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffff"
			if err = s.BindNewAcceptedFactPinsTx(ctx, tx, forged, parents, pins, TemporalQuery{}); !errors.Is(err, ErrDenied) {
				t.Fatal("forged fresh source hash bound", err)
			}
			serialized, err := json.Marshal(derived)
			if err != nil {
				t.Fatal(err)
			}
			var modelCopy Source
			if err = json.Unmarshal(serialized, &modelCopy); err != nil {
				t.Fatal(err)
			}
			if err = s.BindNewAcceptedFactPinsTx(ctx, tx, modelCopy, parents, pins, TemporalQuery{}); !errors.Is(err, ErrDenied) {
				t.Fatal("serialized source forged constructor receipt", err)
			}
			if err = s.BindNewAcceptedFactPinsTx(ctx, tx, derived, parents, pins, TemporalQuery{}); err != nil {
				t.Fatal(err)
			}
			if _, err = s.AccessTx(ctx, tx, []SourceRef{ref(derived)}, "memory_read"); !errors.Is(err, ErrDenied) {
				t.Fatal("binding minted general child read authority", err)
			}
			if err = tx.Commit(ctx); err != nil {
				t.Fatal(err)
			}
			next, err := s.pool.Begin(ctx)
			if err != nil {
				t.Fatal(err)
			}
			defer next.Rollback(ctx)
			if err = s.BindNewAcceptedFactPinsTx(ctx, next, derived, parents, pins, TemporalQuery{}); !errors.Is(err, ErrDenied) {
				t.Fatal("existing source reused fresh-row proof", err)
			}
		})
	}
}

func TestTemporalBoundDerivativeInvalidatesAfterMetadataAndPrivacyIntegration(t *testing.T) {
	s, ctx := fixture(t)
	src := put(t, s, ctx, "rule", "Пиши по-русски.")
	c := propose(t, s, ctx, src, `"ru"`)
	c, err := s.Decide(ctx, Decision{ClaimID: c.ID})
	if err != nil {
		t.Fatal(err)
	}
	_, err = s.SetTemporal(ctx, TemporalUpdate{ClaimID: c.ID, ExpectedHeadRevision: 1, Validity: ValidityUnknown, Class: FreshnessStablePreference})
	if err != nil {
		t.Fatal(err)
	}
	facts := temporalRead(t, s, ctx, []string{c.ID}, TemporalQuery{})
	pins := []AcceptedFactPin{facts.Items[0].Pin}
	parents := []SourceRef{ref(src)}
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Rollback(ctx)
	derived, err := s.CreateDerivedTx(ctx, tx, parents, "memory_read", "view", "Retained quoted DATA", json.RawMessage(`{}`))
	if err != nil {
		t.Fatal(err)
	}
	if err = s.BindNewAcceptedFactPinsTx(ctx, tx, derived, parents, pins, TemporalQuery{}); err != nil {
		t.Fatal(err)
	}
	if err = tx.Commit(ctx); err != nil {
		t.Fatal(err)
	}
	if _, err = s.Get(ctx, derived.ID); err != nil {
		t.Fatal(err)
	}
	past := time.Now().Add(-time.Hour).UTC()
	if _, err = s.SetTemporal(ctx, TemporalUpdate{ClaimID: c.ID, ExpectedRevision: 1, ExpectedHeadRevision: 1, Validity: ValidityInterval, ValidTo: &past, Class: FreshnessStablePreference}); err != nil {
		t.Fatal(err)
	}
	if _, err = s.Get(ctx, derived.ID); !errors.Is(err, ErrStale) {
		t.Fatal("stale temporal view released unchanged source bytes", err)
	}
	current, err := s.Get(ctx, src.ID)
	if err != nil || current.Hash != src.Hash || current.Body != src.Body {
		t.Fatal("logical expiry redacted original history", err)
	}
	if _, err = s.Delete(ctx, src.ID, "delete", src.Revision); err != nil {
		t.Fatal(err)
	}
	var metadata int
	if err = s.pool.QueryRow(ctx, `SELECT (SELECT count(*) FROM memory_claim_temporal_revisions)+(SELECT count(*) FROM memory_temporal_source_pins)+(SELECT count(*) FROM memory_claim_temporal_heads)`).Scan(&metadata); err != nil || metadata != 0 {
		t.Fatal("privacy deletion retained semantic temporal metadata", metadata, err)
	}
	if _, err = s.Get(ctx, derived.ID); err == nil {
		t.Fatal("privacy-deleted history returned")
	}
}
func temporalUpdate(t *testing.T, s *Store, ctx context.Context, c Claim, head int64, from, to *time.Time, class FreshnessClass) TemporalMetadata {
	t.Helper()
	m, err := s.SetTemporal(ctx, TemporalUpdate{ClaimID: c.ID, ExpectedHeadRevision: head, Validity: ValidityInterval, ValidFrom: from, ValidTo: to, Class: class, OwnerVerifiedNow: true})
	if err != nil {
		t.Fatal(err)
	}
	return m
}
func TestTemporalHistoricalValidityAndRecordedKnowledgeIntegration(t *testing.T) {
	s, ctx := fixture(t)
	feb := time.Date(2026, 2, 1, 0, 0, 0, 0, time.UTC)
	march := feb.AddDate(0, 1, 0)
	midFeb := feb.Add(14 * 24 * time.Hour)
	a := propose(t, s, ctx, put(t, s, ctx, "server:a", "В феврале сервер A."), `"A"`)
	var err error
	a, err = s.Decide(ctx, Decision{ClaimID: a.ID})
	if err != nil {
		t.Fatal(err)
	}
	temporalUpdate(t, s, ctx, a, 1, &feb, &march, FreshnessTechnicalFact)
	var knownA time.Time
	if err = s.pool.QueryRow(ctx, `SELECT clock_timestamp()`).Scan(&knownA); err != nil {
		t.Fatal(err)
	}
	b := propose(t, s, ctx, put(t, s, ctx, "server:b", "С марта сервер B."), `"B"`)
	b, err = s.Decide(ctx, Decision{ClaimID: b.ID, ExpectedActiveID: a.ID})
	if err != nil {
		t.Fatal(err)
	}
	temporalUpdate(t, s, ctx, b, 2, &march, nil, FreshnessTechnicalFact)
	ids := []string{a.ID, b.ID}
	for _, tc := range []struct {
		name string
		q    TemporalQuery
		want string
	}{{"february", TemporalQuery{ValidAt: &midFeb}, a.ID}, {"exclusive_march_boundary", TemporalQuery{ValidAt: &march}, b.ID}, {"current", TemporalQuery{}, b.ID}, {"known_before_later_document", TemporalQuery{KnownAt: &knownA}, a.ID}, {"valid_and_known_are_independent", TemporalQuery{ValidAt: &midFeb, KnownAt: &knownA}, a.ID}} {
		t.Run(tc.name, func(t *testing.T) {
			page := temporalRead(t, s, ctx, ids, tc.q)
			if len(page.Items) != 1 || page.Items[0].Claim.ID != tc.want {
				t.Fatalf("wrong temporal selection: %+v", page)
			}
		})
	}
	// A later upload remains a candidate and cannot silently replace B.
	old := propose(t, s, ctx, put(t, s, ctx, "late-old-document", "Архив: старый сервер A."), `"A from old upload"`)
	page := temporalRead(t, s, ctx, append(ids, old.ID), TemporalQuery{})
	if len(page.Items) != 1 || page.Items[0].Claim.ID != b.ID {
		t.Fatal("late upload replaced current owner decision")
	}
	var decisions int
	if err = s.pool.QueryRow(ctx, `SELECT count(*) FROM memory_fact_decision_history`).Scan(&decisions); err != nil || decisions != 2 {
		t.Fatal("owner decision order missing", decisions, err)
	}
}

func TestTemporalUnknownDateAndOwnerCASIntegration(t *testing.T) {
	s, ctx := fixture(t)
	src := put(t, s, ctx, "unknown", "Срок не указан; с завтрашнего утра зависит от неизвестной зоны.")
	c := propose(t, s, ctx, src, `"unknown timing"`)
	c, err := s.Decide(ctx, Decision{ClaimID: c.ID})
	if err != nil {
		t.Fatal(err)
	}
	page := temporalRead(t, s, ctx, []string{c.ID}, TemporalQuery{})
	if len(page.Items) != 1 || page.Items[0].Temporal.Validity != ValidityUnknown || page.Items[0].Temporal.RecordedAt != nil || page.Items[0].Temporal.ValidFrom != nil || page.Items[0].Temporal.VerifiedAt != nil || page.Items[0].Claim.EventTime != nil || page.Items[0].Freshness.Status != "unknown" || page.Items[0].Freshness.CurrentUsable {
		t.Fatal("unknown dates/verification were invented", page)
	}
	now := time.Now().UTC()
	page = temporalRead(t, s, ctx, []string{c.ID}, TemporalQuery{ValidAt: &now})
	if len(page.Items) != 0 || page.UnknownValidity != 1 {
		t.Fatal("unknown validity silently matched dated query", page)
	}
	request := TemporalUpdate{ClaimID: c.ID, ExpectedHeadRevision: 1, Validity: ValidityUnknown, Class: FreshnessStablePreference}
	m, err := s.SetTemporal(ctx, request)
	if err != nil || m.Revision != 1 {
		t.Fatal(m, err)
	}
	if _, err = s.SetTemporal(ctx, request); !errors.Is(err, ErrStale) {
		t.Fatal("temporal CAS reset owner metadata", err)
	}
	request.ExpectedRevision = 1
	request.ExpectedHeadRevision = 0
	if _, err = s.SetTemporal(ctx, request); !errors.Is(err, ErrStale) {
		t.Fatal("owner head CAS bypassed", err)
	}
	for _, raw := range []string{`{"valid_from":"2026-02-01"}`, `{"valid_from":"2026-02-01T10:00:00"}`, `{"valid_from":"tomorrow"}`} {
		var p TemporalUpdate
		if json.Unmarshal([]byte(raw), &p) == nil {
			t.Fatal("unknown timezone/relative date parsed as an instant", raw)
		}
	}
	if _, err = s.SetTemporal(context.Background(), request); !errors.Is(err, ErrDenied) {
		t.Fatal("model context wrote owner temporal metadata", err)
	}
}

func TestTemporalFreshnessExpiryPreservesHistoryAndOwnerDecisionIntegration(t *testing.T) {
	s, ctx := fixture(t)
	c := propose(t, s, ctx, put(t, s, ctx, "technical", "Версия тестового сервиса: 1."), `"version 1"`)
	c, err := s.Decide(ctx, Decision{ClaimID: c.ID})
	if err != nil {
		t.Fatal(err)
	}
	_, err = s.SetTemporal(ctx, TemporalUpdate{ClaimID: c.ID, ExpectedHeadRevision: 1, Validity: ValidityInterval, Class: FreshnessTechnicalFact})
	if err != nil {
		t.Fatal(err)
	}
	page := temporalRead(t, s, ctx, []string{c.ID}, TemporalQuery{})
	if page.Items[0].Freshness.Status != "unverified" {
		t.Fatal("unverified fact treated as current")
	}
	// Seed an actual previously recorded verification without a wall-clock wait.
	// This fixture-only insert does not claim to prove Broker receipt admission.
	_, err = s.pool.Exec(ctx, `INSERT INTO memory_claim_temporal_revisions(claim_id,revision,validity,freshness_class,verified_at,verification_basis,recorded_by) VALUES($1,2,'interval','technical_fact',clock_timestamp()-interval '25 hours','owner_statement','owner')`, c.ID)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = s.pool.Exec(ctx, `UPDATE memory_claim_temporal_heads SET revision=2 WHERE claim_id=$1`, c.ID); err != nil {
		t.Fatal(err)
	}
	page = temporalRead(t, s, ctx, []string{c.ID}, TemporalQuery{})
	if page.Items[0].Freshness.Status != "needs_revalidation" || page.Items[0].Freshness.CurrentUsable || !page.Items[0].Freshness.RequiresRevalidation {
		t.Fatal("expired technical verification remained current", page)
	}
	var status, head string
	var sourceBody string
	if err = s.pool.QueryRow(ctx, `SELECT c.status,h.active_claim_id,v.body FROM memory_claims c JOIN memory_fact_heads h ON h.active_claim_id=c.claim_id JOIN memory_source_revisions v ON v.source_id=c.source_id AND v.revision=c.source_revision WHERE c.claim_id=$1`, c.ID).Scan(&status, &head, &sourceBody); err != nil || status != "confirmed" || head != c.ID || sourceBody == "" {
		t.Fatal("freshness rewrote authority or erased history", status, head, err)
	}
	now := time.Now().UTC()
	history := temporalRead(t, s, ctx, []string{c.ID}, TemporalQuery{ValidAt: &now})
	if len(history.Items) != 1 {
		t.Fatal("logical expiry erased permitted historical evidence")
	}
}
