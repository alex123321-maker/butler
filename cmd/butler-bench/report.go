package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"slices"
)

func readRun(dir string) (Run, []Outcome, error) {
	var run Run
	load := func(path string, v any) error {
		raw, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		return json.Unmarshal(raw, v)
	}
	if err := load(filepath.Join(dir, "plan.json"), &run); err != nil {
		return run, nil, err
	}
	p := run.Plan
	if p.Version != version || p.Protocol != protocolHash() || run.PlanHash != digest(p) || !shaRE.MatchString(p.SetupHash) || !commitRE.MatchString(p.Commit) || !nonceRE.MatchString(run.ID) || run.Candidate.Provider == "" || run.Candidate.Model == "" || !run.Candidate.validEffort() {
		return run, nil, errors.New("invalid run identity or incompatible benchmark version")
	}
	baseline, _, err := makePlan(run.Setup, p.Seed, p.Group, p.Cases, p.Repetitions, p.Timeout)
	if err != nil || digest(baseline) != run.PlanHash || len(baseline.Slots) != len(p.Slots) {
		return run, nil, errors.New("incomplete planned matrix")
	}
	var rows []Outcome
	if _, err := caseArchive(dir, p); err != nil {
		return run, nil, err
	}
	for i, slot := range p.Slots {
		row := Outcome{Slot: slot, Status: "not_run"}
		err := load(rowFile(dir, "slot", i), &row)
		if errors.Is(err, os.ErrNotExist) {
			var started string
			if err := load(rowFile(dir, "started", i), &started); err == nil {
				if started != sessionID(run, i) {
					return run, nil, errors.New("invalid interrupted slot")
				}
				row.Status = "interrupted"
				row.Session = started
				for k := 0; k < slot.Steps; k++ {
					var t Turn
					err := load(turnFile(dir, i, k), &t)
					if errors.Is(err, os.ErrNotExist) {
						break
					}
					if err != nil {
						return run, nil, err
					}
					if t.Event != eventID(run, i, k) || t.Reason == "" || t.LatencyMS < 0 {
						return run, nil, errors.New("invalid interrupted turn")
					}
					row.Turns = append(row.Turns, t)
					row.MS += t.LatencyMS
				}
			} else if !errors.Is(err, os.ErrNotExist) {
				return run, nil, err
			}
		} else if err != nil {
			return run, nil, err
		} else if err := validateRow(run, i, row); err != nil {
			return run, nil, err
		}
		for k, turn := range row.Turns {
			var retained Turn
			if err := load(turnFile(dir, i, k), &retained); err != nil || digest(retained) != digest(turn) {
				return run, nil, errors.New("durable turn and result differ")
			}
			if err := verifyProfileEvidence(filepath.Join(dir, turn.Event), run.Candidate, turn); err != nil {
				return run, nil, err
			}
			if err := verifyFiles(filepath.Join(dir, turn.Event), turn.EvidenceSHA); err != nil {
				return run, nil, err
			}
		}
		rows = append(rows, row)
	}
	entries, err := filepath.Glob(filepath.Join(dir, "slot-*.json"))
	if err != nil {
		return run, nil, err
	}
	valid := map[string]bool{}
	for i := range p.Slots {
		valid[rowFile(dir, "slot", i)] = true
	}
	for _, entry := range entries {
		if !valid[entry] {
			return run, nil, errors.New("unexpected result slot")
		}
	}
	return run, rows, nil
}

func validateRow(run Run, i int, row Outcome) error {
	fail := fmt.Errorf("invalid result for slot %d", i+1)
	slot := run.Plan.Slots[i]
	if row.Slot != slot || row.Session != sessionID(run, i) || row.MS < 0 || len(row.Turns) > slot.Steps {
		return fail
	}
	switch row.Status {
	case "unavailable":
		if row.Reason == "" || len(row.Turns) != 0 {
			return fail
		}
		return nil
	case "pass":
		if len(row.Turns) != slot.Steps || row.Reason != "" {
			return fail
		}
	case "fail", "error":
		if len(row.Turns) == 0 || row.Reason == "pass" || row.Reason == "" || row.Status != classify(row.Reason) || row.Turns[len(row.Turns)-1].Reason != row.Reason {
			return fail
		}
	default:
		return fail
	}
	seen := map[string]bool{}
	for k, t := range row.Turns {
		if t.Event != eventID(run, i, k) || t.Reason == "" || t.LatencyMS < 0 || t.ModelCalls != nil && *t.ModelCalls < 0 || t.InputTokens != nil && *t.InputTokens < 0 || t.OutputTokens != nil && *t.OutputTokens < 0 {
			return fail
		}
		if t.Run != "" && seen[t.Run] {
			return fail
		}
		seen[t.Run] = true
		if k < len(row.Turns)-1 && t.Reason != "pass" {
			return fail
		}
		if t.Reason == "pass" {
			qualified := t.WebReady && t.Status == "completed" && shaRE.MatchString(t.AnswerHash)
			if slot.ID == "wait-for-tab-choice" {
				qualified = t.GuardedWait && t.Status == "waiting_for_user" && t.AnswerHash == ""
			}
			if !qualified || !t.MatchedModel || t.Input == "" || t.Run == "" || t.ModelCalls == nil || *t.ModelCalls < 1 {
				return fail
			}
		} else if row.Status == "pass" {
			return fail
		}
	}
	return nil
}

type Metrics struct {
	Timings      map[string]distribution `json:"step_timings_us,omitempty"`
	Planned      int                     `json:"planned"`
	Passed       int                     `json:"passed"`
	Percent      float64                 `json:"pass_percent_of_planned"`
	States       map[string]int          `json:"states"`
	Cases        int                     `json:"distinct_cases"`
	Consistent   int                     `json:"cases_passing_all_repeats"`
	Timed        int                     `json:"timed_slots"`
	P50          *int64                  `json:"p50_elapsed_ms"`
	P95          *int64                  `json:"p95_elapsed_ms"`
	ToolCalls    int                     `json:"observed_tool_calls"`
	ModelCalls   int                     `json:"observed_model_attempts"`
	KnownInput   int64                   `json:"known_input_tokens"`
	KnownOutput  int64                   `json:"known_output_tokens"`
	UnknownUsage int                     `json:"observed_turns_without_full_usage"`
}

type distribution struct {
	Measured int    `json:"measured"`
	Unknown  int    `json:"unknown"`
	P50      *int64 `json:"p50"`
	P95      *int64 `json:"p95"`
}

func percentiles(observations []*int64) distribution {
	var values []int64
	for _, v := range observations {
		if v != nil {
			values = append(values, *v)
		}
	}
	slices.Sort(values)
	d := distribution{Measured: len(values), Unknown: len(observations) - len(values)}
	if n := len(values); n > 0 {
		d.P50 = &values[(n*50+99)/100-1]
		d.P95 = &values[(n*95+99)/100-1]
	}
	return d
}

func metrics(rows []Outcome) Metrics {
	m := Metrics{Planned: len(rows), States: map[string]int{}}
	var times []int64
	samples := map[string][]*int64{}
	allPassed := map[string]bool{}
	for _, r := range rows {
		m.States[r.Status]++
		if _, ok := allPassed[r.Slot.ID]; !ok {
			allPassed[r.Slot.ID] = true
		}
		if r.Status == "pass" {
			m.Passed++
		} else {
			allPassed[r.Slot.ID] = false
		}
		if len(r.Turns) > 0 {
			times = append(times, r.MS)
		}
		for _, t := range r.Turns {
			samples["ingress_http"] = append(samples["ingress_http"], t.HTTPUS)
			for _, s := range t.Durations {
				samples[s.Stage] = append(samples[s.Stage], s.US)
			}
			m.ToolCalls += len(t.Tools)
			if t.ModelCalls != nil {
				m.ModelCalls += *t.ModelCalls
			}
			if t.InputTokens == nil || t.OutputTokens == nil {
				m.UnknownUsage++
			}
			m.KnownInput += t.KnownInput
			m.KnownOutput += t.KnownOutput
		}
	}
	m.Cases = len(allPassed)
	m.Timings = map[string]distribution{}
	for k, v := range samples {
		m.Timings[k] = percentiles(v)
	}
	for _, ok := range allPassed {
		if ok {
			m.Consistent++
		}
	}
	if m.Planned > 0 {
		m.Percent = float64(m.Passed) * 100 / float64(m.Planned)
	}
	slices.Sort(times)
	m.Timed = len(times)
	if m.Timed > 0 {
		m.P50 = &times[(m.Timed*50+99)/100-1]
		m.P95 = &times[(m.Timed*95+99)/100-1]
	}
	return m
}

type Summary struct {
	Candidate    Candidate          `json:"candidate"`
	PlanHash     string             `json:"plan_sha256"`
	Total        Metrics            `json:"total"`
	Groups       map[string]Metrics `json:"by_group"`
	Features     map[string]Metrics `json:"by_feature"`
	Reasons      map[string]int     `json:"failure_reasons"`
	MacroPercent float64            `json:"mean_group_pass_percent"`
}

func summarize(r Run, rows []Outcome) Summary {
	out := Summary{Candidate: r.Candidate, PlanHash: r.PlanHash, Total: metrics(rows), Groups: map[string]Metrics{}, Features: map[string]Metrics{}, Reasons: map[string]int{}}
	groups, features := map[string][]Outcome{}, map[string][]Outcome{}
	for _, row := range rows {
		groups[row.Slot.Group] = append(groups[row.Slot.Group], row)
		features[row.Slot.Feature] = append(features[row.Slot.Feature], row)
		if row.Reason != "" {
			out.Reasons[row.Reason]++
		}
	}
	for k, v := range groups {
		out.Groups[k] = metrics(v)
		out.MacroPercent += out.Groups[k].Percent / float64(len(groups))
	}
	for k, v := range features {
		out.Features[k] = metrics(v)
	}
	return out
}

type Pair struct {
	Case   string `json:"case"`
	Repeat int    `json:"repeat"`
	A      string `json:"a"`
	B      string `json:"b"`
}
type Comparison struct {
	A           Summary `json:"a"`
	B           Summary `json:"b"`
	WinsA       int     `json:"a_only_pass"`
	WinsB       int     `json:"b_only_pass"`
	Both        int     `json:"both_pass"`
	Neither     int     `json:"both_fail"`
	Unscored    int     `json:"unpaired_or_unattributed"`
	Differences []Pair  `json:"differences"`
}

func attributable(row Outcome) bool {
	if len(row.Turns) == 0 || row.Status != "pass" && row.Status != "fail" {
		return false
	}
	if row.Status == "fail" && row.Reason != "wrong_answer" && row.Reason != "unexpected_tool" && row.Reason != "missing_tool_evidence" {
		return false
	}
	for _, t := range row.Turns {
		if !t.MatchedModel || t.ModelCalls == nil || *t.ModelCalls == 0 {
			return false
		}
	}
	return true
}
func compare(a Run, ar []Outcome, b Run, br []Outcome) (Comparison, error) {
	var out Comparison
	if a.PlanHash != b.PlanHash || a.ID == b.ID || a.Candidate == b.Candidate || len(ar) != len(br) {
		return out, errors.New("two different model runs with identical cases, seed, repeats, setup, time limit and protocol required")
	}
	out.A, out.B = summarize(a, ar), summarize(b, br)
	for i, x := range ar {
		y := br[i]
		if x.Slot != y.Slot {
			return out, errors.New("unpaired slot")
		}
		switch {
		case !attributable(x) || !attributable(y):
			out.Unscored++
		case x.Status == "pass" && y.Status == "pass":
			out.Both++
		case x.Status == "pass":
			out.WinsA++
		case y.Status == "pass":
			out.WinsB++
		default:
			out.Neither++
		}
		if x.Status != y.Status || x.Reason != y.Reason {
			out.Differences = append(out.Differences, Pair{x.Slot.ID, x.Slot.Repeat, x.Status + " " + x.Reason, y.Status + " " + y.Reason})
		}
	}
	return out, nil
}
