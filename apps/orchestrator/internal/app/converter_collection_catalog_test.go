package app

import (
	"testing"

	"github.com/butler/butler/internal/memory/catalog"
	"github.com/butler/butler/internal/ownership"
)

func TestConverterCollectionCatalogRequiresActualReturnIntegration(t *testing.T) {
	f := newDeveloperFixture(t)
	current, err := f.env.GetActionContext(f.f.ctx, f.parentRun)
	if err != nil {
		t.Fatal(err)
	}
	check := func(expected bool) {
		t.Helper()
		ready, e := f.converters.CollectionAvailable(f.f.ctx, current)
		if e != nil || ready != expected {
			t.Fatal("current collection phase mismatch", ready, e)
		}
	}
	check(false)
	child, worker, candidate := f.testedChild(t)
	check(false)
	result := f.finishChild(t, child, worker)
	check(false) // Final alone is insufficient before immutable054 publication.
	collected, err := f.service.Manager.CollectGenerated(f.f.ctx, f.f.parent.TaskID, child.TaskID, result.ResultID)
	if err != nil || collected.Report == nil || collected.Contract.State != "result_ready" {
		t.Fatal("actual return publication failed", err)
	}
	check(true)
	// Catalog observation must not grant the package to the parent.
	parentCtx := ownership.WithToken(f.f.ctx, f.f.leases.Authority(), ownership.Token{SessionKey: current.SessionKey, RunID: current.RunID, LeaseID: current.LeaseID, FenceEpoch: current.LeaseEpoch})
	tx, err := f.f.pool.Begin(parentCtx)
	if err != nil {
		t.Fatal(err)
	}
	_, err = f.f.sources.AccessTx(parentCtx, tx, []catalog.SourceRef{candidate.BodySource}, "memory_read")
	_ = tx.Rollback(parentCtx)
	if err == nil {
		t.Fatal("catalog observation granted candidate DATA")
	}
	foreign := current
	foreign.TaskID = child.TaskID
	if ready, e := f.converters.CollectionAvailable(f.f.ctx, foreign); e == nil || ready {
		t.Fatal("foreign task catalog accepted")
	}
	stale := current
	stale.LeaseEpoch++
	if ready, e := f.converters.CollectionAvailable(f.f.ctx, stale); e == nil || ready {
		t.Fatal("stale lease catalog accepted")
	}
	f.deleteSource(t, catalog.SourceRef(*collected.Report))
	check(false)
}
