package budget

import (
	"strings"

	"github.com/butler/butler/internal/modelprovider"
)

const AgentSubscriptionBasis = "subscription_agent_no_overage"

func (policy Policy) AgentQuote(provider, model string) (Units, error) {
	price, exists := policy.Models[provider+":"+model]
	if !exists || provider != modelprovider.ProviderAntigravityACP || !strings.HasPrefix(model, "gemini-") || price.Basis != AgentSubscriptionBasis {
		return Units{}, ErrUnknownCost
	}
	if price.InputMicrosPerToken != 0 || price.OutputMicrosPerToken != 0 || policy.ModelMillis <= 0 || policy.ModelInputBytes <= 0 || policy.AgentOutputBytes <= 0 {
		return Units{}, ErrInvalid
	}
	return Units{ModelTurns: 1, ActiveMillis: policy.ModelMillis, ProviderInputBytes: policy.ModelInputBytes, EgressBytes: policy.AgentOutputBytes}, nil
}

func (allowance ModelAllowance) Validate(policy Policy) error {
	if allowance.Agent {
		if allowance.InputTokens != 0 || allowance.OutputTokens != 0 {
			return ErrInvalid
		}
		_, err := policy.AgentQuote(allowance.Provider, allowance.Model)
		return err
	}
	if allowance.InputTokens <= 0 || allowance.OutputTokens <= 0 || allowance.OutputTokens > policy.OutputTokens {
		return ErrInvalid
	}
	return nil
}
