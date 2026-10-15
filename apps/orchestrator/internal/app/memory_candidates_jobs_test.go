package app

import (
	"testing"
	"time"

	"github.com/butler/butler/internal/budget"
	"github.com/butler/butler/internal/memory/candidates"
	"github.com/butler/butler/internal/memory/jobs"
	"github.com/butler/butler/internal/providerauth"
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
	name    string
	output  int64
	timeout time.Duration
}

type candidateProviderWithoutLimits struct{ transport.ModelProvider }

func (candidateProviderWithoutLimits) Name() string { return antigravity.Name }

type candidateProviderWithoutTimeout struct{ transport.ModelProvider }

func (candidateProviderWithoutTimeout) Name() string                    { return antigravity.Name }
func (candidateProviderWithoutTimeout) InferenceLimits() (int64, int64) { return 1048576, 32768 }

func (p candidateProfileProvider) Name() string                    { return p.name }
func (p candidateProfileProvider) InferenceLimits() (int64, int64) { return 1048576, p.output }
func (p candidateProfileProvider) InferenceTimeout() time.Duration { return p.timeout }

func candidateQualifiedProfile() (providerfactory.Result, *budget.Controller) {
	p := providerfactory.Result{Provider: candidateProfileProvider{name: antigravity.Name, output: 32768, timeout: 300 * time.Second}, ModelName: antigravity.FlashHighModel}
	b := &budget.Controller{Policy: budget.DefaultPolicy()}
	b.Policy.OutputTokens, b.Policy.ModelMillis = 32768, 300000
	return p, b
}

func TestCandidateProfileRequiresActualMeasuredHighAndMatchingBudget(t *testing.T) {
	for _, change := range []string{"qualified", "nil_provider", "nil_budget", "missing_limits", "missing_timeout", "short_native_timeout", "wrong_provider", "wrong_model", "default8192", "larger_unmeasured_ceiling", "output_underbudget", "time_underbudget"} {
		t.Run(change, func(t *testing.T) {
			p, b := candidateQualifiedProfile()
			switch change {
			case "nil_provider":
				p.Provider = nil
			case "nil_budget":
				b = nil
			case "missing_limits":
				p.Provider = candidateProviderWithoutLimits{}
			case "missing_timeout":
				p.Provider = candidateProviderWithoutTimeout{}
			case "short_native_timeout":
				p.Provider = candidateProfileProvider{name: antigravity.Name, output: 32768, timeout: 299 * time.Second}
			case "wrong_provider":
				p.Provider = candidateProfileProvider{name: "other", output: 32768, timeout: 300 * time.Second}
			case "wrong_model":
				p.ModelName = "other"
			case "default8192":
				p.Provider = candidateProfileProvider{name: antigravity.Name, output: 8192, timeout: 300 * time.Second}
			case "larger_unmeasured_ceiling":
				p.Provider = candidateProfileProvider{name: antigravity.Name, output: 65536, timeout: 300 * time.Second}
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

func TestCandidateNativeProfileUsesActualAdapterTimeout(t *testing.T) {
	for _, timeout := range []time.Duration{60 * time.Second, 120 * time.Second, 299 * time.Second, 300 * time.Second, 301 * time.Second} {
		t.Run(timeout.String(), func(t *testing.T) {
			p, b := candidateQualifiedProfile()
			native, err := antigravity.NewProvider(antigravity.Config{Model: antigravity.FlashHighModel, MaximumOutputTokens: 32768, Timeout: timeout, AuthSource: providerauth.NewManager(nil)})
			if err != nil {
				t.Fatal(err)
			}
			p.Provider = native
			if candidateProfileReady(p, b) != (timeout >= 300*time.Second) {
				t.Fatal("actual native timeout disagrees with qualification", timeout)
			}
		})
	}
}
