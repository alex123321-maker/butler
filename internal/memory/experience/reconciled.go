package experience

import (
	"github.com/butler/butler/internal/actionauth"
	"github.com/butler/butler/internal/budget"
	"github.com/butler/butler/internal/memory/transcript"
)

// ReconciledAttempt is a distinct current supplement to an original stopped
// attempt. Original RunID/revision/UNKNOWN and the absence of a final do not
// change. A separately authorized owner Resume owns this read and its final.
type ReconciledAttempt struct {
	OriginalSnapshotID           string                 `json:"original_snapshot_id"`
	OriginalObservationHash      string                 `json:"original_observation_hash"`
	OriginalAccessRevision       int64                  `json:"original_access_revision"`
	ProofHash                    string                 `json:"proof_hash"`
	ControlID                    string                 `json:"control_id"`
	CurrentTaskRevision          int64                  `json:"current_task_revision"`
	CurrentConstraintsRevision   int64                  `json:"current_constraints_revision"`
	OriginalCurrentInputHash     string                 `json:"original_current_input_hash"`
	OriginalCurrentInputRevision int64                  `json:"original_current_input_revision"`
	ContinuationInputID          string                 `json:"continuation_input_id"`
	ContinuationInputHash        string                 `json:"continuation_input_hash"`
	ContinuationInputRevision    int64                  `json:"continuation_input_revision"`
	ContinuationRunID            string                 `json:"continuation_run_id"`
	ContinuationResultID         string                 `json:"continuation_result_id"`
	ContinuationMessageID        string                 `json:"continuation_message_id"`
	ContinuationReport           string                 `json:"continuation_report"`
	TargetActionID               string                 `json:"target_action_id"`
	CurrentEffect                actionauth.EffectState `json:"current_effect"`
	ReconciliationEvidenceID     string                 `json:"reconciliation_evidence_id"`
	Lookup                       Action                 `json:"lookup"`
	LookupPin                    transcript.ToolPin     `json:"lookup_pin"`
	Heads                        []ReconciledHeadPin    `json:"heads"`
	Usage                        *budget.ModelUsage     `json:"usage"`
}

// These content-free heads cover only the exact durable control, original/run
// continuation, final and effect rows used by the owning adapter. Supervision
// does not reread a checkpoint, owner instruction or tool response body.
type ReconciledHeadPin struct {
	Kind     string `json:"kind"`
	ID       string `json:"id"`
	Hash     string `json:"hash"`
	Revision int64  `json:"revision"`
}

func (r ReconciledAttempt) validate(o Observation) error {
	if !hexHash.MatchString(r.ProofHash) || r.OriginalAccessRevision < 1 {
		return ErrEvidence
	}
	if o.TerminalAttempt == nil || o.TerminalAttempt.InputStatus != "blocked" || o.TerminalAttempt.Reason != "effect_unknown" || o.TerminalAttempt.RunState != "tool_running" || r.CurrentTaskRevision != o.TaskRevision+1 || r.CurrentConstraintsRevision != o.ConstraintsRevision+1 || r.ContinuationRunID == o.RunID || r.ContinuationInputID == o.TerminalAttempt.InputID || r.CurrentEffect != actionauth.StateAbsent || r.OriginalCurrentInputRevision <= o.TerminalAttempt.InputRevision || r.ContinuationInputRevision < 1 || !hexHash.MatchString(r.OriginalObservationHash) || !hexHash.MatchString(r.OriginalCurrentInputHash) || !hexHash.MatchString(r.ContinuationInputHash) || !text(r.ContinuationReport, 4096) {
		return ErrEvidence
	}
	for _, id := range []string{r.OriginalSnapshotID, r.ControlID, r.ContinuationInputID, r.ContinuationRunID, r.ContinuationResultID, r.ContinuationMessageID, r.TargetActionID, r.ReconciliationEvidenceID} {
		if !text(id, 160) {
			return ErrEvidence
		}
	}
	lookup := r.Lookup
	if lookup.Tool != "http.request" || lookup.Effect != actionauth.StateConfirmed || lookup.Failure != "" || !text(lookup.ID, 160) || !text(lookup.ToolCallID, 160) || !hexHash.MatchString(lookup.ContractHash) || !text(lookup.ContractRevision, 160) || !hexHash.MatchString(lookup.ArgumentsHash) || !stringsValid(lookup.Evidence, 16, 160) || len(lookup.Evidence) == 0 || !text(lookup.Result, 2048) || r.LookupPin.ID != lookup.ToolCallID || r.LookupPin.RunID != r.ContinuationRunID || !hexHash.MatchString(r.LookupPin.Hash) || r.LookupPin.Revision < 1 {
		return ErrEvidence
	}
	dependency := false
	for _, d := range o.Environment.Tools {
		dependency = dependency || d.Tool == lookup.Tool && d.ContractHash == lookup.ContractHash && d.ContractRevision == lookup.ContractRevision
	}
	if !dependency {
		return ErrEvidence
	}
	if len(r.Heads) != 9+len(o.Actions) || len(r.Heads) > 9+MaxActions {
		return ErrEvidence
	}
	prior := ""
	counts := map[string]int{}
	for _, pin := range r.Heads {
		key := pin.Kind + ":" + pin.ID
		if key <= prior || !text(pin.ID, 160) || !hexHash.MatchString(pin.Hash) || pin.Revision < 1 {
			return ErrEvidence
		}
		prior = key
		counts[pin.Kind]++
	}
	if counts["run"] != 2 || counts["control"] != 1 || counts["target"] != 1 || counts["transition"] != 1 || counts["final"] != 1 || counts["effect"] != len(o.Actions)+1 || counts["input"] != 2 || len(counts) != 7 {
		return ErrEvidence
	}
	found := false
	for _, action := range o.Actions {
		found = found || action.ID == r.TargetActionID && action.Effect == actionauth.StateUnknown && action.Failure == "effect_unknown"
	}
	if !found {
		return ErrEvidence
	}
	return nil
}

func (r ReconciledAttempt) proofContext(o Observation) map[string]any {
	actions := map[string]string{}
	for _, action := range o.Actions {
		actions[action.ID] = ActionHash(action)
	}
	return map[string]any{
		"original_snapshot_id": r.OriginalSnapshotID, "original_observation_hash": r.OriginalObservationHash, "original_access_revision": r.OriginalAccessRevision, "original_attempt": o.TerminalAttempt,
		"task_id": o.TaskID, "session_key": o.SessionKey, "task_revision": o.TaskRevision, "constraints_revision": o.ConstraintsRevision,
		"control_id": r.ControlID, "current_task_revision": r.CurrentTaskRevision, "current_constraints_revision": r.CurrentConstraintsRevision,
		"original_current_input_hash": r.OriginalCurrentInputHash, "original_current_input_revision": r.OriginalCurrentInputRevision,
		"continuation_input_id": r.ContinuationInputID, "continuation_input_hash": r.ContinuationInputHash, "continuation_input_revision": r.ContinuationInputRevision,
		"continuation_run_id": r.ContinuationRunID, "continuation_result_id": r.ContinuationResultID, "continuation_message_id": r.ContinuationMessageID,
		"target_action_id": r.TargetActionID, "reconciliation_evidence_id": r.ReconciliationEvidenceID, "lookup_action_id": r.Lookup.ID, "heads": r.Heads,
		"original_action_hashes": actions, "original_tool_pins": o.ToolPins,
	}
}

// ActionHash identifies the exact typed historical action without retaining
// another copy of its response text in immutable witness metadata.
func ActionHash(action Action) string { return hash(action) }
