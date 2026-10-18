package experience

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/butler/butler/internal/memory/catalog"
	"github.com/butler/butler/internal/memory/memorytest"
	"github.com/butler/butler/internal/owneridentity"
)

func TestExperienceStaleObservationAllowsOnlyOwningWorkingErasureIntegration(t *testing.T) {
	pool := memorytest.Database(t)
	_, file, _, _ := runtime.Caller(0)
	migration := func(suffix string) string {
		t.Helper()
		raw, err := os.ReadFile(filepath.Join(filepath.Dir(file), "../../../migrations/082_experience_working_erasure."+suffix+".sql"))
		if err != nil {
			t.Fatal(err)
		}
		return string(raw)
	}
	// Reproduce the old veto before applying the actual upgrade to populated data.
	if _, err := pool.Exec(t.Context(), migration("down")); err != nil {
		t.Fatal(err)
	}
	f := newLessonFixtureWithPool(t, pool, filepath.Join(t.TempDir(), "anchor"))
	live, _ := f.consumer(t)
	tx, err := pool.Begin(live)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = f.s.Sources.AccessTx(live, tx, []catalog.SourceRef{f.snapshot}, "memory_read"); err != nil {
		t.Fatal(err)
	}
	if err = tx.Commit(live); err != nil {
		t.Fatal(err)
	}
	if _, err = pool.Exec(f.ctx, `UPDATE tasks SET last_run_id='consumer' WHERE task_id='task-1'`); err != nil {
		t.Fatal(err)
	}
	put := func(key string) catalog.Source {
		t.Helper()
		src, err := f.s.Sources.Put(f.ctx, catalog.PutSource{ExternalKey: key, Kind: "document", ScopeType: "user", ScopeID: "owner", Body: "Unrelated bounded deletion sentinel"})
		if err != nil {
			t.Fatal(err)
		}
		return src
	}
	seed, later := put("erasure-seed"), put("erasure-later")
	if _, err = f.s.Sources.Delete(f.ctx, seed.ID, "delete", seed.Revision); err != nil {
		t.Fatal(err)
	}
	var did string
	if err = pool.QueryRow(f.ctx, `SELECT deletion_id FROM memory_deletions ORDER BY watermark DESC LIMIT 1`).Scan(&did); err != nil {
		t.Fatal(err)
	}
	other := owneridentity.WithActor(t.Context(), owneridentity.Actor{OwnerID: "other-owner", Channel: "web", DeliveryID: "foreign-deletion-control"})
	if _, err = pool.Exec(other, `INSERT INTO sessions(session_key,user_id,channel)VALUES('foreign-session','other-owner','web')`); err != nil {
		t.Fatal(err)
	}
	foreignSource, err := f.s.Sources.Put(other, catalog.PutSource{ExternalKey: "foreign-sentinel", Kind: "document", ScopeType: "user", ScopeID: "other-owner", Body: "Independent owner sentinel"})
	if err != nil {
		t.Fatal(err)
	}
	foreignDeletion, err := f.s.Sources.Delete(other, foreignSource.ID, "delete", foreignSource.Revision)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = pool.Exec(f.ctx, `UPDATE tool_calls SET result='{"corrected":true}' WHERE tool_call_id=$1`, f.observation.ToolPins[0].ID); err != nil {
		t.Fatal(err)
	}
	var stale bool
	if err = pool.QueryRow(f.ctx, `SELECT NOT butler_memory_experience_sources_current(ARRAY(SELECT source_id FROM memory_run_sources WHERE run_id='consumer'))`).Scan(&stale); err != nil || !stale {
		t.Fatal("actual original observation was not stale", err)
	}
	if _, err = f.s.Sources.Delete(f.ctx, f.parent.ID, "delete", f.parent.Revision); err == nil || !strings.Contains(err.Error(), "run experience tool observations changed") {
		t.Fatal("pre-upgrade erasure bug was not reproduced", err)
	}
	if _, err = pool.Exec(f.ctx, migration("up")); err != nil {
		t.Fatal(err)
	}
	canonical := `jsonb_build_object('plan','[]'::jsonb,'pending_actions','[]'::jsonb,'scratch',jsonb_build_object('privacy_redacted',TRUE,'deletion_id',$1::text))`
	for _, mode := range []string{"ordinary", "unknown_deletion", "foreign_deletion", "private_payload", "identity", "revision", "authorized_erasure"} {
		t.Run(mode, func(t *testing.T) {
			tx, err := pool.Begin(f.ctx)
			if err != nil {
				t.Fatal(err)
			}
			defer tx.Rollback(f.ctx)
			setting := did
			if mode == "unknown_deletion" {
				setting = "not-an-owning-deletion"
			}
			if mode == "foreign_deletion" {
				setting = foreignDeletion.ID
			}
			if mode != "ordinary" {
				if _, err = tx.Exec(f.ctx, `SELECT set_config('butler.memory_redaction',$1,TRUE)`, setting); err != nil {
					t.Fatal(err)
				}
			}
			value, extra := canonical, ""
			if mode == "private_payload" {
				value += ` || '{"private":"cannot publish under erasure"}'::jsonb`
			}
			if mode == "identity" {
				extra = `,goal='rewritten owner goal',last_run_id=''`
			}
			delta := "1"
			if mode == "revision" {
				delta = "2"
			}
			_, err = tx.Exec(f.ctx, `UPDATE tasks SET working_json=`+value+`,working_revision=working_revision+`+delta+extra+` WHERE task_id='task-1'`, setting)
			if mode == "authorized_erasure" {
				if err != nil {
					t.Fatal("exact authorized content removal was denied", err)
				}
			} else if err == nil {
				t.Fatal("erasure admitted an unauthorized or material publication")
			}
		})
	}
	// The failed old deletion already appended its fail-closed external anchor.
	// Replay it explicitly; never reset the anchor or invent a successful retry.
	for i := 0; i < 2; i++ {
		if err = f.s.Sources.Reconcile(f.ctx); err != nil {
			t.Fatal("pending deletion could not replay after upgrade", err)
		}
	}
	if _, err = f.s.Sources.Get(f.ctx, f.parent.ID); err == nil {
		t.Fatal("replayed original deletion revived content")
	}
	if _, err = f.s.Sources.Delete(f.ctx, later.ID, "delete", later.Revision); err != nil {
		t.Fatal("later actual owner deletion was vetoed", err)
	}
	if _, err = pool.Exec(f.ctx, `UPDATE tasks SET working_json='{"scratch":{"private":"late"}}' WHERE task_id='task-1'`); err == nil {
		t.Fatal("ordinary stale publication became permitted after deletion")
	}
	if _, err = pool.Exec(f.ctx, migration("down")); err == nil {
		t.Fatal("populated erasure protection was downgraded")
	}
}
