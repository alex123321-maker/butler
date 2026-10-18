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
)

func temporalPositiveDayFixture(t *testing.T) (map[string]any, map[string]any, string, time.Time, budget.Units) {
	t.Helper()
	day := "2026-10-18"
	deadline := time.Date(2026, 10, 19, 0, 0, 0, 0, time.UTC)
	quoted := budget.Units{ModelTurns: 2, Tokens: 2162688, ActiveMillis: 600000, ProviderInputBytes: 16777216}
	scopeID := budget.ScopeID("owner_day", temporalNativeOwner, day)
	scope := map[string]any{"scope_id": scopeID, "kind": "owner_day", "owner_id": "owner", "subject_id": day, "deadline": deadline, "used_json": quoted}
	reservation := func(id, op string, charge budget.Units) map[string]any {
		return map[string]any{"reservation_id": id, "operation_id": op, "fingerprint": strings.Repeat("f", 64), "owner_id": "owner", "created_at": "2026-10-18T01:00:00Z", "request_json": budget.Request{OwnerID: "owner", OperationID: op, ScopeIDs: []string{scopeID}, Charge: charge, CostKnown: true}}
	}
	one := budget.Units{ModelTurns: 1, Tokens: 1081344, ActiveMillis: 300000, ProviderInputBytes: 8388608}
	reservations := []any{reservation("seal", "prospective-owner-day-history-freeze:"+strings.Repeat("a", 64), budget.Units{}), reservation("bg-rsv", "bg", one), reservation("fg-rsv", "fg", one)}
	background := []any{map[string]any{"operation_id": "bg", "job_id": "bg-job", "reservation_id": "bg-rsv", "provider": "antigravity", "model": "gemini-3.8-flash-high", "quote": one, "state": "completed", "usage": nil, "created_at": "2026-10-18T01:00:00Z", "observed_at": nil}}
	foreground := []any{map[string]any{"operation_id": "fg", "reservation_id": "fg-rsv", "provider": "antigravity", "model": "gemini-3.8-flash-high", "input_allowance": 1048576, "output_allowance": 32768, "actual_input_tokens": nil, "actual_output_tokens": nil, "created_at": "2026-10-18T01:00:00Z", "observed_at": nil}}
	calls := []any{}
	for i, op := range []string{"bg", "fg"} {
		var usage any
		if i == 0 {
			usage = map[string]any{"InputTokens": 5, "OutputTokens": 2, "TotalTokens": 12}
		}
		calls = append(calls, map[string]any{"entry_id": op + "-entry", "operation_id": op, "reservation_id": op + "-rsv", "observation_json": map[string]any{"EntryID": op + "-entry", "OperationID": op, "ReservationID": op + "-rsv", "Usage": usage, "FinishedAt": "2026-10-18T01:01:00Z"}})
	}
	prior := map[string]any{"format": "butler.temporal.predecessor89.v1", "closed": true, "paid_entries": 2, "scopes": []any{scope}, "reservations": reservations, "model_jobs": background, "background": background, "foreground": foreground, "calls": calls}
	db := func(container, name string, port int, schemas []string) map[string]any {
		windows := []any{}
		for _, schema := range schemas {
			window := map[string]any{"schema": schema, "scopes": []any{}, "reservations": []any{}, "foreground": []any{}, "background": []any{}, "foreground_table_present": true, "background_table_present": true}
			if schema == "consolidation_native_v1" {
				window["scopes"] = []any{scope}
				window["reservations"] = reservations
				window["background"] = background
				window["foreground"] = foreground
			}
			windows = append(windows, window)
		}
		return map[string]any{"container": container, "database": name, "port": port, "schemas": schemas, "windows": windows}
	}
	current := map[string]any{"format": "butler.consolidation.current-owner-day.v2", "day": day, "owner": "owner", "databases": []any{
		db("butler-lab-postgres-1", "butler", 5432, []string{"public"}),
		db("issue84-20261016-pg", "issue84_test", 5432, []string{"summary_native_v1", "summary_native_v2", "summary_native_v3", "summary_native_v4", "summary_native_v5", "summary_native_v6", "summary_native_v7"}),
		db("butler-issue85-postgres", "issue85_native_test", 18585, []string{"memory_temporal_native_v1", "memory_temporal_native_v2", "memory_temporal_native_v3", "memory_temporal_native_v4", "public"}),
		db("butler-issue86-native-postgres-1", "memory_read_native_test", 5432, []string{"public"}),
		db("butler-issue87-native-v1-pg", "memory_adaptive_test", 5432, []string{"adaptive_native_v1"}),
		db("issue86-20261016-pg", "memory_read_test", 5432, []string{"consolidation_native_v1", "public"}),
	}}
	return current, prior, day, deadline, quoted
}

func TestTemporalV5PositiveCarryOneUseIntegration(t *testing.T) {
	pool := memorytest.Database(t)
	var now time.Time
	if pool.QueryRow(context.Background(), `SELECT clock_timestamp()`).Scan(&now) != nil {
		t.Fatal("controlled database clock unavailable")
	}
	h := &temporalV5History{Hash: strings.Repeat("a", 64), Manifest: temporalV5HistoryManifest{Day: now.UTC().Format("2006-01-02"), Deadline: now.UTC().Truncate(24 * time.Hour).Add(24 * time.Hour)}, Carry: budget.Units{ModelTurns: 2, Tokens: 2162688, ActiveMillis: 600000, ProviderInputBytes: 16777216}}
	proof := t.TempDir()
	used, err := temporalApplyV5History(context.Background(), pool, proof, h)
	if err != nil || used != h.Carry {
		t.Fatal("positive original charge import failed", err)
	}
	for _, hash := range []string{h.Hash, strings.Repeat("b", 64)} {
		copy := *h
		copy.Hash = hash
		if _, err = temporalApplyV5History(context.Background(), pool, t.TempDir(), &copy); err == nil {
			t.Fatal("same or another hash imported the predecessor twice")
		}
	}
	var actual budget.Units
	var reserved, models int
	if err = pool.QueryRow(context.Background(), `SELECT used_json,(SELECT count(*) FROM execution_budget_reservations),(SELECT count(*) FROM execution_model_attempts)+(SELECT count(*) FROM memory_model_attempts) FROM execution_budget_scopes WHERE kind='owner_day'`).Scan(&actual, &reserved, &models); err != nil || actual != h.Carry || reserved != 1 || models != 0 {
		t.Fatal("carry refunded, duplicated or counted as another model attempt", actual, reserved, models)
	}
}

func TestTemporalV5CurrentDayPositiveCarryRetainsUnknownUsage(t *testing.T) {
	current, prior, day, deadline, want := temporalPositiveDayFixture(t)
	a, _ := json.Marshal(current)
	b, _ := json.Marshal(prior)
	units, usage, err := temporalValidatePositiveDay(a, b, day, deadline)
	if err != nil || units != want || usage.NativeEntries != 2 || usage.NativeKnownUsage != 1 || usage.NativeUnknownUsage != 1 || usage.KnownFullTokenLowerBound != 12 || usage.ProductAttempts != 2 || usage.ProductNULL != 2 || usage.ProductObserved != 0 {
		t.Fatal("exact positive carry or independent native/product unknowns lost", err, units, usage)
	}
	if string(a) != string(mustTemporalJSON(t, current)) {
		t.Fatal("frozen inventory was mutated")
	}
	p := temporalNativePolicy(units)
	if p.Validate() != nil || p.Day.ModelTurns != 146 || p.Day.ExternalCalls != 96 {
		t.Fatal("prior current charges not included once in finite new policy", p.Day)
	}
}

func mustTemporalJSON(t *testing.T, v any) []byte {
	t.Helper()
	raw, err := json.Marshal(v)
	if err != nil {
		t.Fatal(err)
	}
	return raw
}

func TestTemporalV5PositiveCarryRejectsOmissionRefundAndOtherPopulations(t *testing.T) {
	for _, name := range []string{"open_window", "duplicate_reservation", "missing_product", "known_product_replaces_null", "missing_native_entry", "duplicate_native_entry", "wrong_entry_reservation", "positive_prior_carry", "wrong_day", "negative_units", "missing_main", "main_attempt", "second_schema_spend", "main_table_presence", "changed_original_charge", "missing_current_foreground", "uncorrelated_background_usage", "invalid_native_usage", "omitted_background_usage", "omitted_foreground_observation"} {
		t.Run(name, func(t *testing.T) {
			current, prior, day, deadline, _ := temporalPositiveDayFixture(t)
			dbs := current["databases"].([]any)
			main := dbs[0].(map[string]any)["windows"].([]any)[0].(map[string]any)
			current89 := dbs[5].(map[string]any)["windows"].([]any)[0].(map[string]any)
			switch name {
			case "open_window":
				prior["closed"] = false
			case "duplicate_reservation":
				prior["reservations"] = append(prior["reservations"].([]any), prior["reservations"].([]any)[1])
			case "missing_product":
				prior["model_jobs"] = []any{}
			case "known_product_replaces_null":
				current89["background"] = []any{map[string]any{"operation_id": "bg", "usage": map[string]any{"TotalTokens": 12}}}
			case "missing_native_entry":
				prior["calls"] = []any{} // Missing an entry cannot establish a lower total.
			case "duplicate_native_entry":
				prior["calls"] = append(prior["calls"].([]any), prior["calls"].([]any)[0])
			case "wrong_entry_reservation":
				prior["calls"].([]any)[0].(map[string]any)["reservation_id"] = "foreign"
			case "positive_prior_carry":
				r := prior["reservations"].([]any)[0].(map[string]any)
				q := r["request_json"].(budget.Request)
				q.Charge.ModelTurns = 1
				r["request_json"] = q
			case "wrong_day":
				current["day"] = "2026-10-19"
			case "negative_units":
				s := prior["scopes"].([]any)[0].(map[string]any)
				u := s["used_json"].(budget.Units)
				u.Tokens = -1
				s["used_json"] = u
			case "missing_main":
				current["databases"] = dbs[1:]
			case "main_attempt":
				main["foreground"] = []any{map[string]any{"operation_id": "main-unknown", "observed_at": nil}}
			case "second_schema_spend":
				dbs[1].(map[string]any)["windows"].([]any)[0].(map[string]any)["reservations"] = []any{map[string]any{"operation_id": "other"}}
			case "main_table_presence":
				delete(main, "foreground_table_present")
			case "changed_original_charge":
				r := current89["reservations"].([]any)[1].(map[string]any)
				q := r["request_json"].(budget.Request)
				q.Charge.Tokens--
				r["request_json"] = q
			case "missing_current_foreground":
				delete(current89, "foreground")
			case "uncorrelated_background_usage":
				prior["model_jobs"] = []any{map[string]any{"operation_id": "bg", "usage": map[string]any{"InputTokens": 5, "OutputTokens": 2, "TotalTokens": 12}}}
			case "invalid_native_usage":
				prior["calls"].([]any)[0].(map[string]any)["observation_json"].(map[string]any)["Usage"] = map[string]any{"InputTokens": 5, "OutputTokens": 2, "TotalTokens": -1}
			case "omitted_background_usage":
				// Shared fixture aliases deliberately mutate prior and current:
				// equality alone must not turn an absent nullable field into NULL.
				delete(prior["background"].([]any)[0].(map[string]any), "usage")
			case "omitted_foreground_observation":
				delete(prior["foreground"].([]any)[0].(map[string]any), "observed_at")
			}
			if _, _, err := temporalValidatePositiveDay(mustTemporalJSON(t, current), mustTemporalJSON(t, prior), day, deadline); err == nil {
				t.Fatal("incomplete/refunded current-day population accepted")
			}
		})
	}
}

func TestTemporalV5FrozenHistoryWitnessesRejectUsageForgery(t *testing.T) {
	root := os.Getenv("BUTLER_TEMPORAL_V5_HISTORY_TEST_ROOT")
	if root == "" {
		t.Skip("explicit immutable full-history fixture required")
	}
	for _, change := range []string{"product_null", "native_total", "product_usage_omitted", "foreground_observation_omitted"} {
		t.Run(change, func(t *testing.T) {
			dir := t.TempDir()
			entries, err := os.ReadDir(root)
			if err != nil {
				t.Fatal("frozen history fixture unavailable")
			}
			for _, entry := range entries {
				if !entry.Type().IsRegular() || filepath.Ext(entry.Name()) != ".json" {
					t.Fatal("only bounded regular historical JSON fixtures are permitted")
				}
				raw, err := os.ReadFile(filepath.Join(root, entry.Name()))
				if err != nil || len(raw) > 32*1024*1024 {
					t.Fatal("bounded historical fixture unavailable")
				}
				if os.WriteFile(filepath.Join(dir, entry.Name()), raw, 0600) != nil {
					t.Fatal("controlled immutable fixture copy unavailable")
				}
			}
			raw, err := os.ReadFile(filepath.Join(dir, "history.json"))
			if err != nil {
				t.Fatal(err)
			}
			var manifest temporalV5HistoryManifest
			if json.Unmarshal(raw, &manifest) != nil {
				t.Fatal("controlled history envelope unavailable")
			}
			path := filepath.Join(dir, manifest.Predecessor.Path)
			data, err := os.ReadFile(path)
			if err != nil {
				t.Fatal(err)
			}
			var prior map[string]any
			if json.Unmarshal(data, &prior) != nil {
				t.Fatal("controlled predecessor unavailable")
			}
			if change == "product_null" {
				prior["model_jobs"].([]any)[0].(map[string]any)["usage"] = nil
			} else if change == "native_total" {
				usage := prior["calls"].([]any)[0].(map[string]any)["observation_json"].(map[string]any)["Usage"].(map[string]any)
				usage["TotalTokens"] = usage["TotalTokens"].(float64) + 100
			} else {
				field, key := "background", "usage"
				if change == "foreground_observation_omitted" {
					field, key = "foreground", "observed_at"
				} else {
					delete(prior["model_jobs"].([]any)[0].(map[string]any), key)
				}
				delete(prior[field].([]any)[0].(map[string]any), key)
				currentPath := filepath.Join(dir, manifest.Current.Path)
				currentRaw, err := os.ReadFile(currentPath)
				if err != nil {
					t.Fatal("controlled current inventory unavailable")
				}
				var current map[string]any
				if json.Unmarshal(currentRaw, &current) != nil {
					t.Fatal("controlled current inventory invalid")
				}
				changed := 0
				for _, database := range current["databases"].([]any) {
					db := database.(map[string]any)
					if db["container"] != "issue86-20261016-pg" || db["database"] != "memory_read_test" {
						continue
					}
					for _, value := range db["windows"].([]any) {
						window := value.(map[string]any)
						if window["schema"] == "consolidation_native_v1" {
							delete(window[field].([]any)[0].(map[string]any), key)
							changed++
						}
					}
				}
				if changed != 1 {
					t.Fatal("one controlled current population required")
				}
				currentRaw = mustTemporalJSON(t, current)
				if os.WriteFile(currentPath, currentRaw, 0600) != nil {
					t.Fatal("controlled current inventory write unavailable")
				}
				manifest.Current.SHA256 = experienceDigest(string(currentRaw))
			}
			data = mustTemporalJSON(t, prior)
			if os.WriteFile(path, data, 0600) != nil {
				t.Fatal("controlled predecessor write unavailable")
			}
			manifest.Predecessor.SHA256 = experienceDigest(string(data))
			raw = mustTemporalJSON(t, manifest)
			if os.WriteFile(filepath.Join(dir, "history.json"), raw, 0600) != nil {
				t.Fatal("controlled manifest write unavailable")
			}
			if _, err = loadTemporalV5History(filepath.Join(dir, "history.json"), experienceDigest(string(raw))); err == nil {
				t.Fatal("forged usage diverged from current/original witnesses but was accepted")
			}
		})
	}
}
