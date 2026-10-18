package app

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

func TestLatestClosedOwnerV8RetainsIndependentProductPopulation(t *testing.T) {
	for _, count := range []int{0, 1} {
		p, l, outcomes, entries, previous, day, deadline := closedTemporalV7Fixture(t, count)
		p["format"] = "butler.consolidation.predecessor88.v4"
		r := p["reservations"].([]any)[0].(map[string]any)
		data := mustTemporalJSON(t, r["request_json"])
		var q map[string]any
		if json.Unmarshal(data, &q) != nil {
			t.Fatal("request unavailable")
		}
		r["operation_id"] = "issue88-v8-current-day-carry:" + previous.Hash
		q["operation_id"] = r["operation_id"]
		r["request_json"] = q
		l["reservations"] = p["reservations"]
		closedTemporalV7AppendAttempt(t, p, l, entries, 2, false)
		closedTemporalV7AppendAttempt(t, p, l, entries, 3, false)
		used, usage, err := closedTemporalLeaf(mustTemporalJSON(t, p), mustTemporalJSON(t, l), mustTemporalJSON(t, outcomes), entries, previous, day, deadline, "butler.consolidation.predecessor88.v4", "issue88-v8-current-day-carry:")
		if err != nil || used.ModelTurns != previous.Carry.ModelTurns+3 || usage.NativeEntries != count || usage.ProductAttempts != 3 || usage.ProductWithoutNativeEntry != 3-count || usage.ProductNULL != 3-count {
			t.Fatal("V8 admission/native counts conflated", used, usage, err)
		}
		if _, _, err = closedTemporalLeaf(mustTemporalJSON(t, p), mustTemporalJSON(t, l), mustTemporalJSON(t, outcomes), entries, previous, day, deadline, "butler.consolidation.predecessor88.v4", "issue88-v7-current-day-carry:"); err == nil {
			t.Fatal("new stage accepted predecessor carry prefix")
		}
	}
}

func TestLatestClosedOwnerV8FrozenHistoryRejectsForgeries(t *testing.T) {
	root := os.Getenv("BUTLER_LATEST_CLOSED_OWNER_HISTORY_TEST_ROOT")
	if root == "" {
		t.Skip("explicit original closed V8 history required")
	}
	raw, err := os.ReadFile(filepath.Join(root, "history.json"))
	if err != nil {
		t.Fatal(err)
	}
	h, err := loadLatestClosedOwnerHistory(filepath.Join(root, "history.json"), experienceDigest(string(raw)))
	if err != nil {
		t.Fatal("actual recursively validated closed V8 unavailable", err)
	}
	if h.Previous.Carry.ModelTurns != 145 || h.Previous.Usage.NativeEntries != 145 || h.Previous.Usage.NativeUnknownUsage != 1 || h.Carry.ModelTurns != 145+int64(h.NewUsage.ProductAttempts) || h.Usage.NativeEntries != 145+h.NewUsage.NativeEntries || h.Usage.NativeUnknownUsage != 1+h.NewUsage.NativeUnknownUsage || h.Usage.KnownFullTokenLowerBound != 1105554+h.NewUsage.KnownFullTokenLowerBound || h.SeparateAdaptive.NativeEntries != 98 || h.SeparateAdaptive.NativeUnknownUsage != 1 {
		t.Fatal("closed V8 duplicated/refunded earlier inclusive scopes or unknowns", h.Carry, h.Usage, h.NewUsage)
	}
	t.Logf("latestclosed summary: %s", mustTemporalJSON(t, map[string]any{"Hash": h.Hash, "Day": h.Day, "Deadline": h.Deadline, "Carry": h.Carry, "Usage": h.Usage, "NewUsage": h.NewUsage, "SeparateAdaptive": h.SeparateAdaptive}))
	for _, name := range []string{"native_usage", "native_file", "product_null_key", "original_outcomes", "closure_unchecked", "missing_main", "missing_seventh", "extra_current_main", "all_owner_native", "all_owner_prior", "operator_digest", "operator_cost", "old_unknown_cause", "recursive_history", "schema_identity", "version_identity", "future_schema", "future_receipt", "future_missing", "entry_omitted"} {
		t.Run(name, func(t *testing.T) {
			dir := temporalV6CopyHistory(t, root)
			var m closedOwnerManifest
			if json.Unmarshal(raw, &m) != nil {
				t.Fatal("original manifest invalid")
			}
			change := func(ref *consolidationHistoryRef, edit func(map[string]any)) {
				data, err := os.ReadFile(filepath.Join(dir, ref.Path))
				var value map[string]any
				if err != nil || json.Unmarshal(data, &value) != nil {
					t.Fatal("original witness unavailable")
				}
				edit(value)
				data = mustTemporalJSON(t, value)
				if os.WriteFile(filepath.Join(dir, ref.Path), data, 0600) != nil {
					t.Fatal("private witness copy failed")
				}
				ref.SHA256 = experienceDigest(string(data))
			}
			switch name {
			case "native_usage":
				change(&m.Predecessor, func(p map[string]any) {
					p["entries"].([]any)[0].(map[string]any)["observation_json"].(map[string]any)["usage"].(map[string]any)["TotalTokens"] = 1
				})
			case "native_file":
				change(&m.Entries[0], func(p map[string]any) { p["usage"].(map[string]any)["TotalTokens"] = 1 })
			case "product_null_key":
				change(&m.Predecessor, func(p map[string]any) { delete(p["foreground"].([]any)[0].(map[string]any), "observed_at") })
			case "original_outcomes":
				m.Outcomes = m.Ledger
			case "closure_unchecked":
				change(&m.Closure, func(p map[string]any) { p["return_safe"].(map[string]any)["active_work_checked"] = false })
			case "missing_main", "missing_seventh", "extra_current_main":
				change(&m.Current, func(p map[string]any) {
					dbs := p["databases"].([]any)
					if name == "missing_main" {
						p["databases"] = dbs[1:]
						return
					}
					if name == "missing_seventh" {
						p["databases"] = dbs[:6]
						return
					}
					dbs[0].(map[string]any)["windows"].([]any)[0].(map[string]any)["foreground"] = []any{map[string]any{"operation_id": "foreign"}}
				})
			case "all_owner_native", "all_owner_prior":
				change(&m.AllOwners, func(p map[string]any) {
					for _, db := range p["databases"].([]any) {
						for _, row := range db.(map[string]any)["windows"].([]any) {
							w := row.(map[string]any)
							if name == "all_owner_native" && w["schema"] == temporalV8Schema {
								w["native_temporal"].([]any)[0].(map[string]any)["usage"].(map[string]any)["TotalTokens"] = 1
								return
							}
							if name == "all_owner_prior" && w["schema"] == temporalV7Schema {
								w["reservations"] = []any{}
								return
							}
						}
					}
					t.Fatal("actual mutation target missing")
				})
			case "operator_digest":
				change(&m.Operator, func(p map[string]any) { p["artifact_hashes"].(map[string]any)["full-ledger.json"] = "foreign" })
			case "operator_cost":
				change(&m.Operator, func(p map[string]any) { p["new_stage_known_full_tokens_lower_bound"] = 0 })
			case "old_unknown_cause":
				change(&m.Operator, func(p map[string]any) { p["old89v2_failure_cause"] = "retrospectively_classified" })
			case "recursive_history":
				change(&m.Previous, func(p map[string]any) { p["schema"] = "foreign" })
			case "schema_identity":
				m.Schema = temporalV7Schema
			case "version_identity":
				m.Version = 9
			case "future_schema":
				v := true
				m.Future.Present = &v
			case "future_receipt":
				v := 1
				m.Future.Count = &v
			case "future_missing":
				m.Future.Count = nil
			case "entry_omitted":
				m.Entries = m.Entries[1:]
			}
			data := mustTemporalJSON(t, m)
			if os.WriteFile(filepath.Join(dir, "history.json"), data, 0600) != nil {
				t.Fatal("manifest copy failed")
			}
			if _, err := loadLatestClosedOwnerHistory(filepath.Join(dir, "history.json"), experienceDigest(string(data))); err == nil {
				t.Fatal("forged latest closed V8 accepted")
			}
		})
	}
}
