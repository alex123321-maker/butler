package app

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/butler/butler/internal/budget"
)

// This read-only lab receipt validates a CLOSED V7 leaf. It grants no runtime,
// provider or cross-database authority. The consumer still owns its atomic
// namespace claim, finite policy and one-use import before its first admission.
type closedTemporalV7Manifest struct {
	Format, Owner, Day string
	Deadline           time.Time
	Previous           consolidationHistoryRef   `json:"previous_v7_history"`
	Current            consolidationHistoryRef   `json:"current"`
	AllOwners          consolidationHistoryRef   `json:"all_owners"`
	Predecessor        consolidationHistoryRef   `json:"closed_v7"`
	Ledger             consolidationHistoryRef   `json:"original_v7_ledger"`
	Outcomes           consolidationHistoryRef   `json:"original_v7_outcomes"`
	Entries            []consolidationHistoryRef `json:"original_v7_entries"`
	Closure            consolidationHistoryRef   `json:"closed_v7_handback"`
	Operator           consolidationHistoryRef   `json:"operator_accounting"`
	SetupFailure       consolidationHistoryRef   `json:"zero_auth_setup_failure"`
	SetupOutcomes      consolidationHistoryRef   `json:"zero_auth_setup_outcomes"`
	Future             closedFuture90            `json:"future90"`
}

type closedFuture90 struct {
	Container, Database, Schema, Window, Receipt string
	Present, Table                               *bool
	Count                                        *int
}

func validateClosedFuture90(f closedFuture90) error {
	if f.Container != "issue86-20261016-pg" || f.Database != "memory_read_test" || f.Schema != "experience_native_v1" || f.Window != "experience-explicit-v1" || f.Receipt != "public.issue90_native_windows" || f.Present == nil || *f.Present || f.Table == nil || f.Count == nil || *f.Count != 0 {
		return errors.New("exact unconsumed future90 namespace receipt required")
	}
	return nil
}

// Usage is inclusive only for the experiment owner/day. Historical Oct15 and
// the distinct adaptive owner remain separate recursively validated evidence.
type closedTemporalV7History struct {
	Hash, Day        string
	Deadline         time.Time
	Carry            budget.Units
	Usage, NewUsage  temporalV5CurrentUsage
	SeparateAdaptive temporalV5CurrentUsage
	Historical       *consolidationScreenHistory
	Previous         *temporalV5History
	Manifest         closedTemporalV7Manifest
	SourceHashes     map[string]string
}

func closedTemporalV7Leaf(priorRaw, ledgerRaw, outcomesRaw []byte, entries map[int64][]byte, previous *temporalV5History, day string, deadline time.Time) (budget.Units, temporalV5CurrentUsage, error) {
	return closedTemporalLeaf(priorRaw, ledgerRaw, outcomesRaw, entries, previous, day, deadline, "butler.consolidation.predecessor88.v3", "issue88-v7-current-day-carry:")
}

func closedTemporalLeaf(priorRaw, ledgerRaw, outcomesRaw []byte, entries map[int64][]byte, previous *temporalV5History, day string, deadline time.Time, format, prefix string) (budget.Units, temporalV5CurrentUsage, error) {
	ledgerRaw = temporalEmptyOriginalAggregates(ledgerRaw, "entries", "foreground")
	if err := consolidationV2Corroborate(priorRaw, ledgerRaw, outcomesRaw, entries, day); err != nil {
		return budget.Units{}, temporalV5CurrentUsage{}, err
	}
	used, usage, err := consolidationValidateTemporalLeafMinimum(priorRaw, ledgerRaw, previous, day, deadline, format, prefix, 0)
	if err != nil {
		return used, usage, err
	}
	var p consolidationV2Predecessor
	var ledger struct {
		Cases []struct {
			ID, Status string
			Run        string `json:"run_id"`
			Count      int    `json:"native_entries"`
		} `json:"cases"`
		Foreground []struct {
			Operation string     `json:"operation_id"`
			Run       string     `json:"run_id"`
			Observed  *time.Time `json:"observed_at"`
		} `json:"foreground"`
	}
	if json.Unmarshal(priorRaw, &p) != nil || json.Unmarshal(ledgerRaw, &ledger) != nil {
		return used, usage, errors.New("closed V7 original sequence unavailable")
	}
	byID := map[int64]consolidationV2Entry{}
	for _, e := range p.Entries {
		byID[e.ID] = e
	}
	started, native := map[string]bool{}, map[string]bool{}
	for i := int64(1); i <= int64(len(p.Entries)); i++ {
		e := byID[i]
		if e.Kind == "start" {
			if started[e.Run] {
				return used, usage, errors.New("closed V7 run repeats Start")
			}
			started[e.Run] = true
		} else if !started[e.Run] {
			return used, usage, errors.New("closed V7 continuation precedes Start")
		}
		native[e.Observation.Operation] = true
	}
	caseRuns := map[string]bool{}
	for _, c := range ledger.Cases {
		if c.Count > temporalMaxPerOriginal || c.Count < 0 || c.Status == "completed" && c.Count == 0 || c.Run != "" && caseRuns[c.Run] {
			return used, usage, errors.New("closed V7 original native count or run differs")
		}
		if c.Run != "" {
			caseRuns[c.Run] = true
		}
	}
	for _, a := range ledger.Foreground {
		if !caseRuns[a.Run] || !native[a.Operation] && a.Observed != nil {
			return used, usage, errors.New("closed V7 orphan product lacks original run or NULL observation")
		}
	}
	return used, usage, nil
}

// Remove only the corroborated latest leaf from a deep decoded copy. Re-run
// the earlier V7 inventory recipe on the remainder, rather than adding the
// inclusive 109,73,49,... scopes to the new inclusive scope.
func closedTemporalV7Inventory(raw, prior []byte, previous *temporalV5History, root string) error {
	if previous == nil || previous.SuccessorV7 == nil {
		return errors.New("validated V7 predecessor history required")
	}
	cleaned, err := temporalClearCurrentLeaf(raw, prior, "butler-issue85-postgres", "issue85_native_test", temporalV7Schema)
	if err != nil {
		return err
	}
	v7 := previous.SuccessorV7
	read := func(r consolidationHistoryRef) ([]byte, error) { return consolidationV2Read(root, r) }
	old89, err := read(v7.Manifest.Predecessor)
	if err != nil {
		return err
	}
	cleaned, err = temporalV7ClearLeaf(cleaned, old89)
	if err != nil {
		return err
	}
	oldManifestRaw, err := read(v7.Manifest.Previous)
	var m consolidationV3Manifest
	if err != nil || json.Unmarshal(oldManifestRaw, &m) != nil {
		return errors.New("original consolidation v3 manifest unavailable")
	}
	old88, err := read(m.Predecessor)
	if err != nil {
		return err
	}
	v6, err := loadTemporalV5History(filepath.Join(root, m.Previous.Path), m.Previous.SHA256)
	if err != nil || v6.SuccessorV6 == nil {
		return errors.New("original temporal v6 lineage unavailable")
	}
	return consolidationV3ValidateInventory(cleaned, old88, v6.SuccessorV6, root)
}

// Every other owner/schema remains byte-value identical to the preceding
// validated seven-store inventory. The one added V7 schema is corroborated
// against its full original ledger, not removed by an unchecked schema name.
func closedTemporalV7AllOwners(raw, oldRaw, ledgerRaw []byte) error {
	return closedTemporalAllOwners(raw, oldRaw, ledgerRaw, temporalV7Schema)
}

func closedTemporalAllOwners(raw, oldRaw, ledgerRaw []byte, targetSchema string) error {
	ledgerRaw = temporalEmptyOriginalAggregates(ledgerRaw, "entries", "foreground")
	var now, old map[string]json.RawMessage
	var dbs []map[string]json.RawMessage
	var ledger map[string]json.RawMessage
	if json.Unmarshal(raw, &now) != nil || json.Unmarshal(oldRaw, &old) != nil || json.Unmarshal(now["databases"], &dbs) != nil || len(dbs) != 7 || json.Unmarshal(ledgerRaw, &ledger) != nil {
		return errors.New("complete closed V7 all-owner inventory required")
	}
	found := 0
	for _, db := range dbs {
		var container, database string
		var windows []map[string]json.RawMessage
		if json.Unmarshal(db["container"], &container) != nil || json.Unmarshal(db["database"], &database) != nil || json.Unmarshal(db["windows"], &windows) != nil {
			return errors.New("all-owner store identity unavailable")
		}
		kept := []map[string]json.RawMessage{}
		for _, w := range windows {
			var schema string
			_ = json.Unmarshal(w["schema"], &schema)
			if schema != targetSchema || container != "butler-issue85-postgres" || database != "issue85_native_test" {
				kept = append(kept, w)
				continue
			}
			found++
			for key, identity := range map[string]string{"scopes": "scope_id", "reservations": "reservation_id"} {
				a, e1 := consolidationV2Records(w[key])
				b, e2 := consolidationV2Records(ledger[key])
				if e1 != nil || e2 != nil || !temporalSameRecords(a, b, identity) {
					return errors.New("all-owner V7 full record differs from original ledger")
				}
			}
			keys := append(append([]string{}, temporalForegroundWitnessKeys...), "run_id")
			if !temporalWitnessProjection(w["foreground"], ledger["foreground"], "operation_id", keys...) {
				return errors.New("all-owner V7 product/run nullable fields differ from original ledger")
			}
			var native []map[string]json.RawMessage
			var originals []map[string]json.RawMessage
			if json.Unmarshal(w["native_temporal"], &native) != nil || native == nil || json.Unmarshal(ledger["entries"], &originals) != nil || len(native) != len(originals) {
				return errors.New("all-owner V7 native population differs")
			}
			byID := map[string]map[string]json.RawMessage{}
			for _, row := range originals {
				byID[string(row["id"])] = row
			}
			seen := map[string]bool{}
			for _, row := range native {
				id := string(row["entry_id"])
				source, ok := byID[id]
				var observation map[string]json.RawMessage
				if !ok || seen[id] || json.Unmarshal(source["observation_json"], &observation) != nil {
					return errors.New("all-owner V7 native identity differs")
				}
				seen[id] = true
				for _, field := range []string{"run_id", "operation_id", "reservation_id", "created_at", "finished_at"} {
					if row[field] == nil || source[field] == nil || !temporalEqualValue(row[field], source[field]) {
						return errors.New("all-owner V7 native admission or time differs")
					}
				}
				if row["usage"] == nil || observation["usage"] == nil || !temporalEqualValue(row["usage"], observation["usage"]) {
					return errors.New("all-owner V7 native NULL or usage differs")
				}
			}
			var state struct{ Temporal struct{ Closed bool } }
			var observers struct{ Temporal, Consolidation bool }
			if json.Unmarshal(w["native_window_states"], &state) != nil || !state.Temporal.Closed || json.Unmarshal(w["native_observers_present"], &observers) != nil || !observers.Temporal || observers.Consolidation {
				return errors.New("closed V7 native observer proof unavailable")
			}
			var bg []json.RawMessage
			if json.Unmarshal(w["background"], &bg) != nil || bg == nil || len(bg) != 0 {
				return errors.New("unexpected V7 all-owner background attempt")
			}
		}
		db["windows"], _ = json.Marshal(kept)
	}
	if found != 1 {
		return errors.New("one corroborated closed V7 all-owner schema required")
	}
	now["databases"], _ = json.Marshal(dbs)
	nowRaw, _ := json.Marshal(now)
	if !temporalEqualValue(nowRaw, oldRaw) {
		return errors.New("additional or changed owner/schema population outside closed V7")
	}
	return nil
}

func loadClosedTemporalV7History(path, expected string) (*closedTemporalV7History, error) {
	raw, err := os.ReadFile(path)
	h := &closedTemporalV7History{Hash: expected, SourceHashes: map[string]string{}}
	if err != nil || len(raw) > 32*1024*1024 || len(expected) != 64 || experienceDigest(string(raw)) != expected || json.Unmarshal(raw, &h.Manifest) != nil || h.Manifest.Format != "butler.memory.closed-owner-leaf.v1" || h.Manifest.Owner != "owner" {
		return nil, errors.New("bounded immutable closed V7 manifest required")
	}
	m := h.Manifest
	if err = validateClosedFuture90(m.Future); err != nil {
		return nil, err
	}
	root := filepath.Dir(path)
	read := func(ref consolidationHistoryRef) ([]byte, error) {
		raw, e := consolidationV2Read(root, ref)
		if e == nil {
			h.SourceHashes[ref.Path] = ref.SHA256
		}
		return raw, e
	}
	if _, err = read(m.Previous); err != nil {
		return nil, err
	}
	h.Previous, err = loadTemporalV5History(filepath.Join(root, m.Previous.Path), m.Previous.SHA256)
	if err != nil || h.Previous.SuccessorV7 == nil || m.Day != h.Previous.Manifest.Day || !m.Deadline.Equal(h.Previous.Manifest.Deadline) {
		return nil, errors.New("same-day recursively validated V7 prehistory required")
	}
	prior, err := read(m.Predecessor)
	if err != nil {
		return nil, err
	}
	ledger, err := read(m.Ledger)
	if err != nil {
		return nil, err
	}
	outcomes, err := read(m.Outcomes)
	if err != nil {
		return nil, err
	}
	if len(m.Entries) > temporalMaxNative {
		return nil, errors.New("bounded original V7 entries required")
	}
	entries := map[int64][]byte{}
	for _, ref := range m.Entries {
		data, e := read(ref)
		var entry struct{ ID int64 }
		if e != nil || json.Unmarshal(data, &entry) != nil || entry.ID < 1 || entries[entry.ID] != nil || ref.Path != fmt.Sprintf("closed-v7-entry-%03d.json", entry.ID) {
			return nil, errors.New("original closed V7 entry identity differs")
		}
		entries[entry.ID] = data
	}
	h.Carry, h.NewUsage, err = closedTemporalV7Leaf(prior, ledger, outcomes, entries, h.Previous, m.Day, m.Deadline)
	if err != nil {
		return nil, err
	}
	closure, err := read(m.Closure)
	if err != nil {
		return nil, err
	}
	if err = consolidationV3ValidateClosure(closure, outcomes); err != nil {
		return nil, err
	}
	current, err := read(m.Current)
	if err != nil {
		return nil, err
	}
	if err = closedTemporalV7Inventory(current, prior, h.Previous, root); err != nil {
		return nil, err
	}
	all, err := read(m.AllOwners)
	if err != nil {
		return nil, err
	}
	oldAll, err := read(h.Previous.SuccessorV7.Manifest.AllOwners)
	if err != nil {
		return nil, err
	}
	if err = closedTemporalV7AllOwners(all, oldAll, ledger); err != nil {
		return nil, err
	}
	// Operator/control-plane accounting and the zero-auth setup failure are
	// immutable distinct receipts; neither is invented as an owner model quote.
	for _, ref := range []consolidationHistoryRef{m.Operator, m.SetupFailure, m.SetupOutcomes} {
		if _, err = read(ref); err != nil {
			return nil, err
		}
	}
	if h.NewUsage.KnownFullTokenLowerBound > int64(^uint64(0)>>1)-h.Previous.Usage.KnownFullTokenLowerBound {
		return nil, errors.New("inclusive closed V7 token total overflow")
	}
	h.Hash, h.Day, h.Deadline = expected, m.Day, m.Deadline
	h.Usage = consolidationV2AddUsage(h.Previous.Usage, h.NewUsage)
	h.SeparateAdaptive, h.Historical = h.Previous.SuccessorV7.Usage87, h.Previous.Historical
	return h, nil
}
