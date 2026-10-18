package catalog

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/butler/butler/internal/ownership"
	"github.com/jackc/pgx/v5"
)

func TestCapturedOrdinaryRunGrantsAtFinalPublicationIntegration(t *testing.T) {
	for _, mode := range []string{"accepted", "ancestor_grant", "ancestor_pin", "owner", "task_revision", "task_session", "expired_lease", "wrong_lease", "wrong_fence", "missing_token", "wrong_run"} {
		t.Run(mode, func(t *testing.T) {
			s, owner := fixture(t)
			live := seedRun(t, s)
			if _, err := s.pool.Exec(owner, `INSERT INTO tasks(task_id,session_key,owner_id,goal) VALUES('current','web:owner','owner','read permitted DATA');UPDATE runs SET task_id='current',task_revision=1 WHERE run_id='run'`); err != nil {
				t.Fatal(err)
			}
			original, err := s.Put(owner, PutSource{ExternalKey: "ordinary-project", Kind: "document", ScopeType: "project", ScopeID: "owner", Body: "ordinary original DATA"})
			if err != nil {
				t.Fatal(err)
			}
			selected, err := s.Put(owner, PutSource{ExternalKey: "ordinary-derived", Kind: "model_candidate", ScopeType: "user", ScopeID: "owner", Body: original.Body, Parents: []SourceRef{ref(original)}})
			if err != nil {
				t.Fatal(err)
			}
			if err = s.GrantSource(owner, "current", original.ID, "memory_read", 1); err != nil {
				t.Fatal(err)
			}
			if _, err = s.Get(live, selected.ID); err != nil {
				t.Fatal("ordinary current read failed", err)
			}
			if _, err = s.pool.Exec(owner, `UPDATE runs SET current_state='finalizing' WHERE run_id='run'`); err != nil {
				t.Fatal(err)
			}
			checkCtx := live
			switch mode {
			case "ancestor_grant":
				_, err = s.pool.Exec(owner, `DELETE FROM memory_scope_grants WHERE task_id='current' AND source_id=$1`, original.ID)
			case "ancestor_pin":
				_, err = s.pool.Exec(owner, `DELETE FROM memory_run_sources WHERE run_id='run' AND source_id=$1`, original.ID)
			case "owner":
				_, err = s.pool.Exec(owner, `UPDATE tasks SET owner_id='other' WHERE task_id='current'`)
			case "task_revision":
				_, err = s.pool.Exec(owner, `UPDATE tasks SET revision=revision+1,constraints_revision=constraints_revision+1 WHERE task_id='current'`)
			case "task_session":
				_, err = s.pool.Exec(owner, `INSERT INTO sessions(session_key,user_id,channel) VALUES('other-session','owner','web');UPDATE tasks SET session_key='other-session' WHERE task_id='current'`)
			case "expired_lease":
				_, err = s.pool.Exec(owner, `UPDATE session_ownership SET expires_at=clock_timestamp()-INTERVAL '1 second' WHERE run_id='run'`)
			case "wrong_lease", "wrong_fence", "wrong_run":
				token, _ := ownership.TokenFromContext(live)
				switch mode {
				case "wrong_lease":
					token.LeaseID = "other-lease"
				case "wrong_fence":
					token.FenceEpoch++
				case "wrong_run":
					token.RunID = "other-run"
				}
				checkCtx = ownership.WithToken(context.Background(), ownership.NewAuthority(s.pool), token)
			case "missing_token":
				checkCtx = context.Background()
			}
			if err != nil {
				t.Fatal("ordinary captured proof fault setup", err)
			}
			tx, err := s.pool.Begin(live)
			if err != nil {
				t.Fatal(err)
			}
			defer tx.Rollback(context.Background())
			// This existing guard deliberately proves source ACL/revision/privacy,
			// not a nondelegated task's ordinary source grant. Preserve that seam.
			if err = s.CheckRunSourcesTx(live, tx, "run"); err != nil {
				t.Fatal("unchanged ordinary source pins failed existing guard", err)
			}
			err = s.CheckCapturedRunGrantsTx(checkCtx, tx, "run")
			if mode == "accepted" && err != nil || mode != "accepted" && err == nil {
				t.Fatal("ordinary captured current grant proof", mode, err)
			}
			if mode == "accepted" {
				if _, err = s.AccessTx(live, tx, []SourceRef{ref(selected)}, "memory_read"); !errors.Is(err, ErrDenied) {
					t.Fatal("metadata-only check widened finalizing body access", err)
				}
			}
		})
	}
}

func TestCapturedRunLeaseExpiryAfterGrantWaitIntegration(t *testing.T) {
	s, owner := fixture(t)
	live, cancel := context.WithTimeout(seedRun(t, s), 10*time.Second)
	defer cancel()
	if _, err := s.pool.Exec(owner, `INSERT INTO tasks(task_id,session_key,owner_id,goal) VALUES('current','web:owner','owner','read permitted DATA');UPDATE runs SET task_id='current',task_revision=1 WHERE run_id='run'`); err != nil {
		t.Fatal(err)
	}
	src, err := s.Put(owner, PutSource{ExternalKey: "ordinary-expiring-grant", Kind: "document", ScopeType: "project", ScopeID: "owner", Body: "ordinary current DATA"})
	if err != nil {
		t.Fatal(err)
	}
	if err = s.GrantSource(owner, "current", src.ID, "memory_read", 1); err != nil {
		t.Fatal(err)
	}
	if _, err = s.Get(live, src.ID); err != nil {
		t.Fatal(err)
	}
	if _, err = s.pool.Exec(owner, `UPDATE runs SET current_state='finalizing' WHERE run_id='run';UPDATE session_ownership SET expires_at=clock_timestamp()+INTERVAL '2 seconds' WHERE run_id='run'`); err != nil {
		t.Fatal(err)
	}
	// Reproduce the actual Coordinator's outer SHARE, not only a stale token.
	guarded, release, err := ownership.Begin(live)
	if err != nil {
		t.Fatal(err)
	}
	defer release()
	worker, err := s.pool.Begin(guarded)
	if err != nil {
		t.Fatal(err)
	}
	defer worker.Rollback(context.Background())
	if err = s.CheckRunSourcesTx(guarded, worker, "run"); err != nil {
		t.Fatal(err)
	}
	// Both normal pool connections belong to that outer guard and its business
	// transaction. Keep the controlled blocker on its own explicit connection.
	conn, err := pgx.ConnectConfig(live, s.pool.Config().ConnConfig.Copy())
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close(context.Background())
	blocker, err := conn.Begin(live)
	if err != nil {
		t.Fatal(err)
	}
	defer blocker.Rollback(context.Background())
	if _, err = blocker.Exec(live, `SELECT source_id FROM memory_scope_grants WHERE task_id='current' AND source_id=$1 FOR UPDATE`, src.ID); err != nil {
		t.Fatal(err)
	}
	result := make(chan error, 1)
	pid := worker.Conn().PgConn().PID()
	go func() { result <- s.CheckCapturedRunGrantsTx(guarded, worker, "run") }()
	waited := false
	for {
		var waiting, expired bool
		if err = blocker.QueryRow(live, `SELECT COALESCE((SELECT wait_event_type='Lock' FROM pg_stat_activity WHERE pid=$1),FALSE),EXISTS(SELECT 1 FROM session_ownership WHERE run_id='run' AND expires_at<=clock_timestamp())`, pid).Scan(&waiting, &expired); err != nil {
			t.Fatal(err)
		}
		waited = waited || waiting
		if waited && expired {
			break
		}
		select {
		case err = <-result:
			t.Fatal("captured checker did not wait on its grant", err)
		default:
		}
		time.Sleep(10 * time.Millisecond)
	}
	if err = blocker.Rollback(live); err != nil {
		t.Fatal(err)
	}
	if err = <-result; !errors.Is(err, ErrDenied) {
		t.Fatal("captured checker released after its grant wait crossed lease expiry", err)
	}
}
