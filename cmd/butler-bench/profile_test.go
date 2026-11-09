package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/butler/butler/internal/benchtrace"
	"github.com/butler/butler/internal/transport"
)

func TestProfileComparisonReadsNativeEvidence(t *testing.T) {
	d := t.TempDir()
	c := Candidate{Provider: "openai", Model: "gpt-6-luna", Effort: "low"}
	n := 1
	v := Turn{Run: "r", Reason: "pass", ModelCalls: &n}
	name := benchtrace.Filename("r", "op")
	if err := writeJSON(filepath.Join(d, "model-timing.json"), []timing{{Operation: "op", Trace: name}}); err != nil {
		t.Fatal(err)
	}
	f, err := os.Create(filepath.Join(d, name))
	if err != nil {
		t.Fatal(err)
	}
	for _, r := range []benchtrace.Record{
		{Kind: "request", Run: "r", Operation: "op", Provider: c.Provider, Model: c.Model, Request: &benchtrace.Request{Command: transport.CommandTypeStartRun}},
		{Kind: "event", ModelVersion: &c.Model, ReasoningEffort: "low"},
		{Kind: "end", Outcome: "completed"},
	} {
		r.At = time.Now().UTC()
		if err = json.NewEncoder(f).Encode(r); err != nil {
			t.Fatal(err)
		}
	}
	f.Close()
	if err = verifyProfileEvidence(d, c, v); err != nil {
		t.Fatal(err)
	}
	c.Effort = "max"
	if err = verifyProfileEvidence(d, c, v); err == nil {
		t.Fatal("relabeled native profile accepted")
	}
}

func TestReviewedSkillDurableEvidence(t *testing.T) {
	digest := "88452b40ba1c4d205eb8ef08aab39f69d2b5ea4da7c4a98285866e9955303035"
	s := Step{Rule: "skill", Answer: "1.2.7", Witness: digest, Allowed: []string{"skills.search", "skills.load"}}
	calls := []call{
		{Name: "skills.search", Status: "completed", Result: json.RawMessage(`{"skill_digests":["` + digest + `"],"reload_current_data":true}`)},
		{Name: "skills.load", Status: "completed", Args: json.RawMessage(`{"digest":"` + digest + `"}`), Result: json.RawMessage(`{"digest":"` + digest + `","reload_current_data":true}`)},
	}
	if grade(s, s.Answer, calls) != "pass" || grade(s, "wrong", calls) == "pass" || grade(s, s.Answer, calls[1:]) == "pass" {
		t.Fatal("incorrect grading of durable reviewed skill evidence")
	}
}
