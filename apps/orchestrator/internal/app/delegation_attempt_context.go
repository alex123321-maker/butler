package app

import (
	"context"
	"time"

	flow "github.com/butler/butler/apps/orchestrator/internal/orchestrator"
	"github.com/butler/butler/apps/orchestrator/internal/task"
	"github.com/butler/butler/internal/delegation"
	"github.com/butler/butler/internal/owneridentity"
	"github.com/butler/butler/internal/ownership"
	"github.com/jackc/pgx/v5/pgxpool"
)

type failedDelegationAttemptReader struct {
	pool    *pgxpool.Pool
	manager *task.DelegationManager
}

var _ flow.FailedDelegationAttemptReader = failedDelegationAttemptReader{}

func (a failedDelegationAttemptReader) LoadFailedDelegationAttempts(ctx context.Context, runID string) ([]delegation.AttemptReport, error) {
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	token, ok := ownership.TokenFromContext(ctx)
	if !ok || token.RunID != runID || a.pool == nil || a.manager == nil {
		return nil, delegation.ErrDenied
	}
	if _, actor := owneridentity.FromContext(ctx); actor {
		return nil, delegation.ErrDenied
	}
	tx, err := a.pool.Begin(ctx)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback(ctx)
	var valid bool
	if err = tx.QueryRow(ctx, `SELECT TRUE FROM session_ownership WHERE session_key=$1 AND run_id=$2 AND lease_id=$3 AND fence_epoch=$4 AND expires_at>clock_timestamp() FOR SHARE`, token.SessionKey, token.RunID, token.LeaseID, token.FenceEpoch).Scan(&valid); err != nil {
		return nil, delegation.ErrDenied
	}
	var parentID string
	var revision int64
	if err = tx.QueryRow(ctx, `SELECT COALESCE(task_id,''),COALESCE(task_revision,0) FROM runs WHERE run_id=$1 AND session_key=$2 AND current_state NOT IN ('completed','failed','cancelled','timed_out','finalizing') FOR SHARE`, runID, token.SessionKey).Scan(&parentID, &revision); err != nil {
		return nil, delegation.ErrDenied
	}
	if parentID == "" {
		return nil, tx.Commit(ctx)
	}
	var constraints int64
	if err = tx.QueryRow(ctx, `SELECT t.constraints_revision FROM tasks t JOIN sessions s USING(session_key) WHERE t.task_id=$1 AND t.session_key=$2 AND t.revision=$3 AND t.owner_id=s.user_id AND t.status='active' FOR SHARE OF t`, parentID, token.SessionKey, revision).Scan(&constraints); err != nil {
		return nil, delegation.ErrDenied
	}
	rows, err := tx.Query(ctx, `SELECT c.task_id FROM task_delegations d JOIN tasks c ON c.task_id=d.task_id JOIN tasks p ON p.task_id=d.parent_task_id WHERE p.task_id=$1 AND d.parent_revision=$2 AND d.parent_constraints_revision=$3 AND c.parent_task_id=p.task_id AND c.owner_id=p.owner_id AND c.status='blocked' AND EXISTS(SELECT 1 FROM runs r WHERE r.task_id=c.task_id AND r.task_revision=c.revision AND r.session_key=c.session_key AND r.current_state IN ('failed','cancelled','timed_out')) ORDER BY c.task_id LIMIT 9`, parentID, revision, constraints)
	if err != nil {
		return nil, err
	}
	var children []string
	for rows.Next() {
		var id string
		if err = rows.Scan(&id); err != nil {
			break
		}
		children = append(children, id)
	}
	if err == nil {
		err = rows.Err()
	}
	rows.Close()
	if err != nil || len(children) > 8 {
		return nil, delegation.ErrDenied
	}
	// Release selection locks/connection before the manager takes its tree-first
	// lock order. Each observation independently rechecks the current parent.
	if err = tx.Commit(ctx); err != nil {
		return nil, err
	}
	reports := make([]delegation.AttemptReport, 0, len(children))
	for _, childID := range children {
		r, err := a.manager.ObserveFailedAttempts(ctx, parentID, childID)
		if err != nil {
			return nil, err
		}
		reports = append(reports, r)
	}
	return reports, nil
}
