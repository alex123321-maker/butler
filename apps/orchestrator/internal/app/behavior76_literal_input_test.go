package app

import (
	"testing"

	"github.com/butler/butler/apps/orchestrator/internal/prompt"
)

// These are public composition controls, not the future hidden comparison.
// Actual Input/Controller/PG are used with a scripted provider, never Gemini.
func TestBehavior76LiteralKernelInputIntegration(t *testing.T) {
	t.Setenv("BUTLER_BEHAVIOR76_NATIVE_ENABLE", "disabled")
	t.Setenv(prompt.SettingKeyBasePrompt, prompt.DefaultBasePrompt)
	t.Setenv(prompt.SettingKeyBasePromptEnabled, "true")
	t.Run("ambiguous_then_explicit_stop", func(t *testing.T) {
		f := newBehavior76Fixture(t)
		e := behavior76PublicLiteralInterface()[0].Episode
		second := e.Initial.Tasks[0]
		second.Key = "other"
		e.Initial.Tasks = append(e.Initial.Tasks, second)
		f.seed(e)
		provider := &behavior76SetupProvider{initiativeProjectScriptedProvider: &initiativeProjectScriptedProvider{}, fixture: f}
		step := behavior76Step{ID: "public-ambiguous-stop", Evaluate: true, Event: behavior76Event{Type: "owner_message", Text: "стоп"}}
		outcome, err := f.evaluateLiteral(provider, step)
		if err != nil || outcome.KernelResponse == nil || outcome.KernelResponse.Kind != "task_target_clarification" || outcome.State != "waiting_for_user" || outcome.OwnerInputID != "" || outcome.OwnerRunID != "" || len(outcome.RunIDs) != 0 || len(provider.requests) != 0 {
			t.Fatal("actual kernel clarification not separated from model output", err)
		}
		for _, key := range []string{"work", "other"} {
			r, err := f.tasks.Get(f.ctx, f.lookup("task", key))
			if err != nil || r.Status != "active" || r.Revision != 1 {
				t.Fatal("ambiguous ordinary input changed task authority", err)
			}
		}
		key := "other"
		step.ID, step.RunTaskKey = "public-explicit-stop", &key
		outcome, err = f.evaluateLiteral(provider, step, behavior76LiteralTaskSelection{StepID: step.ID, TaskKey: key, Provenance: "explicit_owner_ui"})
		if err != nil || outcome.KernelResponse == nil || outcome.KernelResponse.Kind != "task_control" || outcome.KernelResponse.ControlID == "" || outcome.State != "completed" || outcome.OwnerInputID != "" || len(outcome.RunIDs) != 0 || len(provider.requests) != 0 {
			t.Fatal("ordinary explicit stop lacks actual kernel control receipt", err)
		}
		selected, err := f.tasks.Get(f.ctx, f.lookup("task", "other"))
		if err != nil || selected.Status != "cancelled" || selected.Revision != 2 {
			t.Fatal("selected ordinary stop not applied", err)
		}
		untouched, err := f.tasks.Get(f.ctx, f.lookup("task", "work"))
		if err != nil || untouched.Status != "active" || untouched.Revision != 1 {
			t.Fatal("explicit stop changed the other task", err)
		}
		var effects int
		if err := f.pool.QueryRow(f.ctx, `SELECT count(*) FROM action_effect_dispatches`).Scan(&effects); err != nil || effects != 0 {
			t.Fatal("public kernel-only input acquired a tool effect", err)
		}
	})
	t.Run("amend_continuation_is_automatic", func(t *testing.T) {
		f := newBehavior76Fixture(t)
		e := behavior76PublicLiteralInterface()[0].Episode
		f.seed(e)
		provider := &behavior76SetupProvider{initiativeProjectScriptedProvider: &initiativeProjectScriptedProvider{}, fixture: f}
		key := "work"
		step := behavior76Step{ID: "public-raw-prepare-only", Evaluate: true, RunTaskKey: &key, Event: behavior76Event{Type: "owner_message", Text: "только подготовь черновик"}}
		outcome, err := f.evaluateLiteral(provider, step, behavior76LiteralTaskSelection{StepID: step.ID, TaskKey: key, Provenance: "explicit_owner_ui"})
		if err != nil || outcome.KernelResponse == nil || outcome.KernelResponse.Kind != "task_control" || outcome.KernelResponse.ContinuationInputID == "" || outcome.OwnerInputID != "" || outcome.OwnerRunID != "" || len(outcome.RunIDs) != 1 || len(outcome.AutomaticRunIDs) != 1 || outcome.RunIDs[0] != outcome.AutomaticRunIDs[0] || len(provider.requests) != 0 || len(outcome.AutomaticInputs) != 1 || outcome.AutomaticInputs[0].Status != "blocked" || outcome.AutomaticInputs[0].ErrorCode == "" {
			t.Fatal("actual automatic continuation mistaken for owner model answer or not drained", err)
		}
		var status, actualRun string
		if err := f.pool.QueryRow(f.ctx, `SELECT status,coalesce(run_id,'') FROM accepted_inputs WHERE input_id=$1`, outcome.KernelResponse.ContinuationInputID).Scan(&status, &actualRun); err != nil || status != "blocked" || actualRun != outcome.AutomaticRunIDs[0] || actualRun != outcome.AutomaticInputs[0].RunID || outcome.AutomaticInputs[0].InputID != outcome.KernelResponse.ContinuationInputID {
			t.Fatal("continuation lacks actual blocked input/run proof", err)
		}
		var runState string
		if err := f.pool.QueryRow(f.ctx, `SELECT current_state FROM runs WHERE run_id=$1`, actualRun).Scan(&runState); err != nil || runState != "failed" {
			t.Fatal("blocked input lost its actual failed run", err)
		}
		var modelAttempts, currentMandates int
		if err := f.pool.QueryRow(f.ctx, `SELECT count(*) FROM execution_model_attempts WHERE run_id=$1`, actualRun).Scan(&modelAttempts); err != nil || modelAttempts != 0 {
			t.Fatal("catalog-denied continuation entered a model", err)
		}
		if err := f.pool.QueryRow(f.ctx, `SELECT count(*) FROM behavior76_task_mandates m JOIN tasks t USING(task_id) WHERE m.task_revision=t.revision`).Scan(&currentMandates); err != nil || currentMandates != 0 {
			t.Fatal("test silently reissued authority after owner amendment", err)
		}
		var controlTranscript int
		if err := f.pool.QueryRow(f.ctx, `SELECT count(*) FROM messages WHERE session_key=$1 AND metadata->>'control_id'=$2`, outcome.KernelResponse.SessionKey, outcome.KernelResponse.ControlID).Scan(&controlTranscript); err != nil || controlTranscript != 1 {
			t.Fatal("raw owner amendment lacks actual control transcript", err)
		}
	})
}
