package singletab

import (
	"context"
	"fmt"
	"os"
	"testing"
	"time"

	"github.com/butler/butler/internal/resources"
	"github.com/jackc/pgx/v5/pgxpool"
)

func TestSingleTabResourceOwningApprovalGuardActualPG(t *testing.T) {
	dsn := os.Getenv("BUTLER_TEST_POSTGRES_URL")
	if dsn == "" {
		t.Skip("requires isolated PostgreSQL")
	}
	ctx := context.Background()
	base, e := pgxpool.New(ctx, dsn)
	if e != nil {
		t.Fatal(e)
	}
	defer base.Close()
	schema := fmt.Sprintf("tabguard_%d", time.Now().UnixNano())
	if _, e = base.Exec(ctx, `CREATE SCHEMA `+schema); e != nil {
		t.Fatal(e)
	}
	defer base.Exec(ctx, `DROP SCHEMA `+schema+` CASCADE`)
	cfg := base.Config()
	cfg.MaxConns = 2
	cfg.ConnConfig.RuntimeParams["search_path"] = schema
	pool, e := pgxpool.NewWithConfig(ctx, cfg)
	if e != nil {
		t.Fatal(e)
	}
	defer pool.Close()
	_, e = pool.Exec(ctx, `CREATE TABLE approvals(approval_id TEXT PRIMARY KEY,status TEXT,approval_type TEXT,run_id TEXT,session_key TEXT);
 CREATE TABLE approval_tab_candidates(approval_id TEXT,internal_tab_ref TEXT,status TEXT,current_url TEXT);
 CREATE TABLE single_tab_sessions(single_tab_session_id TEXT PRIMARY KEY,session_key TEXT,created_by_run_id TEXT,approval_id TEXT,status TEXT,bound_tab_ref TEXT,browser_instance_id TEXT,released_at TIMESTAMPTZ,expires_at TIMESTAMPTZ,created_at TIMESTAMPTZ,current_url TEXT DEFAULT 'https://example.invalid');
 INSERT INTO approvals VALUES('selection','approved','browser_tab_selection','run','owner-session');
 ALTER TABLE approvals ADD COLUMN tool_name TEXT DEFAULT 'single_tab.bind.guarded', ADD COLUMN expires_at TIMESTAMPTZ DEFAULT clock_timestamp()+INTERVAL '15 minutes';
 ALTER TABLE approval_tab_candidates ADD COLUMN candidate_token TEXT DEFAULT 'choice';
 INSERT INTO approval_tab_candidates(approval_id,internal_tab_ref,status,current_url) VALUES('selection','7','selected','https://example.invalid');
 INSERT INTO single_tab_sessions(single_tab_session_id,session_key,created_by_run_id,approval_id,status,bound_tab_ref,browser_instance_id,released_at,expires_at,created_at) VALUES('tab','owner-session','run','selection','ACTIVE','7','fixed-browser',NULL,clock_timestamp()+INTERVAL '1 hour',clock_timestamp());
 CREATE TABLE guarded_single_tab_requests(request_id TEXT,state TEXT,expires_at TIMESTAMPTZ,owner_id TEXT,run_id TEXT,session_key TEXT,task_id TEXT,task_revision BIGINT,constraints_revision BIGINT,browser_instance TEXT);
 INSERT INTO guarded_single_tab_requests VALUES('selection','selected',clock_timestamp()+INTERVAL '15 minutes','owner','run','owner-session','task',1,1,'fixed-browser');
 CREATE TABLE guarded_single_tab_selections(session_id TEXT,request_id TEXT,candidate_token TEXT,tab_ref TEXT,origin TEXT);
 INSERT INTO guarded_single_tab_selections VALUES('tab','selection','choice','7','https://example.invalid');`)
	if e != nil {
		t.Fatal(e)
	}
	binding := resources.Binding{OwnerID: "owner", SessionKey: "owner-session", RunID: "run", TaskID: "task", TaskRevision: 1, ConstraintsRevision: 1, Deadline: time.Now().Add(10 * time.Minute)}
	for _, scenario := range []string{"valid", "opaque_id", "other_run", "other_session", "different_host", "candidate_cancelled", "invalid_origin", "approval_revoked", "session_released", "expired", "missing_binding_deadline", "expired_binding_deadline"} {
		t.Run(scenario, func(t *testing.T) {
			tx, e := pool.Begin(ctx)
			if e != nil {
				t.Fatal(e)
			}
			defer tx.Rollback(ctx)
			b := binding
			id, host := "tab", "fixed-browser"
			query := ""
			switch scenario {
			case "opaque_id":
				id = "unknown"
			case "other_run":
				b.RunID = "other-run"
			case "other_session":
				b.SessionKey = "other-session"
			case "different_host":
				host = "other-host"
			case "candidate_cancelled":
				query = `UPDATE approval_tab_candidates SET status='cancelled'`
			case "invalid_origin":
				query = `UPDATE approval_tab_candidates SET current_url='https://user:password@example.invalid/'`
			case "approval_revoked":
				query = `UPDATE approvals SET status='rejected'`
			case "session_released":
				query = `UPDATE single_tab_sessions SET released_at=clock_timestamp()`
			case "expired":
				query = `UPDATE single_tab_sessions SET expires_at=clock_timestamp()-INTERVAL '1 second'`
			case "missing_binding_deadline":
				b.Deadline = time.Time{}
			case "expired_binding_deadline":
				b.Deadline = time.Now().Add(-time.Second)
			}
			if query != "" {
				if _, e = tx.Exec(ctx, query); e != nil {
					t.Fatal(e)
				}
			}
			got, e := ResourceGuard(host)(ctx, tx, b, id)
			if scenario == "valid" {
				if e != nil || got.ID != "tab" || got.TabRef != "7" || got.Origin != "https://example.invalid" || got.ExpiresAt.After(time.Now().Add(16*time.Minute)) {
					t.Fatal("finite trusted selection absent", e)
				}
			} else if e == nil {
				t.Fatal("ID/host or stale selection granted rights", scenario)
			}
		})
	}
	for _, expiryScenario := range []string{"session_expires_during_candidate_lock_wait", "approval_expires_during_session_lock_wait"} {
		t.Run(expiryScenario, func(t *testing.T) {
			if expiryScenario == "approval_expires_during_session_lock_wait" {
				if _, e := pool.Exec(ctx, `UPDATE approvals SET expires_at=clock_timestamp()+INTERVAL '1 second' WHERE approval_id='selection'`); e != nil {
					t.Fatal(e)
				}
			}
			blocker, e := pool.Begin(ctx)
			if e != nil {
				t.Fatal(e)
			}
			defer blocker.Rollback(ctx)
			barrier := `SELECT approval_id FROM approval_tab_candidates WHERE approval_id='selection' FOR UPDATE`
			if expiryScenario == "approval_expires_during_session_lock_wait" {
				barrier = `SELECT single_tab_session_id FROM single_tab_sessions WHERE single_tab_session_id='tab' FOR UPDATE`
			}
			if _, e = blocker.Exec(ctx, barrier); e != nil {
				t.Fatal(e)
			}
			if expiryScenario == "session_expires_during_candidate_lock_wait" {
				if _, e = blocker.Exec(ctx, `UPDATE single_tab_sessions SET expires_at=clock_timestamp()+INTERVAL '1 second' WHERE single_tab_session_id='tab'`); e != nil {
					t.Fatal(e)
				}
			}
			reader, e := pool.Begin(ctx)
			if e != nil {
				t.Fatal(e)
			}
			defer reader.Rollback(ctx)
			pid := reader.Conn().PgConn().PID()
			done := make(chan error, 1)
			go func() { _, e := ResourceGuard("fixed-browser")(ctx, reader, binding, "tab"); done <- e }()
			until := time.Now().Add(3 * time.Second)
			for {
				var waiting bool
				if e = base.QueryRow(ctx, `SELECT COALESCE(wait_event_type='Lock',false) FROM pg_stat_activity WHERE pid=$1`, pid).Scan(&waiting); e != nil {
					t.Fatal(e)
				}
				if waiting {
					break
				}
				if time.Now().After(until) {
					t.Fatal("selection did not reach row barrier")
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
					t.Fatal("selection lock wait extended expired binding")
				}
			case <-time.After(3 * time.Second):
				t.Fatal("selection guard did not return")
			}
		})
	}
}
