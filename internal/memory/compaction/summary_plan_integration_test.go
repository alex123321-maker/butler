package compaction

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/butler/butler/internal/memory/catalog"
	"github.com/butler/butler/internal/memory/memorytest"
)

// Keep one prepared query alive across publication, correction and migration
// rollback. A cached query plan must never become a cached currentness fact.
func TestSummaryCurrentnessPlanNeverCachesAcceptedFactsIntegration(t *testing.T) {
	pool := memorytest.Database(t)
	ctx := context.Background()
	down, err := os.ReadFile(filepath.Join("..", "..", "..", "migrations", "103_summary_currentness_plan.down.sql"))
	if err != nil {
		t.Fatal(err)
	}
	up, err := os.ReadFile(filepath.Join("..", "..", "..", "migrations", "103_summary_currentness_plan.up.sql"))
	if err != nil {
		t.Fatal(err)
	}
	reference := strings.Replace(string(down), "FUNCTION butler_memory_summary_sources_current(", "FUNCTION butler_memory_summary_old_reference(", 1)
	if _, err = pool.Exec(ctx, reference); err != nil {
		t.Fatal(err)
	}
	check := func(ids []string, want bool) {
		t.Helper()
		var old, current bool
		if err := pool.QueryRow(ctx, `SELECT butler_memory_summary_old_reference($1::text[]),butler_memory_summary_sources_current($1::text[])`, ids).Scan(&old, &current); err != nil || old != want || current != want {
			t.Fatalf("currentness ids=%v old=%v current=%v want=%v err=%v", ids, old, current, want, err)
		}
	}
	check(nil, true)
	check([]string{}, true)
	check([]string{"ordinary-source"}, true)
	t.Setenv("BUTLER_SUMMARY_NATIVE_MANIFEST", filepath.Join(t.TempDir(), "anchor"))
	// This helper always uses summaryProvider: no native inference is admitted.
	f := newConsumerFixture(t, pool)
	root := f.policy.Job.Sources[0].ID
	check([]string{root}, true)
	f.poll(t)
	if did, err := f.worker.ProcessOne(f.ctx); err != nil || !did {
		t.Fatal(did, err)
	}
	var result string
	if err = pool.QueryRow(ctx, `SELECT result_source_id FROM memory_model_attempts WHERE state='published'`).Scan(&result); err != nil {
		t.Fatal(err)
	}
	check([]string{root, result}, true)
	if _, err = f.cat.Correct(f.ctx, catalog.Correction{SourceID: "message:" + f.firstMessage(t), ExpectedRevision: 1, Body: "Поправка: только черновик, отправку отменить"}); err != nil {
		t.Fatal(err)
	}
	check([]string{root, result}, false)
	for _, migration := range [][]byte{down, up} {
		if _, err = pool.Exec(ctx, string(migration)); err != nil {
			t.Fatal(err)
		}
		check([]string{root, result}, false)
		check([]string{"ordinary-source"}, true)
	}
}
