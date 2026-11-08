package main

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"embed"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"runtime"
	"slices"
	"strings"
	"time"
)

const version = "butler-bench/1"

// Include the evaluator and runner in the comparison identity.
//
//go:embed cases.go discovery.tsv grade.go live.go matrix.go report.go evidence.go timeline.sql replay.go
var protocol embed.FS

func digest(v any) string {
	raw, err := json.Marshal(v)
	if err != nil {
		panic(err)
	}
	sum := sha256.Sum256(raw)
	return hex.EncodeToString(sum[:])
}
func protocolHash() string {
	h := sha256.New()
	files, _ := protocol.ReadDir(".")
	for _, f := range files {
		raw, _ := protocol.ReadFile(f.Name())
		h.Write([]byte(f.Name() + "\x00"))
		h.Write(raw)
	}
	return hex.EncodeToString(h.Sum(nil))
}

type Slot struct {
	ID       string  `json:"case"`
	Group    string  `json:"group"`
	Feature  string  `json:"feature"`
	Repeat   int     `json:"repeat"`
	CaseHash string  `json:"case_sha256"`
	Steps    int     `json:"steps"`
	Binding  Binding `json:"binding,omitzero"`
}
type Plan struct {
	Version     string        `json:"version"`
	Protocol    string        `json:"protocol_sha256"`
	SetupHash   string        `json:"setup_sha256"`
	Commit      string        `json:"butler_commit"`
	Seed        string        `json:"seed"`
	Group       string        `json:"group,omitempty"`
	Cases       string        `json:"selected_cases,omitempty"`
	Repetitions int           `json:"repetitions"`
	Timeout     time.Duration `json:"slot_timeout_ns"`
	Slots       []Slot        `json:"slots"`
}
type Run struct {
	Setup     Setup     `json:"lab"`
	Runner    string    `json:"runner"`
	Started   time.Time `json:"created_utc"`
	Plan      Plan      `json:"plan"`
	PlanHash  string    `json:"plan_sha256"`
	Candidate Candidate `json:"candidate"`
	ID        string    `json:"run_id"`
}
type Outcome struct {
	Slot    Slot   `json:"slot"`
	Session string `json:"session,omitempty"`
	Status  string `json:"status"`
	Reason  string `json:"reason,omitempty"`
	Turns   []Turn `json:"turns,omitempty"`
	MS      int64  `json:"elapsed_ms"`
	WallMS  int64  `json:"wall_ms_including_collection"`
}

func makePlan(setup Setup, seed, group, ids string, repeats int, timeout time.Duration) (Plan, []Case, error) {
	p := Plan{Version: version, Protocol: protocolHash(), SetupHash: digest(setup), Commit: setup.Commit, Seed: seed, Timeout: timeout}
	if seed == "" || repeats < 1 || repeats > 100 || timeout <= 0 {
		return p, nil, errors.New("invalid seed, repetitions (1 to 100) or time limit")
	}
	if group == "" && ids == "" {
		group = "reasoning"
	}
	p.Group, p.Cases, p.Repetitions = group, ids, repeats
	selected := map[string]bool{}
	if ids != "" {
		for _, id := range strings.Split(ids, ",") {
			selected[strings.TrimSpace(id)] = true
		}
	}
	found := map[string]bool{}
	usedSessions := map[string]bool{}
	var work []Case
	for rep := 1; rep <= repeats; rep++ {
		var block []Case
		for _, c := range cases(seed, rep, setup) {
			if group != "" && group != "all" && c.Group != group || len(selected) > 0 && !selected[c.ID] {
				continue
			}
			found[c.ID] = true
			block = append(block, c)
		}
		// Stable shuffled blocks prevent always warming up on the same feature.
		slices.SortFunc(block, func(a, b Case) int {
			return strings.Compare(digest([]any{seed, rep, a.ID}), digest([]any{seed, rep, b.ID}))
		})
		for _, c := range block {
			binding, assigned := setup.Bindings[fmt.Sprintf("%s/%d", c.ID, rep)]
			if assigned {
				if usedSessions[binding.Session] || binding.Session == "" || !taskRE.MatchString(binding.Task) {
					return p, nil, fmt.Errorf("invalid or reused task/session binding for %s/%d", c.ID, rep)
				}
				usedSessions[binding.Session] = true
			}
			work = append(work, c)
			p.Slots = append(p.Slots, Slot{c.ID, c.Group, c.Feature, rep, digest(c), len(c.Steps), binding})
		}
	}
	for id := range selected {
		if !found[id] {
			return p, nil, fmt.Errorf("unknown or filtered case %q", id)
		}
	}
	if len(work) == 0 {
		return p, nil, errors.New("empty benchmark selection")
	}
	return p, work, nil
}

func writeJSON(path string, v any) error {
	b, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		return err
	}
	b = append(b, '\n')
	if path == "" || path == "-" {
		_, err = os.Stdout.Write(b)
		return err
	}
	f, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0600)
	if err != nil {
		return err
	}
	_, err = f.Write(b)
	if err == nil {
		err = f.Sync()
	}
	return errors.Join(err, f.Close())
}

func sessionID(r Run, n int) string {
	if key := r.Plan.Slots[n].Binding.Session; key != "" {
		return key
	}
	return fmt.Sprintf("web:bench:%s:%04d", r.ID, n+1)
}
func eventID(r Run, n, k int) string { return fmt.Sprintf("bench-%s-%04d-%02d", r.ID, n+1, k+1) }
func rowFile(dir, kind string, n int) string {
	return filepath.Join(dir, fmt.Sprintf("%s-%04d.json", kind, n+1))
}
func turnFile(dir string, slot, k int) string {
	return filepath.Join(dir, fmt.Sprintf("turn-%04d-%02d.json", slot+1, k+1))
}
func newRun(plan Plan, candidate Candidate, setup Setup) Run {
	var nonce [12]byte
	rand.Read(nonce[:])
	return Run{Plan: plan, PlanHash: digest(plan), Candidate: candidate, ID: hex.EncodeToString(nonce[:]), Setup: setup, Started: time.Now().UTC(), Runner: runtime.GOOS + "/" + runtime.GOARCH + " " + runtime.Version()}
}

type executor interface {
	turn(context.Context, string, string, Step, string) Turn
}

func execute(ctx context.Context, client executor, dir string, run Run, work []Case) error {
	if err := os.Mkdir(dir, 0700); err != nil {
		return fmt.Errorf("new run directory required: %w", err)
	}
	if err := writeJSON(filepath.Join(dir, "plan.json"), run); err != nil {
		return err
	}
	if err := writeJSON(filepath.Join(dir, "cases.json"), work); err != nil {
		return err
	}
	for i, c := range work {
		if err := ctx.Err(); err != nil {
			return err
		}
		row := Outcome{Slot: run.Plan.Slots[i], Session: sessionID(run, i), Status: "unavailable", Reason: c.Missing}
		if c.Missing == "" {
			// This record is durable before the first POST. An interrupted slot is
			// visible to report/compare and never silently submitted again.
			if err := writeJSON(rowFile(dir, "started", i), row.Session); err != nil {
				return err
			}
			start := time.Now()
			window, cancel := context.WithTimeout(ctx, run.Plan.Timeout)
			row.Status, row.Reason = "pass", ""
			for k, step := range c.Steps {
				step.Task = row.Slot.Binding.Task
				event := eventID(run, i, k)
				archive := filepath.Join(dir, event)
				if err := os.Mkdir(archive, 0700); err != nil {
					cancel()
					return err
				}
				if err := writeJSON(filepath.Join(archive, "input.json"), map[string]string{"event": event, "session": row.Session, "task": step.Task, "prompt": step.Prompt}); err != nil {
					cancel()
					return err
				}
				turn := client.turn(window, row.Session, event, step, archive)
				var err error
				turn.EvidenceSHA, err = seal(archive)
				if err != nil {
					cancel()
					return err
				}
				if err := writeJSON(turnFile(dir, i, k), turn); err != nil {
					cancel()
					return err
				}
				row.MS += turn.LatencyMS
				row.Turns = append(row.Turns, turn)
				if turn.Reason != "pass" {
					row.Status, row.Reason = classify(turn.Reason), turn.Reason
					break
				}
			}
			cancel()
			row.WallMS = time.Since(start).Milliseconds()
		}
		if err := writeJSON(rowFile(dir, "slot", i), row); err != nil {
			return err
		}
		fmt.Fprintf(os.Stderr, "[%d/%d] %s %s %s\n", i+1, len(work), row.Status, c.ID, row.Reason)
		// A lost submission or uncertain observer may leave work in flight.
		// Keep the original plan, stop issuing new work and expose all remaining slots.
		if row.Status == "error" {
			return fmt.Errorf("collection stopped: %s", row.Reason)
		}
	}
	return nil
}

func classify(reason string) string {
	switch reason {
	case "wrong_answer", "unexpected_tool", "unresolved_effect", "missing_tool_evidence", "input_failed", "input_blocked", "input_cancelled", "input_expired", "input_waiting_for_user", "final_evidence_missing", "web_result_missing":
		return "fail"
	default:
		return "error"
	}
}

var commitRE = regexp.MustCompile("^[0-9a-f]{40}$")
var nonceRE = regexp.MustCompile("^[0-9a-f]{24}$")
var taskRE = regexp.MustCompile("^task_[0-9a-f]{32}$")
var safeCodeRE = regexp.MustCompile("^[a-z][a-z0-9_]{0,79}$")
var shaRE = regexp.MustCompile("^[0-9a-f]{64}$")
