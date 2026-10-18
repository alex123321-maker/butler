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

// Prospective lab recipe only. The complete latest owner leaf is imported once;
// the independently authenticated adaptive owner is visible but not rebilled.
type temporalV6Manifest struct {
	Format, Owner, Day, Schema string
	Deadline                   time.Time
	Previous                   consolidationHistoryRef   `json:"previous89_history"`
	Current                    consolidationHistoryRef   `json:"current"`
	AllOwners                  consolidationHistoryRef   `json:"all_owners"`
	Predecessor                consolidationHistoryRef   `json:"predecessor89"`
	Ledger89                   consolidationHistoryRef   `json:"original89_ledger"`
	Screen89                   consolidationHistoryRef   `json:"original89_screen"`
	Ledger87                   consolidationHistoryRef   `json:"original87_ledger"`
	History87                  consolidationHistoryRef   `json:"original87_history"`
	Inventory87                consolidationHistoryRef   `json:"original87_inventory"`
	OldLedger87                consolidationHistoryRef   `json:"original87_old_ledger"`
	OldNative87                consolidationHistoryRef   `json:"original87_old_native"`
	Predecessors87             []consolidationHistoryRef `json:"original87_predecessors"`
	Future                     struct {
		Schema  string `json:"schema"`
		Window  string `json:"window_id"`
		Present *bool  `json:"schema_present"`
		Table   *bool  `json:"receipt_table_present"`
		Count   *int   `json:"receipt_count"`
	} `json:"future_window"`
}

type temporalV6History struct {
	Hash     string
	Manifest temporalV6Manifest
	Previous *consolidationScreenHistory
	Carry    budget.Units
	Usage89  temporalV5CurrentUsage
	Usage87  temporalV5CurrentUsage
}

func temporalV6Leaf(priorRaw, ledgerRaw, screenRaw []byte, previous *consolidationScreenHistory, day string, deadline time.Time) (budget.Units, temporalV5CurrentUsage, error) {
	var prior temporalV5Predecessor
	var usage temporalV5CurrentUsage
	if previous == nil || previous.Successor == nil || json.Unmarshal(priorRaw, &prior) != nil || prior.Format != "butler.temporal.predecessor89.v2" || !prior.Closed || prior.Paid != 11 || len(prior.Calls) != 11 {
		return budget.Units{}, usage, errors.New("closed complete latest consolidation leaf required")
	}
	// Corroboration is against both original immutable witnesses, not a second
	// newly exported copy. The fixed v2 format is not accepted by the v1 loader.
	if err := temporalCorroborateOriginal89(priorRaw, ledgerRaw, screenRaw, day); err != nil {
		return budget.Units{}, usage, err
	}
	max := consolidationScreenPolicy("antigravity", "gemini-3.8-flash-high", previous.CurrentCarry).Day
	used, actualDeadline, err := consolidationValidateHistoryWindow(prior.consolidationHistoryWindow, day, map[string]string{}, "native89v2")
	if err != nil || !actualDeadline.Equal(deadline) || !consolidationV2Within(used, max) {
		return used, usage, errors.New("latest inclusive scope differs from original reservations")
	}
	operations := map[string]string{}
	modelQuotes := map[string]bool{}
	carryCount := 0
	for _, r := range prior.Reservations {
		if r.ID == "" || operations[r.Operation] != "" || !consolidationV2Within(r.Request.Charge, max) {
			return used, usage, errors.New("duplicate or invalid latest leaf quote")
		}
		operations[r.Operation] = r.ID
		if r.Operation == "issue89-v2-current-day-carry:"+previous.Hash {
			carryCount++
			if r.Request.Charge != previous.CurrentCarry {
				return used, usage, errors.New("altered inclusive predecessor charge")
			}
		} else if r.Request.Charge.ModelTurns == 1 {
			modelQuotes[r.Operation] = true
		} else if r.Request.Charge.ModelTurns != 0 {
			return used, usage, errors.New("unlabelled inclusive quote")
		}
	}
	if carryCount != 1 || len(modelQuotes) != 11 || used.ModelTurns != previous.CurrentCarry.ModelTurns+11 {
		return used, usage, errors.New("latest model population omitted or counted twice")
	}
	// Original raw ledger rows include run IDs. Use its exact product population
	// for correlations; the projection retains only the fixed nullable fields.
	var original struct {
		Foreground []struct {
			Operation   string     `json:"operation_id"`
			Reservation string     `json:"reservation_id"`
			ObservedAt  *time.Time `json:"observed_at"`
		} `json:"foreground"`
		Background []struct {
			Operation   string           `json:"operation_id"`
			Reservation string           `json:"reservation_id"`
			Usage       *transport.Usage `json:"usage"`
		} `json:"model_jobs"`
	}
	if json.Unmarshal(ledgerRaw, &original) != nil || len(original.Foreground)+len(original.Background) != len(modelQuotes) {
		return used, usage, errors.New("complete latest product population required")
	}
	seenProducts := map[string]bool{}
	for _, a := range original.Foreground {
		if !modelQuotes[a.Operation] || operations[a.Operation] != a.Reservation || seenProducts[a.Operation] {
			return used, usage, errors.New("foreground latest admission differs")
		}
		seenProducts[a.Operation] = true
		usage.ProductAttempts++
		if a.ObservedAt == nil {
			usage.ProductNULL++
		} else {
			usage.ProductObserved++
		}
	}
	for _, a := range original.Background {
		if !modelQuotes[a.Operation] || operations[a.Operation] != a.Reservation || seenProducts[a.Operation] || a.Usage != nil && a.Usage.Validate() != nil {
			return used, usage, errors.New("background latest admission differs")
		}
		seenProducts[a.Operation] = true
		usage.ProductAttempts++
		if a.Usage == nil {
			usage.ProductNULL++
		} else {
			usage.ProductObserved++
		}
	}
	seenEntries, seenOperations := map[string]bool{}, map[string]bool{}
	for _, c := range prior.Calls {
		o := c.Observation
		if c.ID == "" || seenEntries[c.ID] || seenOperations[c.Operation] || !seenProducts[c.Operation] || operations[c.Operation] != c.Reservation || o.EntryID != c.ID || o.OperationID != c.Operation || o.ReservationID != c.Reservation || o.FinishedAt.IsZero() {
			return used, usage, errors.New("latest native entry lacks exact admitted operation")
		}
		seenEntries[c.ID], seenOperations[c.Operation] = true, true
		usage.NativeEntries++
		if err = temporalV6AddNativeUsage(&usage, o.Usage); err != nil {
			return used, usage, err
		}
	}
	usage.ProductWithoutNativeEntry = usage.ProductAttempts - usage.NativeEntries
	if usage.ProductWithoutNativeEntry != 0 {
		return used, usage, errors.New("latest native/product population differs")
	}
	return used, usage, nil
}

func temporalV6AddNativeUsage(out *temporalV5CurrentUsage, u *transport.Usage) error {
	if u == nil {
		out.NativeUnknownUsage++
		return nil
	}
	if u.Validate() != nil {
		return errors.New("invalid retained full native usage")
	}
	out.NativeKnownUsage++
	if u.TotalTokens == 0 {
		out.NativeMissingTotal++
	} else if int64(u.TotalTokens) > int64(^uint64(0)>>1)-out.KnownFullTokenLowerBound {
		return errors.New("retained native total overflow")
	} else {
		out.KnownFullTokenLowerBound += int64(u.TotalTokens)
	}
	return nil
}

// Clear only an already corroborated population from a deep copy. It creates no
// permission; all remaining records still face the previous recipe's validator.
func temporalV6ClearLeaf(raw, priorRaw []byte) ([]byte, error) {
	var current map[string]json.RawMessage
	var dbs []map[string]json.RawMessage
	var prior map[string]json.RawMessage
	if json.Unmarshal(raw, &current) != nil || json.Unmarshal(current["databases"], &dbs) != nil || json.Unmarshal(priorRaw, &prior) != nil || len(dbs) != 7 {
		return nil, errors.New("complete seven-store current inventory required")
	}
	removed, seventh := 0, 0
	kept := []map[string]json.RawMessage{}
	for _, db := range dbs {
		var container, name string
		var port int
		var schemas []string
		var windows []map[string]json.RawMessage
		if json.Unmarshal(db["container"], &container) != nil || json.Unmarshal(db["database"], &name) != nil || json.Unmarshal(db["port"], &port) != nil || json.Unmarshal(db["schemas"], &schemas) != nil || json.Unmarshal(db["windows"], &windows) != nil {
			return nil, errors.New("current store envelope differs")
		}
		if container == "butler-issue87-native-v5-pg" {
			seventh++
			if name != "memory_adaptive_test" || port != 5432 || len(schemas) != 1 || schemas[0] != "adaptive_native_v5" || len(windows) != 1 {
				return nil, errors.New("exact retained seventh store required")
			}
			var schema string
			var fg, bg *bool
			_ = json.Unmarshal(windows[0]["schema"], &schema)
			_ = json.Unmarshal(windows[0]["foreground_table_present"], &fg)
			_ = json.Unmarshal(windows[0]["background_table_present"], &bg)
			if schema != schemas[0] || fg == nil || !*fg || bg == nil || !*bg {
				return nil, errors.New("seventh store table presence missing")
			}
			for _, key := range []string{"scopes", "reservations", "foreground", "background"} {
				rows, err := consolidationV2Records(windows[0][key])
				if err != nil || len(rows) != 0 {
					return nil, errors.New("current owner has unreviewed seventh store activity")
				}
			}
			continue
		}
		for _, window := range windows {
			var schema string
			_ = json.Unmarshal(window["schema"], &schema)
			if container != "issue86-20261016-pg" || name != "memory_read_test" || schema != "consolidation_native_v2" {
				continue
			}
			removed++
			for field, key := range map[string]string{"scopes": "scope_id", "reservations": "reservation_id", "foreground": "operation_id", "background": "operation_id"} {
				a, e1 := consolidationV2Records(window[field])
				b, e2 := consolidationV2Records(prior[field])
				if e1 != nil || e2 != nil || !temporalSameRecords(a, b, key) {
					return nil, errors.New("latest leaf differs from current seven-store inventory")
				}
				window[field] = json.RawMessage(`[]`)
			}
		}
		db["windows"], _ = json.Marshal(windows)
		kept = append(kept, db)
	}
	if removed != 1 || seventh != 1 {
		return nil, errors.New("latest leaf or seventh store omitted")
	}
	current["databases"], _ = json.Marshal(kept)
	return json.Marshal(current)
}

func temporalV6AdaptiveWitness(raw, originalRaw []byte, day string) (temporalV5CurrentUsage, error) {
	var usage temporalV5CurrentUsage
	var envelope struct {
		Format, Day, Owner string
		Databases          []struct {
			Container, Database string
			Port                int
			Windows             []json.RawMessage
		}
	}
	if json.Unmarshal(raw, &envelope) != nil || envelope.Format != "butler.temporal.seven-store-ledgers.v6" || envelope.Day != day || len(envelope.Databases) != 7 {
		return usage, errors.New("complete all-owner seven-store evidence required")
	}
	var original map[string]json.RawMessage
	if json.Unmarshal(originalRaw, &original) != nil {
		return usage, errors.New("original closed adaptive ledger required")
	}
	found := 0
	want := map[string]int{"butler-lab-postgres-1:butler": 5432, "issue84-20261016-pg:issue84_test": 5432, "butler-issue85-postgres:issue85_native_test": 18585, "butler-issue86-native-postgres-1:memory_read_native_test": 5432, "butler-issue87-native-v1-pg:memory_adaptive_test": 5432, "issue86-20261016-pg:memory_read_test": 5432, "butler-issue87-native-v5-pg:memory_adaptive_test": 5432}
	for _, db := range envelope.Databases {
		key := db.Container + ":" + db.Database
		if want[key] != db.Port || db.Port == 0 || db.Windows == nil || len(db.Windows) == 0 {
			return usage, errors.New("all-owner seven-store inventory identity differs")
		}
		delete(want, key)
		if db.Container != "butler-issue87-native-v5-pg" {
			continue
		}
		found++
		if db.Database != "memory_adaptive_test" || db.Port != 5432 || len(db.Windows) != 1 {
			return usage, errors.New("adaptive retained database identity differs")
		}
		var fields map[string]json.RawMessage
		var schema string
		if json.Unmarshal(db.Windows[0], &fields) != nil || json.Unmarshal(fields["schema"], &schema) != nil || schema != "adaptive_native_v5" || !temporalEqualValue(fields["adaptive_window"], original["window"]) || !temporalWitnessExact(fields["scopes"], original["scopes"], "scope_id") || !temporalWitnessExact(fields["reservations"], original["reservations"], "reservation_id") || !temporalWitnessExact(fields["foreground"], original["foreground"], "operation_id") {
			return usage, errors.New("adaptive live resource population differs from original ledger")
		}
		var state struct {
			Closed bool
			Paid   int `json:"paid_entries"`
		}
		if json.Unmarshal(fields["adaptive_window"], &state) != nil || !state.Closed || state.Paid != 98 {
			return usage, errors.New("complete closed adaptive population required")
		}
		if !temporalV6NumericProjection(fields["native_adaptive"], original["native_entries"], "entry_id", "entry_id", "run_id", "operation_id", "reservation_id", "kind", "native_usage", "full_total_known", "usage_invalid", "observed_at") {
			return usage, errors.New("adaptive native usage differs from original observer")
		}
		var entries []struct {
			ID          int64            `json:"entry_id"`
			Run         string           `json:"run_id"`
			Operation   string           `json:"operation_id"`
			Reservation string           `json:"reservation_id"`
			Usage       *transport.Usage `json:"native_usage"`
			FullKnown   bool             `json:"full_total_known"`
			Invalid     bool             `json:"usage_invalid"`
			Observed    *time.Time       `json:"observed_at"`
		}
		var foreground []struct {
			Operation   string     `json:"operation_id"`
			Run         string     `json:"run_id"`
			Reservation string     `json:"reservation_id"`
			Observed    *time.Time `json:"observed_at"`
		}
		var reservations []struct {
			Operation string         `json:"operation_id"`
			ID        string         `json:"reservation_id"`
			Owner     string         `json:"owner_id"`
			Request   budget.Request `json:"request_json"`
		}
		if json.Unmarshal(fields["native_adaptive"], &entries) != nil || len(entries) != 98 || json.Unmarshal(fields["foreground"], &foreground) != nil || len(foreground) != 98 || json.Unmarshal(fields["reservations"], &reservations) != nil {
			return usage, errors.New("complete adaptive admission witnesses required")
		}
		ops := map[string]string{}
		for _, r := range reservations {
			if r.Owner != "adaptive-prospective-owner-v1" || r.Request.OwnerID != r.Owner || ops[r.Operation] != "" || r.ID == "" {
				return usage, errors.New("adaptive owner or reservation identity differs")
			}
			ops[r.Operation] = r.ID
		}
		runs := map[string]string{}
		for _, a := range foreground {
			if a.Run == "" || ops[a.Operation] != a.Reservation || runs[a.Operation] != "" {
				return usage, errors.New("adaptive product operation lacks exact admission")
			}
			runs[a.Operation] = a.Run
			usage.ProductAttempts++
			if a.Observed == nil {
				usage.ProductNULL++
			} else {
				usage.ProductObserved++
			}
		}
		seen := map[string]bool{}
		for _, e := range entries {
			if e.ID < 1 || e.ID > int64(len(entries)) || seen[e.Operation] || runs[e.Operation] != e.Run || ops[e.Operation] != e.Reservation || e.Observed == nil || e.Invalid || e.FullKnown != (e.Usage != nil && e.Usage.TotalTokens > 0) {
				return usage, errors.New("adaptive native admission or nullable usage differs")
			}
			seen[e.Operation] = true
			usage.NativeEntries++
			if err := temporalV6AddNativeUsage(&usage, e.Usage); err != nil {
				return usage, err
			}
		}
	}
	if found != 1 || len(want) != 0 {
		return usage, errors.New("seventh adaptive store omitted or duplicated")
	}
	return usage, nil
}

func loadTemporalV6History(path, expected string) (*temporalV6History, error) {
	raw, err := os.ReadFile(path)
	h := &temporalV6History{Hash: expected}
	if err != nil || len(expected) != 64 || experienceDigest(string(raw)) != expected || json.Unmarshal(raw, &h.Manifest) != nil || h.Manifest.Format != "butler.temporal.native-history.v2" || h.Manifest.Owner != "owner" || h.Manifest.Schema != "memory_temporal_native_v6" {
		return nil, errors.New("complete frozen temporal v6 history required")
	}
	if h.Manifest.Future.Schema != h.Manifest.Schema || h.Manifest.Future.Window != "temporal-explicit-v6" || h.Manifest.Future.Present == nil || *h.Manifest.Future.Present || h.Manifest.Future.Table == nil || h.Manifest.Future.Count == nil || *h.Manifest.Future.Count != 0 {
		return nil, errors.New("fresh unconsumed v6 schema and one-use receipt required")
	}
	root := filepath.Dir(path)
	read := func(ref consolidationHistoryRef) ([]byte, error) { return consolidationV2Read(root, ref) }
	if _, err = read(h.Manifest.Previous); err != nil {
		return nil, err
	}
	h.Previous, err = loadConsolidationV2History(filepath.Join(root, h.Manifest.Previous.Path), h.Manifest.Previous.SHA256)
	if err != nil || h.Previous.CurrentCarry.ModelTurns != 38 || h.Previous.ProspectiveDay != h.Manifest.Day || !h.Previous.ProspectiveDeadline.Equal(h.Manifest.Deadline) {
		return nil, errors.New("same-day recursive original38 predecessor proof required")
	}
	prior, err := read(h.Manifest.Predecessor)
	if err != nil {
		return nil, err
	}
	ledger, err := read(h.Manifest.Ledger89)
	if err != nil {
		return nil, err
	}
	screen, err := read(h.Manifest.Screen89)
	if err != nil {
		return nil, err
	}
	h.Carry, h.Usage89, err = temporalV6Leaf(prior, ledger, screen, h.Previous, h.Manifest.Day, h.Manifest.Deadline)
	if err != nil || h.Carry.ModelTurns != 49 {
		return nil, errors.New("latest inclusive49 owner leaf required")
	}
	current, err := read(h.Manifest.Current)
	if err != nil {
		return nil, err
	}
	cleaned, err := temporalV6ClearLeaf(current, prior)
	if err != nil {
		return nil, err
	}
	// Re-prove the entire earlier current-day chain against this newly read
	// inventory, including all old original receipts and NULL observations.
	previousRaw, err := read(h.Manifest.Previous)
	if err != nil {
		return nil, err
	}
	var previousManifest consolidationV2Manifest
	if json.Unmarshal(previousRaw, &previousManifest) != nil {
		return nil, errors.New("previous history unavailable")
	}
	old88, err := read(previousManifest.Predecessor)
	if err != nil {
		return nil, err
	}
	oldHistory, err := loadTemporalV5History(filepath.Join(root, previousManifest.Previous.Path), previousManifest.Previous.SHA256)
	if err != nil {
		return nil, err
	}
	old89, err := read(oldHistory.Manifest.Predecessor)
	if err != nil {
		return nil, err
	}
	if err = consolidationV2ValidateInventory(cleaned, old88, oldHistory, old89, h.Manifest.Day, h.Manifest.Deadline); err != nil {
		return nil, err
	}
	all, err := read(h.Manifest.AllOwners)
	if err != nil {
		return nil, err
	}
	original87, err := read(h.Manifest.Ledger87)
	if err != nil {
		return nil, err
	}
	h.Usage87, err = temporalV6AdaptiveWitness(all, original87, h.Manifest.Day)
	if err != nil {
		return nil, err
	}
	history87, err := read(h.Manifest.History87)
	if err != nil {
		return nil, err
	}
	var previous87 struct {
		Format, Owner, Day string
		Deadline           time.Time
		Inventory          string                    `json:"inventory_sha256"`
		Ledger             string                    `json:"historical_ledger_sha256"`
		Native             string                    `json:"historical_native_sha256"`
		Predecessors       []consolidationHistoryRef `json:"closed_latest_issue89"`
	}
	if json.Unmarshal(history87, &previous87) != nil || previous87.Format != "butler.adaptive.native-history.v5" || previous87.Owner != "adaptive-prospective-owner-v1" || previous87.Day != h.Manifest.Day || !previous87.Deadline.Equal(h.Manifest.Deadline) || previous87.Inventory != h.Manifest.Inventory87.SHA256 || previous87.Ledger != h.Manifest.OldLedger87.SHA256 || previous87.Native != h.Manifest.OldNative87.SHA256 || len(previous87.Predecessors) != 3 || len(h.Manifest.Predecessors87) != 3 {
		return nil, errors.New("complete original adaptive predecessor lineage required")
	}
	for _, ref := range append([]consolidationHistoryRef{h.Manifest.Inventory87, h.Manifest.OldLedger87, h.Manifest.OldNative87}, h.Manifest.Predecessors87...) {
		if _, err = read(ref); err != nil {
			return nil, err
		}
	}
	for i, ref := range previous87.Predecessors {
		if ref != h.Manifest.Predecessors87[i] {
			return nil, errors.New("adaptive original predecessor references differ")
		}
	}
	return h, nil
}

func temporalV6NumericProjection(projected, original json.RawMessage, key string, required ...string) bool {
	read := func(raw json.RawMessage) (map[int64]json.RawMessage, bool) {
		var rows []json.RawMessage
		if json.Unmarshal(raw, &rows) != nil || rows == nil {
			return nil, false
		}
		out := map[int64]json.RawMessage{}
		for _, row := range rows {
			var fields map[string]json.RawMessage
			var id int64
			if json.Unmarshal(row, &fields) != nil || json.Unmarshal(fields[key], &id) != nil || id < 1 || out[id] != nil {
				return nil, false
			}
			out[id] = row
		}
		return out, true
	}
	p, ok := read(projected)
	if !ok {
		return false
	}
	o, ok := read(original)
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
