package app

import (
	"testing"

	flow "github.com/butler/butler/apps/orchestrator/internal/orchestrator"
	"github.com/butler/butler/internal/budget"
	"github.com/butler/butler/internal/providerfactory"
)

func TestExperienceNativeForegroundConfigurationDefaultsAndPredicate(t *testing.T) {
	policy := budget.DefaultPolicy()
	policy.ModelMillis, policy.OutputTokens = 300000, 32768
	controller := budget.NewController(nil, policy)
	result := providerfactory.Result{Provider: &experienceOriginalControlProvider{}, ModelName: "gemini-3.8-flash-high"}
	base := flow.Config{ContextBudgetTokens: 65536, ProfileStore: profileStoreAdapter{}}
	legacy := experienceNativeForegroundConfig(base, result, controller, nil)
	if legacy.ContextBudgetTokens != 65536 || legacy.ProfileStore.(profileStoreAdapter).temporal {
		t.Fatal("legacy90 defaults changed")
	}
	for _, enabled := range []bool{false, true} {
		selected := experienceNativeForegroundConfig(base, result, controller, &experienceNativeForeground{ContextTokens: 131072, TemporalEnabled: enabled})
		if selected.ContextBudgetTokens != 131072 || selected.ProfileStore.(profileStoreAdapter).temporal != temporalReadConfigured(enabled, result, controller) {
			t.Fatal("explicit foreground bypassed shared predicate")
		}
	}
	wrong := result
	wrong.ModelName = "unsupported"
	if experienceNativeForegroundConfig(base, wrong, controller, &experienceNativeForeground{ContextTokens: 131072, TemporalEnabled: true}).ProfileStore.(profileStoreAdapter).temporal {
		t.Fatal("wrong profile enabled temporal context")
	}
}
