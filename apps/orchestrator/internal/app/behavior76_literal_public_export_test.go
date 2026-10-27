package app

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/butler/butler/apps/orchestrator/internal/prompt"
)

// Actual public Go→JSONL→independent PS proof bridge, not a native experiment.
// The output path is new and exclusive; all data comes from declared public
// fixtures. No private auth, hidden controls or expectations are constructed.
func TestBehavior76ExportLiteralPublicProofIntegration(t *testing.T) {
	dir := os.Getenv("BUTLER_BEHAVIOR76_LITERAL_PUBLIC_PROOF_OUTPUT")
	if dir == "" {
		t.Skip("explicit new public proof export required")
	}
	if filepath.Clean(dir) != dir || !strings.HasPrefix(dir, "/src20/.cache/behavior76/literal-actual-proofs-") {
		t.Fatal("new bounded public proof path required")
	}
	if err := os.Mkdir(dir, 0700); err != nil {
		t.Fatal("public proof cannot overwrite old output")
	}
	t.Setenv("BUTLER_BEHAVIOR76_NATIVE_ENABLE", "disabled")
	t.Setenv(prompt.SettingKeyBasePrompt, prompt.DefaultBasePrompt)
	t.Setenv(prompt.SettingKeyBasePromptEnabled, "true")
	export := func(f *behavior76Fixture, name string, value behavior76LiteralEpisode, outcomes []behavior76Outcome) {
		caseDir := filepath.Join(dir, name)
		f.export(filepath.Join(caseDir, "tables"))
		behavior76Write(t, filepath.Join(caseDir, "aliases.json"), f.aliases())
		behavior76Write(t, filepath.Join(caseDir, "public-input.json"), value)
		behavior76Write(t, filepath.Join(caseDir, "actual-outcomes.json"), outcomes)
		behavior76Write(t, filepath.Join(caseDir, "actual-snapshot.json"), f.snapshot())
	}
	t.Run("kernel_ambiguity_then_selected_stop", func(t *testing.T) {
		f := newBehavior76LiteralFixture(t, context.Background(), nil)
		value := behavior76PublicLiteralInterface()[0]
		other := value.Episode.Initial.Tasks[0]
		other.Key = "other"
		value.Episode.Initial.Tasks = append(value.Episode.Initial.Tasks, other)
		value.Episode.Steps[0].RunTaskKey = nil
		value.Episode.Steps[0].Event.Text = "стоп"
		key := "other"
		value.Episode.Steps[1].RunTaskKey = &key
		value.Episode.Steps[1].Event.Text = "стоп"
		value.OwnerTaskSelections = []behavior76LiteralTaskSelection{{StepID: value.Episode.Steps[1].ID, TaskKey: key, Provenance: "explicit_owner_ui"}}
		f.seedLiteral(value)
		first, err := f.evaluateLiteral(&behavior76SetupProvider{initiativeProjectScriptedProvider: &initiativeProjectScriptedProvider{}, fixture: f}, value.Episode.Steps[0])
		if err != nil || first.KernelResponse == nil {
			t.Fatal("actual public clarification unavailable", err)
		}
		second, err := f.evaluateLiteral(&behavior76SetupProvider{initiativeProjectScriptedProvider: &initiativeProjectScriptedProvider{}, fixture: f}, value.Episode.Steps[1], value.OwnerTaskSelections[0])
		if err != nil || second.KernelResponse == nil {
			t.Fatal("actual public selected stop unavailable", err)
		}
		export(f, "kernel-stop", value, []behavior76Outcome{first, second})
	})
	t.Run("kernel_amend_failed_automatic", func(t *testing.T) {
		f := newBehavior76LiteralFixture(t, context.Background(), nil)
		value := behavior76PublicLiteralInterface()[0]
		value.Episode.Steps = value.Episode.Steps[:1]
		value.OwnerTaskSelections = value.OwnerTaskSelections[:1]
		value.Episode.Steps[0].Event.Text = "только подготовь черновик"
		f.seedLiteral(value)
		outcome, err := f.evaluateLiteral(&behavior76SetupProvider{initiativeProjectScriptedProvider: &initiativeProjectScriptedProvider{}, fixture: f}, value.Episode.Steps[0], value.OwnerTaskSelections[0])
		if err != nil || outcome.KernelResponse == nil || len(outcome.AutomaticInputs) != 1 || outcome.AutomaticInputs[0].Status != "blocked" {
			t.Fatal("actual public failed continuation unavailable", err)
		}
		export(f, "kernel-amend", value, []behavior76Outcome{outcome})
	})
	t.Run("business_read_prepare_late_commit", func(t *testing.T) {
		f := newBehavior76LiteralFixture(t, context.Background(), nil)
		value := behavior76LiteralEpisode{Version: behavior76LiteralVersion, Dimension: "deadline_amendment", Polarity: "positive", Episode: behavior76DevelopmentEpisode()}
		value.Episode.Initial.Tasks[0].ExecutionPolicy = "normal"
		value.Deadlines = []behavior76LiteralDeadline{{ResourceKey: "draft", DeadlineAt: time.Now().UTC().Add(24 * time.Hour)}}
		value.DeadlineChanges = []behavior76LiteralDeadlineChange{{BeforeStepID: value.Episode.Steps[1].ID, ResourceKey: "draft", DeadlineAt: time.Now().UTC().Add(-24 * time.Hour), Provenance: "synthetic_service_configuration"}}
		for i := range value.Episode.Steps {
			value.Episode.Steps[i].Event = behavior76Event{Type: "owner_message", Text: "Public actual proof composition only."}
			value.OwnerTaskSelections = append(value.OwnerTaskSelections, behavior76LiteralTaskSelection{StepID: value.Episode.Steps[i].ID, TaskKey: "A", Provenance: "explicit_owner_ui"})
		}
		f.seedLiteral(value)
		first, err := f.evaluateLiteral(&behavior76Scripted{initiativeProjectScriptedProvider: &initiativeProjectScriptedProvider{}}, value.Episode.Steps[0], value.OwnerTaskSelections[0])
		if err != nil || first.State != "completed" {
			t.Fatal("actual read/prepare unavailable", err)
		}
		if _, err = f.applyLiteralServiceChanges(value, value.Episode.Steps[1].ID); err != nil {
			t.Fatal(err)
		}
		second, err := f.evaluateLiteral(&behavior76Scripted{initiativeProjectScriptedProvider: &initiativeProjectScriptedProvider{}, commit: true, commitVersion: 2}, value.Episode.Steps[1], value.OwnerTaskSelections[1])
		if err != nil || second.State != "completed" {
			t.Fatal("actual late confirmed commit unavailable", err)
		}
		export(f, "business", value, []behavior76Outcome{first, second})
	})
	behavior76Write(t, filepath.Join(dir, "public-proof-receipt.json"), map[string]any{"version": "behavior76-actual-public-proof-v1", "native_calls": 0, "hidden_controls": false, "actual_certificate": false, "not_a_full_study": true, "cases": 3})
}
