package app

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/butler/butler/internal/budget"
)

func closedTemporalV7Fixture(t *testing.T, count int) (map[string]any, map[string]any, []any, map[int64][]byte, *temporalV5History, string, time.Time) {
	t.Helper()
	_, p, l, outcomes, entries, previous, _, day, deadline := consolidationV2Fixture(t)
	p["format"] = "butler.consolidation.predecessor88.v3"
	r := p["reservations"].([]any)[0].(map[string]any)
	r["operation_id"] = "issue88-v7-current-day-carry:" + previous.Hash
	q := r["request_json"].(budget.Request)
	q.OperationID = r["operation_id"].(string)
	r["request_json"] = q
	l["reservations"] = p["reservations"]
	if count == 0 {
		p["window"].(map[string]any)["paid_entries"] = 0
		l["window"] = p["window"]
		p["entries"], l["entries"] = []any{}, []any{}
		entries = map[int64][]byte{}
		for _, row := range []any{p["foreground"].([]any)[0], l["foreground"].([]any)[0]} {
			m := row.(map[string]any)
			m["actual_input_tokens"], m["actual_output_tokens"], m["observed_at"] = nil, nil, nil
		}
		for _, row := range []any{l["cases"].([]any)[0], outcomes[0]} {
			row.(map[string]any)["status"], row.(map[string]any)["native_entries"] = "failed", 0
		}
	}
	return p, l, outcomes, entries, previous, day, deadline
}

func closedTemporalV7AppendAttempt(t *testing.T, p, l map[string]any, entries map[int64][]byte, id int, native bool) {
	t.Helper()
	op, rsv := fmt.Sprintf("extra-op-%d", id), fmt.Sprintf("extra-rsv-%d", id)
	q := budget.Units{ModelTurns: 1, Tokens: 1081344, ActiveMillis: 300000, ProviderInputBytes: 8388608}
	var reservation, product map[string]any
	_ = json.Unmarshal(mustTemporalJSON(t, p["reservations"].([]any)[1]), &reservation)
	_ = json.Unmarshal(mustTemporalJSON(t, p["foreground"].([]any)[0]), &product)
	reservation["reservation_id"], reservation["operation_id"] = rsv, op
	reservation["request_json"].(map[string]any)["operation_id"] = op
	product["operation_id"], product["reservation_id"] = op, rsv
	if !native {
		product["actual_input_tokens"], product["actual_output_tokens"], product["observed_at"] = nil, nil, nil
	}
	p["reservations"] = append(p["reservations"].([]any), reservation)
	p["foreground"] = append(p["foreground"].([]any), product)
	l["reservations"] = p["reservations"]
	var originalProduct map[string]any
	_ = json.Unmarshal(mustTemporalJSON(t, product), &originalProduct)
	originalProduct["run_id"] = "native88-run"
	l["foreground"] = append(l["foreground"].([]any), originalProduct)
	scope := p["scopes"].([]any)[0].(map[string]any)
	scope["used_json"] = consolidationAddUnits(scope["used_json"].(budget.Units), q)
	l["scopes"] = p["scopes"]
	if native {
		var e map[string]any
		_ = json.Unmarshal(mustTemporalJSON(t, p["entries"].([]any)[0]), &e)
		e["id"], e["kind"] = id, "tool_result"
		o := e["observation_json"].(map[string]any)
		o["id"], o["kind"], o["operation_id"], o["reservation_id"] = id, "tool_result", op, rsv
		p["entries"] = append(p["entries"].([]any), e)
		var original map[string]any
		_ = json.Unmarshal(mustTemporalJSON(t, e), &original)
		original["operation_id"], original["reservation_id"] = op, rsv
		l["entries"] = append(l["entries"].([]any), original)
		entries[int64(id)] = mustTemporalJSON(t, o)
		p["window"].(map[string]any)["paid_entries"] = id
		l["window"] = p["window"]
	}
}

func TestClosedTemporalV7RetainsProductNativeIndependence(t *testing.T) {
	for _, count := range []int{0, 1} {
		t.Run(fmt.Sprintf("native%d", count), func(t *testing.T) {
			p, l, outcomes, entries, prev, day, deadline := closedTemporalV7Fixture(t, count)
			closedTemporalV7AppendAttempt(t, p, l, entries, 2, false)
			closedTemporalV7AppendAttempt(t, p, l, entries, 3, false)
			if count != 0 {
				p["entries"].([]any)[0].(map[string]any)["observation_json"].(map[string]any)["usage"] = nil
				l["entries"].([]any)[0].(map[string]any)["observation_json"].(map[string]any)["usage"] = nil
				entries[1] = mustTemporalJSON(t, p["entries"].([]any)[0].(map[string]any)["observation_json"])
			}
			carry, usage, err := closedTemporalV7Leaf(mustTemporalJSON(t, p), mustTemporalJSON(t, l), mustTemporalJSON(t, outcomes), entries, prev, day, deadline)
			if err != nil || carry.ModelTurns != prev.Carry.ModelTurns+3 || usage.ProductAttempts != 3 || usage.NativeEntries != count || usage.NativeUnknownUsage != count || usage.ProductNULL != 3-count || usage.ProductWithoutNativeEntry != 3-count || usage.KnownFullTokenLowerBound != 0 {
				t.Fatal("independent paid admission/native populations lost", carry, usage, err)
			}
			if count == 0 {
				if _, _, err = consolidationValidateTemporalLeaf(mustTemporalJSON(t, p), mustTemporalJSON(t, l), prev, day, deadline, "butler.consolidation.predecessor88.v3", "issue88-v7-current-day-carry:"); err == nil {
					t.Fatal("legacy positive-population restriction silently relaxed")
				}
			}
		})
	}
}

func TestClosedTemporalV7RejectsConsistentInvalidSequence(t *testing.T) {
	for _, kind := range []string{"all_continuations", "thirteen_per_original", "observed_without_native"} {
		t.Run(kind, func(t *testing.T) {
			p, l, outcomes, entries, prev, day, deadline := closedTemporalV7Fixture(t, 1)
			switch kind {
			case "all_continuations":
				for _, e := range []any{p["entries"].([]any)[0], l["entries"].([]any)[0]} {
					e.(map[string]any)["kind"] = "tool_result"
					e.(map[string]any)["observation_json"].(map[string]any)["kind"] = "tool_result"
				}
				entries[1] = mustTemporalJSON(t, p["entries"].([]any)[0].(map[string]any)["observation_json"])
			case "thirteen_per_original":
				for i := 2; i <= 13; i++ {
					closedTemporalV7AppendAttempt(t, p, l, entries, i, true)
				}
				l["cases"].([]any)[0].(map[string]any)["native_entries"] = 13
				outcomes[0].(map[string]any)["native_entries"] = 13
			case "observed_without_native":
				closedTemporalV7AppendAttempt(t, p, l, entries, 2, false)
				for _, row := range []any{p["foreground"].([]any)[1], l["foreground"].([]any)[1]} {
					row.(map[string]any)["observed_at"] = "2026-10-18T02:01:00Z"
				}
			}
			if _, _, err := closedTemporalV7Leaf(mustTemporalJSON(t, p), mustTemporalJSON(t, l), mustTemporalJSON(t, outcomes), entries, prev, day, deadline); err == nil {
				t.Fatal("self-consistent invalid original/native witness accepted")
			}
		})
	}
}

func TestClosedTemporalV7FrozenHistoryRejectsForgeries(t *testing.T) {
	root := os.Getenv("BUTLER_CLOSED_TEMPORAL_V7_HISTORY_TEST_ROOT")
	if root == "" {
		t.Skip("explicit immutable closed V7 fixture required")
	}
	raw, err := os.ReadFile(filepath.Join(root, "history.json"))
	if err != nil {
		t.Fatal(err)
	}
	h, err := loadClosedTemporalV7History(filepath.Join(root, "history.json"), experienceDigest(string(raw)))
	if err != nil {
		t.Fatal("actual complete closed V7 history invalid", err)
	}
	if h.Carry.ModelTurns != 145 || h.Usage.NativeEntries != 145 || h.Usage.NativeKnownUsage != 144 || h.Usage.NativeUnknownUsage != 1 || h.Usage.KnownFullTokenLowerBound != 1105554 || h.NewUsage.NativeEntries != 36 || h.NewUsage.ProductAttempts != 36 || h.SeparateAdaptive.NativeEntries != 98 || h.SeparateAdaptive.NativeUnknownUsage != 1 {
		t.Fatal("actual closed leaf lost exact independent populations", h.Carry, h.Usage, h.NewUsage, h.SeparateAdaptive)
	}
	t.Logf("closed145 summary: %s", mustTemporalJSON(t, map[string]any{"Hash": h.Hash, "Day": h.Day, "Deadline": h.Deadline, "Carry": h.Carry, "Usage": h.Usage, "NewUsage": h.NewUsage, "SeparateAdaptive": h.SeparateAdaptive}))
	for _, name := range []string{"native_usage", "missing_null_key", "entry_witness", "outcome_denominator", "closed_unchecked", "current_missing_main", "current_extra_main", "current_missing_seventh", "all_owner_native", "all_owner_extra", "adaptive_null", "recursive", "future_schema", "future_receipt", "future_missing", "entry_omitted"} {
		t.Run(name, func(t *testing.T) {
			dir := temporalV6CopyHistory(t, root)
			var m closedTemporalV7Manifest
			if json.Unmarshal(raw, &m) != nil {
				t.Fatal("manifest invalid")
			}
			change := func(ref *consolidationHistoryRef, edit func(map[string]any)) {
				data, err := os.ReadFile(filepath.Join(dir, ref.Path))
				var value map[string]any
				if err != nil || json.Unmarshal(data, &value) != nil {
					t.Fatal("original fixture unavailable")
				}
				edit(value)
				data = mustTemporalJSON(t, value)
				if os.WriteFile(filepath.Join(dir, ref.Path), data, 0600) != nil {
					t.Fatal("private fixture update failed")
				}
				ref.SHA256 = experienceDigest(string(data))
			}
			switch name {
			case "native_usage", "missing_null_key":
				change(&m.Predecessor, func(p map[string]any) {
					if name == "native_usage" {
						p["entries"].([]any)[0].(map[string]any)["observation_json"].(map[string]any)["usage"].(map[string]any)["TotalTokens"] = 1
					} else {
						delete(p["foreground"].([]any)[0].(map[string]any), "observed_at")
					}
				})
			case "entry_witness":
				change(&m.Entries[0], func(p map[string]any) { p["usage"].(map[string]any)["TotalTokens"] = 1 })
			case "outcome_denominator":
				m.Outcomes = m.Ledger
			case "closed_unchecked":
				change(&m.Closure, func(p map[string]any) { p["return_safe"].(map[string]any)["active_work_checked"] = false })
			case "current_missing_main", "current_extra_main", "current_missing_seventh":
				change(&m.Current, func(p map[string]any) {
					d := p["databases"].([]any)
					if name == "current_missing_main" {
						p["databases"] = d[1:]
						return
					}
					if name == "current_missing_seventh" {
						p["databases"] = d[:6]
						return
					}
					d[0].(map[string]any)["windows"].([]any)[0].(map[string]any)["foreground"] = []any{map[string]any{"operation_id": "extra"}}
				})
			case "all_owner_native", "all_owner_extra", "adaptive_null":
				change(&m.AllOwners, func(p map[string]any) {
					for _, db := range p["databases"].([]any) {
						for _, row := range db.(map[string]any)["windows"].([]any) {
							w := row.(map[string]any)
							if name == "adaptive_null" && w["schema"] == "adaptive_native_v5" {
								for _, e := range w["native_adaptive"].([]any) {
									if e.(map[string]any)["native_usage"] == nil {
										delete(e.(map[string]any), "native_usage")
										return
									}
								}
							}
							if w["schema"] == temporalV7Schema {
								if name == "all_owner_native" {
									w["native_temporal"].([]any)[0].(map[string]any)["usage"].(map[string]any)["TotalTokens"] = 1
									return
								}
								if name == "all_owner_extra" {
									w["reservations"] = append(w["reservations"].([]any), map[string]any{"reservation_id": "foreign"})
									return
								}
							}
						}
					}
					t.Fatal("actual mutation target missing")
				})
			case "recursive":
				change(&m.Previous, func(p map[string]any) { p["schema"] = "foreign" })
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
				t.Fatal("manifest update failed")
			}
			if _, err := loadClosedTemporalV7History(filepath.Join(dir, "history.json"), experienceDigest(string(data))); err == nil {
				t.Fatal("forged closed owner lineage accepted", strings.TrimSpace(name))
			}
		})
	}
}
