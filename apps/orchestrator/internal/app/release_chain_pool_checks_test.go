package app

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/butler/butler/apps/orchestrator/internal/session"
	"github.com/butler/butler/apps/orchestrator/internal/task"
	"github.com/butler/butler/internal/memory/catalog"
	"github.com/butler/butler/internal/ownership"
	"github.com/jackc/pgx/v5/pgxpool"
)

func TestReleaseChainV19FixturePoolProfile(t *testing.T) {
	for _, size := range []int32{-1, 0, 1, 3, 8, 11, 100} {
		if releaseChainFixturePoolAllowed(size) {
			t.Fatal("arbitrary fixture pool admitted", size)
		}
	}
	if !releaseChainFixturePoolAllowed(2) || !releaseChainFixturePoolAllowed(10) {
		t.Fatal("fixed legacy or production pool denied")
	}
}

// Hold the two connections available to the old native fixture. Real source,
// lease and task checks must expire on that exhausted legacy pool, but finish
// under their unchanged bounds with the explicit production-default headroom.
// This models pool exhaustion; it does not identify the historical holders.
func TestReleaseChainV19FixturePoolHeadroomIntegration(t *testing.T) {
	if os.Getenv("BUTLER_TEST_ACTIONAUTH_POSTGRES_URL") == "" {
		t.Skip("isolated actionauth PostgreSQL required")
	}
	for _, size := range []int32{2, 10} {
		name := "legacy_two"
		if size == 10 {
			name = "native_production_ten"
		}
		t.Run(name, func(t *testing.T) {
			observer := &releaseChainSourceObserver{}
			pool, _ := e2eDatabaseWithTracerAndPool(t, observer, size)
			if pool.Config().MaxConns != size {
				t.Fatal("pool setting not installed")
			}
			ctx, cancel := context.WithTimeout(context.Background(), 8*time.Second)
			defer cancel()
			_, err := pool.Exec(ctx, `INSERT INTO sessions(session_key,user_id,channel) VALUES('headroom-session','headroom-owner','web'); INSERT INTO runs(run_id,session_key,input_event_id,status,autonomy_mode,current_state,model_provider,started_at,updated_at) VALUES('headroom-run','headroom-session','headroom-input','model_running','mode_1','model_running','fixture',clock_timestamp(),clock_timestamp()); INSERT INTO session_ownership(session_key,run_id,lease_id,fence_epoch,expires_at) VALUES('headroom-session','headroom-run','headroom-lease',1,clock_timestamp()+INTERVAL '1 hour')`)
			if err != nil {
				t.Fatal("isolated owning setup unavailable", err)
			}
			manifest := catalog.NewFileManifest(filepath.Join(t.TempDir(), "manifest"))
			if err = manifest.Initialize(); err != nil {
				t.Fatal(err)
			}
			store := catalog.New(pool, manifest)
			live := ownership.WithToken(ctx, ownership.NewAuthority(pool), ownership.Token{SessionKey: "headroom-session", RunID: "headroom-run", LeaseID: "headroom-lease", FenceEpoch: 1})
			sources := releaseChainObservedSources{converterSources: converterSources{store, pool}, observer: observer}
			leases := releaseChainObservedLeases{PostgresLeaseManager: session.NewPostgresLeaseManager(pool), pool: pool, observer: observer}
			tasks := releaseChainObservedTasks{Store: task.NewStore(pool), pool: pool, observer: observer}
			var held []*pgxpool.Conn
			for range 2 {
				conn, e := pool.Acquire(ctx)
				if e != nil {
					t.Fatal(e)
				}
				held = append(held, conn)
				defer conn.Release()
			}
			checks := []func(context.Context) error{
				func(c context.Context) error { return sources.CheckRunSources(c, "headroom-run") },
				func(c context.Context) error { _, e := leases.GetLease(c, "headroom-lease"); return e },
				func(c context.Context) error { return tasks.ValidateRunAuthority(c, "headroom-run") },
			}
			errs := make([]error, len(checks))
			var group sync.WaitGroup
			for i, check := range checks {
				group.Add(1)
				go func(i int, check func(context.Context) error) {
					defer group.Done()
					bound := 750 * time.Millisecond
					if i == 0 {
						bound = 2 * time.Second
					}
					if size == 2 {
						bound = 80 * time.Millisecond
					}
					bounded, stop := context.WithTimeout(live, bound)
					defer stop()
					errs[i] = check(bounded)
				}(i, check)
			}
			group.Wait()
			for _, err := range errs {
				if size == 2 && !errors.Is(err, context.DeadlineExceeded) {
					t.Fatal("legacy saturation did not fail closed", err)
				}
				if size == 10 && err != nil {
					t.Fatal("headroom current check failed", err)
				}
			}
			if size == 2 {
				if len(observer.snapshot().Failed) != 3 {
					t.Fatal("legacy failures missing")
				}
				return
			}
			if len(observer.snapshot().Failed) != 0 {
				t.Fatal("healthy headroom failure invented")
			}
			// Additional capacity cannot authorize a revoked execution. The source
			// guard remains the production implementation and rechecks its lease.
			if _, err = pool.Exec(ctx, `UPDATE session_ownership SET expires_at=clock_timestamp()-INTERVAL '1 second' WHERE session_key='headroom-session'`); err != nil {
				t.Fatal(err)
			}
			bounded, stop := context.WithTimeout(live, 2*time.Second)
			defer stop()
			if err = sources.CheckRunSources(bounded, "headroom-run"); err == nil {
				t.Fatal("extra connections bypassed revoked lease")
			}
		})
	}
}
