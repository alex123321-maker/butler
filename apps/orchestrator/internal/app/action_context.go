package app

import (
	"context"
	"fmt"

	approvals "github.com/butler/butler/apps/orchestrator/internal/approval"
	flow "github.com/butler/butler/apps/orchestrator/internal/orchestrator"
	"github.com/butler/butler/apps/orchestrator/internal/session"
	"github.com/butler/butler/apps/orchestrator/internal/task"
	"github.com/butler/butler/internal/actionauth"
	commonv1 "github.com/butler/butler/internal/gen/common/v1"
	"github.com/jackc/pgx/v5/pgxpool"
)

// The private reader assembles authority from service-owned state. Tool Broker
// cannot mint owners, task versions or worker fences from model arguments.
type actionContextReader struct {
	runs      flow.RunManager
	sessions  session.Repository
	leases    *session.PostgresLeaseManager
	tasks     task.Manager
	approvals *approvals.Service
	pool      *pgxpool.Pool
	mandateID string
}

func (r *actionContextReader) GetActionContext(ctx context.Context, runID string) (actionauth.ContextSnapshot, error) {
	var c actionauth.ContextSnapshot
	if r == nil || r.runs == nil || r.sessions == nil || r.leases == nil || r.pool == nil {
		return c, fmt.Errorf("authoritative action context unavailable")
	}
	run, err := r.runs.GetRun(ctx, runID)
	if err != nil {
		return c, err
	}
	sessionRecord, err := r.sessions.GetSessionByKey(ctx, run.GetSessionKey())
	if err != nil {
		return c, err
	}
	lease, err := r.leases.GetLease(ctx, run.GetLeaseId())
	if err != nil {
		return c, err
	}
	lease, err = r.leases.ValidateLease(ctx, lease.LeaseID, runID, lease.FenceEpoch)
	if err != nil || lease.SessionKey != run.GetSessionKey() {
		return c, fmt.Errorf("run ownership is expired or replaced")
	}
	c = actionauth.ContextSnapshot{RunID: runID, RunState: run.GetCurrentState().String(), SessionKey: run.GetSessionKey(), OwnerID: sessionRecord.UserID, AutonomyMode: int32(run.GetAutonomyMode()), MandateID: r.mandateID, LeaseID: lease.LeaseID, LeaseEpoch: lease.FenceEpoch, LeaseExpiresAt: lease.ExpiresAt}
	// This projection stays inside orchestrator; it is a read of Run Service's
	// admission binding, never a Broker write into another service's tables.
	var id string
	var pinned int64
	if err = r.pool.QueryRow(ctx, `SELECT COALESCE(task_id,''),COALESCE(task_revision,0) FROM runs WHERE run_id=$1`, runID).Scan(&id, &pinned); err != nil {
		return c, err
	}
	if id != "" {
		if r.tasks == nil {
			return c, fmt.Errorf("current task authority unavailable")
		}
		record, err := r.tasks.GetByRun(ctx, runID)
		if err != nil || record.TaskID != id || record.OwnerID != c.OwnerID || record.SessionKey != c.SessionKey {
			return c, fmt.Errorf("task authority is incompatible")
		}
		c.TaskID = record.TaskID
		c.PinnedTaskRevision = pinned
		c.TaskRevision = record.Revision
		c.ConstraintsRevision = record.ConstraintsRevision
		c.TaskStatus = record.Status
	}
	return c, nil
}
func (r *actionContextReader) VerifyActionApproval(ctx context.Context, request actionauth.ApprovalRequest) (actionauth.ApprovalSnapshot, error) {
	c, err := r.GetActionContext(ctx, request.Binding.RunID)
	if err != nil || c.OwnerID != request.Binding.OwnerID || c.TaskID != request.Binding.TaskID || c.TaskRevision != request.Binding.TaskRevision || c.ConstraintsRevision != request.Binding.ConstraintsRevision || r.approvals == nil {
		return actionauth.ApprovalSnapshot{}, fmt.Errorf("current approved action authority unavailable")
	}
	return r.approvals.VerifyActionApproval(ctx, request)
}
func autonomyMode(value string) commonv1.AutonomyMode {
	switch value {
	case "mode_0":
		return commonv1.AutonomyMode_AUTONOMY_MODE_0
	case "mode_2":
		return commonv1.AutonomyMode_AUTONOMY_MODE_2
	case "mode_3":
		return commonv1.AutonomyMode_AUTONOMY_MODE_3
	default:
		return commonv1.AutonomyMode_AUTONOMY_MODE_1
	}
}
