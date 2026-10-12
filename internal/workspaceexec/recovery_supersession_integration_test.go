package workspaceexec

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/butler/butler/internal/owneridentity"
)

func TestWorkspaceOwnerDiscardSupersedesOnlyStaleUnattemptedIntentIntegration(t *testing.T) {
	f := newDiscardFixture(t, false)
	// Bring the initial fixture Stop's constraints revision to the actual
	// controller's revision+constraints increment semantics before its intent.
	if _, err := f.pool.Exec(f.ctx, `UPDATE tasks SET constraints_revision=constraints_revision+1 WHERE task_id='wp-task'`); err != nil {
		t.Fatal(err)
	}
	f.req.ConstraintsRevision++
	old := f.request(t)
	// These are real PG owner-control state transitions, without a worker/model:
	// Amend rev2->3, then Stop rev3->4. They match Controller.Apply's revision,
	// constraints and RevokeForControlTx lease/fence changes. API composition is
	// exercised separately by the owning control suite; this is a kernel race.
	if _, err := f.pool.Exec(f.ctx, `UPDATE tasks SET status='active',revision=revision+1,constraints_revision=constraints_revision+1 WHERE task_id='wp-task';UPDATE session_ownership SET lease_id='',fence_epoch=fence_epoch+1,expires_at=clock_timestamp() WHERE session_key='wp-session'`); err != nil {
		t.Fatal(err)
	}
	live, err := f.runtime.recovery.Get(f.owner, f.workspaceID)
	if err != nil || live.CanDiscard || live.TaskRevision != 3 {
		t.Fatal("active amended task enabled discard", err)
	}
	if f.runtime.ProcessRecovery(f.ctx, old.RecoveryID) == nil {
		t.Fatal("old intent executed after amendment")
	}
	if _, err = f.pool.Exec(f.ctx, `UPDATE tasks SET status='cancelled',revision=revision+1,constraints_revision=constraints_revision+1 WHERE task_id='wp-task';UPDATE session_ownership SET lease_id='',fence_epoch=fence_epoch+1,expires_at=clock_timestamp() WHERE session_key='wp-session'`); err != nil {
		t.Fatal(err)
	}
	if f.runtime.ProcessRecovery(f.ctx, old.RecoveryID) == nil {
		t.Fatal("fresh Stop silently revalidated the old intent")
	}
	var phase string
	var unattempted bool
	if f.pool.QueryRow(f.ctx, `SELECT phase,attempted_at IS NULL FROM code_workspace_recoveries WHERE recovery_id=$1`, old.RecoveryID).Scan(&phase, &unattempted) != nil || phase != "requested" || !unattempted {
		t.Fatal("stale intent consumed an erasure attempt")
	}
	f.retained(t, false)
	freshView, err := f.runtime.recovery.Get(f.owner, f.workspaceID)
	if err != nil || freshView.TaskRevision != 4 || freshView.ConstraintsRevision != 4 || freshView.RecoveryID != old.RecoveryID || freshView.Phase != "requested" || !freshView.CanDiscard {
		t.Fatal("fresh stopped revision cannot replace stale unattempted intent", err)
	}
	req := DiscardRequest{WorkspaceID: f.workspaceID, ResourceVersion: freshView.ResourceVersion, WorkspaceGeneration: freshView.WorkspaceGeneration, TaskRevision: freshView.TaskRevision, ConstraintsRevision: freshView.ConstraintsRevision, PreviousRecoveryID: old.RecoveryID}
	currentOwner := owneridentity.WithActor(context.Background(), owneridentity.Actor{OwnerID: "wp-owner", Channel: "web", DeliveryID: "fresh-current-stale-replacement"})
	for _, mode := range []string{"same_delivery", "missing_previous", "stale_task", "stale_constraints", "wrong_resource", "foreign"} {
		bad, actor := req, currentOwner
		switch mode {
		case "same_delivery":
			actor = f.owner
		case "missing_previous":
			bad.PreviousRecoveryID = ""
		case "stale_task":
			bad.TaskRevision = 2
		case "stale_constraints":
			bad.ConstraintsRevision = 2
		case "wrong_resource":
			bad.ResourceVersion++
		case "foreign":
			actor = owneridentity.WithActor(context.Background(), owneridentity.Actor{OwnerID: "foreign", Channel: "web", DeliveryID: "foreign-stale-replacement"})
		}
		if _, err := f.runtime.recovery.RequestDiscard(actor, bad); err == nil {
			t.Fatalf("%s replaced old intent without exact new owner decision", mode)
		}
	}
	fresh, err := f.runtime.recovery.RequestDiscard(currentOwner, req)
	if err != nil || fresh.RecoveryID == old.RecoveryID || fresh.Phase != "requested" {
		t.Fatal("new explicit current owner decision denied", err)
	}
	if f.pool.QueryRow(f.ctx, `SELECT phase,attempted_at IS NULL AND finished_at IS NOT NULL FROM code_workspace_recoveries WHERE recovery_id=$1`, old.RecoveryID).Scan(&phase, &unattempted) != nil || phase != "superseded" || !unattempted {
		t.Fatal("unattempted superseded history lost")
	}
	// Lost acknowledgement of the OLD delivery must never borrow the replacement
	// delivery's identity, current permission or independently confirmed receipt.
	replay, err := f.runtime.recovery.RequestDiscard(f.owner, f.req)
	if err != nil || replay.RecoveryID != old.RecoveryID || replay.Phase != "superseded" || replay.CanDiscard || replay.TaskRevision != 2 || replay.ConstraintsRevision != 2 {
		t.Fatal("old lost ACK borrowed replacement identity/current authority", err)
	}
	if f.runtime.ProcessRecovery(f.ctx, old.RecoveryID) == nil {
		t.Fatal("superseded worker replayed old decision")
	}
	if _, err = os.Stat(filepath.Join(f.root, f.workspaceID, "private", "write-pending")); err != nil {
		t.Fatal("decision supersession erased the copy")
	}
	f.retained(t, false)
	if err = f.runtime.ProcessRecovery(f.ctx, fresh.RecoveryID); err != nil {
		t.Fatal("fresh decision failed independent physical erasure", err)
	}
	f.retained(t, true)
	replay, err = f.runtime.recovery.RequestDiscard(f.owner, f.req)
	if err != nil || replay.RecoveryID != old.RecoveryID || replay.Phase != "superseded" || replay.CanDiscard || replay.ResourceVersion != f.req.ResourceVersion || replay.WorkspaceGeneration != f.req.WorkspaceGeneration {
		t.Fatal("old receipt became new discard success", err)
	}
	var oldReceipts int
	if f.pool.QueryRow(f.ctx, `SELECT count(*) FROM code_workspace_erasure_receipts WHERE recovery_id=$1`, old.RecoveryID).Scan(&oldReceipts) != nil || oldReceipts != 0 {
		t.Fatal("unattempted old intent acquired physical receipt")
	}
}

func TestWorkspaceOwnerDiscardCannotSupersedeCurrentUnattemptedIntentIntegration(t *testing.T) {
	f := newDiscardFixture(t, false)
	old := f.request(t)
	view, err := f.runtime.recovery.Get(f.owner, f.workspaceID)
	if err != nil || view.CanDiscard {
		t.Fatal("current unattempted request enabled replacement", err)
	}
	req := f.req
	req.PreviousRecoveryID = old.RecoveryID
	actor := owneridentity.WithActor(context.Background(), owneridentity.Actor{OwnerID: "wp-owner", Channel: "web", DeliveryID: "different-but-not-stale"})
	if _, err = f.runtime.recovery.RequestDiscard(actor, req); err == nil {
		t.Fatal("different delivery alone replaced current pending intent")
	}
	f.retained(t, false)
}
