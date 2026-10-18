package catalog

import (
	"context"
	"errors"
	"testing"
)

func TestCapturedOrdinaryRunGrantsAtFinalPublicationIntegration(t *testing.T) {
	for _, mode := range []string{"accepted", "ancestor_grant", "ancestor_pin", "owner", "task_revision", "task_session"} {
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
			err = s.CheckCapturedRunGrantsTx(live, tx, "run")
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
