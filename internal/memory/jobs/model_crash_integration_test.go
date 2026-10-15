package jobs

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"testing"
	"time"

	"github.com/butler/butler/internal/budget"
	"github.com/butler/butler/internal/memory/catalog"
	"github.com/butler/butler/internal/transport"
	"github.com/jackc/pgx/v5/pgxpool"
)

// A controlled transport with an actual HTTP boundary in the child process.
// The parent observes complete request bytes before killing the child, while
// the response remains unknown. This does not contact a generative provider.
type modelCrashHTTPProvider struct {
	modelFixtureProvider
	endpoint string
}

func (p *modelCrashHTTPProvider) StartRun(ctx context.Context, r transport.StartRunRequest) (transport.EventStream, error) {
	if e := transport.CheckInferenceAuthority(ctx); e != nil {
		return nil, e
	}
	if e := transport.CheckInferenceDispatch(ctx); e != nil {
		return nil, e
	}
	data, e := json.Marshal(r)
	if e != nil {
		return nil, e
	}
	req, e := http.NewRequestWithContext(ctx, http.MethodPost, p.endpoint, bytes.NewReader(data))
	if e != nil {
		return nil, e
	}
	resp, e := http.DefaultClient.Do(req)
	if e != nil {
		return nil, e
	}
	resp.Body.Close()
	return nil, ErrClaim // Parent must kill the process before any response.
}

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
	if phase == "after_http_send" {
		b.Policy.ModelMillis = 5000
	}
	b.Policy.Models["antigravity:fixture"] = budget.ModelPrice{Basis: "subscription_no_overage"}
	s := New(pool, c, b, nil)
	w := &ModelWorker{Jobs: s, Provider: &modelFixtureProvider{}, Model: "fixture"}
	j, ok, e := s.ClaimKind(ctx, "model")
	if e != nil || !ok {
		os.Exit(23)
	}
	if phase == "after_http_send" {
		w.Provider = &modelCrashHTTPProvider{endpoint: os.Getenv("BUTLER_ISSUE82_CRASH_HTTP")}
		_ = w.execute(ctx, j, QuoteProbe{})
		os.Exit(29)
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
	for _, phase := range []string{"before_dispatch", "after_dispatch", "after_http_send", "before_commit"} {
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
			seen, release := make(chan struct{}), make(chan struct{})
			if phase == "after_http_send" {
				server := httptest.NewServer(http.HandlerFunc(func(rw http.ResponseWriter, r *http.Request) {
					body, e := io.ReadAll(r.Body)
					if e != nil || !json.Valid(body) || len(body) == 0 {
						t.Error("incomplete HTTP request")
					}
					close(seen)
					<-release
				}))
				defer server.Close()
				defer close(release)
				child.Env = append(child.Env, "BUTLER_ISSUE82_CRASH_HTTP="+server.URL)
			}
			if e := child.Start(); e != nil {
				t.Fatal(e)
			}
			killChild := func() {
				t.Helper()
				if e := child.Process.Kill(); e != nil {
					t.Fatal("child was not killed", e)
				}
				e := child.Wait()
				exit, ok := e.(*exec.ExitError)
				if !ok || runtime.GOOS != "windows" && exit.ProcessState.ExitCode() != -1 {
					t.Fatal("child did not terminate by signal", e)
				}
				t.Log("observed forced child exit", exit.ProcessState.String())
			}
			if phase == "after_http_send" {
				sent := false
				select {
				case <-seen:
					sent = true
				case <-time.After(10 * time.Second):
				}
				killChild()
				if !sent {
					t.Fatal("child never sent complete HTTP request")
				}
			} else if phase == "before_commit" {
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
				killChild()
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
			var attempts, reservations, unknown, outputs, chargedScopes int
			if e = w.Jobs.pool.QueryRow(ctx, `SELECT count(*),count(DISTINCT reservation_id),count(*) FILTER(WHERE usage IS NULL),(SELECT count(*) FROM memory_sources WHERE source_kind='model_candidate'),(SELECT count(*) FROM execution_budget_scopes WHERE kind IN ('owner_day','auxiliary') AND (used_json->>'model_turns')::bigint=2) FROM memory_model_attempts`).Scan(&attempts, &reservations, &unknown, &outputs, &chargedScopes); e != nil || attempts != 2 || reservations != 2 || unknown != 1 || outputs != 1 || chargedScopes != 3 {
				t.Fatal(attempts, reservations, unknown, outputs, chargedScopes, e)
			}
			t.Log("killed independent worker", phase, "; persisted reservation retained; one derived output after takeover")
		})
	}
}
