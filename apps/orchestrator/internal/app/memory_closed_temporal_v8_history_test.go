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

// Fixed CLOSED V8/V9 accounting recipes. The latest-inclusive population is
// derived from corroborated original admissions and native entries separately;
// neither 145 nor a presumed final181 is a consumer admission rule.
type closedOwnerManifest struct {
	Format, Owner, Day, Schema, Window string
	Version                            int
	Deadline                           time.Time
	Previous                           consolidationHistoryRef   `json:"previous_history"`
	Current                            consolidationHistoryRef   `json:"current"`
	AllOwners                          consolidationHistoryRef   `json:"all_owners"`
	Predecessor                        consolidationHistoryRef   `json:"closed_stage"`
	Ledger                             consolidationHistoryRef   `json:"original_ledger"`
	Outcomes                           consolidationHistoryRef   `json:"original_outcomes"`
	Entries                            []consolidationHistoryRef `json:"original_entries"`
	Closure                            consolidationHistoryRef   `json:"closed_handback"`
	Operator                           consolidationHistoryRef   `json:"operator_accounting"`
	Pins                               consolidationHistoryRef   `json:"original_pins,omitempty"`
	Future                             closedFuture90            `json:"future90"`
}

type closedOwnerDayHistory struct {
	Hash, Day        string
	Deadline         time.Time
	Carry            budget.Units
	Usage, NewUsage  temporalV5CurrentUsage
	SeparateAdaptive temporalV5CurrentUsage
	Historical       *consolidationScreenHistory
	Previous         *temporalV5History
	Manifest         closedOwnerManifest
	SourceHashes     map[string]string
}

func loadLatestClosedOwnerHistory(path, expected string) (*closedOwnerDayHistory, error) {
	raw, err := os.ReadFile(path)
	var envelope struct{ Format string }
	if err != nil || len(raw) > 32*1024*1024 || len(expected) != 64 || experienceDigest(string(raw)) != expected || json.Unmarshal(raw, &envelope) != nil {
		return nil, errors.New("bounded immutable latest closed-owner manifest required")
	}
	if envelope.Format == "butler.memory.closed-owner-leaf.v1" {
		h, err := loadClosedTemporalV7History(path, expected)
		if err != nil {
			return nil, err
		}
		return &closedOwnerDayHistory{Hash: h.Hash, Day: h.Day, Deadline: h.Deadline, Carry: h.Carry, Usage: h.Usage, NewUsage: h.NewUsage, SeparateAdaptive: h.SeparateAdaptive, Historical: h.Historical, Previous: h.Previous, SourceHashes: h.SourceHashes}, nil
	}
	if envelope.Format != "butler.memory.closed-owner-leaf.v2" {
		return nil, errors.New("unsupported fixed closed-owner recipe")
	}
	h := &closedOwnerDayHistory{Hash: expected, SourceHashes: map[string]string{}}
	if json.Unmarshal(raw, &h.Manifest) != nil || h.Manifest.Owner != "owner" || !((h.Manifest.Version == 8 && h.Manifest.Schema == temporalV8Schema && h.Manifest.Window == temporalV8Window) || (h.Manifest.Version == 9 && h.Manifest.Schema == temporalV9Schema && h.Manifest.Window == temporalV9Window)) {
		return nil, errors.New("complete fixed closed V8/V9 identity required")
	}
	m := h.Manifest
	if err = validateClosedFuture90(m.Future); err != nil {
		return nil, err
	}
	root := filepath.Dir(path)
	read := func(ref consolidationHistoryRef) ([]byte, error) {
		data, e := consolidationV2Read(root, ref)
		if e == nil {
			h.SourceHashes[ref.Path] = ref.SHA256
		}
		return data, e
	}
	if _, err = read(m.Previous); err != nil {
		return nil, err
	}
	h.Previous, err = loadTemporalV5History(filepath.Join(root, m.Previous.Path), m.Previous.SHA256)
	if err != nil || (m.Version == 8 && h.Previous.SuccessorV8 == nil) || (m.Version == 9 && h.Previous.SuccessorV9 == nil) || m.Day != h.Previous.Manifest.Day || !m.Deadline.Equal(h.Previous.Manifest.Deadline) {
		return nil, errors.New("same-day validated prospective V8 lineage required")
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
		return nil, errors.New("bounded original V8 entry population required")
	}
	entries := map[int64][]byte{}
	for _, ref := range m.Entries {
		data, e := read(ref)
		var entry struct{ ID int64 }
		if e != nil || json.Unmarshal(data, &entry) != nil || entry.ID < 1 || entries[entry.ID] != nil || ref.Path != fmt.Sprintf("closed-v%d-entry-%03d.json", m.Version, entry.ID) {
			return nil, errors.New("original closed V8 entry identity differs")
		}
		entries[entry.ID] = data
	}
	h.Carry, h.NewUsage, err = closedTemporalLeaf(prior, ledger, outcomes, entries, h.Previous, m.Day, m.Deadline, fmt.Sprintf("butler.consolidation.predecessor88.v%d", m.Version-4), fmt.Sprintf("issue88-v%d-current-day-carry:", m.Version))
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
	var oldAllRef consolidationHistoryRef
	if m.Version == 8 {
		err = closedTemporalV8Inventory(current, prior, h.Previous, root)
		oldAllRef = h.Previous.SuccessorV8.Manifest.AllOwners
		h.SeparateAdaptive = h.Previous.SuccessorV8.SeparateAdaptive
	} else {
		var cleaned, prior8 []byte
		cleaned, err = temporalClearCurrentLeaf(current, prior, "butler-issue85-postgres", "issue85_native_test", temporalV9Schema)
		previous8 := h.Previous.SuccessorV9
		if err == nil {
			prior8, err = read(previous8.Manifest.Predecessor)
		}
		if err == nil {
			err = closedTemporalV8Inventory(cleaned, prior8, previous8.Previous, root)
		}
		oldAllRef = previous8.Manifest.AllOwners
		h.SeparateAdaptive = previous8.SeparateAdaptive
	}
	if err != nil {
		return nil, err
	}
	all, err := read(m.AllOwners)
	if err != nil {
		return nil, err
	}
	oldAll, err := read(oldAllRef)
	if err != nil {
		return nil, err
	}
	if err = closedTemporalAllOwners(all, oldAll, ledger, m.Schema); err != nil {
		return nil, err
	}
	operator, err := read(m.Operator)
	if err != nil {
		return nil, err
	}
	source := "9a0fd8c805e36ef36d64fee52d38da542a50f233"
	if m.Version == 9 {
		var pins []byte
		pins, err = read(m.Pins)
		if err == nil {
			source, err = closedV9PinsWitness(pins, m, h.Previous)
		}
		if err != nil {
			return nil, err
		}
	}
	if err = closedTemporalOperatorWitness(operator, m, h.Previous, h.Carry, h.NewUsage, source); err != nil {
		return nil, err
	}
	if h.NewUsage.KnownFullTokenLowerBound > int64(^uint64(0)>>1)-h.Previous.Usage.KnownFullTokenLowerBound {
		return nil, errors.New("inclusive closed V8 total overflow")
	}
	h.Day, h.Deadline = m.Day, m.Deadline
	h.Usage = consolidationV2AddUsage(h.Previous.Usage, h.NewUsage)
	h.Historical = h.Previous.Historical
	return h, nil
}

func closedTemporalV8Inventory(current, prior []byte, previous *temporalV5History, root string) error {
	if previous == nil || previous.SuccessorV8 == nil {
		return errors.New("validated V8 predecessor required")
	}
	cleaned, err := temporalClearCurrentLeaf(current, prior, "butler-issue85-postgres", "issue85_native_test", temporalV8Schema)
	if err != nil {
		return err
	}
	previous7 := previous.SuccessorV8
	prior7, err := consolidationV2Read(root, previous7.Manifest.Predecessor)
	if err != nil {
		return err
	}
	return closedTemporalV7Inventory(cleaned, prior7, previous7.Previous, root)
}

func closedTemporalOperatorWitness(raw []byte, m closedOwnerManifest, previous *temporalV5History, carry budget.Units, usage temporalV5CurrentUsage, source string) error {
	var o struct {
		Format, Source, Schema, Window string
		History                        string            `json:"prior_history_sha256"`
		Used                           budget.Units      `json:"used"`
		Prior                          budget.Units      `json:"labelled_prior_carry"`
		Artifacts                      map[string]string `json:"artifact_hashes"`
		Native                         int               `json:"new_stage_native_entries"`
		Known                          int               `json:"new_stage_native_known"`
		Unknown                        int               `json:"new_stage_native_unknown"`
		Missing                        int               `json:"new_stage_native_missing_total"`
		Full                           int64             `json:"new_stage_known_full_tokens_lower_bound"`
		Product                        int               `json:"new_stage_product_attempts"`
		Observed                       int               `json:"new_stage_product_observed"`
		Null                           int               `json:"new_stage_product_null"`
		Orphan                         int               `json:"new_stage_product_without_native_entry"`
		OldCause                       string            `json:"old89v2_failure_cause"`
		Operator                       struct {
			Added   *bool  `json:"added_to_experimental_carry"`
			Invoice string `json:"invoice_cost"`
		} `json:"operator_control_plane_handoff"`
	}
	if previous == nil || json.Unmarshal(raw, &o) != nil || o.Format != fmt.Sprintf("butler.temporal.closed-leaf.v%d", m.Version) || o.Source != source || o.Schema != m.Schema || o.Window != m.Window || o.History != m.Previous.SHA256 || o.Used != carry || o.Prior != previous.Carry || o.Native != usage.NativeEntries || o.Known != usage.NativeKnownUsage || o.Unknown != usage.NativeUnknownUsage || o.Missing != usage.NativeMissingTotal || o.Full != usage.KnownFullTokenLowerBound || o.Product != usage.ProductAttempts || o.Observed != usage.ProductObserved || o.Null != usage.ProductNULL || o.Orphan != usage.ProductWithoutNativeEntry || o.OldCause != "UNKNOWN; unchanged" || o.Operator.Added == nil || *o.Operator.Added || o.Operator.Invoice != "unobserved" {
		return errors.New("closed V8 operator accounting differs from corroborated original populations")
	}
	for name, ref := range map[string]consolidationHistoryRef{"full-ledger.json": m.Ledger, "all-original-outcomes.json": m.Outcomes, "closed-handback.json": m.Closure} {
		if o.Artifacts[name] != ref.SHA256 {
			return errors.New("closed V8 original digest differs from operator witness")
		}
	}
	if m.Version == 9 && o.Artifacts["frozen-pins.json"] != m.Pins.SHA256 {
		return errors.New("closed V9 pins differ from operator witness")
	}
	return nil
}
