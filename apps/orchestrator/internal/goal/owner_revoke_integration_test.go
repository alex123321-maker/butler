package goal_test

import (
	"errors"
	"testing"

	"github.com/butler/butler/apps/orchestrator/internal/goal"
	"github.com/butler/butler/apps/orchestrator/internal/task"
	"github.com/butler/butler/internal/actionauth"
	"github.com/butler/butler/internal/owneridentity"
)

func TestOwnerMandateReferenceRevocationReplaysBeforeCASAndStopsBranchesIntegration(t *testing.T) {
	f := setup(t)
	root := f.create(t, "revoke-root")
	child, err := f.tasks.Create(f.ctx, task.CreateRequest{SessionKey: f.key, OwnerID: f.owner, ParentTaskID: root.TaskID, Goal: "Bounded child", CompletionCriteria: []string{"Child evidence"}})
	if err != nil {
		t.Fatal(err)
	}
	req := goal.MandateRevokeRequest{MandateID: f.m.ID, ExpectedRevision: f.m.Revision}
	actor := owneridentity.WithActor(f.ctx, owneridentity.Actor{OwnerID: f.owner, Channel: "telegram", DeliveryID: "telegram-update-42"})
	r, err := f.manager.RevokeMandate(actor, req)
	if err != nil || r.Revision != f.m.Revision+1 || r.Propagation != "pending" {
		t.Fatalf("revoke: %#v %v", r, err)
	}
	if _, _, _, err = f.manager.ReadTaskAuthority(f.ctx, root.TaskID); err == nil {
		t.Fatal("revoked mandate authorized old assignment before propagation")
	}
	if err = f.manager.Sweep(f.ctx); err != nil {
		t.Fatal(err)
	}
	for _, id := range []string{root.TaskID, child.TaskID} {
		record, e := f.tasks.Get(f.ctx, id)
		if e != nil || record.Status != "cancelled" {
			t.Fatalf("branch not stopped: %#v %v", record, e)
		}
	}
	// New manager, same durable delivery, now-obsolete expected revision.
	replay, err := goal.New(f.pool, f.tasks, f.controller).RevokeMandate(actor, req)
	if err != nil || replay.EventID != r.EventID || replay.Revision != r.Revision {
		t.Fatalf("lost ACK replay failed: %#v %v", replay, err)
	}
	stored, err := actionauth.NewPostgresStore(f.pool).GetMandate(f.ctx, f.m.ID)
	if err != nil || stored.Status != "revoked" || stored.Revision != r.Revision || stored.Budget != f.m.Budget || !stored.BudgetDeadline.Equal(f.m.BudgetDeadline) {
		t.Fatal("revocation changed quota/history or duplicated revision")
	}
	req.ExpectedRevision++
	if _, err = f.manager.RevokeMandate(actor, req); !errors.Is(err, goal.ErrConflict) {
		t.Fatal("same delivery accepted changed request")
	}
	var n int
	if f.pool.QueryRow(f.ctx, `SELECT count(*) FROM owner_authority_events WHERE owner_id=$1 AND source='telegram' AND delivery_id='telegram-update-42'`, f.owner).Scan(&n) != nil || n != 1 {
		t.Fatal("duplicate revocation event")
	}
}

func TestOwnerMandateReferenceRevocationRejectsForeignAndStaleOwnerRequestsIntegration(t *testing.T) {
	f := setup(t)
	req := goal.MandateRevokeRequest{MandateID: f.m.ID, ExpectedRevision: f.m.Revision}
	foreign := owneridentity.WithActor(f.ctx, owneridentity.Actor{OwnerID: "foreign", Channel: "telegram", DeliveryID: "foreign"})
	if _, err := f.manager.RevokeMandate(foreign, req); !errors.Is(err, goal.ErrUnauthorized) {
		t.Fatal("foreign owner revoked mandate")
	}
	if _, err := f.manager.RevokeMandate(f.ctx, req); !errors.Is(err, goal.ErrUnauthorized) {
		t.Fatal("model/no actor revoked mandate")
	}
	req.ExpectedRevision++
	if _, err := f.manager.RevokeMandate(f.actor("stale-revoke"), req); !errors.Is(err, goal.ErrConflict) {
		t.Fatal("stale mandate revision accepted")
	}
	stored, err := actionauth.NewPostgresStore(f.pool).GetMandate(f.ctx, f.m.ID)
	if err != nil || stored.Status != "active" || stored.Revision != f.m.Revision {
		t.Fatal("denied request changed authority")
	}
}
