package experience

import (
	"encoding/json"
	"sort"
	"strings"
	"testing"

	"github.com/butler/butler/internal/actionauth"
	"github.com/butler/butler/internal/memory/catalog"
	"github.com/butler/butler/internal/memory/transcript"
	"github.com/butler/butler/internal/skills"
)

func reconciledObservationFixture() Observation {
	o := terminalObservationFixture()
	h := strings.Repeat("a", 64)
	d := skills.Dependency{Tool: "http.request", ContractHash: h, ContractRevision: "1"}
	o.Environment.Tools = append(o.Environment.Tools, d)
	r := &ReconciledAttempt{OriginalSnapshotID: "original-witness", OriginalObservationHash: h, OriginalAccessRevision: 1, ProofHash: h, ControlID: "owner-resume", CurrentTaskRevision: 2, CurrentConstraintsRevision: 2, OriginalCurrentInputHash: h, OriginalCurrentInputRevision: 2, ContinuationInputID: "read-input", ContinuationInputHash: h, ContinuationInputRevision: 3, ContinuationRunID: "read-run", ContinuationResultID: "read-final", ContinuationMessageID: "read-message", ContinuationReport: "A separately authorized read; the original had no final", TargetActionID: o.Actions[0].ID, CurrentEffect: actionauth.StateAbsent, ReconciliationEvidenceID: "independent-lookup-evidence", Lookup: Action{ID: "lookup-action", ToolCallID: "lookup-call", Tool: d.Tool, ContractHash: h, ContractRevision: "1", ArgumentsHash: h, Effect: actionauth.StateConfirmed, Evidence: []string{"actual-read-evidence"}, Result: "Observed separate recipient receipt"}, LookupPin: transcript.ToolPin{ID: "lookup-call", RunID: "read-run", Hash: h, Revision: 1}}
	for _, kind := range []string{"control", "target", "transition", "final", "run", "run", "effect", "effect", "input", "input"} {
		r.Heads = append(r.Heads, ReconciledHeadPin{Kind: kind, ID: kind + "-" + string(rune('a'+len(r.Heads))), Hash: h, Revision: 1})
	}
	sort.Slice(r.Heads, func(i, j int) bool { return r.Heads[i].Kind+":"+r.Heads[i].ID < r.Heads[j].Kind+":"+r.Heads[j].ID })
	o.ReconciledAttempt = r
	return o
}

func TestReconciledObservationKeepsOriginalBoundary(t *testing.T) {
	o := reconciledObservationFixture()
	if err := o.Validate(); err != nil {
		t.Fatal("explicit separately reconciled observation rejected", err)
	}
	for name, change := range map[string]func(*Observation){
		"fake_original_final":       func(o *Observation) { o.ResultID = o.ReconciledAttempt.ContinuationResultID },
		"fake_original_report":      func(o *Observation) { o.AgentReport = o.ReconciledAttempt.ContinuationReport },
		"rewritten_original_effect": func(o *Observation) { o.Actions[0].Effect = actionauth.StateAbsent },
		"unknown_current_effect":    func(o *Observation) { o.ReconciledAttempt.CurrentEffect = actionauth.StateUnknown },
		"same_original_run":         func(o *Observation) { o.ReconciledAttempt.ContinuationRunID = o.RunID },
		"unbounded_revision_jump":   func(o *Observation) { o.ReconciledAttempt.CurrentTaskRevision++ },
		"failed_lookup":             func(o *Observation) { o.ReconciledAttempt.Lookup.Failure = "failed" },
		"missing_row_heads":         func(o *Observation) { o.ReconciledAttempt.Heads = o.ReconciledAttempt.Heads[:9] },
		"missing_lookup_contract":   func(o *Observation) { o.Environment.Tools = o.Environment.Tools[:1] },
	} {
		t.Run(name, func(t *testing.T) {
			v := reconciledObservationFixture()
			change(&v)
			if v.Validate() == nil {
				t.Fatal("conflated or unsupported reconciled evidence accepted")
			}
		})
	}
	n := notesFixture()
	n.Steps = append(n.Steps, Step{ActionID: o.ReconciledAttempt.Lookup.ID, Comment: "A distinct owner-authorized read later established absence"})
	raw, _ := json.Marshal(o)
	note, _ := json.Marshal(n)
	body, err := (Handler{}).Validate([]catalog.Source{{Body: string(raw)}}, string(note))
	var lesson Lesson
	if err != nil || json.Unmarshal([]byte(body), &lesson) != nil || lesson.Verification != "task_unverified" || lesson.InstructionAuthority || lesson.Observation.ResultID != "" || lesson.Observation.AgentReport != "" || lesson.Observation.Actions[0].Effect != actionauth.StateUnknown || lesson.Observation.ReconciledAttempt.CurrentEffect != actionauth.StateAbsent {
		t.Fatal("model note conflated original failure with current read success", err)
	}
}
