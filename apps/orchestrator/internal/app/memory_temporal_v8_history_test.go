package app

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"time"
)

const temporalV8Schema = "memory_temporal_native_v8"
const temporalV8Window = "temporal-explicit-v8"

type temporalV8Manifest struct {
	Format, Owner, Day, Schema string
	Deadline                   time.Time
	Previous                   consolidationHistoryRef `json:"closed_v7_history"`
	Future                     struct {
		Schema  string `json:"schema"`
		Window  string `json:"window_id"`
		Present *bool  `json:"schema_present"`
		Table   *bool  `json:"receipt_table_present"`
		Count   *int   `json:"receipt_count"`
	} `json:"future_window"`
}

func loadTemporalV8History(path, expected string) (*temporalV5History, error) {
	raw, err := os.ReadFile(path)
	var m temporalV8Manifest
	if err != nil || len(raw) > 32*1024*1024 || len(expected) != 64 || experienceDigest(string(raw)) != expected || json.Unmarshal(raw, &m) != nil || m.Format != "butler.temporal.native-history.v4" || m.Owner != "owner" || m.Schema != temporalV8Schema {
		return nil, errors.New("immutable prospective V8 history required")
	}
	if m.Future.Schema != temporalV8Schema || m.Future.Window != temporalV8Window || m.Future.Present == nil || *m.Future.Present || m.Future.Table == nil || m.Future.Count == nil || *m.Future.Count != 0 {
		return nil, errors.New("fresh unconsumed V8 namespace receipt required")
	}
	root := filepath.Dir(path)
	if _, err = consolidationV2Read(root, m.Previous); err != nil {
		return nil, err
	}
	previous, err := loadClosedTemporalV7History(filepath.Join(root, m.Previous.Path), m.Previous.SHA256)
	if err != nil || m.Day != previous.Day || !m.Deadline.Equal(previous.Deadline) {
		return nil, errors.New("same-day recursively validated closed V7 lineage required")
	}
	return &temporalV5History{Hash: expected, Manifest: temporalV5HistoryManifest{Day: m.Day, Deadline: m.Deadline}, Carry: previous.Carry, Usage: previous.Usage, Historical: previous.Historical, SuccessorV8: previous}, nil
}

func temporalNativeFactsRevision(version int) string {
	switch version {
	case 6, 7:
		return "3"
	case 8:
		return "4"
	case 9:
		return "5"
	}
	return ""
}
