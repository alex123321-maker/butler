package app

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strconv"
	"testing"
	"time"

	"github.com/butler/butler/internal/acceptance"
	"github.com/butler/butler/internal/config"
	"github.com/butler/butler/internal/providerauth"
	"github.com/jackc/pgx/v5/pgxpool"
)

const memoryAuthDSN = "postgresql://butler@memory-eval-postgres:5432/memory_auth_test?sslmode=disable"

type memoryNativeAuthTrace struct {
	RunID    string                               `json:"run_id"`
	InputSHA string                               `json:"input_sha256"`
	Events   []providerauth.AntigravityDiagnostic `json:"diagnostics"`
}

type memoryAuthProbe struct {
	Passed       bool                                 `json:"passed"`
	Milliseconds int64                                `json:"milliseconds"`
	Events       []providerauth.AntigravityDiagnostic `json:"events"`
	Expense      acceptance.MemoryExpense             `json:"expense"`
}
type memoryAuthReceipt struct {
	Started    time.Time         `json:"started_utc"`
	Finished   time.Time         `json:"finished_utc"`
	ModelCalls int               `json:"model_calls"`
	Probes     []memoryAuthProbe `json:"probes"`
}

// Separate fresh encrypted auth database, on the new window's own PG server.
// No transport provider, StartRun, Infer, secret output or old PG mutation.
func TestMemoryEvalNativeAuthPreflight(t *testing.T) {
	if os.Getenv("BUTLER_MEMORY_EVAL_AUTH_PREFLIGHT") != "1" {
		t.Skip("explicit new encrypted auth preflight required")
	}
	private := os.Getenv("BUTLER_MEMORY_EVAL_PRIVATE")
	if !filepath.IsAbs(private) {
		t.Fatal("private output required")
	}
	var pins acceptance.MemoryRuntimePins
	pinRaw, pinErr := acceptance.ReadPrivateFile("/metadata/pins.json", 1<<20)
	executable, exeErr := os.Executable()
	binary, binErr := acceptance.FileHash(executable)
	source, sourceErr := acceptance.TreeHash(shadowRepoRoot(), nil)
	if pinErr != nil || json.Unmarshal(pinRaw, &pins) != nil || exeErr != nil || binErr != nil || sourceErr != nil || pins.Prior == nil || pins.Binary != binary || pins.Source != source {
		t.Fatal("new auth source/binary pins denied")
	}
	suiteFile, err := os.Open(filepath.Join(shadowRepoRoot(), "testdata/acceptance/ru-memory-v2.json"))
	if err != nil {
		t.Fatal("suite unavailable")
	}
	suite, err := acceptance.LoadMemorySuite(suiteFile)
	_ = suiteFile.Close()
	if err != nil {
		t.Fatal("suite denied")
	}
	prior, err := acceptance.LoadMemoryPrior(suite, "/prior", *pins.Prior)
	if err != nil {
		t.Fatal("sealed prior denied")
	}
	plan := prior.Frozen.Plan
	plan.WindowID, plan.SourceSHA256, plan.CodeCommit, plan.HardwareSHA256 = pins.WindowID, pins.Source, pins.Release, pins.Hardware
	schedule, err := acceptance.MemoryPendingSchedule(prior, plan)
	if err != nil {
		t.Fatal("five-input schedule denied")
	}
	deadline, err := acceptance.ValidateMemoryRenewal(prior, pins, schedule, time.Now())
	if err != nil {
		t.Fatal(err)
	}
	var execution acceptance.MemoryExecutionProof
	var hardware struct {
		GoImage string `json:"go_image"`
	}
	ex, exErr := acceptance.ReadPrivateFile("/metadata/auth-execution.json", 1<<20)
	hw, hwErr := acceptance.ReadPrivateFile("/metadata/hardware.json", 1<<20)
	if exErr != nil || hwErr != nil || hashMemoryBytes(hw) != pins.Hardware || json.Unmarshal(hw, &hardware) != nil || json.Unmarshal(ex, &execution) != nil || acceptance.ValidateMemoryAuthExecution(execution, pins, hardware.GoImage) != nil {
		t.Fatal("auth actual container proof denied before credentials")
	}
	pool := memoryEvalDatabaseURL(t, memoryAuthDSN)
	ctx, stopDeadline := context.WithDeadline(context.Background(), deadline)
	defer stopDeadline()
	ctx, cancel := context.WithTimeout(ctx, 75*time.Second)
	defer cancel()
	if ctx.Err() != nil {
		t.Fatal("auth mandate expired before credentials")
	}
	key, err := privateNativeFixtureKey(filepath.Join(private, "settings.key"))
	if err != nil {
		t.Fatal("private auth key denied")
	}
	m := providerauth.NewManager(config.NewPostgresSettingsStore(pool, config.WithSettingsCodec(config.StaticSecretValueCodec{Key: key})))
	file, err := privateNativeFixtureFile(os.Getenv("BUTLER_MEMORY_EVAL_IMPORT_FILE"), 64<<10)
	if err != nil {
		t.Fatal("private auth import denied")
	}
	err = m.ImportAntigravity(ctx, file, "isolated-memory78-renewed-preflight")
	_ = file.Close()
	if err != nil {
		t.Fatal("encrypted import failed")
	}
	r := memoryAuthReceipt{Started: time.Now().UTC()}
	if _, err = pool.Exec(ctx, `CREATE TABLE fixture_memory_auth_admissions(id text PRIMARY KEY,window_id text NOT NULL,input_mask text NOT NULL,started_at timestamptz NOT NULL); CREATE TABLE fixture_memory_auth_observations(id text PRIMARY KEY REFERENCES fixture_memory_auth_admissions(id),record jsonb NOT NULL); CREATE FUNCTION fixture_memory_auth_immutable() RETURNS trigger LANGUAGE plpgsql AS $$ BEGIN RAISE EXCEPTION 'immutable auth receipt'; END $$; CREATE TRIGGER immutable_auth_admission BEFORE UPDATE OR DELETE ON fixture_memory_auth_admissions FOR EACH ROW EXECUTE FUNCTION fixture_memory_auth_immutable(); CREATE TRIGGER immutable_auth_observation BEFORE UPDATE OR DELETE ON fixture_memory_auth_observations FOR EACH ROW EXECUTE FUNCTION fixture_memory_auth_immutable();`); err != nil {
		t.Fatal("auth receipt ledger unavailable")
	}
	// A single operator preflight has at most three control-plane attempts. The
	// same Manager/PG retains any refresh rotation before later discovery fails.
	for n := 0; n < 3; n++ {
		probe := memoryAuthProbe{}
		begin := time.Now()
		id := pins.WindowID + "/auth-probe/" + strconv.Itoa(n+1)
		if _, err = pool.Exec(ctx, `INSERT INTO fixture_memory_auth_admissions VALUES($1,$2,$3,clock_timestamp())`, id, pins.WindowID, pins.Renewal.InputsSHA); err != nil {
			t.Fatal("auth probe admission persistence failed")
		}
		call := providerauth.WithAntigravityDiagnostics(ctx, func(d providerauth.AntigravityDiagnostic) { probe.Events = append(probe.Events, d) })
		_, resolveErr := m.ResolveAntigravityCloudCode(call)
		probe.Passed, probe.Milliseconds = resolveErr == nil, time.Since(begin).Milliseconds()
		zero := int64(0)
		outcome := "failed"
		if probe.Passed {
			outcome = "success"
		}
		probe.Expense = acceptance.MemoryExpense{ID: id, CaseID: schedule.Inputs[0].CaseID, Arm: schedule.Inputs[0].Arm, Stage: "background", Status: "unknown", Outcome: outcome, Calls: 1, EvidenceSHA256: acceptance.MemoryDigest(probe.Events), InputTokens: &zero, OutputTokens: &zero, TotalTokens: &zero, Milliseconds: &probe.Milliseconds, StorageBytes: &zero}
		retain, stop := context.WithTimeout(context.Background(), 10*time.Second)
		_, saveErr := pool.Exec(retain, `INSERT INTO fixture_memory_auth_observations VALUES($1,$2)`, id, probe)
		stop()
		if saveErr != nil || memoryAuthWrite(private, "auth-probe-"+strconv.Itoa(n+1)+".json", probe) != nil {
			t.Fatal("auth probe retention failed; no further call")
		}
		r.Probes = append(r.Probes, probe)
		if probe.Passed || ctx.Err() != nil {
			break
		}
		// Refresh/semantic/persistence failures never repeat a potentially
		// consumed rotation. Only a read-only discovery transport rejection or
		// timeout after durable token resolution can get another finite probe.
		retryDiscovery := false
		unsafeRefresh := providerauth.UnsafeAntigravityRefresh(probe.Events)
		for _, d := range probe.Events {
			if (d.Stage == "oauth_refresh" || d.Stage == "refresh_persistence") && d.Outcome != "started" && d.Outcome != "response_decoded" && d.Outcome != "completed" {
				unsafeRefresh = true
			}
			if d.Stage == "paid_discovery" && (d.Outcome == "deadline" || d.Outcome == "request_failed" || d.Outcome == "http_rejected") {
				retryDiscovery = true
			}
		}
		if unsafeRefresh || !retryDiscovery {
			break
		}
	}
	r.Finished = time.Now().UTC()
	var authLedger json.RawMessage
	retain, stop := context.WithTimeout(context.Background(), 10*time.Second)
	err = pool.QueryRow(retain, `SELECT jsonb_build_object('admissions',(SELECT COALESCE(jsonb_agg(to_jsonb(t) ORDER BY id),'[]'::jsonb) FROM fixture_memory_auth_admissions t),'observations',(SELECT COALESCE(jsonb_agg(to_jsonb(t) ORDER BY id),'[]'::jsonb) FROM fixture_memory_auth_observations t))`).Scan(&authLedger)
	stop()
	if err != nil || memoryAuthWrite(private, "auth-ledger.json", authLedger) != nil {
		t.Fatal("safe auth ledger retention failed")
	}
	out, err := os.OpenFile(filepath.Join(private, "auth-preflight.json"), os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0600)
	if err != nil {
		t.Fatal("exclusive auth receipt denied")
	}
	encodeErr := json.NewEncoder(out).Encode(r)
	syncErr, closeErr := out.Sync(), out.Close()
	if encodeErr != nil || syncErr != nil || closeErr != nil {
		t.Fatal("auth receipt persistence failed")
	}
	if !r.Probes[len(r.Probes)-1].Passed {
		t.Fatal("control-plane preflight failed; no generation admitted")
	}
	t.Log("encrypted auth control-plane preflight passed; model_calls=0")
}

func memoryEvalRenewedAuthPool(t *testing.T) *pgxpool.Pool {
	t.Helper()
	pool, err := pgxpool.New(context.Background(), memoryAuthDSN)
	if err != nil {
		t.Fatal("new window auth store unavailable")
	}
	t.Cleanup(pool.Close)
	return pool
}

func (c *memoryEvalCollector) retainAuthPreflight(ctx context.Context) error {
	raw, err := acceptance.ReadPrivateFile(filepath.Join(c.private, "auth-preflight.json"), 1<<20)
	var r memoryAuthReceipt
	if err != nil || json.Unmarshal(raw, &r) != nil || len(r.Probes) < 1 || len(r.Probes) > 3 || r.ModelCalls != 0 || !r.Probes[len(r.Probes)-1].Passed || r.Started.IsZero() || r.Finished.Before(r.Started) || r.Finished.After(c.started) || r.Finished.Sub(r.Started) > 75*time.Second {
		return errors.New("new auth preflight receipt differs")
	}
	for _, p := range r.Probes {
		if err = c.recordExpense(ctx, p.Expense); err != nil {
			return err
		}
	}
	return nil
}

func memoryAuthWrite(private, name string, value any) error {
	out, err := os.OpenFile(filepath.Join(private, name), os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0600)
	if err != nil {
		return err
	}
	encodeErr := json.NewEncoder(out).Encode(value)
	syncErr, closeErr := out.Sync(), out.Close()
	if encodeErr != nil || syncErr != nil || closeErr != nil {
		return errors.New("private auth retention failed")
	}
	return nil
}
