package app

import (
	"context"
	"encoding/json"
	"time"

	durableinput "github.com/butler/butler/apps/orchestrator/internal/input"
	"github.com/butler/butler/apps/orchestrator/internal/scheduled"
	"github.com/butler/butler/apps/orchestrator/internal/session"
	"github.com/butler/butler/internal/actionauth"
	"github.com/butler/butler/internal/delegation"
	"github.com/butler/butler/internal/memory/sanitize"
	"github.com/butler/butler/internal/owneridentity"
	"github.com/butler/butler/internal/ownership"
	"github.com/butler/butler/internal/resources"
	"github.com/butler/butler/internal/schedulecontrol"
)

type ScheduleControl struct{ Store *scheduled.Store }

func (s *ScheduleControl) ExecuteDispatched(ctx context.Context, e schedulecontrol.Envelope) (schedulecontrol.Receipt, error) {
	denied := func() (schedulecontrol.Receipt, error) { return schedulecontrol.Receipt{}, schedulecontrol.ErrDenied }
	if s == nil || s.Store == nil || !schedulecontrol.IsTool(e.Tool) {
		return denied()
	}
	if _, owner := owneridentity.FromContext(ctx); owner {
		return denied()
	}
	a, err := schedulecontrol.DecodeArgs(e.Tool, e.ArgsJSON)
	if err != nil {
		return denied()
	}
	if a.Job != nil {
		raw, _ := json.Marshal(a.Job)
		original, hashErr := actionauth.ArgumentsHash(string(raw))
		masked, maskedErr := actionauth.ArgumentsHash(sanitize.JSON(string(raw)))
		if hashErr != nil || maskedErr != nil || original != masked {
			return denied()
		}
	}
	contract := schedulecontrol.Contract(e.Tool)
	hash, err := actionauth.ContractHash(contract)
	if err != nil || hash != e.ContractHash {
		return denied()
	}
	envelope := delegation.AdmissionEnvelope{RunID: e.RunID, ToolCallID: e.ToolCallID, EffectID: e.EffectID, ArgsJSON: e.ArgsJSON, ContractHash: e.ContractHash}
	effect := actionauth.EffectPrepare
	if schedulecontrol.ReadOnly(e.Tool) {
		effect = actionauth.EffectRead
	}
	read := func(ctx context.Context, q admissionQuery, lock bool) (actionauth.Grant, error) {
		return readOwnedDispatch(ctx, q, envelope, e.Tool, hash, schedulecontrol.Revision, effect, lock, true)
	}
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	grant, err := read(ctx, s.Store.Pool, false)
	if err != nil {
		return denied()
	}
	b := grant.Binding
	var key string
	var expiry time.Time
	if s.Store.Pool.QueryRow(ctx, `SELECT r.session_key,o.expires_at FROM runs r JOIN session_ownership o USING(session_key) WHERE r.run_id=$1 AND o.run_id=r.run_id AND o.lease_id=$2 AND o.fence_epoch=$3 AND o.expires_at>clock_timestamp()`, e.RunID, grant.LeaseID, grant.LeaseEpoch).Scan(&key, &expiry) != nil {
		return denied()
	}
	var claim durableinput.Record
	if s.Store.Pool.QueryRow(ctx, `SELECT input_id,claim_id FROM accepted_inputs WHERE run_id=$1 AND session_key=$2 AND owner_id=$3 AND COALESCE(task_id,'')=$4 AND status='running' AND checkpoint='tool_dispatching' AND claim_expires_at>clock_timestamp() AND expires_at>clock_timestamp()`, e.RunID, key, b.OwnerID, b.TaskID).Scan(&claim.InputID, &claim.ClaimID) != nil || grant.InputClaimHash == "" || actionauth.InputClaimHash(claim.InputID, claim.ClaimID) != grant.InputClaimHash {
		return denied()
	}
	if grant.ExpiresAt.Before(expiry) {
		expiry = grant.ExpiresAt
	}
	deadline, _ := ctx.Deadline()
	base, stop := context.WithDeadline(context.Background(), deadline)
	forward := context.AfterFunc(ctx, stop)
	defer stop()
	defer forward()
	leases := session.NewPostgresLeaseManager(s.Store.Pool)
	owned := ownership.WithToken(durableinput.WithClaim(base, claim), leases.Authority(), ownership.Token{SessionKey: key, RunID: e.RunID, LeaseID: grant.LeaseID, FenceEpoch: grant.LeaseEpoch})
	tx, err := s.Store.Pool.Begin(owned)
	if err != nil {
		return denied()
	}
	defer tx.Rollback(context.Background())
	// Existing session/task/job locks precede goal/mandate locks. Maintenance
	// takes the same order, so an owner update cannot deadlock occurrence admission.
	var live bool
	if tx.QueryRow(owned, `SELECT TRUE FROM session_ownership o JOIN sessions s USING(session_key) JOIN runs r ON r.run_id=o.run_id WHERE o.session_key=$1 AND o.run_id=$2 AND o.lease_id=$3 AND o.fence_epoch=$4 AND o.expires_at>clock_timestamp() AND s.user_id=$5 AND r.current_state='tool_running' AND r.status=r.current_state FOR SHARE OF o,s,r`, key, e.RunID, grant.LeaseID, grant.LeaseEpoch, b.OwnerID).Scan(&live) != nil {
		return denied()
	}
	if b.TaskID != "" {
		var id string
		if tx.QueryRow(owned, `SELECT task_id FROM tasks WHERE task_id=$1 FOR SHARE`, b.TaskID).Scan(&id) != nil {
			return denied()
		}
	}
	if a.ScheduleID != "" {
		var id string
		if tx.QueryRow(owned, `SELECT schedule_id FROM scheduled_jobs WHERE schedule_id=$1 AND owner_id=$2 AND session_key=$3 FOR UPDATE`, a.ScheduleID, b.OwnerID, key).Scan(&id) != nil {
			return denied()
		}
	}
	p := scheduled.Principal{OwnerID: b.OwnerID, SessionKey: key, RunID: e.RunID, SourceTaskID: b.TaskID, ExecutionPolicy: "normal", Constraints: []string{}, Approved: grant.ApprovalID != ""}
	check := func() error {
		if b.TaskID != "" {
			if (resources.WorkerGuard{}).CheckTx(owned, tx, resources.Binding{OwnerID: b.OwnerID, SessionKey: key, RunID: e.RunID, TaskID: b.TaskID, TaskRevision: b.TaskRevision, ConstraintsRevision: b.ConstraintsRevision, LeaseEpoch: grant.LeaseEpoch, Deadline: expiry}) != nil {
				return schedulecontrol.ErrDenied
			}
		} else {
			var valid bool
			if tx.QueryRow(owned, `SELECT TRUE FROM session_ownership o JOIN sessions s USING(session_key) JOIN runs r ON r.run_id=o.run_id WHERE o.session_key=$1 AND o.run_id=$2 AND o.lease_id=$3 AND o.fence_epoch=$4 AND o.expires_at>clock_timestamp() AND s.user_id=$5 AND r.task_id IS NULL AND r.current_state='tool_running' AND r.status=r.current_state FOR SHARE OF o,s,r`, key, e.RunID, grant.LeaseID, grant.LeaseEpoch, b.OwnerID).Scan(&valid) != nil {
				return schedulecontrol.ErrDenied
			}
		}
		if s.Store.Inputs.CheckDispatchClaimTx(owned, tx, e.RunID, e.ToolCallID, e.EffectID, grant.ID) != nil {
			return schedulecontrol.ErrDenied
		}
		current, err := read(owned, tx, true)
		if err != nil || current.ID != grant.ID || current.InputClaimHash != grant.InputClaimHash || current.Binding.ActionHash != b.ActionHash {
			return schedulecontrol.ErrDenied
		}
		return s.Store.SourceGuard(owned, tx, e.RunID)
	}
	if err = check(); err != nil {
		return denied()
	}
	if b.TaskID != "" {
		record, err := s.Store.Tasks.GetTx(owned, tx, b.TaskID)
		if err != nil {
			return denied()
		}
		if bounded, err := delegation.LoadTx(owned, tx, b.TaskID); err != nil || bounded != nil {
			return denied()
		}
		// An occurrence cannot perpetuate itself by creating a new root schedule.
		// Reads remain useful; mutations need a fresh owner conversation.
		if record.Source["type"] == "scheduled_job" && !schedulecontrol.ReadOnly(e.Tool) {
			return denied()
		}
		p.SourceGoal = record.Goal
		p.ExecutionPolicy = record.ExecutionPolicy
		p.Constraints = record.Constraints
		var now time.Time
		if tx.QueryRow(owned, `SELECT clock_timestamp()`).Scan(&now) != nil {
			return denied()
		}
		assigned, err := actionauth.CheckTaskAuthorityTx(owned, tx, b.TaskID, b.OwnerID, nil, now)
		if err != nil {
			return denied()
		}
		if assigned.TaskID != "" {
			p.Authority = &assigned
		}
	}
	// Serialize owner-wide admission limits, after owning task locks and before
	// any job is modified. The advisory key contains no model-selected identity.
	if _, err = tx.Exec(owned, `SELECT pg_advisory_xact_lock(hashtextextended($1,0))`, "schedule_owner:"+b.OwnerID); err != nil {
		return denied()
	}
	receipt, err := s.Store.ApplyTx(owned, tx, p, e.Tool, e.EffectID, b.ActionHash, a)
	if err != nil {
		return denied()
	}
	if err = check(); err != nil {
		return denied()
	}
	if err = tx.Commit(owned); err != nil {
		return schedulecontrol.Receipt{}, err
	}
	return receipt, nil
}

var _ schedulecontrol.Service = (*ScheduleControl)(nil)
