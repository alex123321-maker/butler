package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestFinalAuditPreservesUnknownAndKnownZero(t *testing.T) {
	fields := map[string]any{"ordinal": 269, "context_id": "shadow-observation", "outcome": "transport_error", "input_tokens": nil, "output_tokens": nil, "total_tokens": nil, "collector_stage": "provider_start", "diagnostic_code": "internal_transport_error", "encoding_reason": "no_visible_final", "final_bytes": nil, "final_hash": nil, "raw_retained": false, "retention_ok": true, "model_version": nil, "finish_reason": nil, "provider_stage": nil, "final_count": 0, "terminal_count": 0, "lifecycle_clean": false}
	raw, _ := json.Marshal(fields)
	var unknown diagnostic
	if json.Unmarshal(raw, &unknown) != nil || unknown.Input != nil || unknown.Output != nil || unknown.Total != nil || unknown.Version != nil {
		t.Fatal("explicit unknown converted or denied")
	}
	fields["input_tokens"], fields["output_tokens"], fields["total_tokens"] = 0, 0, 0
	raw, _ = json.Marshal(fields)
	var zero diagnostic
	if json.Unmarshal(raw, &zero) != nil || zero.Input == nil || zero.Total == nil || *zero.Input != 0 || *zero.Total != 0 {
		t.Fatal("known zero lost")
	}
	for _, mode := range []string{"omission", "unknown", "duplicate"} {
		t.Run(mode, func(t *testing.T) {
			var changed map[string]any
			if json.Unmarshal(raw, &changed) != nil {
				t.Fatal("fixture denied")
			}
			switch mode {
			case "omission":
				delete(changed, "model_version")
			case "unknown":
				changed["provider_error"] = "SYNTHETIC_PRIVATE_TEXT"
			}
			encoded, _ := json.Marshal(changed)
			if mode == "duplicate" {
				encoded = []byte(strings.TrimSuffix(string(encoded), "}") + `,"input_tokens":17}`)
			}
			var rejected diagnostic
			if json.Unmarshal(encoded, &rejected) == nil {
				t.Fatal("changed diagnostic scope accepted")
			}
		})
	}
}

func TestFinalAuditChecksExactPrivateBytesAndEncoding(t *testing.T) {
	for _, tc := range []struct{ text, reason string }{
		{`{"refs":[]}`, "json_object"}, {`{"refs":`, "json_syntax"}, {`[]`, "non_object"},
		{`{"refs":[],"r\u0065fs":[]}`, "duplicate_key"}, {string([]byte{0xff}), "invalid_utf8"},
		{strings.Repeat("x", 8193), "visible_bound"},
	} {
		if encodingReason([]byte(tc.text)) != tc.reason {
			t.Fatal("exact encoding category changed")
		}
	}
	path := filepath.Join(t.TempDir(), "final-269.raw")
	if os.WriteFile(path, []byte(`{"refs":[]}`), 0600) != nil {
		t.Fatal("private fixture denied")
	}
	raw, err := readRaw(path)
	if err != nil || string(raw) != `{"refs":[]}` {
		t.Fatal("original bytes changed")
	}
	if os.Chmod(path, 0644) != nil {
		t.Fatal("mode fixture denied")
	}
	if _, err = readRaw(path); err == nil {
		t.Fatal("public raw file accepted")
	}
}
