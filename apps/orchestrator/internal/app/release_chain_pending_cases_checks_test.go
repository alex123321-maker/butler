package app

import (
	"reflect"
	"testing"

	"github.com/butler/butler/internal/budget"
	"github.com/butler/butler/internal/transport/antigravity"
)

func TestReleaseChainPendingCasesDeclarationPreservesLegacy(t *testing.T) {
	legacy, err := releaseChainHighDeclaredScenarios(releaseChainHighEntry{})
	if err != nil || !reflect.DeepEqual(legacy, releaseChainV5Scenarios()) || len(legacy) != 5 {
		t.Fatal("legacy five cases changed")
	}
	entry := releaseChainHighEntry{ScenarioNames: []string{"restart_sent", "explicit_absence", "lost_ack", "owner_stop"}}
	fixed, err := releaseChainHighDeclaredScenarios(entry)
	if err != nil || !reflect.DeepEqual(fixed, entry.ScenarioNames) || len(fixed) != 4 {
		t.Fatal("fixed remaining-case declaration denied")
	}
	fixed[0] = "foreign"
	if entry.ScenarioNames[0] != "restart_sent" {
		t.Fatal("returned slice changed the frozen declaration")
	}
	for _, names := range [][]string{{}, {"restart_sent"}, {"normal_sent", "restart_sent", "explicit_absence", "lost_ack", "owner_stop"}, {"owner_stop", "restart_sent", "explicit_absence", "lost_ack"}, {"restart_sent", "explicit_absence", "lost_ack", "lost_ack"}} {
		if _, err := releaseChainHighDeclaredScenarios(releaseChainHighEntry{ScenarioNames: names}); err == nil {
			t.Fatal("arbitrary, repeated, completed or reordered scenario accepted")
		}
	}
}

func TestReleaseChainDeclaredInputAllowanceBoundsAndQuotes(t *testing.T) {
	legacy := releaseChainV5Configuration()
	if err := releaseChainHighApplyInputAllowance(releaseChainHighEntry{}, &legacy); err != nil || legacy.Policy.ModelInputBytes != 131072 || legacy.Policy.Day.ProviderInputBytes != 268435456 || budget.DefaultPolicy().ModelInputBytes != 8<<20 {
		t.Fatal("legacy or production input allowance changed")
	}
	entry := releaseChainHighEntry{Maximum: 120, ProviderInputBytes: 4 << 20, Policy: releaseChainV9Policy()}
	cfg := releaseChainHighConfigurationFor(entry.Policy)
	before := cfg.ChildBudget
	if err := releaseChainHighApplyInputAllowance(entry, &cfg); err != nil || cfg.Policy.ModelInputBytes != 4<<20 || cfg.Policy.Day.ProviderInputBytes != 120*(4<<20) || !reflect.DeepEqual(before, cfg.ChildBudget) {
		t.Fatal("declared finite profile changed another child resource", err)
	}
	quote, err := cfg.Policy.ModelQuote(antigravity.Name, antigravity.FlashHighModel, cfg.InputTokens)
	if err != nil || quote.ProviderInputBytes != 4<<20 || cfg.ChildBudget.ProviderInputBytes != 4*quote.ProviderInputBytes || cfg.Policy.Task.ProviderInputBytes < cfg.Policy.Task.ModelTurns*quote.ProviderInputBytes || cfg.Policy.Mandate.ProviderInputBytes < cfg.Policy.Mandate.ModelTurns*quote.ProviderInputBytes {
		t.Fatal("per-request allowance does not fit existing full finite scope quotes", err)
	}
	for _, change := range []func(*releaseChainHighEntry){func(e *releaseChainHighEntry) { e.ProviderInputBytes = 1 << 20 }, func(e *releaseChainHighEntry) { e.ProviderInputBytes = 8 << 20 }, func(e *releaseChainHighEntry) { e.Maximum = 80 }, func(e *releaseChainHighEntry) { e.Policy = releaseChainHighWindowPolicy{} }} {
		bad := entry
		change(&bad)
		candidate := releaseChainV5Configuration()
		if releaseChainHighApplyInputAllowance(bad, &candidate) == nil || candidate.Policy.ModelInputBytes != 131072 {
			t.Fatal("arbitrary profile enlarged or changed legacy allowance")
		}
	}
	if releaseChainHighApplyInputAllowance(entry, nil) == nil {
		t.Fatal("missing trusted configuration accepted")
	}
}
