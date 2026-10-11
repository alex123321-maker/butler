package task

import (
	"context"
	"encoding/json"
	"time"

	"github.com/butler/butler/internal/delegation"
	"github.com/butler/butler/internal/owneridentity"
	"github.com/butler/butler/internal/ownership"
	"github.com/jackc/pgx/v5"
)

func delegationCallerTx(ctx context.Context, tx pgx.Tx, r Record) error {
	if _, ok := ownership.TokenFromContext(ctx); ok {
		return workerTx(ctx, tx, r)
	}
	if actor, ok := owneridentity.FromContext(ctx); ok && actor.OwnerID == r.OwnerID {
		return nil
	}
	return ErrUnauthorized
}

func (m *DelegationManager) queueTx(ctx context.Context, tx pgx.Tx, parent, child Record, c *delegation.Contract) error {
	if err := currentContractTx(ctx, tx, parent, child, *c); err != nil {
		return err
	}
	if err := m.guard.CheckLifecycleTx(ctx, tx, parent, child, *c); err != nil {
		return err
	}
	var incomplete, failed bool
	if err := tx.QueryRow(ctx, `SELECT COALESCE(bool_or(d.state<>'accepted'),false),COALESCE(bool_or(d.state IN ('rejected','blocked','cancelled')),false) FROM task_delegation_dependencies e JOIN task_delegations d ON d.task_id=e.depends_on_task_id WHERE e.task_id=$1`, child.TaskID).Scan(&incomplete, &failed); err != nil {
		return err
	}
	if failed {
		_, err := tx.Exec(ctx, `UPDATE task_delegations SET state='blocked',decision_evidence_json='{"blocked_reason":"dependency_unavailable"}',updated_at=clock_timestamp() WHERE task_id=$1 AND state='waiting'`, child.TaskID)
		c.State = "blocked"
		return err
	}
	if incomplete {
		return nil
	}
	var running int
	if err := tx.QueryRow(ctx, `SELECT count(*) FROM task_delegations WHERE root_task_id=$1 AND state IN ('queued','executing')`, c.RootTaskID).Scan(&running); err != nil {
		return err
	}
	if running >= m.limits.MaxParallel {
		return nil
	}
	if _, err := tx.Exec(ctx, `UPDATE task_delegations SET state='queued',updated_at=clock_timestamp() WHERE task_id=$1 AND state='waiting'`, child.TaskID); err != nil {
		return err
	}
	c.State = "queued"
	if err := m.guard.CheckLifecycleTx(ctx, tx, parent, child, *c); err != nil {
		return err
	}
	input, err := m.inputs.AcceptBoundedTx(ctx, tx, delegation.InputRequest{OwnerID: child.OwnerID, SessionKey: child.SessionKey, TaskID: child.TaskID, EventID: "delegation:" + child.TaskID, ContractSource: c.ContractSource, Deadline: c.Deadline, MaxAttempts: 3})
	if err != nil {
		return err
	}
	c.InputID = input
	if _, err = tx.Exec(ctx, `UPDATE task_delegations SET input_id=$2 WHERE task_id=$1 AND state='queued'`, child.TaskID, input); err != nil {
		return err
	}
	if err = m.guard.CheckLifecycleTx(ctx, tx, parent, child, *c); err != nil {
		return err
	}
	return currentContractTx(ctx, tx, parent, child, *c)
}

// Advance is bounded kernel maintenance, not a new model action. Accepted
// children survive a dead parent process; current task/source/owner authority
// remains mandatory and can deny all new work before any parent restarts.
func (m *DelegationManager) Advance(ctx context.Context, parentID string) ([]DelegationRecord, error) {
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	tx, _, parent, err := m.begin(ctx, parentID)
	if err != nil {
		return nil, err
	}
	defer delegationRollback(tx)
	rows, err := tx.Query(ctx, `SELECT task_id FROM task_delegations WHERE parent_task_id=$1 AND state='waiting' ORDER BY created_at,task_id FOR UPDATE`, parentID)
	if err != nil {
		return nil, err
	}
	var ids []string
	for rows.Next() {
		var id string
		if err = rows.Scan(&id); err != nil {
			break
		}
		ids = append(ids, id)
	}
	if err == nil {
		err = rows.Err()
	}
	rows.Close()
	if err != nil {
		return nil, err
	}
	values := make([]DelegationRecord, 0, len(ids))
	for _, id := range ids {
		v, e := m.loadTx(ctx, tx, id)
		if e != nil {
			return nil, e
		}
		child, e := m.store.GetTx(ctx, tx, id)
		if e != nil {
			return nil, e
		}
		if err = m.queueTx(ctx, tx, parent, child, &v.Contract); err != nil {
			return nil, err
		}
		if v.Contract.State == "blocked" {
			v.BlockedReason = "dependency_unavailable"
		}
		values = append(values, v)
	}
	return values, tx.Commit(ctx)
}

func (m *DelegationManager) MarkExecuting(ctx context.Context, childID string) (DelegationRecord, error) {
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	initial, err := m.store.Get(ctx, childID)
	if err != nil {
		return DelegationRecord{}, err
	}
	tx, _, parent, err := m.begin(ctx, initial.ParentTaskID)
	if err != nil {
		return DelegationRecord{}, err
	}
	defer delegationRollback(tx)
	child, err := m.store.GetTx(ctx, tx, childID)
	if err != nil {
		return DelegationRecord{}, err
	}
	if err = workerTx(ctx, tx, child); err != nil {
		return DelegationRecord{}, err
	}
	v, err := m.loadTx(ctx, tx, childID)
	if err != nil {
		return v, err
	}
	if v.Contract.State != "queued" && v.Contract.State != "executing" {
		return v, ErrConflict
	}
	if err = currentContractTx(ctx, tx, parent, child, v.Contract); err != nil {
		return v, err
	}
	if err = m.guard.CheckLifecycleTx(ctx, tx, parent, child, v.Contract); err != nil {
		return v, err
	}
	// Source/authority locks may wait beyond the original lease or deadline.
	if err = workerTx(ctx, tx, child); err != nil {
		return v, err
	}
	if err = currentContractTx(ctx, tx, parent, child, v.Contract); err != nil {
		return v, err
	}
	if _, err = tx.Exec(ctx, `UPDATE task_delegations SET state='executing',updated_at=clock_timestamp() WHERE task_id=$1`, childID); err != nil {
		return v, err
	}
	v.Contract.State = "executing"
	return v, tx.Commit(ctx)
}

// Collect binds an already durable run result to a versioned structured report.
// It neither invents a result nor trusts the report as objective verification.
func (m *DelegationManager) Collect(ctx context.Context, parentID, childID, resultID string, report delegation.SourceRef) (DelegationRecord, error) {
	if !delegation.ValidSource(report) {
		return DelegationRecord{}, ErrInvalid
	}
	return m.collect(ctx, parentID, childID, resultID, report, false)
}

// CollectGenerated publishes a strict kernel projection of the actual final and
// collects it atomically. Maintenance needs current task/source authority, not
// the completed child's or a disappeared parent's worker lease.
func (m *DelegationManager) CollectGenerated(ctx context.Context, parentID, childID, resultID string) (DelegationRecord, error) {
	return m.collect(ctx, parentID, childID, resultID, delegation.SourceRef{}, true)
}

func (m *DelegationManager) collect(ctx context.Context, parentID, childID, resultID string, report delegation.SourceRef, generated bool) (DelegationRecord, error) {
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	tx, _, parent, err := m.begin(ctx, parentID)
	if err != nil {
		return DelegationRecord{}, err
	}
	defer delegationRollback(tx)
	child, err := m.store.GetTx(ctx, tx, childID)
	if err != nil {
		return DelegationRecord{}, err
	}
	v, err := m.loadTx(ctx, tx, childID)
	if err != nil {
		return v, err
	}
	if err = currentContractTx(ctx, tx, parent, child, v.Contract); err != nil {
		return v, err
	}
	if v.Contract.State != "queued" && v.Contract.State != "executing" && v.Contract.State != "result_ready" {
		return v, ErrConflict
	}
	if generated && v.Report != nil {
		report = *v.Report
	}
	if v.ResultID != "" && v.ResultID != resultID {
		return v, ErrConflict
	}
	r, err := m.resultTx(ctx, tx, child, resultID)
	if err != nil {
		return v, err
	}
	if generated && v.Report == nil {
		creator, ok := m.guard.(DelegationReportCreator)
		if !ok {
			return v, delegation.ErrDenied
		}
		report, err = creator.CreateReportTx(ctx, tx, parent, child, r)
		if err != nil || !delegation.ValidSource(report) {
			if err == nil {
				err = delegation.ErrDenied
			}
			return v, err
		}
	}
	if v.ResultID != "" && (v.ResultID != resultID || v.Report == nil || *v.Report != report) {
		return v, ErrConflict
	}
	if err = m.guard.CheckLifecycleTx(ctx, tx, parent, child, v.Contract); err != nil {
		return v, err
	}
	if err = m.guard.CheckReportTx(ctx, tx, parent, child, r, report); err != nil {
		return v, err
	}
	if err = currentContractTx(ctx, tx, parent, child, v.Contract); err != nil {
		return v, err
	}
	evidence, _ := json.Marshal(delegationEvidence{Report: &report})
	if _, err = tx.Exec(ctx, `UPDATE task_delegations SET state='result_ready',result_id=$2,decision_evidence_json=$3::jsonb,updated_at=clock_timestamp() WHERE task_id=$1`, childID, resultID, evidence); err != nil {
		return v, err
	}
	v.Contract.State, v.ResultID, v.Report = "result_ready", resultID, &report
	return v, tx.Commit(ctx)
}

func (m *DelegationManager) resultTx(ctx context.Context, tx pgx.Tx, child Record, id string) (Result, error) {
	r, err := scanResult(tx.QueryRow(ctx, `SELECT `+resultColumns+` FROM task_results WHERE task_id=$1 AND result_id=$2 FOR SHARE`, child.TaskID, id))
	if err != nil {
		return r, err
	}
	if r.TaskRevision != child.Revision || r.RunID == "" {
		return r, ErrConflict
	}
	var valid bool
	// Completed run identity is immutable. Current runs were locked before task
	// rows in begin; never acquire a new historical run lock after a task lock.
	err = tx.QueryRow(ctx, `SELECT task_id=$2 AND task_revision=$3 AND session_key=$4 AND current_state='completed' FROM runs WHERE run_id=$1`, r.RunID, child.TaskID, child.Revision, child.SessionKey).Scan(&valid)
	if err != nil || !valid {
		return r, ErrConflict
	}
	// A historical final cannot free this executor's slot while another actual
	// attempt of the same task revision is still live. Session admission is
	// already locked by begin, so no new child attempt can race this check.
	err = tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM runs WHERE task_id=$1 AND task_revision=$2 AND current_state NOT IN ('completed','failed','cancelled','timed_out'))`, child.TaskID, child.Revision).Scan(&valid)
	if err != nil || valid {
		return r, ErrConflict
	}
	return r, nil
}

// A kernel/owner verdict may unlock a dependency but never marks task complete.
// Disagreement remains two durable reports; accepting a claim requires the
// independent guard's objective evidence, not a vote by agreeing models.
func (m *DelegationManager) Decide(ctx context.Context, parentID, childID string, decision DelegationDecision) (DelegationRecord, error) {
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	raw, err := json.Marshal(decision)
	if err != nil {
		return DelegationRecord{}, ErrInvalid
	}
	if err = json.Unmarshal(raw, &decision); err != nil {
		return DelegationRecord{}, ErrInvalid
	}
	if !delegation.ValidSource(decision.Report) || len(decision.Evidence) < 1 || len(decision.Evidence) > 32 {
		return DelegationRecord{}, ErrInvalid
	}
	for _, s := range decision.Evidence {
		if !delegation.ValidSource(s) {
			return DelegationRecord{}, ErrInvalid
		}
	}
	tx, _, parent, err := m.begin(ctx, parentID)
	if err != nil {
		return DelegationRecord{}, err
	}
	defer delegationRollback(tx)
	if err = delegationCallerTx(ctx, tx, parent); err != nil {
		return DelegationRecord{}, err
	}
	child, err := m.store.GetTx(ctx, tx, childID)
	if err != nil {
		return DelegationRecord{}, err
	}
	v, err := m.loadTx(ctx, tx, childID)
	if err != nil {
		return v, err
	}
	if v.Report == nil || *v.Report != decision.Report {
		return v, ErrConflict
	}
	if v.Decision != nil {
		a, _ := json.Marshal(v.Decision)
		b, _ := json.Marshal(decision)
		if string(a) != string(b) {
			return v, ErrConflict
		}
		return v, tx.Commit(ctx)
	}
	if v.Contract.State != "result_ready" {
		return v, ErrConflict
	}
	if err = currentContractTx(ctx, tx, parent, child, v.Contract); err != nil {
		return v, err
	}
	r, err := m.resultTx(ctx, tx, child, v.ResultID)
	if err != nil {
		return v, err
	}
	if err = m.guard.CheckLifecycleTx(ctx, tx, parent, child, v.Contract); err != nil {
		return v, err
	}
	if err = m.guard.CheckDecisionTx(ctx, tx, parent, child, r, decision); err != nil {
		return v, err
	}
	if err = delegationCallerTx(ctx, tx, parent); err != nil {
		return v, err
	}
	if err = currentContractTx(ctx, tx, parent, child, v.Contract); err != nil {
		return v, err
	}
	state := "rejected"
	if decision.Accept {
		state = "accepted"
	}
	evidence, _ := json.Marshal(delegationEvidence{Report: v.Report, Decision: &decision})
	if _, err = tx.Exec(ctx, `UPDATE task_delegations SET state=$2,decision_evidence_json=$3::jsonb,updated_at=clock_timestamp() WHERE task_id=$1`, childID, state, evidence); err != nil {
		return v, err
	}
	v.Contract.State, v.Decision = state, &decision
	return v, tx.Commit(ctx)
}

func (m *DelegationManager) SetDependencies(ctx context.Context, parentID, childID string, deps []string) (DelegationRecord, error) {
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	seen := map[string]bool{}
	if len(deps) > m.limits.MaxDependencies {
		return DelegationRecord{}, ErrInvalid
	}
	for _, id := range deps {
		if id == "" || seen[id] {
			return DelegationRecord{}, ErrInvalid
		}
		seen[id] = true
	}
	deps = append([]string(nil), deps...)
	tx, root, parent, err := m.begin(ctx, parentID)
	if err != nil {
		return DelegationRecord{}, err
	}
	defer delegationRollback(tx)
	if err = delegationCallerTx(ctx, tx, parent); err != nil {
		return DelegationRecord{}, err
	}
	child, err := m.store.GetTx(ctx, tx, childID)
	if err != nil {
		return DelegationRecord{}, err
	}
	v, err := m.loadTx(ctx, tx, childID)
	if err != nil {
		return v, err
	}
	if v.Contract.State != "waiting" {
		return v, ErrConflict
	}
	if err = currentContractTx(ctx, tx, parent, child, v.Contract); err != nil {
		return v, err
	}
	if err = m.guard.CheckLifecycleTx(ctx, tx, parent, child, v.Contract); err != nil {
		return v, err
	}
	if err = m.dependenciesTx(ctx, tx, root, parentID, childID, deps); err != nil {
		return v, err
	}
	if err = delegationCallerTx(ctx, tx, parent); err != nil {
		return v, err
	}
	if err = currentContractTx(ctx, tx, parent, child, v.Contract); err != nil {
		return v, err
	}
	return v, tx.Commit(ctx)
}
