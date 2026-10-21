package communication_test

import (
	"context"
	"github.com/butler/butler/apps/orchestrator/internal/approval"
	"github.com/butler/butler/apps/orchestrator/internal/run"
	"github.com/butler/butler/apps/orchestrator/internal/task"
	"github.com/butler/butler/internal/communicationcontrol"
	"github.com/butler/butler/internal/memory/transcript"
	"github.com/butler/butler/internal/owneridentity"
	"testing"
)

func TestCommunicationTaskPauseResponseAndFreshResumeIntegration(t *testing.T) {
	for _, respondWhilePaused := range []bool{true, false} {
		t.Run(map[bool]string{true: "answer_during_pause", false: "resume_still_waits"}[respondWhilePaused], func(t *testing.T) {
			f := setup(t, fixtureOptions{Task: true})
			r, e := f.publish(t, "question", "communication.widget", widget)
			if e != nil {
				t.Fatal(e)
			}
			if e = f.s.Inputs.Finish(f.ctx, f.claim, "waiting_for_user", "", "", 0); e != nil {
				t.Fatal(e)
			}
			tasks := task.NewStore(f.pool)
			ctrl := task.NewController(tasks, f.leases.Authority(), f.leases, run.NewPostgresRepository(f.pool), approval.NewPostgresRepository(f.pool), f.s.Inputs, transcript.NewStore(f.pool))
			ctrl.SetInteractionControls(f.s)
			paused, e := ctrl.Apply(controlActor(f, "pause"), task.ControlRequest{TaskID: f.p.TaskID, ExpectedRevision: f.p.TaskRevision, Kind: task.ControlPause})
			if e != nil {
				t.Fatal("real owner pause", e)
			}
			if respondWhilePaused {
				if _, e = f.s.Respond(f.actor("telegram", "paused-answer"), r.InteractionRef, f.bindings["telegram"], 1, 1, communicationcontrol.Answer{OptionIDs: []string{"a"}}); e != nil {
					t.Fatal("paused response lost", e)
				}
			}
			resumed, e := ctrl.Apply(controlActor(f, "resume"), task.ControlRequest{TaskID: f.p.TaskID, ExpectedRevision: paused.Task.Revision, Kind: task.ControlResume})
			if e != nil {
				t.Fatal("real owner resume", e)
			}
			fresh, e := f.s.Inputs.Get(f.ctx, resumed.ContinuationInputID)
			if e != nil {
				t.Fatal(e)
			}
			if respondWhilePaused {
				if fresh.Status != "queued" {
					t.Fatal("owner resume did not use fresh queued input", fresh.Status)
				}
			} else {
				if fresh.Status != "waiting_for_user" {
					t.Fatal("owner resume executed before unanswered preference", fresh.Status)
				}
				if _, e = f.s.Respond(f.actor("telegram", "later-answer"), r.InteractionRef, f.bindings["telegram"], 1, 1, communicationcontrol.Answer{OptionIDs: []string{"b"}}); e != nil {
					t.Fatal("resumed wait lost question", e)
				}
				fresh, e = f.s.Inputs.Get(f.ctx, resumed.ContinuationInputID)
				if e != nil || fresh.Status != "queued" {
					t.Fatal(fresh.Status, e)
				}
			}
			var oldState string
			_ = f.pool.QueryRow(f.ctx, `SELECT status FROM accepted_inputs WHERE input_id=$1`, f.claim.InputID).Scan(&oldState)
			if oldState != "cancelled" {
				t.Fatal("old input resurrected", oldState)
			}
		})
	}
}
func TestCommunicationTaskAmendAndStopCloseQuestionIntegration(t *testing.T) {
	for _, kind := range []string{task.ControlAmend, task.ControlStop} {
		t.Run(kind, func(t *testing.T) {
			f := setup(t, fixtureOptions{Task: true})
			r, e := f.publish(t, "question", "communication.widget", widget)
			if e != nil {
				t.Fatal(e)
			}
			if e = f.s.Inputs.Finish(f.ctx, f.claim, "waiting_for_user", "", "", 0); e != nil {
				t.Fatal(e)
			}
			ctrl := task.NewController(task.NewStore(f.pool), f.leases.Authority(), f.leases, run.NewPostgresRepository(f.pool), approval.NewPostgresRepository(f.pool), f.s.Inputs, transcript.NewStore(f.pool))
			ctrl.SetInteractionControls(f.s)
			if _, e = ctrl.Apply(controlActor(f, "control"), task.ControlRequest{TaskID: f.p.TaskID, ExpectedRevision: f.p.TaskRevision, Kind: kind, Instruction: "Change request"}); e != nil {
				t.Fatal(e)
			}
			var state string
			_ = f.pool.QueryRow(f.ctx, `SELECT state FROM communication_interactions WHERE interaction_id=$1`, r.InteractionRef).Scan(&state)
			expected := "cancelled"
			if kind == task.ControlAmend {
				expected = "superseded"
			}
			if state != expected {
				t.Fatal(state, expected)
			}
			if e = f.pool.QueryRow(f.ctx, `SELECT obligation_state FROM communication_admissions WHERE input_id=$1`, f.claim.InputID).Scan(&state); e != nil || state != "cancelled" {
				t.Fatal("owner control left open obligation", state, e)
			}
			if _, e = f.s.Respond(f.actor("web", "stale"), r.InteractionRef, f.bindings["web"], 1, 1, communicationcontrol.Answer{OptionIDs: []string{"a"}}); e == nil {
				t.Fatal("stale task continued")
			}
		})
	}
}

func controlActor(f *fixture, delivery string) context.Context {
	return owneridentity.WithActor(f.ctx, owneridentity.Actor{OwnerID: f.p.OwnerID, Channel: "web", SessionKey: f.p.SessionKey, DeliveryID: delivery})
}
