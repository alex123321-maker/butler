package app

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/butler/butler/internal/budget"
	"github.com/butler/butler/internal/config"
	"github.com/butler/butler/internal/memory/catalog"
	"github.com/butler/butler/internal/memory/consolidation"
	memorydoctor "github.com/butler/butler/internal/memory/doctor"
	"github.com/butler/butler/internal/memory/experience"
	"github.com/butler/butler/internal/memory/memorytest"
	"github.com/butler/butler/internal/memory/readcontrol"
	"github.com/butler/butler/internal/providerfactory"
	"github.com/butler/butler/internal/transport/antigravity"
)

// Constants stay authoritative. After a separately reviewed qualification the
// same positive rows execute real constructors, rather than skipping or testing
// an invalid environment that happens to return the expected error.
func TestMemoryEffectiveConstructorAndDoctorMatrixIntegration(t *testing.T) {
	pool := memorytest.Database(t)
	cat := catalog.New(pool, nil)
	profiles := []struct {
		name   string
		change func(*providerfactory.Result, *budget.Controller)
		ready  bool
	}{
		{name: "supported", ready: true},
		{name: "wrong_model", change: func(p *providerfactory.Result, _ *budget.Controller) { p.ModelName = antigravity.DefaultModel }},
		{name: "wrong_output", change: func(p *providerfactory.Result, _ *budget.Controller) {
			p.Provider = candidateProfileProvider{name: antigravity.Name, output: 8192, timeout: 300 * time.Second}
		}},
		{name: "short_timeout", change: func(p *providerfactory.Result, _ *budget.Controller) {
			p.Provider = candidateProfileProvider{name: antigravity.Name, output: 32768, timeout: 299 * time.Second}
		}},
		{name: "short_budget", change: func(_ *providerfactory.Result, b *budget.Controller) { b.Policy.ModelMillis = 299999 }},
		{name: "small_quote", change: func(_ *providerfactory.Result, b *budget.Controller) { b.Policy.OutputTokens = 8192 }},
	}
	for _, profile := range profiles {
		for _, enabled := range []bool{false, true} {
			for _, worker := range []bool{false, true} {
				name := profile.name + "/feature_" + boolString(enabled) + "/jobs_" + boolString(worker)
				t.Run(name, func(t *testing.T) {
					t.Setenv("BUTLER_MEMORY_EXPERIENCE_ENABLED", boolString(enabled))
					t.Setenv("BUTLER_MEMORY_EXPERIENCE_ENVIRONMENT_HASH", strings.Repeat("b", 64))
					t.Setenv("BUTLER_MEMORY_MODEL_JOBS_ENABLED", boolString(worker))
					t.Setenv("BUTLER_MEMORY_CONSOLIDATION_CONSUMER_ENABLED", boolString(enabled))
					t.Setenv("BUTLER_MEMORY_MODEL_CANDIDATES_ENABLED", "false")
					t.Setenv("BUTLER_MEMORY_SUMMARY_CONSUMER_ENABLED", "false")
					t.Setenv("BUTLER_MEMORY_MODEL_QUOTE_PROBE_ENABLED", "false")
					p, b := candidateQualifiedProfile()
					if profile.change != nil {
						profile.change(&p, b)
					}
					store, err := configureExperience(cat, experienceToolsFixture{readcontrol.Contract(readcontrol.Open)}, p, b)
					wantContext := enabled && experience.ReleaseQualified && profile.ready
					if (store != nil) != wantContext || (err != nil) != (enabled && !wantContext) || err != nil && !errors.Is(err, experience.ErrEvidence) {
						t.Fatal("explicit experience constructor did not fail closed", store != nil, err)
					}
					_, w := configureModelJobs(cat, b, p)
					if modelHandlerConfigured(w, experience.Version) != (worker && wantContext) || modelHandlerConfigured(w, consolidation.Version) != (worker && enabled && consolidation.ReleaseQualified && profile.ready) {
						t.Fatal("actual model handler profile/flags differ")
					}
					reads, err := NewMemoryReads(pool, cat, AdaptiveReadOptions{Experience: store})
					if err != nil {
						t.Fatal(err)
					}
					reads.temporal = temporalReadConfigured(enabled, p, b)
					if reads.temporal != (enabled && temporalReleaseQualified && profile.ready) {
						t.Fatal("temporal consumer accepted an unsupported profile")
					}
					states := memoryFeatureConfigurations(config.OrchestratorConfig{MemoryReadEnabled: true, MemoryTemporalEnabled: enabled}, p, b, w, false, reads != nil, false, store != nil)
					byName := map[string]memorydoctor.FeatureConfiguration{}
					for _, state := range states {
						byName[state.Name] = state
					}
					contextState, generation := byName["experience_context"], byName["experience"]
					if contextState.Requested != enabled || contextState.Wired != wantContext || contextState.Qualified != experience.ReleaseQualified || contextState.ProfileReady != profile.ready || contextState.Handler != "" || generation.Requested != (worker && enabled) || generation.Wired != (worker && wantContext) || generation.Handler != experience.Version {
						t.Fatal("Doctor merged context availability with background generation", contextState, generation)
					}
					if byName["temporal_facts"].Wired != reads.temporal {
						t.Fatal("Doctor and actual temporal reader differ")
					}
					reporter := memorydoctor.NewReporter(nil, nil)
					reporter.Configure(memorydoctor.Options{Features: states})
					report, err := reporter.Report(context.Background())
					if err != nil || report["answer_quality"].(map[string]any)["status"] != "unverified" {
						t.Fatal("construction became answer quality", err)
					}
					features := report["effective_features"].(map[string]any)["features"].([]map[string]any)
					found := false
					for _, feature := range features {
						if feature["configuration"].(memorydoctor.FeatureConfiguration).Name == "experience_context" {
							found = true
							if !enabled && feature["state"] != "disabled" {
								t.Fatal("default-off context appeared active")
							}
						}
					}
					if !found {
						t.Fatal("Doctor discarded actual context feature")
					}
				})
			}
		}
	}
}

func boolString(v bool) string {
	if v {
		return "true"
	}
	return "false"
}

func TestExperienceDefaultOffRetainsOrdinarySourceGuardIntegration(t *testing.T) {
	t.Setenv("BUTLER_MEMORY_EXPERIENCE_ENABLED", "false")
	f := newNamedToolControlDispatchFixture(t, nil, false, false, readcontrol.Open)
	prepareMemoryReadCheckpoint(t, f)
	p, b := candidateQualifiedProfile()
	store, err := configureExperience(f.f.sources, experienceToolsFixture{readcontrol.Contract(readcontrol.Open)}, p, b)
	if err != nil || store != nil {
		t.Fatal("default-off experience constructor changed", err)
	}
	guard := experienceRunSources{sources: f.f.sources, catalog: f.f.sources, store: store}
	if err = guard.CheckRunSources(f.owned, f.call.RunId); err != nil {
		t.Fatal("default-off blocked ordinary source", err)
	}
	ref := f.args.Sources[0]
	if _, err = f.f.sources.Delete(f.f.owner, ref.ID, "delete", ref.Revision); err != nil {
		t.Fatal(err)
	}
	if guard.CheckRunSources(f.owned, f.call.RunId) == nil {
		t.Fatal("default-off removed current source guard")
	}
}
