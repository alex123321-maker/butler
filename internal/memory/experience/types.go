// Package experience stores typed lessons inside Episodic Memory. Model notes
// are DATA; task verification, effects, permissions and skill review stay owned
// by their existing services.
package experience

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"regexp"
	"strings"
	"unicode/utf8"

	"github.com/butler/butler/internal/actionauth"
	"github.com/butler/butler/internal/budget"
	"github.com/butler/butler/internal/memory/catalog"
	"github.com/butler/butler/internal/memory/sanitize"
	"github.com/butler/butler/internal/memory/transcript"
	"github.com/butler/butler/internal/skills"
)

const (
	Version        = "task-experience-v1"
	Format         = "butler.episodic.lesson.v1"
	SnapshotFormat = "butler.episodic.observation.v1"
	MaxCases       = 8
	MaxActions     = 24
	MaxBytes       = 24576
	// Component proof is deliberately independent of native A/B/C quality.
	ReleaseQualified = false
)

var ErrEvidence = errors.New("current typed experience evidence required")
var hexHash = regexp.MustCompile(`^[a-f0-9]{64}$`)

// Environment is an operator-attested version fingerprint, never model args.
// Exact historical tool hashes are independently obtained from action bindings.
type Environment struct {
	Fingerprint string              `json:"fingerprint"`
	Tools       []skills.Dependency `json:"tools"`
}

type Action struct {
	ID               string                 `json:"action_id"`
	ToolCallID       string                 `json:"tool_call_id"`
	Tool             string                 `json:"tool"`
	ContractHash     string                 `json:"contract_hash"`
	ContractRevision string                 `json:"contract_revision"`
	ArgumentsHash    string                 `json:"arguments_hash"`
	Effect           actionauth.EffectState `json:"effect"`
	Evidence         []string               `json:"evidence_refs"`
	// Result is sanitized observed DATA, never a task success verdict. The
	// owning adapter obtains it from the matching actual tool call, not notes.
	Result  string `json:"result"`
	Failure string `json:"failure"`
}

type Observation struct {
	Format              string               `json:"format"`
	OwnerID             string               `json:"owner_id"`
	TaskID              string               `json:"task_id"`
	TaskRevision        int64                `json:"task_revision"`
	ConstraintsRevision int64                `json:"constraints_revision"`
	RunID               string               `json:"run_id"`
	ResultID            string               `json:"result_id"`
	TerminalAttempt     *TerminalAttempt     `json:"terminal_attempt,omitempty"`
	SessionKey          string               `json:"session_key"`
	TranscriptRevision  int64                `json:"transcript_revision"`
	ToolPins            []transcript.ToolPin `json:"tool_pins"`
	Goal                string               `json:"goal"`
	Constraints         []string             `json:"constraints"`
	Environment         Environment          `json:"environment"`
	// These are three separate observations. Neither a model's assertion nor
	// successful transport/read effects establish verified task completion.
	AgentReport      string             `json:"agent_report"`
	OwnerVerdict     string             `json:"owner_verdict"`
	OwnerResultID    string             `json:"owner_result_id"`
	VerifiedCriteria []string           `json:"verified_criteria"`
	Actions          []Action           `json:"actions"`
	Failures         []FailedCall       `json:"failed_calls"`
	Unknowns         []string           `json:"unknowns"`
	Usage            *budget.ModelUsage `json:"usage"`
}

// TerminalAttempt is an owning stopped-input observation, not a fabricated
// assistant final or task result. Its exact immutable projection is checked
// again by Catalog and the separate owner-authorized background policy.
type TerminalAttempt struct {
	InputID       string `json:"input_id"`
	InputStatus   string `json:"input_status"`
	Attempts      int    `json:"attempts"`
	Checkpoint    string `json:"checkpoint"`
	InputHash     string `json:"input_hash"`
	InputRevision int64  `json:"input_revision"`
	Reason        string `json:"reason"`
	RunState      string `json:"run_state"`
}

func (a TerminalAttempt) Validate() error {
	if !text(a.InputID, 160) || a.Attempts < 1 || !text(a.Checkpoint, 160) || !hexHash.MatchString(a.InputHash) || a.InputRevision < 1 {
		return ErrEvidence
	}
	if a.InputStatus != "blocked" && a.InputStatus != "failed" {
		return ErrEvidence
	}
	switch a.Reason {
	case "effect_unknown", "execution_stopped", "execution_deadline_exceeded", "prior_attempt_terminal":
	default:
		return ErrEvidence
	}
	if a.RunState != "failed" && a.RunState != "timed_out" && !(a.InputStatus == "blocked" && a.Reason == "effect_unknown" && a.RunState == "tool_running") {
		return ErrEvidence
	}
	return nil
}

type FailedCall struct {
	ToolCallID string `json:"tool_call_id"`
	Tool       string `json:"tool"`
	Status     string `json:"status"`
	Result     string `json:"result"`
}

// Notes is the only model-written part. Every claimed step points to an actual
// action, while the complete typed observations (including failures) are copied
// by code. Applicability wording cannot broaden environment, scope or access.
type Notes struct {
	Summary       string   `json:"summary"`
	When          []string `json:"when"`
	NotApplicable []string `json:"not_applicable"`
	Steps         []Step   `json:"steps"`
}
type Step struct {
	ActionID string `json:"action_id"`
	Comment  string `json:"comment"`
}
type Lesson struct {
	Format               string      `json:"format"`
	Notes                Notes       `json:"model_notes"`
	Observation          Observation `json:"observation"`
	Verification         string      `json:"verification"`
	InstructionAuthority bool        `json:"instruction_authority"`
}

func hash(v any) string {
	b, _ := json.Marshal(v)
	s := sha256.Sum256(b)
	return hex.EncodeToString(s[:])
}
func ref(s catalog.Source) catalog.SourceRef {
	return catalog.SourceRef{ID: s.ID, Revision: s.Revision, AccessRevision: s.AccessRevision, Hash: s.Hash}
}
func text(s string, n int) bool {
	return strings.TrimSpace(s) != "" && utf8.ValidString(s) && len(s) <= n && sanitize.Text(s) == s
}
func stringsValid(ss []string, max, n int) bool {
	if ss == nil || len(ss) > max {
		return false
	}
	seen := map[string]bool{}
	for _, s := range ss {
		if !text(s, n) || seen[s] {
			return false
		}
		seen[s] = true
	}
	return true
}

func (o Observation) Validate() error {
	if !text(o.SessionKey, 256) || o.TranscriptRevision < 1 || len(o.ToolPins) < 1 || len(o.ToolPins) > 2*MaxActions {
		return ErrEvidence
	}
	pins := map[string]bool{}
	priorPin := ""
	for _, p := range o.ToolPins {
		if !text(p.ID, 160) || p.RunID != o.RunID || !hexHash.MatchString(p.Hash) || p.Revision < 1 || pins[p.ID] || p.ID <= priorPin {
			return ErrEvidence
		}
		pins[p.ID] = true
		priorPin = p.ID
	}
	used := map[string]bool{}
	if o.Failures == nil || len(o.Failures) > MaxActions {
		return ErrEvidence
	}
	seenFailures := map[string]bool{}
	for _, f := range o.Failures {
		if !text(f.ToolCallID, 160) || !text(f.Tool, 160) || len(f.Result) > 2048 || !utf8.ValidString(f.Result) || sanitize.Text(f.Result) != f.Result || seenFailures[f.ToolCallID] {
			return ErrEvidence
		}
		seenFailures[f.ToolCallID] = true
		if !pins[f.ToolCallID] {
			return ErrEvidence
		}
		used[f.ToolCallID] = true
		switch f.Status {
		case "failed", "effect_unknown", "execution_stopped":
		default:
			return ErrEvidence
		}
	}
	if o.Format != SnapshotFormat || !text(o.OwnerID, 160) || !text(o.TaskID, 160) || o.TaskRevision < 1 || o.ConstraintsRevision < 1 || !text(o.RunID, 160) || !text(o.Goal, 2048) || !hexHash.MatchString(o.Environment.Fingerprint) || !stringsValid(o.Constraints, 32, 1024) || !stringsValid(o.Unknowns, 16, 512) || !stringsValid(o.VerifiedCriteria, 32, 1024) || len(o.Actions) == 0 || len(o.Actions) > MaxActions || len(o.Environment.Tools) == 0 || len(o.Environment.Tools) > MaxActions {
		return ErrEvidence
	}
	if o.TerminalAttempt == nil {
		if !text(o.ResultID, 160) || !text(o.AgentReport, 4096) {
			return ErrEvidence
		}
	} else if o.TerminalAttempt.Validate() != nil || o.ResultID != "" || o.AgentReport != "" || o.OwnerVerdict != "unknown" || o.OwnerResultID != "" || len(o.VerifiedCriteria) != 0 {
		return ErrEvidence
	}
	if o.OwnerVerdict != "verified" && o.OwnerVerdict != "negative" && o.OwnerVerdict != "unknown" {
		return ErrEvidence
	}
	if o.OwnerVerdict == "verified" && (o.OwnerResultID == "" || len(o.VerifiedCriteria) == 0) {
		return ErrEvidence
	}
	if o.OwnerVerdict != "verified" && len(o.VerifiedCriteria) != 0 {
		return ErrEvidence
	}
	deps := map[string]skills.Dependency{}
	ids := map[string]bool{}
	for _, d := range o.Environment.Tools {
		if !text(d.Tool, 160) || !text(d.ContractRevision, 160) || !hexHash.MatchString(d.ContractHash) || deps[d.Tool].Tool != "" {
			return ErrEvidence
		}
		deps[d.Tool] = d
	}
	for _, a := range o.Actions {
		if !pins[a.ToolCallID] {
			return ErrEvidence
		}
		used[a.ToolCallID] = true
		d, ok := deps[a.Tool]
		if !ok || d.ContractHash != a.ContractHash || d.ContractRevision != a.ContractRevision || !text(a.ID, 160) || ids[a.ID] || !hexHash.MatchString(a.ArgumentsHash) || !stringsValid(a.Evidence, 16, 160) || !utf8.ValidString(a.Result) || len(a.Result) > 2048 || sanitize.Text(a.Result) != a.Result || len(a.Failure) > 80 {
			return ErrEvidence
		}
		ids[a.ID] = true
		switch a.Failure {
		case "", "failed", "effect_unknown", "execution_stopped":
		default:
			return ErrEvidence
		}
		switch a.Effect {
		case actionauth.StateConfirmed:
			if len(a.Evidence) == 0 {
				return ErrEvidence
			}
		case actionauth.StateAbsent, actionauth.StateUnknown, actionauth.StatePrepared, actionauth.StateAllowed, actionauth.StateExecuting:
		default:
			return ErrEvidence
		}
	}
	if len(used) != len(pins) {
		return ErrEvidence
	}
	b, _ := json.Marshal(o)
	if len(b) > MaxBytes {
		return ErrEvidence
	}
	// Usage is trusted numeric accounting. Generic credential sanitization
	// deliberately masks keys containing "token", including numeric counters;
	// inspect only the text-bearing projection, in canonical map key order.
	o.Usage = nil
	b, _ = json.Marshal(o)
	var fields any
	_ = json.Unmarshal(b, &fields)
	b, _ = json.Marshal(fields)
	if sanitize.JSON(string(b)) != string(b) {
		return ErrEvidence
	}
	return nil
}

func verification(o Observation) string {
	if o.OwnerVerdict == "negative" {
		return "owner_negative"
	}
	if o.OwnerVerdict != "verified" {
		return "task_unverified"
	}
	for _, a := range o.Actions {
		if a.Effect != actionauth.StateConfirmed && a.Effect != actionauth.StateAbsent {
			return "outcome_unknown"
		}
	}
	return "owner_verified_with_observed_actions"
}
