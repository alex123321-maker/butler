package app

import (
	"context"
	"testing"
	"time"

	"github.com/butler/butler/internal/memory/memorytest"
)

// Exercise the actual pinned entry constructor before any provider exists.
// This pool belongs to preparation, separately from the eventual native window.
func TestReleaseChainV13PinnedEntryWindowBootstrapIntegration(t *testing.T) {
	pool := memorytest.Database(t)
	ctx := context.Background()
	t.Setenv("BUTLER_SCENARIO_CHAIN_V13_HIGH_DEADLINE", time.Now().UTC().Add(5*time.Minute).Format(time.RFC3339))
	w, err := newReleaseChainV13Window(ctx, pool)
	if err != nil {
		t.Fatal("actual pinned V13 entry cannot create its declared window")
	}
	if w.baseline != 575 || w.maximumNew() != 120 || !w.providerStages || !w.kernelOperations || w.policy != releaseChainV9Policy() || w.pin != releaseChainV13ReviewedPrerequisiteSHA || w.history != releaseChainV12HistoryFold {
		t.Fatal("pinned entry window configuration changed")
	}
	var attempts, maximum, requests int
	if err := pool.QueryRow(ctx, `SELECT attempts,maximum_new_attempts,(SELECT count(*) FROM `+w.table("fixture_chain_v5_high_requests")+`) FROM `+w.table("fixture_chain_v5_high_window")+` WHERE singleton`).Scan(&attempts, &maximum, &requests); err != nil || attempts != 575 || maximum != 120 || requests != 0 {
		t.Fatal("bootstrap window is not an unspent declared120 allocation")
	}
}
