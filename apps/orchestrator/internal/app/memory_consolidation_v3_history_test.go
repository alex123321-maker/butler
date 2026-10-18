package app

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"os"
	"path/filepath"
)

const consolidationV3Schema = "consolidation_native_v3"
const consolidationV3Window = "consolidation-explicit-v3"

type consolidationV3Manifest struct {
	consolidationV2Manifest
	AllOwners consolidationHistoryRef `json:"all_owners"`
	Closure   consolidationHistoryRef `json:"predecessor88_closure"`
	Report    string                  `json:"predecessor_report"`
	Future    struct {
		Schema  string `json:"schema"`
		Window  string `json:"window_id"`
		Present *bool  `json:"schema_present"`
		Table   *bool  `json:"receipt_table_present"`
		Count   *int   `json:"receipt_count"`
	} `json:"future_window"`
}

func consolidationSuccessorSchema(schema string) bool {
	return schema == "consolidation_native_v2" || schema == consolidationV3Schema
}

// This is a fixed lab recipe. It does not create cross-database production
// authority, reinterpret prior failures or add an inclusive predecessor twice.
func loadConsolidationV3History(path, expected string) (*consolidationScreenHistory, error) {
	raw, err := os.ReadFile(path)
	var m consolidationV3Manifest
	if err != nil || len(raw) > 32*1024*1024 || len(expected) != 64 || experienceDigest(string(raw)) != expected || json.Unmarshal(raw, &m) != nil || m.Format != "butler.consolidation.native-history.v4" || m.Owner != "owner" || m.Schema != consolidationV3Schema {
		return nil, errors.New("complete immutable consolidation v3 successor required")
	}
	if m.Future.Schema != m.Schema || m.Future.Window != consolidationV3Window || m.Future.Present == nil || *m.Future.Present || m.Future.Table == nil || m.Future.Count == nil || *m.Future.Count != 0 {
		return nil, errors.New("fresh v3 namespace and unconsumed receipt proof required")
	}
	root := filepath.Dir(path)
	read := func(ref consolidationHistoryRef) ([]byte, error) { return consolidationV2Read(root, ref) }
	if _, err = read(m.Previous); err != nil {
		return nil, err
	}
	previous, err := loadTemporalV5History(filepath.Join(root, m.Previous.Path), m.Previous.SHA256)
	if err != nil || previous.SuccessorV6 == nil || previous.Carry.ModelTurns != 49 || m.Day != previous.Manifest.Day || !m.Deadline.Equal(previous.Manifest.Deadline) {
		return nil, errors.New("same-day recursive temporal-v6 inclusive49 lineage required")
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
	if len(m.Entries) < 1 || len(m.Entries) > temporalMaxNative {
		return nil, errors.New("all original temporal-v6 entry files required")
	}
	entries := map[int64][]byte{}
	for _, ref := range m.Entries {
		data, err := read(ref)
		if err != nil {
			return nil, err
		}
		var entry struct {
			ID int64 `json:"id"`
		}
		if json.Unmarshal(data, &entry) != nil || entry.ID < 1 || entries[entry.ID] != nil || ref.Path != fmt.Sprintf("v6-entry-%03d.json", entry.ID) {
			return nil, errors.New("original temporal-v6 entry identity differs")
		}
		entries[entry.ID] = data
	}
	if err = consolidationV2Corroborate(prior, ledger, outcomes, entries, m.Day); err != nil {
		return nil, err
	}
	closure, err := read(m.Closure)
	if err != nil {
		return nil, err
	}
	if err = consolidationV3ValidateClosure(closure, outcomes); err != nil {
		return nil, err
	}
	carry, usage, err := consolidationValidateTemporalLeaf(prior, ledger, previous, m.Day, m.Deadline, "butler.consolidation.predecessor88.v2", "issue88-v6-current-day-carry:")
	if err != nil {
		return nil, err
	}
	current, err := read(m.Current)
	if err != nil {
		return nil, err
	}
	if err = consolidationV3ValidateInventory(current, prior, previous.SuccessorV6, root); err != nil {
		return nil, err
	}
	all, err := read(m.AllOwners)
	if err != nil {
		return nil, err
	}
	adaptiveOriginal, err := read(previous.SuccessorV6.Manifest.Ledger87)
	if err != nil {
		return nil, err
	}
	adaptive, err := temporalV6AdaptiveWitness(all, adaptiveOriginal, m.Day)
	if err != nil || adaptive != previous.SuccessorV6.Usage87 {
		return nil, errors.New("separate adaptive original population changed")
	}
	previousCurrent, err := read(previous.SuccessorV6.Manifest.Current)
	if err != nil {
		return nil, err
	}
	var old consolidationCurrentDay
	var latest consolidationV2Predecessor
	if json.Unmarshal(previousCurrent, &old) != nil || json.Unmarshal(prior, &latest) != nil {
		return nil, errors.New("complete original operation lineage required")
	}
	operations := map[string]bool{}
	for _, db := range old.Databases {
		for _, window := range db.Windows {
			for _, row := range append(window.Foreground, window.Background...) {
				var a struct {
					Operation string `json:"operation_id"`
				}
				if json.Unmarshal(row, &a) != nil || a.Operation == "" {
					return nil, errors.New("prior operation identity unavailable")
				}
				operations[a.Operation] = true
			}
		}
	}
	for _, a := range latest.Foreground {
		if operations[a.Operation] {
			return nil, errors.New("original model operation repeats across current-day cohorts")
		}
	}
	if usage.KnownFullTokenLowerBound > int64(^uint64(0)>>1)-previous.Usage.KnownFullTokenLowerBound {
		return nil, errors.New("current-day full native usage overflow")
	}
	h := *previous.Historical
	h.Hash, h.ProspectiveDay, h.ProspectiveDeadline, h.CurrentCarry = expected, m.Day, m.Deadline, carry
	h.Successor = &consolidationV2Summary{Schema: m.Schema, Day: m.Day, PreviousHistorySHA: previous.Hash, Previous89: previous.Usage, New88: usage, Current89And88: consolidationV2AddUsage(previous.Usage, usage), SeparateAdaptive: adaptive, Lineage: "161 historical; original89v1/88v5/89v2 inclusive49 once; new88v6 originals; distinct adaptive owner separate", SourceHashes: map[string]string{}}
	for _, ref := range append([]consolidationHistoryRef{m.Previous, m.Current, m.AllOwners, m.Predecessor, m.Ledger, m.Outcomes, m.Closure}, m.Entries...) {
		h.Successor.SourceHashes[ref.Path] = ref.SHA256
	}
	return &h, nil
}

// Collector failure is retained separately from successful opaque handback.
// A status1 window can carry paid quotes/NULLs without becoming a quality pass.
func consolidationV3ValidateClosure(raw, outcomesRaw []byte) error {
	var c struct {
		Status   *int `json:"native_status"`
		Returned struct {
			CAS     bool `json:"opaque_main_cas"`
			Checked bool `json:"active_work_checked"`
		} `json:"return_safe"`
		Main struct {
			Resolved bool  `json:"resolved"`
			Unsafe   *bool `json:"unsafe_refresh"`
			Expired  *bool `json:"expired"`
			Trace    []struct {
				Stage, Outcome string
				HTTP           int `json:"http_status"`
			}
		} `json:"main_auth_safe"`
		Snapshot struct {
			Fresh bool `json:"fresh_main_snapshot_saved"`
		} `json:"main_snapshot_safe"`
	}
	var outcomes []temporalCaseObservation
	if json.Unmarshal(raw, &c) != nil || c.Status == nil || (*c.Status != 0 && *c.Status != 1) || !c.Returned.CAS || !c.Returned.Checked || !c.Main.Resolved || c.Main.Unsafe == nil || *c.Main.Unsafe || c.Main.Expired == nil || *c.Main.Expired || !c.Snapshot.Fresh || json.Unmarshal(outcomesRaw, &outcomes) != nil || len(outcomes) != 12 {
		return errors.New("exact closed original handback proof required")
	}
	paid200 := false
	for _, s := range c.Main.Trace {
		if s.Stage == "paid_discovery" && s.Outcome == "completed" && s.HTTP == 200 {
			paid200 = true
		}
	}
	completed := 0
	for _, o := range outcomes {
		if o.Status == "completed" {
			completed++
		}
	}
	if !paid200 || (*c.Status == 0 && completed != 12) || (*c.Status == 1 && completed == 12) {
		return errors.New("original collector result and exact main200 proof differ")
	}
	return nil
}

// Remove only the corroborated v6 leaf from a deep copy, then re-prove every
// earlier owner population and all seven stores through the reviewed recipes.
func consolidationV3ValidateInventory(raw, priorRaw []byte, previous *temporalV6History, root string) error {
	if previous == nil {
		return errors.New("validated recursive current-day predecessor required")
	}
	var current map[string]json.RawMessage
	var dbs []map[string]json.RawMessage
	var prior map[string]json.RawMessage
	if json.Unmarshal(raw, &current) != nil || json.Unmarshal(current["databases"], &dbs) != nil || len(dbs) != 7 || json.Unmarshal(priorRaw, &prior) != nil {
		return errors.New("complete current seven-store inventory required")
	}
	found := 0
	for _, db := range dbs {
		var container, name string
		var windows []map[string]json.RawMessage
		if json.Unmarshal(db["container"], &container) != nil || json.Unmarshal(db["database"], &name) != nil || json.Unmarshal(db["windows"], &windows) != nil {
			return errors.New("current store identity unavailable")
		}
		for _, window := range windows {
			var schema string
			_ = json.Unmarshal(window["schema"], &schema)
			if container != "butler-issue85-postgres" || name != "issue85_native_test" || schema != "memory_temporal_native_v6" {
				continue
			}
			found++
			for field, key := range map[string]string{"scopes": "scope_id", "reservations": "reservation_id", "foreground": "operation_id", "background": "operation_id"} {
				a, e1 := consolidationV2Records(window[field])
				b, e2 := consolidationV2Records(prior[field])
				if e1 != nil || e2 != nil || !temporalSameRecords(a, b, key) {
					return errors.New("latest temporal-v6 leaf differs from current inventory")
				}
				window[field] = json.RawMessage(`[]`)
			}
		}
		db["windows"], _ = json.Marshal(windows)
	}
	if found != 1 {
		return errors.New("one exact latest temporal-v6 owner leaf required")
	}
	current["databases"], _ = json.Marshal(dbs)
	cleaned, _ := json.Marshal(current)
	read := func(ref consolidationHistoryRef) ([]byte, error) { return consolidationV2Read(root, ref) }
	old89, err := read(previous.Manifest.Predecessor)
	if err != nil {
		return err
	}
	cleaned, err = temporalV6ClearLeaf(cleaned, old89)
	if err != nil {
		return err
	}
	previousRaw, err := read(previous.Manifest.Previous)
	if err != nil {
		return err
	}
	var oldManifest consolidationV2Manifest
	if json.Unmarshal(previousRaw, &oldManifest) != nil {
		return errors.New("recursive original38 manifest unavailable")
	}
	old88, err := read(oldManifest.Predecessor)
	if err != nil {
		return err
	}
	oldHistory, err := loadTemporalV5History(filepath.Join(root, oldManifest.Previous.Path), oldManifest.Previous.SHA256)
	if err != nil {
		return err
	}
	oldest89, err := read(oldHistory.Manifest.Predecessor)
	if err != nil {
		return err
	}
	return consolidationV2ValidateInventory(cleaned, old88, oldHistory, oldest89, previous.Manifest.Day, previous.Manifest.Deadline)
}

// The public receipt and fresh private namespace are one atomic irreversible
// claim. An empty schema is still consumed; no later attempt may reopen it.
func consolidationV3ClaimSchema(ctx context.Context, pool *pgxpool.Pool, historySHA string) error {
	if len(historySHA) != 64 {
		return errors.New("exact frozen history required for one-use namespace")
	}
	tx, err := pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)
	if _, err = tx.Exec(ctx, `CREATE TABLE IF NOT EXISTS public.issue89_native_windows(window_id TEXT PRIMARY KEY,schema_name TEXT NOT NULL UNIQUE,history_sha256 TEXT NOT NULL CHECK(length(history_sha256)=64),created_at TIMESTAMPTZ NOT NULL DEFAULT clock_timestamp())`); err != nil {
		return err
	}
	if _, err = tx.Exec(ctx, `INSERT INTO public.issue89_native_windows(window_id,schema_name,history_sha256) VALUES($1,$2,$3)`, consolidationV3Window, consolidationV3Schema, historySHA); err != nil {
		return errors.New("native v3 one-use receipt already consumed")
	}
	if _, err = tx.Exec(ctx, `CREATE SCHEMA `+pgx.Identifier{consolidationV3Schema}.Sanitize()); err != nil {
		return errors.New("native v3 namespace already exists")
	}
	return tx.Commit(ctx)
}
