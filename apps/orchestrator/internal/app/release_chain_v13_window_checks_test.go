package app

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/butler/butler/internal/memory/memorytest"
	"github.com/jackc/pgx/v5"
)

// Only synthetic admission rows are inserted. No provider, ordinary Core,
// tool/effect, invoice or measured-native-expense assertion is made here.
func TestReleaseChainV13DeclaredWindowAllocationIntegration(t *testing.T) {
	for _, name := range []string{"declared120_stored_exact_ceiling", "legacy80_stays_bounded", "unsupported_limits_or_deadline"} {
		t.Run(name, func(t *testing.T) {
			pool := memorytest.Database(t)
			ctx := context.Background()
			pin, history := strings.Repeat("a", 64), strings.Repeat("f", 64)
			var schema string
			if pool.QueryRow(ctx, `SELECT current_schema()`).Scan(&schema) != nil {
				t.Fatal("synthetic owning schema unavailable")
			}
			deadline := time.Now().UTC().Add(time.Minute)
			policy := releaseChainV9Policy()
			if name == "unsupported_limits_or_deadline" {
				for _, p := range [][2]int{{79, 79}, {81, 81}, {121, 120}, {120, 121}, {80, 81}, {120, 0}} {
					if _, err := createReleaseChainHighWindowWithDeclaredLimit(ctx, pool, pin, history, 575, schema, deadline, p[1], true, policy, p[0]); err == nil {
						t.Fatal("unreviewed declared bound or excessive/zero cap accepted")
					}
				}
				for _, end := range []time.Time{time.Now().UTC().Add(-time.Second), time.Now().UTC().Add(121 * time.Minute)} {
					if _, err := createReleaseChainHighWindow120WithPolicy(ctx, pool, pin, history, 575, schema, end, true, policy); err == nil {
						t.Fatal("expired or expanded global deadline accepted")
					}
				}
				var exists bool
				if pool.QueryRow(ctx, `SELECT to_regclass('fixture_chain_v5_high_window') IS NOT NULL`).Scan(&exists) != nil || exists {
					t.Fatal("rejected bootstrap created an owning window")
				}
				return
			}
			if name == "legacy80_stays_bounded" {
				if _, err := createReleaseChainHighWindowWithPolicy(ctx, pool, pin, history, 575, schema, deadline, 81, true, policy); err == nil {
					t.Fatal("legacy constructor accepted81")
				}
				if _, err := createReleaseChainHighWindowWithPolicy(ctx, pool, pin, history, 575, schema, deadline, 80, true, policy); err != nil {
					t.Fatal("legacy80 unavailable", err)
				}
				if _, err := createReleaseChainHighWindow120WithPolicy(ctx, pool, pin, history, 575, schema, deadline, true, policy); err == nil {
					t.Fatal("new allocation refilled an existing legacy scope")
				}
				var maximum, attempts int
				if pool.QueryRow(ctx, `SELECT maximum_new_attempts,attempts FROM fixture_chain_v5_high_window`).Scan(&maximum, &attempts) != nil || maximum != 80 || attempts != 575 {
					t.Fatal("legacy durable policy changed")
				}
				return
			}
			w, err := createReleaseChainHighWindow120WithPolicy(ctx, pool, pin, history, 575, schema, deadline, true, policy)
			if err != nil {
				t.Fatal("fresh fixed120 allocation rejected", err)
			}
			guard := func(context.Context, pgx.Tx, string) (bool, error) { return true, nil }
			legacy := *w
			legacy.maximum = 0
			if _, _, err := legacy.reserve(ctx, "normal_sent", "start", "synthetic-root", "synthetic-ledger-run", nil, guard); err == nil {
				t.Fatal("default80 reconstructed an unspent120 window")
			}
			for i := 0; i < 120; i++ {
				ordinal, _, err := w.reserve(ctx, "normal_sent", "start", "synthetic-root", "synthetic-ledger-run", i, guard)
				if err != nil || ordinal != 576+i {
					t.Fatal("finite declared120 admission counter wrong", i, ordinal, err)
				}
			}
			if _, _, err := w.reserve(ctx, "normal_sent", "start", "synthetic-root", "synthetic-ledger-run", 120, guard); err == nil {
				t.Fatal("121st entry charged")
			}
			reopened, err := createReleaseChainHighWindow120WithPolicy(ctx, pool, pin, history, 575, schema, deadline.Add(time.Minute), true, policy)
			if err != nil {
				t.Fatal("matching reconstruction unavailable", err)
			}
			if _, _, err := reopened.reserve(ctx, "restart_sent", "start", "other-root", "other-run", nil, guard); err == nil {
				t.Fatal("restart refunded spent120")
			}
			if _, err := createReleaseChainHighWindowWithPolicy(ctx, pool, pin, history, 575, schema, deadline, 80, true, policy); err == nil {
				t.Fatal("legacy constructor downgraded a120 scope")
			}
			for _, query := range []string{`UPDATE fixture_chain_v5_high_window SET attempts=575`, `UPDATE fixture_chain_v5_high_window SET attempts=attempts+1,maximum_new_attempts=121`, `UPDATE fixture_chain_v5_high_window SET attempts=attempts+1,expires_at=expires_at+interval '1 minute'`} {
				if _, err := pool.Exec(ctx, query); err == nil {
					t.Fatal("sealed allowance/history/deadline mutable")
				}
			}
			var maximum, attempts, rows int
			var stored time.Time
			var observation bool
			if pool.QueryRow(ctx, `SELECT maximum_new_attempts,attempts,expires_at,provider_stage_observation FROM fixture_chain_v5_high_window`).Scan(&maximum, &attempts, &stored, &observation) != nil || maximum != 120 || attempts != 695 || !stored.Equal(deadline.Truncate(time.Microsecond)) || !observation || pool.QueryRow(ctx, `SELECT count(*) FROM fixture_chain_v5_high_requests`).Scan(&rows) != nil || rows != 120 {
				t.Fatal("owning120/695 exact durable bound or deadline lost")
			}
		})
	}
}
