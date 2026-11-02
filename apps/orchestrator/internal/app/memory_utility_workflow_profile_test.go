package app

import (
	"fmt"
	"testing"
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
}
