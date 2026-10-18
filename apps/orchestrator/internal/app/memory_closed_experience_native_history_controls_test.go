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
		row := map[string]any{"owner_id": experienceNativeOwner, "operation_id": "op", "reservation_id": "res", "run_id": "run", "observed_at": nil, "actual_input_tokens": nil, "actual_output_tokens": nil}
		reservation := map[string]any{"reservation_id": "res", "operation_id": "op", "request_json": budget.Request{OwnerID: experienceNativeOwner, OperationID: "op", Charge: budget.Units{ModelTurns: 1}}}
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

// Optional immutable real-export check, with no storage/auth/network access.
// An absent fixture is outside this test's assertions, never a reported skip.
func TestClosedExperienceRetainedControlledExports(t *testing.T) {
	root := os.Getenv("BUTLER_CLOSED_EXPERIENCE_CONTROLLED_EXPORTS")
	if root == "" {
		return
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
}
