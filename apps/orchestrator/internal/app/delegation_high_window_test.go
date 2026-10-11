package app

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"os"
	"path/filepath"
	"sync"
	"testing"

	"github.com/butler/butler/internal/memory/memorytest"
	"github.com/butler/butler/internal/transport"
	"github.com/butler/butler/internal/transport/antigravity"
	"github.com/jackc/pgx/v5/pgxpool"
)

// A separate operator fixture; the historical tiered runner remains sealed.
// No production default, owner authority or model retry policy changes here.
const highDelegationPriorAttempts = 22

var highDelegationPins = []struct{ path, hash string }{
	{"deploy/delegation/native-scenario-fifth.json", "5e180087d4e1d43fe52c6afac0c35763b69a8dfe906953a18eac49416bd7cded"},
	{"deploy/antigravity/live-comparison-20261011.json", "6808c50e120b59a40c2c2d9b53babb7dba1e581a3fbcb29ddb3cae6b9491e472"},
	{"deploy/antigravity/native8192-20261011.json", "f3474b9d7b6441d1a9ea66dcf84b8aecc7dbe0c7dfcc1494bff3eaa7c19dcdb5"},
	{"deploy/antigravity/desktop-interception-20261012.json", "208f7d4702451b36e8f27ab25474fcb3edc66e1e9a18074fd5a946d2ba47dffe"},
	{"deploy/antigravity/flash-high-20261012.json", "3ec5ffb14090e7875db47490ad20821645974fd3c9478af5b831128b3558a2cc"},
}

func nativeHighWindow() bool { return os.Getenv("BUTLER_DELEGATION_HIGH_WINDOW") == "1" }

func nativeScenarioModel() string {
	if nativeHighWindow() {
		return antigravity.FlashHighModel
	}
	return antigravity.DefaultModel
}

func nativeScenarioHistory() (nativeHistoricalEvidence, error) {
	if nativeHighWindow() {
		return readHighDelegationHistory(highDelegationHistoryPaths())
	}
	return readNativeHistory(nativeHistoryPaths()...)
}

func highDelegationHistoryPaths() []string {
	paths := nativeHistoryPaths()
	root := filepath.Dir(filepath.Dir(filepath.Dir(paths[0])))
	for _, pin := range highDelegationPins {
		paths = append(paths, filepath.Join(root, filepath.FromSlash(pin.path)))
	}
	return paths
}

func readHighDelegationHistory(paths []string) (nativeHistoricalEvidence, error) {
	if len(paths) != 9 {
		return nativeHistoricalEvidence{}, errNativeFixtureDenied
	}
	h, err := readNativeHistory(paths[:4]...)
	if err != nil {
		return h, err
	}
	raw := make([][]byte, len(highDelegationPins))
	for i, pin := range highDelegationPins {
		raw[i], err = readNativeArtifact(paths[i+4], pin.hash)
		if err != nil {
			return nativeHistoricalEvidence{}, err
		}
	}
	var fifth nativeScenarioEvidence
	if json.Unmarshal(raw[0], &fifth) != nil || fifth.Status != "failed" || fifth.NativeAttempts != 18 || fifth.NewNativeAttempts != 5 || len(fifth.Usage) != 5 || string(nativeJSON(fifth.Historical)) != string(nativeJSON(h)) {
		return nativeHistoricalEvidence{}, errNativeFixtureDenied
	}
	h.Usage = append(h.Usage, fifth.Usage...)
	type diagnosticUsage struct {
		Attempt int    `json:"attempt"`
		Input   *int64 `json:"input_tokens"`
		Output  *int64 `json:"output_tokens"`
	}
	var pair struct {
		Historical   int               `json:"historical_attempts"`
		Total        int               `json:"total_attempts"`
		Observations []diagnosticUsage `json:"observations"`
	}
	var native struct {
		Historical   int               `json:"historical_attempts"`
		Total        int               `json:"total_attempts"`
		Observations []diagnosticUsage `json:"observations"`
	}
	var high struct {
		Live struct {
			Historical int             `json:"historical_attempts"`
			Result     diagnosticUsage `json:"result"`
		} `json:"live"`
	}
	if json.Unmarshal(raw[1], &pair) != nil || pair.Historical != 18 || pair.Total != 20 || len(pair.Observations) != 2 || pair.Observations[0].Attempt != 19 || pair.Observations[1].Attempt != 20 || json.Unmarshal(raw[2], &native) != nil || native.Historical != 20 || native.Total != 21 || len(native.Observations) != 1 || native.Observations[0].Attempt != 21 || json.Unmarshal(raw[4], &high) != nil || high.Live.Historical != 21 || high.Live.Result.Attempt != 22 {
		return nativeHistoricalEvidence{}, errNativeFixtureDenied
	}
	for _, u := range append(pair.Observations, native.Observations[0], high.Live.Result) {
		h.Usage = append(h.Usage, nativeUsageEvidence{ActualInputTokens: u.Input, ActualOutputTokens: u.Output})
	}
	var input, output int64
	h.KnownUsagePairs, h.UnknownUsagePairs = 0, 0
	for _, u := range h.Usage {
		if u.ActualInputTokens == nil && u.ActualOutputTokens == nil {
			h.UnknownUsagePairs++
		} else if u.ActualInputTokens != nil && u.ActualOutputTokens != nil {
			h.KnownUsagePairs++
			input += *u.ActualInputTokens
			output += *u.ActualOutputTokens
		} else {
			return nativeHistoricalEvidence{}, errNativeFixtureDenied
		}
	}
	if len(h.Usage) != 22 || h.KnownUsagePairs != 8 || h.UnknownUsagePairs != 14 || input != 26968 || output != 598 {
		return nativeHistoricalEvidence{}, errNativeFixtureDenied
	}
	baseline := nativeBaselineHash() + "\n"
	for i, pin := range highDelegationPins {
		baseline += pin.hash + "\n"
		count := []int{5, 2, 1, 0, 1}[i]
		h.Artifacts = append(h.Artifacts, nativePriorArtifact{ReportSHA256: pin.hash, NewAttempts: count})
	}
	hash := sha256.Sum256([]byte(baseline))
	h.BaselineSHA256, h.NativeAttempts, h.Status = hex.EncodeToString(hash[:]), 22, "mixed_completed_history"
	return h, nil
}

func newHighDelegationLimiter(ctx context.Context, pool *pgxpool.Pool) (*nativeDelegationLimiter, error) {
	h, err := readHighDelegationHistory(highDelegationHistoryPaths())
	if pool == nil || err != nil {
		return nil, errNativeFixtureDenied
	}
	_, err = pool.Exec(ctx, `CREATE TABLE IF NOT EXISTS fixture_native_limit (
 singleton boolean PRIMARY KEY CHECK(singleton), attempts integer NOT NULL CHECK(attempts BETWEEN 22 AND 34),
 prior_attempts integer NOT NULL CHECK(prior_attempts=22), hard_cap integer NOT NULL CHECK(hard_cap=34),
 new_cap integer NOT NULL CHECK(new_cap=12), baseline_hash text NOT NULL CHECK(length(baseline_hash)=64),
 expires_at timestamptz NOT NULL);
 CREATE TABLE IF NOT EXISTS fixture_native_attempts(attempt integer PRIMARY KEY CHECK(attempt BETWEEN 23 AND 34),
 run_id text NOT NULL,kind text NOT NULL CHECK(kind IN ('start','continue','tool_result')),
 input_hash text NOT NULL CHECK(length(input_hash)=64),created_at timestamptz NOT NULL DEFAULT clock_timestamp());
 CREATE OR REPLACE FUNCTION fixture_high_limit_immutable() RETURNS trigger LANGUAGE plpgsql AS $$ BEGIN
 IF TG_OP='DELETE' THEN RAISE EXCEPTION 'immutable High window'; END IF;
 IF NEW.singleton IS DISTINCT FROM OLD.singleton OR NEW.prior_attempts IS DISTINCT FROM OLD.prior_attempts
 OR NEW.hard_cap IS DISTINCT FROM OLD.hard_cap OR NEW.new_cap IS DISTINCT FROM OLD.new_cap
 OR NEW.baseline_hash IS DISTINCT FROM OLD.baseline_hash OR NEW.expires_at IS DISTINCT FROM OLD.expires_at
 OR NEW.attempts<OLD.attempts THEN RAISE EXCEPTION 'immutable High window'; END IF; RETURN NEW; END $$;
 DROP TRIGGER IF EXISTS fixture_high_guard ON fixture_native_limit;
 CREATE TRIGGER fixture_high_guard BEFORE UPDATE OR DELETE ON fixture_native_limit FOR EACH ROW EXECUTE FUNCTION fixture_high_limit_immutable();`)
	if err == nil {
		_, err = pool.Exec(ctx, `INSERT INTO fixture_native_limit VALUES(TRUE,22,22,34,12,$1,clock_timestamp()+interval '18 minutes') ON CONFLICT DO NOTHING`, h.BaselineSHA256)
	}
	var valid bool
	if err != nil || pool.QueryRow(ctx, `SELECT prior_attempts=22 AND hard_cap=34 AND new_cap=12 AND baseline_hash=$1 FROM fixture_native_limit WHERE singleton`, h.BaselineSHA256).Scan(&valid) != nil || !valid {
		return nil, errNativeFixtureDenied
	}
	return &nativeDelegationLimiter{pool}, nil
}

func (l *nativeDelegationLimiter) reserveHigh(ctx context.Context, kind, runID string, input any, paths []string) error {
	if l == nil || l.pool == nil || runID == "" || kind != "start" && kind != "continue" && kind != "tool_result" {
		return errNativeFixtureDenied
	}
	h, err := readHighDelegationHistory(paths)
	if err != nil {
		return err
	}
	raw, err := json.Marshal(input)
	if err != nil {
		return errNativeFixtureDenied
	}
	hash := sha256.Sum256(raw)
	tx, err := l.pool.Begin(ctx)
	if err != nil {
		return errNativeFixtureDenied
	}
	defer tx.Rollback(ctx)
	var ordinal int
	if tx.QueryRow(ctx, `UPDATE fixture_native_limit SET attempts=attempts+1 WHERE singleton AND prior_attempts=22 AND hard_cap=34 AND new_cap=12 AND baseline_hash=$1 AND attempts<hard_cap AND expires_at>clock_timestamp() RETURNING attempts`, h.BaselineSHA256).Scan(&ordinal) != nil {
		return errNativeFixtureDenied
	}
	if _, err = tx.Exec(ctx, `INSERT INTO fixture_native_attempts(attempt,run_id,kind,input_hash) VALUES($1,$2,$3,$4)`, ordinal, runID, kind, hex.EncodeToString(hash[:])); err != nil || tx.Commit(ctx) != nil {
		return errNativeFixtureDenied
	}
	return nil
}

func TestHighDelegationHistory(t *testing.T) {
	h, err := readHighDelegationHistory(highDelegationHistoryPaths())
	if err != nil || h.NativeAttempts != 22 || h.KnownUsagePairs != 8 || h.UnknownUsagePairs != 14 || len(h.Artifacts) != 9 {
		t.Fatal("High carry-forward history unavailable")
	}
}

func TestHighDelegationAdmissionIntegration(t *testing.T) {
	t.Setenv("BUTLER_DELEGATION_HIGH_WINDOW", "1")
	pool, ctx := memorytest.Database(t), context.Background()
	l, err := newNativeDelegationLimiter(ctx, pool)
	if err != nil {
		t.Fatal("High bound unavailable")
	}
	for _, q := range []string{"UPDATE fixture_native_limit SET attempts=21", "UPDATE fixture_native_limit SET hard_cap=35", "UPDATE fixture_native_limit SET baseline_hash=repeat('f',64)", "UPDATE fixture_native_limit SET expires_at=expires_at+interval '1 hour'", "DELETE FROM fixture_native_limit"} {
		if _, err := pool.Exec(ctx, q); err == nil {
			t.Fatal("High spending history or deadline changed")
		}
	}
	var wg sync.WaitGroup
	allowed := make(chan bool, 16)
	for i := 0; i < 16; i++ {
		wg.Add(1)
		go func() { defer wg.Done(); allowed <- l.reserve(ctx, "start", "same-run", nil) == nil }()
	}
	wg.Wait()
	close(allowed)
	n := 0
	for ok := range allowed {
		if ok {
			n++
		}
	}
	if n != 12 {
		t.Fatal("concurrent High allowance exceeded", n)
	}
	if _, err = newNativeDelegationLimiter(ctx, pool); err != nil {
		t.Fatal("reconstruction failed")
	}
	for _, kind := range []string{"start", "continue", "tool_result"} {
		if l.reserve(ctx, kind, "same-run", nil) == nil {
			t.Fatal("restart refunded a High attempt")
		}
	}
	var first, last, count int
	if pool.QueryRow(ctx, `SELECT min(attempt),max(attempt),count(*) FROM fixture_native_attempts`).Scan(&first, &last, &count) != nil || first != 23 || last != 34 || count != 12 {
		t.Fatal("durable ordinals lost")
	}
}

func TestHighDelegationRevalidationAndEntryChargesIntegration(t *testing.T) {
	t.Setenv("BUTLER_DELEGATION_HIGH_WINDOW", "1")
	pool, ctx := memorytest.Database(t), context.Background()
	l, err := newNativeDelegationLimiter(ctx, pool)
	if err != nil {
		t.Fatal("High bound unavailable")
	}
	// The real provider has no HTTP/auth setup; invalid methods fail locally.
	p := &limitedNativeDelegationProvider{Provider: &antigravity.Provider{}, limit: l}
	_, _ = p.StartRun(ctx, transport.StartRunRequest{Context: transport.TransportRunContext{RunID: "invalid"}})
	_, _ = p.ContinueRun(ctx, transport.ContinueRunRequest{RunID: "invalid"})
	_, _ = p.SubmitToolResult(ctx, transport.SubmitToolResultRequest{RunID: "invalid"})
	var spent int
	if pool.QueryRow(ctx, `SELECT attempts FROM fixture_native_limit`).Scan(&spent) != nil || spent != 25 {
		t.Fatal("entry kinds escaped pre-call charge")
	}
	for i := range highDelegationHistoryPaths() {
		paths := highDelegationHistoryPaths()
		paths[i] = filepath.Join(t.TempDir(), "missing")
		for _, kind := range []string{"start", "continue", "tool_result"} {
			if l.reserveHigh(ctx, kind, "invalid", nil, paths) == nil {
				t.Fatal("changed history admitted")
			}
		}
	}
	if pool.QueryRow(ctx, `SELECT attempts FROM fixture_native_limit`).Scan(&spent) != nil || spent != 25 {
		t.Fatal("failed history spent an attempt")
	}
	// Expiry is fixture-only injection; the public UPDATE guard is checked above.
	if _, err = pool.Exec(ctx, `ALTER TABLE fixture_native_limit DISABLE TRIGGER fixture_high_guard; UPDATE fixture_native_limit SET expires_at=clock_timestamp()-interval '1 second'; ALTER TABLE fixture_native_limit ENABLE TRIGGER fixture_high_guard`); err != nil {
		t.Fatal("fixture expiry unavailable")
	}
	if l.reserve(ctx, "start", "expired", nil) == nil {
		t.Fatal("expired High window admitted")
	}
}

func TestHighDelegationCompositionIntegration(t *testing.T) {
	t.Setenv("BUTLER_DELEGATION_HIGH_WINDOW", "1")
	f := newNativeScenarioFixture(t, context.Background())
	for _, width := range []int{1, 2} {
		f.composition(t, width)
	}
	var spent int
	if f.pool.QueryRow(f.ctx, `SELECT attempts FROM fixture_native_limit`).Scan(&spent) != nil || spent != 22 || nativeScenarioModel() != antigravity.FlashHighModel {
		t.Fatal("High composition spent or substituted inference")
	}
}

func TestHighDelegationActualScenarioIntegration(t *testing.T) {
	if !nativeHighWindow() || os.Getenv("BUTLER_DELEGATION_NATIVE_ACCEPTANCE") != "1" {
		t.Skip("separate sealed High operator window required")
	}
	runNativeDelegationActualScenario(t)
}
