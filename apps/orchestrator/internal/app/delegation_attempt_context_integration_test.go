package app

import (
	"strings"
	"testing"

	"github.com/butler/butler/internal/memory/catalog"
	"github.com/butler/butler/internal/owneridentity"
	"github.com/butler/butler/internal/ownership"
)

func TestDelegationFailedAttemptOwnedContextIntegration(t *testing.T) {
	f, c, child, _ := failedDelegationFixture(t, "rate_limited", true)
	ctx := f.leasedWorker(t, f.parent, "observing-parent")
	reader := failedDelegationAttemptReader{pool: f.pool, manager: f.manager}
	reports, err := reader.LoadFailedDelegationAttempts(ctx, "observing-parent")
	if err != nil || len(reports) != 1 || reports[0].TaskID != child.TaskID || reports[0].ParentTaskID != f.parent.TaskID || reports[0].ModelUsage[1].InputTokens != nil {
		t.Fatal("current parent did not observe real blocked attempt", err, reports)
	}
	var pins int
	if err = f.pool.QueryRow(f.ctx, `SELECT count(*) FROM memory_run_sources WHERE run_id='observing-parent'`).Scan(&pins); err != nil || pins != len(c.Sources)+1 {
		t.Fatal("current parent source dependencies absent", err, pins)
	}
	if _, err = reader.LoadFailedDelegationAttempts(ctx, "foreign-run"); err == nil {
		t.Fatal("caller-selected run exposed another task")
	}
	mixed := owneridentity.WithActor(ctx, owneridentity.Actor{OwnerID: f.parent.OwnerID, Channel: "web", DeliveryID: "mixed"})
	if _, err = reader.LoadFailedDelegationAttempts(mixed, "observing-parent"); err == nil {
		t.Fatal("owner identity widened a worker's source scope")
	}
	if _, err = f.sources.Delete(f.owner, c.ContractSource.ID, "delete", c.ContractSource.Revision); err != nil {
		t.Fatal(err)
	}
	if _, err = reader.LoadFailedDelegationAttempts(ctx, "observing-parent"); err == nil {
		t.Fatal("recovery released a deleted source observation")
	}
	if err = f.sources.CheckRunSources(ctx, "observing-parent"); err == nil {
		t.Fatal("late model/event release ignored source deletion")
	}
}

func TestDelegationFailedAttemptOwnedContextNoTaskIntegration(t *testing.T) {
	f := newDelegationGuardFixture(t)
	ctx := f.leasedWorker(t, f.parent, "conversation")
	if _, err := f.pool.Exec(f.ctx, `UPDATE runs SET task_id=NULL,task_revision=NULL WHERE run_id='conversation'`); err != nil {
		t.Fatal(err)
	}
	reader := failedDelegationAttemptReader{pool: f.pool, manager: f.manager}
	if reports, err := reader.LoadFailedDelegationAttempts(ctx, "conversation"); err != nil || len(reports) != 0 {
		t.Fatal("ordinary conversation acquired an invented task", err, reports)
	}
	token, _ := ownership.TokenFromContext(ctx)
	if _, err := f.leases.ReleaseLease(f.ctx, token.LeaseID); err != nil {
		t.Fatal(err)
	}
	if _, err := reader.LoadFailedDelegationAttempts(ctx, "conversation"); err == nil {
		t.Fatal("ended conversation worker retained observation access")
	}
}

func TestDelegationFailedAttemptHistoryBoundIntegration(t *testing.T) {
	f, _, child, _ := failedDelegationFixture(t, "rate_limited", true)
	if _, err := f.pool.Exec(f.ctx, `INSERT INTO runs(run_id,session_key,input_event_id,status,current_state,autonomy_mode,model_provider,started_at,updated_at,task_id,task_revision) SELECT 'failed-extra-'||n,$1,'fixture','failed','failed','mode_1','fixture',clock_timestamp()+n*INTERVAL '1 second',clock_timestamp(),$2,$3 FROM generate_series(1,17) n`, child.SessionKey, child.TaskID, child.Revision); err != nil {
		t.Fatal(err)
	}
	r, err := f.manager.ObserveFailedAttempts(f.owner, f.parent.TaskID, child.TaskID)
	if err != nil || len(r.Attempts) != 16 || !r.AttemptsTruncated || len(r.ModelUsage) != 2 || r.Reserved.ModelTurns != 2 {
		t.Fatal("bounded history hid branch spending or omitted truncation", err, r)
	}
}

func TestDelegationFailedAttemptSecretSourceIntegration(t *testing.T) {
	f, c, child, _ := failedDelegationFixture(t, "rate_limited", true)
	ctx := f.leasedWorker(t, f.parent, "secret-observing-parent")
	// This is deliberately synthetic credential-like content, never a lab secret.
	if _, err := f.sources.Correct(f.owner, catalog.Correction{SourceID: c.ContractSource.ID, ExpectedRevision: c.ContractSource.Revision, Title: "private", Body: "password=" + strings.Repeat("x", 24), Author: "owner"}); err != nil {
		t.Fatal(err)
	}
	if _, err := f.manager.ObserveFailedAttempts(ctx, f.parent.TaskID, child.TaskID); err == nil {
		t.Fatal("secret/corrected body released a failed-attempt observation")
	}
}

func TestDelegationFailedAttemptPriorRevisionSpendingIntegration(t *testing.T) {
	f, _, child, oldRun := failedDelegationFixture(t, "rate_limited", true)
	if _, err := f.pool.Exec(f.ctx, `UPDATE tasks SET revision=revision+1 WHERE task_id=$1`, child.TaskID); err != nil {
		t.Fatal(err)
	}
	if _, err := f.pool.Exec(f.ctx, `INSERT INTO runs(run_id,session_key,input_event_id,status,current_state,autonomy_mode,model_provider,started_at,updated_at,task_id,task_revision) VALUES('new-failed-revision',$1,'fixture','failed','failed','mode_1','fixture',clock_timestamp(),clock_timestamp(),$2,$3)`, child.SessionKey, child.TaskID, child.Revision+1); err != nil {
		t.Fatal(err)
	}
	r, err := f.manager.ObserveFailedAttempts(f.owner, f.parent.TaskID, child.TaskID)
	if err != nil || r.TaskRevision != child.Revision+2 || len(r.Attempts) != 2 || r.Attempts[0].RunID != "new-failed-revision" || r.Attempts[0].TaskRevision != child.Revision+1 || r.Attempts[1].RunID != oldRun || r.Attempts[1].TaskRevision != child.Revision || len(r.Actions) != 1 || r.Actions[0].TaskRevision != child.Revision || len(r.ModelUsage) != 2 || r.ModelUsage[0].RunID != oldRun || r.ModelUsage[0].TaskRevision != child.Revision || r.ModelUsage[0].InputTokens == nil || *r.ModelUsage[0].InputTokens != 7 || r.ModelUsage[1].InputTokens != nil || r.Reserved.ModelTurns != 2 {
		t.Fatal("revision hid earlier measured/unknown branch spending", err, r)
	}
}
