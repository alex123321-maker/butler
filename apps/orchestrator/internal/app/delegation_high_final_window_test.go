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
	"time"

	"github.com/butler/butler/internal/memory/memorytest"
	"github.com/butler/butler/internal/transport/antigravity"
	"github.com/jackc/pgx/v5/pgxpool"
)

const (
	highFinalPriorAttempts = 39
	highFinalPriorReport   = "58edb6f022399e385118c51de8e3f9f7890fbfc74a5dd3b39de8b42839651505"
	highFinalPriorRelease  = "bdd85cdc752a739e0a4fa9be4789bf2f8fc7132d"
)

func highFinalDelegationWindow() bool {
	return os.Getenv("BUTLER_DELEGATION_HIGH_FINAL_WINDOW") == "1"
}

func highFinalDelegationHistoryPaths() []string {
	paths := highRemainderDelegationHistoryPaths()
	return append(paths, filepath.Join(filepath.Dir(paths[11]), "native-remainder-high-20261012.json"))
}

// A typed check supplements the immutable report hash. Both attempts crossed
// provider entry and ended with transport errors; unknown usage stays unknown.
func validateHighFinalFailedScope(p nativeScenarioEvidence, h nativeHistoricalEvidence) error {
	if p.Status != "failed" || p.Model != antigravity.FlashHighModel || p.ReleaseRef != highFinalPriorRelease || p.ExecutionMode != "scripted_parents_and_width1_baseline_only_width2_children_native_high" || p.ScriptedModelTurns == nil || *p.ScriptedModelTurns != 8 || p.NativeAttempts != 39 || p.NewNativeAttempts != 2 || p.MaxAttempts != 39 || p.MaxNewAttempts != 2 || len(p.Attempts) != 2 || len(p.Usage) != 2 || len(p.Widths) != 2 || string(nativeJSON(p.Historical)) != string(nativeJSON(h)) || p.OwnerStopDenies || p.RevisionDenies || p.DeletionDenies || len(p.ControlScopes) != 0 || p.ConfirmedEffectsAfterControls != nil || p.OwnedState == nil || !p.OwnedState.Available || !p.OwnedState.WorkersStopped || p.OwnedState.Status != "captured_before_cleanup" {
		return errNativeFixtureDenied
	}
	for i, w := range p.Widths {
		a := w.FullAcceptance
		if w.Width != i+1 || !w.ParentWorkerKilled || !w.ParentReceiptCheck || w.ParentRunID == "" || a == nil || !a.ParentFinalAvailable || !a.ParentFinalUnverified || !a.ParentFinalComplete || a.ParentFinalPrivacyRedacted || a.ParentReceipt == nil || a.ParentReceipt.Reason != "passed" || !a.ParentReceipt.IDsMatch || a.ResolutionPolicy != "owner_selected_source_a_for_fixture" {
			return errNativeFixtureDenied
		}
		if i == 0 {
			if w.ExecutionMode != "scripted_parent_and_children_component_baseline" || len(w.ObjectiveChecks) != 2 || !w.ObjectiveChecks[0] || !w.ObjectiveChecks[1] || w.MaximumChildOverlap != 1 || len(w.ChildTaskIDs) != 2 || len(w.ChildRunIDs) != 2 || len(w.ChildResultIDs) != 2 || !w.ParentTaskStillActive || !w.IndependentObservationsKept || !w.UnverifiedAcceptanceDenied || a.Stage != "independent_decisions_checked" || !a.StructuredOwningReportsChecked || !a.IndependentOriginalChecks || !a.ConflictingOriginalsRetained || a.AcceptedChild != w.ChildTaskIDs[0] || a.RejectedChild != w.ChildTaskIDs[1] || !a.PreviousResultAndReportKept || !a.OwnerAPIAuthenticated || !a.ChildFinalsStayUnverified || !a.ChildTasksStayActive {
				return errNativeFixtureDenied
			}
		} else if w.ExecutionMode != "scripted_parent_native_high_children" || len(w.ObjectiveChecks) != 0 || w.MaximumChildOverlap != 0 || len(w.ChildTaskIDs) != 0 || len(w.ChildRunIDs) != 0 || len(w.ChildResultIDs) != 0 || w.ParentTaskStillActive || w.IndependentObservationsKept || w.UnverifiedAcceptanceDenied || a.Stage != "parent_receipts_checked" || a.StructuredOwningReportsChecked || a.IndependentOriginalChecks || a.ConflictingOriginalsRetained || a.OwnerAPIAuthenticated || a.PreviousResultAndReportKept || a.ChildFinalsStayUnverified || a.ChildTasksStayActive || a.AcceptedChild != "" || a.RejectedChild != "" {
			return errNativeFixtureDenied
		}
	}
	if p.Attempts[0].RunID == p.Attempts[1].RunID {
		return errNativeFixtureDenied
	}
	for i, attempt := range p.Attempts {
		if attempt.Attempt != 38+i || attempt.Kind != "start" || attempt.RunID == "" || !nativeAuditHash.MatchString(attempt.InputHash) || attempt.CreatedAt.IsZero() || p.Usage[i].RunID != attempt.RunID || p.Usage[i].ReservationID == "" || p.Usage[i].ActualInputTokens != nil || p.Usage[i].ActualOutputTokens != nil {
			return errNativeFixtureDenied
		}
		runHash := nativeAuditRef(attempt.RunID)
		matches := 0
		for _, row := range p.OwnedState.Rows {
			if row.Kind == "run" && row.Ref == runHash {
				matches++
				if row.State != "failed" || row.Status != "failed" || row.ErrorCode != "transport_error" || row.TransportCode != "transport_connection_error" || !row.Finished {
					return errNativeFixtureDenied
				}
			}
			if row.Kind == "final" && row.Run == runHash {
				return errNativeFixtureDenied
			}
		}
		if matches != 1 {
			return errNativeFixtureDenied
		}
	}
	return nil
}

func readHighFinalDelegationHistory(paths []string) (nativeHistoricalEvidence, error) {
	if len(paths) != 13 {
		return nativeHistoricalEvidence{}, errNativeFixtureDenied
	}
	h, err := readHighRemainderDelegationHistory(paths[:12])
	if err != nil {
		return nativeHistoricalEvidence{}, errNativeFixtureDenied
	}
	raw, err := readHighRemainderPriorArtifact(paths[12], highFinalPriorReport)
	var previous nativeScenarioEvidence
	if err != nil || json.Unmarshal(raw, &previous) != nil || validateHighFinalFailedScope(previous, h) != nil {
		return nativeHistoricalEvidence{}, errNativeFixtureDenied
	}
	h.Usage = append(h.Usage, previous.Usage...)
	h.UnknownUsagePairs += 2
	h.NativeAttempts = highFinalPriorAttempts
	if len(h.Usage) != 39 || h.KnownUsagePairs != 17 || h.UnknownUsagePairs != 22 {
		return nativeHistoricalEvidence{}, errNativeFixtureDenied
	}
	baseline := sha256.Sum256([]byte(h.BaselineSHA256 + "\n" + highFinalPriorReport + "\n"))
	h.BaselineSHA256 = hex.EncodeToString(baseline[:])
	h.Artifacts = append(h.Artifacts, nativePriorArtifact{ReportSHA256: highFinalPriorReport, ReleaseRef: previous.ReleaseRef, NewAttempts: 2})
	return h, nil
}

func newHighFinalDelegationLimiter(ctx context.Context, pool *pgxpool.Pool) (*nativeDelegationLimiter, error) {
	h, err := readHighFinalDelegationHistory(highFinalDelegationHistoryPaths())
	if pool == nil || err != nil {
		return nil, errNativeFixtureDenied
	}
	// Reject an old window before changing even its fixture trigger definitions.
	// A compatible reconstruction may reuse its own immutable row, never migrate
	// a previous 37/39 window just because the historical total is now 39.
	var exists bool
	if pool.QueryRow(ctx, `SELECT to_regclass('fixture_native_limit') IS NOT NULL`).Scan(&exists) != nil {
		return nil, errNativeFixtureDenied
	}
	if exists {
		var compatible bool
		if pool.QueryRow(ctx, `SELECT prior_attempts=39 AND hard_cap=41 AND new_cap=2 AND baseline_hash=$1 FROM fixture_native_limit WHERE singleton`, h.BaselineSHA256).Scan(&compatible) != nil || !compatible {
			return nil, errNativeFixtureDenied
		}
	}
	_, err = pool.Exec(ctx, `CREATE TABLE IF NOT EXISTS fixture_native_limit (
 singleton boolean PRIMARY KEY CHECK(singleton),attempts integer NOT NULL CHECK(attempts BETWEEN 39 AND 41),
 prior_attempts integer NOT NULL CHECK(prior_attempts=39),hard_cap integer NOT NULL CHECK(hard_cap=41),
 new_cap integer NOT NULL CHECK(new_cap=2),baseline_hash text NOT NULL CHECK(length(baseline_hash)=64),expires_at timestamptz NOT NULL);
 CREATE TABLE IF NOT EXISTS fixture_native_attempts(attempt integer PRIMARY KEY CHECK(attempt BETWEEN 40 AND 41),
 run_id text NOT NULL,kind text NOT NULL CHECK(kind IN ('start','continue','tool_result')),input_hash text NOT NULL CHECK(length(input_hash)=64),created_at timestamptz NOT NULL DEFAULT clock_timestamp());
 CREATE OR REPLACE FUNCTION fixture_high_final_immutable() RETURNS trigger LANGUAGE plpgsql AS $$ BEGIN
 IF TG_OP='DELETE' THEN RAISE EXCEPTION 'immutable final High window'; END IF;
 IF NEW.singleton IS DISTINCT FROM OLD.singleton OR NEW.prior_attempts IS DISTINCT FROM OLD.prior_attempts
 OR NEW.hard_cap IS DISTINCT FROM OLD.hard_cap OR NEW.new_cap IS DISTINCT FROM OLD.new_cap
 OR NEW.baseline_hash IS DISTINCT FROM OLD.baseline_hash OR NEW.expires_at IS DISTINCT FROM OLD.expires_at
 OR NEW.attempts<OLD.attempts THEN RAISE EXCEPTION 'immutable final High window'; END IF; RETURN NEW; END $$;
 DROP TRIGGER IF EXISTS fixture_high_final_guard ON fixture_native_limit;
 CREATE TRIGGER fixture_high_final_guard BEFORE UPDATE OR DELETE ON fixture_native_limit FOR EACH ROW EXECUTE FUNCTION fixture_high_final_immutable();`)
	if err == nil {
		_, err = pool.Exec(ctx, `INSERT INTO fixture_native_limit VALUES(TRUE,39,39,41,2,$1,clock_timestamp()+interval '18 minutes') ON CONFLICT DO NOTHING`, h.BaselineSHA256)
	}
	var valid bool
	if err != nil || pool.QueryRow(ctx, `SELECT prior_attempts=39 AND hard_cap=41 AND new_cap=2 AND baseline_hash=$1 FROM fixture_native_limit WHERE singleton`, h.BaselineSHA256).Scan(&valid) != nil || !valid {
		return nil, errNativeFixtureDenied
	}
	return &nativeDelegationLimiter{pool}, nil
}

func (l *nativeDelegationLimiter) reserveHighFinal(ctx context.Context, kind, runID string, input any) error {
	return l.reserveHighFinalPaths(ctx, kind, runID, input, highFinalDelegationHistoryPaths())
}

func (l *nativeDelegationLimiter) reserveHighFinalPaths(ctx context.Context, kind, runID string, input any, paths []string) error {
	if l == nil || l.pool == nil || runID == "" || kind != "start" && kind != "continue" && kind != "tool_result" {
		return errNativeFixtureDenied
	}
	h, err := readHighFinalDelegationHistory(paths)
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
	if tx.QueryRow(ctx, `UPDATE fixture_native_limit SET attempts=attempts+1 WHERE singleton AND prior_attempts=39 AND hard_cap=41 AND new_cap=2 AND baseline_hash=$1 AND attempts<hard_cap AND expires_at>clock_timestamp() RETURNING attempts`, h.BaselineSHA256).Scan(&ordinal) != nil {
		return errNativeFixtureDenied
	}
	var withinWindow bool
	if tx.QueryRow(ctx, `SELECT expires_at>clock_timestamp() FROM fixture_native_limit WHERE singleton`).Scan(&withinWindow) != nil || !withinWindow {
		return errNativeFixtureDenied
	}
	if _, err = tx.Exec(ctx, `INSERT INTO fixture_native_attempts(attempt,run_id,kind,input_hash) VALUES($1,$2,$3,$4)`, ordinal, runID, kind, hex.EncodeToString(hash[:])); err != nil || tx.Commit(ctx) != nil {
		return errNativeFixtureDenied
	}
	return nil
}

func TestHighFinalDelegationHistory(t *testing.T) {
	h, err := readHighFinalDelegationHistory(highFinalDelegationHistoryPaths())
	if err != nil || h.NativeAttempts != 39 || len(h.Artifacts) != 13 || len(h.Usage) != 39 || h.KnownUsagePairs != 17 || h.UnknownUsagePairs != 22 {
		t.Fatal("final High history unavailable")
	}
	var input, output int64
	for _, u := range h.Usage {
		if u.ActualInputTokens != nil {
			input += *u.ActualInputTokens
			output += *u.ActualOutputTokens
		}
	}
	if input != 62760 || output != 1810 {
		t.Fatal("final High carried usage changed")
	}
	if _, err = readHighFinalDelegationHistory(highFinalDelegationHistoryPaths()[:12]); err != errNativeFixtureDenied {
		t.Fatal("final High incomplete history admitted")
	}
	raw, err := readHighRemainderPriorArtifact(highFinalDelegationHistoryPaths()[12], highFinalPriorReport)
	if err != nil {
		t.Fatal("failed39 scope fixture unavailable")
	}
	for _, change := range []struct {
		name   string
		mutate func(*nativeScenarioEvidence)
	}{
		{"promoted_failed_report", func(p *nativeScenarioEvidence) { p.Status = "passed" }},
		{"forged_39_count", func(p *nativeScenarioEvidence) { p.NewNativeAttempts = 0 }},
		{"script_as_native", func(p *nativeScenarioEvidence) { *p.ScriptedModelTurns = 0 }},
		{"unknown_as_zero", func(p *nativeScenarioEvidence) {
			zero := int64(0)
			p.Usage[0].ActualInputTokens = &zero
			p.Usage[0].ActualOutputTokens = &zero
		}},
		{"duplicate_native_run", func(p *nativeScenarioEvidence) { p.Attempts[1].RunID = p.Attempts[0].RunID }},
		{"successful_width2", func(p *nativeScenarioEvidence) { p.Widths[1].ObjectiveChecks = []bool{true, true} }},
		{"controls_not_run", func(p *nativeScenarioEvidence) { p.OwnerStopDenies = true }},
		{"missing_owned_state", func(p *nativeScenarioEvidence) { p.OwnedState = nil }},
		{"wrong_transport_class", func(p *nativeScenarioEvidence) {
			for i := range p.OwnedState.Rows {
				r := &p.OwnedState.Rows[i]
				if r.Kind == "run" && r.ErrorCode == "transport_error" {
					r.TransportCode = "rate_limited"
				}
			}
		}},
		{"forged_native_final", func(p *nativeScenarioEvidence) {
			p.OwnedState.Rows = append(p.OwnedState.Rows, nativeOwnedRow{Kind: "final", Run: nativeAuditRef(p.Attempts[0].RunID)})
		}},
	} {
		t.Run(change.name, func(t *testing.T) {
			var p nativeScenarioEvidence
			if json.Unmarshal(raw, &p) != nil {
				t.Fatal("failed39 fixture decode denied")
			}
			prior, e := readHighRemainderDelegationHistory(highRemainderDelegationHistoryPaths())
			if e != nil {
				t.Fatal("prior scope fixture unavailable")
			}
			change.mutate(&p)
			if validateHighFinalFailedScope(p, prior) != errNativeFixtureDenied {
				t.Fatal("forged failed39 scope admitted")
			}
		})
	}
}

func TestHighFinalDelegationLimitIntegration(t *testing.T) {
	t.Setenv("BUTLER_DELEGATION_HIGH_FINAL_WINDOW", "1")
	t.Setenv("BUTLER_DELEGATION_HIGH_REMAINDER_WINDOW", "1")
	pool, ctx := memorytest.Database(t), context.Background()
	l, err := newNativeDelegationLimiter(ctx, pool)
	if err != nil || nativeScenarioModel() != antigravity.FlashHighModel {
		t.Fatal("final High selector denied")
	}
	var originalExpiry time.Time
	if pool.QueryRow(ctx, `SELECT expires_at FROM fixture_native_limit`).Scan(&originalExpiry) != nil {
		t.Fatal("final High deadline unavailable")
	}
	for _, query := range []string{"UPDATE fixture_native_limit SET attempts=38", "UPDATE fixture_native_limit SET hard_cap=42", "UPDATE fixture_native_limit SET new_cap=3", "UPDATE fixture_native_limit SET prior_attempts=38", "UPDATE fixture_native_limit SET expires_at=expires_at+interval '1 minute'", "UPDATE fixture_native_limit SET baseline_hash=repeat('f',64)", "DELETE FROM fixture_native_limit"} {
		if _, err = pool.Exec(ctx, query); err == nil {
			t.Fatal("final High immutable limit changed")
		}
	}
	var wg sync.WaitGroup
	allowed := make(chan bool, 16)
	for i := 0; i < 16; i++ {
		wg.Add(1)
		go func() { defer wg.Done(); allowed <- l.reserve(ctx, "start", "synthetic-final-run", nil) == nil }()
	}
	wg.Wait()
	close(allowed)
	n := 0
	for ok := range allowed {
		if ok {
			n++
		}
	}
	if n != 2 {
		t.Fatal("final High concurrent cap failed", n)
	}
	l, err = newNativeDelegationLimiter(ctx, pool)
	if err != nil {
		t.Fatal("final High reconstruction denied")
	}
	for _, kind := range []string{"start", "continue", "tool_result"} {
		if l.reserve(ctx, kind, "synthetic-final-run", nil) != errNativeFixtureDenied {
			t.Fatal("final High reconstruction refunded spending")
		}
	}
	var first, last, count, attempts int
	var expiry time.Time
	if pool.QueryRow(ctx, `SELECT min(attempt),max(attempt),count(*) FROM fixture_native_attempts`).Scan(&first, &last, &count) != nil || first != 40 || last != 41 || count != 2 || pool.QueryRow(ctx, `SELECT attempts,expires_at FROM fixture_native_limit`).Scan(&attempts, &expiry) != nil || attempts != 41 || !expiry.Equal(originalExpiry) {
		t.Fatal("final High ordinals or deadline changed")
	}
}

func TestHighFinalDelegationRevalidationIntegration(t *testing.T) {
	pool, ctx := memorytest.Database(t), context.Background()
	l, err := newHighFinalDelegationLimiter(ctx, pool)
	if err != nil {
		t.Fatal("final High limiter unavailable")
	}
	for i := range highFinalDelegationHistoryPaths() {
		for _, mode := range []string{"missing", "forged39"} {
			paths := highFinalDelegationHistoryPaths()
			paths[i] = filepath.Join(t.TempDir(), mode)
			if mode == "forged39" && os.WriteFile(paths[i], []byte(`{"status":"passed","native_attempts":39,"new_native_attempts":0}`), 0600) != nil {
				t.Fatal("forged39 fixture unavailable")
			}
			for _, kind := range []string{"start", "continue", "tool_result"} {
				if l.reserveHighFinalPaths(ctx, kind, "synthetic-final-run", nil, paths) != errNativeFixtureDenied {
					t.Fatal("incomplete or forged13pin history admitted", i, mode, kind)
				}
			}
		}
	}
	var attempts, rows int
	if pool.QueryRow(ctx, `SELECT attempts,(SELECT count(*) FROM fixture_native_attempts) FROM fixture_native_limit`).Scan(&attempts, &rows) != nil || attempts != 39 || rows != 0 {
		t.Fatal("invalid final High history changed spending")
	}
	if _, err = pool.Exec(ctx, `ALTER TABLE fixture_native_limit DISABLE TRIGGER fixture_high_final_guard; UPDATE fixture_native_limit SET expires_at=clock_timestamp()-interval '1 second'; ALTER TABLE fixture_native_limit ENABLE TRIGGER fixture_high_final_guard`); err != nil {
		t.Fatal("isolated final expiry unavailable")
	}
	if _, err = newHighFinalDelegationLimiter(ctx, pool); err != nil {
		t.Fatal("expired final reconstruction unavailable")
	}
	for _, kind := range []string{"start", "continue", "tool_result"} {
		if l.reserveHighFinal(ctx, kind, "synthetic-final-run", nil) != errNativeFixtureDenied {
			t.Fatal("expired final window admitted")
		}
	}
}

func TestHighFinalDelegationOldWindowDeniedIntegration(t *testing.T) {
	pool, ctx := memorytest.Database(t), context.Background()
	old, err := newHighRemainderDelegationLimiter(ctx, pool)
	if err != nil {
		t.Fatal("isolated old remainder unavailable")
	}
	for _, run := range []string{"synthetic-old-38", "synthetic-old-39"} {
		if old.reserveHighRemainder(ctx, "start", run, nil) != nil {
			t.Fatal("isolated spent old remainder unavailable")
		}
	}
	var oldAttempts, oldRows int
	if pool.QueryRow(ctx, `SELECT attempts,(SELECT count(*) FROM fixture_native_attempts) FROM fixture_native_limit`).Scan(&oldAttempts, &oldRows) != nil || oldAttempts != 39 || oldRows != 2 {
		t.Fatal("old window did not retain its spent 39 ceiling")
	}
	var before []byte
	if pool.QueryRow(ctx, `SELECT to_jsonb(l) FROM fixture_native_limit l`).Scan(&before) != nil {
		t.Fatal("old window evidence unavailable")
	}
	var triggersBefore, triggersAfter string
	if pool.QueryRow(ctx, `SELECT string_agg(pg_get_triggerdef(oid),';' ORDER BY tgname) FROM pg_trigger WHERE tgrelid='fixture_native_limit'::regclass AND NOT tgisinternal`).Scan(&triggersBefore) != nil {
		t.Fatal("old window trigger evidence unavailable")
	}
	if _, err := newHighFinalDelegationLimiter(ctx, pool); err != errNativeFixtureDenied {
		t.Fatal("final window reused old remainder row")
	}
	var after []byte
	if pool.QueryRow(ctx, `SELECT to_jsonb(l) FROM fixture_native_limit l`).Scan(&after) != nil || string(before) != string(after) {
		t.Fatal("final constructor rewrote old remainder row")
	}
	if pool.QueryRow(ctx, `SELECT string_agg(pg_get_triggerdef(oid),';' ORDER BY tgname) FROM pg_trigger WHERE tgrelid='fixture_native_limit'::regclass AND NOT tgisinternal`).Scan(&triggersAfter) != nil || triggersBefore != triggersAfter {
		t.Fatal("final constructor changed old remainder triggers")
	}
	l := &nativeDelegationLimiter{pool}
	if l.reserveHighFinal(ctx, "start", "synthetic-final-run", nil) != errNativeFixtureDenied {
		t.Fatal("final admission used old window limits")
	}
	if pool.QueryRow(ctx, `SELECT attempts,(SELECT count(*) FROM fixture_native_attempts) FROM fixture_native_limit`).Scan(&oldAttempts, &oldRows) != nil || oldAttempts != 39 || oldRows != 2 {
		t.Fatal("denied final admission rewrote spent old window")
	}
}

func TestHighFinalDelegationExpiryAfterUnchangedRowWaitIntegration(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	pool := memorytest.Database(t)
	l, err := newHighFinalDelegationLimiter(ctx, pool)
	if err != nil {
		t.Fatal("final expiry limiter unavailable")
	}
	if _, err = pool.Exec(ctx, `ALTER TABLE fixture_native_limit DISABLE TRIGGER fixture_high_final_guard; UPDATE fixture_native_limit SET expires_at=clock_timestamp()+interval '4 seconds'; ALTER TABLE fixture_native_limit ENABLE TRIGGER fixture_high_final_guard`); err != nil {
		t.Fatal("isolated final expiry unavailable")
	}
	blocker, err := pool.Begin(ctx)
	if err != nil {
		t.Fatal("final row locker unavailable")
	}
	defer blocker.Rollback(context.Background())
	var original int
	if blocker.QueryRow(ctx, `SELECT attempts FROM fixture_native_limit WHERE singleton FOR UPDATE`).Scan(&original) != nil || original != 39 {
		t.Fatal("unchanged final row lock unavailable")
	}
	done := make(chan error, 1)
	go func() { done <- l.reserveHighFinal(ctx, "start", "synthetic-expired-final", nil) }()
	for {
		if _, err = blocker.Exec(ctx, `SELECT pg_stat_clear_snapshot()`); err != nil {
			t.Fatal("final wait observation unavailable")
		}
		var waiting, expired bool
		if blocker.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM pg_stat_activity WHERE datname=current_database() AND pid<>pg_backend_pid() AND wait_event_type='Lock' AND pg_backend_pid()=ANY(pg_blocking_pids(pid)) AND query LIKE 'UPDATE fixture_native_limit%'),clock_timestamp()>=expires_at FROM fixture_native_limit WHERE singleton`).Scan(&waiting, &expired) != nil {
			t.Fatal("final wait evidence unavailable")
		}
		if waiting {
			if expired {
				t.Fatal("final admission began after expiry")
			}
			break
		}
		select {
		case <-done:
			t.Fatal("final admission did not wait on actual row")
		case <-ctx.Done():
			t.Fatal("final row wait timed out")
		case <-time.After(10 * time.Millisecond):
		}
	}
	for {
		var expired bool
		if blocker.QueryRow(ctx, `SELECT clock_timestamp()>=expires_at FROM fixture_native_limit WHERE singleton`).Scan(&expired) != nil {
			t.Fatal("natural final expiry unavailable")
		}
		if expired {
			break
		}
		select {
		case <-done:
			t.Fatal("final admission escaped unchanged row lock")
		case <-ctx.Done():
			t.Fatal("natural final expiry timed out")
		case <-time.After(10 * time.Millisecond):
		}
	}
	if blocker.Rollback(ctx) != nil {
		t.Fatal("unchanged final row release unavailable")
	}
	select {
	case entryErr := <-done:
		if entryErr != errNativeFixtureDenied {
			t.Fatal("post-wait expired final admission was not denied")
		}
	case <-ctx.Done():
		t.Fatal("post-wait final admission timed out")
	}
	var attempts, rows int
	if pool.QueryRow(ctx, `SELECT attempts,(SELECT count(*) FROM fixture_native_attempts) FROM fixture_native_limit`).Scan(&attempts, &rows) != nil || attempts != 39 || rows != 0 {
		t.Fatal("post-wait expiry changed final spending")
	}
}
