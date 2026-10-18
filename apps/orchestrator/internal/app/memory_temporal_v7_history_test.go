package app

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"time"

	"github.com/butler/butler/internal/budget"
	"github.com/butler/butler/internal/transport"
)

const temporalV7Schema = "memory_temporal_native_v7"
const temporalV7Window = "temporal-explicit-v7"

type temporalV7Manifest struct {
	Format, Owner, Day, Schema string
	Deadline                   time.Time
	Previous                   consolidationHistoryRef `json:"previous89_history"`
	Current                    consolidationHistoryRef `json:"current"`
	AllOwners                  consolidationHistoryRef `json:"all_owners"`
	Predecessor                consolidationHistoryRef `json:"predecessor89"`
	Ledger                     consolidationHistoryRef `json:"original89_ledger"`
	Screen                     consolidationHistoryRef `json:"original89_screen"`
	Schedule                   consolidationHistoryRef `json:"original89_schedule"`
	Closure                    consolidationHistoryRef `json:"predecessor89_closure"`
	Future                     struct {
		Schema  string `json:"schema"`
		Window  string `json:"window_id"`
		Present *bool  `json:"schema_present"`
		Table   *bool  `json:"receipt_table_present"`
		Count   *int   `json:"receipt_count"`
	} `json:"future_window"`
}

type temporalV7History struct {
	Hash             string
	Manifest         temporalV7Manifest
	Previous         *consolidationScreenHistory
	Carry            budget.Units
	Usage89, Usage87 temporalV5CurrentUsage
}

// Original SQL aggregates may be explicitly NULL for an empty population. Only
// that present empty aggregate is normalized in a private validation copy. Row
// observations, absent keys, frozen original bytes and all charges are retained.
func temporalEmptyOriginalAggregates(raw []byte, keys ...string) []byte {
	var fields map[string]json.RawMessage
	if json.Unmarshal(raw, &fields) != nil {
		return raw
	}
	for _, key := range keys {
		if string(fields[key]) == "null" {
			fields[key] = json.RawMessage(`[]`)
		}
	}
	out, err := json.Marshal(fields)
	if err != nil {
		return raw
	}
	return out
}

// A new cohort's native count is independent of its charged product count.
// Failed/unexecuted originals remain in the denominator; missing provider entry
// is not a refund and cannot replace an admitted NULL product observation.
func temporalV7Leaf(priorRaw, ledgerRaw, screenRaw, scheduleRaw []byte, previous *consolidationScreenHistory, day string, deadline time.Time) (budget.Units, temporalV5CurrentUsage, error) {
	if previous == nil || previous.Successor == nil || previous.Successor.Schema != consolidationV3Schema {
		return budget.Units{}, temporalV5CurrentUsage{}, errors.New("validated consolidation v3 recursive lineage required")
	}
	ledger := temporalEmptyOriginalAggregates(ledgerRaw, "calls", "foreground", "model_jobs")
	screen := temporalEmptyOriginalAggregates(screenRaw, "calls")
	used, usage, err := temporalValidateConsolidationLeaf(priorRaw, ledger, screen, day, deadline, temporalConsolidationRecipe{Format: "butler.temporal.predecessor89.v3", CarryPrefix: "issue89-v3-current-day-carry:", Namespace: "native89v3", Hash: previous.Hash, Carry: previous.CurrentCarry, Max: consolidationScreenPolicy("antigravity", "gemini-3.8-flash-high", previous.CurrentCarry).Day, ExpectedPaid: -1})
	if err != nil {
		return used, usage, err
	}
	if err = temporalV7CorroborateSlotRuns(priorRaw, ledger, screen, scheduleRaw); err != nil {
		return used, usage, err
	}
	return used, usage, nil
}

func temporalV7CorroborateSlotRuns(priorRaw, ledgerRaw, screenRaw, scheduleRaw []byte) error {
	var prior struct {
		Slots []struct {
			ID, State  string
			Definition json.RawMessage
		} `json:"slots"`
		Calls []struct {
			ID          string `json:"entry_id"`
			Operation   string `json:"operation_id"`
			Reservation string `json:"reservation_id"`
			Run         string `json:"run_id"`
			Slot        string `json:"slot_id"`
			Kind        string `json:"kind"`
		} `json:"calls"`
	}
	var ledger struct {
		Calls []struct {
			ID          string                  `json:"entry_id"`
			Operation   string                  `json:"operation_id"`
			Run         string                  `json:"run_id"`
			Slot        string                  `json:"slot_id"`
			Kind        string                  `json:"kind"`
			Observation consolidationScreenCall `json:"observation"`
		} `json:"calls"`
		Foreground []struct {
			Operation   string     `json:"operation_id"`
			Run         string     `json:"run_id"`
			Reservation string     `json:"reservation_id"`
			Observed    *time.Time `json:"observed_at"`
		} `json:"foreground"`
		Background []struct {
			Operation   string           `json:"operation_id"`
			Job         string           `json:"job_id"`
			Reservation string           `json:"reservation_id"`
			Usage       *transport.Usage `json:"usage"`
		} `json:"model_jobs"`
	}
	var screen struct {
		Slots []consolidationScreenSlot
		Calls []consolidationScreenCall
	}
	var schedule struct {
		Format  string
		Maximum int `json:"maximum_actual_entries"`
		Slots   []consolidationScreenSlot
	}
	if json.Unmarshal(priorRaw, &prior) != nil || json.Unmarshal(ledgerRaw, &ledger) != nil || json.Unmarshal(screenRaw, &screen) != nil || json.Unmarshal(scheduleRaw, &schedule) != nil || schedule.Format != "consolidation-nine-logical-slots-v1" || schedule.Maximum != 99 || len(schedule.Slots) != 9 || len(prior.Slots) != 9 || len(screen.Slots) != 9 || len(prior.Calls) != len(ledger.Calls) || len(prior.Calls) != len(screen.Calls) {
		return errors.New("complete original nine-slot schedule and native run witnesses required")
	}
	bySlot := map[string]consolidationScreenSlot{}
	closedPrefix := false
	for i, declared := range schedule.Slots {
		actual := screen.Slots[i]
		kind, arm, limit := "foreground", "baseline", 16
		if i%3 == 0 {
			kind, arm, limit = "background", "preparation", 1
		} else if i%3 == 2 {
			arm = "consolidated"
		}
		if declared.ID == "" || bySlot[declared.ID].ID != "" || declared.CaseID == "" || declared.Kind != kind || declared.Arm != arm || declared.Limit != limit || declared.State != "not_executed" || declared.RunID != "" || declared.Session != "" || declared.Answer != "" || !declared.StartedAt.IsZero() || !declared.FinishedAt.IsZero() || actual.ID != declared.ID || actual.CaseID != declared.CaseID || actual.Kind != kind || actual.Arm != arm || actual.Limit != limit {
			return errors.New("original slot identity/order differs from preauth schedule")
		}
		if i%3 > 0 && declared.CaseID != schedule.Slots[i-1].CaseID {
			return errors.New("original pair identity differs")
		}
		if actual.State != "completed" && actual.State != "failed" && actual.State != "not_executed" {
			return errors.New("closed history contains running original")
		}
		if closedPrefix && actual.State != "not_executed" {
			return errors.New("original cohort selectively resumed")
		}
		if actual.State != "completed" {
			closedPrefix = true
		}
		if actual.State == "not_executed" && (actual.RunID != "" || actual.Session != "" || actual.Answer != "" || !actual.StartedAt.IsZero() || !actual.FinishedAt.IsZero()) {
			return errors.New("unexecuted original has execution artifacts")
		}
		if actual.State != "not_executed" && (actual.StartedAt.IsZero() || actual.FinishedAt.IsZero()) {
			return errors.New("executed original lacks final timing")
		}
		if actual.FinishedAt.Before(actual.StartedAt) {
			return errors.New("original slot timing inverted")
		}
		p := prior.Slots[i]
		// Safe definition contains exactly these fields. Answer and current task
		// bodies stay in the immutable screen and never enter inventory metadata.
		projected, _ := json.Marshal(actual)
		if p.ID != actual.ID || p.State != actual.State || !temporalObjectProjection(p.Definition, projected, "ID", "CaseID", "Kind", "Arm", "State", "Session", "RunID", "Limit", "StartedAt", "FinishedAt") {
			return errors.New("live original slot differs from immutable outcome")
		}
		bySlot[actual.ID] = actual
	}
	products := map[string]string{}
	observed := map[string]bool{}
	backgroundJobs := map[string]string{}
	for _, a := range ledger.Foreground {
		if a.Operation == "" || a.Run == "" || a.Reservation == "" || products[a.Operation] != "" {
			return errors.New("foreground original identity absent")
		}
		products[a.Operation] = a.Run
		observed[a.Operation] = a.Observed != nil
	}
	for _, a := range ledger.Background {
		if a.Operation == "" || a.Job == "" || a.Reservation == "" || products[a.Operation] != "" {
			return errors.New("background original identity absent")
		}
		products[a.Operation] = a.Operation
		backgroundJobs[a.Operation] = a.Job
		observed[a.Operation] = a.Usage != nil
	}
	native := map[string]bool{}
	slotCounts, runStarts := map[string]int{}, map[string]string{}
	for index, p := range prior.Calls {
		if ledger.Calls[index].ID != p.ID || screen.Calls[index].EntryID != p.ID {
			return errors.New("native order differs from original witnesses")
		}
		var l *struct {
			ID          string                  `json:"entry_id"`
			Operation   string                  `json:"operation_id"`
			Run         string                  `json:"run_id"`
			Slot        string                  `json:"slot_id"`
			Kind        string                  `json:"kind"`
			Observation consolidationScreenCall `json:"observation"`
		}
		for j := range ledger.Calls {
			if ledger.Calls[j].ID == p.ID {
				l = &ledger.Calls[j]
			}
		}
		var s *consolidationScreenCall
		for j := range screen.Calls {
			if screen.Calls[j].EntryID == p.ID {
				s = &screen.Calls[j]
			}
		}
		if l == nil || s == nil || native[p.Operation] || products[p.Operation] != p.Run || p.Run == "" || p.Slot == "" || (p.Kind != "start" && p.Kind != "tool_result") || p.Operation != l.Operation || p.Run != l.Run || p.Slot != l.Slot || p.Kind != l.Kind || p.Run != s.RunID || p.Slot != s.SlotID || p.Kind != s.Kind || p.Reservation != s.ReservationID || l.Observation.RunID != p.Run || l.Observation.SlotID != p.Slot || l.Observation.Kind != p.Kind {
			return errors.New("native entry run/slot differs from both original witnesses")
		}
		slot := bySlot[p.Slot]
		if slot.ID == "" || slot.State == "not_executed" || (slot.RunID != "" && slot.RunID != p.Run) {
			return errors.New("native entry belongs to absent or unexecuted original")
		}
		slotCounts[p.Slot]++
		if slotCounts[p.Slot] > slot.Limit {
			return errors.New("native original exceeded its declared entry ceiling")
		}
		if slot.Kind == "background" {
			if p.Kind != "start" || products[p.Operation] != p.Operation || backgroundJobs[p.Operation] == "" {
				return errors.New("background native run identity differs")
			}
		}
		if p.Kind == "start" {
			if runStarts[p.Run] != "" {
				return errors.New("native run started more than once")
			}
			var req transport.StartRunRequest
			if json.Unmarshal(s.Request, &req) != nil || req.Context.RunID != p.Run || (slot.Kind == "foreground" && (req.Context.SessionKey != slot.Session || slot.Session == "")) || (slot.Kind == "background" && req.Context.SessionKey != "memory:"+backgroundJobs[p.Operation]) {
				return errors.New("native start lacks original session/run binding")
			}
			runStarts[p.Run] = p.Slot
		} else {
			var req transport.SubmitToolResultRequest
			if runStarts[p.Run] != p.Slot || json.Unmarshal(s.Request, &req) != nil || req.RunID != p.Run || req.ToolCallRef == "" {
				return errors.New("native continuation lacks its preceding owning Start")
			}
		}
		native[p.Operation] = true
	}
	for id, slot := range bySlot {
		if slot.State == "completed" && slotCounts[id] == 0 {
			return errors.New("completed original lacks native execution")
		}
	}
	for op, known := range observed {
		if !native[op] && known {
			return errors.New("product without native entry lost its NULL observation")
		}
	}
	return nil
}

func loadTemporalV7History(path, expected string) (*temporalV7History, error) {
	raw, err := os.ReadFile(path)
	h := &temporalV7History{Hash: expected}
	if err != nil || len(raw) > 32*1024*1024 || len(expected) != 64 || experienceDigest(string(raw)) != expected || json.Unmarshal(raw, &h.Manifest) != nil || h.Manifest.Format != "butler.temporal.native-history.v3" || h.Manifest.Owner != "owner" || h.Manifest.Schema != temporalV7Schema {
		return nil, errors.New("complete frozen temporal v7 history required")
	}
	m := h.Manifest
	if m.Future.Schema != m.Schema || m.Future.Window != temporalV7Window || m.Future.Present == nil || *m.Future.Present || m.Future.Table == nil || m.Future.Count == nil || *m.Future.Count != 0 {
		return nil, errors.New("fresh unconsumed v7 namespace/receipt required")
	}
	root := filepath.Dir(path)
	read := func(ref consolidationHistoryRef) ([]byte, error) { return consolidationV2Read(root, ref) }
	if _, err = read(m.Previous); err != nil {
		return nil, err
	}
	h.Previous, err = loadConsolidationV3History(filepath.Join(root, m.Previous.Path), m.Previous.SHA256)
	if err != nil || h.Previous.ProspectiveDay != m.Day || !h.Previous.ProspectiveDeadline.Equal(m.Deadline) {
		return nil, errors.New("same-day original consolidation v3 lineage required")
	}
	prior, err := read(m.Predecessor)
	if err != nil {
		return nil, err
	}
	ledger, err := read(m.Ledger)
	if err != nil {
		return nil, err
	}
	screen, err := read(m.Screen)
	if err != nil {
		return nil, err
	}
	schedule, err := read(m.Schedule)
	if err != nil {
		return nil, err
	}
	h.Carry, h.Usage89, err = temporalV7Leaf(prior, ledger, screen, schedule, h.Previous, m.Day, m.Deadline)
	if err != nil {
		return nil, err
	}
	closure, err := read(m.Closure)
	if err != nil {
		return nil, err
	}
	if err = temporalV7ValidateClosure(closure, screen); err != nil {
		return nil, err
	}
	current, err := read(m.Current)
	if err != nil {
		return nil, err
	}
	cleaned, err := temporalV7ClearLeaf(current, prior)
	if err != nil {
		return nil, err
	}
	previousRaw, err := read(m.Previous)
	if err != nil {
		return nil, err
	}
	var previousManifest consolidationV3Manifest
	if json.Unmarshal(previousRaw, &previousManifest) != nil {
		return nil, errors.New("original consolidation v3 manifest unavailable")
	}
	old88, err := read(previousManifest.Predecessor)
	if err != nil {
		return nil, err
	}
	oldHistory, err := loadTemporalV5History(filepath.Join(root, previousManifest.Previous.Path), previousManifest.Previous.SHA256)
	if err != nil || oldHistory.SuccessorV6 == nil {
		return nil, errors.New("original temporal v6 lineage unavailable")
	}
	v6 := oldHistory.SuccessorV6
	if err = consolidationV3ValidateInventory(cleaned, old88, v6, root); err != nil {
		return nil, err
	}
	all, err := read(m.AllOwners)
	if err != nil {
		return nil, err
	}
	original87, err := read(v6.Manifest.Ledger87)
	if err != nil {
		return nil, err
	}
	h.Usage87, err = temporalV6AdaptiveWitness(all, original87, m.Day)
	if err != nil || h.Usage87 != v6.Usage87 {
		return nil, errors.New("separate adaptive original population changed")
	}
	return h, nil
}

func temporalV7ValidateClosure(raw, screenRaw []byte) error {
	var screen struct{ Slots []consolidationScreenSlot }
	if json.Unmarshal(screenRaw, &screen) != nil || len(screen.Slots) != 9 {
		return errors.New("complete closed original outcome denominator required")
	}
	completed := 0
	for _, s := range screen.Slots {
		if s.State == "completed" {
			completed++
		}
	}
	return nativeValidateClosedHandback(raw, completed, 9)
}

func temporalV7ClearLeaf(raw, priorRaw []byte) ([]byte, error) {
	return temporalClearCurrentLeaf(raw, priorRaw, "issue86-20261016-pg", "memory_read_test", consolidationV3Schema)
}

func temporalClearCurrentLeaf(raw, priorRaw []byte, targetContainer, targetDatabase, targetSchema string) ([]byte, error) {
	var current map[string]json.RawMessage
	var dbs []map[string]json.RawMessage
	var prior map[string]json.RawMessage
	if json.Unmarshal(raw, &current) != nil || json.Unmarshal(current["databases"], &dbs) != nil || len(dbs) != 7 || json.Unmarshal(priorRaw, &prior) != nil {
		return nil, errors.New("complete current seven-store inventory required")
	}
	found := 0
	latestOperations := map[string]bool{}
	for _, field := range []string{"foreground", "background"} {
		rows, err := consolidationV2Records(prior[field])
		if err != nil {
			return nil, err
		}
		for _, raw := range rows {
			var row struct {
				Operation string `json:"operation_id"`
			}
			if json.Unmarshal(raw, &row) != nil || row.Operation == "" || latestOperations[row.Operation] {
				return nil, errors.New("latest product identity duplicated")
			}
			latestOperations[row.Operation] = true
		}
	}
	for _, db := range dbs {
		var container, name string
		var windows []map[string]json.RawMessage
		if json.Unmarshal(db["container"], &container) != nil || json.Unmarshal(db["database"], &name) != nil || json.Unmarshal(db["windows"], &windows) != nil {
			return nil, errors.New("current store identity required")
		}
		for _, window := range windows {
			var schema string
			_ = json.Unmarshal(window["schema"], &schema)
			if container != targetContainer || name != targetDatabase || schema != targetSchema {
				for _, field := range []string{"foreground", "background"} {
					rows, err := consolidationV2Records(window[field])
					if err != nil {
						return nil, err
					}
					for _, raw := range rows {
						var row struct {
							Operation string `json:"operation_id"`
						}
						if json.Unmarshal(raw, &row) != nil || latestOperations[row.Operation] {
							return nil, errors.New("new cohort reused an earlier admitted operation")
						}
					}
				}
				continue
			}
			found++
			for field, key := range map[string]string{"scopes": "scope_id", "reservations": "reservation_id", "foreground": "operation_id", "background": "operation_id"} {
				a, e1 := consolidationV2Records(window[field])
				b, e2 := consolidationV2Records(prior[field])
				if e1 != nil || e2 != nil || !temporalSameRecords(a, b, key) {
					return nil, errors.New("latest consolidation v3 leaf differs from current inventory")
				}
				window[field] = json.RawMessage(`[]`)
			}
		}
		db["windows"], _ = json.Marshal(windows)
	}
	if found != 1 {
		return nil, errors.New("one exact latest consolidation v3 leaf required")
	}
	current["databases"], _ = json.Marshal(dbs)
	return json.Marshal(current)
}
