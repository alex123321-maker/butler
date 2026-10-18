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

func TestTemporalV8FactsContractBoundary(t *testing.T) {
	for _, item := range []struct {
		version  int
		revision string
	}{{6, "3"}, {7, "3"}, {8, "4"}, {9, "5"}, {0, ""}, {10, ""}} {
		if got := temporalNativeFactsRevision(item.version); got != item.revision {
			t.Fatal("historical versus prospective contract boundary changed", item.version, got)
		}
	}
	h := &temporalV5History{SuccessorV8: &closedTemporalV7History{}}
	if temporalNativeVersion(h) != 8 || temporalNativeFactsRevision(temporalNativeVersion(h)) != "4" {
		t.Fatal("new whole cohort did not select contract4")
	}
}

func TestTemporalV8CurrentDayCarryOneUseIntegration(t *testing.T) {
	pool := memorytest.Database(t)
	ctx := context.Background()
	var now time.Time
	if pool.QueryRow(ctx, `SELECT clock_timestamp()`).Scan(&now) != nil {
		t.Fatal("database clock unavailable")
	}
	h := &temporalV5History{Hash: strings.Repeat("c", 64), Manifest: temporalV5HistoryManifest{Day: now.UTC().Format("2006-01-02"), Deadline: now.UTC().Truncate(24 * time.Hour).Add(24 * time.Hour)}, Carry: budget.Units{ModelTurns: 145, Tokens: 156794880, ActiveMillis: 55380000, ProviderInputBytes: 1216348160, ExternalCalls: 99, EgressBytes: 415236096}, SuccessorV8: &closedTemporalV7History{}}
	if used, err := temporalApplyV5History(ctx, pool, t.TempDir(), h); err != nil || used != h.Carry {
		t.Fatal("inclusive145 import changed", used, err)
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
	if err := pool.QueryRow(ctx, `SELECT used_json,(SELECT count(*)FROM execution_budget_reservations),(SELECT count(*)FROM execution_model_attempts)+(SELECT count(*)FROM memory_model_attempts),(SELECT operation_id FROM execution_budget_reservations) FROM execution_budget_scopes WHERE kind='owner_day'`).Scan(&used, &count, &attempts, &operation); err != nil || used != h.Carry || count != 1 || attempts != 0 || operation != "issue88-v8-current-day-carry:"+h.Hash {
		t.Fatal("carry duplicated/refunded or fabricated inference", used, count, attempts, err)
	}
}

func TestTemporalV8NamespaceClaimOneUseIntegration(t *testing.T) {
	if os.Getenv("BUTLER_TEMPORAL_V8_NAMESPACE_CONTROL") != "1" {
		t.Skip("dedicated disposable V8 database only")
	}
	ctx := context.Background()
	pool, err := pgxpool.New(ctx, os.Getenv("BUTLER_TEST_MEMORY_POSTGRES_URL"))
	if err != nil {
		t.Fatal("dedicated PG unavailable")
	}
	defer pool.Close()
	if pool.Config().ConnConfig.Database != "temporal_v8_history_test" {
		t.Fatal("exact dedicated namespace-control database required")
	}
	if _, err = pool.Exec(ctx, `CREATE SCHEMA memory_temporal_native_v8`); err != nil {
		t.Fatal(err)
	}
	if temporalClaimNativeWindow(ctx, pool, 8) == nil {
		t.Fatal("existing empty namespace reopened")
	}
	var table bool
	if pool.QueryRow(ctx, `SELECT to_regclass('public.issue88_native_windows') IS NOT NULL`).Scan(&table) != nil || table {
		t.Fatal("failed namespace claim consumed receipt")
	}
	if _, err = pool.Exec(ctx, `DROP SCHEMA memory_temporal_native_v8`); err != nil {
		t.Fatal(err)
	}
	if err = temporalClaimNativeWindow(ctx, pool, 8); err != nil {
		t.Fatal("fresh atomic claim failed", err)
	}
	if temporalClaimNativeWindow(ctx, pool, 8) == nil {
		t.Fatal("consumed V8 cohort reopened")
	}
	var count int
	if pool.QueryRow(ctx, `SELECT count(*)FROM public.issue88_native_windows WHERE window_id='temporal-explicit-v8'`).Scan(&count) != nil || count != 1 {
		t.Fatal("V8 receipt duplicated")
	}
}

func TestTemporalV8FrozenHistoryRejectsForgeries(t *testing.T) {
	root := os.Getenv("BUTLER_TEMPORAL_V8_HISTORY_TEST_ROOT")
	if root == "" {
		t.Skip("explicit immutable whole V8 history required")
	}
	raw, err := os.ReadFile(filepath.Join(root, "history.json"))
	if err != nil {
		t.Fatal(err)
	}
	h, err := loadTemporalV5History(filepath.Join(root, "history.json"), experienceDigest(string(raw)))
	if err != nil || h.SuccessorV8 == nil || h.Carry.ModelTurns != 145 || h.Usage.NativeEntries != 145 || h.Usage.NativeUnknownUsage != 1 || h.Usage.KnownFullTokenLowerBound != 1105554 || temporalNativeVersion(h) != 8 {
		t.Fatal("actual wholeV8 closed145 predecessor invalid", err)
	}
	for _, name := range []string{"future_schema", "future_receipt", "missing_future", "wrong_schema", "wrong_day", "wrong_deadline", "wrong_previous_hash"} {
		t.Run(name, func(t *testing.T) {
			dir := temporalV6CopyHistory(t, root)
			var m temporalV8Manifest
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
				t.Fatal("forged V8 wrapper accepted")
			}
		})
	}
}
