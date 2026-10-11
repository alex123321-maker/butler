package task

import (
	"context"
	"encoding/json"
	"errors"
	"time"

	"github.com/butler/butler/internal/actionauth"
	"github.com/butler/butler/internal/budget"
	"github.com/butler/butler/internal/owneridentity"
	"github.com/jackc/pgx/v5"
)

// OwnerOverview projects owning records without granting authority or treating
// a model answer as verified task completion. It contains no source/args bodies.
type OwnerOverview struct {
	TaskID            string                 `json:"task_id"`
	Revision          int64                  `json:"revision"`
	ObservedAt        time.Time              `json:"observed_at"`
	Assignment        *actionauth.Assignment `json:"assignment,omitempty"`
	Initiative        *InitiativeOverview    `json:"initiative,omitempty"`
	Children          []ChildOverview        `json:"children"`
	ChildrenTruncated bool                   `json:"children_truncated"`
	Actions           []ActionOverview       `json:"actions"`
	ActionsTruncated  bool                   `json:"actions_truncated"`
	Budget            BudgetOverview         `json:"budget"`
}

type ChildOverview struct {
	TaskID             string `json:"task_id"`
	Revision           int64  `json:"revision"`
	Status             string `json:"status"`
	DelegationState    string `json:"delegation_state"`
	CompletionResultID string `json:"completion_result_id,omitempty"`
}

type InitiativeOverview struct {
	ID                 string `json:"initiative_id"`
	SourceID           string `json:"source_id"`
	ObservationRef     string `json:"observation_ref"`
	Status             string `json:"status"`
	Reason             string `json:"reason"`
	Confidence         int    `json:"confidence"`
	Importance         int    `json:"importance"`
	ExpectedModelCalls int    `json:"expected_model_calls"`
}

type ActionOverview struct {
	ID           string `json:"action_id"`
	Tool         string `json:"tool_name"`
	TaskRevision int64  `json:"task_revision"`
	Effect       string `json:"effect"`
	EffectState  string `json:"effect_state"`
	ContractHash string `json:"contract_hash"`
}

type BudgetOverview struct {
	RootTaskID            string        `json:"root_task_id"`
	Limits                *budget.Units `json:"limits,omitempty"`
	Reserved              *budget.Units `json:"reserved,omitempty"`
	Deadline              *time.Time    `json:"deadline,omitempty"`
	ModelAttempts         int64         `json:"model_attempts"`
	KnownInputTokens      int64         `json:"known_input_tokens"`
	KnownOutputTokens     int64         `json:"known_output_tokens"`
	UnknownInputAttempts  int64         `json:"unknown_input_attempts"`
	UnknownOutputAttempts int64         `json:"unknown_output_attempts"`
}

func (s *Store) OwnerOverview(ctx context.Context, id string) (OwnerOverview, error) {
	actor, ok := owneridentity.FromContext(ctx)
	if !ok {
		return OwnerOverview{}, ErrUnauthorized
	}
	ctx, cancel := context.WithTimeout(ctx, 2*time.Second)
	defer cancel()
	tx, err := s.pool.BeginTx(ctx, pgx.TxOptions{IsoLevel: pgx.RepeatableRead, AccessMode: pgx.ReadOnly})
	if err != nil {
		return OwnerOverview{}, err
	}
	defer tx.Rollback(ctx)
	var owner string
	v := OwnerOverview{TaskID: id, Children: []ChildOverview{}, Actions: []ActionOverview{}}
	err = tx.QueryRow(ctx, `SELECT owner_id,revision,clock_timestamp() FROM tasks WHERE task_id=$1`, id).Scan(&owner, &v.Revision, &v.ObservedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return OwnerOverview{}, ErrNotFound
	}
	if err != nil {
		return OwnerOverview{}, err
	}
	if owner != actor.OwnerID {
		return OwnerOverview{}, ErrUnauthorized
	}
	a, err := actionauth.GetTaskAssignment(ctx, tx, id)
	if err != nil {
		return OwnerOverview{}, err
	}
	if a.TaskID != "" {
		if a.OwnerID != owner {
			return OwnerOverview{}, ErrUnauthorized
		}
		v.Assignment = &a
	}
	i := &InitiativeOverview{}
	err = tx.QueryRow(ctx, `SELECT initiative_id,source_id,observation_ref,status,reason,confidence,importance,expected_model_calls FROM initiatives WHERE task_id=$1 AND owner_id=$2`, id, owner).Scan(&i.ID, &i.SourceID, &i.ObservationRef, &i.Status, &i.Reason, &i.Confidence, &i.Importance, &i.ExpectedModelCalls)
	if err == nil {
		v.Initiative = i
	} else if !errors.Is(err, pgx.ErrNoRows) {
		return OwnerOverview{}, err
	}
	rows, err := tx.Query(ctx, `SELECT t.task_id,t.revision,t.status,COALESCE(d.state,''),COALESCE(t.completion_result_id,'') FROM tasks t LEFT JOIN task_delegations d ON d.task_id=t.task_id WHERE t.parent_task_id=$1 AND t.owner_id=$2 ORDER BY t.created_at,t.task_id LIMIT 101`, id, owner)
	if err != nil {
		return OwnerOverview{}, err
	}
	for rows.Next() {
		var child ChildOverview
		if err = rows.Scan(&child.TaskID, &child.Revision, &child.Status, &child.DelegationState, &child.CompletionResultID); err != nil {
			rows.Close()
			return OwnerOverview{}, err
		}
		v.Children = append(v.Children, child)
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		return OwnerOverview{}, err
	}
	if len(v.Children) > 100 {
		v.ChildrenTruncated = true
		v.Children = v.Children[:100]
	}
	rows, err = tx.Query(ctx, `SELECT i.action_id,i.binding->>'tool_name',COALESCE((i.binding->>'task_revision')::bigint,0),CASE WHEN i.binding->>'effect' IN ('read','prepare','commit') THEN i.binding->>'effect' ELSE 'unknown' END,COALESCE(e.state,'unavailable'),i.binding->>'contract_hash' FROM action_intents i LEFT JOIN action_effects e ON e.action_id=i.effect_id WHERE i.binding->>'task_id'=$1 AND i.binding->>'owner_id'=$2 ORDER BY i.created_at DESC,i.action_id LIMIT 51`, id, owner)
	if err != nil {
		return OwnerOverview{}, err
	}
	for rows.Next() {
		var action ActionOverview
		if err = rows.Scan(&action.ID, &action.Tool, &action.TaskRevision, &action.Effect, &action.EffectState, &action.ContractHash); err != nil {
			rows.Close()
			return OwnerOverview{}, err
		}
		v.Actions = append(v.Actions, action)
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		return OwnerOverview{}, err
	}
	if len(v.Actions) > 50 {
		v.ActionsTruncated = true
		v.Actions = v.Actions[:50]
	}
	var rootOwner string
	err = tx.QueryRow(ctx, `WITH RECURSIVE parents AS (SELECT task_id,parent_task_id FROM tasks WHERE task_id=$1 UNION SELECT t.task_id,t.parent_task_id FROM tasks t JOIN parents p ON t.task_id=p.parent_task_id) SELECT task_id,owner_id FROM tasks WHERE task_id=(SELECT task_id FROM parents WHERE parent_task_id IS NULL)`, id).Scan(&v.Budget.RootTaskID, &rootOwner)
	if err != nil {
		return OwnerOverview{}, err
	}
	if rootOwner != owner {
		return OwnerOverview{}, ErrUnauthorized
	}
	var limits, reserved []byte
	var deadline time.Time
	err = tx.QueryRow(ctx, `SELECT limits_json,used_json,deadline FROM execution_budget_scopes WHERE kind='task' AND owner_id=$1 AND subject_id=$2`, owner, v.Budget.RootTaskID).Scan(&limits, &reserved, &deadline)
	if err == nil {
		v.Budget.Limits = &budget.Units{}
		v.Budget.Reserved = &budget.Units{}
		v.Budget.Deadline = &deadline
		if json.Unmarshal(limits, v.Budget.Limits) != nil || json.Unmarshal(reserved, v.Budget.Reserved) != nil {
			return OwnerOverview{}, ErrInvalid
		}
	} else if !errors.Is(err, pgx.ErrNoRows) {
		return OwnerOverview{}, err
	}
	// Actual usage stays separate from reservations. Missing values remain
	// explicit; known subtotals cannot imply a complete token or invoice total.
	err = tx.QueryRow(ctx, `WITH RECURSIVE tree AS (SELECT task_id FROM tasks WHERE task_id=$1 AND owner_id=$2 UNION SELECT t.task_id FROM tasks t JOIN tree p ON t.parent_task_id=p.task_id WHERE t.owner_id=$2) SELECT count(*),COALESCE(sum(a.actual_input_tokens),0),COALESCE(sum(a.actual_output_tokens),0),count(*) FILTER(WHERE a.actual_input_tokens IS NULL),count(*) FILTER(WHERE a.actual_output_tokens IS NULL) FROM execution_model_attempts a JOIN runs r ON r.run_id=a.run_id JOIN tree ON tree.task_id=r.task_id`, v.Budget.RootTaskID, owner).Scan(&v.Budget.ModelAttempts, &v.Budget.KnownInputTokens, &v.Budget.KnownOutputTokens, &v.Budget.UnknownInputAttempts, &v.Budget.UnknownOutputAttempts)
	if err != nil {
		return OwnerOverview{}, err
	}
	if err = tx.Commit(ctx); err != nil {
		return OwnerOverview{}, err
	}
	return v, nil
}
