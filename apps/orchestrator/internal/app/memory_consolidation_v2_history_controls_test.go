package app

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/butler/butler/internal/budget"
	"github.com/butler/butler/internal/memory/memorytest"
)

func TestConsolidationV2CurrentDayCarryOneUseIntegration(t *testing.T) {
	pool := memorytest.Database(t)
	ctx := context.Background()
	var now time.Time
	if err := pool.QueryRow(ctx, `SELECT clock_timestamp()`).Scan(&now); err != nil {
		t.Fatal(err)
	}
	history := &consolidationScreenHistory{Hash: strings.Repeat("a", 64), ProspectiveDay: now.UTC().Format("2006-01-02"), ProspectiveDeadline: now.UTC().Truncate(24 * time.Hour).Add(24 * time.Hour), CurrentCarry: budget.Units{ModelTurns: 3, Tokens: 3244032, ActiveMillis: 900000, ProviderInputBytes: 25165824}, Successor: &consolidationV2Summary{Schema: "consolidation_native_v2"}}
	policy := consolidationScreenPolicy("antigravity", "gemini-3.8-flash-high", history.CurrentCarry)
	start, results := make(chan struct{}), make(chan error, 2)
	for i := 0; i < 2; i++ {
		go func() { <-start; results <- consolidationV2ApplyHistory(ctx, pool, history, policy) }()
	}
	close(start)
	passed := 0
	for i := 0; i < 2; i++ {
		if <-results == nil {
			passed++
		}
	}
	if passed != 1 {
		t.Fatal("concurrent import did not produce exactly one winner", passed)
	}
	copy := *history
	copy.Hash = strings.Repeat("b", 64)
	if consolidationV2ApplyHistory(ctx, pool, &copy, policy) == nil {
		t.Fatal("another history hash imported the same current day twice")
	}
	var used budget.Units
	var reservations, attempts, markers int
	if err := pool.QueryRow(ctx, `SELECT used_json,(SELECT count(*)FROM execution_budget_reservations),(SELECT count(*)FROM execution_model_attempts)+(SELECT count(*)FROM memory_model_attempts),(SELECT count(*)FROM consolidation_native_current_day_carry)FROM execution_budget_scopes WHERE kind='owner_day'`).Scan(&used, &reservations, &attempts, &markers); err != nil || used != history.CurrentCarry || reservations != 1 || attempts != 0 || markers != 1 {
		t.Fatal("positive import duplicated/refunded quote or fabricated inference", used, reservations, attempts, markers, err)
	}
}

func consolidationV2Fixture(t *testing.T) (map[string]any, map[string]any, map[string]any, []any, map[int64][]byte, *temporalV5History, []byte, string, time.Time) {
	t.Helper()
	current, old, day, deadline, carry := temporalPositiveDayFixture(t)
	oldRaw := mustTemporalJSON(t, old)
	_, oldUsage, err := temporalValidatePositiveDay(mustTemporalJSON(t, current), oldRaw, day, deadline)
	if err != nil {
		t.Fatal(err)
	}
	previous := &temporalV5History{Hash: strings.Repeat("a", 64), Carry: carry, Usage: oldUsage}
	scopeID := budget.ScopeID("owner_day", "owner", day)
	one := budget.Units{ModelTurns: 1, Tokens: 1081344, ActiveMillis: 300000, ProviderInputBytes: 8388608}
	used := consolidationAddUnits(carry, one)
	scope := map[string]any{"scope_id": scopeID, "kind": "owner_day", "owner_id": "owner", "subject_id": day, "deadline": deadline, "used_json": used}
	reservation := func(id, operation string, charge budget.Units) any {
		return map[string]any{"reservation_id": id, "operation_id": operation, "fingerprint": strings.Repeat("f", 64), "owner_id": "owner", "created_at": "2026-10-18T02:00:00Z", "request_json": budget.Request{OwnerID: "owner", OperationID: operation, ScopeIDs: []string{scopeID}, Charge: charge, CostKnown: true}}
	}
	reservations := []any{reservation("carry88", "issue88-v5-current-day-carry:"+previous.Hash, carry), reservation("native88-reservation", "native88-operation", one)}
	foreground := map[string]any{"operation_id": "native88-operation", "reservation_id": "native88-reservation", "provider": "antigravity", "model": "gemini-3.8-flash-high", "input_allowance": 1048576, "output_allowance": 32768, "actual_input_tokens": 5, "actual_output_tokens": 2, "created_at": "2026-10-18T02:00:00Z", "observed_at": "2026-10-18T02:01:00Z"}
	observation := map[string]any{"id": 1, "run_id": "native88-run", "kind": "start", "operation_id": "native88-operation", "reservation_id": "native88-reservation", "usage": map[string]any{"InputTokens": 5, "OutputTokens": 2, "TotalTokens": 12}}
	entry := map[string]any{"id": 1, "run_id": "native88-run", "kind": "start", "finished_at": "2026-10-18T02:01:00Z", "observation_json": observation}
	prior := map[string]any{"format": "butler.consolidation.predecessor88.v1", "window": map[string]any{"sealed": true, "closed": true, "paid_entries": 1}, "scopes": []any{scope}, "reservations": reservations, "foreground": []any{foreground}, "background": []any{}, "entries": []any{entry}}
	var ledger map[string]any
	_ = json.Unmarshal(mustTemporalJSON(t, prior), &ledger)
	originalFG := ledger["foreground"].([]any)[0].(map[string]any)
	originalFG["run_id"] = "native88-run"
	originalEntry := ledger["entries"].([]any)[0].(map[string]any)
	originalEntry["operation_id"], originalEntry["reservation_id"] = "native88-operation", "native88-reservation"
	cases, outcomes := []any{}, []any{}
	for i := 1; i <= 12; i++ {
		id := fmt.Sprintf("t%02d", i)
		state, run, n := "not_executed", "", 0
		if i == 1 {
			state, run, n = "completed", "native88-run", 1
		}
		cases = append(cases, map[string]any{"case_id": id, "status": state, "run_id": run, "native_entries": n})
		outcomes = append(outcomes, map[string]any{"case_id": id, "status": state, "run_id": run, "native_entries": n})
	}
	ledger["cases"] = cases
	entries := map[int64][]byte{1: mustTemporalJSON(t, observation)}
	db := current["databases"].([]any)[2].(map[string]any)
	db["schemas"] = append(db["schemas"].([]string), "memory_temporal_native_v5")
	db["windows"] = append(db["windows"].([]any), map[string]any{"schema": "memory_temporal_native_v5", "foreground_table_present": true, "background_table_present": true, "scopes": prior["scopes"], "reservations": prior["reservations"], "foreground": prior["foreground"], "background": prior["background"]})
	return current, prior, ledger, outcomes, entries, previous, oldRaw, day, deadline
}

func TestConsolidationV2LatestLeafCountsOriginalAttemptsOnce(t *testing.T) {
	current, prior, ledger, outcomes, entries, previous, oldRaw, day, deadline := consolidationV2Fixture(t)
	p, l, o := mustTemporalJSON(t, prior), mustTemporalJSON(t, ledger), mustTemporalJSON(t, outcomes)
	if err := consolidationV2Corroborate(p, l, o, entries, day); err != nil {
		t.Fatal(err)
	}
	carry, usage, err := consolidationV2ValidateLeaf(p, l, previous, day, deadline)
	if err != nil || carry.ModelTurns != 3 || usage.NativeEntries != 1 || usage.KnownFullTokenLowerBound != 12 || usage.ProductAttempts != 1 || usage.ProductObserved != 1 {
		t.Fatal("latest leaf lost exact original quotes/usage", carry, usage, err)
	}
	if err = consolidationV2ValidateInventory(mustTemporalJSON(t, current), p, previous, oldRaw, day, deadline); err != nil {
		t.Fatal(err)
	}
	all := consolidationV2AddUsage(previous.Usage, usage)
	if all.NativeEntries != 3 || all.NativeUnknownUsage != 1 || all.ProductNULL != 2 || all.ProductAttempts != 3 || all.KnownFullTokenLowerBound != 24 {
		t.Fatal("predecessor scope counted as extra inference", all)
	}
	policy := consolidationScreenPolicy("antigravity", "gemini-3.8-flash-high", carry)
	if policy.Validate() != nil || policy.Day.ModelTurns != 102 || policy.Day.ExternalCalls != 96 {
		t.Fatal("new cohort imported inclusive leaf more than once")
	}
}

func TestConsolidationV2RetainsNativeUnknownAndAdmittedWithoutEntry(t *testing.T) {
	_, prior, ledger, outcomes, entries, previous, _, day, deadline := consolidationV2Fixture(t)
	prior["entries"].([]any)[0].(map[string]any)["observation_json"].(map[string]any)["usage"] = nil
	ledger["entries"].([]any)[0].(map[string]any)["observation_json"].(map[string]any)["usage"] = nil
	entries[1] = mustTemporalJSON(t, prior["entries"].([]any)[0].(map[string]any)["observation_json"])
	// A separately admitted foreground failure can stop before the native
	// observer. Its original quote/NULL row remains charged, without inventing
	// a native entry or assigning measured zero usage.
	quote := budget.Units{ModelTurns: 1, Tokens: 1081344, ActiveMillis: 300000, ProviderInputBytes: 8388608}
	scope := prior["scopes"].([]any)[0].(map[string]any)
	scope["used_json"] = consolidationAddUnits(scope["used_json"].(budget.Units), quote)
	request := budget.Request{OwnerID: "owner", OperationID: "admitted-no-native", ScopeIDs: []string{budget.ScopeID("owner_day", "owner", day)}, Charge: quote, CostKnown: true}
	reservation := map[string]any{"reservation_id": "admitted-no-native-rsv", "operation_id": request.OperationID, "fingerprint": strings.Repeat("e", 64), "owner_id": "owner", "created_at": "2026-10-18T02:02:00Z", "request_json": request}
	prior["reservations"] = append(prior["reservations"].([]any), reservation)
	product := map[string]any{"operation_id": request.OperationID, "reservation_id": "admitted-no-native-rsv", "provider": "antigravity", "model": "gemini-3.8-flash-high", "input_allowance": 1048576, "output_allowance": 32768, "actual_input_tokens": nil, "actual_output_tokens": nil, "created_at": "2026-10-18T02:02:00Z", "observed_at": nil}
	prior["foreground"] = append(prior["foreground"].([]any), product)
	ledger["scopes"], ledger["reservations"] = prior["scopes"], prior["reservations"]
	var originalProduct map[string]any
	_ = json.Unmarshal(mustTemporalJSON(t, product), &originalProduct)
	originalProduct["run_id"] = "admitted-no-native-run"
	ledger["foreground"] = append(ledger["foreground"].([]any), originalProduct)
	for _, row := range []any{ledger["cases"].([]any)[1], outcomes[1]} {
		item := row.(map[string]any)
		item["run_id"], item["status"] = "admitted-no-native-run", "failed"
	}
	p, l := mustTemporalJSON(t, prior), mustTemporalJSON(t, ledger)
	if err := consolidationV2Corroborate(p, l, mustTemporalJSON(t, outcomes), entries, day); err != nil {
		t.Fatal(err)
	}
	carry, usage, err := consolidationV2ValidateLeaf(p, l, previous, day, deadline)
	if err != nil || carry.ModelTurns != 4 || usage.NativeEntries != 1 || usage.NativeUnknownUsage != 1 || usage.NativeKnownUsage != 0 || usage.KnownFullTokenLowerBound != 0 || usage.ProductAttempts != 2 || usage.ProductObserved != 1 || usage.ProductNULL != 1 || usage.ProductWithoutNativeEntry != 1 {
		t.Fatal("unknown or admitted quote became refund/native measured zero", carry, usage, err)
	}
}

func TestConsolidationV2RejectsChangedWitnessesOrExtraOwnerRows(t *testing.T) {
	for _, name := range []string{"carry_refund", "native_total", "entry_reservation", "missing_product_null_key", "missing_entry_file", "extra_main_attempt", "missing_latest_window", "changed_original_outcome", "duplicate_native_entry"} {
		t.Run(name, func(t *testing.T) {
			current, prior, ledger, outcomes, entries, previous, oldRaw, day, deadline := consolidationV2Fixture(t)
			switch name {
			case "carry_refund":
				r := prior["reservations"].([]any)[0].(map[string]any)
				q := r["request_json"].(budget.Request)
				q.Charge.ModelTurns--
				r["request_json"] = q
			case "native_total":
				prior["entries"].([]any)[0].(map[string]any)["observation_json"].(map[string]any)["usage"].(map[string]any)["TotalTokens"] = 112
			case "entry_reservation":
				prior["entries"].([]any)[0].(map[string]any)["observation_json"].(map[string]any)["reservation_id"] = "foreign"
			case "missing_product_null_key":
				delete(prior["foreground"].([]any)[0].(map[string]any), "observed_at")
			case "missing_entry_file":
				delete(entries, 1)
			case "extra_main_attempt":
				current["databases"].([]any)[0].(map[string]any)["windows"].([]any)[0].(map[string]any)["foreground"] = []any{map[string]any{"operation_id": "unreviewed", "observed_at": nil}}
			case "missing_latest_window":
				db := current["databases"].([]any)[2].(map[string]any)
				db["windows"] = db["windows"].([]any)[:5]
				db["schemas"] = db["schemas"].([]string)[:5]
			case "changed_original_outcome":
				outcomes[0].(map[string]any)["native_entries"] = 0
			case "duplicate_native_entry":
				prior["entries"] = append(prior["entries"].([]any), prior["entries"].([]any)[0])
			}
			p, l := mustTemporalJSON(t, prior), mustTemporalJSON(t, ledger)
			e1 := consolidationV2Corroborate(p, l, mustTemporalJSON(t, outcomes), entries, day)
			_, _, e2 := consolidationV2ValidateLeaf(p, l, previous, day, deadline)
			e3 := consolidationV2ValidateInventory(mustTemporalJSON(t, current), p, previous, oldRaw, day, deadline)
			if e1 == nil && e2 == nil && e3 == nil {
				t.Fatal("changed successor evidence accepted")
			}
		})
	}
}
