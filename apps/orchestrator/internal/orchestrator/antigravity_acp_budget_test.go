package orchestrator

import (
	"context"
	"errors"
	"testing"

	"github.com/butler/butler/internal/budget"
	"github.com/butler/butler/internal/transport"
	"github.com/butler/butler/internal/transport/antigravityacp"
)

type unqualifiedACPProvider struct {
	*antigravityacp.Provider
	calls int
}

func (provider *unqualifiedACPProvider) StartRun(context.Context, transport.StartRunRequest) (transport.EventStream, error) {
	provider.calls++
	return nil, errors.New("unqualified runtime reached")
}

func (provider *unqualifiedACPProvider) ContinueRun(context.Context, transport.ContinueRunRequest) (transport.EventStream, error) {
	provider.calls++
	return nil, errors.New("unqualified runtime reached")
}

func (provider *unqualifiedACPProvider) SubmitToolResult(context.Context, transport.SubmitToolResultRequest) (transport.EventStream, error) {
	provider.calls++
	return nil, errors.New("unqualified runtime reached")
}

func TestAntigravityACPRequiresExplicitBudgetEveryInferenceEntry(t *testing.T) {
	ctx := context.Background()
	raw := &unqualifiedACPProvider{Provider: &antigravityacp.Provider{}}
	provider := &budgetedProvider{ModelProvider: raw, service: &Service{}}
	checks := []struct {
		name string
		call func() (transport.EventStream, error)
	}{
		{"start", func() (transport.EventStream, error) {
			return provider.StartRun(ctx, transport.StartRunRequest{Context: transport.TransportRunContext{RunID: "run"}})
		}},
		{"continue", func() (transport.EventStream, error) {
			return provider.ContinueRun(ctx, transport.ContinueRunRequest{RunID: "run"})
		}},
		{"tool_result", func() (transport.EventStream, error) {
			return provider.SubmitToolResult(ctx, transport.SubmitToolResultRequest{RunID: "run"})
		}},
	}
	for _, check := range checks {
		t.Run(check.name, func(t *testing.T) {
			stream, err := check.call()
			if stream != nil || !errors.Is(err, budget.ErrUnknownCost) {
				t.Fatalf("expected refusal before runtime dispatch, got stream=%v error=%v", stream, err)
			}
			if raw.calls != 0 {
				t.Fatal("unqualified provider launched before native inference limits were established")
			}
		})
	}
}

func TestAntigravityACPAdmitsOnlyDeclaredAgentBudget(t *testing.T) {
	policy := budget.DefaultPolicy()
	policy.Models["antigravity-acp:gemini-fixture"] = budget.ModelPrice{Basis: budget.AgentSubscriptionBasis}
	provider := &budgetedProvider{ModelProvider: &antigravityacp.Provider{}, service: &Service{config: Config{ProviderName: "antigravity-acp", ModelName: "gemini-fixture", Budgets: budget.NewController(nil, policy)}}}
	if input, err := provider.inferenceAllowance(); err != nil || input != 0 {
		t.Fatal("declared opaque allowance refused or fabricated tokens", input, err)
	}
	provider.service.config.Budgets.Policy.AgentOutputBytes = 100
	if _, err := provider.inferenceAllowance(); err == nil {
		t.Fatal("provider output bound exceeded policy")
	}
	provider.service.config.Budgets.Policy = policy
	provider.service.config.Budgets.Policy.ModelMillis = 1
	if _, err := provider.inferenceAllowance(); err == nil {
		t.Fatal("provider time bound exceeded policy")
	}
}
