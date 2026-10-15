package catalog

import (
	"encoding/json"
	"errors"
	"testing"
)

func TestTemporalRunObservationRejectsMetadataOnlyChangeAndLatePublicationIntegration(t *testing.T) {
	s, owner := fixture(t)
	live := seedRun(t, s)
	c := propose(t, s, owner, put(t, s, owner, "rule", "Пиши по-русски."), `"ru"`)
	var err error
	c, err = s.Decide(owner, Decision{ClaimID: c.ID})
	if err != nil {
		t.Fatal(err)
	}
	if _, err = s.SetTemporal(owner, TemporalUpdate{ClaimID: c.ID, ExpectedHeadRevision: 1, Validity: ValidityUnknown, Class: FreshnessStablePreference}); err != nil {
		t.Fatal(err)
	}
	page, err := s.ReadAcceptedFacts(live, []string{c.ID}, TemporalQuery{})
	if err != nil || len(page.Items) != 1 {
		t.Fatal(page, err)
	}
	if err = s.CheckRunSources(live, "run"); err != nil {
		t.Fatal(err)
	}
	// Only metadata changes: the accepted head and original bytes stay intact.
	if _, err = s.SetTemporal(owner, TemporalUpdate{ClaimID: c.ID, ExpectedRevision: 1, ExpectedHeadRevision: 1, Validity: ValidityUnknown, Class: FreshnessTechnicalFact}); err != nil {
		t.Fatal(err)
	}
	if err = s.CheckRunSources(live, "run"); !errors.Is(err, ErrStale) {
		t.Fatal("prepared run retained changed temporal fact", err)
	}
	if _, err = s.ReadAcceptedFacts(live, []string{c.ID}, TemporalQuery{}); !errors.Is(err, ErrStale) {
		t.Fatal("same run silently refreshed stale observation", err)
	}
	if _, err = s.pool.Exec(owner, `INSERT INTO messages(message_id,session_key,run_id,role,content) VALUES('late','web:owner','run','assistant','stale result')`); err == nil {
		t.Fatal("SQL late publication bypassed temporal guard")
	}
	// A correction from the owner remains receivable even after invalidation.
	if _, err = s.pool.Exec(owner, `INSERT INTO messages(message_id,session_key,run_id,role,content) VALUES('owner-new','web:owner','run','user','исправление')`); err != nil {
		t.Fatal(err)
	}
	if _, err = s.Delete(owner, c.SourceID, "delete", c.SourceRevision); err != nil {
		t.Fatal("privacy cleanup blocked by temporal fence", err)
	}
}

func TestTemporalPinBatchRollbackDoesNotLeavePartialProtectionIntegration(t *testing.T) {
	s, ctx := fixture(t)
	src := put(t, s, ctx, "one", "данные")
	c := propose(t, s, ctx, src, `"a"`)
	var err error
	c, err = s.Decide(ctx, Decision{ClaimID: c.ID})
	if err != nil {
		t.Fatal(err)
	}
	f := temporalRead(t, s, ctx, []string{c.ID}, TemporalQuery{}).Items[0]
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Rollback(ctx)
	d, err := s.CreateDerivedTx(ctx, tx, []SourceRef{ref(src)}, "memory_read", "view", "candidate DATA", json.RawMessage(`{}`))
	if err != nil {
		t.Fatal(err)
	}
	bad := f.Pin
	bad.ClaimID = "missing-claim"
	if err = bindTemporalPinsTx(ctx, tx, d.ID, []AcceptedFactPin{f.Pin, bad}, TemporalQuery{}); err == nil {
		t.Fatal("bad second pin admitted")
	}
	if err = tx.Commit(ctx); err != nil {
		t.Fatal("savepoint did not isolate rejected batch", err)
	}
	var count int
	if err = s.pool.QueryRow(ctx, `SELECT count(*) FROM memory_temporal_source_pins WHERE derived_source_id=$1`, d.ID).Scan(&count); err != nil || count != 0 {
		t.Fatal("partial temporal protection survived rejected batch", count, err)
	}
}
