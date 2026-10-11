package app

import (
	"fmt"

	"github.com/butler/butler/internal/budget"
	"github.com/butler/butler/internal/providerfactory"
)

// Match ordinary model admission: the actual provider request limit must fit
// policy. A smaller bookkeeping quote cannot impose a cap on a provider.
func converterContinuationAllowance(selected providerfactory.Result, policy budget.Policy) (budget.ModelAllowance, error) {
	a := budget.ModelAllowance{Provider: selected.ProviderName, Model: selected.ModelName}
	declared, ok := selected.Provider.(interface{ InferenceLimits() (int64, int64) })
	if !ok {
		return a, fmt.Errorf("converter continuation requires declared provider inference limits")
	}
	a.InputTokens, a.OutputTokens = declared.InferenceLimits()
	if a.InputTokens <= 0 || a.OutputTokens <= 0 || a.OutputTokens > policy.OutputTokens {
		return a, fmt.Errorf("converter continuation provider inference limits exceed current budget policy")
	}
	// The ordinary budget reserves the full policy allowance, conservatively
	// covering even a provider whose enforced output limit is smaller.
	a.OutputTokens = policy.OutputTokens
	return a, nil
}
