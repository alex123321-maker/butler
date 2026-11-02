package app

import (
	"fmt"
	"testing"
	"time"

	"github.com/butler/butler/internal/acceptance"
)

// Operator-only research selection, never a model/tool argument or product default.
// Empty retains the closed none profile; medium is explicit in both freeze and run.
func utilityWorkflowTerraEffort(selected string) (string, error) {
	switch selected {
	case "", "none":
		return "none", nil
	case "medium":
		return "medium", nil
	default:
		return "", fmt.Errorf("unknown trusted workflow Terra reasoning profile")
	}
}

func utilityNativeJournalName(plan utilityNativePlan) string {
	// The exact same suite can be repeated without consuming or overwriting
	// another window's journal, including the shared private Gemini auth DB.
	return "utility_abc_" + acceptance.MemoryDigest(plan)[:16]
}

func TestMemoryUtilityWorkflowReasoningProfiles(t *testing.T) {
	for _, tc := range []struct{ input, want string }{{"", "none"}, {"none", "none"}, {"medium", "medium"}} {
		got, err := utilityWorkflowTerraEffort(tc.input)
		if err != nil || got != tc.want {
			t.Fatalf("profile %q: %q %v", tc.input, got, err)
		}
	}
	for _, invalid := range []string{"max", "high", "MEDIUM", " medium", "medium\n"} {
		if _, err := utilityWorkflowTerraEffort(invalid); err == nil {
			t.Fatal("unfrozen profile accepted")
		}
	}
	plan := utilityNativePlan{SuiteSHA256: "same-suite", Source: "same-source", Deadline: time.Date(2026, 11, 2, 0, 0, 0, 0, time.UTC)}
	first := utilityNativeJournalName(plan)
	if first != utilityNativeJournalName(plan) {
		t.Fatal("unstable journal identity")
	}
	plan.Deadline = plan.Deadline.Add(time.Nanosecond)
	if first == utilityNativeJournalName(plan) {
		t.Fatal("repeated suite reused a journal window")
	}
}
