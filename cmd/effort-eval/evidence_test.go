package main

import (
	"encoding/json"
	"github.com/butler/butler/internal/acceptance/effort"
	"strings"
	"testing"
)

func TestControlledEvidencePairsShareDraftAndHideFutureSource(t *testing.T) {
	f := evidenceManifest("frozen", modelInventory{})
	if len(f.Slots) != 18 || f.MaxTaskCalls != 1 {
		t.Fatal(f)
	}
	for _, c := range f.Cases {
		draft := effort.Reply{GoalVersion: 1, Decision: "answer", Value: c.Expected.Value, Answer: "draft"}
		initial := c
		initial.Available = nil
		b, _ := effort.ModelInput(initial, effort.State{Sources: initial.Initial})
		if strings.Contains(string(b), "verified_update_t1") || strings.Contains(string(b), evidenceTruth(c.ID)) {
			t.Fatal("future source leaked")
		}
		for _, fresh := range []bool{false, true} {
			r, decorate := evidenceReview(c, fresh)
			s := decorate(effort.State{Goal: c.Goal, GoalVersion: 1, Sources: r.Initial}, &draft)
			if s.Candidate != &draft || s.Goal != c.Goal || s.GoalVersion != 1 {
				t.Fatal("pair changed its shared draft/goal")
			}
			b, _ = json.Marshal(s)
			got := strings.Contains(string(b), "verified_update_t1")
			if got != fresh || len(s.Actions) != map[bool]int{false: 0, true: 1}[fresh] {
				t.Fatal("evidence treatment mismatch")
			}
			if fresh && r.Expected.Value != evidenceTruth(c.ID) || !fresh && r.Expected.Decision != "unavailable" {
				t.Fatal("wrong fixed rubric")
			}
		}
	}
}
