package task

import (
	"context"
	"encoding/json"
	"time"

	"github.com/butler/butler/internal/budget"
	"github.com/butler/butler/internal/delegation"
	"github.com/jackc/pgx/v5"
)

func limitsOf(u budget.Units) delegation.Limits {
	return delegation.Limits{ModelTurns: u.ModelTurns, Tokens: u.Tokens, CostMicros: u.CostMicros, ExternalCalls: u.ExternalCalls, Children: u.Children, ActiveMillis: u.ActiveMillis, EgressBytes: u.EgressBytes, ProviderInputBytes: u.ProviderInputBytes}
}
func budgetWithin(a, b delegation.Limits) bool {
	av := []int64{a.ModelTurns, a.Tokens, a.CostMicros, a.ExternalCalls, a.Children, a.ActiveMillis, a.EgressBytes, a.ProviderInputBytes}
	bv := []int64{b.ModelTurns, b.Tokens, b.CostMicros, b.ExternalCalls, b.Children, b.ActiveMillis, b.EgressBytes, b.ProviderInputBytes}
	for i, v := range av {
		if v < 0 || v > bv[i] {
			return false
		}
	}
	return true
}
func narrower(r DelegationRequest, p delegation.Contract) bool {
	if r.Deadline.After(p.Deadline) || !budgetWithin(r.Budget, p.Budget) {
		return false
	}
	for _, s := range r.Sources {
		found := s == p.ContractSource
		for _, allowed := range p.Sources {
			if s == allowed {
				found = true
			}
		}
		if !found {
			return false
		}
	}
	for _, permission := range r.Permissions {
		for _, resource := range permission.Resources {
			for _, effect := range permission.Effects {
				if !delegation.Allows(&p, permission.Tool, permission.ContractHash, resource, effect, permission.Credentials) {
					return false
				}
			}
		}
	}
	return true
}

// Dependencies are manager-owned metadata and may refer only to siblings in the
// same bounded tree. A waiting node may replace its edges, never its contract.
func (m *DelegationManager) dependenciesTx(ctx context.Context, tx pgx.Tx, root, parent, id string, deps []string) error {
	for _, other := range deps {
		if other == id {
			return ErrConflict
		}
		var valid bool
		if err := tx.QueryRow(ctx, `SELECT root_task_id=$2 AND parent_task_id=$3 FROM task_delegations WHERE task_id=$1 FOR SHARE`, other, root, parent).Scan(&valid); err != nil || !valid {
			return ErrConflict
		}
		if id != "" {
			var cycle bool
			if err := tx.QueryRow(ctx, `WITH RECURSIVE edges AS(SELECT depends_on_task_id FROM task_delegation_dependencies WHERE task_id=$1 UNION SELECT e.depends_on_task_id FROM task_delegation_dependencies e JOIN edges p ON e.task_id=p.depends_on_task_id) SELECT EXISTS(SELECT 1 FROM edges WHERE depends_on_task_id=$2)`, other, id).Scan(&cycle); err != nil {
				return err
			} else if cycle {
				return ErrConflict
			}
		}
	}
	if id == "" {
		return nil
	}
	if _, err := tx.Exec(ctx, `DELETE FROM task_delegation_dependencies WHERE task_id=$1`, id); err != nil {
		return err
	}
	for _, other := range deps {
		if _, err := tx.Exec(ctx, `INSERT INTO task_delegation_dependencies(task_id,depends_on_task_id) VALUES($1,$2)`, id, other); err != nil {
			return err
		}
	}
	return nil
}

type delegationEvidence struct {
	Report   *delegation.SourceRef `json:"report_source,omitempty"`
	Decision *DelegationDecision   `json:"decision,omitempty"`
	Blocked  string                `json:"blocked_reason,omitempty"`
}

func (m *DelegationManager) loadTx(ctx context.Context, tx pgx.Tx, id string) (DelegationRecord, error) {
	c, err := delegation.LoadTx(ctx, tx, id)
	if err != nil {
		return DelegationRecord{}, err
	}
	if c == nil {
		return DelegationRecord{}, ErrNotFound
	}
	value := DelegationRecord{Contract: *c}
	var evidence []byte
	if err = tx.QueryRow(ctx, `SELECT result_id,decision_evidence_json FROM task_delegations WHERE task_id=$1`, id).Scan(&value.ResultID, &evidence); err != nil {
		return value, err
	}
	if string(evidence) != "[]" {
		var e delegationEvidence
		if err = json.Unmarshal(evidence, &e); err != nil {
			return value, err
		}
		value.Report, value.Decision = e.Report, e.Decision
		value.BlockedReason = e.Blocked
	}
	return value, nil
}

// Get exposes metadata, not source/report bodies or current permission.
// User-facing callers authenticate the owner; lifecycle rechecks authority.
func (m *DelegationManager) Get(ctx context.Context, id string) (DelegationRecord, error) {
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	tx, err := m.store.pool.Begin(ctx)
	if err != nil {
		return DelegationRecord{}, err
	}
	defer delegationRollback(tx)
	v, err := m.loadTx(ctx, tx, id)
	if err != nil {
		return v, err
	}
	return v, tx.Commit(ctx)
}

func currentContractTx(ctx context.Context, tx pgx.Tx, parent, child Record, c delegation.Contract) error {
	var valid bool
	if c.ParentTaskID != parent.TaskID || c.TaskID != child.TaskID || parent.OwnerID != child.OwnerID || parent.Status != "active" || child.Status != "active" || parent.Revision != c.ParentRevision || parent.ConstraintsRevision != c.ConstraintsRevision {
		return ErrConflict
	}
	if err := tx.QueryRow(ctx, `SELECT $1>clock_timestamp()`, c.Deadline).Scan(&valid); err != nil {
		return err
	}
	if !valid {
		return delegation.ErrDenied
	}
	return nil
}
