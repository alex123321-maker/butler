package app

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"testing"

	"github.com/butler/butler/internal/actionauth"
	"github.com/butler/butler/internal/delegation"
	"github.com/butler/butler/internal/memory/memorytest"
	"github.com/butler/butler/internal/transport"
	"github.com/butler/butler/internal/transport/antigravity"
	"github.com/jackc/pgx/v5/pgxpool"
)

const (
	nativeDelegationAttemptCap        = 12 // New experiment only; owner/day limits stay 12.
	nativeDelegationPriorAttempts     = 3
	nativeDelegationTotalAttemptCap   = nativeDelegationPriorAttempts + nativeDelegationAttemptCap
	nativeDelegationPriorReportSHA256 = "28c75c31827f8a25ca8ed101e6e04b8264a7e66ee9ce18808d504d5af748c8f0"
	nativeDelegationPriorRelease      = "85bef824e838caef64407891d5b228f982c51032"
)

var errNativeFixtureDenied = errors.New("native delegation fixture admission denied")

// This counter is an additional fixture spending limit, not owner authority or
// a substitute for the actual production root/delegation/day budget ledger.
// It charges every attempted provider call, without replay deduplication.
type nativeDelegationLimiter struct{ pool *pgxpool.Pool }

type nativeHistoricalEvidence struct {
	Status         string                `json:"status"`
	ReportSHA256   string                `json:"report_sha256"`
	ReleaseRef     string                `json:"release_ref"`
	NativeAttempts int                   `json:"native_attempts"`
	Usage          []nativeUsageEvidence `json:"usage"`
}

// Only the committed content-free FAILED artifact supplies historical accounting.
// This fixed source path is kernel fixture configuration, never an environment or
// model-selected path. Missing/changed bytes deny before import or inference.
func nativeHistoryPath() string {
	_, file, _, _ := runtime.Caller(0)
	return filepath.Join(filepath.Dir(file), "..", "..", "..", "..", "deploy", "delegation", "native-scenario-failed.json")
}

func readNativeHistory(path string) (nativeHistoricalEvidence, error) {
	var h nativeHistoricalEvidence
	stat, err := os.Lstat(path)
	if err != nil || !stat.Mode().IsRegular() || stat.Size() <= 0 || stat.Size() > 32768 {
		return h, errNativeFixtureDenied
	}
	f, err := os.Open(path)
	if err != nil {
		return h, errNativeFixtureDenied
	}
	defer f.Close()
	opened, err := f.Stat()
	if err != nil || !os.SameFile(stat, opened) {
		return h, errNativeFixtureDenied
	}
	raw, err := io.ReadAll(io.LimitReader(f, 32769))
	hash := sha256.Sum256(raw)
	if err != nil || len(raw) > 32768 || hex.EncodeToString(hash[:]) != nativeDelegationPriorReportSHA256 {
		return h, errNativeFixtureDenied
	}
	var prior struct {
		Status         string                `json:"status"`
		ReleaseRef     string                `json:"release_ref"`
		NativeAttempts int                   `json:"native_attempts"`
		Usage          []nativeUsageEvidence `json:"usage"`
	}
	if json.Unmarshal(raw, &prior) != nil || prior.Status != "failed" || prior.ReleaseRef != nativeDelegationPriorRelease || prior.NativeAttempts != nativeDelegationPriorAttempts || len(prior.Usage) != nativeDelegationPriorAttempts {
		return h, errNativeFixtureDenied
	}
	return nativeHistoricalEvidence{Status: prior.Status, ReportSHA256: nativeDelegationPriorReportSHA256, ReleaseRef: prior.ReleaseRef, NativeAttempts: prior.NativeAttempts, Usage: prior.Usage}, nil
}

func newNativeDelegationLimiter(ctx context.Context, pool *pgxpool.Pool) (*nativeDelegationLimiter, error) {
	if pool == nil {
		return nil, errNativeFixtureDenied
	}
	if _, err := readNativeHistory(nativeHistoryPath()); err != nil {
		return nil, err
	}
	_, err := pool.Exec(ctx, `CREATE TABLE fixture_native_limit(singleton boolean PRIMARY KEY CHECK(singleton),attempts integer NOT NULL CHECK(attempts BETWEEN 3 AND 15),prior_attempts integer NOT NULL CHECK(prior_attempts=3),hard_cap integer NOT NULL CHECK(hard_cap=15),new_cap integer NOT NULL CHECK(new_cap=12),baseline_sha256 text NOT NULL CHECK(baseline_sha256='28c75c31827f8a25ca8ed101e6e04b8264a7e66ee9ce18808d504d5af748c8f0'));
 INSERT INTO fixture_native_limit VALUES(TRUE,3,3,15,12,'28c75c31827f8a25ca8ed101e6e04b8264a7e66ee9ce18808d504d5af748c8f0');
 CREATE FUNCTION fixture_native_limit_immutable() RETURNS trigger LANGUAGE plpgsql AS $$ BEGIN
 IF NEW.singleton IS DISTINCT FROM OLD.singleton OR NEW.prior_attempts IS DISTINCT FROM OLD.prior_attempts OR NEW.hard_cap IS DISTINCT FROM OLD.hard_cap OR NEW.new_cap IS DISTINCT FROM OLD.new_cap OR NEW.baseline_sha256 IS DISTINCT FROM OLD.baseline_sha256 OR NEW.attempts<OLD.attempts THEN RAISE EXCEPTION 'immutable fixture baseline'; END IF; RETURN NEW; END $$;
 CREATE TRIGGER fixture_native_limit_guard BEFORE UPDATE ON fixture_native_limit FOR EACH ROW EXECUTE FUNCTION fixture_native_limit_immutable();
 CREATE TABLE fixture_native_attempts(attempt integer PRIMARY KEY CHECK(attempt BETWEEN 4 AND 15),run_id text NOT NULL,kind text NOT NULL CHECK(kind IN('start','continue','tool_result')),input_hash text NOT NULL CHECK(length(input_hash)=64),created_at timestamptz NOT NULL DEFAULT clock_timestamp())`)
	if err != nil {
		return nil, errNativeFixtureDenied
	}
	return &nativeDelegationLimiter{pool}, nil
}

func (l *nativeDelegationLimiter) reserve(ctx context.Context, kind, runID string, input any) error {
	if l == nil || l.pool == nil || runID == "" || kind != "start" && kind != "continue" && kind != "tool_result" {
		return errNativeFixtureDenied
	}
	if _, err := readNativeHistory(nativeHistoryPath()); err != nil {
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
	var attempt int
	if tx.QueryRow(ctx, `UPDATE fixture_native_limit SET attempts=attempts+1 WHERE singleton AND prior_attempts=$1 AND hard_cap=$2 AND new_cap=$3 AND baseline_sha256=$4 AND attempts<hard_cap AND attempts-prior_attempts<new_cap RETURNING attempts`, nativeDelegationPriorAttempts, nativeDelegationTotalAttemptCap, nativeDelegationAttemptCap, nativeDelegationPriorReportSHA256).Scan(&attempt) != nil {
		return errNativeFixtureDenied
	}
	if _, err = tx.Exec(ctx, `INSERT INTO fixture_native_attempts(attempt,run_id,kind,input_hash) VALUES($1,$2,$3,$4)`, attempt, runID, kind, hex.EncodeToString(hash[:])); err != nil {
		return errNativeFixtureDenied
	}
	if tx.Commit(ctx) != nil {
		return errNativeFixtureDenied
	}
	return nil
}

// Concrete native provider only: this gate cannot substitute a scripted model,
// endpoint, transport or event stream. Start/continue/result share one durable
// counter across widths and worker reconstruction. Cancellation spends no turn.
type limitedNativeDelegationProvider struct {
	*antigravity.Provider
	limit   *nativeDelegationLimiter
	observe func(context.Context, transport.StartRunRequest) error
}

func (p *limitedNativeDelegationProvider) StartRun(ctx context.Context, r transport.StartRunRequest) (transport.EventStream, error) {
	if p != nil && p.observe != nil && p.observe(ctx, r) != nil {
		return nil, errNativeFixtureDenied
	}
	if p == nil || p.Provider == nil || p.limit.reserve(ctx, "start", r.Context.RunID, r) != nil {
		return nil, errNativeFixtureDenied
	}
	return p.Provider.StartRun(ctx, r)
}
func (p *limitedNativeDelegationProvider) ContinueRun(ctx context.Context, r transport.ContinueRunRequest) (transport.EventStream, error) {
	if p == nil || p.Provider == nil || p.limit.reserve(ctx, "continue", r.RunID, r) != nil {
		return nil, errNativeFixtureDenied
	}
	return p.Provider.ContinueRun(ctx, r)
}
func (p *limitedNativeDelegationProvider) SubmitToolResult(ctx context.Context, r transport.SubmitToolResultRequest) (transport.EventStream, error) {
	if p == nil || p.Provider == nil || p.limit.reserve(ctx, "tool_result", r.RunID, r) != nil {
		return nil, errNativeFixtureDenied
	}
	return p.Provider.SubmitToolResult(ctx, r)
}

type nativeResearchObservation struct {
	SourceID       string `json:"source_id"`
	SourceRevision int64  `json:"source_revision"`
	SourceHash     string `json:"source_hash"`
	ObservedValue  int64  `json:"observed_value"`
	ObservedDate   string `json:"observed_date"`
	Uncertainty    string `json:"uncertainty"`
}

// This fixture checker recognizes one immutable synthetic source observation.
// It does not verify owner task completion or accept a model/report verdict.
func verifyNativeResearchObservation(raw string, source delegation.SourceRef, value int64, date string) error {
	if len(raw) > 8192 || !delegation.ValidSource(source) {
		return errNativeFixtureDenied
	}
	if _, err := actionauth.ArgumentsHash(raw); err != nil {
		return errNativeFixtureDenied
	}
	var result nativeResearchObservation
	d := json.NewDecoder(strings.NewReader(raw))
	d.DisallowUnknownFields()
	if d.Decode(&result) != nil || d.Decode(new(any)) != io.EOF || result.SourceID != source.ID || result.SourceRevision != source.Revision || result.SourceHash != source.Hash || result.ObservedValue != value || result.ObservedDate != date || result.Uncertainty != "source_only" {
		return errNativeFixtureDenied
	}
	return nil
}

func TestNativeDelegationObjectiveFixtureRejectsNarrativeAuthority(t *testing.T) {
	source := delegation.SourceRef{ID: "source:a", Revision: 1, AccessRevision: 1, Hash: validationHash}
	raw := `{"source_id":"source:a","source_revision":1,"source_hash":"` + validationHash + `","observed_value":17,"observed_date":"2026-10-10","uncertainty":"source_only"}`
	if verifyNativeResearchObservation(raw, source, 17, "2026-10-10") != nil {
		t.Fatal("exact immutable observation rejected")
	}
	for _, changed := range []string{
		strings.Replace(raw, `"observed_value":17`, `"observed_value":23`, 1),
		strings.Replace(raw, `"source_revision":1`, `"source_revision":2`, 1),
		strings.Replace(raw, `"source:a"`, `"source:b"`, 1),
		strings.Replace(raw, `"source_only"`, `"verified"`, 1),
		strings.TrimSuffix(raw, "}") + `,"accepted":true}`,
		strings.TrimSuffix(raw, "}") + `,"observed_value":17}`,
		raw + raw,
		"Both models agree: " + raw,
	} {
		if verifyNativeResearchObservation(changed, source, 17, "2026-10-10") == nil {
			t.Fatal("forged observation or narrative promoted to objective evidence")
		}
	}
	if verifyNativeAdmissionReceipts(`{"admitted_task_ids":["child-a","child-b"]}`, []string{"child-b", "child-a"}) != nil {
		t.Fatal("exact independent admission receipts rejected")
	}
	for _, changed := range []string{
		`{"admitted_task_ids":["child-a","child-a"]}`,
		`{"admitted_task_ids":["child-a","other"]}`,
		`{"admitted_task_ids":["child-a","child-b"],"accepted":true}`,
		`{"admitted_task_ids":["child-a","child-b"],"admitted_task_ids":["child-a","child-b"]}`,
		`{"admitted_task_ids":["child-a","child-b"]}{}`,
	} {
		if verifyNativeAdmissionReceipts(changed, []string{"child-a", "child-b"}) == nil {
			t.Fatal("parent narrative or forged receipt accepted")
		}
	}
}

// This is an exact receipt projection check, never a child success verdict.
func verifyNativeAdmissionReceipts(raw string, children []string) error {
	if len(children) != 2 || children[0] == "" || children[1] == "" || children[0] == children[1] {
		return errNativeFixtureDenied
	}
	if _, err := actionauth.ArgumentsHash(raw); err != nil {
		return errNativeFixtureDenied
	}
	var receipt struct {
		IDs []string `json:"admitted_task_ids"`
	}
	d := json.NewDecoder(strings.NewReader(raw))
	d.DisallowUnknownFields()
	if d.Decode(&receipt) != nil || d.Decode(new(any)) != io.EOF || len(receipt.IDs) != 2 || receipt.IDs[0] == receipt.IDs[1] {
		return errNativeFixtureDenied
	}
	if receipt.IDs[0] == children[0] && receipt.IDs[1] == children[1] || receipt.IDs[0] == children[1] && receipt.IDs[1] == children[0] {
		return nil
	}
	return errNativeFixtureDenied
}

func TestNativeDelegationTwelveAttemptBoundIntegration(t *testing.T) {
	pool := memorytest.Database(t)
	ctx := context.Background()
	limit, err := newNativeDelegationLimiter(ctx, pool)
	if err != nil {
		t.Fatal("private counter initialization denied")
	}
	var initial, prior, cap, remaining int
	var baseline string
	if pool.QueryRow(ctx, `SELECT attempts,prior_attempts,hard_cap,new_cap,baseline_sha256 FROM fixture_native_limit`).Scan(&initial, &prior, &cap, &remaining, &baseline) != nil || initial != 3 || prior != 3 || cap != 15 || remaining != 12 || baseline != nativeDelegationPriorReportSHA256 {
		t.Fatal("prior spending or remaining allowance lost")
	}
	for _, query := range []string{`UPDATE fixture_native_limit SET attempts=0`, `UPDATE fixture_native_limit SET prior_attempts=0`, `UPDATE fixture_native_limit SET hard_cap=16`, `UPDATE fixture_native_limit SET new_cap=13`, `UPDATE fixture_native_limit SET baseline_sha256=repeat('f',64)`} {
		if _, err := pool.Exec(ctx, query); err == nil {
			t.Fatal("immutable spending baseline was changed")
		}
	}
	var wg sync.WaitGroup
	allowed := make(chan bool, 16)
	for i := 0; i < 16; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			allowed <- limit.reserve(ctx, "start", "same-run-replayed", struct{ Marker string }{"synthetic private input"}) == nil
		}()
	}
	wg.Wait()
	close(allowed)
	count := 0
	for ok := range allowed {
		if ok {
			count++
		}
	}
	if count != nativeDelegationAttemptCap {
		t.Fatal("parallel/replayed attempts crossed the global cap", count)
	}
	// A reconstructed limiter neither resets the counter nor treats the same run
	// identity as a free retry. No native provider was constructed by this test.
	restarted := &nativeDelegationLimiter{pool}
	for _, kind := range []string{"start", "continue", "tool_result"} {
		if restarted.reserve(ctx, kind, "same-run-replayed", nil) == nil {
			t.Fatal("restart or continuation bypassed the shared attempt cap")
		}
	}
	var persisted int
	if pool.QueryRow(ctx, `SELECT count(*) FROM fixture_native_attempts`).Scan(&persisted) != nil || persisted != nativeDelegationAttemptCap {
		t.Fatal("attempt evidence was not durable", persisted)
	}
	var total, first, last int
	if pool.QueryRow(ctx, `SELECT attempts FROM fixture_native_limit`).Scan(&total) != nil || total != nativeDelegationTotalAttemptCap || pool.QueryRow(ctx, `SELECT min(attempt),max(attempt) FROM fixture_native_attempts`).Scan(&first, &last) != nil || first != 4 || last != 15 {
		t.Fatal("historical attempts were reset or copied as new attempts")
	}
}

func TestNativeDelegationCarryoverBaselineRejectsMismatch(t *testing.T) {
	h, err := readNativeHistory(nativeHistoryPath())
	if err != nil || h.NativeAttempts != 3 || h.ReportSHA256 != nativeDelegationPriorReportSHA256 || len(h.Usage) != 3 || h.Usage[0].ActualInputTokens != nil || h.Usage[0].ActualOutputTokens != nil || h.Usage[1].ActualInputTokens != nil || h.Usage[1].ActualOutputTokens != nil || h.Usage[2].ActualInputTokens == nil || *h.Usage[2].ActualInputTokens != 5526 || h.Usage[2].ActualOutputTokens == nil || *h.Usage[2].ActualOutputTokens != 101 {
		t.Fatal("known failed spending or nullable usage was changed")
	}
	raw, err := os.ReadFile(nativeHistoryPath())
	if err != nil {
		t.Fatal("committed baseline unavailable")
	}
	for _, name := range []string{"missing", "changed-count", "changed-release", "oversize", "symlink"} {
		t.Run(name, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "baseline.json")
			b := raw
			switch name {
			case "changed-count":
				b = []byte(strings.Replace(string(raw), `"native_attempts":3`, `"native_attempts":0`, 1))
			case "changed-release":
				b = []byte(strings.Replace(string(raw), nativeDelegationPriorRelease, strings.Repeat("f", 40), 1))
			case "oversize":
				b = []byte(strings.Repeat("x", 32769))
			case "symlink":
				if os.Symlink(nativeHistoryPath(), path) != nil {
					t.Fatal("fixture symlink creation failed")
				}
			}
			if name != "missing" && name != "symlink" && os.WriteFile(path, b, 0600) != nil {
				t.Fatal("fixture baseline creation failed")
			}
			if _, err = readNativeHistory(path); !errors.Is(err, errNativeFixtureDenied) {
				t.Fatal("missing/forged prior experiment accepted")
			}
		})
	}
}

func TestNativeDelegationProviderMethodsAreChargedBeforeValidationIntegration(t *testing.T) {
	pool, ctx := memorytest.Database(t), context.Background()
	limit, err := newNativeDelegationLimiter(ctx, pool)
	if err != nil {
		t.Fatal("private counter initialization denied")
	}
	// This uninitialized concrete native provider has no client or auth source.
	// Invalid requests synchronously fail protocol validation before network entry;
	// the reviewed wrapper still must durably charge all three attempted methods.
	p := &limitedNativeDelegationProvider{Provider: &antigravity.Provider{}, limit: limit}
	stream, err := p.StartRun(ctx, transport.StartRunRequest{Context: transport.TransportRunContext{RunID: "synthetic-rejected-start"}})
	if err == nil || stream != nil {
		t.Fatal("invalid start reached model execution")
	}
	stream, err = p.ContinueRun(ctx, transport.ContinueRunRequest{RunID: "synthetic-rejected-continue"})
	if err == nil || stream != nil {
		t.Fatal("invalid continuation reached model execution")
	}
	stream, err = p.SubmitToolResult(ctx, transport.SubmitToolResultRequest{RunID: "synthetic-rejected-result"})
	if err == nil || stream != nil {
		t.Fatal("invalid tool result reached model execution")
	}
	var total, starts, continues, results int
	if pool.QueryRow(ctx, `SELECT attempts FROM fixture_native_limit`).Scan(&total) != nil || total != 6 || pool.QueryRow(ctx, `SELECT count(*) FILTER(WHERE kind='start'),count(*) FILTER(WHERE kind='continue'),count(*) FILTER(WHERE kind='tool_result') FROM fixture_native_attempts`).Scan(&starts, &continues, &results) != nil || starts != 1 || continues != 1 || results != 1 {
		t.Fatal("native entry method was not charged with carryover")
	}
}
