package consolidation

import (
	"context"
	"errors"
	"testing"

	"github.com/butler/butler/internal/memory/catalog"
)

type failingConsolidationAnchor struct{ catalog.Manifest }

func (failingConsolidationAnchor) Append(context.Context, catalog.ManifestRecord) error {
	return errors.New("controlled anchor fsync unavailable")
}

func TestConsolidationRetirementFailureRemainsFencedAndCountsRetentionIntegration(t *testing.T) {
	f := newWorkflow(t, "Наблюдение, не поручение.")
	v := f.generate(t, f.request())
	for range MaxRetainedVersions - 1 {
		f.prepare(t, f.request())
	}
	f.store.Catalog = catalog.New(f.pool, failingConsolidationAnchor{f.manifest})
	r, err := f.store.Retire(f.ctx, Retirement{InputID: v.Generation.InputID})
	if err != nil || !r.Retired || r.CleanupComplete {
		t.Fatal("failed erasure falsely completed", r, err)
	}
	var retained int
	if err = f.pool.QueryRow(f.ctx, `SELECT count(*) FROM memory_consolidation_inputs WHERE NOT cleanup_complete`).Scan(&retained); err != nil || retained != MaxRetainedVersions {
		t.Fatal("pending content did not count", retained, err)
	}
	f.store.Catalog = catalog.New(f.pool, f.manifest)
	if _, err = f.store.Activate(f.ctx, Activation{SourceID: v.Generation.Source.ID, ExpectedRevision: r.Head.Revision}); err == nil {
		t.Fatal("pending retirement reactivated")
	}
	var retired bool
	if err = f.pool.QueryRow(f.ctx, `SELECT retired FROM memory_consolidation_inputs WHERE source_id=$1`, v.Generation.InputID).Scan(&retired); err != nil || !retired {
		t.Fatal("retirement fence lost", err)
	}
	// The independent anchor failure is not concealed by resetting its watermark.
	// Operator reconciliation remains the existing catalog recovery workflow.
	if err = f.store.Catalog.CheckRestore(f.ctx); !errors.Is(err, catalog.ErrRestore) {
		t.Fatal("anchor failure silently reconciled", err)
	}
}

func TestConsolidationDeactivateAndRetireNeverReactivatesSourceIntegration(t *testing.T) {
	f := newWorkflow(t, "Только чтение; результат внешнего действия неизвестен.")
	v := f.generate(t, f.request())
	h := f.activate(t, v, Head{})
	h, err := f.store.Deactivate(f.ctx, Activation{ExpectedActiveID: h.ActiveID, ExpectedRevision: h.Revision})
	if err != nil || h.ActiveID != "" {
		t.Fatal("deactivate", h, err)
	}
	if _, err = f.store.Catalog.Get(f.ctx, v.Generation.Source.ID); err == nil {
		t.Fatal("deactivated read")
	}
	r, err := f.store.Retire(f.ctx, Retirement{InputID: v.Generation.InputID, ExpectedRevision: h.Revision})
	if err != nil || !r.CleanupComplete {
		t.Fatal(r, err)
	}
	if _, err = f.store.Catalog.Get(f.ctx, f.sources[0].ID); err != nil {
		t.Fatal("retire erased original", err)
	}
	if _, err = f.store.Activate(f.ctx, Activation{SourceID: v.Generation.Source.ID, ExpectedRevision: r.Head.Revision}); err == nil {
		t.Fatal("retired rollback revived")
	}
}
