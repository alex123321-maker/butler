package catalog

import (
	"testing"
	"time"
)

func TestTemporalDistinctFreshnessPoliciesAndHistoricalConflictIntegration(t *testing.T) {
	s, ctx := fixture(t)
	a := propose(t, s, ctx, put(t, s, ctx, "A", "value A"), `"A"`)
	var err error
	a, err = s.Decide(ctx, Decision{ClaimID: a.ID})
	if err != nil {
		t.Fatal(err)
	}
	for i, tc := range []struct {
		class  FreshnessClass
		age    time.Duration
		status string
	}{{FreshnessStablePreference, 365 * 24 * time.Hour, "stable"}, {FreshnessWorkStatus, 16 * time.Minute, "needs_revalidation"}, {FreshnessTechnicalFact, 25 * time.Hour, "needs_revalidation"}, {FreshnessTechnicalFact, time.Hour, "fresh"}, {FreshnessCompletedEpisode, 365 * 24 * time.Hour, "historical"}, {FreshnessUnknown, time.Hour, "unknown"}} {
		// A controlled historical owner-verification seed, not a claim of Broker
		// admission. Recording time stays the actual database recording instant.
		if _, err = s.pool.Exec(ctx, `INSERT INTO memory_claim_temporal_revisions(claim_id,revision,validity,freshness_class,verified_at,verification_basis,recorded_by) VALUES($1,$2,'interval',$3,clock_timestamp()-($4::bigint*interval '1 millisecond'),'owner_statement','owner')`, a.ID, i+1, tc.class, tc.age.Milliseconds()); err != nil {
			t.Fatal(err)
		}
		if _, err = s.pool.Exec(ctx, `INSERT INTO memory_claim_temporal_heads(claim_id,revision) VALUES($1,$2) ON CONFLICT(claim_id) DO UPDATE SET revision=EXCLUDED.revision`, a.ID, i+1); err != nil {
			t.Fatal(err)
		}
		page := temporalRead(t, s, ctx, []string{a.ID}, TemporalQuery{})
		if len(page.Items) != 1 || page.Items[0].Freshness.Status != tc.status {
			t.Fatal("uniform or missing per-type freshness", tc.class, page)
		}
	}
	var count int
	if err = s.pool.QueryRow(ctx, `SELECT count(*) FROM memory_fact_decision_history WHERE claim_id=$1`, a.ID).Scan(&count); err != nil || count != 1 {
		t.Fatal("freshness changed owner decision history", count, err)
	}
}

func TestTemporalOverlappingAcceptedValidityReportsSelectedConflictIntegration(t *testing.T) {
	s, ctx := fixture(t)
	start := time.Date(2026, 2, 1, 0, 0, 0, 0, time.UTC)
	end := start.AddDate(0, 1, 0)
	at := start.Add(14 * 24 * time.Hour)
	a := propose(t, s, ctx, put(t, s, ctx, "amber", "янтарный"), `"янтарный"`)
	var err error
	a, err = s.Decide(ctx, Decision{ClaimID: a.ID})
	if err != nil {
		t.Fatal(err)
	}
	temporalUpdate(t, s, ctx, a, 1, &start, &end, FreshnessTechnicalFact)
	b := propose(t, s, ctx, put(t, s, ctx, "teal", "бирюзовый"), `"бирюзовый"`)
	b, err = s.Decide(ctx, Decision{ClaimID: b.ID, ExpectedActiveID: a.ID})
	if err != nil {
		t.Fatal(err)
	}
	temporalUpdate(t, s, ctx, b, 2, &start, &end, FreshnessTechnicalFact)
	page := temporalRead(t, s, ctx, []string{a.ID, b.ID}, TemporalQuery{ValidAt: &at})
	if len(page.Items) != 2 || !page.Items[0].Conflict || !page.Items[1].Conflict {
		t.Fatal("selected contradictory periods silently resolved", page)
	}
	current := temporalRead(t, s, ctx, []string{a.ID, b.ID}, TemporalQuery{})
	if len(current.Items) != 1 || current.Items[0].Claim.ID != b.ID || current.Items[0].Conflict {
		t.Fatal("historical conflict rewrote current owner head", current)
	}
}
