package app

import (
	"fmt"

	"github.com/butler/butler/internal/budget"
	"github.com/butler/butler/internal/providerfactory"
	"github.com/butler/butler/internal/transport/antigravity"
)

// Raising the native ceiling must not under-reserve ordinary foreground calls
// even when the independently gated summary consumer is disabled.
func validateExpandedNativeOutputBudget(p providerfactory.Result, policy budget.Policy) error {
	if p.Provider == nil || p.Provider.Name() != antigravity.Name {
		return nil
	}
	limits, ok := p.Provider.(interface{ InferenceLimits() (int64, int64) })
	if !ok {
		return nil
	}
	_, output := limits.InferenceLimits()
	if output > 8192 && policy.OutputTokens < output {
		return fmt.Errorf("configured native generated-token ceiling exceeds execution budget")
	}
	return nil
}
