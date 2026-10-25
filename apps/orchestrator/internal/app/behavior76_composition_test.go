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

// This exercises complete actual owner Input/Core/source/context preparation
// after every event. The provider is explicitly scripted and performs no tools;
// it establishes no native quality, semantic success or subscription expense.
func TestBehavior76CorpusCompositionPreflight(t *testing.T) {
	if os.Getenv("BUTLER_BEHAVIOR76_CONTROL_PREFLIGHT") != "composition-only" {
		t.Skip("explicit zero-native composition gate required")
	}
	behavior76CompositionPreflight(t, behavior76StimuliSHA)
}

func TestBehavior76FreshCorpusCompositionPreflight(t *testing.T) {
	if os.Getenv("BUTLER_BEHAVIOR76_CONTROL_PREFLIGHT") != "fresh-composition-only" {
		t.Skip("explicit fresh zero-native composition required")
	}
	behavior76CompositionPreflight(t, behavior76FreshStimuliSHA)
}

func behavior76CompositionPreflight(t *testing.T, expectedSHA string) {
	behavior76CompositionPreflightConfigured(t, expectedSHA, false)
}

func TestBehavior76LocalCorpusCompositionPreflight(t *testing.T) {
	if os.Getenv("BUTLER_BEHAVIOR76_CONTROL_PREFLIGHT") != "local-composition-only" {
		t.Skip("explicit local zero-native composition required")
	}
	behavior76CompositionPreflightConfigured(t, behavior76LocalStimuliSHA, true)
}

func behavior76CompositionPreflightConfigured(t *testing.T, expectedSHA string, local bool) {
	behavior76CompositionPreflightMode(t, expectedSHA, local, false)
}

func TestBehavior76StreamCorpusCompositionPreflight(t *testing.T) {
	if os.Getenv("BUTLER_BEHAVIOR76_CONTROL_PREFLIGHT") != "stream-composition-only" {
		t.Skip("explicit stream zero-native composition required")
	}
	behavior76CompositionPreflightMode(t, behavior76StreamStimuliSHA, true, true)
}

func behavior76CompositionPreflightMode(t *testing.T, expectedSHA string, local, streaming bool) {
	raw, err := os.ReadFile(os.Getenv("BUTLER_BEHAVIOR76_STIMULI"))
	if err != nil || behavior76Hash(raw) != expectedSHA {
		t.Fatal("frozen independent stimulus commitment unavailable")
	}
	episodes, err := behavior76ReadEpisodes(strings.NewReader(string(raw)))
	if err != nil || len(episodes) != 24 {
		t.Fatal("frozen stimulus interface unavailable")
	}
	arms, turns, steps := 0, 0, 0
	for n, e := range episodes {
		for arm := 0; arm < 2; arm++ {
			t.Run(fmt.Sprintf("slot-%02d-public-arm-%d", n+1, arm), func(t *testing.T) {
				base := prompt.DefaultBasePrompt
				if arm == 1 {
					base += "\n\n" + prompt.TaskContinuityProtocol
				}
				t.Setenv(prompt.SettingKeyBasePrompt, base)
				t.Setenv(prompt.SettingKeyBasePromptEnabled, "true")
				var f *behavior76Fixture
				var global atomic.Bool
				if streaming {
					f = newBehavior76StreamFixture(t, context.Background(), &global)
				} else if local {
					f = newBehavior76LocalFixture(t, context.Background(), &global)
				} else {
					f = newBehavior76Fixture(t)
				}
				streamDir := ""
				if streaming {
					streamDir = t.TempDir()
					if err := f.authorityTrace.enableStream(streamDir, f.scope.binding, &global); err != nil {
						t.Fatal(err)
					}
				}
				f.seed(e)
				provider := &behavior76SetupProvider{initiativeProjectScriptedProvider: &initiativeProjectScriptedProvider{}, fixture: f}
				for _, step := range e.Steps {
					if _, err := f.apply(step); err != nil {
						t.Fatal("typed event setup failed:", behavior76SafeError(err))
					}
					steps++
					if !step.Evaluate {
						continue
					}
					inactive := false
					if step.RunTaskKey != nil && *step.RunTaskKey != "" {
						r, err := f.tasks.Get(f.ctx, f.lookup("task", *step.RunTaskKey))
						if err != nil {
							t.Fatal("actual task state unavailable")
						}
						inactive = r.Status != "active"
					}
					outcome, err := f.evaluate(provider, step)
					if err != nil {
						t.Fatal("actual input/context preparation failed:", behavior76SafeError(err))
					}
					if outcome.State != "completed" && !(inactive && (outcome.State == "admission_denied" || outcome.State == "failed")) {
						t.Fatalf("unexpected preparation outcome state=%s phase=%s category=%s", outcome.State, outcome.FailurePhase, outcome.ErrorCode)
					}
					turns++
				}
				if local {
					if _, err := f.quiesce(f.ctx); err != nil || global.Load() {
						t.Fatal("local composition quiescence unavailable", err)
					}
					if streaming {
						f.streamQuiescent = true
						f.sealFixtureEvidence(streamDir)
						if global.Load() || !f.authorityTrace.snapshot().stream.Sealed {
							t.Fatal("stream composition seal unavailable")
						}
						if _, err := os.Stat(filepath.Join(streamDir, "sealed-files.json")); err != nil {
							t.Fatal(err)
						}
					}
				}
				arms++
			})
		}
	}
	if arms != 48 || turns != 96 {
		t.Fatalf("composition coverage incomplete arms=%d turns=%d", arms, turns)
	}
	t.Logf("zero-native composition:48 arms/96 owner turns/%d original steps; scripted provider only, quality unassessed", steps)
}
