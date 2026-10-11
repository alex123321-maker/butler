package app

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
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

const nativeDelegationAttemptCap = 12

var errNativeFixtureDenied = errors.New("native delegation fixture admission denied")

// This counter is an additional fixture spending limit, not owner authority or
// a substitute for the actual production root/delegation/day budget ledger.
// It charges every attempted provider call, without replay deduplication.
type nativeDelegationLimiter struct{ pool *pgxpool.Pool }

func newNativeDelegationLimiter(ctx context.Context, pool *pgxpool.Pool) (*nativeDelegationLimiter, error) {
	if pool == nil {
		return nil, errNativeFixtureDenied
	}
	_, err := pool.Exec(ctx, `CREATE TABLE fixture_native_limit(singleton boolean PRIMARY KEY CHECK(singleton),attempts integer NOT NULL CHECK(attempts BETWEEN 0 AND 12));
 INSERT INTO fixture_native_limit VALUES(TRUE,0);
 CREATE TABLE fixture_native_attempts(attempt integer PRIMARY KEY,run_id text NOT NULL,kind text NOT NULL CHECK(kind IN('start','continue','tool_result')),input_hash text NOT NULL CHECK(length(input_hash)=64),created_at timestamptz NOT NULL DEFAULT clock_timestamp())`)
	if err != nil {
		return nil, errNativeFixtureDenied
	}
	return &nativeDelegationLimiter{pool}, nil
}

func (l *nativeDelegationLimiter) reserve(ctx context.Context, kind, runID string, input any) error {
	if l == nil || l.pool == nil || runID == "" || kind != "start" && kind != "continue" && kind != "tool_result" {
		return errNativeFixtureDenied
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
	if tx.QueryRow(ctx, `UPDATE fixture_native_limit SET attempts=attempts+1 WHERE singleton AND attempts<12 RETURNING attempts`).Scan(&attempt) != nil {
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
}
