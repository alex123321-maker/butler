package app

import (
	"bytes"
	"encoding/json"
	"errors"
)

func temporalEqualValue(a, b json.RawMessage) bool {
	if a == nil || b == nil {
		return false
	}
	x, e1 := temporalCanonical(a)
	y, e2 := temporalCanonical(b)
	return e1 == nil && e2 == nil && bytes.Equal(x, y)
}

func temporalObjectProjection(projected, original json.RawMessage, required ...string) bool {
	var p, o map[string]json.RawMessage
	if json.Unmarshal(projected, &p) != nil || json.Unmarshal(original, &o) != nil || p == nil || o == nil {
		return false
	}
	if len(required) > 0 {
		if len(p) != len(required) {
			return false
		}
		for _, key := range required {
			if p[key] == nil {
				return false
			}
		}
	}
	for key, value := range p {
		if !temporalEqualValue(value, o[key]) {
			return false
		}
	}
	return true
}

func temporalWitnessRecords(raw json.RawMessage, key string) (map[string]json.RawMessage, bool) {
	var rows []json.RawMessage
	if json.Unmarshal(raw, &rows) != nil || rows == nil {
		return nil, false
	}
	result := map[string]json.RawMessage{}
	for _, row := range rows {
		var fields map[string]json.RawMessage
		var id string
		if json.Unmarshal(row, &fields) != nil || json.Unmarshal(fields[key], &id) != nil || id == "" || result[id] != nil {
			return nil, false
		}
		result[id] = row
	}
	return result, true
}

// Exact safe field sets exported by the fixed current-day inventory. Nullable
// observations must be present as explicit null, never inferred from omission.
var temporalForegroundWitnessKeys = []string{"operation_id", "reservation_id", "provider", "model", "input_allowance", "output_allowance", "actual_input_tokens", "actual_output_tokens", "created_at", "observed_at"}
var temporalBackgroundWitnessKeys = []string{"operation_id", "job_id", "reservation_id", "provider", "model", "quote", "state", "usage", "created_at", "observed_at"}

func temporalWitnessProjection(projected, original json.RawMessage, key string, required ...string) bool {
	p, ok := temporalWitnessRecords(projected, key)
	if !ok {
		return false
	}
	o, ok := temporalWitnessRecords(original, key)
	if !ok || len(p) != len(o) {
		return false
	}
	for id, row := range p {
		if !temporalObjectProjection(row, o[id], required...) {
			return false
		}
	}
	return true
}

func temporalWitnessExact(projected, original json.RawMessage, key string) bool {
	var p, o []json.RawMessage
	return json.Unmarshal(projected, &p) == nil && json.Unmarshal(original, &o) == nil && temporalSameRecords(p, o, key)
}

func temporalRequiredWitnessRecords(rows []json.RawMessage, required []string) bool {
	if rows == nil {
		return false
	}
	for _, row := range rows {
		if !temporalObjectProjection(row, row, required...) {
			return false
		}
	}
	return true
}

// The newly exported content-free projection must agree with both immutable
// original witnesses, not merely with its own self-consistent counters/hash.
// Product nullable usage and independently captured full native usage remain
// separate; neither copy may silently replace the other.
func temporalCorroborateOriginal89(priorRaw, ledgerRaw, screenRaw []byte, day string) error {
	var p, l, s map[string]json.RawMessage
	if json.Unmarshal(priorRaw, &p) != nil || json.Unmarshal(ledgerRaw, &l) != nil || json.Unmarshal(screenRaw, &s) != nil {
		return errors.New("original predecessor witnesses unavailable")
	}
	var native bool
	var slots []json.RawMessage
	if json.Unmarshal(s["native"], &native) != nil || !native || json.Unmarshal(s["slots"], &slots) != nil || len(slots) != 9 {
		return errors.New("complete native predecessor schedule witness required")
	}
	var originalScopes []json.RawMessage
	if json.Unmarshal(l["scopes"], &originalScopes) != nil || originalScopes == nil {
		return errors.New("original predecessor scopes unavailable")
	}
	selected := []json.RawMessage{}
	for _, raw := range originalScopes {
		var scope struct {
			Kind  string
			Owner string `json:"owner_id"`
			Day   string `json:"subject_id"`
		}
		if json.Unmarshal(raw, &scope) != nil {
			return errors.New("original predecessor scope invalid")
		}
		if scope.Kind == "owner_day" && scope.Owner == temporalNativeOwner && scope.Day == day {
			selected = append(selected, raw)
		}
	}
	scopes, _ := json.Marshal(selected)
	if !temporalWitnessExact(p["scopes"], scopes, "scope_id") || !temporalWitnessExact(p["reservations"], l["reservations"], "reservation_id") || !temporalWitnessProjection(p["foreground"], l["foreground"], "operation_id", temporalForegroundWitnessKeys...) || !temporalWitnessProjection(p["background"], l["model_jobs"], "operation_id", temporalBackgroundWitnessKeys...) || !temporalWitnessProjection(p["model_jobs"], l["model_jobs"], "operation_id", temporalBackgroundWitnessKeys...) {
		return errors.New("product predecessor projection diverges from original ledger")
	}
	calls, ok := temporalWitnessRecords(p["calls"], "entry_id")
	if !ok {
		return errors.New("projected native entries unavailable")
	}
	original, ok := temporalWitnessRecords(l["calls"], "entry_id")
	if !ok || len(calls) != len(original) {
		return errors.New("original native ledger entry population differs")
	}
	screen, ok := temporalWitnessRecords(s["calls"], "EntryID")
	if !ok || len(calls) != len(screen) {
		return errors.New("original native screen entry population differs")
	}
	for id, raw := range calls {
		var call, entry map[string]json.RawMessage
		if json.Unmarshal(raw, &call) != nil || json.Unmarshal(original[id], &entry) != nil || !temporalEqualValue(call["entry_id"], entry["entry_id"]) || !temporalEqualValue(call["operation_id"], entry["operation_id"]) || !temporalObjectProjection(call["observation_json"], entry["observation"], "EntryID", "OperationID", "ReservationID", "Usage", "FinishedAt") || !temporalObjectProjection(call["observation_json"], screen[id], "EntryID", "OperationID", "ReservationID", "Usage", "FinishedAt") {
			return errors.New("native predecessor usage diverges from original witnesses")
		}
	}
	return nil
}
