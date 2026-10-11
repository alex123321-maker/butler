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
	nativeDelegationPriorAttempts     = 13
	nativeDelegationTotalAttemptCap   = nativeDelegationPriorAttempts + nativeDelegationAttemptCap
	nativeDelegationPriorReportSHA256 = "28c75c31827f8a25ca8ed101e6e04b8264a7e66ee9ce18808d504d5af748c8f0"
	nativeDelegationPriorRelease      = "85bef824e838caef64407891d5b228f982c51032"
	nativeDelegationFollowupSHA256    = "f5d25f36b574464508a4124ac7490d525e75d8c8cf60111a0bb637637e777619"
	nativeDelegationFollowupRelease   = "6bf29435cdf74ee143f4d3dd1c12defb2f9d1aa4"
	nativeDelegationThirdSHA256       = "b7a4b35b7b587fb8f0561cbdb18f5ad8088b7939e673ffa290e06ec10410b89b"
	nativeDelegationThirdRelease      = "38e3b87353cd3fbf25fb90e77fcf76b83a4ac09d"
	nativeDelegationFourthSHA256      = "72d95bb5ac0bcf5bd4d8d4b28b1de6beb10d819a348c7e5efb187295ce28a1f9"
	nativeDelegationFourthRelease     = "b91e72d881ea933257110d4452cf8d4b91ae3aba"
)

var errNativeFixtureDenied = errors.New("native delegation fixture admission denied")

// This counter is an additional fixture spending limit, not owner authority or
// a substitute for the actual production root/delegation/day budget ledger.
// It charges every attempted provider call, without replay deduplication.
type nativeDelegationLimiter struct{ pool *pgxpool.Pool }

type nativeHistoricalEvidence struct {
	Status            string                `json:"status"`
	BaselineSHA256    string                `json:"baseline_sha256"`
	Artifacts         []nativePriorArtifact `json:"artifacts"`
	NativeAttempts    int                   `json:"native_attempts"`
	KnownUsagePairs   int                   `json:"known_usage_pairs"`
	UnknownUsagePairs int                   `json:"unknown_usage_pairs"`
	Usage             []nativeUsageEvidence `json:"usage"`
}

type nativePriorArtifact struct {
	ReportSHA256 string `json:"report_sha256"`
	ReleaseRef   string `json:"release_ref"`
	NewAttempts  int    `json:"new_attempts"`
}

// All four committed content-free FAILED artifacts supply historical accounting.
// This fixed source path is kernel fixture configuration, never an environment or
// model-selected path. Missing/changed bytes deny before import or inference.
func nativeHistoryPaths() []string {
	_, file, _, _ := runtime.Caller(0)
	dir := filepath.Join(filepath.Dir(file), "..", "..", "..", "..", "deploy", "delegation")
	return []string{filepath.Join(dir, "native-scenario-failed.json"), filepath.Join(dir, "native-scenario-followup-failed.json"), filepath.Join(dir, "native-scenario-third-failed.json"), filepath.Join(dir, "native-scenario-fourth.json")}
}

func nativeBaselineHash() string {
	hash := sha256.Sum256([]byte(nativeDelegationPriorReportSHA256 + "\n" + nativeDelegationFollowupSHA256 + "\n" + nativeDelegationThirdSHA256 + "\n" + nativeDelegationFourthSHA256 + "\n"))
	return hex.EncodeToString(hash[:])
}

func readNativeArtifact(path, expectedHash string) ([]byte, error) {
	stat, err := os.Lstat(path)
	if err != nil || !stat.Mode().IsRegular() || stat.Size() <= 0 || stat.Size() > 32768 {
		return nil, errNativeFixtureDenied
	}
	f, err := os.Open(path)
	if err != nil {
		return nil, errNativeFixtureDenied
	}
	defer f.Close()
	opened, err := f.Stat()
	if err != nil || !os.SameFile(stat, opened) {
		return nil, errNativeFixtureDenied
	}
	raw, err := io.ReadAll(io.LimitReader(f, 32769))
	hash := sha256.Sum256(raw)
	if err != nil || len(raw) > 32768 || hex.EncodeToString(hash[:]) != expectedHash {
		return nil, errNativeFixtureDenied
	}
	return raw, nil
}

func readNativeHistory(paths ...string) (nativeHistoricalEvidence, error) {
	var h nativeHistoricalEvidence
	if len(paths) != 4 {
		return h, errNativeFixtureDenied
	}
	raw, err := readNativeArtifact(paths[0], nativeDelegationPriorReportSHA256)
	if err != nil {
		return h, err
	}
	var prior struct {
		Status         string                `json:"status"`
		ReleaseRef     string                `json:"release_ref"`
		NativeAttempts int                   `json:"native_attempts"`
		Usage          []nativeUsageEvidence `json:"usage"`
	}
	if json.Unmarshal(raw, &prior) != nil || prior.Status != "failed" || prior.ReleaseRef != nativeDelegationPriorRelease || prior.NativeAttempts != 3 || len(prior.Usage) != 3 {
		return h, errNativeFixtureDenied
	}
	raw, err = readNativeArtifact(paths[1], nativeDelegationFollowupSHA256)
	if err != nil {
		return h, err
	}
	var followup struct {
		Status            string                `json:"status"`
		ReleaseRef        string                `json:"release_ref"`
		NativeAttempts    int                   `json:"native_attempts"`
		NewNativeAttempts int                   `json:"new_native_attempts"`
		NewUsage          []nativeUsageEvidence `json:"new_usage"`
		Historical        struct {
			ReportSHA256   string                `json:"report_sha256"`
			ReleaseRef     string                `json:"release_ref"`
			NativeAttempts int                   `json:"native_attempts"`
			Usage          []nativeUsageEvidence `json:"usage"`
		} `json:"historical"`
	}
	if json.Unmarshal(raw, &followup) != nil || followup.Status != "failed" || followup.ReleaseRef != nativeDelegationFollowupRelease || followup.NativeAttempts != 5 || followup.NewNativeAttempts != 2 || len(followup.NewUsage) != 2 || followup.Historical.ReportSHA256 != nativeDelegationPriorReportSHA256 || followup.Historical.ReleaseRef != nativeDelegationPriorRelease || followup.Historical.NativeAttempts != 3 {
		return h, errNativeFixtureDenied
	}
	oldUsage, _ := json.Marshal(prior.Usage)
	repeatedUsage, _ := json.Marshal(followup.Historical.Usage)
	if string(oldUsage) != string(repeatedUsage) {
		return h, errNativeFixtureDenied
	}
	oldBaseline := sha256.Sum256([]byte(nativeDelegationPriorReportSHA256 + "\n" + nativeDelegationFollowupSHA256 + "\n"))
	h = nativeHistoricalEvidence{Status: "failed", BaselineSHA256: hex.EncodeToString(oldBaseline[:]), Artifacts: []nativePriorArtifact{{nativeDelegationPriorReportSHA256, nativeDelegationPriorRelease, 3}, {nativeDelegationFollowupSHA256, nativeDelegationFollowupRelease, 2}}, NativeAttempts: 5, KnownUsagePairs: 1, UnknownUsagePairs: 4, Usage: append(prior.Usage, followup.NewUsage...)}
	raw, err = readNativeArtifact(paths[2], nativeDelegationThirdSHA256)
	if err != nil {
		return nativeHistoricalEvidence{}, err
	}
	var third struct {
		Status            string                   `json:"status"`
		ReleaseRef        string                   `json:"release_ref"`
		NativeAttempts    int                      `json:"native_attempts"`
		NewNativeAttempts int                      `json:"new_native_attempts"`
		NewUsage          []nativeUsageEvidence    `json:"new_usage"`
		Historical        nativeHistoricalEvidence `json:"historical"`
	}
	if json.Unmarshal(raw, &third) != nil || third.Status != "failed" || third.ReleaseRef != nativeDelegationThirdRelease || third.NativeAttempts != 9 || third.NewNativeAttempts != 4 || len(third.NewUsage) != 4 {
		return nativeHistoricalEvidence{}, errNativeFixtureDenied
	}
	expectedHistory, _ := json.Marshal(h)
	repeatedHistory, _ := json.Marshal(third.Historical)
	if string(expectedHistory) != string(repeatedHistory) {
		return nativeHistoricalEvidence{}, errNativeFixtureDenied
	}
	thirdBaseline := sha256.Sum256([]byte(nativeDelegationPriorReportSHA256 + "\n" + nativeDelegationFollowupSHA256 + "\n" + nativeDelegationThirdSHA256 + "\n"))
	h.BaselineSHA256, h.NativeAttempts = hex.EncodeToString(thirdBaseline[:]), 9
	h.Artifacts = append(h.Artifacts, nativePriorArtifact{nativeDelegationThirdSHA256, nativeDelegationThirdRelease, 4})
	h.Usage = append(h.Usage, third.NewUsage...)
	h.KnownUsagePairs, h.UnknownUsagePairs = 0, 0
	for _, usage := range h.Usage {
		if usage.ActualInputTokens == nil && usage.ActualOutputTokens == nil {
			h.UnknownUsagePairs++
		} else if usage.ActualInputTokens != nil && usage.ActualOutputTokens != nil {
			h.KnownUsagePairs++
		} else {
			return nativeHistoricalEvidence{}, errNativeFixtureDenied
		}
	}
	if h.KnownUsagePairs != 2 || h.UnknownUsagePairs != 7 {
		return nativeHistoricalEvidence{}, errNativeFixtureDenied
	}
	raw, err = readNativeArtifact(paths[3], nativeDelegationFourthSHA256)
	if err != nil {
		return nativeHistoricalEvidence{}, err
	}
	var fourth struct {
		Status            string                   `json:"status"`
		ReleaseRef        string                   `json:"release_ref"`
		NativeAttempts    int                      `json:"native_attempts"`
		NewNativeAttempts int                      `json:"new_native_attempts"`
		NewUsage          []nativeUsageEvidence    `json:"new_usage"`
		Historical        nativeHistoricalEvidence `json:"historical"`
	}
	if json.Unmarshal(raw, &fourth) != nil || fourth.Status != "failed" || fourth.ReleaseRef != nativeDelegationFourthRelease || fourth.NativeAttempts != 13 || fourth.NewNativeAttempts != 4 || len(fourth.NewUsage) != 4 {
		return nativeHistoricalEvidence{}, errNativeFixtureDenied
	}
	expectedHistory, _ = json.Marshal(h)
	repeatedHistory, _ = json.Marshal(fourth.Historical)
	if string(expectedHistory) != string(repeatedHistory) {
		return nativeHistoricalEvidence{}, errNativeFixtureDenied
	}
	h.BaselineSHA256, h.NativeAttempts = nativeBaselineHash(), nativeDelegationPriorAttempts
	h.Artifacts = append(h.Artifacts, nativePriorArtifact{nativeDelegationFourthSHA256, nativeDelegationFourthRelease, 4})
	h.Usage = append(h.Usage, fourth.NewUsage...)
	h.KnownUsagePairs, h.UnknownUsagePairs = 0, 0
	for _, usage := range h.Usage {
		if usage.ActualInputTokens == nil && usage.ActualOutputTokens == nil {
			h.UnknownUsagePairs++
		} else if usage.ActualInputTokens != nil && usage.ActualOutputTokens != nil {
			h.KnownUsagePairs++
		} else {
			return nativeHistoricalEvidence{}, errNativeFixtureDenied
		}
	}
	if h.KnownUsagePairs != 3 || h.UnknownUsagePairs != 10 {
		return nativeHistoricalEvidence{}, errNativeFixtureDenied
	}
	return h, nil
}

func newNativeDelegationLimiter(ctx context.Context, pool *pgxpool.Pool) (*nativeDelegationLimiter, error) {
	if pool == nil {
		return nil, errNativeFixtureDenied
	}
	if _, err := readNativeHistory(nativeHistoryPaths()...); err != nil {
		return nil, err
	}
	_, err := pool.Exec(ctx, `CREATE TABLE fixture_native_limit(singleton boolean PRIMARY KEY CHECK(singleton),attempts integer NOT NULL CHECK(attempts BETWEEN 13 AND 25),prior_attempts integer NOT NULL CHECK(prior_attempts=13),hard_cap integer NOT NULL CHECK(hard_cap=25),new_cap integer NOT NULL CHECK(new_cap=12),first_report_sha256 text NOT NULL CHECK(first_report_sha256='28c75c31827f8a25ca8ed101e6e04b8264a7e66ee9ce18808d504d5af748c8f0'),followup_report_sha256 text NOT NULL CHECK(followup_report_sha256='f5d25f36b574464508a4124ac7490d525e75d8c8cf60111a0bb637637e777619'),third_report_sha256 text NOT NULL CHECK(third_report_sha256='b7a4b35b7b587fb8f0561cbdb18f5ad8088b7939e673ffa290e06ec10410b89b'),fourth_report_sha256 text NOT NULL CHECK(fourth_report_sha256='72d95bb5ac0bcf5bd4d8d4b28b1de6beb10d819a348c7e5efb187295ce28a1f9'));
 INSERT INTO fixture_native_limit VALUES(TRUE,13,13,25,12,'28c75c31827f8a25ca8ed101e6e04b8264a7e66ee9ce18808d504d5af748c8f0','f5d25f36b574464508a4124ac7490d525e75d8c8cf60111a0bb637637e777619','b7a4b35b7b587fb8f0561cbdb18f5ad8088b7939e673ffa290e06ec10410b89b','72d95bb5ac0bcf5bd4d8d4b28b1de6beb10d819a348c7e5efb187295ce28a1f9');
 CREATE FUNCTION fixture_native_limit_immutable() RETURNS trigger LANGUAGE plpgsql AS $$ BEGIN
 IF NEW.singleton IS DISTINCT FROM OLD.singleton OR NEW.prior_attempts IS DISTINCT FROM OLD.prior_attempts OR NEW.hard_cap IS DISTINCT FROM OLD.hard_cap OR NEW.new_cap IS DISTINCT FROM OLD.new_cap OR NEW.first_report_sha256 IS DISTINCT FROM OLD.first_report_sha256 OR NEW.followup_report_sha256 IS DISTINCT FROM OLD.followup_report_sha256 OR NEW.third_report_sha256 IS DISTINCT FROM OLD.third_report_sha256 OR NEW.fourth_report_sha256 IS DISTINCT FROM OLD.fourth_report_sha256 OR NEW.attempts<OLD.attempts THEN RAISE EXCEPTION 'immutable fixture baseline'; END IF; RETURN NEW; END $$;
 CREATE TRIGGER fixture_native_limit_guard BEFORE UPDATE ON fixture_native_limit FOR EACH ROW EXECUTE FUNCTION fixture_native_limit_immutable();
 CREATE TABLE fixture_native_attempts(attempt integer PRIMARY KEY CHECK(attempt BETWEEN 14 AND 25),run_id text NOT NULL,kind text NOT NULL CHECK(kind IN('start','continue','tool_result')),input_hash text NOT NULL CHECK(length(input_hash)=64),created_at timestamptz NOT NULL DEFAULT clock_timestamp())`)
	if err != nil {
		return nil, errNativeFixtureDenied
	}
	return &nativeDelegationLimiter{pool}, nil
}

func (l *nativeDelegationLimiter) reserve(ctx context.Context, kind, runID string, input any) error {
	return l.reserveWithHistory(ctx, kind, runID, input, nativeHistoryPaths())
}

// The separate path argument permits adversarial no-inference fixture tests.
// All bytes must still match the fixed committed hashes; it cannot supply an
// alternative baseline or increase allowance. The actual wrapper uses reserve.
func (l *nativeDelegationLimiter) reserveWithHistory(ctx context.Context, kind, runID string, input any, paths []string) error {
	if l == nil || l.pool == nil || runID == "" || kind != "start" && kind != "continue" && kind != "tool_result" {
		return errNativeFixtureDenied
	}
	if _, err := readNativeHistory(paths...); err != nil {
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
	if tx.QueryRow(ctx, `UPDATE fixture_native_limit SET attempts=attempts+1 WHERE singleton AND prior_attempts=$1 AND hard_cap=$2 AND new_cap=$3 AND first_report_sha256=$4 AND followup_report_sha256=$5 AND third_report_sha256=$6 AND fourth_report_sha256=$7 AND attempts<hard_cap AND attempts-prior_attempts<new_cap RETURNING attempts`, nativeDelegationPriorAttempts, nativeDelegationTotalAttemptCap, nativeDelegationAttemptCap, nativeDelegationPriorReportSHA256, nativeDelegationFollowupSHA256, nativeDelegationThirdSHA256, nativeDelegationFourthSHA256).Scan(&attempt) != nil {
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
	var firstBaseline, followupBaseline, thirdBaseline, fourthBaseline string
	if pool.QueryRow(ctx, `SELECT attempts,prior_attempts,hard_cap,new_cap,first_report_sha256,followup_report_sha256,third_report_sha256,fourth_report_sha256 FROM fixture_native_limit`).Scan(&initial, &prior, &cap, &remaining, &firstBaseline, &followupBaseline, &thirdBaseline, &fourthBaseline) != nil || initial != 13 || prior != 13 || cap != 25 || remaining != 12 || firstBaseline != nativeDelegationPriorReportSHA256 || followupBaseline != nativeDelegationFollowupSHA256 || thirdBaseline != nativeDelegationThirdSHA256 || fourthBaseline != nativeDelegationFourthSHA256 {
		t.Fatal("prior spending or remaining allowance lost")
	}
	for _, query := range []string{`UPDATE fixture_native_limit SET attempts=0`, `UPDATE fixture_native_limit SET prior_attempts=0`, `UPDATE fixture_native_limit SET hard_cap=26`, `UPDATE fixture_native_limit SET new_cap=13`, `UPDATE fixture_native_limit SET first_report_sha256=repeat('f',64)`, `UPDATE fixture_native_limit SET followup_report_sha256=repeat('f',64)`, `UPDATE fixture_native_limit SET third_report_sha256=repeat('f',64)`, `UPDATE fixture_native_limit SET fourth_report_sha256=repeat('f',64)`} {
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
	if pool.QueryRow(ctx, `SELECT attempts FROM fixture_native_limit`).Scan(&total) != nil || total != nativeDelegationTotalAttemptCap || pool.QueryRow(ctx, `SELECT min(attempt),max(attempt) FROM fixture_native_attempts`).Scan(&first, &last) != nil || first != 14 || last != 25 {
		t.Fatal("historical attempts were reset or copied as new attempts")
	}
}

func TestNativeDelegationCarryoverBaselineRejectsMismatch(t *testing.T) {
	paths := nativeHistoryPaths()
	h, err := readNativeHistory(paths...)
	if err != nil || h.NativeAttempts != 13 || h.BaselineSHA256 != nativeBaselineHash() || len(h.Artifacts) != 4 || h.Artifacts[0].ReportSHA256 != nativeDelegationPriorReportSHA256 || h.Artifacts[1].ReportSHA256 != nativeDelegationFollowupSHA256 || len(h.Usage) != 13 || h.KnownUsagePairs != 3 || h.UnknownUsagePairs != 10 || h.Usage[0].ActualInputTokens != nil || h.Usage[0].ActualOutputTokens != nil || h.Usage[1].ActualInputTokens != nil || h.Usage[1].ActualOutputTokens != nil || h.Usage[2].ActualInputTokens == nil || *h.Usage[2].ActualInputTokens != 5526 || h.Usage[2].ActualOutputTokens == nil || *h.Usage[2].ActualOutputTokens != 101 || h.Usage[3].ActualInputTokens != nil || h.Usage[3].ActualOutputTokens != nil || h.Usage[4].ActualInputTokens != nil || h.Usage[4].ActualOutputTokens != nil {
		t.Fatal("known failed spending or nullable usage was changed")
	}
	var knownInput, knownOutput int64
	for _, u := range h.Usage {
		if u.ActualInputTokens != nil {
			knownInput += *u.ActualInputTokens
		}
		if u.ActualOutputTokens != nil {
			knownOutput += *u.ActualOutputTokens
		}
	}
	if h.Artifacts[2].ReportSHA256 != nativeDelegationThirdSHA256 || h.Artifacts[2].ReleaseRef != nativeDelegationThirdRelease || h.Artifacts[2].NewAttempts != 4 || h.Artifacts[3].ReportSHA256 != nativeDelegationFourthSHA256 || h.Artifacts[3].ReleaseRef != nativeDelegationFourthRelease || h.Artifacts[3].NewAttempts != 4 || knownInput != 16810 || knownOutput != 280 {
		t.Fatal("fourth failed history or known subtotal was changed")
	}
	for index, original := range paths {
		for _, name := range []string{"missing", "changed-count", "changed-release", "oversize", "symlink", "reversed-order"} {
			t.Run([]string{"first/", "followup/", "third/", "fourth/"}[index]+name, func(t *testing.T) {
				raw, err := os.ReadFile(original)
				if err != nil {
					t.Fatal("committed baseline unavailable")
				}
				path := filepath.Join(t.TempDir(), "baseline.json")
				b := raw
				switch name {
				case "changed-count":
					b = []byte(strings.Replace(string(raw), `"native_attempts":`, `"altered_attempts":`, 1))
				case "changed-release":
					b = []byte(strings.Replace(string(raw), `"release_ref":`, `"altered_release":`, 1))
				case "oversize":
					b = []byte(strings.Repeat("x", 32769))
				case "symlink":
					if os.Symlink(original, path) != nil {
						t.Fatal("fixture symlink creation failed")
					}
				}
				if name != "missing" && name != "symlink" && os.WriteFile(path, b, 0600) != nil {
					t.Fatal("fixture baseline creation failed")
				}
				pair := append([]string(nil), paths...)
				pair[index] = path
				if name == "reversed-order" {
					pair = []string{paths[1], paths[0]}
				}
				if _, err = readNativeHistory(pair...); !errors.Is(err, errNativeFixtureDenied) {
					t.Fatal("missing/forged prior experiment accepted")
				}
			})
		}
	}
}

func TestNativeDelegationEveryReservationRevalidatesAllArtifactsIntegration(t *testing.T) {
	pool, ctx := memorytest.Database(t), context.Background()
	limit, err := newNativeDelegationLimiter(ctx, pool)
	if err != nil {
		t.Fatal("private counter initialization denied")
	}
	paths := nativeHistoryPaths()
	for index, original := range paths {
		t.Run([]string{"first", "followup", "third", "fourth"}[index], func(t *testing.T) {
			raw, err := os.ReadFile(original)
			if err != nil {
				t.Fatal("committed baseline unavailable")
			}
			path := filepath.Join(t.TempDir(), "baseline.json")
			pair := append([]string(nil), paths...)
			pair[index] = path
			if os.WriteFile(path, raw, 0600) != nil || limit.reserveWithHistory(ctx, "start", "synthetic-before-baseline-change", nil, pair) != nil {
				t.Fatal("exact pinned artifact copy denied")
			}
			if os.WriteFile(path, append(raw, '\n'), 0600) != nil {
				t.Fatal("fixture artifact replacement failed")
			}
			for _, kind := range []string{"start", "continue", "tool_result"} {
				if limit.reserveWithHistory(ctx, kind, "synthetic-after-baseline-change", nil, pair) == nil {
					t.Fatal("changed artifact bypassed per-entry validation")
				}
			}
		})
	}
	var total, entries int
	if pool.QueryRow(ctx, `SELECT attempts FROM fixture_native_limit`).Scan(&total) != nil || total != 17 || pool.QueryRow(ctx, `SELECT count(*) FROM fixture_native_attempts`).Scan(&entries) != nil || entries != 4 {
		t.Fatal("artifact denial spent or reset accounting")
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
	if pool.QueryRow(ctx, `SELECT attempts FROM fixture_native_limit`).Scan(&total) != nil || total != 16 || pool.QueryRow(ctx, `SELECT count(*) FILTER(WHERE kind='start'),count(*) FILTER(WHERE kind='continue'),count(*) FILTER(WHERE kind='tool_result') FROM fixture_native_attempts`).Scan(&starts, &continues, &results) != nil || starts != 1 || continues != 1 || results != 1 {
		t.Fatal("native entry method was not charged with carryover")
	}
}
