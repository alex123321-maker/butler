package catalog

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/jackc/pgx/v5/pgconn"
)

func TestTemporalCurrentnessPlansDoNotCacheEmptyOrAcceptedStateIntegration(t *testing.T) {
	s, owner := fixture(t)
	live := seedRun(t, s)
	down, err := os.ReadFile(filepath.Join("..", "..", "..", "migrations", "104_temporal_currentness_plans.down.sql"))
	if err != nil {
		t.Fatal(err)
	}
	up, err := os.ReadFile(filepath.Join("..", "..", "..", "migrations", "104_temporal_currentness_plans.up.sql"))
	if err != nil {
		t.Fatal(err)
	}
	reference := string(down)
	for _, name := range []string{"pins", "sources", "run"} {
		reference = strings.ReplaceAll(reference, "butler_memory_temporal_"+name+"_current", "butler_memory_temporal_"+name+"_reference")
	}
	if _, err = s.pool.Exec(owner, reference); err != nil {
		t.Fatal(err)
	}
	check := func(want bool) {
		t.Helper()
		var old, current bool
		if err := s.pool.QueryRow(owner, `SELECT butler_memory_temporal_run_reference($1),butler_memory_temporal_run_current($1)`, "run").Scan(&old, &current); err != nil || old != want || current != want {
			t.Fatal("old/current run predicate disagreement", old, current, want, err)
		}
		if err := s.pool.QueryRow(owner, `SELECT butler_memory_temporal_pins_reference(pins),butler_memory_temporal_pins_current(pins) FROM (SELECT COALESCE(jsonb_agg(to_jsonb(p)),'[]'::jsonb) pins FROM memory_temporal_run_pins p WHERE run_id=$1) q`, "run").Scan(&old, &current); err != nil || old != want || current != want {
			t.Fatal("old/current pin predicate disagreement", old, current, want, err)
		}
	}
	check(true) // An empty accepted-metadata state is observed, never cached.
	for _, raw := range []any{nil, "[]"} {
		var old, current bool
		if err = s.pool.QueryRow(owner, `SELECT butler_memory_temporal_pins_reference($1::jsonb),butler_memory_temporal_pins_current($1::jsonb)`, raw).Scan(&old, &current); err != nil || !old || !current {
			t.Fatal("empty/null predicate changed", err)
		}
	}
	for _, name := range []string{"reference", "current"} {
		var current bool
		err = s.pool.QueryRow(owner, `SELECT butler_memory_temporal_pins_`+name+`($1::jsonb)`, "{}").Scan(&current)
		var pg *pgconn.PgError
		if !errors.As(err, &pg) || pg.Code != "22023" {
			t.Fatal("invalid recordset became an empty authorization", name, err)
		}
	}
	c := propose(t, s, owner, put(t, s, owner, "rule", "Пиши по-русски."), `"ru"`)
	c, err = s.Decide(owner, Decision{ClaimID: c.ID})
	if err != nil {
		t.Fatal(err)
	}
	if _, err = s.SetTemporal(owner, TemporalUpdate{ClaimID: c.ID, ExpectedHeadRevision: 1, Validity: ValidityUnknown, Class: FreshnessStablePreference}); err != nil {
		t.Fatal(err)
	}
	if page, err := s.ReadAcceptedFacts(live, []string{c.ID}, TemporalQuery{}); err != nil || len(page.Items) != 1 {
		t.Fatal(page, err)
	}
	check(true)
	if _, err = s.SetTemporal(owner, TemporalUpdate{ClaimID: c.ID, ExpectedRevision: 1, ExpectedHeadRevision: 1, Validity: ValidityUnknown, Class: FreshnessTechnicalFact}); err != nil {
		t.Fatal(err)
	}
	check(false)
	for _, sql := range [][]byte{down, up} {
		if _, err = s.pool.Exec(owner, string(sql)); err != nil {
			t.Fatal(err)
		}
		check(false)
	}
}
