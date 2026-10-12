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
	initiativeHighPriorReport  = "724c8f3f0e4ba6864952e57b52d5111f135fbe8a27c88f00d893a2b4df753df3"
	initiativeHighPriorRelease = "51b3ea62179edf2b31d1b1fbd4d0b08137484ab4"
)

// This new initiative experiment carries the successful final delegation report
// as well as every earlier failed/diagnostic artifact. It never reopens a
// delegation window or treats the eight scripted turns as native usage.
func initiativeHighHistoryPaths() []string {
	paths := highFinalDelegationHistoryPaths()
	return append(paths, filepath.Join(filepath.Dir(paths[12]), "native-final-high-20261012.json"))
}

func readInitiativeHighHistory(paths []string) (nativeHistoricalEvidence, error) {
	if len(paths) != 14 {
		return nativeHistoricalEvidence{}, errNativeFixtureDenied
	}
	h, err := readHighFinalDelegationHistory(paths[:13])
	if err != nil {
		return nativeHistoricalEvidence{}, errNativeFixtureDenied
	}
	raw, err := readHighRemainderPriorArtifact(paths[13], initiativeHighPriorReport)
	var previous nativeScenarioEvidence
	if err != nil || json.Unmarshal(raw, &previous) != nil || previous.Status != "passed" ||
		previous.Model != antigravity.FlashHighModel || previous.ReleaseRef != initiativeHighPriorRelease ||
		previous.ExecutionMode != "scripted_parents_and_width1_baseline_only_width2_children_native_high" ||
		previous.ScriptedModelTurns == nil || *previous.ScriptedModelTurns != 8 ||
		previous.NativeAttempts != 41 || previous.NewNativeAttempts != 2 ||
		previous.MaxAttempts != 41 || previous.MaxNewAttempts != 2 ||
		len(previous.Attempts) != 2 || len(previous.Usage) != 2 || len(previous.Widths) != 2 ||
		string(nativeJSON(previous.Historical)) != string(nativeJSON(h)) ||
		!previous.OwnerStopDenies || !previous.RevisionDenies || !previous.DeletionDenies ||
		len(previous.ControlScopes) != 3 || previous.ConfirmedEffectsAfterControls == nil ||
		*previous.ConfirmedEffectsAfterControls != 4 ||
		previous.OwnedState == nil || !previous.OwnedState.Available || !previous.OwnedState.WorkersStopped {
		return nativeHistoricalEvidence{}, errNativeFixtureDenied
	}
	for i, width := range previous.Widths {
		a := width.FullAcceptance
		if width.Width != i+1 || !width.ParentWorkerKilled || !width.ParentReceiptCheck ||
			width.MaximumChildOverlap != i+1 || len(width.ObjectiveChecks) != 2 ||
			!width.ObjectiveChecks[0] || !width.ObjectiveChecks[1] ||
			len(width.ChildTaskIDs) != 2 || len(width.ChildRunIDs) != 2 || len(width.ChildResultIDs) != 2 ||
			a == nil || !a.ParentFinalAvailable || !a.ParentFinalUnverified || !a.ParentFinalComplete ||
			a.ParentReceipt == nil || a.ParentReceipt.Reason != "passed" || !a.ParentReceipt.IDsMatch ||
			!a.StructuredOwningReportsChecked || !a.IndependentOriginalChecks || !a.ConflictingOriginalsRetained ||
			!a.ChildFinalsStayUnverified || !a.ChildTasksStayActive ||
			a.ResolutionPolicy != "owner_selected_source_a_for_fixture" {
			return nativeHistoricalEvidence{}, errNativeFixtureDenied
		}
		if i == 0 {
			if width.ExecutionMode != "scripted_parent_and_children_component_baseline" ||
				a.Stage != "independent_decisions_checked" || !a.OwnerAPIAuthenticated ||
				!a.PreviousResultAndReportKept || a.AcceptedChild != width.ChildTaskIDs[0] ||
				a.RejectedChild != width.ChildTaskIDs[1] {
				return nativeHistoricalEvidence{}, errNativeFixtureDenied
			}
		} else if width.ExecutionMode != "scripted_parent_native_high_children" ||
			a.Stage != "independently_checked_result_ready_for_controls" || a.OwnerAPIAuthenticated ||
			a.PreviousResultAndReportKept || a.AcceptedChild != "" || a.RejectedChild != "" {
			return nativeHistoricalEvidence{}, errNativeFixtureDenied
		}
	}
	var input, output int64
	for i, attempt := range previous.Attempts {
		usage := previous.Usage[i]
		if attempt.Attempt != 40+i || attempt.Kind != "start" || attempt.RunID == "" ||
			!nativeAuditHash.MatchString(attempt.InputHash) || attempt.CreatedAt.IsZero() ||
			usage.RunID != attempt.RunID || usage.ReservationID == "" ||
			usage.ActualInputTokens == nil || usage.ActualOutputTokens == nil ||
			*usage.ActualInputTokens < 0 || *usage.ActualOutputTokens < 0 ||
			attempt.RunID != previous.Widths[1].ChildRunIDs[i] {
			return nativeHistoricalEvidence{}, errNativeFixtureDenied
		}
		input += *usage.ActualInputTokens
		output += *usage.ActualOutputTokens
	}
	if previous.Attempts[0].RunID == previous.Attempts[1].RunID || input != 6298 || output != 325 {
		return nativeHistoricalEvidence{}, errNativeFixtureDenied
	}
	h.Usage = append(h.Usage, previous.Usage...)
	h.NativeAttempts, h.KnownUsagePairs = 41, h.KnownUsagePairs+2
	if len(h.Usage) != 41 || h.KnownUsagePairs != 19 || h.UnknownUsagePairs != 22 {
		return nativeHistoricalEvidence{}, errNativeFixtureDenied
	}
	baseline := sha256.Sum256([]byte(h.BaselineSHA256 + "\n" + initiativeHighPriorReport + "\n"))
	h.BaselineSHA256 = hex.EncodeToString(baseline[:])
	h.Artifacts = append(h.Artifacts, nativePriorArtifact{
		ReportSHA256: initiativeHighPriorReport, ReleaseRef: previous.ReleaseRef, NewAttempts: 2,
	})
	return h, nil
}

// The independent table names intentionally cannot migrate an old delegation
// counter. Production task/mandate/day budgets remain separate and mandatory.
type initiativeHighLimiter struct{ pool *pgxpool.Pool }

func newInitiativeHighLimiter(ctx context.Context, pool *pgxpool.Pool) (*initiativeHighLimiter, error) {
	h, err := readInitiativeHighHistory(initiativeHighHistoryPaths())
	if pool == nil || err != nil {
		return nil, errNativeFixtureDenied
	}
	var exists bool
	if pool.QueryRow(ctx, "SELECT to_regclass('fixture_initiative_native_limit') IS NOT NULL").Scan(&exists) != nil {
		return nil, errNativeFixtureDenied
	}
	if exists {
		var compatible bool
		if pool.QueryRow(ctx, "SELECT prior_attempts=41 AND hard_cap=45 AND new_cap=4 AND baseline_hash=$1 FROM fixture_initiative_native_limit WHERE singleton", h.BaselineSHA256).Scan(&compatible) != nil || !compatible {
			return nil, errNativeFixtureDenied
		}
	}
	const ddl = "CREATE TABLE IF NOT EXISTS fixture_initiative_native_limit (" +
		"singleton boolean PRIMARY KEY CHECK(singleton),attempts integer NOT NULL CHECK(attempts BETWEEN 41 AND 45)," +
		"prior_attempts integer NOT NULL CHECK(prior_attempts=41),hard_cap integer NOT NULL CHECK(hard_cap=45)," +
		"new_cap integer NOT NULL CHECK(new_cap=4),baseline_hash text NOT NULL CHECK(length(baseline_hash)=64),expires_at timestamptz NOT NULL);" +
		"CREATE TABLE IF NOT EXISTS fixture_initiative_native_attempts(" +
		"attempt integer PRIMARY KEY CHECK(attempt BETWEEN 42 AND 45),run_id text NOT NULL," +
		"kind text NOT NULL CHECK(kind IN ('start','continue','tool_result'))," +
		"input_hash text NOT NULL CHECK(length(input_hash)=64),created_at timestamptz NOT NULL DEFAULT clock_timestamp());" +
		"CREATE OR REPLACE FUNCTION fixture_initiative_native_immutable() RETURNS trigger LANGUAGE plpgsql AS $$ BEGIN " +
		"IF TG_OP='DELETE' THEN RAISE EXCEPTION 'immutable initiative native window'; END IF; " +
		"IF NEW.singleton IS DISTINCT FROM OLD.singleton OR NEW.prior_attempts IS DISTINCT FROM OLD.prior_attempts " +
		"OR NEW.hard_cap IS DISTINCT FROM OLD.hard_cap OR NEW.new_cap IS DISTINCT FROM OLD.new_cap " +
		"OR NEW.baseline_hash IS DISTINCT FROM OLD.baseline_hash OR NEW.expires_at IS DISTINCT FROM OLD.expires_at " +
		"OR NEW.attempts<OLD.attempts THEN RAISE EXCEPTION 'immutable initiative native window'; END IF; RETURN NEW; END $$;" +
		"DROP TRIGGER IF EXISTS fixture_initiative_native_guard ON fixture_initiative_native_limit;" +
		"CREATE TRIGGER fixture_initiative_native_guard BEFORE UPDATE OR DELETE ON fixture_initiative_native_limit " +
		"FOR EACH ROW EXECUTE FUNCTION fixture_initiative_native_immutable();"
	if _, err = pool.Exec(ctx, ddl); err != nil {
		return nil, errNativeFixtureDenied
	}
	if _, err = pool.Exec(ctx, "INSERT INTO fixture_initiative_native_limit VALUES(TRUE,41,41,45,4,$1,clock_timestamp()+interval '18 minutes') ON CONFLICT DO NOTHING", h.BaselineSHA256); err != nil {
		return nil, errNativeFixtureDenied
	}
	var valid bool
	if pool.QueryRow(ctx, "SELECT prior_attempts=41 AND hard_cap=45 AND new_cap=4 AND baseline_hash=$1 FROM fixture_initiative_native_limit WHERE singleton", h.BaselineSHA256).Scan(&valid) != nil || !valid {
		return nil, errNativeFixtureDenied
	}
	return &initiativeHighLimiter{pool: pool}, nil
}

func (l *initiativeHighLimiter) reserve(ctx context.Context, kind, runID string, input any) error {
	return l.reservePaths(ctx, kind, runID, input, initiativeHighHistoryPaths())
}

func (l *initiativeHighLimiter) reservePaths(ctx context.Context, kind, runID string, input any, paths []string) error {
	if l == nil || l.pool == nil || runID == "" || len(runID) > 256 ||
		kind != "start" && kind != "continue" && kind != "tool_result" {
		return errNativeFixtureDenied
	}
	h, err := readInitiativeHighHistory(paths)
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
	if tx.QueryRow(ctx, "UPDATE fixture_initiative_native_limit SET attempts=attempts+1 WHERE singleton AND prior_attempts=41 AND hard_cap=45 AND new_cap=4 AND baseline_hash=$1 AND attempts<hard_cap AND expires_at>clock_timestamp() RETURNING attempts", h.BaselineSHA256).Scan(&ordinal) != nil {
		return errNativeFixtureDenied
	}
	var withinWindow bool
	if tx.QueryRow(ctx, "SELECT expires_at>clock_timestamp() FROM fixture_initiative_native_limit WHERE singleton").Scan(&withinWindow) != nil || !withinWindow {
		return errNativeFixtureDenied
	}
	if _, err = tx.Exec(ctx, "INSERT INTO fixture_initiative_native_attempts(attempt,run_id,kind,input_hash) VALUES($1,$2,$3,$4)", ordinal, runID, kind, hex.EncodeToString(hash[:])); err != nil || tx.Commit(ctx) != nil {
		return errNativeFixtureDenied
	}
	return nil
}

func TestInitiativeHighHistory(t *testing.T) {
	h, err := readInitiativeHighHistory(initiativeHighHistoryPaths())
	if err != nil || h.NativeAttempts != 41 || len(h.Artifacts) != 14 || len(h.Usage) != 41 ||
		h.KnownUsagePairs != 19 || h.UnknownUsagePairs != 22 {
		t.Fatal("initiative native carried history unavailable")
	}
	var input, output int64
	for _, usage := range h.Usage {
		if usage.ActualInputTokens != nil {
			input += *usage.ActualInputTokens
			output += *usage.ActualOutputTokens
		}
	}
	if input != 69058 || output != 2135 {
		t.Fatal("initiative native historical usage changed")
	}
	for _, incomplete := range [][]string{nil, initiativeHighHistoryPaths()[:13]} {
		if _, err := readInitiativeHighHistory(incomplete); err != errNativeFixtureDenied {
			t.Fatal("initiative incomplete history allowed")
		}
	}
}

func TestInitiativeHighLimitIntegration(t *testing.T) {
	pool := memorytest.Database(t)
	ctx := context.Background()
	limit, err := newInitiativeHighLimiter(ctx, pool)
	if err != nil {
		t.Fatal("initiative counter unavailable")
	}
	var initialExpiry time.Time
	if pool.QueryRow(ctx, "SELECT expires_at FROM fixture_initiative_native_limit").Scan(&initialExpiry) != nil {
		t.Fatal("initial initiative deadline unavailable")
	}
	for _, query := range []string{
		"UPDATE fixture_initiative_native_limit SET attempts=40",
		"UPDATE fixture_initiative_native_limit SET hard_cap=46",
		"UPDATE fixture_initiative_native_limit SET new_cap=5",
		"UPDATE fixture_initiative_native_limit SET baseline_hash=repeat('0',64)",
		"UPDATE fixture_initiative_native_limit SET expires_at=clock_timestamp()+interval '1 day'",
		"DELETE FROM fixture_initiative_native_limit",
	} {
		if _, err = pool.Exec(ctx, query); err == nil {
			t.Fatal("immutable initiative metadata changed")
		}
	}
	results := make(chan bool, 16)
	var wg sync.WaitGroup
	for range 16 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			results <- limit.reserve(ctx, "start", "synthetic-initiative-run", nil) == nil
		}()
	}
	wg.Wait()
	close(results)
	admitted := 0
	for result := range results {
		if result {
			admitted++
		}
	}
	var attempts, count, first, last int
	if pool.QueryRow(ctx, "SELECT attempts,(SELECT count(*) FROM fixture_initiative_native_attempts),(SELECT min(attempt) FROM fixture_initiative_native_attempts),(SELECT max(attempt) FROM fixture_initiative_native_attempts) FROM fixture_initiative_native_limit").Scan(&attempts, &count, &first, &last) != nil ||
		admitted != 4 || attempts != 45 || count != 4 || first != 42 || last != 45 {
		t.Fatal("initiative concurrent native allowance changed")
	}
	reconstructed, err := newInitiativeHighLimiter(ctx, pool)
	if err != nil {
		t.Fatal("initiative immutable reconstruction unavailable")
	}
	for _, kind := range []string{"start", "continue", "tool_result"} {
		if reconstructed.reserve(ctx, kind, "synthetic-initiative-run", nil) != errNativeFixtureDenied {
			t.Fatal("spent initiative native allowance reopened")
		}
	}
	var expiry time.Time
	if pool.QueryRow(ctx, "SELECT expires_at FROM fixture_initiative_native_limit").Scan(&expiry) != nil || !expiry.Equal(initialExpiry) {
		t.Fatal("initiative reconstruction extended original deadline")
	}
}

func TestInitiativeHighRevalidationIntegration(t *testing.T) {
	pool := memorytest.Database(t)
	ctx := context.Background()
	limit, err := newInitiativeHighLimiter(ctx, pool)
	if err != nil {
		t.Fatal("initiative revalidation counter unavailable")
	}
	paths := initiativeHighHistoryPaths()
	for i, path := range paths {
		for _, kind := range []string{"start", "continue", "tool_result"} {
			missing := append([]string{}, paths...)
			missing[i] = filepath.Join(t.TempDir(), "missing-report")
			if limit.reservePaths(ctx, kind, "synthetic-initiative-run", nil, missing) != errNativeFixtureDenied {
				t.Fatal("missing initiative native history admitted")
			}
			bytes, readErr := os.ReadFile(path)
			if readErr != nil || len(bytes) == 0 {
				t.Fatal("known public fixture history unavailable")
			}
			bytes[0] ^= 1
			bad := filepath.Join(t.TempDir(), "changed-report")
			if os.WriteFile(bad, bytes, 0600) != nil {
				t.Fatal("changed report fixture unavailable")
			}
			changed := append([]string{}, paths...)
			changed[i] = bad
			if limit.reservePaths(ctx, kind, "synthetic-initiative-run", nil, changed) != errNativeFixtureDenied {
				t.Fatal("changed initiative native history admitted")
			}
		}
	}
	for _, invalid := range []struct{ kind, run string }{{"unknown", "run"}, {"start", ""}} {
		if limit.reserve(ctx, invalid.kind, invalid.run, nil) != errNativeFixtureDenied {
			t.Fatal("invalid initiative native entry admitted")
		}
	}
	if limit.reserve(ctx, "start", "run", make(chan bool)) != errNativeFixtureDenied {
		t.Fatal("unserializable initiative native input admitted")
	}
	var attempts, count int
	if pool.QueryRow(ctx, "SELECT attempts,(SELECT count(*) FROM fixture_initiative_native_attempts) FROM fixture_initiative_native_limit").Scan(&attempts, &count) != nil || attempts != 41 || count != 0 {
		t.Fatal("revalidation failure spent an initiative native request")
	}
}

func TestInitiativeHighPreservesSpentDelegationWindowIntegration(t *testing.T) {
	pool := memorytest.Database(t)
	ctx := context.Background()
	old, err := newHighFinalDelegationLimiter(ctx, pool)
	if err != nil {
		t.Fatal("old delegation window fixture unavailable")
	}
	for range 2 {
		if old.reserveHighFinal(ctx, "start", "synthetic-old-delegation-run", nil) != nil {
			t.Fatal("old delegation window could not reach its original ceiling")
		}
	}
	var beforeExpiry time.Time
	var beforeTrigger string
	if pool.QueryRow(ctx, "SELECT expires_at,(SELECT pg_get_triggerdef(oid) FROM pg_trigger WHERE tgrelid='fixture_native_limit'::regclass AND tgname='fixture_high_final_guard') FROM fixture_native_limit").Scan(&beforeExpiry, &beforeTrigger) != nil {
		t.Fatal("old delegation metadata unavailable")
	}
	current, err := newInitiativeHighLimiter(ctx, pool)
	if err != nil || current.reserve(ctx, "start", "synthetic-new-initiative-run", nil) != nil {
		t.Fatal("independent initiative window unavailable")
	}
	var oldAttempts, oldRows, newAttempts, newRows int
	var afterExpiry time.Time
	var afterTrigger string
	query := "SELECT attempts,expires_at,(SELECT count(*) FROM fixture_native_attempts)," +
		"(SELECT pg_get_triggerdef(oid) FROM pg_trigger WHERE tgrelid='fixture_native_limit'::regclass AND tgname='fixture_high_final_guard')," +
		"(SELECT attempts FROM fixture_initiative_native_limit),(SELECT count(*) FROM fixture_initiative_native_attempts) FROM fixture_native_limit"
	if pool.QueryRow(ctx, query).Scan(&oldAttempts, &afterExpiry, &oldRows, &afterTrigger, &newAttempts, &newRows) != nil ||
		oldAttempts != 41 || oldRows != 2 || !afterExpiry.Equal(beforeExpiry) || afterTrigger != beforeTrigger ||
		newAttempts != 42 || newRows != 1 {
		t.Fatal("new initiative fixture changed an old delegation counter, rows, deadline or guard")
	}
	if old.reserveHighFinal(ctx, "continue", "synthetic-old-delegation-run", nil) != errNativeFixtureDenied {
		t.Fatal("old spent delegation allowance reopened")
	}
}

func TestInitiativeHighExpiryAfterRowWaitIntegration(t *testing.T) {
	pool := memorytest.Database(t)
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	limit, err := newInitiativeHighLimiter(ctx, pool)
	if err != nil {
		t.Fatal("initiative expiry counter unavailable")
	}
	// Only this isolated test schema shortens its initial deadline. The live
	// constructor/runner never disable the guard or alter the fixed 18 minutes.
	if _, err = pool.Exec(ctx, "ALTER TABLE fixture_initiative_native_limit DISABLE TRIGGER fixture_initiative_native_guard; UPDATE fixture_initiative_native_limit SET expires_at=clock_timestamp()+interval '3 seconds'; ALTER TABLE fixture_initiative_native_limit ENABLE TRIGGER fixture_initiative_native_guard"); err != nil {
		t.Fatal("private short expiry fixture unavailable")
	}
	blocker, err := pool.Begin(ctx)
	if err != nil {
		t.Fatal("initiative unchanged-row locker unavailable")
	}
	defer blocker.Rollback(ctx)
	if _, err = blocker.Exec(ctx, "SELECT singleton FROM fixture_initiative_native_limit FOR UPDATE"); err != nil {
		t.Fatal("initiative counter row unavailable")
	}
	done := make(chan error, 1)
	go func() { done <- limit.reserve(ctx, "start", "synthetic-expiry-run", nil) }()
	for {
		if _, err = blocker.Exec(ctx, "SELECT pg_stat_clear_snapshot()"); err != nil {
			t.Fatal("initiative expiry wait observer unavailable")
		}
		var waiting, expired bool
		query := "SELECT EXISTS(SELECT 1 FROM pg_stat_activity WHERE datname=current_database() AND pid<>pg_backend_pid() AND wait_event_type='Lock' AND pg_backend_pid()=ANY(pg_blocking_pids(pid)) AND query LIKE 'UPDATE fixture_initiative_native_limit%'),clock_timestamp()>=expires_at FROM fixture_initiative_native_limit"
		if blocker.QueryRow(ctx, query).Scan(&waiting, &expired) != nil {
			t.Fatal("initiative expiry wait evidence unavailable")
		}
		if waiting {
			if expired {
				t.Fatal("initiative wait began after expiry")
			}
			break
		}
		select {
		case <-done:
			t.Fatal("initiative reservation did not wait on the actual row")
		case <-ctx.Done():
			t.Fatal("initiative row wait deadline")
		case <-time.After(10 * time.Millisecond):
		}
	}
	for {
		var expired bool
		if blocker.QueryRow(ctx, "SELECT clock_timestamp()>=expires_at FROM fixture_initiative_native_limit").Scan(&expired) != nil {
			t.Fatal("initiative natural expiry unavailable")
		}
		if expired {
			break
		}
		select {
		case <-done:
			t.Fatal("initiative reservation escaped the unchanged lock")
		case <-ctx.Done():
			t.Fatal("initiative expiry deadline")
		case <-time.After(10 * time.Millisecond):
		}
	}
	if blocker.Rollback(ctx) != nil {
		t.Fatal("initiative unchanged-row release unavailable")
	}
	select {
	case result := <-done:
		if result != errNativeFixtureDenied {
			t.Fatal("initiative expired waited reservation admitted")
		}
	case <-ctx.Done():
		t.Fatal("initiative post-wait expiry deadline")
	}
	var attempts, count int
	if pool.QueryRow(ctx, "SELECT attempts,(SELECT count(*) FROM fixture_initiative_native_attempts) FROM fixture_initiative_native_limit").Scan(&attempts, &count) != nil || attempts != 41 || count != 0 {
		t.Fatal("initiative expired wait spent or retained a provisional request")
	}
}
