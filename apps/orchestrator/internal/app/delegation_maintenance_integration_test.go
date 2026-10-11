package app

import (
	"testing"

	durableinput "github.com/butler/butler/apps/orchestrator/internal/input"
	"github.com/butler/butler/apps/orchestrator/internal/session"
	"github.com/butler/butler/apps/orchestrator/internal/task"
)

func TestDelegationMaintenanceDoesNotFreeWidthForHistoricalFinalWithLiveAttemptIntegration(t *testing.T) {
	f := newDelegationGuardFixture(t)
	var err error
	f.manager, err = task.NewDelegationManager(f.store, session.NewPostgresRepository(f.pool), durableinput.NewStore(f.pool), f.guard, task.DefaultDelegationLimits())
	if err != nil {
		t.Fatal(err)
	}
	first, child := f.admit(t, "historical-first")
	second, waiting := f.admit(t, "historical-second")
	if first.State != "queued" || second.State != "waiting" {
		t.Fatal(first.State, second.State)
	}
	result, _, _ := f.complete(t, first, child, false, false)
	var leaseID string
	if err = f.pool.QueryRow(f.ctx, `SELECT lease_id FROM session_ownership WHERE session_key=$1`, child.SessionKey).Scan(&leaseID); err != nil {
		t.Fatal(err)
	}
	if released, releaseErr := f.leases.ReleaseLease(f.ctx, leaseID); releaseErr != nil || !released {
		t.Fatal("completed attempt lease could not be released", released, releaseErr)
	}
	f.worker(t, child, "second-live-attempt")
	var sourcesBefore int
	if err = f.pool.QueryRow(f.ctx, `SELECT count(*) FROM memory_sources`).Scan(&sourcesBefore); err != nil {
		t.Fatal(err)
	}
	if _, err = f.manager.CollectGenerated(f.ctx, f.parent.TaskID, child.TaskID, result.ResultID); err == nil {
		t.Fatal("historical final released a currently occupied child slot")
	}
	if err = (&delegationMaintenance{pool: f.pool, manager: f.manager}).Sweep(f.ctx); err != nil {
		t.Fatal(err)
	}
	current, err := f.manager.Get(f.ctx, child.TaskID)
	if err != nil || current.Contract.State != "executing" || current.Report != nil {
		t.Fatal("collection rewrote the live attempt", current, err)
	}
	queued, err := f.manager.Get(f.ctx, waiting.TaskID)
	if err != nil || queued.Contract.State != "waiting" || queued.Contract.InputID != "" {
		t.Fatal("historical final admitted another executor", queued, err)
	}
	var receipts, reports int
	if err = f.pool.QueryRow(f.ctx, `SELECT count(*) FROM accepted_inputs WHERE task_id=$1`, waiting.TaskID).Scan(&receipts); err != nil {
		t.Fatal(err)
	}
	if err = f.pool.QueryRow(f.ctx, `SELECT count(*) FROM memory_sources`).Scan(&reports); err != nil {
		t.Fatal(err)
	}
	if receipts != 0 || reports != sourcesBefore {
		t.Fatal("denied collection left a receipt or report", receipts, reports)
	}
}

func TestDelegationMaintenanceCollectsThenAdvancesSingleWidthIntegration(t *testing.T) {
	f := newDelegationGuardFixture(t)
	var err error
	f.manager, err = task.NewDelegationManager(f.store, session.NewPostgresRepository(f.pool), durableinput.NewStore(f.pool), f.guard, task.DefaultDelegationLimits())
	if err != nil {
		t.Fatal(err)
	}
	first, firstTask := f.admit(t, "maintenance-first")
	second, secondTask := f.admit(t, "maintenance-second")
	if first.State != "queued" || second.State != "waiting" {
		t.Fatal("single width admission changed", first.State, second.State)
	}
	result, _, _ := f.complete(t, first, firstTask, false, false)
	w := &delegationMaintenance{pool: f.pool, manager: f.manager}
	if err = w.Sweep(f.ctx); err != nil {
		t.Fatal(err)
	}
	collected, err := f.manager.Get(f.ctx, firstTask.TaskID)
	if err != nil || collected.Contract.State != "result_ready" || collected.ResultID != result.ResultID || collected.Report == nil {
		t.Fatal(collected, err)
	}
	advanced, err := f.manager.Get(f.ctx, secondTask.TaskID)
	if err != nil || advanced.Contract.State != "queued" || advanced.Contract.InputID == "" {
		t.Fatal("committed first final did not free the executor slot", advanced, err)
	}
	if err = w.Sweep(f.ctx); err != nil {
		t.Fatal(err)
	}
	var count int
	if err = f.pool.QueryRow(f.ctx, `SELECT count(*) FROM accepted_inputs WHERE task_id=$1`, secondTask.TaskID).Scan(&count); err != nil || count != 1 {
		t.Fatal("maintenance duplicated the bounded receipt", count, err)
	}
	current, err := f.store.Get(f.ctx, firstTask.TaskID)
	if err != nil || current.Status != "active" {
		t.Fatal("collection promoted semantic task completion", current, err)
	}
}

func TestDelegationContextTransitionUsesActualChildFenceIntegration(t *testing.T) {
	f := newDelegationGuardFixture(t)
	c, child := f.admit(t, "transition-contract")
	ctx := f.leasedWorker(t, child, "transition-run")
	reader := delegatedContextReader{sources: f.sources, manager: f.manager}
	selected, err := reader.LoadDelegatedContext(ctx, "transition-run")
	if err != nil || selected == nil || selected.TaskID != child.TaskID {
		t.Fatal(selected, err)
	}
	current, err := f.manager.Get(f.ctx, child.TaskID)
	if err != nil || current.Contract.State != "executing" {
		t.Fatal("prepared source context lacked current execution transition", current, err)
	}
	if _, err = f.pool.Exec(f.ctx, `UPDATE session_ownership SET fence_epoch=fence_epoch+1 WHERE session_key=$1`, child.SessionKey); err != nil {
		t.Fatal(err)
	}
	if _, err = reader.LoadDelegatedContext(ctx, "transition-run"); err == nil {
		t.Fatal("stale child retained context release")
	}
	if _, err = f.sources.Delete(f.owner, c.ContractSource.ID, "delete", c.ContractSource.Revision); err != nil {
		t.Fatal(err)
	}
	if _, err = reader.LoadDelegatedContext(ctx, "transition-run"); err == nil {
		t.Fatal("deleted contract retained context release")
	}
}
