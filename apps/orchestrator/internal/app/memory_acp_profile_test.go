package app

import (
	"context"
	"testing"
	"time"

	"github.com/butler/butler/internal/budget"
	"github.com/butler/butler/internal/config"
	"github.com/butler/butler/internal/modelprovider"
	"github.com/butler/butler/internal/providerfactory"
	"github.com/butler/butler/internal/transport"
)

type acpMemoryProfile struct {
	transport.ModelProvider
	limits transport.AgentExecutionLimits
}

func (acpMemoryProfile) Name() string                                           { return modelprovider.ProviderAntigravityACP }
func (p acpMemoryProfile) AgentExecutionLimits() transport.AgentExecutionLimits { return p.limits }

func TestACPMemoryProfileRequiresExactIndependentBoundsAndBudget(t *testing.T) {
	for _, mode := range []string{"qualified", "nil_provider", "nil_budget", "wrong_identity", "wrong_model", "missing_bounds", "short_timeout", "wrong_input_bound", "wrong_output_bound", "missing_basis", "native_basis", "time_underbudget", "input_underbudget", "output_underbudget"} {
		t.Run(mode, func(t *testing.T) {
			limited := acpMemoryProfile{limits: transport.AgentExecutionLimits{InputBytes: 2 << 20, OutputBytes: 64 << 10, WallTime: 300 * time.Second}}
			p := providerfactory.Result{Provider: limited, ProviderName: modelprovider.ProviderAntigravityACP, ModelName: "gemini-3.8-flash"}
			b := &budget.Controller{Policy: budget.DefaultPolicy()}
			b.Policy.ModelMillis = 300000
			b.Policy.Models[p.ProviderName+":"+p.ModelName] = budget.ModelPrice{Basis: budget.AgentSubscriptionBasis}
			switch mode {
			case "nil_provider":
				p.Provider = nil
			case "nil_budget":
				b = nil
			case "wrong_identity":
				p.ProviderName = "antigravity"
			case "wrong_model":
				p.ModelName = "gemini-other"
			case "missing_bounds":
				p.Provider = candidateProviderWithoutLimits{}
			case "short_timeout":
				limited.limits.WallTime = 299 * time.Second
				p.Provider = limited
			case "wrong_input_bound":
				limited.limits.InputBytes = 1 << 20
				p.Provider = limited
			case "wrong_output_bound":
				limited.limits.OutputBytes = 128 << 10
				p.Provider = limited
			case "missing_basis":
				b.Policy.Models = map[string]budget.ModelPrice{}
			case "native_basis":
				b.Policy.Models[p.ProviderName+":"+p.ModelName] = budget.ModelPrice{Basis: "subscription_no_overage"}
			case "time_underbudget":
				b.Policy.ModelMillis = 299000
			case "input_underbudget":
				b.Policy.ModelInputBytes = 1 << 20
			case "output_underbudget":
				b.Policy.AgentOutputBytes = 32768
			}
			want := mode == "qualified"
			if acpMemoryProfileReady(p, b) != want || candidateProfileReady(p, b) != want || summaryProfileReady(p, b) != want {
				t.Fatal("unmeasured ACP memory profile accepted", mode)
			}
			got, err := selectMemoryProvider(context.Background(), config.OrchestratorConfig{MemoryModelProvider: modelprovider.ProviderAntigravityACP}, p, nil, b)
			if (err == nil) != want || want && got.Provider != p.Provider {
				t.Fatal("explicit ACP memory selection lost its gate", mode, err)
			}
		})
	}
}
