package app

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"io"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/butler/butler/internal/memory/memorytest"
	"github.com/butler/butler/internal/transport/antigravity"
	"github.com/jackc/pgx/v5/pgxpool"
)

const (
	highRemainderPriorAttempts    = 37
	highRemainderPriorReport      = "afa63de06b6991feb3a96b322d442d573d4d2834c51a70c07e5fd4419f751a91"
	highRemainderPriorRelease     = "b2100bd2bcd4147b1168a252cd7369c3415c5140"
	highRemainderPreflightReport  = "ce8835a6fefcd600c6b28b88fed73212453c5af54016bdd6d8c77c4e506dbeae"
	highRemainderPreflightRelease = "a473dbd124a8b437de3058e81e0b76455b57bddb"
)

func highRemainderDelegationWindow() bool {
	return os.Getenv("BUTLER_DELEGATION_HIGH_REMAINDER_WINDOW") == "1"
}

func highRemainderDelegationHistoryPaths() []string {
	paths := fullHighDelegationHistoryPaths()
	dir := filepath.Dir(paths[9])
	return append(paths, filepath.Join(dir, "native-acceptance-high-20261012.json"), filepath.Join(dir, "native-remainder-high-preflight-20261012.json"))
}

// The exact full High report is 43,072 bytes. Its larger content-free audit
// needs a separate bound; historical readers retain their original 32 KiB cap.
func readHighRemainderPriorArtifact(path, expectedHash string) ([]byte, error) {
	const limit = 65536
	stat, err := os.Lstat(path)
	if err != nil || !stat.Mode().IsRegular() || stat.Size() <= 0 || stat.Size() > limit {
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
	raw, err := io.ReadAll(io.LimitReader(f, limit+1))
	hash := sha256.Sum256(raw)
	if err != nil || len(raw) > limit || hex.EncodeToString(hash[:]) != expectedHash {
		return nil, errNativeFixtureDenied
	}
	return raw, nil
}

// Carry every spent attempt, including the failed width2 overlap gate. This
// does not promote the failed report or its model finals to owner completion.
func readHighRemainderDelegationHistory(paths []string) (nativeHistoricalEvidence, error) {
	if len(paths) != 12 {
		return nativeHistoricalEvidence{}, errNativeFixtureDenied
	}
	h, err := readFullHighDelegationHistory(paths[:10])
	if err != nil {
		return nativeHistoricalEvidence{}, errNativeFixtureDenied
	}
	raw, err := readHighRemainderPriorArtifact(paths[10], highRemainderPriorReport)
	var previous nativeScenarioEvidence
	if err != nil || json.Unmarshal(raw, &previous) != nil || previous.Status != "failed" || previous.Model != antigravity.FlashHighModel || previous.ReleaseRef != highRemainderPriorRelease || previous.NativeAttempts != 37 || previous.NewNativeAttempts != 10 || previous.MaxAttempts != 39 || previous.MaxNewAttempts != 12 || len(previous.Attempts) != 10 || len(previous.Usage) != 10 || len(previous.Widths) != 2 || string(nativeJSON(previous.Historical)) != string(nativeJSON(h)) || previous.OwnerStopDenies || previous.RevisionDenies || previous.DeletionDenies {
		return nativeHistoricalEvidence{}, errNativeFixtureDenied
	}
	for i, w := range previous.Widths {
		a := w.FullAcceptance
		if w.Width != i+1 || !w.ParentWorkerKilled || !w.ParentReceiptCheck || len(w.ObjectiveChecks) != 2 || !w.ObjectiveChecks[0] || !w.ObjectiveChecks[1] || w.MaximumChildOverlap != 1 || len(w.ChildTaskIDs) != 2 || len(w.ChildRunIDs) != 2 || len(w.ChildResultIDs) != 2 || a == nil || !a.ParentFinalAvailable || !a.ParentFinalUnverified || !a.ParentFinalComplete || a.ParentFinalPrivacyRedacted || a.ParentReceipt == nil || a.ParentReceipt.Reason != "passed" || !a.ParentReceipt.IDsMatch || a.ResolutionPolicy != "owner_selected_source_a_for_fixture" {
			return nativeHistoricalEvidence{}, errNativeFixtureDenied
		}
		if i == 0 {
			if a.Stage != "independent_decisions_checked" || !a.StructuredOwningReportsChecked || !a.IndependentOriginalChecks || !a.ConflictingOriginalsRetained || a.AcceptedChild != w.ChildTaskIDs[0] || a.RejectedChild != w.ChildTaskIDs[1] || !a.PreviousResultAndReportKept || !a.OwnerAPIAuthenticated || !a.ChildFinalsStayUnverified || !a.ChildTasksStayActive || !w.ParentTaskStillActive || !w.IndependentObservationsKept || !w.UnverifiedAcceptanceDenied {
				return nativeHistoricalEvidence{}, errNativeFixtureDenied
			}
		} else if a.Stage != "parent_receipts_checked" || a.StructuredOwningReportsChecked || a.IndependentOriginalChecks || a.ConflictingOriginalsRetained || a.OwnerAPIAuthenticated || a.PreviousResultAndReportKept || a.ChildFinalsStayUnverified || a.ChildTasksStayActive || a.AcceptedChild != "" || a.RejectedChild != "" || w.ParentTaskStillActive || w.IndependentObservationsKept {
			return nativeHistoricalEvidence{}, errNativeFixtureDenied
		}
	}
	for i, attempt := range previous.Attempts {
		if attempt.Attempt != 28+i || attempt.RunID == "" || attempt.Kind != "start" && attempt.Kind != "tool_result" || len(attempt.InputHash) != 64 {
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
	if len(h.Usage) != 37 || h.KnownUsagePairs != 17 || h.UnknownUsagePairs != 20 || input != 62760 || output != 1810 {
		return nativeHistoricalEvidence{}, errNativeFixtureDenied
	}
	baseline := sha256.Sum256([]byte(h.BaselineSHA256 + "\n" + highRemainderPriorReport + "\n"))
	h.BaselineSHA256 = hex.EncodeToString(baseline[:])
	h.NativeAttempts = highRemainderPriorAttempts
	h.Artifacts = append(h.Artifacts, nativePriorArtifact{ReportSHA256: highRemainderPriorReport, ReleaseRef: previous.ReleaseRef, NewAttempts: 10})
	// The sealed first remainder failed during scripted width2 parent admission
	// without entering a native child. Keep that failure and its separate
	// component scope; neither its script turns nor usage become native calls.
	raw, err = readHighRemainderPriorArtifact(paths[11], highRemainderPreflightReport)
	var preflight nativeScenarioEvidence
	if err != nil || json.Unmarshal(raw, &preflight) != nil || preflight.Status != "failed" || preflight.Model != antigravity.FlashHighModel || preflight.ReleaseRef != highRemainderPreflightRelease || preflight.ExecutionMode != "scripted_parents_and_width1_baseline_only_width2_children_native_high" || preflight.ScriptedModelTurns == nil || *preflight.ScriptedModelTurns != 6 || preflight.NativeAttempts != 37 || preflight.NewNativeAttempts != 0 || preflight.MaxAttempts != 39 || preflight.MaxNewAttempts != 2 || len(preflight.Attempts) != 0 || len(preflight.Usage) != 0 || len(preflight.Widths) != 2 || string(nativeJSON(preflight.Historical)) != string(nativeJSON(h)) || preflight.OwnerStopDenies || preflight.RevisionDenies || preflight.DeletionDenies || len(preflight.ControlScopes) != 0 || preflight.ConfirmedEffectsAfterControls != nil {
		return nativeHistoricalEvidence{}, errNativeFixtureDenied
	}
	w := preflight.Widths[0]
	a := w.FullAcceptance
	if w.Width != 1 || w.ExecutionMode != "scripted_parent_and_children_component_baseline" || !w.ParentWorkerKilled || !w.ParentReceiptCheck || len(w.ObjectiveChecks) != 2 || !w.ObjectiveChecks[0] || !w.ObjectiveChecks[1] || w.MaximumChildOverlap != 1 || len(w.ChildTaskIDs) != 2 || len(w.ChildRunIDs) != 2 || len(w.ChildResultIDs) != 2 || !w.ParentTaskStillActive || !w.IndependentObservationsKept || !w.UnverifiedAcceptanceDenied || a == nil || a.Stage != "independent_decisions_checked" || a.ResolutionPolicy != "owner_selected_source_a_for_fixture" || !a.ParentFinalAvailable || !a.ParentFinalUnverified || !a.ParentFinalComplete || a.ParentFinalPrivacyRedacted || a.ParentReceipt == nil || a.ParentReceipt.Reason != "passed" || !a.ParentReceipt.IDsMatch || !a.StructuredOwningReportsChecked || !a.IndependentOriginalChecks || !a.ConflictingOriginalsRetained || a.AcceptedChild != w.ChildTaskIDs[0] || a.RejectedChild != w.ChildTaskIDs[1] || !a.PreviousResultAndReportKept || !a.OwnerAPIAuthenticated || !a.ChildFinalsStayUnverified || !a.ChildTasksStayActive {
		return nativeHistoricalEvidence{}, errNativeFixtureDenied
	}
	w = preflight.Widths[1]
	a = w.FullAcceptance
	if w.Width != 2 || w.ExecutionMode != "scripted_parent_native_high_children" || w.ParentWorkerKilled || w.ParentReceiptCheck || w.ParentRunID != "" || len(w.ObjectiveChecks) != 0 || w.MaximumChildOverlap != 0 || len(w.ChildTaskIDs) != 0 || len(w.ChildRunIDs) != 0 || len(w.ChildResultIDs) != 0 || w.ParentTaskStillActive || w.IndependentObservationsKept || w.UnverifiedAcceptanceDenied || a == nil || a.Stage != "parent_admission" || a.ResolutionPolicy != "owner_selected_source_a_for_fixture" || a.ParentFinalAvailable || a.ParentFinalUnverified || a.ParentFinalComplete || a.ParentFinalPrivacyRedacted || a.ParentReceipt != nil || a.StructuredOwningReportsChecked || a.IndependentOriginalChecks || a.ConflictingOriginalsRetained || a.AcceptedChild != "" || a.RejectedChild != "" || a.PreviousResultAndReportKept || a.OwnerAPIAuthenticated || a.ChildFinalsStayUnverified || a.ChildTasksStayActive {
		return nativeHistoricalEvidence{}, errNativeFixtureDenied
	}
	baseline = sha256.Sum256([]byte(h.BaselineSHA256 + "\n" + highRemainderPreflightReport + "\n"))
	h.BaselineSHA256 = hex.EncodeToString(baseline[:])
	h.Artifacts = append(h.Artifacts, nativePriorArtifact{ReportSHA256: highRemainderPreflightReport, ReleaseRef: preflight.ReleaseRef, NewAttempts: 0})
	return h, nil
}

func newHighRemainderDelegationLimiter(ctx context.Context, pool *pgxpool.Pool) (*nativeDelegationLimiter, error) {
	h, err := readHighRemainderDelegationHistory(highRemainderDelegationHistoryPaths())
	if pool == nil || err != nil {
		return nil, errNativeFixtureDenied
	}
	_, err = pool.Exec(ctx, `CREATE TABLE IF NOT EXISTS fixture_native_limit (
 singleton boolean PRIMARY KEY CHECK(singleton), attempts integer NOT NULL CHECK(attempts BETWEEN 37 AND 39),
 prior_attempts integer NOT NULL CHECK(prior_attempts=37), hard_cap integer NOT NULL CHECK(hard_cap=39),
 new_cap integer NOT NULL CHECK(new_cap=2), baseline_hash text NOT NULL CHECK(length(baseline_hash)=64),
 expires_at timestamptz NOT NULL);
 CREATE TABLE IF NOT EXISTS fixture_native_attempts(attempt integer PRIMARY KEY CHECK(attempt BETWEEN 38 AND 39),
 run_id text NOT NULL,kind text NOT NULL CHECK(kind IN ('start','continue','tool_result')),
 input_hash text NOT NULL CHECK(length(input_hash)=64),created_at timestamptz NOT NULL DEFAULT clock_timestamp());
 CREATE OR REPLACE FUNCTION fixture_high_remainder_immutable() RETURNS trigger LANGUAGE plpgsql AS $$ BEGIN
 IF TG_OP='DELETE' THEN RAISE EXCEPTION 'immutable High remainder'; END IF;
 IF NEW.singleton IS DISTINCT FROM OLD.singleton OR NEW.prior_attempts IS DISTINCT FROM OLD.prior_attempts
 OR NEW.hard_cap IS DISTINCT FROM OLD.hard_cap OR NEW.new_cap IS DISTINCT FROM OLD.new_cap
 OR NEW.baseline_hash IS DISTINCT FROM OLD.baseline_hash OR NEW.expires_at IS DISTINCT FROM OLD.expires_at
 OR NEW.attempts<OLD.attempts THEN RAISE EXCEPTION 'immutable High remainder'; END IF; RETURN NEW; END $$;
 DROP TRIGGER IF EXISTS fixture_high_remainder_guard ON fixture_native_limit;
 CREATE TRIGGER fixture_high_remainder_guard BEFORE UPDATE OR DELETE ON fixture_native_limit FOR EACH ROW EXECUTE FUNCTION fixture_high_remainder_immutable();`)
	if err == nil {
		_, err = pool.Exec(ctx, `INSERT INTO fixture_native_limit VALUES(TRUE,37,37,39,2,$1,clock_timestamp()+interval '18 minutes') ON CONFLICT DO NOTHING`, h.BaselineSHA256)
	}
	var valid bool
	if err != nil || pool.QueryRow(ctx, `SELECT prior_attempts=37 AND hard_cap=39 AND new_cap=2 AND baseline_hash=$1 FROM fixture_native_limit WHERE singleton`, h.BaselineSHA256).Scan(&valid) != nil || !valid {
		return nil, errNativeFixtureDenied
	}
	return &nativeDelegationLimiter{pool}, nil
}

func (l *nativeDelegationLimiter) reserveHighRemainder(ctx context.Context, kind, runID string, input any) error {
	return l.reserveHighRemainderPaths(ctx, kind, runID, input, highRemainderDelegationHistoryPaths())
}

func (l *nativeDelegationLimiter) reserveHighRemainderPaths(ctx context.Context, kind, runID string, input any, paths []string) error {
	if l == nil || l.pool == nil || runID == "" || kind != "start" && kind != "continue" && kind != "tool_result" {
		return errNativeFixtureDenied
	}
	h, err := readHighRemainderDelegationHistory(paths)
	if err != nil {
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
	var ordinal int
	if tx.QueryRow(ctx, `UPDATE fixture_native_limit SET attempts=attempts+1 WHERE singleton AND prior_attempts=37 AND hard_cap=39 AND new_cap=2 AND baseline_hash=$1 AND attempts<hard_cap AND expires_at>clock_timestamp() RETURNING attempts`, h.BaselineSHA256).Scan(&ordinal) != nil {
		return errNativeFixtureDenied
	}
	// A predicate evaluated before an unchanged-row wait is insufficient. Read
	// the database clock while holding the row before committing either write.
	var withinWindow bool
	if tx.QueryRow(ctx, `SELECT expires_at>clock_timestamp() FROM fixture_native_limit WHERE singleton`).Scan(&withinWindow) != nil || !withinWindow {
		return errNativeFixtureDenied
	}
	if _, err = tx.Exec(ctx, `INSERT INTO fixture_native_attempts(attempt,run_id,kind,input_hash) VALUES($1,$2,$3,$4)`, ordinal, runID, kind, hex.EncodeToString(hash[:])); err != nil || tx.Commit(ctx) != nil {
		return errNativeFixtureDenied
	}
	return nil
}

func TestHighRemainderDelegationHistory(t *testing.T) {
	h, err := readHighRemainderDelegationHistory(highRemainderDelegationHistoryPaths())
	if err != nil || h.NativeAttempts != 37 || len(h.Artifacts) != 12 || h.Artifacts[11].NewAttempts != 0 || h.Artifacts[11].ReportSHA256 != highRemainderPreflightReport || len(h.Usage) != 37 || h.KnownUsagePairs != 17 || h.UnknownUsagePairs != 20 {
		t.Fatal("High remainder history unavailable")
	}
	if _, err = readHighRemainderDelegationHistory(highRemainderDelegationHistoryPaths()[:11]); err != errNativeFixtureDenied {
		t.Fatal("High remainder accepted incomplete carried history")
	}
}

func TestHighRemainderDelegationLimitIntegration(t *testing.T) {
	t.Setenv("BUTLER_DELEGATION_HIGH_REMAINDER_WINDOW", "1")
	pool, ctx := memorytest.Database(t), context.Background()
	l, err := newHighRemainderDelegationLimiter(ctx, pool)
	if err != nil {
		t.Fatal("High remainder limiter unavailable")
	}
	var originalExpiry time.Time
	if pool.QueryRow(ctx, `SELECT expires_at FROM fixture_native_limit`).Scan(&originalExpiry) != nil {
		t.Fatal("High remainder deadline unavailable")
	}
	for _, query := range []string{"UPDATE fixture_native_limit SET attempts=36", "UPDATE fixture_native_limit SET hard_cap=40", "UPDATE fixture_native_limit SET new_cap=3", "UPDATE fixture_native_limit SET prior_attempts=36", "UPDATE fixture_native_limit SET expires_at=expires_at+interval '1 minute'", "UPDATE fixture_native_limit SET baseline_hash=repeat('f',64)", "DELETE FROM fixture_native_limit"} {
		if _, err = pool.Exec(ctx, query); err == nil {
			t.Fatal("High remainder immutable budget changed")
		}
	}
	var wg sync.WaitGroup
	allowed := make(chan bool, 16)
	for i := 0; i < 16; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			allowed <- l.reserveHighRemainder(ctx, "start", "synthetic-remainder-run", nil) == nil
		}()
	}
	wg.Wait()
	close(allowed)
	var n int
	for ok := range allowed {
		if ok {
			n++
		}
	}
	if n != 2 {
		t.Fatal("High remainder concurrent cap failed", n)
	}
	l, err = newHighRemainderDelegationLimiter(ctx, pool)
	if err != nil {
		t.Fatal("High remainder reconstruction failed")
	}
	for _, kind := range []string{"start", "continue", "tool_result"} {
		if l.reserveHighRemainder(ctx, kind, "synthetic-remainder-run", nil) != errNativeFixtureDenied {
			t.Fatal("High remainder reconstruction refunded spending")
		}
	}
	var first, last, count, attempts int
	var reconstructedExpiry time.Time
	if pool.QueryRow(ctx, `SELECT min(attempt),max(attempt),count(*) FROM fixture_native_attempts`).Scan(&first, &last, &count) != nil || first != 38 || last != 39 || count != 2 || pool.QueryRow(ctx, `SELECT attempts,expires_at FROM fixture_native_limit`).Scan(&attempts, &reconstructedExpiry) != nil || attempts != 39 || !originalExpiry.Equal(reconstructedExpiry) {
		t.Fatal("High remainder ordinals or immutable deadline invalid")
	}
}

func TestHighRemainderDelegationRevalidationIntegration(t *testing.T) {
	pool, ctx := memorytest.Database(t), context.Background()
	l, err := newHighRemainderDelegationLimiter(ctx, pool)
	if err != nil {
		t.Fatal("High remainder limiter unavailable")
	}
	for i := range highRemainderDelegationHistoryPaths() {
		for _, mode := range []string{"missing", "tampered"} {
			paths := highRemainderDelegationHistoryPaths()
			paths[i] = filepath.Join(t.TempDir(), mode)
			if mode == "tampered" && os.WriteFile(paths[i], []byte(`{"status":"passed"}`), 0600) != nil {
				t.Fatal("tampered history fixture unavailable")
			}
			for _, kind := range []string{"start", "continue", "tool_result"} {
				if l.reserveHighRemainderPaths(ctx, kind, "synthetic-remainder-run", nil, paths) != errNativeFixtureDenied {
					t.Fatal("missing or tampered carried history admitted", i, mode, kind)
				}
			}
		}
	}
	for _, invalid := range []struct {
		kind, run string
		input     any
	}{{"invalid", "run", nil}, {"start", "", nil}, {"start", "run", make(chan int)}} {
		if l.reserveHighRemainder(ctx, invalid.kind, invalid.run, invalid.input) != errNativeFixtureDenied {
			t.Fatal("invalid High remainder entry admitted")
		}
	}
	var attempts, rows int
	if pool.QueryRow(ctx, `SELECT attempts,(SELECT count(*) FROM fixture_native_attempts) FROM fixture_native_limit`).Scan(&attempts, &rows) != nil || attempts != 37 || rows != 0 {
		t.Fatal("invalid High remainder history changed spending")
	}
	// The zero-call failure still changes provenance. A previous remainder row
	// cannot be reused just because its attempt baseline is also 37.
	raw, err := readHighRemainderPriorArtifact(highRemainderDelegationHistoryPaths()[11], highRemainderPreflightReport)
	var preflight nativeScenarioEvidence
	h, historyErr := readHighRemainderDelegationHistory(highRemainderDelegationHistoryPaths())
	if err != nil || json.Unmarshal(raw, &preflight) != nil || historyErr != nil || preflight.Historical.BaselineSHA256 == h.BaselineSHA256 {
		t.Fatal("zero-call failure baseline evidence unavailable")
	}
	setIsolatedBaseline := func(baseline string) {
		tx, err := pool.Begin(ctx)
		if err != nil {
			t.Fatal("isolated baseline transaction unavailable")
		}
		defer tx.Rollback(ctx)
		if _, err = tx.Exec(ctx, `ALTER TABLE fixture_native_limit DISABLE TRIGGER fixture_high_remainder_guard`); err != nil {
			t.Fatal("isolated baseline guard unavailable")
		}
		if _, err = tx.Exec(ctx, `UPDATE fixture_native_limit SET baseline_hash=$1`, baseline); err != nil {
			t.Fatal("isolated baseline fixture unavailable")
		}
		if _, err = tx.Exec(ctx, `ALTER TABLE fixture_native_limit ENABLE TRIGGER fixture_high_remainder_guard`); err != nil || tx.Commit(ctx) != nil {
			t.Fatal("isolated baseline guard restoration unavailable")
		}
	}
	setIsolatedBaseline(preflight.Historical.BaselineSHA256)
	if _, err = newHighRemainderDelegationLimiter(ctx, pool); err != errNativeFixtureDenied {
		t.Fatal("previous remainder baseline reused without its zero-call failure")
	}
	var priorBaseline string
	if pool.QueryRow(ctx, `SELECT attempts,baseline_hash,(SELECT count(*) FROM fixture_native_attempts) FROM fixture_native_limit`).Scan(&attempts, &priorBaseline, &rows) != nil || attempts != 37 || rows != 0 || priorBaseline != preflight.Historical.BaselineSHA256 {
		t.Fatal("denied remainder reconstruction changed previous baseline")
	}
	setIsolatedBaseline(h.BaselineSHA256)
	if _, err = pool.Exec(ctx, `ALTER TABLE fixture_native_limit DISABLE TRIGGER fixture_high_remainder_guard; UPDATE fixture_native_limit SET expires_at=clock_timestamp()-interval '1 second'; ALTER TABLE fixture_native_limit ENABLE TRIGGER fixture_high_remainder_guard`); err != nil {
		t.Fatal("isolated remainder expiry unavailable")
	}
	for _, kind := range []string{"start", "continue", "tool_result"} {
		if l.reserveHighRemainder(ctx, kind, "synthetic-remainder-run", nil) != errNativeFixtureDenied {
			t.Fatal("expired High remainder admitted")
		}
	}
}

func TestHighRemainderDelegationExpiryAfterUnchangedRowWaitIntegration(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	pool := memorytest.Database(t)
	l, err := newHighRemainderDelegationLimiter(ctx, pool)
	if err != nil {
		t.Fatal("remainder expiry limiter unavailable")
	}
	if _, err = pool.Exec(ctx, `ALTER TABLE fixture_native_limit DISABLE TRIGGER fixture_high_remainder_guard; UPDATE fixture_native_limit SET expires_at=clock_timestamp()+interval '4 seconds'; ALTER TABLE fixture_native_limit ENABLE TRIGGER fixture_high_remainder_guard`); err != nil {
		t.Fatal("isolated remainder expiry unavailable")
	}
	blocker, err := pool.Begin(ctx)
	if err != nil {
		t.Fatal("remainder row locker unavailable")
	}
	defer blocker.Rollback(context.Background())
	var original int
	if blocker.QueryRow(ctx, `SELECT attempts FROM fixture_native_limit WHERE singleton FOR UPDATE`).Scan(&original) != nil || original != 37 {
		t.Fatal("unchanged remainder row lock unavailable")
	}
	done := make(chan error, 1)
	go func() { done <- l.reserveHighRemainder(ctx, "start", "synthetic-expired-remainder", nil) }()
	for {
		if _, err = blocker.Exec(ctx, `SELECT pg_stat_clear_snapshot()`); err != nil {
			t.Fatal("remainder wait observation unavailable")
		}
		var waiting, expired bool
		if blocker.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM pg_stat_activity WHERE datname=current_database() AND pid<>pg_backend_pid() AND wait_event_type='Lock' AND pg_backend_pid()=ANY(pg_blocking_pids(pid)) AND query LIKE 'UPDATE fixture_native_limit%'), clock_timestamp()>=expires_at FROM fixture_native_limit WHERE singleton`).Scan(&waiting, &expired) != nil {
			t.Fatal("remainder wait evidence unavailable")
		}
		if waiting {
			if expired {
				t.Fatal("remainder admission did not start before expiry")
			}
			break
		}
		select {
		case <-done:
			t.Fatal("remainder admission did not wait on the actual row")
		case <-ctx.Done():
			t.Fatal("remainder row wait timed out")
		case <-time.After(10 * time.Millisecond):
		}
	}
	for {
		var expired bool
		if blocker.QueryRow(ctx, `SELECT clock_timestamp()>=expires_at FROM fixture_native_limit WHERE singleton`).Scan(&expired) != nil {
			t.Fatal("natural remainder expiry unavailable")
		}
		if expired {
			break
		}
		select {
		case <-done:
			t.Fatal("remainder admission escaped unchanged row lock")
		case <-ctx.Done():
			t.Fatal("natural remainder expiry timed out")
		case <-time.After(10 * time.Millisecond):
		}
	}
	if blocker.Rollback(ctx) != nil {
		t.Fatal("unchanged remainder row release unavailable")
	}
	select {
	case entryErr := <-done:
		if entryErr != errNativeFixtureDenied {
			t.Fatal("expired remainder admission was not denied after row wait")
		}
	case <-ctx.Done():
		t.Fatal("post-wait remainder admission timed out")
	}
	var attempts, rows int
	if pool.QueryRow(ctx, `SELECT attempts,(SELECT count(*) FROM fixture_native_attempts) FROM fixture_native_limit`).Scan(&attempts, &rows) != nil || attempts != 37 || rows != 0 {
		t.Fatal("post-wait expiry changed remainder spending")
	}
}
