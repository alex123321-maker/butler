package catalog

import (
	"context"
	"errors"
	"testing"
	"time"
)

func TestTemporalReadSerializesOwnerMetadataAndSupersessionIntegration(t *testing.T) {
	for _, kind := range []string{"metadata", "supersession", "source_correction"} {
		t.Run(kind, func(t *testing.T) {
			s, owner := fixture(t)
			ctx, stop := context.WithTimeout(owner, 8*time.Second)
			defer stop()
			src := put(t, s, ctx, "rule", "owner rule")
			a := propose(t, s, ctx, src, `"A"`)
			var err error
			a, err = s.Decide(ctx, Decision{ClaimID: a.ID})
			if err != nil {
				t.Fatal(err)
			}
			var b Claim
			if kind == "supersession" {
				b = propose(t, s, ctx, src, `"B"`)
			}
			tx, err := s.pool.Begin(ctx)
			if err != nil {
				t.Fatal(err)
			}
			defer tx.Rollback(ctx)
			page, err := s.ReadAcceptedFactsTx(ctx, tx, []string{a.ID}, TemporalQuery{})
			if err != nil || len(page.Items) != 1 {
				t.Fatal(page, err)
			}
			result := make(chan error, 1)
			go func() {
				switch kind {
				case "metadata":
					_, err := s.SetTemporal(ctx, TemporalUpdate{ClaimID: a.ID, ExpectedHeadRevision: 1, Validity: ValidityUnknown, Class: FreshnessStablePreference})
					result <- err
				case "supersession":
					_, err := s.Decide(ctx, Decision{ClaimID: b.ID, ExpectedActiveID: a.ID})
					result <- err
				default:
					_, err := s.Correct(ctx, Correction{SourceID: src.ID, ExpectedRevision: 1, Body: "corrected owner rule"})
					result <- err
				}
			}()
			// Observe a real PG lock waiter, rather than relying on an arbitrary sleep.
			for {
				var waiting bool
				if _, err = tx.Exec(ctx, `SELECT pg_stat_clear_snapshot()`); err != nil {
					t.Fatal(err)
				}
				if err = tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM pg_stat_activity WHERE datname=current_database() AND pid<>pg_backend_pid() AND wait_event_type='Lock' AND query LIKE '%memory_%')`).Scan(&waiting); err != nil {
					t.Fatal(err)
				}
				if waiting {
					break
				}
				select {
				case err := <-result:
					t.Fatal("owner mutation crossed retained read locks", err)
				default:
				}
				if _, err = tx.Exec(ctx, `SELECT pg_sleep(0.01)`); err != nil {
					t.Fatal(err)
				}
			}
			if err = tx.Commit(ctx); err != nil {
				t.Fatal(err)
			}
			if err = <-result; err != nil {
				t.Fatal("owner mutation deadlocked or failed after read released", err)
			}
			next, err := s.pool.Begin(ctx)
			if err != nil {
				t.Fatal(err)
			}
			defer next.Rollback(ctx)
			err = s.CheckAcceptedFactPinsTx(ctx, next, []AcceptedFactPin{page.Items[0].Pin}, TemporalQuery{})
			if err == nil || !errors.Is(err, ErrStale) && !errors.Is(err, ErrDenied) {
				t.Fatal("late pin proof ignored completed owner mutation", err)
			}
		})
	}
}
