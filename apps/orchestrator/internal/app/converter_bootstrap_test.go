package app

import (
	"testing"

	"github.com/butler/butler/internal/budget"
	"github.com/butler/butler/internal/providerfactory"
	"github.com/butler/butler/internal/transport"
)

type converterBootstrapProvider struct {
	transport.ModelProvider
	input, output int64
}

func (p converterBootstrapProvider) InferenceLimits() (int64, int64) { return p.input, p.output }

func TestConverterBootstrapRejectsUnenforceableAllowance(t *testing.T) {
	for _, tc := range []struct {
		name          string
		input, output int64
		policy        int64
		denied        bool
	}{
		{"native_limit_matches_policy", 1048576, 8192, 8192, false},
		{"smaller_request_gets_conservative_quote", 4096, 128, 8192, false},
		{"bookkeeping_cannot_reduce_provider_output", 4096, 8193, 8192, true},
		{"missing_input_bound", 0, 8192, 8192, true},
		{"missing_output_bound", 4096, 0, 8192, true},
		{"missing_policy_bound", 4096, 128, 0, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			selected := providerfactory.Result{Provider: converterBootstrapProvider{input: tc.input, output: tc.output}, ProviderName: "fixture", ModelName: "bounded"}
			a, err := converterContinuationAllowance(selected, budget.Policy{OutputTokens: tc.policy})
			if (err != nil) != tc.denied {
				t.Fatalf("denied=%v, error=%v", tc.denied, err)
			}
			if err == nil && (a.InputTokens != tc.input || a.OutputTokens != tc.policy || a.Provider != "fixture" || a.Model != "bounded") {
				t.Fatal("prospective quote lost selected identity or finite policy allowance")
			}
		})
	}
	if _, err := converterContinuationAllowance(providerfactory.Result{}, budget.Policy{OutputTokens: 8192}); err == nil {
		t.Fatal("provider without a declared request bound was admitted")
	}
}
