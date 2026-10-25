package main

import (
	"encoding/json"
	"github.com/butler/butler/internal/acceptance/effort"
	"os"
	"path/filepath"
	"testing"
)

func TestSummaryIncludesFailedSpendAndUnknownDenominators(t *testing.T) {
	dir := t.TempDir()
	s1 := slot{Case: "c1", Profile: "profile", Mode: effort.Baseline}
	s2 := slot{Case: "c2", Profile: "profile", Mode: effort.Baseline}
	f := manifest{Source: "frozen", Cases: []effort.Case{{ID: "c1"}, {ID: "c2"}}, Slots: []slot{s1, s2}}
	if e := writeJSON(filepath.Join(dir, "manifest.json"), f); e != nil {
		t.Fatal(e)
	}
	rows := `{"slot":"c1/profile/baseline/0","stage":"response_stream","status":"completed","output":"{}","total_tokens":100,"thinking_tokens":0}` + "\n" + `{"slot":"c2/profile/baseline/0","stage":"http_round_trip","status":"failed","total_tokens":50}` + "\n" + `{"slot":"c2/profile/baseline/0","stage":"http_round_trip","status":"failed"}` + "\n"
	if e := os.WriteFile(filepath.Join(dir, "attempts.jsonl"), []byte(rows), 0600); e != nil {
		t.Fatal(e)
	}
	rows = `{"slot":"c1/profile/baseline/0","milliseconds":100,"outcome":{"status":"completed","correct":true,"state":{"case_id":"c1"}}}` + "\n" + `{"slot":"c2/profile/baseline/0","milliseconds":900,"outcome":{"status":"failed","reason":"sample_failed","state":{"case_id":"c2"}}}` + "\n"
	if e := os.WriteFile(filepath.Join(dir, "outcomes.jsonl"), []byte(rows), 0600); e != nil {
		t.Fatal(e)
	}
	if e := summarize(dir); e != nil {
		t.Fatal(e)
	}
	raw, e := os.ReadFile(filepath.Join(dir, "summary.json"))
	if e != nil {
		t.Fatal(e)
	}
	var out struct {
		Arms []armSummary `json:"arms"`
	}
	if e = json.Unmarshal(raw, &out); e != nil {
		t.Fatal(e)
	}
	s := out.Arms[0]
	if s.Planned != 2 || s.Correct != 1 || s.Total != 150 || s.UnknownTotal != 1 || s.UnknownThinking != 2 || s.Attempts != 3 || s.HTTPStages != 3 || s.NativeFinals != 1 || s.P50 != 100 || s.P95 != 900 || s.KnownTokensPerCorrect == nil || *s.KnownTokensPerCorrect != 150 {
		t.Fatalf("%+v", s)
	}
}
