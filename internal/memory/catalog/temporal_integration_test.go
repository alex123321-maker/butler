package catalog

import (
	"context"
	"encoding/json"
	"errors"
	"testing"
	"time"
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
