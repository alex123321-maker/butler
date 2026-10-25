package main

import (
	"bufio"
	"encoding/json"
	"errors"
	"math"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/butler/butler/internal/acceptance/effort"
)

type attemptExport struct {
	Slot           string  `json:"slot"`
	Status         string  `json:"status"`
	Stage          string  `json:"stage"`
	Reason         string  `json:"reason"`
	Input          string  `json:"input"`
	Output         *string `json:"output"`
	InputTokens    *int64  `json:"input_tokens"`
	OutputTokens   *int64  `json:"output_tokens"`
	TotalTokens    *int64  `json:"total_tokens"`
	ThinkingTokens *int64  `json:"thinking_tokens"`
	ModelVersion   *string `json:"model_version"`
}
type outcomeExport struct {
	Slot         string         `json:"slot"`
	Outcome      effort.Outcome `json:"outcome"`
	Milliseconds int64          `json:"milliseconds"`
}
type armSummary struct {
	Arm                   string         `json:"arm"`
	Planned               int            `json:"planned"`
	Observed              int            `json:"observed"`
	Completed             int            `json:"terminal_candidates"`
	Correct               int            `json:"correct_fixed_facets"`
	CorrectClarifications int            `json:"correct_clarifications"`
	CorrectUnavailability int            `json:"correct_unavailability"`
	Attempts              int            `json:"admitted_attempts"`
	HTTPStages            int            `json:"http_or_response_stages"`
	CompletedGenerations  int            `json:"valid_native_json_replies"`
	NativeFinals          int            `json:"complete_native_text_finals"`
	FixtureReads          int            `json:"fixture_reads"`
	SnapshotBytes         int64          `json:"retained_snapshot_bytes_not_full_wire"`
	Input                 int64          `json:"known_input_tokens"`
	Output                int64          `json:"known_output_tokens"`
	Total                 int64          `json:"known_total_tokens"`
	Thinking              int64          `json:"known_thinking_tokens"`
	UnknownTotal          int            `json:"total_tokens_unavailable_attempts"`
	UnknownThinking       int            `json:"thinking_tokens_unavailable_attempts"`
	KnownTokensPerCorrect *float64       `json:"known_tokens_per_correct_response_lower_bound"`
	P50                   int64          `json:"all_slot_p50_ms_nearest_rank"`
	P95                   int64          `json:"all_slot_p95_ms_nearest_rank"`
	Failures              map[string]int `json:"slot_failures"`
	Versions              []string       `json:"observed_model_versions"`
	times                 []int64
	versions              map[string]bool
}
type blindReview struct {
	ID              string          `json:"candidate_id"`
	CurrentGoal     string          `json:"current_goal"`
	GoalVersion     int             `json:"goal_version"`
	Case            effort.Case     `json:"fixture"`
	ObservedSources []effort.Source `json:"observed_sources"`
	Candidate       *effort.Reply   `json:"candidate"`
	Status          string          `json:"slot_status"`
}

func readLines[T any](path string) ([]T, error) {
	f, e := os.Open(path)
	if e != nil {
		return nil, e
	}
	defer f.Close()
	s := bufio.NewScanner(f)
	s.Buffer(make([]byte, 4096), 2*1024*1024)
	var out []T
	for s.Scan() {
		var row T
		if json.Unmarshal(s.Bytes(), &row) != nil {
			return nil, errors.New("invalid research export")
		}
		out = append(out, row)
		if len(out) > 10000 {
			return nil, errors.New("oversized research export")
		}
	}
	return out, s.Err()
}
func armID(slot string) string {
	p := strings.Split(slot, "/")
	if len(p) != 4 {
		return "invalid"
	}
	return p[1] + "/" + p[2]
}
func percentile(v []int64, p float64) int64 {
	if len(v) == 0 {
		return 0
	}
	sort.Slice(v, func(i, j int) bool { return v[i] < v[j] })
	return v[int(math.Ceil(p*float64(len(v))))-1]
}
func writeJSON(path string, v any) error {
	b, e := json.MarshalIndent(v, "", "  ")
	if e != nil {
		return e
	}
	return os.WriteFile(path, append(b, '\n'), 0600)
}

func summarize(dir string) error {
	raw, e := os.ReadFile(filepath.Join(dir, "manifest.json"))
	if e != nil {
		return e
	}
	var m manifest
	if json.Unmarshal(raw, &m) != nil {
		return errors.New("invalid manifest")
	}
	attempts, e := readLines[attemptExport](filepath.Join(dir, "attempts.jsonl"))
	if e != nil {
		return e
	}
	outcomes, e := readLines[outcomeExport](filepath.Join(dir, "outcomes.jsonl"))
	if e != nil {
		return e
	}
	arms := map[string]*armSummary{}
	slots := map[string]bool{}
	for _, s := range m.Slots {
		key := armID(s.ID())
		if arms[key] == nil {
			arms[key] = &armSummary{Arm: key, Failures: map[string]int{}, versions: map[string]bool{}}
		}
		arms[key].Planned++
		slots[s.ID()] = true
	}
	for _, a := range attempts {
		if !slots[a.Slot] {
			return errors.New("attempt outside frozen matrix")
		}
		s := arms[armID(a.Slot)]
		s.Attempts++
		s.SnapshotBytes += int64(len(a.Input))
		if a.Stage == "http_round_trip" || a.Stage == "response_stream" {
			s.HTTPStages++
		}
		if a.Status == "completed" {
			s.CompletedGenerations++
		}
		if a.Output != nil && strings.TrimSpace(*a.Output) != "" {
			s.NativeFinals++
		}
		if a.InputTokens != nil {
			s.Input += *a.InputTokens
		}
		if a.OutputTokens != nil {
			s.Output += *a.OutputTokens
		}
		if a.TotalTokens != nil {
			s.Total += *a.TotalTokens
		} else {
			s.UnknownTotal++
		}
		if a.ThinkingTokens != nil {
			s.Thinking += *a.ThinkingTokens
		} else {
			s.UnknownThinking++
		}
		if a.ModelVersion != nil {
			s.versions[*a.ModelVersion] = true
		}
	}
	seen := map[string]bool{}
	reviews := []blindReview{}
	mapping := map[string]string{}
	for _, o := range outcomes {
		if !slots[o.Slot] || seen[o.Slot] {
			return errors.New("outcome outside matrix or duplicated")
		}
		seen[o.Slot] = true
		s := arms[armID(o.Slot)]
		s.Observed++
		s.times = append(s.times, o.Milliseconds)
		s.FixtureReads += len(o.Outcome.State.Actions)
		if o.Outcome.Status == "completed" {
			s.Completed++
		} else {
			s.Failures[o.Outcome.Reason]++
		}
		if o.Outcome.Correct {
			s.Correct++
			if c := o.Outcome.State.Candidate; c != nil {
				if c.Decision == "clarify" {
					s.CorrectClarifications++
				}
				if c.Decision == "unavailable" {
					s.CorrectUnavailability++
				}
			}
		}
		id := digest([]byte(m.Source + "/review/" + o.Slot))[:16]
		mapping[id] = o.Slot
		var c effort.Case
		for _, x := range m.Cases {
			if x.ID == o.Outcome.State.CaseID {
				c = x
			}
		}
		if m.Protocol == "issue97-controlled-evidence-v1" {
			parts := strings.Split(o.Slot, "/")
			if len(parts) == 4 && parts[2] != "shared_draft" {
				c, _ = evidenceReview(c, parts[2] == "review_fresh")
			}
		}
		reviews = append(reviews, blindReview{ID: id, CurrentGoal: o.Outcome.State.Goal, GoalVersion: o.Outcome.State.GoalVersion, Case: c, ObservedSources: o.Outcome.State.Sources, Candidate: o.Outcome.State.Candidate, Status: o.Outcome.Status})
	}
	var summary []armSummary
	for _, s := range arms {
		s.P50 = percentile(s.times, .5)
		s.P95 = percentile(s.times, .95)
		if s.Correct > 0 {
			v := float64(s.Total) / float64(s.Correct)
			s.KnownTokensPerCorrect = &v
		}
		for v := range s.versions {
			s.Versions = append(s.Versions, v)
		}
		sort.Strings(s.Versions)
		summary = append(summary, *s)
	}
	sort.Slice(summary, func(i, j int) bool { return summary[i].Arm < summary[j].Arm })
	sort.Slice(reviews, func(i, j int) bool { return reviews[i].ID < reviews[j].ID })
	if e = writeJSON(filepath.Join(dir, "summary.json"), struct {
		Protocol          string `json:"protocol"`
		Planned, Observed int
		Arms              []armSummary `json:"arms"`
		CurrencyCost      any          `json:"currency_cost"`
		QuotaCost         any          `json:"quota_cost"`
		HumanRaters       int          `json:"human_raters"`
	}{m.Protocol, len(m.Slots), len(outcomes), summary, nil, nil, 0}); e != nil {
		return e
	}
	if e = writeJSON(filepath.Join(dir, "review-blinded.json"), reviews); e != nil {
		return e
	}
	return writeJSON(filepath.Join(dir, "review-mapping.json"), mapping)
}
