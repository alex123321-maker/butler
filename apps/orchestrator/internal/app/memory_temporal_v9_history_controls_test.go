package app

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/butler/butler/internal/budget"
	"github.com/butler/butler/internal/memory/memorytest"
	"github.com/jackc/pgx/v5/pgxpool"
)

func TestTemporalV9FactsContractBoundary(t *testing.T) {
	for _, item := range []struct {
		version  int
		revision string
	}{{6, "3"}, {7, "3"}, {8, "4"}, {9, "5"}, {0, ""}, {10, ""}} {
		if got := temporalNativeFactsRevision(item.version); got != item.revision {
			t.Fatal("historical versus prospective contract boundary changed", item.version, got)
		}
	}
	h := &temporalV5History{SuccessorV9: &closedOwnerDayHistory{}}
	if temporalNativeVersion(h) != 9 || temporalNativeFactsRevision(temporalNativeVersion(h)) != "5" {
		t.Fatal("new whole cohort did not select contract5")
	}
}

func TestTemporalV9CurrentDayCarryOneUseIntegration(t *testing.T) {
	pool := memorytest.Database(t)
	ctx := context.Background()
	var now time.Time
	if pool.QueryRow(ctx, `SELECT clock_timestamp()`).Scan(&now) != nil {
		t.Fatal("database clock unavailable")
	}
	h := &temporalV5History{Hash: strings.Repeat("c", 64), Manifest: temporalV5HistoryManifest{Day: now.UTC().Format("2006-01-02"), Deadline: now.UTC().Truncate(24 * time.Hour).Add(24 * time.Hour)}, Carry: budget.Units{ModelTurns: 175, Tokens: 189235200, ActiveMillis: 66540000, ProviderInputBytes: 1468006400, ExternalCalls: 117, EgressBytes: 490733568}, SuccessorV9: &closedOwnerDayHistory{}}
	if used, err := temporalApplyV5History(ctx, pool, t.TempDir(), h); err != nil || used != h.Carry {
		t.Fatal("inclusive175 import changed", used, err)
	}
	for _, hash := range []string{h.Hash, strings.Repeat("d", 64)} {
		copy := *h
		copy.Hash = hash
		if _, err := temporalApplyV5History(ctx, pool, t.TempDir(), &copy); err == nil {
			t.Fatal("inclusive current charge imported again")
		}
	}
	var used budget.Units
	var count, attempts int
	var operation string
	if err := pool.QueryRow(ctx, `SELECT used_json,(SELECT count(*)FROM execution_budget_reservations),(SELECT count(*)FROM execution_model_attempts)+(SELECT count(*)FROM memory_model_attempts),(SELECT operation_id FROM execution_budget_reservations) FROM execution_budget_scopes WHERE kind='owner_day'`).Scan(&used, &count, &attempts, &operation); err != nil || used != h.Carry || count != 1 || attempts != 0 || operation != "issue88-v9-current-day-carry:"+h.Hash {
		t.Fatal("carry duplicated/refunded or fabricated inference", used, count, attempts, err)
	}
}

func TestTemporalV9NamespaceClaimOneUseIntegration(t *testing.T) {
	if os.Getenv("BUTLER_TEMPORAL_V9_NAMESPACE_CONTROL") != "1" {
		t.Skip("dedicated disposable V9 database only")
	}
	ctx := context.Background()
	pool, err := pgxpool.New(ctx, os.Getenv("BUTLER_TEST_MEMORY_POSTGRES_URL"))
	if err != nil {
		t.Fatal("dedicated PG unavailable")
	}
	defer pool.Close()
	if pool.Config().ConnConfig.Database != "temporal_v9_history_test" {
		t.Fatal("exact dedicated namespace-control database required")
	}
	if _, err = pool.Exec(ctx, `CREATE SCHEMA memory_temporal_native_v9`); err != nil {
		t.Fatal(err)
	}
	if temporalClaimNativeWindow(ctx, pool, 9) == nil {
		t.Fatal("existing empty namespace reopened")
	}
	var table bool
	if pool.QueryRow(ctx, `SELECT to_regclass('public.issue88_native_windows') IS NOT NULL`).Scan(&table) != nil || table {
		t.Fatal("failed namespace claim consumed receipt")
	}
	if _, err = pool.Exec(ctx, `DROP SCHEMA memory_temporal_native_v9`); err != nil {
		t.Fatal(err)
	}
	if err = temporalClaimNativeWindow(ctx, pool, 9); err != nil {
		t.Fatal("fresh atomic claim failed", err)
	}
	if temporalClaimNativeWindow(ctx, pool, 9) == nil {
		t.Fatal("consumed V9 cohort reopened")
	}
	var count int
	if pool.QueryRow(ctx, `SELECT count(*)FROM public.issue88_native_windows WHERE window_id='temporal-explicit-v9'`).Scan(&count) != nil || count != 1 {
		t.Fatal("V9 receipt duplicated")
	}
}

func TestTemporalV9FrozenHistoryRejectsForgeries(t *testing.T) {
	root := os.Getenv("BUTLER_TEMPORAL_V9_HISTORY_TEST_ROOT")
	if root == "" {
		t.Skip("explicit immutable whole V9 history required")
	}
	raw, err := os.ReadFile(filepath.Join(root, "history.json"))
	if err != nil {
		t.Fatal(err)
	}
	h, err := loadTemporalV5History(filepath.Join(root, "history.json"), experienceDigest(string(raw)))
	if err != nil || h.SuccessorV9 == nil || h.Carry.ModelTurns != 175 || h.Usage.NativeEntries != 175 || h.Usage.NativeUnknownUsage != 1 || h.Usage.KnownFullTokenLowerBound != 1321800 || temporalNativeVersion(h) != 8 {
		t.Fatal("actual wholeV9 closed175 predecessor invalid", err)
	}
	for _, name := range []string{"future_schema", "future_receipt", "missing_future", "wrong_schema", "wrong_day", "wrong_deadline", "wrong_previous_hash"} {
		t.Run(name, func(t *testing.T) {
			dir := temporalV6CopyHistory(t, root)
			var m temporalV9Manifest
			if json.Unmarshal(raw, &m) != nil {
				t.Fatal("manifest invalid")
			}
			switch name {
			case "future_schema":
				v := true
				m.Future.Present = &v
			case "future_receipt":
				v := 1
				m.Future.Count = &v
			case "missing_future":
				m.Future.Present = nil
			case "wrong_schema":
				m.Schema = temporalV7Schema
			case "wrong_day":
				m.Day = "2026-10-15"
			case "wrong_deadline":
				m.Deadline = m.Deadline.Add(time.Hour)
			case "wrong_previous_hash":
				m.Previous.SHA256 = strings.Repeat("f", 64)
			}
			data := mustTemporalJSON(t, m)
			if os.WriteFile(filepath.Join(dir, "history.json"), data, 0600) != nil {
				t.Fatal("manifest copy failed")
			}
			if _, err := loadTemporalV5History(filepath.Join(dir, "history.json"), experienceDigest(string(data))); err == nil {
				t.Fatal("forged V9 wrapper accepted")
			}
		})
	}
}
