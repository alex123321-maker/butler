package input

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/butler/butler/apps/orchestrator/internal/completion"
	flow "github.com/butler/butler/apps/orchestrator/internal/orchestrator"
	"github.com/butler/butler/apps/orchestrator/internal/session"
	"github.com/butler/butler/apps/orchestrator/internal/task"
	commonv1 "github.com/butler/butler/internal/gen/common/v1"
	runv1 "github.com/butler/butler/internal/gen/run/v1"
	sessionv1 "github.com/butler/butler/internal/gen/session/v1"
	toolbrokerv1 "github.com/butler/butler/internal/gen/toolbroker/v1"
	"github.com/butler/butler/internal/ownership"
	"github.com/jackc/pgx/v5/pgxpool"
)

type failingScopedCatalog struct {
	pool   *pgxpool.Pool
	calls  int
	pinned bool
}

func TestEarlyTaskPinIsIdempotentAndRejectsOldFenceAndRevisionIntegration(t *testing.T) {
	f := newLifecycleFixture(t)
	manager := session.NewPostgresLeaseManager(f.pool)
	first, err := f.runs.CreateRun(f.ctx, &sessionv1.CreateRunRequest{SessionKey: f.key, InputEvent: &runv1.InputEvent{EventId: "early-pin-first"}, AutonomyMode: commonv1.AutonomyMode_AUTONOMY_MODE_1, ModelProvider: "fixture", MetadataJson: "{}"})
	if err != nil {
		t.Fatal(err)
	}
	lease, err := manager.AcquireLease(f.ctx, session.AcquireLeaseParams{SessionKey: f.key, RunID: first.RunId, OwnerID: "first-worker", TTL: time.Minute})
	if err != nil {
		t.Fatal(err)
	}
	owned := ownership.WithToken(f.ctx, manager.Authority(), ownership.Token{SessionKey: f.key, RunID: first.RunId, LeaseID: lease.LeaseID, FenceEpoch: lease.FenceEpoch})
	for range 2 {
		if err = f.tasks.AttachRun(owned, f.receipt.TaskID, 1, first.RunId, f.key); err != nil {
			t.Fatal("repeat early task attachment was not idempotent", err)
		}
	}
	second, err := f.runs.CreateRun(f.ctx, &sessionv1.CreateRunRequest{SessionKey: f.key, InputEvent: &runv1.InputEvent{EventId: "early-pin-second"}, AutonomyMode: commonv1.AutonomyMode_AUTONOMY_MODE_1, ModelProvider: "fixture", MetadataJson: "{}"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err = f.pool.Exec(f.ctx, `UPDATE session_ownership SET expires_at=clock_timestamp()-interval '1 second' WHERE session_key=$1`, f.key); err != nil {
		t.Fatal(err)
	}
	fresh, err := manager.AcquireLease(f.ctx, session.AcquireLeaseParams{SessionKey: f.key, RunID: second.RunId, OwnerID: "second-worker", TTL: time.Minute})
	if err != nil || fresh.FenceEpoch <= lease.FenceEpoch {
		t.Fatal("takeover did not create a fresh worker fence")
	}
	if err = f.tasks.AttachRun(owned, f.receipt.TaskID, 1, first.RunId, f.key); !errors.Is(err, ownership.ErrStale) {
		t.Fatal("old worker rebound the task after takeover", err)
	}
	freshOwned := ownership.WithToken(f.ctx, manager.Authority(), ownership.Token{SessionKey: f.key, RunID: second.RunId, LeaseID: fresh.LeaseID, FenceEpoch: fresh.FenceEpoch})
	if _, err = f.pool.Exec(f.ctx, `UPDATE tasks SET revision=2,constraints_revision=2 WHERE task_id=$1`, f.receipt.TaskID); err != nil {
		t.Fatal(err)
	}
	if err = f.tasks.AttachRun(freshOwned, f.receipt.TaskID, 1, second.RunId, f.key); !errors.Is(err, task.ErrConflict) {
		t.Fatal("fresh worker accepted old task revision", err)
	}
	if err = f.tasks.AttachRun(freshOwned, f.receipt.TaskID, 2, second.RunId, f.key); err != nil {
		t.Fatal(err)
	}
	var historicalRevision, currentRevision int64
	if err = f.pool.QueryRow(f.ctx, `SELECT task_revision FROM runs WHERE run_id=$1`, first.RunId).Scan(&historicalRevision); err != nil {
		t.Fatal(err)
	}
	if err = f.pool.QueryRow(f.ctx, `SELECT task_revision FROM runs WHERE run_id=$1`, second.RunId).Scan(&currentRevision); err != nil || historicalRevision != 1 || currentRevision != 2 {
		t.Fatal("fresh task attempt rewrote historical pin")
	}
}

func (*failingScopedCatalog) ListTools(context.Context) ([]*toolbrokerv1.ToolContract, error) {
	return nil, errors.New("unscoped catalog must not serve owned run")
}
func (c *failingScopedCatalog) ListToolsForRun(ctx context.Context, runID string) ([]*toolbrokerv1.ToolContract, error) {
	c.calls++
	var id string
	var revision int64
	if err := c.pool.QueryRow(ctx, `SELECT task_id,task_revision FROM runs WHERE run_id=$1`, runID).Scan(&id, &revision); err != nil {
		return nil, err
	}
	c.pinned = id != "" && revision == 1
	return nil, errors.New("current extension catalog unavailable")
}
func TestScopedCatalogFailureRetainsEarlyTaskAttemptWithoutInferenceIntegration(t *testing.T) {
	f := newLifecycleFixture(t)
	p := &lifecycleProvider{}
	catalog := &failingScopedCatalog{pool: f.pool}
	executor := flow.NewService(f.sessions, session.NewPostgresLeaseManager(f.pool), f.runs, f.transcript, p, flow.Config{Completion: completion.New(f.pool), ProviderName: "fixture", ModelName: "fixture", OwnerID: "worker", LeaseTTL: 30, Tasks: f.tasks, Checkpoints: f.store, ApprovalService: f.approvals, ActionAuthorizer: f.tools, Tools: f.tools, ToolCatalog: catalog}, nil)
	worker := NewService(f.store, f.sessions, executor, f.runs, nil)
	worker.process(f.ctx, f.claim(t))
	result := f.assertReceipt(t, "blocked")
	if catalog.calls != 1 || !catalog.pinned || len(p.calls) != 0 || f.tools.calls != 0 {
		t.Fatal("catalog failure lost task pin or made inference/effect")
	}
	var taskID string
	var revision int64
	if err := f.pool.QueryRow(f.ctx, `SELECT task_id,task_revision FROM runs WHERE run_id=$1`, result.RunID).Scan(&taskID, &revision); err != nil || taskID == "" || revision != 1 {
		t.Fatal("failed preparation erased truthful task attempt")
	}
	current, err := f.tasks.Get(f.ctx, taskID)
	if err != nil || current.Revision != 2 || current.ConstraintsRevision != 1 || current.Status != "blocked" || current.CompletionResultID != "" {
		t.Fatalf("technical preparation failure: revision=%d status=%s completed=%t error=%v", current.Revision, current.Status, current.CompletionResultID != "", err)
	}
}
