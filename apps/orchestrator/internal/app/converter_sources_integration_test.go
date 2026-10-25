package app

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/butler/butler/internal/generatedtools"
	"github.com/butler/butler/internal/ownership"
)

func TestConverterCompoundSourceProofDoesNotBorrowAuthorityReserveIntegration(t *testing.T) {
	profile := generatedtools.Profile{Dependency: generatedtools.Dependency{ID: "butler-converter-ir", Version: "1.0.0", SHA256: strings.Repeat("a", 64)}, Checker: generatedtools.Checker}
	f := newDeveloperWorkerFixture(t, profile, "", true)
	f.dispatch(t, f.parentRun, "converters.discover", map[string]any{"source_ref": f.needRef})
	f.dispatch(t, f.parentRun, generatedtools.DeveloperAdmissionTool, f.args())
	token := ownership.Token{SessionKey: f.f.parent.SessionKey, RunID: f.parentRun}
	if err := f.f.pool.QueryRow(f.f.ctx, `SELECT lease_id,fence_epoch FROM session_ownership WHERE run_id=$1`, f.parentRun).Scan(&token.LeaseID, &token.FenceEpoch); err != nil {
		t.Fatal(err)
	}
	owned := ownership.WithToken(f.f.ctx, nil, token)
	reserved := f.f.sources.RunObservationPool(owned)
	if reserved == f.f.pool || reserved.Config().MaxConns != 1 {
		t.Fatal("separate one-connection reserve required")
	}
	connection, err := reserved.Acquire(f.f.ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer connection.Release()
	ctx, cancel := context.WithTimeout(owned, time.Second)
	defer cancel()
	sources := converterSources{f.f.sources, f.f.pool}
	if err = sources.CheckRunSources(ctx, f.parentRun); err != nil {
		t.Fatal("compound source proof borrowed occupied observer reserve", err)
	}
	if err = sources.CheckRunSources(ctx, "unowned-run"); err == nil {
		t.Fatal("pool selection bypassed current run ownership")
	}
}
