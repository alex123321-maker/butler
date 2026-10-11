package broker

import (
	"context"
	"encoding/json"
	"sync/atomic"
	"testing"
	"time"

	"github.com/butler/butler/internal/resourceexec"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

func TestSingleTabResourceOwnerControlActualBrokerSerialOrder(t *testing.T) {
	for _, scenario := range []string{"claim_before_revoke", "revoke_before_claim", "selection_replay_before_claim"} {
		t.Run(scenario, func(t *testing.T) {
			ctx := context.Background()
			var armed atomic.Bool
			entered := make(chan uint32, 1)
			release := make(chan struct{})
			pause := func(ctx context.Context, tx pgx.Tx, _ string) error {
				if armed.CompareAndSwap(true, false) {
					entered <- tx.Conn().PgConn().PID()
					select {
					case <-release:
					case <-ctx.Done():
						return ctx.Err()
					}
				}
				return nil
			}
			backend := &singleTabPageBackend{}
			f := singleTabBrokerWithGuard(t, backend, nil, pause)
			config := f.pool.Config()
			config.MaxConns = 1
			monitor, e := pgxpool.NewWithConfig(ctx, config)
			if e != nil {
				t.Fatal(e)
			}
			defer monitor.Close()
			owner, binding := singleTabOwnerContext()
			var approval, token string
			if f.pool.QueryRow(ctx, `SELECT request_id,candidate_token FROM guarded_single_tab_selections WHERE session_id=$1`, f.selectedTab).Scan(&approval, &token) != nil {
				t.Fatal("own selection missing")
			}
			request := singleTabRequest(t, f, "serial-observe", "single_tab.status", `{"session_id":"approved-user-tab"}`)
			brokerDone := make(chan error, 1)
			controlDone := make(chan error, 1)
			runBroker := func() { _, e := f.server.ExecuteToolCall(ctx, request); brokerDone <- e }
			runControl := func() {
				if scenario == "selection_replay_before_claim" {
					_, e := f.selection.Select(owner, binding, approval, token)
					controlDone <- e
				} else {
					controlDone <- f.selection.Revoke(owner, binding, approval)
				}
			}
			armed.Store(true)
			if scenario == "claim_before_revoke" {
				go runBroker()
			} else {
				go runControl()
			}
			var blocker uint32
			select {
			case blocker = <-entered:
			case <-time.After(3 * time.Second):
				t.Fatal("current authority barrier not reached")
			}
			// Even failure paths must release the deterministic fixture barrier.
			released := false
			defer func() {
				if !released {
					close(release)
				}
			}()
			if scenario == "claim_before_revoke" {
				go runControl()
			} else {
				go runBroker()
			}
			until := time.Now().Add(3 * time.Second)
			for {
				var waiting bool
				e = monitor.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM pg_stat_activity a WHERE a.datname=current_database() AND $1=ANY(pg_blocking_pids(a.pid)) AND a.wait_event_type='Lock')`, blocker).Scan(&waiting)
				if e != nil {
					t.Fatal(e)
				}
				if waiting {
					break
				}
				if time.Now().After(until) {
					t.Fatal("second actual controller/Broker did not wait on owning barrier")
				}
				time.Sleep(5 * time.Millisecond)
			}
			close(release)
			released = true
			for name, done := range map[string]<-chan error{"broker": brokerDone, "owner": controlDone} {
				select {
				case e = <-done:
					if e != nil {
						t.Fatal("actual serial operation failed", name, e)
					}
				case <-time.After(5 * time.Second):
					t.Fatal("owning wait did not resolve", name)
				}
			}
			var reservations, mappings int
			if f.pool.QueryRow(ctx, `SELECT (SELECT count(*) FROM execution_budget_reservations),(SELECT count(*) FROM execution_resource_dispatches WHERE single_tab_binding IS NOT NULL)`).Scan(&reservations, &mappings) != nil {
				t.Fatal("atomic counters missing")
			}
			expected := 1
			if scenario == "revoke_before_claim" {
				expected = 0
			}
			if reservations != expected || mappings != expected {
				t.Fatal("wrong actual commit order", reservations, mappings, expected)
			}
			if scenario == "revoke_before_claim" && backend.calls.Load() != 0 {
				t.Fatal("revoked tab reached runtime")
			}
			t.Logf("actual pool2 owner/Broker serial order=%s reservations=%d no-timeout-success", scenario, reservations)
		})
	}
}

func TestSingleTabResourceGuardedMappingCannotBeLegacyOrMutated(t *testing.T) {
	f := singleTabBroker(t, &singleTabPageBackend{}, nil)
	ctx := context.Background()
	if _, e := f.pool.Exec(ctx, `UPDATE guarded_single_tab_selections SET origin='https://foreign.invalid' WHERE session_id=$1`, f.selectedTab); e == nil {
		t.Fatal("selected origin mutated")
	}
	if _, e := f.pool.Exec(ctx, `UPDATE guarded_single_tab_requests SET expires_at=expires_at+INTERVAL '1 second'`); e == nil {
		t.Fatal("authority extended")
	}
	if _, e := f.pool.Exec(ctx, `UPDATE approvals SET tool_name='single_tab.bind'`); e != nil {
		t.Fatal(e)
	}
	req := singleTabRequest(t, f, "legacy-denied", "single_tab.status", `{"session_id":"approved-user-tab"}`)
	out, e := f.server.ExecuteToolCall(ctx, req)
	if e != nil || out.GetResult().GetStatus() == "completed" {
		t.Fatal("legacy marker satisfied guard", e)
	}
	var count int
	f.pool.QueryRow(ctx, `SELECT count(*) FROM execution_budget_reservations`).Scan(&count)
	if count != 0 {
		t.Fatal("legacy selection consumed budget")
	}
}

func TestSingleTabResourceOwnerReplayExpiresDuringLegacySessionWait(t *testing.T) {
	f := singleTabBroker(t, &singleTabPageBackend{}, nil)
	ctx, b := singleTabOwnerContext()
	b.Deadline = time.Now().Add(time.Second)
	var approval, token string
	if f.pool.QueryRow(ctx, `SELECT request_id,candidate_token FROM guarded_single_tab_selections WHERE session_id=$1`, f.selectedTab).Scan(&approval, &token) != nil {
		t.Fatal("selection absent")
	}
	blocker, e := f.pool.Begin(ctx)
	if e != nil {
		t.Fatal(e)
	}
	defer blocker.Rollback(context.Background())
	if _, e = blocker.Exec(ctx, `SELECT single_tab_session_id FROM single_tab_sessions WHERE single_tab_session_id=$1 FOR UPDATE`, f.selectedTab); e != nil {
		t.Fatal(e)
	}
	cfg := f.pool.Config()
	cfg.MaxConns = 1
	monitor, e := pgxpool.NewWithConfig(ctx, cfg)
	if e != nil {
		t.Fatal(e)
	}
	defer monitor.Close()
	done := make(chan error, 1)
	go func() { _, e := f.selection.Select(ctx, b, approval, token); done <- e }()
	until := time.Now().Add(3 * time.Second)
	for {
		var waiting bool
		if e = monitor.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM pg_stat_activity WHERE $1=ANY(pg_blocking_pids(pid)) AND wait_event_type='Lock')`, blocker.Conn().PgConn().PID()).Scan(&waiting); e != nil {
			t.Fatal(e)
		}
		if waiting {
			break
		}
		if time.Now().After(until) {
			t.Fatal("owner replay did not reach legacy session row barrier")
		}
		time.Sleep(5 * time.Millisecond)
	}
	if _, e = blocker.Exec(ctx, `SELECT pg_sleep(1.1)`); e != nil {
		t.Fatal(e)
	}
	if e = blocker.Commit(ctx); e != nil {
		t.Fatal(e)
	}
	select {
	case e = <-done:
		if e == nil {
			t.Fatal("replayed owner binding ignored current deadline after legacy row wait")
		}
	case <-time.After(3 * time.Second):
		t.Fatal("expired replay did not return")
	}
}

func TestSingleTabResourceOwnerChoicesHaveNoNativeIdentity(t *testing.T) {
	f := singleTabBroker(t, &singleTabPageBackend{}, nil)
	ctx, b := singleTabOwnerContext()
	var approval string
	f.pool.QueryRow(ctx, `SELECT request_id FROM guarded_single_tab_selections WHERE session_id=$1`, f.selectedTab).Scan(&approval)
	if e := f.selection.Revoke(ctx, b, approval); e != nil {
		t.Fatal(e)
	}
	out, e := f.selection.Request(ctx, b)
	if e != nil {
		t.Fatal(e)
	}
	raw, _ := json.Marshal(out)
	var value map[string]any
	json.Unmarshal(raw, &value)
	if len(out.Candidates) != 1 || out.Candidates[0].Ordinal != 1 || len(out.Candidates[0].Token) != 48 {
		t.Fatal("bounded opaque choice missing")
	}
	if _, ok := value["browser_instance_id"]; ok {
		t.Fatal("native host exposed")
	}
	// Complete successful contract checked above and unknown identity rejection
	// remains independent of this owner-only DATA display.
	if _, e = resourceexec.SingleTabOrigin(out.Candidates[0].Origin); e != nil {
		t.Fatal(e)
	}
}
