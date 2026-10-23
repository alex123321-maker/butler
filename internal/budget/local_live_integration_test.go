package budget

import (
	"context"
	"encoding/json"
	"errors"
	"sync/atomic"
	"testing"
	"time"

	"github.com/butler/butler/internal/security/content"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

func TestLocalLiveAuthoritySurvivesBusinessPoolSaturationIntegration(t *testing.T) {
	for _, name := range []string{"shared", "task_revoked", "lease_expired", "source_revoked", "observer_unavailable"} {
		t.Run(name, func(t *testing.T) {
			reserved := name != "shared"
			pool := ownerDayBudgetDB(t)
			cfg := pool.Config().Copy()
			cfg.MaxConns = 1
			reads, err := pgxpool.NewWithConfig(context.Background(), cfg)
			if err != nil {
				t.Fatal(err)
			}
			t.Cleanup(reads.Close)
			p := testPolicy()
			p.Models["local:google/embeddinggemma-2"] = ModelPrice{Basis: "local_no_billing"}
			c := NewController(pool, p)
			if reserved {
				c = NewControllerWithAuthorityReads(pool, reads, p)
			}
			c.SetAuthorityResolver(func(context.Context, pgx.Tx, string, string) (MandateScope, error) { return MandateScope{}, nil })
			target, err := NewLocalTarget("local-models:9090", "test-pinned")
			if err != nil {
				t.Fatal(err)
			}
			var sourceRevoked atomic.Bool
			access := func(context.Context, pgx.Tx) error {
				if sourceRevoked.Load() {
					return errors.New("private source/SQL material")
				}
				return nil
			}
			call, release, err := c.AdmitLocalModel(modelCtx("run"), "run", target, LocalOperation{Kind: "query", Items: 1, InputBytes: 16, Timeout: 10 * time.Second, Origin: content.Derive(content.MemoryDerived)}, access)
			if err != nil {
				t.Fatal(err)
			}
			defer release()
			before, err := c.Ledger.SnapshotOwner(context.Background(), "owner")
			if err != nil {
				t.Fatal(err)
			}
			var held []*pgxpool.Conn
			for range pool.Config().MaxConns {
				conn, e := pool.Acquire(context.Background())
				if e != nil {
					t.Fatal(e)
				}
				held = append(held, conn)
				defer conn.Release()
			}
			if !reserved {
				select {
				case <-call.Done():
				case <-time.After(2 * time.Second):
					t.Fatal("shared-pool counterfactual did not cancel")
				}
				if got := LocalCallError(call).Error(); got != "local_authority_pool_timeout" {
					t.Fatal("missing safe pool diagnostic", got)
				}
				return
			}
			select {
			case <-call.Done():
				t.Fatalf("local query cancelled by business pool occupancy: %v", call.Err())
			case <-time.After(750 * time.Millisecond):
			}
			// Returned-result acceptance keeps its business transaction: the
			// source callback may commit applicability pins. Free one slot only.
			held[0].Release()
			check, stop := context.WithTimeout(modelCtx("run"), time.Second)
			err = c.CheckLocalAuthority(check, "run", access)
			stop()
			if err != nil {
				t.Fatal("returned-result authority failed", err)
			}
			held[0], err = pool.Acquire(context.Background())
			if err != nil {
				t.Fatal(err)
			}
			defer held[0].Release()
			// Actual revocation must still stop the local operation while all
			// business connections remain occupied; no budget refund is involved.
			expected := "local_authority_authority_denied"
			switch name {
			case "source_revoked":
				sourceRevoked.Store(true)
				expected = "local_authority_source_unavailable"
			case "observer_unavailable":
				observer, e := reads.Acquire(context.Background())
				if e != nil {
					t.Fatal(e)
				}
				defer observer.Release()
				expected = "local_authority_pool_timeout"
			default:
				revoke := `UPDATE tasks SET status='paused' WHERE task_id='root'`
				if name == "lease_expired" {
					revoke = `UPDATE session_ownership SET expires_at=clock_timestamp()-INTERVAL '1 second'`
				}
				if _, err = held[0].Exec(context.Background(), revoke); err != nil {
					t.Fatal(err)
				}
			}
			select {
			case <-call.Done():
			case <-time.After(2 * time.Second):
				t.Fatal("revoked task did not cancel local query")
			}
			if got := LocalCallError(call).Error(); got != expected {
				t.Fatal("missing safe revocation diagnostic", got)
			}
			for _, conn := range held {
				conn.Release()
			}
			after, err := c.Ledger.SnapshotOwner(context.Background(), "owner")
			if err != nil {
				t.Fatal(err)
			}
			b, _ := json.Marshal(before)
			a, _ := json.Marshal(after)
			if string(a) != string(b) {
				t.Fatal("authority observations changed budget history")
			}
		})
	}
}
