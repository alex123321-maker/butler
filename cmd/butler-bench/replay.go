package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
)

// Exact snapshot used by the online grader. Kept apart from the later timeline
// because a timeout or a new durable state can change what a later query sees.
type observation struct {
	Status      string `json:"input_state"`
	Run         string `json:"run"`
	Answer      string `json:"answer"`
	AnswerState string `json:"answer_state"`
	AnswerHash  string `json:"answer_sha256"`
	WebReady    bool   `json:"web_result_ready"`
	GuardedWait bool   `json:"guarded_wait"`
	Attempts    int    `json:"model_attempts"`
	Matched     bool   `json:"matched_model"`
	Calls       []call `json:"calls"`
}

func observationFor(s snapshot) observation {
	return observation{s.status, s.run, s.answer, s.answerState, s.answerHash, s.webReady, s.guardedWait, s.attempts, s.matched, s.calls}
}
func (o observation) snapshot() snapshot {
	return snapshot{status: o.Status, run: o.Run, answer: o.Answer, answerState: o.AnswerState, answerHash: o.AnswerHash, webReady: o.WebReady, guardedWait: o.GuardedWait, attempts: o.Attempts, matched: o.Matched, calls: o.Calls}
}
func caseArchive(dir string, p Plan) ([]Case, error) {
	raw, err := os.ReadFile(filepath.Join(dir, "cases.json"))
	if err != nil {
		return nil, err
	}
	var cases []Case
	if json.Unmarshal(raw, &cases) != nil || len(cases) != len(p.Slots) {
		return nil, errors.New("incomplete case archive")
	}
	for i, c := range cases {
		s := p.Slots[i]
		if digest(c) != s.CaseHash || c.ID != s.ID || len(c.Steps) != s.Steps || c.Group != s.Group || c.Feature != s.Feature {
			return nil, errors.New("archived case does not match frozen plan")
		}
	}
	return cases, nil
}

type replayResult struct {
	Plan          string `json:"plan_sha256"`
	PlannedSlots  int    `json:"planned_slots"`
	VerifiedTurns int    `json:"verified_evidence_bundles"`
	Regraded      int    `json:"regraded_turns"`
	Unavailable   int    `json:"turns_without_grading_snapshot"`
	NoTurns       int    `json:"slots_without_turns"`
}

func replay(dir string, r Run, rows []Outcome) (replayResult, error) {
	out := replayResult{Plan: r.PlanHash, PlannedSlots: len(rows)}
	work, err := caseArchive(dir, r.Plan)
	if err != nil {
		return out, err
	}
	for i, row := range rows {
		if len(row.Turns) == 0 {
			out.NoTurns++
		}
		for k, t := range row.Turns {
			out.VerifiedTurns++
			raw, err := os.ReadFile(filepath.Join(dir, t.Event, "grading.json"))
			if errors.Is(err, os.ErrNotExist) && t.Reason != "pass" && t.Reason != "wrong_answer" {
				out.Unavailable++
				continue
			}
			if err != nil {
				return out, err
			}
			var o observation
			if json.Unmarshal(raw, &o) != nil || o.Run != t.Run || o.Status != t.Status || o.Matched != t.MatchedModel || t.ModelCalls != nil && o.Attempts != *t.ModelCalls {
				return out, errors.New("grading snapshot identity mismatch")
			}
			want := t.Reason
			if t.ObservedResult != "" {
				want = t.ObservedResult
			}
			if got := verdict(o.snapshot(), work[i].Steps[k]); got != want {
				return out, fmt.Errorf("replay divergence for %s: %s vs %s", t.Event, got, want)
			}
			out.Regraded++
		}
	}
	return out, nil
}
