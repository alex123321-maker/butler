package app

import (
	"strings"
	"testing"

	"github.com/butler/butler/internal/acceptance"
)

func TestMemoryUtilityInstructionsAreScopedToCurrentGoal(t *testing.T) {
	suite, err := acceptance.NewMemoryUtilitySuite()
	if err != nil {
		t.Fatal(err)
	}
	for _, c := range suite.Cases {
		rules := utilityWholeInstructions(c)
		for token, goal := range map[string]string{"?verify_unknown=current": "unknown_effect", "?validate_ready=maybe": "verified_negative", "/patches/{artifact_id}/checks": "prepared_artifact"} {
			if strings.Contains(rules, token) != (c.Family == "regression" && c.Condition == goal) {
				t.Fatal("unrelated special task instruction", c.Family, c.Condition, token)
			}
		}
		if strings.Contains(rules, "route/codec/guard") != (c.Family == "procedure") || !strings.Contains(rules, "limited") {
			t.Fatal("procedure or limited guidance is not scoped", c.Family, c.Condition)
		}
		// Regression words such as ready are explicitly present in the owner's
		// task and are not hidden random historical answer values.
		if c.Family != "regression" && c.Current == "" && c.Expected.Value != "" && strings.Contains(rules, c.Expected.Value) {
			t.Fatal("oracle value leaked into operational instructions")
		}
	}
}
