package app

import (
	"testing"

	"github.com/butler/butler/apps/orchestrator/internal/completion"
	"github.com/butler/butler/internal/actionauth"
)

func TestDelegationProjectionRetainsIndependentFinalLimitations(t *testing.T) {
	for _, tc := range []struct {
		name, limitation, expected string
		denied                     bool
	}{
		{"unavailable effect evidence", "effect_evidence_unavailable", "verification_missing", false},
		{"incomplete model output", "model_output_incomplete", "execution_failed", false},
		{"unknown final limitation", "unrecognized_future_limitation", "", true},
		{"unrelated unresolved action", "unresolved_action:foreign", "", true},
		{"confirmed action cannot be unresolved", "unresolved_action:confirmed", "", true},
		{"actual unresolved action", "unresolved_action:unknown", "", false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			a := delegationReportEvidence{
				// No task-verification reason is preloaded: an owner verdict must
				// not erase independently recorded missing effect evidence.
				reasons: map[string]bool{},
				final: completion.FinalRecord{Limitations: []string{tc.limitation}, Effects: []completion.Effect{
					{ActionID: "confirmed", EffectState: string(actionauth.StateConfirmed)},
					{ActionID: "unknown", EffectState: string(actionauth.StateUnknown)},
				}},
			}
			err := retainDelegationFinalLimitations(&a)
			if (err != nil) != tc.denied {
				t.Fatal("unexpected limitation admission", err)
			}
			if tc.expected != "" && !a.reasons[tc.expected] {
				t.Fatal("recorded final limitation was lost", a.reasons)
			}
		})
	}
}
