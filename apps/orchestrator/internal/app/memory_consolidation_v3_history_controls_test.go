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
	"github.com/butler/butler/internal/ingress"
	"github.com/butler/butler/internal/memory/memorytest"
	"github.com/butler/butler/internal/owneridentity"
)

func TestConsolidationV3WorkerRejectsOwnerControlIdentity(t *testing.T) {
	ctx := owneridentity.WithActor(context.Background(), owneridentity.Actor{OwnerID: "owner", Channel: "web", DeliveryID: "controlled"})
	x := &consolidationOrdinaryExecutor{done: make(chan consolidationOrdinaryExecution, 1)}
	if _, err := x.Execute(ctx, ingress.InputEvent{}); err == nil {
		t.Fatal("owner actor reached foreground execution")
	}
	if got := <-x.done; got.err == nil || got.result != nil {
		t.Fatal("identity rejection fabricated run")
	}
	if _, err := (&consolidationScreenProvider{}).reserve(ctx, "run", "start", nil); err == nil {
		t.Fatal("owner actor reached model admission")
	}
}

func TestConsolidationV3LatestTemporalLeafCountsOnce(t *testing.T) {
	_, prior, ledger, outcomes, entries, previous, _, day, deadline := consolidationV2Fixture(t)
	prior["format"] = "butler.consolidation.predecessor88.v2"
	for _, row := range []any{prior["reservations"].([]any)[0], ledger["reservations"].([]any)[0]} {
		r := row.(map[string]any)
		r["operation_id"] = "issue88-v6-current-day-carry:" + previous.Hash
		var q budget.Request
		if json.Unmarshal(mustTemporalJSON(t, r["request_json"]), &q) != nil {
			t.Fatal("fixture request unavailable")
		}
		q.OperationID = r["operation_id"].(string)
		r["request_json"] = q
	}
	p, l := mustTemporalJSON(t, prior), mustTemporalJSON(t, ledger)
	if err := consolidationV2Corroborate(p, l, mustTemporalJSON(t, outcomes), entries, day); err != nil {
		t.Fatal(err)
	}
	carry, usage, err := consolidationValidateTemporalLeaf(p, l, previous, day, deadline, "butler.consolidation.predecessor88.v2", "issue88-v6-current-day-carry:")
	if err != nil || carry.ModelTurns != previous.Carry.ModelTurns+1 || usage.NativeEntries != 1 || usage.KnownFullTokenLowerBound != 12 {
		t.Fatal("latest temporal charge duplicated or omitted", carry, usage, err)
	}
	if _, _, err = consolidationV2ValidateLeaf(p, l, previous, day, deadline); err == nil {
		t.Fatal("new v6 leaf accepted under old v5 carry recipe")
	}
}

func TestConsolidationV3CurrentDayCarryOneUseIntegration(t *testing.T) {
	pool := memorytest.Database(t)
	ctx := context.Background()
	var now time.Time
	if pool.QueryRow(ctx, `SELECT clock_timestamp()`).Scan(&now) != nil {
		t.Fatal("database clock unavailable")
	}
	h := &consolidationScreenHistory{Hash: strings.Repeat("c", 64), ProspectiveDay: now.UTC().Format("2006-01-02"), ProspectiveDeadline: now.UTC().Truncate(24 * time.Hour).Add(24 * time.Hour), CurrentCarry: budget.Units{ModelTurns: 50, Tokens: 54067200, ActiveMillis: 18840000, ProviderInputBytes: 419430400, ExternalCalls: 32, EgressBytes: 134217728}, Successor: &consolidationV2Summary{Schema: consolidationV3Schema}}
	policy := consolidationScreenPolicy("antigravity", "gemini-3.8-flash-high", h.CurrentCarry)
	start, results := make(chan struct{}), make(chan error, 2)
	for i := 0; i < 2; i++ {
		go func() { <-start; results <- consolidationV2ApplyHistory(ctx, pool, h, policy) }()
	}
	close(start)
	winners := 0
	for i := 0; i < 2; i++ {
		if <-results == nil {
			winners++
		}
	}
	copy := *h
	copy.Hash = strings.Repeat("d", 64)
	if winners != 1 || consolidationV2ApplyHistory(ctx, pool, &copy, policy) == nil {
		t.Fatal("current carry can be imported twice", winners)
	}
	var used budget.Units
	var operation string
	var quotes, attempts, receipts int
	if err := pool.QueryRow(ctx, `SELECT used_json,(SELECT operation_id FROM execution_budget_reservations),(SELECT count(*) FROM execution_budget_reservations),(SELECT count(*) FROM execution_model_attempts)+(SELECT count(*) FROM memory_model_attempts),(SELECT count(*) FROM consolidation_native_current_day_carry) FROM execution_budget_scopes WHERE kind='owner_day'`).Scan(&used, &operation, &quotes, &attempts, &receipts); err != nil || used != h.CurrentCarry || quotes != 1 || attempts != 0 || receipts != 1 || operation != "issue89-v3-current-day-carry:"+h.Hash {
		t.Fatal("carry refunded, duplicated or fabricated model work", used, quotes, attempts, receipts, err)
	}
	if policy.Validate() != nil || policy.Day.ModelTurns != 149 || policy.Day.ExternalCalls != 128 {
		t.Fatal("full9 finite99/96 allowance changed")
	}
}

func TestConsolidationV3NamespaceClaimOneUseIntegration(t *testing.T) {
	if os.Getenv("BUTLER_CONSOLIDATION_V3_NAMESPACE_CONTROL") != "1" {
		t.Fatal("namespace control requires its dedicated disposable PostgreSQL")
	}
	pool := memorytest.Database(t)
	ctx := context.Background()
	var database string
	if pool.QueryRow(ctx, `SELECT current_database()`).Scan(&database) != nil || database != "consolidation_v3_history_test" {
		t.Fatal("namespace control cannot consume a retained native namespace")
	}
	if err := consolidationV3ClaimSchema(ctx, pool, strings.Repeat("a", 64)); err != nil {
		t.Fatal(err)
	}
	if err := consolidationV3ClaimSchema(ctx, pool, strings.Repeat("b", 64)); err == nil {
		t.Fatal("used namespace reopened under another history")
	}
	var count, attempts int
	var schema, hash string
	if err := pool.QueryRow(ctx, `SELECT count(*),min(schema_name),min(history_sha256),(SELECT count(*) FROM execution_model_attempts)+(SELECT count(*) FROM memory_model_attempts) FROM public.issue89_native_windows WHERE window_id=$1`, consolidationV3Window).Scan(&count, &schema, &hash, &attempts); err != nil || count != 1 || schema != consolidationV3Schema || hash != strings.Repeat("a", 64) || attempts != 0 {
		t.Fatal("namespace receipt changed or invented inference", err)
	}
}

func TestConsolidationV3FrozenHistoryRejectsForgeries(t *testing.T) {
	root := os.Getenv("BUTLER_CONSOLIDATION_V3_HISTORY_TEST_ROOT")
	if root == "" {
		t.Skip("explicit immutable complete seven-store v3 history required")
	}
	raw, err := os.ReadFile(filepath.Join(root, "history.json"))
	if err != nil {
		t.Fatal(err)
	}
	h, err := loadConsolidationV3History(filepath.Join(root, "history.json"), experienceDigest(string(raw)))
	if err != nil || h.Successor == nil || h.Successor.Schema != consolidationV3Schema || h.Attempts != 161 || h.Unknown != 8 || h.ProductUnknown != 9 || h.CurrentCarry.ModelTurns <= 49 || h.Successor.Previous89.NativeEntries != 49 || h.Successor.SeparateAdaptive.NativeEntries != 98 || h.Successor.SeparateAdaptive.NativeUnknownUsage != 1 {
		t.Fatal("complete original history did not validate", err)
	}
	for _, name := range []string{"native_total", "omitted_product_null", "entry_file_usage", "entry_operation", "outcome_denominator", "altered_carry", "scope_field_omission", "missing_seventh", "extra_main_attempt", "extra_seventh_attempt", "changed_recursive_history", "adaptive_native_usage", "adaptive_unknown_omitted", "future_schema_exists", "future_receipt_consumed", "future_proof_omitted"} {
		t.Run(name, func(t *testing.T) {
			dir := temporalV6CopyHistory(t, root)
			var m consolidationV3Manifest
			if json.Unmarshal(raw, &m) != nil {
				t.Fatal("manifest unavailable")
			}
			change := func(ref *consolidationHistoryRef, edit func(map[string]any)) {
				data, err := os.ReadFile(filepath.Join(dir, ref.Path))
				if err != nil {
					t.Fatal(err)
				}
				var p map[string]any
				if json.Unmarshal(data, &p) != nil {
					t.Fatal("fixture unavailable")
				}
				edit(p)
				data = mustTemporalJSON(t, p)
				if os.WriteFile(filepath.Join(dir, ref.Path), data, 0600) != nil {
					t.Fatal("fixture mutation failed")
				}
				ref.SHA256 = experienceDigest(string(data))
			}
			switch name {
			case "native_total", "omitted_product_null", "entry_operation", "altered_carry", "scope_field_omission":
				change(&m.Predecessor, func(p map[string]any) {
					switch name {
					case "native_total":
						p["entries"].([]any)[0].(map[string]any)["observation_json"].(map[string]any)["usage"].(map[string]any)["TotalTokens"] = 1
					case "omitted_product_null":
						delete(p["foreground"].([]any)[0].(map[string]any), "observed_at")
					case "entry_operation":
						p["entries"].([]any)[0].(map[string]any)["observation_json"].(map[string]any)["operation_id"] = "foreign"
					case "altered_carry":
						p["reservations"].([]any)[0].(map[string]any)["request_json"].(map[string]any)["charge"].(map[string]any)["model_turns"] = 0
					case "scope_field_omission":
						delete(p["scopes"].([]any)[0].(map[string]any), "deadline")
					}
				})
			case "entry_file_usage":
				change(&m.Entries[0], func(p map[string]any) { p["usage"].(map[string]any)["TotalTokens"] = 1 })
			case "outcome_denominator":
				m.Outcomes = m.Ledger
			case "missing_seventh", "extra_main_attempt", "extra_seventh_attempt":
				change(&m.Current, func(p map[string]any) {
					dbs := p["databases"].([]any)
					if name == "missing_seventh" {
						p["databases"] = dbs[:6]
						return
					}
					index := 0
					if name == "extra_seventh_attempt" {
						index = 6
					}
					dbs[index].(map[string]any)["windows"].([]any)[0].(map[string]any)["foreground"] = []any{map[string]any{"operation_id": "foreign", "observed_at": nil}}
				})
			case "changed_recursive_history":
				change(&m.Previous, func(p map[string]any) { p["schema"] = "foreign" })
			case "adaptive_native_usage", "adaptive_unknown_omitted":
				change(&m.AllOwners, func(p map[string]any) {
					for _, row := range p["databases"].([]any)[6].(map[string]any)["windows"].([]any)[0].(map[string]any)["native_adaptive"].([]any) {
						e := row.(map[string]any)
						if name == "adaptive_native_usage" && e["native_usage"] != nil {
							e["native_usage"].(map[string]any)["TotalTokens"] = 1
							return
						}
						if name == "adaptive_unknown_omitted" && e["native_usage"] == nil {
							delete(e, "native_usage")
							return
						}
					}
				})
			case "future_schema_exists":
				v := true
				m.Future.Present = &v
			case "future_receipt_consumed":
				v := 1
				m.Future.Count = &v
			case "future_proof_omitted":
				m.Future.Present = nil
			}
			data := mustTemporalJSON(t, m)
			if os.WriteFile(filepath.Join(dir, "history.json"), data, 0600) != nil {
				t.Fatal("mutated manifest unavailable")
			}
			if _, err := loadConsolidationV3History(filepath.Join(dir, "history.json"), experienceDigest(string(data))); err == nil {
				t.Fatal("forged or incomplete successor accepted")
			}
		})
	}
}
