package app

import (
	"time"

	"github.com/butler/butler/internal/budget"
	"github.com/butler/butler/internal/modelprovider"
	"github.com/butler/butler/internal/providerfactory"
	"github.com/butler/butler/internal/transport"
)

// Independent ACP compatibility gates: fixed 32-case extraction, exact-prefix
// compaction/restart/retained-timeout recovery, duplicate/UNKNOWN consolidation,
// and live cancellation/revocation. See issue-200-acp-laba-20261116.md.
// These do not claim native token/cost/latency parity or qualify experience.
const memoryACPCandidateReleaseQualified = true
const memoryACPSummaryReleaseQualified = true
const memoryACPConsolidationReleaseQualified = true

func acpMemoryProfileReady(p providerfactory.Result, b *budget.Controller) bool {
	if p.Provider == nil || b == nil || p.ProviderName != modelprovider.ProviderAntigravityACP || p.Provider.Name() != modelprovider.ProviderAntigravityACP || p.ModelName != "gemini-3.8-flash" || b.Policy.ModelMillis < 300000 {
		return false
	}
	limited, ok := p.Provider.(interface {
		AgentExecutionLimits() transport.AgentExecutionLimits
	})
	if !ok {
		return false
	}
	limits := limited.AgentExecutionLimits()
	if !limits.Valid() || limits.WallTime != 300*time.Second || limits.InputBytes != 2<<20 || limits.OutputBytes != 64<<10 {
		return false
	}
	policy := b.Policy
	policy.ModelMillis = 300000 // Same five-minute clamp as the owning worker.
	quote, err := policy.AgentQuote(p.ProviderName, p.ModelName)
	return err == nil && quote.Tokens == 0 && quote.CostMicros == 0 && quote.ModelTurns == 1 && quote.ProviderInputBytes >= limits.InputBytes && quote.EgressBytes >= limits.OutputBytes
}
