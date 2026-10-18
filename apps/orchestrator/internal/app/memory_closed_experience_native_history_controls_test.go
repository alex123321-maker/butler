package app

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/butler/butler/internal/budget"
	"github.com/butler/butler/internal/transport"
)

func closedExperienceJSON(value any) json.RawMessage { raw, _ := json.Marshal(value); return raw }

func TestClosedExperienceProductsPreserveUnknownAndRejectForgeries(t *testing.T) {
	fixture := func() (map[string]map[string]json.RawMessage, []experienceNativeEntry, map[string]experienceNativeSlot) {
		now := time.Date(2026, 10, 18, 8, 0, 0, 0, time.UTC)
		millis := int64(3)
		row := map[string]any{"operation_id": "op", "reservation_id": "res", "run_id": "run", "observed_at": nil, "actual_input_tokens": nil, "actual_output_tokens": nil}
		reservation := map[string]any{"reservation_id": "res", "operation_id": "op", "owner_id": experienceNativeOwner, "request_json": budget.Request{OwnerID: experienceNativeOwner, OperationID: "op", Charge: budget.Units{ModelTurns: 1}}}
		db := map[string]json.RawMessage{"execution_model_attempts": closedExperienceJSON([]any{row}), "memory_model_attempts": json.RawMessage(`[]`), "execution_budget_reservations": closedExperienceJSON([]any{reservation})}
		return map[string]map[string]json.RawMessage{"source-database.json": db}, []experienceNativeEntry{{Sequence: 1, Slot: "slot", RunID: "run", Operation: "op", Reservation: "res", Kind: "start", Request: json.RawMessage(`{}`), Finished: true, Millis: &millis}}, map[string]experienceNativeSlot{"slot": {Sequence: 1, ID: "slot", Kind: "original", Maximum: 12, Started: &now}}
	}
	t.Run("charged_unknown_not_zero", func(t *testing.T) {
		db, entries, slots := fixture()
		u, err := closedExperienceProducts(db, entries, slots)
		if err != nil || u.NativeUnknownUsage != 1 || u.ProductNULL != 1 || u.KnownFullTokenLowerBound != 0 {
			t.Fatal(u, err)
		}
	})
	t.Run("admitted_without_native_entry", func(t *testing.T) {
		db, _, slots := fixture()
		u, err := closedExperienceProducts(db, nil, slots)
		if err != nil || u.ProductWithoutNativeEntry != 1 || u.ProductNULL != 1 || u.NativeEntries != 0 {
			t.Fatal(u, err)
		}
	})
	t.Run("incomplete_capture_remains_unknown", func(t *testing.T) {
		db, entries, slots := fixture()
		entries[0].Finished = false
		entries[0].Millis = nil
		u, err := closedExperienceProducts(db, entries, slots)
		if err != nil || u.NativeUnknownUsage != 1 {
			t.Fatal(u, err)
		}
	})
	t.Run("full_total_and_missing_total", func(t *testing.T) {
		db, entries, slots := fixture()
		entries[0].Usage = &transport.Usage{InputTokens: 3, OutputTokens: 2, TotalTokens: 11}
		u, err := closedExperienceProducts(db, entries, slots)
		if err != nil || u.KnownFullTokenLowerBound != 11 {
			t.Fatal(u, err)
		}
		entries[0].Usage.TotalTokens = 0
		u, err = closedExperienceProducts(db, entries, slots)
		if err != nil || u.NativeMissingTotal != 1 || u.KnownFullTokenLowerBound != 0 {
			t.Fatal(u, err)
		}
	})
	t.Run("independent_observed_usage_matches_native", func(t *testing.T) {
		db, entries, slots := fixture()
		entries[0].Usage = &transport.Usage{InputTokens: 3, OutputTokens: 2, TotalTokens: 11}
		var rows []map[string]any
		_ = json.Unmarshal(db["source-database.json"]["execution_model_attempts"], &rows)
		rows[0]["observed_at"] = "2026-10-18T08:00:00Z"
		rows[0]["actual_input_tokens"] = 3
		rows[0]["actual_output_tokens"] = 2
		db["source-database.json"]["execution_model_attempts"] = closedExperienceJSON(rows)
		u, err := closedExperienceProducts(db, entries, slots)
		if err != nil || u.ProductObserved != 1 || u.KnownFullTokenLowerBound != 11 {
			t.Fatal(u, err)
		}
		rows[0]["actual_output_tokens"] = 7
		db["source-database.json"]["execution_model_attempts"] = closedExperienceJSON(rows)
		if _, err = closedExperienceProducts(db, entries, slots); err == nil {
			t.Fatal("foreground product/native usage mismatch accepted")
		}
	})
	for name, mutate := range map[string]func(map[string]map[string]json.RawMessage, []experienceNativeEntry, map[string]experienceNativeSlot){
		"wrong_reservation": func(_ map[string]map[string]json.RawMessage, e []experienceNativeEntry, _ map[string]experienceNativeSlot) {
			e[0].Reservation = "other"
		},
		"wrong_run": func(_ map[string]map[string]json.RawMessage, e []experienceNativeEntry, _ map[string]experienceNativeSlot) {
			e[0].RunID = "other"
		},
		"missing_slot": func(_ map[string]map[string]json.RawMessage, e []experienceNativeEntry, _ map[string]experienceNativeSlot) {
			e[0].Slot = "other"
		},
		"continuation_before_start": func(_ map[string]map[string]json.RawMessage, e []experienceNativeEntry, _ map[string]experienceNativeSlot) {
			e[0].Kind = "continuation"
		},
		"sequence_gap": func(_ map[string]map[string]json.RawMessage, e []experienceNativeEntry, _ map[string]experienceNativeSlot) {
			e[0].Sequence = 2
		},
		"invented_finished_usage": func(_ map[string]map[string]json.RawMessage, e []experienceNativeEntry, _ map[string]experienceNativeSlot) {
			e[0].Finished = false
			e[0].Usage = &transport.Usage{}
		},
		"foreign_clone": func(db map[string]map[string]json.RawMessage, _ []experienceNativeEntry, s map[string]experienceNativeSlot) {
			slot := s["slot"]
			slot.Kind = "control"
			slot.Sequence = 10
			s["slot"] = slot
			db["case-011-database.json"] = db["source-database.json"]
		},
		"omitted_null_key": func(db map[string]map[string]json.RawMessage, _ []experienceNativeEntry, _ map[string]experienceNativeSlot) {
			var rows []map[string]any
			_ = json.Unmarshal(db["source-database.json"]["execution_model_attempts"], &rows)
			delete(rows[0], "actual_input_tokens")
			db["source-database.json"]["execution_model_attempts"] = closedExperienceJSON(rows)
		},
		"changed_copied_prefix": func(db map[string]map[string]json.RawMessage, _ []experienceNativeEntry, _ map[string]experienceNativeSlot) {
			clone := map[string]json.RawMessage{}
			for k, v := range db["source-database.json"] {
				clone[k] = v
			}
			var rows []map[string]any
			_ = json.Unmarshal(clone["execution_model_attempts"], &rows)
			rows[0]["actual_input_tokens"] = 7
			clone["execution_model_attempts"] = closedExperienceJSON(rows)
			db["case-010-database.json"] = clone
		},
	} {
		t.Run(name, func(t *testing.T) {
			db, e, s := fixture()
			mutate(db, e, s)
			if _, err := closedExperienceProducts(db, e, s); err == nil {
				t.Fatal("forged product/native population accepted")
			}
		})
	}
}

func TestClosedExperienceReferencesAndHandbackFailClosed(t *testing.T) {
	root := t.TempDir()
	if os.Mkdir(filepath.Join(root, "actual"), 0700) != nil {
		t.Fatal("mkdir")
	}
	path := filepath.Join(root, "actual", "row.json")
	raw := []byte(`{"actual":true}`)
	if os.WriteFile(path, raw, 0600) != nil {
		t.Fatal("write")
	}
	hash := experienceDigest(string(raw))
	if got, err := closedExperienceRead(root, "actual/row.json", hash, 1024); err != nil || !reflect.DeepEqual(got, raw) {
		t.Fatal(err)
	}
	for _, name := range []string{"../row.json", "actual/../row.json", "actual\\row.json", "/row.json", "C:/row.json"} {
		if _, err := closedExperienceRead(root, name, hash, 1024); err == nil {
			t.Fatal("unsafe path accepted", name)
		}
	}
	if _, err := closedExperienceRead(root, "actual/row.json", strings.Repeat("f", 64), 1024); err == nil {
		t.Fatal("changed hash accepted")
	}
	if _, err := closedExperienceRead(root, "actual/row.json", hash, 1); err == nil {
		t.Fatal("oversize accepted")
	}
	var h closedExperienceHandback
	_ = json.Unmarshal([]byte(`{"native_status":1,"handback_status":0,"return_safe":{"opaque_main_cas":true,"active_work_checked":true,"raw_material_printed":false},"main_auth_safe":{"resolved":true,"unsafe_refresh":false,"expired":false,"trace":[{"stage":"paid_discovery","outcome":"completed","http_status":200}]},"main_snapshot_safe":{"fresh_main_snapshot_saved":true,"ciphertext_sha256":"`+strings.Repeat("a", 64)+`","raw_material_printed":false}}`), &h)
	if h.validate() != nil {
		t.Fatal("failed native cohort with safe completed handback rejected")
	}
	for name, mutate := range map[string]func(*closedExperienceHandback){"discovery_missing": func(h *closedExperienceHandback) { h.Auth.Trace = nil }, "http_not_200": func(h *closedExperienceHandback) {
		h.Auth.Trace = append([]struct {
			Stage, Outcome string
			HTTP           int `json:"http_status"`
		}{}, h.Auth.Trace...)
		h.Auth.Trace[0].HTTP = 403
	}, "snapshot_missing": func(h *closedExperienceHandback) { h.Snapshot.Saved = false }, "cas_missing": func(h *closedExperienceHandback) { h.Return.CAS = false }, "null_not_false": func(h *closedExperienceHandback) { h.Auth.Expired = nil }} {
		t.Run(name, func(t *testing.T) {
			copy := h
			mutate(&copy)
			if copy.validate() == nil {
				t.Fatal("incomplete handback accepted")
			}
		})
	}
}

func TestClosedExperienceFinitePresealRejectsExpandedAuthority(t *testing.T) {
	now := time.Date(2026, 10, 18, 8, 0, 0, 0, time.UTC)
	history := experienceNativeLineage{Hash: strings.Repeat("a", 64), Day: "2026-10-18", Deadline: now.Add(16 * time.Hour), Carry: budget.Units{ModelTurns: 3, Tokens: 3 * 1081344}, Usage: temporalV5CurrentUsage{ProductAttempts: 3, ProductNULL: 3, ProductWithoutNativeEntry: 3}}
	policy, _ := experienceNativeCarryPolicy(false, history.Carry)
	source, _ := experienceNativeCarryPolicy(true, history.Carry)
	deadline, _ := experienceNativeWholeDeadline(now, history.Day, history.Deadline)
	plan := experienceNativeSeal{Created: now, Deadline: deadline, Policy: policy, SourcePolicy: source, FGMillis: experienceNativeForegroundLimit.Milliseconds(), MaximumMillis: experienceNativeWholeMaximum.Milliseconds(), Binaries: map[string]string{}, Environments: map[string]string{}, Source: map[string]string{"sample.go": strings.Repeat("b", 64)}}
	for _, name := range []string{"recipient", "broker", "broker_control", "extension", "http"} {
		plan.Binaries[name] = strings.Repeat("c", 64)
	}
	for _, p := range []struct {
		name, version, ready string
		outage               bool
	}{{"project-v1-normal", "v1", "true", false}, {"project-v1-receipt-outage", "v1", "true", true}, {"project-v2-normal", "v2", "verified", false}} {
		configuration := experienceEnvironment(experienceRecipientConfig{Version: p.version, Ready: p.ready, Outage: p.outage})
		plan.Environments[p.name] = experienceDigest(strings.Join([]string{configuration, plan.Binaries["recipient"], plan.Binaries["broker"], plan.Binaries["extension"], plan.Binaries["http"]}, "\x00"))
	}
	if err := closedExperienceFinitePlan(plan, history); err != nil {
		t.Fatal(err)
	}
	for name, mutate := range map[string]func(*experienceNativeSeal){"day_refund": func(p *experienceNativeSeal) { p.Policy.Day.ModelTurns-- }, "task_increase": func(p *experienceNativeSeal) { p.Policy.Task.ModelTurns++ }, "source_note_partition": func(p *experienceNativeSeal) { p.SourcePolicy.Task.ModelTurns = 12 }, "extended_window": func(p *experienceNativeSeal) { p.Deadline = p.Deadline.Add(time.Second) }, "longer_foreground": func(p *experienceNativeSeal) { p.FGMillis++ }, "swapped_environment": func(p *experienceNativeSeal) { p.Environments = map[string]string{} }, "missing_binary": func(p *experienceNativeSeal) { p.Binaries = map[string]string{} }} {
		t.Run(name, func(t *testing.T) {
			raw, _ := json.Marshal(plan)
			var copy experienceNativeSeal
			_ = json.Unmarshal(raw, &copy)
			mutate(&copy)
			if closedExperienceFinitePlan(copy, history) == nil {
				t.Fatal("changed finite authority accepted")
			}
		})
	}
}

// Optional immutable real-export check, with no storage/auth/network access.
// The dedicated acceptance driver supplies the complete immutable fixture and
// rejects skips; ordinary local unit runs must not count its absence as a pass.
func TestClosedExperienceRetainedControlledExports(t *testing.T) {
	root := os.Getenv("BUTLER_CLOSED_EXPERIENCE_CONTROLLED_EXPORTS")
	if root == "" {
		t.Skip("explicit complete retained controlled exports required")
	}
	var window struct {
		Entries []experienceNativeEntry
		Slots   []experienceNativeSlot
	}
	raw, err := os.ReadFile(filepath.Join(root, "window-final.json"))
	if err != nil || json.Unmarshal(raw, &window) != nil {
		t.Fatal("exact controlled fixture absent")
	}
	slots := map[string]experienceNativeSlot{}
	for _, s := range window.Slots {
		slots[s.ID] = s
	}
	databases := map[string]map[string]json.RawMessage{}
	for i := 9; i <= 57; i++ {
		name := "source-database.json"
		if i >= 10 {
			name = fmt.Sprintf("case-%03d-database.json", i)
		}
		raw, err := os.ReadFile(filepath.Join(root, name))
		var db map[string]json.RawMessage
		if err != nil || json.Unmarshal(raw, &db) != nil {
			t.Fatal("full allocated fixture required")
		}
		databases[name] = db
	}
	u, err := closedExperienceProducts(databases, window.Entries, slots)
	if err != nil || u.ProductAttempts != 211 || u.NativeEntries != 211 || u.ProductObserved != 211 || u.ProductNULL != 0 {
		t.Fatal(u, err)
	}
	var history struct{ History experienceNativeLineage }
	raw, err = os.ReadFile(filepath.Join(root, "history-import.json"))
	if err != nil || json.Unmarshal(raw, &history) != nil {
		t.Fatal("original controlled carry absent")
	}
	list := []map[string]json.RawMessage{}
	for _, db := range databases {
		list = append(list, db)
	}
	quotes, err := experienceNativeUniqueQuotes(list, history.History)
	if err != nil || quotes.ModelRows != 211 || quotes.CarryRows != 1 || quotes.UniqueReservations != 368 {
		t.Fatal(quotes, err)
	}
	var journal map[string]json.RawMessage
	raw, err = os.ReadFile(filepath.Join(root, "global-journal-final.json"))
	if err != nil || json.Unmarshal(raw, &journal) != nil {
		t.Fatal("original controlled global journal absent")
	}
	var windows []struct {
		Window string `json:"window_id"`
	}
	if json.Unmarshal(journal["experience_native_windows"], &windows) != nil || len(windows) != 1 {
		t.Fatal("one controlled window required")
	}
	m := closedExperienceManifest{Preseal: windows[0].Window, Owner: experienceNativeOwner, Previous: history.History.Hash}
	if err = closedExperienceJournal(journal, slots, window.Entries, databases, m, history.History, quotes); err != nil {
		t.Fatal("actual controlled journal", err)
	}
	for name, mutate := range map[string]func(map[string]json.RawMessage){
		"refund_global_used": func(j map[string]json.RawMessage) {
			var rows []map[string]any
			_ = json.Unmarshal(j["experience_native_windows"], &rows)
			rows[0]["used"] = 210
			j["experience_native_windows"] = closedExperienceJSON(rows)
		},
		"omit_admission": func(j map[string]json.RawMessage) {
			var rows []map[string]any
			_ = json.Unmarshal(j["experience_native_entries"], &rows)
			j["experience_native_entries"] = closedExperienceJSON(rows[1:])
		},
		"lose_history_binding": func(j map[string]json.RawMessage) { j["experience_native_history"] = json.RawMessage(`[]`) },
		"omit_allocated_clone": func(j map[string]json.RawMessage) {
			var rows []map[string]any
			_ = json.Unmarshal(j["experience_native_case_databases"], &rows)
			j["experience_native_case_databases"] = closedExperienceJSON(rows[1:])
		},
		"change_recorded_usage": func(j map[string]json.RawMessage) {
			var rows []map[string]any
			_ = json.Unmarshal(j["experience_native_entries"], &rows)
			rows[0]["usage_json"] = nil
			j["experience_native_entries"] = closedExperienceJSON(rows)
		},
	} {
		t.Run(name, func(t *testing.T) {
			copy := map[string]json.RawMessage{}
			for k, v := range journal {
				copy[k] = v
			}
			mutate(copy)
			if closedExperienceJournal(copy, slots, window.Entries, databases, m, history.History, quotes) == nil {
				t.Fatal("changed retained journal accepted")
			}
		})
	}
}
