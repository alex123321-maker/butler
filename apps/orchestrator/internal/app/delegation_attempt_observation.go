package app

import (
	"context"
	"encoding/json"
	"time"

	"github.com/butler/butler/apps/orchestrator/internal/task"
	"github.com/butler/butler/internal/actionauth"
	"github.com/butler/butler/internal/delegation"
	"github.com/butler/butler/internal/transport"
	"github.com/jackc/pgx/v5"
)

var _ task.DelegationAttemptObserver = (*DelegationGuards)(nil)

// A blocked child's former execution permissions/deadline are not resurrected.
// Current parent authority and Catalog access govern the observation. AccessTx
// additionally pins sources to a live parent run for deletion/event-release
// checks. No child/report/result/effect/budget state is changed.
func (g *DelegationGuards) ObserveAttemptsTx(ctx context.Context, tx pgx.Tx, parent, child task.Record, c delegation.Contract) (delegation.AttemptReport, error) {
	fail := func() (delegation.AttemptReport, error) { return delegation.AttemptReport{}, delegation.ErrDenied }
	if currentDelegationTask(ctx, tx, parent) != nil {
		return fail()
	}
	var now time.Time
	if err := tx.QueryRow(ctx, `SELECT clock_timestamp()`).Scan(&now); err != nil {
		return fail()
	}
	if _, err := actionauth.CheckTaskAuthorityTx(ctx, tx, parent.TaskID, parent.OwnerID, nil, now); err != nil {
		return fail()
	}
	if _, err := g.sources.AccessTx(ctx, tx, refsForDelegation(c), "memory_read"); err != nil {
		return delegation.AttemptReport{}, err
	}
	r := delegation.AttemptReport{TaskID: child.TaskID, ParentTaskID: parent.TaskID, TaskRevision: child.Revision, ConstraintsRevision: child.ConstraintsRevision, ContractRevision: c.Revision, TaskStatus: child.Status, DelegationState: c.State, Sources: append(append([]delegation.SourceRef(nil), c.Sources...), c.ContractSource), Uncertainty: []delegation.Reason{{Kind: "execution_failed"}, {Kind: "verification_missing"}}}
	// Only exact known persisted transport subtypes may leave the owning DB.
	// Raw error messages, metadata, checkpoints and source bodies are not scanned.
	codes := []string{string(transport.ErrorTypeProviderUnavailable), string(transport.ErrorTypeTransportConnectionError), string(transport.ErrorTypeProviderTimeout), string(transport.ErrorTypeProviderProtocolError), string(transport.ErrorTypeCapabilityMismatch), string(transport.ErrorTypeInvalidToolRequest), string(transport.ErrorTypeStatefulSessionLost), string(transport.ErrorTypeRateLimited), string(transport.ErrorTypeInternalTransportError), string(transport.ErrorTypeInputBytesExceeded), string(transport.ErrorTypeMemorySourceStale), string(transport.ErrorTypeMemorySourceTimeout)}
	rows, err := tx.Query(ctx, `SELECT run_id,current_state,CASE WHEN error_type=ANY($4) THEN error_type ELSE 'execution_failed' END,CASE WHEN error_type='transport_error' AND error_message=ANY($5) THEN error_message ELSE '' END FROM runs WHERE task_id=$1 AND task_revision=$2 AND session_key=$3 AND current_state IN ('failed','cancelled','timed_out') ORDER BY started_at DESC,run_id DESC LIMIT 17`, child.TaskID, child.Revision, child.SessionKey, []string{"transport_error", "execution_stopped", "tool_execution_error", "execution_failed", "lease_lost", "task_revision_conflict"}, codes)
	if err != nil {
		return fail()
	}
	for rows.Next() {
		var a delegation.FailedAttempt
		if err = rows.Scan(&a.RunID, &a.State, &a.ErrorCode, &a.DiagnosticCode); err != nil {
			break
		}
		r.Attempts = append(r.Attempts, a)
	}
	if err == nil {
		err = rows.Err()
	}
	rows.Close()
	if err != nil || len(r.Attempts) == 0 {
		return fail()
	}
	if len(r.Attempts) > 16 {
		r.AttemptsTruncated = true
		r.Attempts = r.Attempts[:16]
	}
	// Include current journal states for every failed attempt, even ones outside
	// the bounded visible run list. A journal observation cannot prove task success.
	rows, err = tx.Query(ctx, `SELECT i.run_id,i.action_id,i.action_hash,i.binding,i.effect_id,e.state FROM action_intents i JOIN action_effects e ON e.action_id=i.effect_id JOIN runs r USING(run_id) WHERE r.task_id=$1 AND r.task_revision=$2 AND r.session_key=$3 AND r.current_state IN ('failed','cancelled','timed_out') ORDER BY i.action_id LIMIT 65 FOR SHARE OF i,e`, child.TaskID, child.Revision, child.SessionKey)
	if err != nil {
		return fail()
	}
	for rows.Next() {
		var a delegation.AttemptAction
		var hash string
		var raw []byte
		var b actionauth.ActionBinding
		if err = rows.Scan(&a.RunID, &a.ActionID, &hash, &raw, &a.EffectID, &a.State); err != nil {
			break
		}
		if json.Unmarshal(raw, &b) != nil || b.ActionID != a.ActionID || b.ActionHash != hash || b.RunID != a.RunID || b.TaskID != child.TaskID || b.TaskRevision != child.Revision || b.ConstraintsRevision != child.ConstraintsRevision || b.OwnerID != child.OwnerID {
			err = delegation.ErrDenied
			break
		}
		switch a.State {
		case "prepared", "allowed", "executing", "effect_confirmed", "effect_absent", "effect_unknown":
		default:
			err = delegation.ErrDenied
		}
		if err != nil {
			break
		}
		r.Actions = append(r.Actions, a)
	}
	if err == nil {
		err = rows.Err()
	}
	rows.Close()
	if err != nil {
		return fail()
	}
	if len(r.Actions) > 64 {
		r.ActionsTruncated = true
		r.Actions = r.Actions[:64]
	}
	for _, a := range r.Actions {
		if a.State == "effect_unknown" || a.State == "executing" {
			r.Uncertainty = append(r.Uncertainty, delegation.Reason{Kind: "effect_unknown"})
			break
		}
	}
	a := delegationReportEvidence{refs: map[string]delegation.EvidenceRef{}}
	if err = loadDelegationBudget(ctx, tx, child, &a); err != nil {
		return delegation.AttemptReport{}, err
	}
	r.Reserved, r.ReservationIDs = a.used, a.reservations
	// Branch allowance does not reset at a task revision. Preserve measured and
	// unknown usage from earlier revisions as well as the current failed attempt.
	rows, err = tx.Query(ctx, `SELECT a.run_id,r.task_revision,a.operation_id,a.reservation_id,a.actual_input_tokens,a.actual_output_tokens FROM execution_model_attempts a JOIN runs r USING(run_id) WHERE r.task_id=$1 AND r.session_key=$2 ORDER BY a.created_at,a.operation_id LIMIT 129`, child.TaskID, child.SessionKey)
	if err != nil {
		return fail()
	}
	for rows.Next() {
		var u delegation.AttemptUsage
		if err = rows.Scan(&u.RunID, &u.TaskRevision, &u.OperationID, &u.ReservationID, &u.InputTokens, &u.OutputTokens); err != nil {
			break
		}
		if u.TaskRevision < 1 || !hasExact(r.ReservationIDs, u.ReservationID) || u.InputTokens != nil && *u.InputTokens < 0 || u.OutputTokens != nil && *u.OutputTokens < 0 {
			err = delegation.ErrDenied
			break
		}
		r.ModelUsage = append(r.ModelUsage, u)
	}
	if err == nil {
		err = rows.Err()
	}
	rows.Close()
	if err != nil || len(r.ModelUsage) > 128 {
		return fail()
	}
	// Revalidate current parent authority/source state and DB time after waits.
	if _, err = g.sources.AccessTx(ctx, tx, refsForDelegation(c), "memory_read"); err != nil {
		return delegation.AttemptReport{}, err
	}
	if err = tx.QueryRow(ctx, `SELECT clock_timestamp()`).Scan(&r.ObservedAt); err != nil {
		return fail()
	}
	if _, err = actionauth.CheckTaskAuthorityTx(ctx, tx, parent.TaskID, parent.OwnerID, nil, r.ObservedAt); err != nil {
		return fail()
	}
	return r, nil
}
