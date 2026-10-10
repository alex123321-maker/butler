package input

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	flow "github.com/butler/butler/apps/orchestrator/internal/orchestrator"
	"github.com/butler/butler/apps/orchestrator/internal/session"
	"github.com/butler/butler/internal/actionauth"
	toolbrokerv1 "github.com/butler/butler/internal/gen/toolbroker/v1"
	"github.com/butler/butler/internal/ownership"
)

type slotAwareTools struct {
	*lifecycleTools
	t          *testing.T
	references []string
}

func (x *slotAwareTools) PrepareAction(ctx context.Context, call *toolbrokerv1.ToolCall) (actionauth.PreparedAction, error) {
	ref := actionauth.LogicalReference(ctx)
	if ref == "" {
		x.t.Fatal("prepare ran without a trusted action slot")
	}
	var captured string
	if err := x.pool.QueryRow(ctx, `SELECT checkpoint_json->'action_slots'->$2->>'reference' FROM accepted_inputs WHERE run_id=$1`, call.GetRunId(), call.GetToolCallId()).Scan(&captured); err != nil || captured != ref {
		x.t.Fatal("action slot was not persisted before preparation")
	}
	x.references = append(x.references, ref)
	prepared, err := x.lifecycleTools.PrepareAction(ctx, call)
	if err != nil {
		return prepared, err
	}
	prepared.EffectID, _, err = actionauth.EffectIdentity(prepared.Binding, ref)
	return prepared, err
}
func TestActionSlotPersistsBeforePrepareAndSurvivesApprovalRecoveryIntegration(t *testing.T) {
	f := newLifecycleFixture(t)
	f.tools.unknown = true
	tools := &slotAwareTools{lifecycleTools: f.tools, t: t}
	executor := flow.NewService(f.sessions, session.NewPostgresLeaseManager(f.pool), f.runs, f.transcript, &lifecycleProvider{}, flow.Config{ProviderName: "fixture", ModelName: "fixture", OwnerID: "worker", LeaseTTL: 30, Tasks: f.tasks, Checkpoints: f.store, ApprovalService: f.approvals, ActionAuthorizer: tools, Tools: tools}, nil)
	worker := NewService(f.store, f.sessions, executor, f.runs, nil)
	worker.process(f.ctx, f.claim(t))
	waiting := f.assertReceipt(t, "waiting_for_user")
	_, raw, err := f.approvals.LoadContinuationByRun(f.ctx, waiting.RunID)
	if err != nil {
		t.Fatal(err)
	}
	var continuation struct {
		LogicalActionRef string `json:"logical_action_ref"`
		EffectID         string `json:"effect_id"`
	}
	if json.Unmarshal([]byte(raw), &continuation) != nil || continuation.LogicalActionRef == "" || continuation.EffectID == "" {
		t.Fatal("continuation lost permanent action identity")
	}
	if _, _, err = f.resolve(t, true, ownerActor()); err != nil {
		t.Fatal(err)
	}
	worker.process(f.ctx, f.claim(t))
	f.assertReceipt(t, "blocked")
	if len(tools.references) != 2 || tools.references[0] != continuation.LogicalActionRef || tools.references[1] != continuation.LogicalActionRef || f.tools.calls != 1 {
		t.Fatal("approval recovery replaced the saved intent slot")
	}
}

func TestActionSlotDistinctIntentAndRevisionChecksIntegration(t *testing.T) {
	f := newLifecycleFixture(t)
	worker, _ := f.worker(f.store, nil)
	worker.process(f.ctx, f.claim(t))
	waiting := f.assertReceipt(t, "waiting_for_user")
	if _, _, err := f.resolve(t, true, ownerActor()); err != nil {
		t.Fatal(err)
	}
	claim := f.claim(t)
	leases := session.NewPostgresLeaseManager(f.pool)
	lease, err := leases.AcquireLease(f.ctx, session.AcquireLeaseParams{SessionKey: f.key, RunID: waiting.RunID, OwnerID: "slot-test", TTL: time.Minute})
	if err != nil {
		t.Fatal(err)
	}
	defer leases.ReleaseLease(context.Background(), lease.LeaseID)
	ctx := ownership.WithToken(WithClaim(f.ctx, claim), leases.Authority(), ownership.Token{SessionKey: f.key, RunID: waiting.RunID, LeaseID: lease.LeaseID, FenceEpoch: lease.FenceEpoch})
	hash, err := actionauth.ArgumentsHash(`{"method":"POST","url":"https://fixture.invalid/submit","body":"one"}`)
	if err != nil {
		t.Fatal(err)
	}
	original, err := f.store.EnsureActionSlot(ctx, waiting.RunID, "stable-call", "http.request", hash)
	if err != nil {
		t.Fatal(err)
	}
	fresh, err := f.store.EnsureActionSlot(ctx, waiting.RunID, "another-explicit-call", "http.request", hash)
	if err != nil || fresh == original {
		t.Fatal("identical arguments collapsed distinct intentions")
	}
	if err = f.store.SaveCheckpoint(ctx, waiting.RunID, "tool_prepared", `{"replacement":true}`); err != nil {
		t.Fatal(err)
	}
	retained, err := f.store.EnsureActionSlot(ctx, waiting.RunID, "stable-call", "http.request", hash)
	if err != nil || retained != original {
		t.Fatal("checkpoint_data replacement lost permanent slot")
	}
	if _, err = f.pool.Exec(ctx, `UPDATE tasks SET constraints_revision=constraints_revision+1 WHERE task_id=$1`, claim.TaskID); err != nil {
		t.Fatal(err)
	}
	if _, err = f.store.EnsureActionSlot(ctx, waiting.RunID, "stable-call", "http.request", hash); err == nil {
		t.Fatal("changed task constraints reused prepared intent slot")
	}
}
