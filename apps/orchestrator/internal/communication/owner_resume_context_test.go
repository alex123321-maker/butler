package communication_test

import (
	"context"
	"path/filepath"
	"testing"
	"time"

	"github.com/butler/butler/apps/orchestrator/internal/approval"
	"github.com/butler/butler/apps/orchestrator/internal/completion"
	"github.com/butler/butler/apps/orchestrator/internal/input"
	flow "github.com/butler/butler/apps/orchestrator/internal/orchestrator"
	"github.com/butler/butler/apps/orchestrator/internal/run"
	"github.com/butler/butler/apps/orchestrator/internal/session"
	"github.com/butler/butler/apps/orchestrator/internal/task"
	"github.com/butler/butler/internal/communicationcontrol"
	commonv1 "github.com/butler/butler/internal/gen/common/v1"
	"github.com/butler/butler/internal/memory/catalog"
	"github.com/butler/butler/internal/memory/transcript"
	"github.com/butler/butler/internal/owneridentity"
	"github.com/butler/butler/internal/transport"
)

type ownerResumedProvider struct {
	resumedProvider
	claim                 input.Record
	revision, constraints int64
}

func (p *ownerResumedProvider) StartRun(ctx context.Context, q transport.StartRunRequest) (transport.EventStream, error) {
	p.f.p.RunID = q.Context.RunID
	p.f.p.InputID = p.claim.InputID
	p.f.p.TaskRevision = p.revision
	p.f.p.ConstraintsRevision = p.constraints
	return p.resumedProvider.StartRun(ctx, q)
}

func TestCommunicationOwnerResumeNewRunRetainsCanonicalAnswerIntegration(t *testing.T) {
	for _, tc := range []struct {
		name         string
		beforeResume bool
		invalid      string
	}{
		{"answer_before_resume", true, ""}, {"answer_after_resume", false, ""}, {"wrong_input", true, "input"}, {"wrong_revision", true, "revision"}, {"catalog", true, ""}, {"deleted_answer", true, "deleted"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			f := setup(t, fixtureOptions{Task: true})
			if tc.name == "catalog" || tc.invalid == "deleted" {
				manifest := catalog.NewFileManifest(filepath.Join(t.TempDir(), "privacy"))
				if e := manifest.Initialize(); e != nil {
					t.Fatal(e)
				}
				f.s.Sources = catalog.New(f.pool, manifest)
				if e := f.s.Sources.PinTranscript(f.ctx, f.p.SessionKey, 0); e != nil {
					t.Fatal(e)
				}
				f.s.SourceGuard = f.s.Sources.CheckRunSourcesTx
			}
			question, e := f.publish(t, "question", "communication.widget", widget)
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
				t.Fatal(e)
			}
			answer := func() {
				t.Helper()
				if _, e = f.s.Respond(f.actor("web", "answer"), question.InteractionRef, f.bindings["web"], 1, 1, communicationcontrol.Answer{OptionIDs: []string{"a"}}); e != nil {
					t.Fatal(e)
				}
			}
			if tc.beforeResume {
				answer()
			}
			resumed, e := ctrl.Apply(controlActor(f, "resume"), task.ControlRequest{TaskID: f.p.TaskID, ExpectedRevision: paused.Task.Revision, Kind: task.ControlResume})
			if e != nil {
				t.Fatal(e)
			}
			if !tc.beforeResume {
				answer()
			}
			claim, e := f.s.Inputs.Claim(context.Background(), time.Minute)
			if e != nil || claim.InputID != resumed.ContinuationInputID {
				t.Fatal(claim, e)
			}
			if tc.invalid == "input" {
				if _, e = f.pool.Exec(f.ctx, `UPDATE communication_interactions SET input_id=$2 WHERE interaction_id=$1`, question.InteractionRef, f.claim.InputID); e != nil {
					t.Fatal(e)
				}
			}
			if tc.invalid == "revision" {
				if _, e = f.pool.Exec(f.ctx, `UPDATE communication_interactions SET task_revision=task_revision-1 WHERE interaction_id=$1`, question.InteractionRef); e != nil {
					t.Fatal(e)
				}
			}
			if tc.invalid == "deleted" {
				actor := owneridentity.WithActor(context.Background(), owneridentity.Actor{OwnerID: f.p.OwnerID, Channel: "web", DeliveryID: "erase"})
				ref := "message:response:" + question.InteractionRef
				src, e := f.s.Sources.Get(actor, ref)
				if e != nil {
					t.Fatal(e)
				}
				if _, e = f.s.Sources.Delete(actor, ref, "delete", src.Revision); e != nil {
					t.Fatal(e)
				}
			}
			provider := &ownerResumedProvider{resumedProvider: resumedProvider{f: f}, claim: claim, revision: resumed.Task.Revision, constraints: resumed.Task.ConstraintsRevision}
			writer := completion.New(f.pool)
			writer.Communication = f.s
			writer.DataGuard = f.s.SourceGuard
			core := flow.NewService(session.NewPostgresRepository(f.pool), f.leases, run.NewService(run.NewPostgresRepository(f.pool), nil), transcript.NewStore(f.pool), provider, flow.Config{Tasks: tasks, Communication: f.s, CommunicationAdmission: func(ctx context.Context, id string) error {
				return f.s.AdmitRun(ctx, id, false, []string{"communication.message", "communication.widget"})
			}, Completion: writer, Checkpoints: f.s.Inputs, OwnerID: "new-owner-resume-worker", LeaseTTL: 30, ProviderName: "fixture", ModelName: "fixture"}, nil)
			event := claim.Event
			event.IdempotencyKey = claim.InputID
			result, e := core.Execute(input.WithClaim(context.Background(), claim), event)
			if tc.invalid != "" {
				if e == nil || provider.starts != 0 || f.count(t, "communication_publications") != 1 {
					t.Fatal("foreign/stale answer reached new provider", result, e, provider.starts)
				}
				return
			}
			if e != nil || result == nil || result.CurrentState != commonv1.RunState_RUN_STATE_COMPLETED || provider.starts != 1 {
				t.Fatal("new owner run lost accepted preference", result, e, provider.starts)
			}
		})
	}
}
