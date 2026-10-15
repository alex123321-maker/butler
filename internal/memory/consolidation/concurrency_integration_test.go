package consolidation

import (
	"errors"
	"sync"
	"testing"

	"github.com/butler/butler/internal/memory/catalog"
)

func TestConsolidationConcurrentOwnerCASAndLateGenerationNeverOverwriteHeadIntegration(t *testing.T) {
	f := newWorkflow(t, "Гипотеза не подтверждена; исключения действуют.")
	a := f.generate(t, f.request())
	b := f.generate(t, f.request())
	var wg sync.WaitGroup
	errs := make(chan error, 2)
	for _, v := range []Preview{a, b} {
		wg.Add(1)
		go func(v Preview) {
			defer wg.Done()
			_, err := f.store.Activate(f.ctx, Activation{SourceID: v.Generation.Source.ID})
			errs <- err
		}(v)
	}
	wg.Wait()
	close(errs)
	success, stale := 0, 0
	for err := range errs {
		if err == nil {
			success++
		} else if errors.Is(err, catalog.ErrStale) {
			stale++
		} else {
			t.Fatal(err)
		}
	}
	if success != 1 || stale != 1 {
		t.Fatal("concurrent overwrite", success, stale)
	}
	tx, err := f.pool.Begin(f.ctx)
	if err != nil {
		t.Fatal(err)
	}
	head, err := readHead(f.ctx, tx, "owner", a.Generation.ViewKey, false)
	_ = tx.Rollback(f.ctx)
	if err != nil || head.Revision != 1 {
		t.Fatal(head, err)
	}
	other := a
	if other.Generation.Source.ID == head.ActiveID {
		other = b
	}
	f.prepare(t, f.request())
	var chosen Head
	f.provider.during = func() {
		var e error
		chosen, e = f.store.Activate(f.ctx, Activation{SourceID: other.Generation.Source.ID, ExpectedActiveID: head.ActiveID, ExpectedRevision: head.Revision})
		if e != nil {
			t.Fatal(e)
		}
	}
	if did, err := f.process(t); err != nil || !did {
		t.Fatal(did, err)
	}
	tx, err = f.pool.Begin(f.ctx)
	if err != nil {
		t.Fatal(err)
	}
	current, err := readHead(f.ctx, tx, "owner", a.Generation.ViewKey, false)
	_ = tx.Rollback(f.ctx)
	if err != nil || current != chosen || current.Revision != 2 {
		t.Fatal("late model overwrote owner choice", current, chosen, err)
	}
	var base string
	var revision int64
	if err = f.pool.QueryRow(f.ctx, `SELECT base_source_id,base_revision FROM memory_consolidation_generations ORDER BY created_at DESC LIMIT 1`).Scan(&base, &revision); err != nil || base != chosen.ActiveID || revision != chosen.Revision {
		t.Fatal("late diff ignored current head", base, revision, err)
	}
}

func TestConsolidationChangedPreparedSnapshotRequiresFreshOwnerPolicyIntegration(t *testing.T) {
	f := newWorkflow(t, "Старое наблюдение. Исключение Б.")
	old := f.request()
	f.prepare(t, old)
	corrected, err := f.store.Catalog.Correct(f.ctx, catalog.Correction{SourceID: f.sources[0].ID, ExpectedRevision: 1, Body: "Исправленное наблюдение. Исключение В; прежнее условие Б отменено."})
	if err != nil {
		t.Fatal(err)
	}
	if did, err := f.process(t); err != nil || did || f.provider.calls.Load() != 0 {
		t.Fatal("scanner rebased old snapshot or invoked provider", did, err)
	}
	var reason string
	if err = f.pool.QueryRow(f.ctx, `SELECT reason FROM memory_model_policies WHERE policy_id=$1`, old.Policy.ID).Scan(&reason); err != nil || reason != "source_unavailable" {
		t.Fatal("stale policy reason missing", reason, err)
	}
	f.sources[0] = corrected
	fresh := f.generate(t, f.request())
	if fresh.Generation.PolicyID == old.Policy.ID || f.provider.calls.Load() != 1 {
		t.Fatal("fresh preparation reused stale authority")
	}
	f.activate(t, fresh, Head{})
}
