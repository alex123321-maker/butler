package delegation

import (
	"bytes"
	"encoding/json"
	"io"
	"strings"
)

// Report is a bounded DATA projection. It contains no instructions, free-form
// success verdict or permission. Narrative belongs only in its Catalog source.
// Shape validation never verifies an effect; the kernel must compare every
// result/action/evidence/reservation reference to its actual owning records.
type EvidenceRef struct {
	Kind     string `json:"kind"` // source, artifact, action, result, reservation
	ID       string `json:"id"`
	Revision int64  `json:"revision,omitempty"`
	Hash     string `json:"hash,omitempty"`
}
type ActionObservation struct {
	ActionID string        `json:"action_id"`
	EffectID string        `json:"effect_id"`
	State    string        `json:"state"`
	Evidence []EvidenceRef `json:"evidence_refs"`
}
type Reason struct {
	Kind string       `json:"kind"`
	Ref  *EvidenceRef `json:"evidence_ref,omitempty"`
}
type Report struct {
	TaskID           string              `json:"task_id"`
	TaskRevision     int64               `json:"task_revision"`
	ContractRevision int64               `json:"contract_revision"`
	ResultID         string              `json:"result_id"`
	Accomplished     []EvidenceRef       `json:"accomplished"`
	Actions          []ActionObservation `json:"actual_actions"`
	Evidence         []EvidenceRef       `json:"evidence_refs"`
	Uncertainty      []Reason            `json:"uncertainty"`
	Blockers         []Reason            `json:"blockers"`
	Budget           struct {
		ReservationIDs []string `json:"reservation_ids"`
		Used           Limits   `json:"used"`
	} `json:"budget"`
}

func ParseReport(raw []byte) (Report, error) {
	var r Report
	if len(raw) > 32768 {
		return r, ErrDenied
	}
	d := json.NewDecoder(bytes.NewReader(raw))
	d.DisallowUnknownFields()
	if d.Decode(&r) != nil || d.Decode(new(any)) != io.EOF || !validID(r.TaskID) || r.TaskRevision < 1 || r.ContractRevision != 1 || !validID(r.ResultID) || len(r.Actions) > 64 || len(r.Evidence) > 128 || len(r.Accomplished) > 128 || len(r.Uncertainty) > 32 || len(r.Blockers) > 32 || len(r.Budget.ReservationIDs) > 128 {
		return Report{}, ErrDenied
	}
	refs := append(append([]EvidenceRef(nil), r.Evidence...), r.Accomplished...)
	seen := map[string]bool{}
	for _, a := range r.Actions {
		if !validID(a.ActionID) || !validID(a.EffectID) || seen[a.ActionID] || len(a.Evidence) > 32 {
			return Report{}, ErrDenied
		}
		seen[a.ActionID] = true
		switch a.State {
		case "prepared", "allowed", "executing", "effect_confirmed", "effect_absent", "effect_unknown":
		default:
			return Report{}, ErrDenied
		}
		refs = append(refs, a.Evidence...)
	}
	for _, reason := range append(append([]Reason(nil), r.Uncertainty...), r.Blockers...) {
		switch reason.Kind {
		case "effect_unknown", "conflicting_evidence", "source_stale", "verification_missing", "dependency_unavailable", "budget_exhausted", "resource_busy", "authority_revoked", "deadline_expired", "execution_failed":
		default:
			return Report{}, ErrDenied
		}
		if reason.Ref != nil {
			refs = append(refs, *reason.Ref)
		}
	}
	for _, ref := range refs {
		switch ref.Kind {
		case "source", "artifact", "action", "result", "reservation":
		default:
			return Report{}, ErrDenied
		}
		if !validID(ref.ID) || ref.Revision < 0 {
			return Report{}, ErrDenied
		}
		if ref.Hash != "" && !ValidSource(SourceRef{ID: ref.ID, Revision: 1, AccessRevision: 1, Hash: ref.Hash}) {
			return Report{}, ErrDenied
		}
	}
	seen = map[string]bool{}
	for _, id := range r.Budget.ReservationIDs {
		if !validID(id) || seen[id] {
			return Report{}, ErrDenied
		}
		seen[id] = true
	}
	for _, v := range []int64{r.Budget.Used.ModelTurns, r.Budget.Used.Tokens, r.Budget.Used.CostMicros, r.Budget.Used.ExternalCalls, r.Budget.Used.Children, r.Budget.Used.ActiveMillis, r.Budget.Used.EgressBytes, r.Budget.Used.ProviderInputBytes} {
		if v < 0 {
			return Report{}, ErrDenied
		}
	}
	return r, nil
}
func validID(id string) bool {
	if id == "" || len(id) > 512 || strings.ContainsAny(id, "\x00\r\n \t") {
		return false
	}
	return true
}
