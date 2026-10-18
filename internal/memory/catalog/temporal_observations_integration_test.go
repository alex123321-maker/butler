package catalog

import (
	"encoding/json"
	"errors"
	"strings"
	"testing"
)

func TestTemporalBoundObservationsExposeOnlyExplicitMetadataIntegration(t *testing.T) {
	s, owner := fixture(t)
	a := put(t, s, owner, "bound-a", "selected original A body")
	b := put(t, s, owner, "privacy-only-b", "unselected original B body")
	c := propose(t, s, owner, a, `"selected A claim value"`)
	var err error
	c, err = s.Decide(owner, Decision{ClaimID: c.ID})
	if err != nil {
		t.Fatal(err)
	}
	if _, err = s.SetTemporal(owner, TemporalUpdate{ClaimID: c.ID, ExpectedHeadRevision: 1, Validity: ValidityUnknown, Class: FreshnessStablePreference}); err != nil {
		t.Fatal(err)
	}
	other, err := s.Propose(owner, ClaimRequest{SourceID: b.ID, SourceRevision: 1, Key: "different", Value: json.RawMessage(`"unselected B claim value"`), Summary: "unselected semantic assertion"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err = s.Decide(owner, Decision{ClaimID: other.ID}); err != nil {
		t.Fatal(err)
	}
	page := temporalRead(t, s, owner, []string{c.ID}, TemporalQuery{})
	tx, err := s.pool.Begin(owner)
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Rollback(owner)
	parents := []SourceRef{ref(a), ref(b)}
	d, err := s.CreateDerivedTx(owner, tx, parents, "memory_read", "view", "selected DATA view", json.RawMessage(`{}`))
	if err != nil {
		t.Fatal(err)
	}
	if err = s.BindNewAcceptedFactPinsTx(owner, tx, d, parents, []AcceptedFactPin{page.Items[0].Pin}, TemporalQuery{}); err != nil {
		t.Fatal(err)
	}
	observed, err := s.ReadBoundTemporalObservationsTx(owner, tx, []SourceRef{ref(d)})
	if err != nil || len(observed.Items) != 1 || observed.Items[0].ClaimID != c.ID || observed.Items[0].OnSourceID != d.ID || observed.Items[0].Source.ID != a.ID || observed.Items[0].Temporal.Class != FreshnessStablePreference {
		t.Fatal(observed, err)
	}
	raw, _ := json.Marshal(observed)
	for _, hidden := range []string{"selected A claim value", "unselected", "original A body"} {
		if strings.Contains(string(raw), hidden) {
			t.Fatal("metadata enrichment released a parent value or body")
		}
	}
	direct, err := s.ReadAcceptedFactsForSourcesTx(owner, tx, []SourceRef{ref(d)}, TemporalQuery{})
	if err != nil || len(direct.Items) != 0 {
		t.Fatal("DATA derivative became its own accepted fact", direct, err)
	}
	if err = tx.Commit(owner); err != nil {
		t.Fatal(err)
	}
	if _, err = s.SetTemporal(owner, TemporalUpdate{ClaimID: c.ID, ExpectedRevision: 1, ExpectedHeadRevision: 1, Validity: ValidityUnknown, Class: FreshnessTechnicalFact}); err != nil {
		t.Fatal(err)
	}
	next, err := s.pool.Begin(owner)
	if err != nil {
		t.Fatal(err)
	}
	defer next.Rollback(owner)
	if _, err = s.ReadBoundTemporalObservationsTx(owner, next, []SourceRef{ref(d)}); !errors.Is(err, ErrStale) {
		t.Fatal("bound metadata silently refreshed after owner change", err)
	}
}
