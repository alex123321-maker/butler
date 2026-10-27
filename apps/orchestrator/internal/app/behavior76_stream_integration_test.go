package app

import (
	"context"
	"os"
	"path/filepath"
	"sync/atomic"
	"testing"
	"time"

	"github.com/butler/butler/apps/orchestrator/internal/prompt"
)

// Public controls only: real Core/Input/Broker/PostgreSQL, scripted provider,
// actual UNKNOWN then a different successful fixture. No native credentials.
func TestBehavior76AuthorityStreamFixtureIntegration(t *testing.T) {
	t.Setenv(prompt.SettingKeyBasePrompt, prompt.DefaultBasePrompt)
	t.Setenv(prompt.SettingKeyBasePromptEnabled, "true")
	ctx, cancel := context.WithTimeout(t.Context(), 90*time.Second)
	defer cancel()
	root := t.TempDir()
	if output := os.Getenv("BUTLER_BEHAVIOR76_STREAM_FIXTURE_OUTPUT"); output != "" {
		if output != "/src20/.cache/behavior76/stream-fixture-public-v1" {
			t.Fatal("fixed new public output required")
		}
		if err := os.Mkdir(output, 0700); err != nil {
			t.Fatal(err)
		}
		root = output
	}
	var global atomic.Bool
	for n, foreign := range []bool{true, false} {
		name := []string{"unknown", "next-success"}[n]
		t.Run(name, func(t *testing.T) {
			f := newBehavior76StreamFixture(t, ctx, &global)
			dir := filepath.Join(root, name)
			if err := os.Mkdir(dir, 0700); err != nil {
				t.Fatal(err)
			}
			if err := f.authorityTrace.enableStream(dir, f.scope.binding, &global); err != nil {
				t.Fatal(err)
			}
			episode := behavior76DevelopmentEpisode()
			base := &behavior76PublicHandoff{initiativeProjectScriptedProvider: &initiativeProjectScriptedProvider{}}
			if foreign {
				other := episode.Initial.Tasks[0]
				other.Key = "B"
				episode.Initial.Tasks = append(episode.Initial.Tasks, other)
				episode.Initial.Resources = append(episode.Initial.Resources, behavior76Resource{Key: "foreign", TaskKey: "B", Body: "public separate object", Version: 1})
				base.resource = "foreign"
			}
			f.seed(episode)
			provider := behavior76PublicWrapped(t, f, &global, base)
			outcome, err := f.evaluate(provider, episode.Steps[0])
			expected := "completed"
			if foreign {
				expected = "fixture_local_unknown"
			}
			if err != nil || outcome.State != expected || global.Load() {
				t.Fatal("actual fixture outcome", outcome.State, err)
			}
			if foreign {
				tail, e := f.evaluate(provider, episode.Steps[1])
				if e != nil || tail.State != "not_executed_after_fixture_local_unknown" || tail.OwnerInputID != "" {
					t.Fatal("stopped tail admission", e)
				}
			}
			receipt, err := f.quiesce(ctx)
			if err != nil {
				t.Fatal("actual worker/backend quiescence", err)
			}
			f.streamQuiescent = true
			behavior76Write(t, filepath.Join(dir, "quiescence.json"), receipt)
			behavior76Write(t, filepath.Join(dir, "outcome.json"), outcome)
			f.export(filepath.Join(dir, "tables"))
			f.sealFixtureEvidence(dir)
			trace := f.authorityTrace.snapshot()
			if global.Load() || trace.stream == nil || !trace.stream.Sealed || trace.stream.EventCount == 0 || trace.OpenSpans != 0 || trace.Dropped != 0 || trace.Failed {
				t.Fatal("actual stream evidence failed")
			}
			t.Logf("public %s: %d original events / %d segments; actual quiescence; native/auth=0", name, trace.stream.EventCount, len(trace.stream.Segments))
		})
		if global.Load() || t.Failed() {
			t.Fatal("global fault prevents next fixture")
		}
	}
}
