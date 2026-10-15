package app

import (
	"testing"

	"github.com/butler/butler/internal/budget"
	"github.com/butler/butler/internal/memory/candidates"
	"github.com/butler/butler/internal/memory/jobs"
	"github.com/butler/butler/internal/providerfactory"
	"github.com/butler/butler/internal/transport"
	"github.com/butler/butler/internal/transport/antigravity"
)

func TestCandidateProductionReleaseCannotBeEnabledBeforeAcceptance(t *testing.T) {
	p, b := candidateQualifiedProfile()
	for _, global := range []string{"", "true"} {
		for _, flag := range []string{"", "true"} {
			t.Setenv("BUTLER_MEMORY_MODEL_JOBS_ENABLED", global)
			t.Setenv("BUTLER_MEMORY_MODEL_CANDIDATES_ENABLED", flag)
			handlers := map[string]jobs.ModelHandler{}
			configureCandidateHandler(handlers, p, b)
			want := memoryCandidateReleaseQualified && global == "true" && flag == "true"
			if (handlers[candidates.Version] != nil) != want {
				t.Fatal("release/profile/operator gates disagreed", global, flag)
			}
		}
	}
}

type candidateProfileProvider struct {
	transport.ModelProvider
	name   string
	output int64
}

type candidateProviderWithoutLimits struct{ transport.ModelProvider }

func (candidateProviderWithoutLimits) Name() string { return antigravity.Name }

func (p candidateProfileProvider) Name() string                    { return p.name }
func (p candidateProfileProvider) InferenceLimits() (int64, int64) { return 1048576, p.output }

func candidateQualifiedProfile() (providerfactory.Result, *budget.Controller) {
	p := providerfactory.Result{Provider: candidateProfileProvider{name: antigravity.Name, output: 32768}, ModelName: antigravity.FlashHighModel}
	b := &budget.Controller{Policy: budget.DefaultPolicy()}
	b.Policy.OutputTokens, b.Policy.ModelMillis = 32768, 300000
	return p, b
}

func TestCandidateProfileRequiresActualMeasuredHighAndMatchingBudget(t *testing.T) {
	for _, change := range []string{"qualified", "nil_provider", "nil_budget", "missing_limits", "wrong_provider", "wrong_model", "default8192", "larger_unmeasured_ceiling", "output_underbudget", "time_underbudget"} {
		t.Run(change, func(t *testing.T) {
			p, b := candidateQualifiedProfile()
			switch change {
			case "nil_provider":
				p.Provider = nil
			case "nil_budget":
				b = nil
			case "missing_limits":
				p.Provider = candidateProviderWithoutLimits{}
			case "wrong_provider":
				p.Provider = candidateProfileProvider{name: "other", output: 32768}
			case "wrong_model":
				p.ModelName = "other"
			case "default8192":
				p.Provider = candidateProfileProvider{name: antigravity.Name, output: 8192}
			case "larger_unmeasured_ceiling":
				p.Provider = candidateProfileProvider{name: antigravity.Name, output: 65536}
				b.Policy.OutputTokens = 65536
			case "output_underbudget":
				b.Policy.OutputTokens = 8192
			case "time_underbudget":
				b.Policy.ModelMillis = 120000
			}
			if candidateProfileReady(p, b) != (change == "qualified") {
				t.Fatal("unmeasured profile or incomplete reservation accepted", change)
			}
		})
	}
}
