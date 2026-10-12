package app

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"os"
	"path/filepath"
	"runtime"
	"sync"
	"testing"

	"github.com/butler/butler/internal/memory/memorytest"
	"github.com/butler/butler/internal/transport"
	"github.com/butler/butler/internal/transport/antigravity"
	"github.com/jackc/pgx/v5/pgxpool"
)

const (
	fullHighPriorAttempts = 27
	fullHighPriorReport   = "f4649a539eed17bf01db034209a757bbfdb268759b2446c852f2260009bba0d2"
	fullHighCorpusHash    = "c742601b09913754de88a330280af0eb7139ecd62c0e2ce24f5d5433a8830d06"
)

func fullHighDelegationWindow() bool {
	return os.Getenv("BUTLER_DELEGATION_HIGH_ACCEPTANCE_WINDOW") == "1" || highRemainderDelegationWindow()
}

func fullHighDelegationHistoryPaths() []string {
	paths := highDelegationHistoryPaths()
	root := filepath.Dir(filepath.Dir(filepath.Dir(paths[0])))
	return append(paths, filepath.Join(root, "deploy", "delegation", "native-scenario-high-20261012.json"))
}

// Carry the failed High window unchanged, including NULL usage and its precise
// incomplete scope. A new owner request cannot refund its five native calls.
func readFullHighDelegationHistory(paths []string) (nativeHistoricalEvidence, error) {
	if len(paths) != 10 {
		return nativeHistoricalEvidence{}, errNativeFixtureDenied
	}
	h, err := readHighDelegationHistory(paths[:9])
	if err != nil {
		return h, err
	}
	raw, err := readNativeArtifact(paths[9], fullHighPriorReport)
	var previous nativeScenarioEvidence
	if err != nil || json.Unmarshal(raw, &previous) != nil || previous.Status != "failed" || previous.Model != antigravity.FlashHighModel || previous.ReleaseRef != "7431af70a7f9f8699be86a2a73c053c2be56a3c5" || previous.NativeAttempts != 27 || previous.NewNativeAttempts != 5 || previous.MaxAttempts != 34 || previous.MaxNewAttempts != 12 || len(previous.Attempts) != 5 || len(previous.Usage) != 5 || len(previous.Widths) != 1 || string(nativeJSON(previous.Historical)) != string(nativeJSON(h)) {
		return nativeHistoricalEvidence{}, errNativeFixtureDenied
	}
	w := previous.Widths[0]
	if w.Width != 1 || !w.ParentWorkerKilled || w.ParentReceiptCheck || len(w.ObjectiveChecks) != 2 || !w.ObjectiveChecks[0] || !w.ObjectiveChecks[1] {
		return nativeHistoricalEvidence{}, errNativeFixtureDenied
	}
	for i, attempt := range previous.Attempts {
		if attempt.Attempt != 23+i || attempt.RunID == "" {
			return nativeHistoricalEvidence{}, errNativeFixtureDenied
		}
	}
	h.Usage = append(h.Usage, previous.Usage...)
	h.KnownUsagePairs, h.UnknownUsagePairs = 0, 0
	var input, output int64
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
	if len(h.Usage) != 27 || h.KnownUsagePairs != 11 || h.UnknownUsagePairs != 16 || input != 38196 || output != 1016 {
		return nativeHistoricalEvidence{}, errNativeFixtureDenied
	}
	baseline := sha256.Sum256([]byte(h.BaselineSHA256 + "\n" + fullHighPriorReport + "\n"))
	h.BaselineSHA256 = hex.EncodeToString(baseline[:])
	h.NativeAttempts = fullHighPriorAttempts
	h.Artifacts = append(h.Artifacts, nativePriorArtifact{ReportSHA256: fullHighPriorReport, ReleaseRef: previous.ReleaseRef, NewAttempts: 5})
	return h, nil
}

func readFullHighDelegationCorpus(path string) (nativeResearchCorpus, error) {
	if path == "" {
		_, file, _, _ := runtime.Caller(0)
		path = filepath.Join(filepath.Dir(file), "..", "..", "..", "..", "scripts", "dev", "fixtures", "delegation-high-conflict-20261012.json")
	}
	raw, err := readNativeArtifact(path, fullHighCorpusHash)
	var c nativeResearchCorpus
	if err != nil || json.Unmarshal(raw, &c) != nil || c.Version != 1 || len(c.Sources) != 2 || c.RequiredUncertainty != "source_only" || c.Sources[0].ExpectedValue != 17 || c.Sources[1].ExpectedValue != 23 || c.Sources[0].ExpectedDate != "2026-10-11" || c.Sources[1].ExpectedDate != c.Sources[0].ExpectedDate {
		return c, errNativeFixtureDenied
	}
	return c, nil
}

func newFullHighDelegationLimiter(ctx context.Context, pool *pgxpool.Pool) (*nativeDelegationLimiter, error) {
	h, err := readFullHighDelegationHistory(fullHighDelegationHistoryPaths())
	if pool == nil || err != nil {
		return nil, errNativeFixtureDenied
	}
	_, err = pool.Exec(ctx, `CREATE TABLE IF NOT EXISTS fixture_native_limit (
 singleton boolean PRIMARY KEY CHECK(singleton), attempts integer NOT NULL CHECK(attempts BETWEEN 27 AND 39),
 prior_attempts integer NOT NULL CHECK(prior_attempts=27), hard_cap integer NOT NULL CHECK(hard_cap=39),
 new_cap integer NOT NULL CHECK(new_cap=12), baseline_hash text NOT NULL CHECK(length(baseline_hash)=64),
 expires_at timestamptz NOT NULL);
 CREATE TABLE IF NOT EXISTS fixture_native_attempts(attempt integer PRIMARY KEY CHECK(attempt BETWEEN 28 AND 39),
 run_id text NOT NULL,kind text NOT NULL CHECK(kind IN ('start','continue','tool_result')),
 input_hash text NOT NULL CHECK(length(input_hash)=64),created_at timestamptz NOT NULL DEFAULT clock_timestamp());
 CREATE OR REPLACE FUNCTION fixture_full_high_immutable() RETURNS trigger LANGUAGE plpgsql AS $$ BEGIN
 IF TG_OP='DELETE' THEN RAISE EXCEPTION 'immutable full High window'; END IF;
 IF NEW.singleton IS DISTINCT FROM OLD.singleton OR NEW.prior_attempts IS DISTINCT FROM OLD.prior_attempts
 OR NEW.hard_cap IS DISTINCT FROM OLD.hard_cap OR NEW.new_cap IS DISTINCT FROM OLD.new_cap
 OR NEW.baseline_hash IS DISTINCT FROM OLD.baseline_hash OR NEW.expires_at IS DISTINCT FROM OLD.expires_at
 OR NEW.attempts<OLD.attempts THEN RAISE EXCEPTION 'immutable full High window'; END IF; RETURN NEW; END $$;
 DROP TRIGGER IF EXISTS fixture_full_high_guard ON fixture_native_limit;
 CREATE TRIGGER fixture_full_high_guard BEFORE UPDATE OR DELETE ON fixture_native_limit FOR EACH ROW EXECUTE FUNCTION fixture_full_high_immutable();`)
	if err == nil {
		_, err = pool.Exec(ctx, `INSERT INTO fixture_native_limit VALUES(TRUE,27,27,39,12,$1,clock_timestamp()+interval '18 minutes') ON CONFLICT DO NOTHING`, h.BaselineSHA256)
	}
	var valid bool
	if err != nil || pool.QueryRow(ctx, `SELECT prior_attempts=27 AND hard_cap=39 AND new_cap=12 AND baseline_hash=$1 FROM fixture_native_limit WHERE singleton`, h.BaselineSHA256).Scan(&valid) != nil || !valid {
		return nil, errNativeFixtureDenied
	}
	return &nativeDelegationLimiter{pool}, nil
}

func (l *nativeDelegationLimiter) reserveFullHigh(ctx context.Context, kind, runID string, input any, paths []string) error {
	if l == nil || l.pool == nil || runID == "" || kind != "start" && kind != "continue" && kind != "tool_result" {
		return errNativeFixtureDenied
	}
	h, err := readFullHighDelegationHistory(paths)
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
	if tx.QueryRow(ctx, `UPDATE fixture_native_limit SET attempts=attempts+1 WHERE singleton AND prior_attempts=27 AND hard_cap=39 AND new_cap=12 AND baseline_hash=$1 AND attempts<hard_cap AND expires_at>clock_timestamp() RETURNING attempts`, h.BaselineSHA256).Scan(&ordinal) != nil {
		return errNativeFixtureDenied
	}
	// UPDATE can test expiry before waiting on an unchanged row lock. Once it
	// holds that lock, check the database clock again before committing a slot.
	// A denial rolls back both the provisional counter and any attempt record.
	var withinWindow bool
	if tx.QueryRow(ctx, `SELECT expires_at>clock_timestamp() FROM fixture_native_limit WHERE singleton`).Scan(&withinWindow) != nil || !withinWindow {
		return errNativeFixtureDenied
	}
	if _, err = tx.Exec(ctx, `INSERT INTO fixture_native_attempts(attempt,run_id,kind,input_hash) VALUES($1,$2,$3,$4)`, ordinal, runID, kind, hex.EncodeToString(hash[:])); err != nil || tx.Commit(ctx) != nil {
		return errNativeFixtureDenied
	}
	return nil
}

func TestFullHighDelegationHistory(t *testing.T) {
	h, err := readFullHighDelegationHistory(fullHighDelegationHistoryPaths())
	if err != nil || h.NativeAttempts != 27 || len(h.Artifacts) != 10 || h.KnownUsagePairs != 11 || h.UnknownUsagePairs != 16 {
		t.Fatal("full High history unavailable")
	}
	if _, err := readFullHighDelegationCorpus(""); err != nil {
		t.Fatal("pinned conflict corpus unavailable")
	}
}

func TestFullHighDelegationLimitIntegration(t *testing.T) {
	t.Setenv("BUTLER_DELEGATION_HIGH_ACCEPTANCE_WINDOW", "1")
	pool, ctx := memorytest.Database(t), context.Background()
	l, err := newNativeDelegationLimiter(ctx, pool)
	if err != nil {
		t.Fatal("full High limiter unavailable")
	}
	for _, query := range []string{"UPDATE fixture_native_limit SET attempts=26", "UPDATE fixture_native_limit SET hard_cap=40", "UPDATE fixture_native_limit SET expires_at=expires_at+interval '1 minute'", "UPDATE fixture_native_limit SET baseline_hash=repeat('f',64)", "DELETE FROM fixture_native_limit"} {
		if _, err = pool.Exec(ctx, query); err == nil {
			t.Fatal("full High immutable ceiling changed")
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
	var n int
	for ok := range allowed {
		if ok {
			n++
		}
	}
	if n != 12 {
		t.Fatal("full High concurrent cap failed", n)
	}
	if _, err = newNativeDelegationLimiter(ctx, pool); err != nil {
		t.Fatal("full High reconstruction failed")
	}
	if l.reserve(ctx, "continue", "same-run", nil) == nil {
		t.Fatal("full High reconstruction refunded spending")
	}
	var first, last, count int
	if pool.QueryRow(ctx, `SELECT min(attempt),max(attempt),count(*) FROM fixture_native_attempts`).Scan(&first, &last, &count) != nil || first != 28 || last != 39 || count != 12 {
		t.Fatal("full High durable ordinals invalid")
	}
}

func TestFullHighDelegationRevalidationIntegration(t *testing.T) {
	t.Setenv("BUTLER_DELEGATION_HIGH_ACCEPTANCE_WINDOW", "1")
	pool, ctx := memorytest.Database(t), context.Background()
	l, err := newNativeDelegationLimiter(ctx, pool)
	if err != nil {
		t.Fatal("full High limiter unavailable")
	}
	p := &limitedNativeDelegationProvider{Provider: &antigravity.Provider{}, limit: l}
	_, _ = p.StartRun(ctx, transport.StartRunRequest{Context: transport.TransportRunContext{RunID: "invalid"}})
	_, _ = p.ContinueRun(ctx, transport.ContinueRunRequest{RunID: "invalid"})
	_, _ = p.SubmitToolResult(ctx, transport.SubmitToolResultRequest{RunID: "invalid"})
	var spent int
	if pool.QueryRow(ctx, `SELECT attempts FROM fixture_native_limit`).Scan(&spent) != nil || spent != 30 {
		t.Fatal("full High entry charge lost")
	}
	for i := range fullHighDelegationHistoryPaths() {
		paths := fullHighDelegationHistoryPaths()
		paths[i] = filepath.Join(t.TempDir(), "missing")
		for _, kind := range []string{"start", "continue", "tool_result"} {
			if l.reserveFullHigh(ctx, kind, "invalid", nil, paths) == nil {
				t.Fatal("changed history admitted")
			}
		}
	}
	if pool.QueryRow(ctx, `SELECT attempts FROM fixture_native_limit`).Scan(&spent) != nil || spent != 30 {
		t.Fatal("invalid history charged a native method")
	}
	if _, err = pool.Exec(ctx, `ALTER TABLE fixture_native_limit DISABLE TRIGGER fixture_full_high_guard; UPDATE fixture_native_limit SET expires_at=clock_timestamp()-interval '1 second'; ALTER TABLE fixture_native_limit ENABLE TRIGGER fixture_full_high_guard`); err != nil {
		t.Fatal("private expiry unavailable")
	}
	if l.reserve(ctx, "start", "expired", nil) == nil {
		t.Fatal("expired full High window admitted")
	}
}

func TestFullHighDelegationCompositionIntegration(t *testing.T) {
	t.Setenv("BUTLER_DELEGATION_HIGH_ACCEPTANCE_WINDOW", "1")
	f := newNativeScenarioFixture(t, context.Background())
	for _, width := range []int{1, 2} {
		f.composition(t, width)
	}
	var spent int
	if f.pool.QueryRow(f.ctx, `SELECT attempts FROM fixture_native_limit`).Scan(&spent) != nil || spent != 27 || nativeScenarioModel() != antigravity.FlashHighModel {
		t.Fatal("full High composition spent or substituted inference")
	}
}

func TestFullHighDelegationActualAcceptanceIntegration(t *testing.T) {
	if highRemainderDelegationWindow() {
		t.Fatal("remainder requires its separate declared mixed-evidence entry")
	}
	if !fullHighDelegationWindow() || os.Getenv("BUTLER_DELEGATION_NATIVE_ACCEPTANCE") != "1" {
		t.Skip("separate owner-authorized full High acceptance required")
	}
	runNativeDelegationActualScenario(t)
}
