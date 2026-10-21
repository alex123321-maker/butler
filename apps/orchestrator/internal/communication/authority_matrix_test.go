package communication_test

import (
	"context"
	"testing"
	"time"

	"github.com/butler/butler/apps/orchestrator/internal/goal"
	"github.com/butler/butler/apps/orchestrator/internal/input"
	"github.com/butler/butler/apps/orchestrator/internal/run"
	"github.com/butler/butler/apps/orchestrator/internal/session"
	"github.com/butler/butler/apps/orchestrator/internal/task"
	"github.com/butler/butler/internal/actionauth"
	"github.com/butler/butler/internal/communicationcontrol"
	"github.com/butler/butler/internal/owneridentity"
	"github.com/butler/butler/internal/ownership"
	"github.com/jackc/pgx/v5/pgxpool"
)

func communicationAssignedTask(t *testing.T, pool *pgxpool.Pool, ctx context.Context, tasks *task.Store, key, owner, mode string) task.Record {
	t.Helper()
	m := goal.New(pool, tasks, nil)
	actor := func(id string) context.Context {
		return owneridentity.WithActor(ctx, owneridentity.Actor{OwnerID: owner, Channel: "web", SessionKey: key, DeliveryID: id})
	}
	g, e := m.PutGoal(actor("goal"), goal.GoalRequest{Goal: actionauth.GoalPolicy{Kind: "objective", Status: "active", Title: "Owner communication", Purpose: "owner_communication", ExpiresAt: time.Now().Add(24 * time.Hour)}})
	if e != nil {
		t.Fatal(e)
	}
	grant, e := m.PutMandate(actor("mandate"), goal.MandateRequest{Mandate: actionauth.Mandate{GoalID: g.SubjectID, GoalRevision: g.Revision, Status: "active", Purpose: "owner_communication", Tools: []string{"communication.message", "communication.widget"}, ResourceScopes: []actionauth.ResourceScope{{Origin: "tool:communication.message"}, {Origin: "tool:communication.widget"}}, Effects: []actionauth.Effect{actionauth.EffectPrepare}, Privacy: "local_only", Budget: actionauth.MandateBudget{ModelTurns: 8, Tokens: 1000000, ExternalCalls: 8, Children: 1, ActiveMillis: 1000000, EgressBytes: 33554432, ProviderInputBytes: 33554432}, ExpiresAt: time.Now().Add(2 * time.Hour), Intervention: []string{"scope_change", "effect_unknown", "budget_exhausted"}, Reporting: "exceptions"}})
	if e != nil {
		t.Fatal(e)
	}
	r, e := m.CreateOwnerTask(actor("task"), goal.TaskRequest{SessionKey: key, GoalID: g.SubjectID, MandateID: grant.SubjectID, Purpose: "owner_communication", Goal: "Get current owner preference", CompletionCriteria: []string{"Owner answer recorded"}, InitiativeReason: "explicit owner work", ObservationRef: "fixture:owner", ResultDelivery: mode})
	if e != nil {
		t.Fatal(e)
	}
	t.Cleanup(func() {
		if _, e := pool.Exec(context.Background(), `DELETE FROM task_authority_bindings WHERE task_id=$1`, r.TaskID); e != nil {
			t.Error(e)
		}
	})
	return r
}

func TestCommunicationAnswerRetainedButFreshAuthorityRequiredMatrixIntegration(t *testing.T) {
	for _, scenario := range []string{"expired_mandate", "revoked_mandate", "paused_goal", "source_revoked_after_answer", "stale_task_revision", "lost_input_claim", "stale_fence"} {
		t.Run(scenario, func(t *testing.T) {
			f := setup(t, fixtureOptions{Authority: true})
			p, e := f.publish(t, "question", "communication.widget", widget)
			if e != nil {
				t.Fatal(e)
			}
			if e = f.s.Inputs.Finish(f.ctx, f.claim, "waiting_for_user", "", "", 0); e != nil {
				t.Fatal(e)
			}
			// Expiry/revocation of execution authority does not destroy preference
			// DATA. It must independently deny the subsequent fresh execution.
			if scenario == "expired_mandate" {
				_, e = f.pool.Exec(f.ctx, `UPDATE action_mandates SET expires_at=clock_timestamp()-INTERVAL '1 second' WHERE mandate_id=(SELECT mandate_id FROM task_authority_bindings WHERE task_id=$1)`, f.p.TaskID)
			}
			if scenario == "revoked_mandate" {
				_, e = f.pool.Exec(f.ctx, `UPDATE action_mandates SET status='revoked',revoked_at=clock_timestamp() WHERE mandate_id=(SELECT mandate_id FROM task_authority_bindings WHERE task_id=$1)`, f.p.TaskID)
			}
			if scenario == "paused_goal" {
				_, e = f.pool.Exec(f.ctx, `UPDATE owner_goals SET status='paused' WHERE goal_id=(SELECT goal_id FROM task_authority_bindings WHERE task_id=$1)`, f.p.TaskID)
			}
			if e != nil {
				t.Fatal(e)
			}
			if _, e = f.s.Respond(f.actor("telegram", "answer"), p.InteractionRef, f.bindings["telegram"], 1, 1, communicationcontrol.Answer{OptionIDs: []string{"a"}}); e != nil {
				t.Fatal("preference lost with execution authority", e)
			}
			if _, e = f.leases.ReleaseLease(f.ctx, f.lease.LeaseID); e != nil {
				t.Fatal(e)
			}
			claim, e := f.s.Inputs.Claim(f.ctx, time.Minute)
			if e != nil {
				t.Fatal(e)
			}
			lease, e := f.leases.AcquireLease(f.ctx, session.AcquireLeaseParams{SessionKey: f.p.SessionKey, RunID: f.p.RunID, OwnerID: "fresh-worker", TTL: time.Minute})
			if e != nil {
				t.Fatal(e)
			}
			live := ownership.WithToken(input.WithClaim(context.Background(), claim), f.leases.Authority(), ownership.Token{SessionKey: f.p.SessionKey, RunID: f.p.RunID, LeaseID: lease.LeaseID, FenceEpoch: lease.FenceEpoch})
			if _, e = run.NewService(run.NewPostgresRepository(f.pool), nil).RefreshExecutionLease(live, f.p.RunID, lease.LeaseID); e != nil {
				t.Fatal(e)
			}
			switch scenario {
			case "source_revoked_after_answer":
				f.revoked.Store(true)
			case "stale_task_revision":
				_, e = f.pool.Exec(f.ctx, `UPDATE tasks SET constraints_revision=constraints_revision+1 WHERE task_id=$1`, f.p.TaskID)
			case "lost_input_claim":
				_, e = f.pool.Exec(f.ctx, `UPDATE accepted_inputs SET claim_expires_at=clock_timestamp()-INTERVAL '1 second' WHERE input_id=$1`, f.p.InputID)
			case "stale_fence":
				_, e = f.leases.ReleaseLease(f.ctx, lease.LeaseID)
			}
			if e != nil {
				t.Fatal(e)
			}
			if _, e = f.s.ResumeInteraction(live, f.p.RunID, p.InteractionRef); e == nil {
				t.Fatal("stale authority resumed")
			}
			var state string
			var answer []byte
			if e = f.pool.QueryRow(f.ctx, `SELECT state,response_json FROM communication_interactions WHERE interaction_id=$1`, p.InteractionRef).Scan(&state, &answer); e != nil || state != "resolved" || len(answer) == 0 {
				t.Fatal("denied resume erased answer", state, e)
			}
			var status string
			if e = f.pool.QueryRow(f.ctx, `SELECT status FROM tasks WHERE task_id=$1`, f.p.TaskID).Scan(&status); e != nil || status != "waiting_for_user" {
				t.Fatal("failed resume activated Task", status, e)
			}
			if f.count(t, "communication_publications") != 1 || f.count(t, "messages") != 2 {
				t.Fatal("denial replayed tool or inference")
			}
		})
	}
}
