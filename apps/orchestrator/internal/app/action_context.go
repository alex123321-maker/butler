package app

import (
	"context"
	"encoding/json"
	"fmt"
	"github.com/butler/butler/apps/orchestrator/internal/communication"
	"strings"
	"time"

	approvals "github.com/butler/butler/apps/orchestrator/internal/approval"
	flow "github.com/butler/butler/apps/orchestrator/internal/orchestrator"
	"github.com/butler/butler/apps/orchestrator/internal/session"
	"github.com/butler/butler/apps/orchestrator/internal/task"
	"github.com/butler/butler/internal/actionauth"
	"github.com/butler/butler/internal/delegation"
	commonv1 "github.com/butler/butler/internal/gen/common/v1"
	"github.com/butler/butler/internal/memory/experience"
	"github.com/butler/butler/internal/owneridentity"
	"github.com/butler/butler/internal/ownership"
	"github.com/jackc/pgx/v5/pgxpool"
)

// The private reader assembles authority from service-owned state. Tool Broker
// cannot mint owners, task versions or worker fences from model arguments.
type actionContextReader struct {
	runs                     flow.RunManager
	sessions                 session.Repository
	leases                   *session.PostgresLeaseManager
	tasks                    task.Manager
	approvals                *approvals.Service
	pool                     *pgxpool.Pool
	mandateID                string
	converterLifecycleReady  bool
	delegationReady          bool
	guardedTabRequestReady   bool
	guardedTabRuntimeReady   bool
	memoryCandidateReady     bool
	communicationReady       bool
	schedulesReady           bool
	toolDiscoveryReady       bool
	memoryReadReady          bool
	memoryTemporalReady      bool
	workspaceReady           bool
	converterDelegationReady bool
	lessons                  *experience.Store
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
	c.ConverterLifecycleReady = r.converterLifecycleReady
	c.DelegationReady = r.delegationReady
	c.GuardedTabRequestReady = r.guardedTabRequestReady
	c.GuardedTabRuntimeReady = r.guardedTabRuntimeReady
	c.MemoryCandidateReady = r.memoryCandidateReady
	c.SchedulesReady = r.schedulesReady
	if r.communicationReady {
		var raw []byte
		e := r.pool.QueryRow(ctx, `SELECT a.methods FROM communication_admissions a JOIN accepted_inputs i USING(input_id) WHERE i.run_id=$1 AND a.schema_epoch=1 AND a.owner_id=$2 AND a.session_key=$3`, runID, c.OwnerID, c.SessionKey).Scan(&raw)
		if e == nil && json.Unmarshal(raw, &c.CommunicationMethods) == nil {
			c.CommunicationReady = true
			c.CommunicationMode = "explicit"
			c.CommunicationTargets = map[string]string{}
			for _, method := range c.CommunicationMethods {
				if hash, e := communication.DestinationHash(ctx, r.pool, c.OwnerID, strings.TrimPrefix(method, "communication.")); e == nil {
					c.CommunicationTargets[method] = hash
				}
			}
		}
	}
	c.MemoryReadReady = r.memoryReadReady
	c.MemoryTemporalReady = r.memoryTemporalReady
	c.WorkspaceReady = r.workspaceReady
	if r.lessons != nil {
		c.ExperienceEnvironmentHash = r.lessons.EnvironmentHash
	}
	if r.communicationReady || r.toolDiscoveryReady || r.schedulesReady || r.guardedTabRequestReady || r.memoryCandidateReady || r.memoryReadReady || r.workspaceReady {
		var inputID, claimID string
		if r.pool.QueryRow(ctx, `SELECT input_id,claim_id FROM accepted_inputs WHERE run_id=$1 AND session_key=$2 AND owner_id=$3 AND status='running' AND claim_expires_at>clock_timestamp() AND expires_at>clock_timestamp()`, runID, c.SessionKey, c.OwnerID).Scan(&inputID, &claimID) == nil {
			c.InputClaimHash = actionauth.InputClaimHash(inputID, claimID)
		}
	}
	c.ConverterDelegationReady = r.converterDelegationReady
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
		c.ExecutionPolicy = record.ExecutionPolicy
		tx, err := r.pool.Begin(ctx)
		if err != nil {
			return c, err
		}
		defer tx.Rollback(ctx)
		var at time.Time
		if err = tx.QueryRow(ctx, `SELECT clock_timestamp()`).Scan(&at); err != nil {
			return c, err
		}
		a, err := actionauth.CheckTaskAuthorityTx(ctx, tx, record.TaskID, record.OwnerID, nil, at)
		if err != nil {
			return c, err
		}
		c.Delegation, err = delegation.LoadTx(ctx, tx, record.TaskID)
		if err != nil {
			return c, err
		}
		live := ownership.WithToken(owneridentity.WithActor(ctx, owneridentity.Actor{}), nil, ownership.Token{SessionKey: c.SessionKey, RunID: c.RunID, LeaseID: c.LeaseID, FenceEpoch: c.LeaseEpoch})
		if err = r.lessons.CheckPinnedTx(live, tx, runID); err != nil {
			return c, err
		}
		if a.TaskID != "" {
			m, g, err := actionauth.CheckAssignmentTx(ctx, tx, a, at)
			if err != nil {
				return c, err
			}
			c.Authority = &a
			c.MandateID = a.MandateID
			c.ModelPrivacy = m.Privacy
			c.AuthorityDeadline, err = actionauth.AuthorityDeadline(m, g, at)
			if err != nil {
				return c, err
			}
			c.DenyTools, c.DenyResources, err = actionauth.ReadGoalDenialsTx(ctx, tx, a.GoalID, a.OwnerID, at)
			if err != nil {
				return c, err
			}
		}
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
