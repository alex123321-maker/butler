package app

import (
	"context"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/butler/butler/apps/orchestrator/internal/prompt"
	"github.com/jackc/pgx/v5/pgxpool"
)

// No synthetic positive metadata can replace the unknown actual history/PASS.
// Extend the exact metadata tamper matrix only after root installs real pins.
func TestReleaseChainV5BridgeUnknownActualHistoryDeniesBeforeProvider(t *testing.T) {
	if releaseChainV5ReviewedPrerequisiteSHA == "" {
		if releaseChainV5Baseline != 0 || releaseChainV5ReviewedQualityHistoryArtifactSHA != "" {
			t.Fatal("draft invented a baseline or actual history pin")
		}
		if _, err := newReleaseChainV5Window(context.Background(), &pgxpool.Pool{}); err == nil {
			t.Fatal("unset bridge touched a synthetic pool")
		}
		if _, err := newReleaseChainV5NativeProvider(context.Background(), &pgxpool.Pool{}, "must-not-read-import", "must-not-create-key"); err == nil {
			t.Fatal("unset bridge touched credentials or a provider")
		}
	}
	if _, err := createReleaseChainV5Window(context.Background(), &pgxpool.Pool{}, "", "", 0, time.Minute); err == nil {
		t.Fatal("zero scope created durable state")
	}
}

func TestReleaseChainV5ProspectiveHealthyThenFaultsAndFiniteBudgets(t *testing.T) {
	want := []string{"normal_sent", "restart_sent", "explicit_absence", "lost_ack", "owner_stop"}
	got := releaseChainV5Scenarios()
	if !reflect.DeepEqual(got, want) || releaseChainV5NewCap != 80 {
		t.Fatal("healthy baseline, fault order or prospective finite cap changed")
	}
	for _, scenario := range want {
		if !releaseChainV5ScenarioKnown(scenario) {
			t.Fatal("a required scenario is denied by admission")
		}
	}
	if releaseChainV5ScenarioKnown("unknown") {
		t.Fatal("unreviewed scenario can enter the frozen window")
	}
	t.Setenv(prompt.SettingKeyBasePrompt, releaseChainV5BasePrompt())
	t.Setenv(prompt.SettingKeyBasePromptEnabled, "true")
	state, err := prompt.NewStaticManager().Get(context.Background())
	if err != nil || !state.Enabled || state.EffectivePrompt != prompt.DefaultBasePrompt+"\n\n"+releaseChainV5ToolPlanningPolicy {
		t.Fatal("declared serial planning policy did not enter ordinary operator settings")
	}
	trusted := prompt.NewAssembler().Assemble(state, prompt.Context{}).TrustedInstructions()
	if strings.Count(trusted, releaseChainV5ToolPlanningPolicy) != 1 || !strings.HasPrefix(trusted, prompt.DefaultBasePrompt) {
		t.Fatal("ordinary Core prompt assembly lost or rewrote the declared policy")
	}
	got[0] = "changed"
	if !reflect.DeepEqual(releaseChainV5Scenarios(), want) {
		t.Fatal("one caller can mutate prospective scenario order")
	}
	cfg := releaseChainV5Configuration()
	p := cfg.Policy
	if err := p.Validate(); err != nil {
		t.Fatal("ordinary Core resource policy invalid", err)
	}
	// Separate isolated scenarios retain normal Task/Mandate/Day admission.
	// The one shared native cap supplements those ledgers; it cannot grant them.
	quote := int64(1048576 + 32768)
	if p.Task.ModelTurns != 24 || p.Mandate.ModelTurns != 40 || p.Day.ModelTurns != 80 || p.Conversation != p.Task ||
		p.Task.Tokens != 24*quote || p.Mandate.Tokens != 40*quote || p.Day.Tokens != 80*quote ||
		p.Task.ActiveMillis != 24*120000 || p.Mandate.ActiveMillis != 40*120000 || p.Day.ActiveMillis != 80*120000 ||
		p.ModelMillis != 120000 || p.OutputTokens != 32768 || p.ModelInputBytes != 131072 || p.TaskLifetimeMillis != 1200000 ||
		cfg.FixtureTimeout != 20*time.Minute || cfg.InputTokens != 1048576 || cfg.OutputTokens != 32768 ||
		cfg.ChildBudget.ModelTurns != 4 || cfg.ChildBudget.Tokens != 4*quote || cfg.ChildBudget.ActiveMillis != 600000 || cfg.ChildBudget.ExternalCalls != 4 {
		t.Fatal("frozen finite root/child resources differ")
	}
}
