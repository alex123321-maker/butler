package app

import (
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/butler/butler/internal/budget"
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
	background := []any{map[string]any{"operation_id": "bg", "usage": nil}}
	foreground := []any{map[string]any{"operation_id": "fg", "observed_at": nil}}
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
	for _, name := range []string{"open_window", "duplicate_reservation", "missing_product", "known_product_replaces_null", "missing_native_entry", "duplicate_native_entry", "wrong_entry_reservation", "positive_prior_carry", "wrong_day", "negative_units", "missing_main", "main_attempt", "second_schema_spend", "main_table_presence", "changed_original_charge", "missing_current_foreground"} {
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
			}
			if _, _, err := temporalValidatePositiveDay(mustTemporalJSON(t, current), mustTemporalJSON(t, prior), day, deadline); err == nil {
				t.Fatal("incomplete/refunded current-day population accepted")
			}
		})
	}
}
