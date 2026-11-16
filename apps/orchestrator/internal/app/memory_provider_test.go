package app

import (
	"context"
	"testing"

	"github.com/butler/butler/internal/budget"
	"github.com/butler/butler/internal/config"
	"github.com/butler/butler/internal/modelprovider"
	"github.com/butler/butler/internal/providerauth"
	"github.com/butler/butler/internal/providerfactory"
	"github.com/butler/butler/internal/transport/antigravity"
)

func TestACPMemorySelectionPreservesQualifiedNativeConsumers(t *testing.T) {
	t.Setenv("BUTLER_MEMORY_MODEL_JOBS_ENABLED", "true")
	t.Setenv("BUTLER_MEMORY_MODEL_CANDIDATES_ENABLED", "true")
	t.Setenv("BUTLER_MEMORY_SUMMARY_CONSUMER_ENABLED", "true")
	p, b := candidateQualifiedProfile()
	b.Policy.Models = map[string]budget.ModelPrice{antigravity.Name + ":" + antigravity.FlashHighModel: {Basis: "subscription_no_overage"}}
	primary := providerfactory.Result{ProviderName: modelprovider.ProviderAntigravityACP, ModelName: "gemini-3.8-flash"}
	cfg := config.OrchestratorConfig{MemoryModelProvider: "antigravity", AntigravityModel: antigravity.FlashHighModel, AntigravityMaximumOutputTokens: 32768, OpenAITimeoutSeconds: 300, MemoryTemporalEnabled: true}
	builder := providerfactory.New(providerauth.NewManager(nil), nil)
	got, err := selectMemoryProvider(context.Background(), cfg, primary, builder, b)
	if err != nil || !summaryProfileReady(got, b) || !candidateProfileReady(got, b) || !temporalReadConfigured(true, got, b) || got.ProviderName != antigravity.Name {
		t.Fatal("qualified background capabilities lost", err)
	}
	for _, change := range []string{"unset", "unqualified", "missing_basis", "other_provider", "wrong_foreground"} {
		t.Run(change, func(t *testing.T) {
			c, foreground := cfg, primary
			policy := b.Policy
			controller := &budget.Controller{Policy: policy}
			switch change {
			case "unset":
				c.MemoryModelProvider = ""
			case "unqualified":
				c.AntigravityMaximumOutputTokens = 8192
			case "missing_basis":
				controller.Policy.Models = map[string]budget.ModelPrice{}
			case "other_provider":
				c.MemoryModelProvider = "openai"
			case "wrong_foreground":
				foreground = p
			}
			if _, err := selectMemoryProvider(context.Background(), c, foreground, builder, controller); err == nil {
				t.Fatal("memory downgrade or implicit fallback accepted")
			}
		})
	}
}

func TestMemoryProviderDefaultPreservesExistingComposition(t *testing.T) {
	p, b := candidateQualifiedProfile()
	got, err := selectMemoryProvider(context.Background(), config.OrchestratorConfig{}, p, nil, b)
	if err != nil || got.Provider != p.Provider || got.ModelName != p.ModelName {
		t.Fatal("existing provider changed", err)
	}
}
