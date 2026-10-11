package task_test

import (
	"context"
	"errors"
	"testing"

	"github.com/butler/butler/apps/orchestrator/internal/approval"
	"github.com/butler/butler/apps/orchestrator/internal/input"
	"github.com/butler/butler/apps/orchestrator/internal/run"
	"github.com/butler/butler/apps/orchestrator/internal/session"
	"github.com/butler/butler/apps/orchestrator/internal/task"
	"github.com/butler/butler/internal/memory/memorytest"
	"github.com/butler/butler/internal/memory/transcript"
	"github.com/jackc/pgx/v5"
)

func TestControlGuardDenialDoesNotRevokeOrCommitIntegration(t *testing.T) {
	pool := memorytest.Database(t)
	ctx := context.Background()
	tasks := task.NewStore(pool)
	leases := session.NewPostgresLeaseManager(pool)
	if _, e := pool.Exec(ctx, `INSERT INTO sessions(session_key,user_id,channel) VALUES('guard-owner','guard-owner','web')`); e != nil {
		t.Fatal(e)
	}
	record, createError := tasks.Create(ctx, task.CreateRequest{SessionKey: "guard-owner", OwnerID: "guard-owner", Goal: "prepare", CompletionCriteria: []string{"review"}})
	if createError != nil {
		t.Fatal(createError)
	}
	controls := task.NewController(tasks, leases.Authority(), leases, run.NewPostgresRepository(pool), approval.NewPostgresRepository(pool), input.NewStore(pool), transcript.NewStore(pool))
	f := &controlsFixture{ctx: ctx, pool: pool, tasks: tasks, leases: leases, controls: controls, session: "guard-owner", owner: "guard-owner", root: record}
	_, lease := f.startRun(t, f.root.TaskID, "prepared")
	denied := errors.New("current source denied")
	_, e := f.controls.ApplyWithGuard(f.actor("guard-denied", "web"), task.ControlRequest{TaskID: f.root.TaskID, ExpectedRevision: 1, Kind: task.ControlStop}, func(ctx context.Context, tx pgx.Tx) error {
		var state string
		if e := tx.QueryRow(ctx, `SELECT status FROM tasks WHERE task_id=$1`, f.root.TaskID).Scan(&state); e != nil {
			return e
		}
		if state != "active" {
			t.Fatal(state)
		}
		return denied
	})
	if !errors.Is(e, denied) {
		t.Fatal("guard denial ignored", e)
	}
	var current string
	var epoch int64
	if e = f.pool.QueryRow(f.ctx, `SELECT lease_id,fence_epoch FROM session_ownership WHERE session_key=$1`, f.session).Scan(&current, &epoch); e != nil || current != lease.LeaseID || epoch != lease.FenceEpoch {
		t.Fatal("denied guard revoked live authority", current, epoch, e)
	}
	r, e := f.tasks.Get(f.ctx, f.root.TaskID)
	if e != nil || r.Status != "active" || r.Revision != 1 {
		t.Fatal("denied guard mutated task", r, e)
	}
	var n int
	if e = f.pool.QueryRow(f.ctx, `SELECT count(*) FROM task_control_events WHERE owner_id=$1 AND delivery_id='guard-denied'`, f.owner).Scan(&n); e != nil || n != 0 {
		t.Fatal("denied receipt committed", n, e)
	}
	calls := 0
	allow := func(context.Context, pgx.Tx) error { calls++; return nil }
	req := task.ControlRequest{TaskID: f.root.TaskID, ExpectedRevision: 1, Kind: task.ControlStop}
	first, e := f.controls.ApplyWithGuard(f.actor("guard-allowed", "web"), req, allow)
	if e != nil {
		t.Fatal(e)
	}
	second, e := f.controls.ApplyWithGuard(f.actor("guard-allowed", "web"), req, allow)
	if e != nil || first.ControlID != second.ControlID || calls != 2 {
		t.Fatal("guarded receipt recovery", calls, e)
	}
}
