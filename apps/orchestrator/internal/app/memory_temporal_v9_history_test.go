package app

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"time"
)

const temporalV9Schema = "memory_temporal_native_v9"
const temporalV9Window = "temporal-explicit-v9"

type temporalV9Manifest struct {
	Format, Owner, Day, Schema string
	Deadline                   time.Time
	Previous                   consolidationHistoryRef `json:"latest_closed_history"`
	Future                     struct {
		Schema  string `json:"schema"`
		Window  string `json:"window_id"`
		Present *bool  `json:"schema_present"`
		Table   *bool  `json:"receipt_table_present"`
		Count   *int   `json:"receipt_count"`
	} `json:"future_window"`
}

func loadTemporalV9History(path, expected string) (*temporalV5History, error) {
	raw, err := os.ReadFile(path)
	var m temporalV9Manifest
	if err != nil || len(raw) > 32*1024*1024 || len(expected) != 64 || experienceDigest(string(raw)) != expected || json.Unmarshal(raw, &m) != nil || m.Format != "butler.temporal.native-history.v5" || m.Owner != "owner" || m.Schema != temporalV9Schema {
		return nil, errors.New("immutable prospective V9 history required")
	}
	if m.Future.Schema != temporalV9Schema || m.Future.Window != temporalV9Window || m.Future.Present == nil || *m.Future.Present || m.Future.Table == nil || m.Future.Count == nil || *m.Future.Count != 0 {
		return nil, errors.New("fresh unconsumed V9 namespace receipt required")
	}
	root := filepath.Dir(path)
	if _, err = consolidationV2Read(root, m.Previous); err != nil {
		return nil, err
	}
	previous, err := loadLatestClosedOwnerHistory(filepath.Join(root, m.Previous.Path), m.Previous.SHA256)
	if err != nil || previous.Manifest.Version != 8 || previous.Day != m.Day || !previous.Deadline.Equal(m.Deadline) {
		return nil, errors.New("same-day original closed V8 lineage required")
	}
	return &temporalV5History{Hash: expected, Manifest: temporalV5HistoryManifest{Day: m.Day, Deadline: m.Deadline}, Carry: previous.Carry, Usage: previous.Usage, Historical: previous.Historical, SuccessorV9: previous}, nil
}
