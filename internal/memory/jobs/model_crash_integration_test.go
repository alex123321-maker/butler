package jobs

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"

	"github.com/butler/butler/internal/budget"
	"github.com/butler/butler/internal/memory/catalog"
	"github.com/jackc/pgx/v5/pgxpool"
)

// Executed only by the owning test as a separate process on its isolated schema.
func TestModelCrashProcessHelper(t *testing.T) {
	phase := os.Getenv("BUTLER_ISSUE82_CRASH_PHASE")
	if phase == "" {
		return
	}
	ctx := context.Background()
	cfg, e := pgxpool.ParseConfig(os.Getenv("BUTLER_ISSUE82_CRASH_DSN"))
	if e != nil {
		os.Exit(21)
	}
	cfg.MaxConns = 2
	pool, e := pgxpool.NewWithConfig(ctx, cfg)
	if e != nil {
		os.Exit(22)
	}
	c := catalog.New(pool, catalog.NewFileManifest(os.Getenv("BUTLER_ISSUE82_CRASH_ANCHOR")))
	b := budget.NewController(pool, budget.DefaultPolicy())
	b.Policy.ModelMillis = 1000
	b.Policy.Models["antigravity:fixture"] = budget.ModelPrice{Basis: "subscription_no_overage"}
	s := New(pool, c, b, nil)
	w := &ModelWorker{Jobs: s, Provider: &modelFixtureProvider{}, Model: "fixture"}
	j, ok, e := s.ClaimKind(ctx, "model")
	if e != nil || !ok {
		os.Exit(23)
	}
	a, _, e := w.Admit(ctx, j)
	if e != nil {
		os.Exit(24)
	}
	if phase == "before_dispatch" {
		os.Exit(82)
	}
	if e = s.dispatchModel(ctx, j, a); e != nil {
		os.Exit(25)
	}
	if phase == "after_dispatch" {
		os.Exit(82)
	}
	if e = s.observeModel(ctx, a, nil, "crashed response"); e != nil {
		os.Exit(26)
	}
	// The test DB trigger blocks after an uncommitted derived insertion. The
	// parent kills this process there, exercising the real publication transaction.
	_, e = s.publishModel(ctx, j, a, "crashed response")
	if e != nil {
		os.Exit(27)
	}
	os.Exit(28)
}

func TestModelActualProcessCrashRecoveryIntegration(t *testing.T) {
	for _, phase := range []string{"before_dispatch", "after_dispatch", "before_commit"} {
		t.Run(phase, func(t *testing.T) {
			w, ctx, src := modelFixture(t)
			anchor := filepath.Join(t.TempDir(), "shared-anchor")
			m := catalog.NewFileManifest(anchor)
			if e := m.Initialize(); e != nil {
				t.Fatal(e)
			}
			// Empty test catalog has no deletion history. Provision the explicit shared
			// anchor once for BOTH processes before authorizing any background job.
			w.Jobs.catalog = catalog.New(w.Jobs.pool, m)
			q := policyRequest(src)
			if _, e := w.Jobs.CreateModelPolicy(ctx, q); e != nil {
				t.Fatal(e)
			}
			if e := w.Jobs.scheduleModelPolicy(ctx, q.ID); e != nil {
				t.Fatal(e)
			}
			var schema string
			if e := w.Jobs.pool.QueryRow(ctx, `SELECT current_schema()`).Scan(&schema); e != nil {
				t.Fatal(e)
			}
			if phase == "before_commit" {
				_, e := w.Jobs.pool.Exec(ctx, `CREATE FUNCTION issue82_crash_pause() RETURNS trigger LANGUAGE plpgsql AS $$ BEGIN IF NEW.source_kind='model_candidate' THEN PERFORM pg_sleep(30); END IF; RETURN NEW; END $$; CREATE TRIGGER issue82_crash_pause AFTER INSERT ON memory_sources FOR EACH ROW EXECUTE FUNCTION issue82_crash_pause()`)
				if e != nil {
					t.Fatal(e)
				}
			}
			child := exec.Command(os.Args[0], "-test.run=^TestModelCrashProcessHelper$")
			child.Env = append(os.Environ(), "BUTLER_ISSUE82_CRASH_PHASE="+phase, "BUTLER_ISSUE82_CRASH_DSN="+os.Getenv("BUTLER_TEST_MEMORY_POSTGRES_URL")+"&search_path="+schema+",public&application_name=issue82-crash-"+schema, "BUTLER_ISSUE82_CRASH_ANCHOR="+anchor)
			if e := child.Start(); e != nil {
				t.Fatal(e)
			}
			if phase == "before_commit" {
				found := false
				until := time.Now().Add(10 * time.Second)
				for time.Now().Before(until) {
					var blocked bool
					_ = w.Jobs.pool.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM pg_stat_activity WHERE application_name=$1 AND wait_event='PgSleep')`, "issue82-crash-"+schema).Scan(&blocked)
					if blocked {
						found = true
						break
					}
					time.Sleep(25 * time.Millisecond)
				}
				_ = child.Process.Kill()
				_ = child.Wait()
				if !found {
					t.Fatal("child never entered atomic publication fault")
				}
				if _, e := w.Jobs.pool.Exec(ctx, `DROP TRIGGER issue82_crash_pause ON memory_sources; DROP FUNCTION issue82_crash_pause()`); e != nil {
					t.Fatal(e)
				}
			} else {
				e := child.Wait()
				exit, ok := e.(*exec.ExitError)
				if !ok || exit.ExitCode() != 82 {
					t.Fatal("wrong crash boundary", e)
				}
			}
			// Accelerate the persisted two-minute lease timeout; no authority is renewed.
			if _, e := w.Jobs.pool.Exec(ctx, `UPDATE memory_aux_jobs SET claim_until=clock_timestamp()-INTERVAL '1 second' WHERE policy_id=$1`, q.ID); e != nil {
				t.Fatal(e)
			}
			worked, e := w.ProcessOne(ctx)
			if e != nil || !worked {
				t.Fatal(worked, e)
			}
			var attempts, unknown, outputs int
			if e = w.Jobs.pool.QueryRow(ctx, `SELECT count(*),count(*) FILTER(WHERE usage IS NULL),(SELECT count(*) FROM memory_sources WHERE source_kind='model_candidate') FROM memory_model_attempts`).Scan(&attempts, &unknown, &outputs); e != nil || attempts != 2 || unknown != 1 || outputs != 1 {
				t.Fatal(attempts, unknown, outputs, e)
			}
			t.Log("killed independent worker", phase, "; persisted reservation retained; one derived output after takeover")
		})
	}
}
