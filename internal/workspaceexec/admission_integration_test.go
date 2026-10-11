package workspaceexec

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/butler/butler/internal/actionauth"
	"github.com/butler/butler/internal/budget"
	commonv1 "github.com/butler/butler/internal/gen/common/v1"
	toolbrokerv1 "github.com/butler/butler/internal/gen/toolbroker/v1"
	"github.com/butler/butler/internal/owneridentity"
	"github.com/jackc/pgx/v5/pgxpool"
)

// Actual PG context with explicit exact fixture approval. This tests kernel
// admission, not a human approval, Broker application or runtime bootstrap.
type admissionReader struct {
	pool     *pgxpool.Pool
	approval actionauth.ApprovalSnapshot
}

func (r *admissionReader) GetActionContext(ctx context.Context, run string) (actionauth.ContextSnapshot, error) {
	c := actionauth.ContextSnapshot{RunID: run, AutonomyMode: int32(commonv1.AutonomyMode_AUTONOMY_MODE_1), WorkspaceReady: true}
	err := r.pool.QueryRow(ctx, `SELECT r.current_state,r.session_key,s.user_id,r.task_id,t.revision,r.task_revision,t.constraints_revision,t.status,o.lease_id,o.fence_epoch,o.expires_at FROM runs r JOIN sessions s USING(session_key) JOIN tasks t ON t.task_id=r.task_id JOIN session_ownership o ON o.session_key=r.session_key WHERE r.run_id=$1 AND o.run_id=r.run_id`, run).Scan(&c.RunState, &c.SessionKey, &c.OwnerID, &c.TaskID, &c.TaskRevision, &c.PinnedTaskRevision, &c.ConstraintsRevision, &c.TaskStatus, &c.LeaseID, &c.LeaseEpoch, &c.LeaseExpiresAt)
	if err != nil {
		return c, err
	}
	var input, claim string
	if err = r.pool.QueryRow(ctx, `SELECT input_id,claim_id FROM accepted_inputs WHERE run_id=$1 AND status='running'`, run).Scan(&input, &claim); err != nil {
		return c, err
	}
	c.InputClaimHash = actionauth.InputClaimHash(input, claim)
	return c, nil
}
func (r *admissionReader) VerifyActionApproval(context.Context, actionauth.ApprovalRequest) (actionauth.ApprovalSnapshot, error) {
	return r.approval, nil
}

func TestWorkspaceDispatchClaimAtomicallyBindsInputBudgetSourceAndResourceIntegration(t *testing.T) {
	for _, mode := range []string{"allowed", "changed_input", "expired_input", "wrong_checkpoint", "expired_worker", "changed_task", "deleted_source", "missing_budget"} {
		t.Run(mode, func(t *testing.T) {
			f := newSourceFixture(t)
			if _, err := f.pool.Exec(f.ctx, `INSERT INTO accepted_inputs(input_id,session_key,event_id,dedup_key,source,owner_id,event_json,task_id,run_id,status,checkpoint,claim_id,claim_expires_at,expires_at) VALUES('wp-input','wp-session','fixture','workspace-claim','test','wp-owner','{}','wp-task','wp-run','running','tool_dispatching','wp-input-claim',clock_timestamp()+interval '1 hour',clock_timestamp()+interval '1 hour')`); err != nil {
				t.Fatal(err)
			}
			b := budget.NewController(f.pool, budget.DefaultPolicy())
			d, err := NewDispatchStore(f.pool, f.resource, b, f.provenance, Profile{ID: "fixture-workspaces", Target: "127.0.0.1:50055"})
			if err != nil {
				t.Fatal(err)
			}
			if err = d.CheckReady(f.ctx); err != nil {
				t.Fatal(err)
			}
			auth := actionauth.NewPostgresStore(f.pool)
			auth.SetBudgets(b)
			auth.SetDispatchClaimGuard(d)
			reader := &admissionReader{pool: f.pool}
			service := actionauth.NewService(auth, reader, nil)
			call := &toolbrokerv1.ToolCall{RunId: "wp-run", ToolCallId: "workspace-fixture", ToolName: CreateTool, ArgsJson: `{}`, AutonomyMode: commonv1.AutonomyMode_AUTONOMY_MODE_1}
			contract := Contracts(d.profile.Target)[0]
			prepared, err := service.Prepare(f.ctx, call, contract)
			if err != nil {
				t.Fatal(err)
			}
			reader.approval = actionauth.ApprovalSnapshot{ApprovalID: "fixture-approval", Status: "approved", Binding: prepared.Binding, ExpiresAt: time.Now().Add(time.Minute), ResolvedAt: time.Now()}
			grant, err := service.Authorize(f.ctx, call, contract, prepared.Binding.ActionID, "fixture-approval", "")
			if err != nil {
				t.Fatal(err)
			}
			checkpoint, _ := json.Marshal(map[string]any{"checkpoint_data": map[string]string{"tool_call_id": call.ToolCallId, "effect_id": prepared.EffectID, "authorization_id": grant.ID}})
			if _, err = f.pool.Exec(f.ctx, `UPDATE accepted_inputs SET checkpoint_json=$1 WHERE input_id='wp-input'`, checkpoint); err != nil {
				t.Fatal(err)
			}
			owned, err := d.PrepareContext(f.ctx, call, contract, grant)
			if err != nil {
				t.Fatal(err)
			}
			switch mode {
			case "changed_input":
				_, err = f.pool.Exec(f.ctx, `UPDATE accepted_inputs SET claim_id='new-claim' WHERE input_id='wp-input'`)
			case "expired_input":
				_, err = f.pool.Exec(f.ctx, `UPDATE accepted_inputs SET claim_expires_at=clock_timestamp()-interval '1 second' WHERE input_id='wp-input'`)
			case "wrong_checkpoint":
				_, err = f.pool.Exec(f.ctx, `UPDATE accepted_inputs SET checkpoint_json='{}' WHERE input_id='wp-input'`)
			case "expired_worker":
				_, err = f.pool.Exec(f.ctx, `UPDATE session_ownership SET expires_at=clock_timestamp()-interval '1 second' WHERE session_key='wp-session'`)
			case "changed_task":
				_, err = f.pool.Exec(f.ctx, `UPDATE tasks SET revision=revision+1 WHERE task_id='wp-task'`)
			case "deleted_source":
				owner := owneridentity.WithActor(f.ctx, owneridentity.Actor{OwnerID: "wp-owner", Channel: "web", DeliveryID: "delete"})
				_, err = f.catalog.Delete(owner, f.source.ID, "delete", f.source.Revision)
			case "missing_budget":
				auth.SetBudgets(nil)
			}
			if err != nil {
				t.Fatal(err)
			}
			err = auth.ClaimGrant(owned, grant.ID)
			var grants, effect string
			var reservations, permissions, leases, tickets, copies int
			if e := f.pool.QueryRow(f.ctx, `SELECT state FROM action_authorizations WHERE authorization_id=$1`, grant.ID).Scan(&grants); e != nil {
				t.Fatal(e)
			}
			if e := f.pool.QueryRow(f.ctx, `SELECT state FROM action_effects WHERE action_id=$1`, prepared.EffectID).Scan(&effect); e != nil {
				t.Fatal(e)
			}
			if e := f.pool.QueryRow(f.ctx, `SELECT (SELECT count(*) FROM execution_budget_reservations),(SELECT count(*) FROM code_workspace_authorizations),(SELECT count(*) FROM execution_resource_leases),(SELECT count(*) FROM code_workspace_dispatches),(SELECT count(*) FROM code_workspaces)`).Scan(&reservations, &permissions, &leases, &tickets, &copies); e != nil {
				t.Fatal(e)
			}
			if mode != "allowed" {
				if err == nil || grants != "authorized" || effect != "allowed" || reservations != 0 || permissions != 0 || leases != 0 || tickets != 0 || copies != 0 {
					t.Fatal("failed admission partially spent/published", grants, effect, reservations, permissions, leases, tickets, copies, err)
				}
				return
			}
			if err != nil || grants != "inflight" || effect != "executing" || reservations != 1 || permissions != 1 || leases != 1 || tickets != 1 || copies != 0 {
				t.Fatal("atomic workspace admission", grants, effect, reservations, permissions, leases, tickets, copies, err)
			}
			var live bool
			if err = f.pool.QueryRow(f.ctx, `SELECT d.deadline>clock_timestamp() AND d.deadline<=clock_timestamp()+interval '30 seconds' AND l.state='claimed' AND r.inflight_lease_id=l.lease_id AND r.resource_id='workspace:'||a.workspace_id AND a.input_claim_hash=$2 FROM code_workspace_dispatches d JOIN code_workspace_authorizations a USING(authorization_id) JOIN execution_resource_leases l ON l.lease_id=d.resource_lease_id JOIN execution_resources r ON r.resource_id=d.resource_id WHERE d.authorization_id=$1`, grant.ID, grant.InputClaimHash).Scan(&live); err != nil || !live {
				t.Fatal("private mapping not exact/finite", err)
			}
			if err = auth.ClaimGrant(owned, grant.ID); err == nil {
				t.Fatal("spent dispatch reused")
			}
		})
	}
}
