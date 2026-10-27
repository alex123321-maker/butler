package app

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/butler/butler/apps/orchestrator/internal/prompt"
)

func behavior76ReadBoundLiteral(t *testing.T) []behavior76LiteralEpisode {
	t.Helper()
	if !behavior76LiteralBindingsFresh() {
		t.Fatal("fresh literal controls are not bound")
	}
	raw, err := os.ReadFile(os.Getenv("BUTLER_BEHAVIOR76_STIMULI"))
	if err != nil || behavior76Hash(raw) != behavior76LiteralStimuliSHA {
		t.Fatal("literal stimulus commitment unavailable")
	}
	values, err := behavior76ReadLiteralEpisodes(strings.NewReader(string(raw)))
	if err != nil {
		t.Fatal("complete literal interface unavailable")
	}
	return values
}

func TestBehavior76PreflightLiteralSetup(t *testing.T) {
	if os.Getenv("BUTLER_BEHAVIOR76_CONTROL_PREFLIGHT") != "literal-setup-only" {
		t.Skip("explicit zero-native literal setup gate required")
	}
	for n, value := range behavior76ReadBoundLiteral(t) {
		t.Run(fmt.Sprintf("slot-%02d", n+1), func(t *testing.T) {
			f := newBehavior76LiteralFixture(t, context.Background(), nil)
			f.seedLiteral(value)
			for _, step := range value.Episode.Steps {
				if _, err := f.applyLiteralServiceChanges(value, step.ID); err != nil {
					t.Fatal("declared literal service change unavailable")
				}
				if _, err := f.apply(step); err != nil {
					t.Fatal("actual owning setup event unavailable")
				}
			}
			if f.snapshot().LiteralService == nil {
				t.Fatal("literal snapshot unavailable")
			}
		})
	}
}

func TestBehavior76PreflightLiteralComposition(t *testing.T) {
	if os.Getenv("BUTLER_BEHAVIOR76_CONTROL_PREFLIGHT") != "literal-composition-only" {
		t.Skip("explicit zero-native literal composition gate required")
	}
	behavior76LiteralComposition(t, behavior76ReadBoundLiteral(t))
}

// The public neutral matrix exercises this same actual composition path before
// independent authorship. Neither branch reads expectations or constructs auth.
func TestBehavior76LiteralPublicCompositionIntegration(t *testing.T) {
	t.Setenv("BUTLER_BEHAVIOR76_NATIVE_ENABLE", "disabled")
	behavior76LiteralComposition(t, behavior76PublicLiteralInterface())
}

func behavior76LiteralComposition(t *testing.T, values []behavior76LiteralEpisode) {
	if _, err := behavior76LiteralDeclaredPlan(values, []string{"public-baseline", "public-candidate"}); err != nil {
		t.Fatal("full literal composition plan required")
	}
	arms, turns := 0, 0
	for n, value := range values {
		for arm := 0; arm < 2; arm++ {
			t.Run(fmt.Sprintf("slot-%02d-public-arm-%d", n+1, arm), func(t *testing.T) {
				base := prompt.DefaultBasePrompt
				if arm == 1 {
					base += "\n\n" + prompt.TaskContinuityProtocol
				}
				t.Setenv(prompt.SettingKeyBasePrompt, base)
				t.Setenv(prompt.SettingKeyBasePromptEnabled, "true")
				var global atomic.Bool
				f := newBehavior76LiteralFixture(t, context.Background(), &global)
				streamDir := t.TempDir()
				if err := f.authorityTrace.enableStream(streamDir, f.scope.binding, &global); err != nil {
					t.Fatal("composition observer startup unavailable")
				}
				f.seedLiteral(value)
				provider := &behavior76SetupProvider{initiativeProjectScriptedProvider: &initiativeProjectScriptedProvider{}, fixture: f}
				for _, step := range value.Episode.Steps {
					if _, err := f.applyLiteralServiceChanges(value, step.ID); err != nil {
						t.Fatal("composition service change unavailable")
					}
					if _, err := f.apply(step); err != nil {
						t.Fatal("composition owning event unavailable")
					}
					if !step.Evaluate {
						continue
					}
					var selections []behavior76LiteralTaskSelection
					for _, selection := range value.OwnerTaskSelections {
						if selection.StepID == step.ID {
							selections = append(selections, selection)
						}
					}
					inactive := false
					if step.RunTaskKey != nil {
						r, err := f.tasks.Get(f.ctx, f.lookup("task", *step.RunTaskKey))
						if err != nil {
							t.Fatal("composition actual task state unavailable")
						}
						inactive = r.Status != "active"
					}
					outcome, err := f.evaluateLiteral(provider, step, selections...)
					if err != nil || global.Load() {
						t.Fatal("actual literal input/context global failure")
					}
					if outcome.State != "completed" && !(outcome.KernelResponse != nil && outcome.State == "waiting_for_user") && !(inactive && outcome.State == "admission_denied") {
						t.Fatal("unexpected literal operational preparation outcome")
					}
					for _, automatic := range outcome.AutomaticInputs {
						if automatic.Status != "completed" {
							t.Fatal("automatic continuation not operationally complete")
						}
					}
					turns++
				}
				if _, err := f.quiesce(f.ctx); err != nil || global.Load() {
					t.Fatal("literal composition quiescence unavailable")
				}
				f.streamQuiescent = true
				f.sealFixtureEvidence(streamDir)
				if global.Load() || !f.authorityTrace.snapshot().stream.Sealed {
					t.Fatal("literal composition observer seal unavailable")
				}
				if _, err := os.Stat(filepath.Join(streamDir, "sealed-files.json")); err != nil {
					t.Fatal("literal composition file seal missing")
				}
				arms++
			})
		}
	}
	if arms != 32 || turns != 68 {
		t.Fatalf("literal composition incomplete arms=%d owner_opportunities=%d", arms, turns)
	}
	t.Log("actual literal composition32 arms/68 owner opportunities; scripted provider only; native/auth=0; quality unassessed")
}
