package app

import (
	"context"
	"fmt"
	"os"
	"time"

	"github.com/butler/butler/internal/budget"
	"github.com/butler/butler/internal/config"
	"github.com/butler/butler/internal/modelprovider"
	"github.com/butler/butler/internal/providerfactory"
)

// Memory generation has consumer-specific measured profiles. Selecting ACP for
// the foreground must neither qualify those consumers nor silently disable them.
// This is an explicit independent backend, never a retry or fallback route.
func selectMemoryProvider(ctx context.Context, cfg config.OrchestratorConfig, primary providerfactory.Result, builder *providerfactory.Factory, b *budget.Controller) (providerfactory.Result, error) {
	if cfg.MemoryModelProvider == "" {
		if primary.ProviderName == modelprovider.ProviderAntigravityACP && nativeMemoryRequested(cfg) {
			return providerfactory.Result{}, fmt.Errorf("ACP memory consumers require an explicitly selected qualified BUTLER_MEMORY_MODEL_PROVIDER; ACP does not inherit native memory qualification")
		}
		return primary, nil
	}
	if cfg.MemoryModelProvider == modelprovider.ProviderAntigravityACP {
		if !acpMemoryProfileReady(primary, b) || !memoryACPCandidateReleaseQualified || !memoryACPSummaryReleaseQualified || !memoryACPConsolidationReleaseQualified {
			return providerfactory.Result{}, fmt.Errorf("explicit ACP memory backend requires its independently qualified exact profile and opaque budget")
		}
		return primary, nil
	}
	if cfg.MemoryModelProvider != modelprovider.ProviderAntigravity || primary.ProviderName != modelprovider.ProviderAntigravityACP || builder == nil {
		return providerfactory.Result{}, fmt.Errorf("independent memory provider is supported only for ACP foreground with the qualified native Antigravity profile")
	}
	result, err := builder.Build(ctx, providerfactory.BuildConfig{
		ActiveProvider: cfg.MemoryModelProvider, AntigravityModel: cfg.AntigravityModel,
		AntigravityMaximumOutputTokens: cfg.AntigravityMaximumOutputTokens,
		Timeout:                        time.Duration(cfg.OpenAITimeoutSeconds) * time.Second,
	})
	if err != nil {
		return providerfactory.Result{}, err
	}
	if !candidateProfileReady(result, b) || !summaryProfileReady(result, b) {
		return providerfactory.Result{}, fmt.Errorf("independent memory provider does not match the measured native memory profile and budget")
	}
	if _, err = b.Policy.ModelQuote(result.ProviderName, result.ModelName, 1048576); err != nil {
		return providerfactory.Result{}, fmt.Errorf("independent memory provider requires its own explicit native budget basis")
	}
	return result, nil
}

func nativeMemoryRequested(cfg config.OrchestratorConfig) bool {
	if cfg.MemoryTemporalEnabled || os.Getenv("BUTLER_MEMORY_EXPERIENCE_ENABLED") == "true" {
		return true
	}
	if os.Getenv("BUTLER_MEMORY_MODEL_JOBS_ENABLED") != "true" {
		return false
	}
	for _, key := range []string{"BUTLER_MEMORY_MODEL_CANDIDATES_ENABLED", "BUTLER_MEMORY_SUMMARY_CONSUMER_ENABLED", "BUTLER_MEMORY_CONSOLIDATION_CONSUMER_ENABLED", "BUTLER_MEMORY_MODEL_QUOTE_PROBE_ENABLED"} {
		if os.Getenv(key) == "true" {
			return true
		}
	}
	return false
}
