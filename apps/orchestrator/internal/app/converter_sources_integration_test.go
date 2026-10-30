package app

import (
	"context"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/butler/butler/internal/generatedtools"
	"github.com/butler/butler/internal/memory/catalog"
	"github.com/butler/butler/internal/ownership"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

type sourceTransactionTrace struct{ commits, rollbacks atomic.Int32 }

func (s *sourceTransactionTrace) TraceQueryStart(ctx context.Context, _ *pgx.Conn, q pgx.TraceQueryStartData) context.Context {
	switch strings.ToLower(strings.TrimSpace(q.SQL)) {
	case "commit":
		s.commits.Add(1)
	case "rollback":
		s.rollbacks.Add(1)
	}
	return ctx
}
func (*sourceTransactionTrace) TraceQueryEnd(context.Context, *pgx.Conn, pgx.TraceQueryEndData) {}

func TestConverterOrdinarySourceObservationRollsBackAndRechecksIntegration(t *testing.T) {
	profile := generatedtools.Profile{Dependency: generatedtools.Dependency{ID: "butler-converter-ir", Version: "1.0.0", SHA256: strings.Repeat("a", 64)}, Checker: generatedtools.Checker}
	f := newDeveloperWorkerFixture(t, profile, "", true)
	var token ownership.Token
	token.RunID, token.SessionKey = f.parentRun, f.f.parent.SessionKey
	if err := f.f.pool.QueryRow(f.f.ctx, `SELECT lease_id,fence_epoch FROM session_ownership WHERE run_id=$1`, f.parentRun).Scan(&token.LeaseID, &token.FenceEpoch); err != nil {
		t.Fatal(err)
	}
	trace := &sourceTransactionTrace{}
	cfg := f.f.pool.Config().Copy()
	cfg.MaxConns, cfg.MinConns = 1, 0
	cfg.ConnConfig.Tracer = trace
	reserved, err := pgxpool.NewWithConfig(f.f.ctx, cfg)
	if err != nil {
		t.Fatal(err)
	}
	defer reserved.Close()
	manifest := catalog.NewFileManifest(filepath.Join(t.TempDir(), "deletions"))
	if err = manifest.Initialize(); err != nil {
		t.Fatal(err)
	}
	sources := converterSources{catalog.NewWithRunObservationPool(f.f.pool, reserved, manifest), f.f.pool}
	owned := ownership.WithToken(f.f.ctx, nil, token)
	if err = sources.CheckRunSources(owned, f.parentRun); err != nil {
		t.Fatal(err)
	}
	if trace.commits.Load() != 0 || trace.rollbacks.Load() != 1 {
		t.Fatal("ordinary observation introduced a durable commit", trace.commits.Load(), trace.rollbacks.Load())
	}
	// Rollback is not cached permission: deny a newly revoked lease before
	// releasing more provider/tool data.
	if _, err = f.f.pool.Exec(f.f.ctx, `UPDATE session_ownership SET expires_at=clock_timestamp()-interval '1 second' WHERE run_id=$1`, f.parentRun); err != nil {
		t.Fatal(err)
	}
	if err = sources.CheckRunSources(owned, f.parentRun); err == nil {
		t.Fatal("revoked lease borrowed an earlier observation")
	}
	if trace.commits.Load() != 0 {
		t.Fatal("denied observation committed")
	}
}

func TestConverterCompoundSourceProofDoesNotBorrowAuthorityReserveIntegration(t *testing.T) {
	profile := generatedtools.Profile{Dependency: generatedtools.Dependency{ID: "butler-converter-ir", Version: "1.0.0", SHA256: strings.Repeat("a", 64)}, Checker: generatedtools.Checker}
	manifestPath := filepath.Join(t.TempDir(), "deletions")
	f := newDeveloperWorkerFixture(t, profile, manifestPath, true)
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
	trace := &sourceTransactionTrace{}
	cfg := f.f.pool.Config().Copy()
	cfg.ConnConfig.Tracer = trace
	business, err := pgxpool.NewWithConfig(f.f.ctx, cfg)
	if err != nil {
		t.Fatal(err)
	}
	defer business.Close()
	compound := converterSources{catalog.NewWithRunObservationPool(business, reserved, catalog.NewFileManifest(manifestPath)), business}
	if err = compound.CheckRunSources(ctx, f.parentRun); err != nil {
		t.Fatal(err)
	}
	if trace.commits.Load() != 1 || trace.rollbacks.Load() != 0 {
		t.Fatal("compound converter proof lost its owning commit", trace.commits.Load(), trace.rollbacks.Load())
	}
}
