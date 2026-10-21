package communication_test

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/butler/butler/apps/orchestrator/internal/completion"
	"github.com/butler/butler/apps/orchestrator/internal/input"
	flow "github.com/butler/butler/apps/orchestrator/internal/orchestrator"
	"github.com/butler/butler/apps/orchestrator/internal/run"
	"github.com/butler/butler/apps/orchestrator/internal/session"
	"github.com/butler/butler/apps/orchestrator/internal/task"
	"github.com/butler/butler/internal/communicationcontrol"
	commonv1 "github.com/butler/butler/internal/gen/common/v1"
	"github.com/butler/butler/internal/ingress"
	"github.com/butler/butler/internal/memory/transcript"
)

func TestCommunicationFreshOwnershipAtEveryHandoffIntegration(t *testing.T) {
	for _, withTask := range []bool{false, true} {
		for _, boundary := range []string{"question_commit_before_finish", "response_before_finish", "finish_before_response", "fresh_claim_before_lease", "fresh_lease_before_run_refresh", "run_refresh_before_activation", "task_activation_before_provider"} {
			t.Run(map[bool]string{false: "conversation/", true: "task/"}[withTask]+boundary, func(t *testing.T) {
				f := setup(t, fixtureOptions{Task: withTask})
				prepared := map[string]any{}
				if withTask {
					job, e := task.NewStore(f.pool).Get(f.ctx, f.p.TaskID)
					if e != nil {
						t.Fatal(e)
					}
					prepared["Task"] = job
				}
				raw, _ := json.Marshal(prepared)
				if e := f.s.Inputs.SaveCheckpoint(f.ctx, f.p.RunID, "prepared", string(raw)); e != nil {
					t.Fatal(e)
				}
				if e := f.s.Inputs.SaveCheckpoint(f.ctx, f.p.RunID, "tool_dispatching", `{}`); e != nil {
					t.Fatal(e)
				}
				if _, e := f.pool.Exec(f.ctx, `UPDATE communication_admissions SET correction_count=1 WHERE input_id=$1`, f.p.InputID); e != nil {
					t.Fatal(e)
				}
				p, e := f.publish(t, "question", "communication.widget", widget)
				if e != nil {
					t.Fatal(e)
				}
				finish := func() {
					t.Helper()
					if e := f.s.Inputs.Finish(f.ctx, f.claim, "waiting_for_user", "", "", 0); e != nil {
						t.Fatal(e)
					}
				}
				answer := func() {
					t.Helper()
					if _, e := f.s.Respond(f.actor("telegram", "answer"), p.InteractionRef, f.bindings["telegram"], 1, 1, communicationcontrol.Answer{OptionIDs: []string{"a"}}); e != nil {
						t.Fatal(e)
					}
				}
				if boundary == "finish_before_response" {
					finish()
					answer()
				} else if boundary == "question_commit_before_finish" {
					// Worker dies immediately after publication. No-expiry wait is not
					// reclaimed or expired just because its ordinary Input TTL passed.
					if _, e = f.pool.Exec(f.ctx, `UPDATE accepted_inputs SET claim_expires_at=clock_timestamp()-INTERVAL '1 second',expires_at=clock_timestamp()-INTERVAL '25 hours' WHERE input_id=$1`, f.p.InputID); e != nil {
						t.Fatal(e)
					}
					if e = f.s.Inputs.ExpireQueued(f.ctx); e != nil {
						t.Fatal(e)
					}
					if _, e = f.s.Inputs.Claim(f.ctx, time.Minute); e == nil {
						t.Fatal("unanswered question reclaimed")
					}
					answer()
					if e = f.s.Inputs.Finish(f.ctx, f.claim, "waiting_for_user", "", "", 0); e == nil {
						t.Fatal("dead worker overwrote answer")
					}
				} else {
					answer()
					finish()
				}
				if _, e = f.leases.ReleaseLease(f.ctx, f.lease.LeaseID); e != nil {
					t.Fatal(e)
				}
				if boundary != "question_commit_before_finish" && boundary != "response_before_finish" && boundary != "finish_before_response" {
					runCrash(t, f, "handoff:"+boundary, p.InteractionRef, 0, true)
					stored, e := f.s.Inputs.Get(f.ctx, f.p.InputID)
					if e != nil || stored.ClaimID == "" || stored.ClaimID == f.claim.ClaimID {
						t.Fatal("child did not acquire fresh claim", e)
					}
					// Real process death leaves finite ownership intact until expiry.
					var until time.Time
					if e = f.pool.QueryRow(f.ctx, `SELECT GREATEST(i.claim_expires_at,COALESCE(o.expires_at,i.claim_expires_at)) FROM accepted_inputs i JOIN session_ownership o USING(session_key) WHERE i.input_id=$1`, f.p.InputID).Scan(&until); e != nil {
						t.Fatal(e)
					}
					if wait := time.Until(until.Add(20 * time.Millisecond)); wait > 0 {
						timer := time.NewTimer(wait)
						select {
						case <-timer.C:
						case <-f.ctx.Done():
							timer.Stop()
							t.Fatal(f.ctx.Err())
						}
					}
				}
				claim, e := f.s.Inputs.Claim(f.ctx, time.Minute)
				if e != nil {
					t.Fatal(e)
				}
				if _, e = f.s.Inputs.Claim(f.ctx, time.Minute); e == nil {
					t.Fatal("two simultaneous input claims")
				}
				provider := &resumedProvider{f: f}
				writer := completion.New(f.pool)
				writer.Communication = f.s
				core := flow.NewService(session.NewPostgresRepository(f.pool), f.leases, run.NewService(run.NewPostgresRepository(f.pool), nil), transcript.NewStore(f.pool), provider, flow.Config{Communication: f.s, Completion: writer, Checkpoints: f.s.Inputs, OwnerID: "replacement-worker", LeaseTTL: 30, ProviderName: "fixture", ModelName: "fixture"}, nil)
				result, e := core.Resume(input.WithClaim(context.Background(), claim), f.p.RunID, ingress.InputEvent{SessionKey: f.p.SessionKey, Source: "fixture", PayloadJSON: `{"text":"ask owner"}`})
				if e != nil || result.CurrentState != commonv1.RunState_RUN_STATE_COMPLETED || provider.starts != 1 {
					t.Fatal("fresh handoff failed", result, e, provider.starts)
				}
				var count, correction int
				if e = f.pool.QueryRow(f.ctx, `SELECT publication_count,correction_count FROM communication_admissions WHERE input_id=$1`, f.p.InputID).Scan(&count, &correction); e != nil || count != 2 || correction != 1 {
					t.Fatal("allowance reset", count, correction, e)
				}
				if f.count(t, "communication_publications") != 2 || f.count(t, "messages") != 4 {
					t.Fatal("handoff repeated question/response/final")
				}
			})
		}
	}
}
