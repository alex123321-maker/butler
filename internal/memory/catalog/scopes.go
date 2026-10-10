package catalog

import (
	"context"

	"github.com/butler/butler/internal/ownership"
)

type Scope struct{ Type, ID string }

// ExtraScopes is server-derived from the actual current run/task and exact
// owner-created source grants. Scope names/citation text cannot grant access.
func (s *Store) ExtraScopes(ctx context.Context, purpose string) ([]Scope, error) {
	token, ok := ownership.TokenFromContext(ctx)
	if !ok {
		return nil, ErrDenied
	}
	tx, e := s.pool.Begin(ctx)
	if e != nil {
		return nil, e
	}
	defer tx.Rollback(ctx)
	if _, e = s.AccessTx(ctx, tx, nil, purpose); e != nil {
		return nil, e
	}
	var task, owner string
	if e = tx.QueryRow(ctx, `SELECT COALESCE(r.task_id,''),s.user_id FROM runs r JOIN sessions s USING(session_key) WHERE r.run_id=$1`, token.RunID).Scan(&task, &owner); e != nil {
		return nil, ErrDenied
	}
	result := []Scope{}
	if task == "" {
		return result, tx.Commit(ctx)
	}
	result = append(result, Scope{"task", task})
	rows, e := tx.Query(ctx, `SELECT DISTINCT s.scope_type,s.scope_id FROM memory_scope_grants g JOIN memory_sources s USING(source_id) JOIN tasks t USING(task_id) WHERE g.task_id=$1 AND g.owner_id=$2 AND s.owner_id=$2 AND g.purpose=$3 AND g.task_revision=t.revision AND g.access_revision=s.access_revision AND s.status='active' AND s.scope_type IN ('project','task','session') ORDER BY s.scope_type,s.scope_id LIMIT 9`, task, owner, purpose)
	if e != nil {
		return nil, e
	}
	count := 0
	for rows.Next() {
		var scope Scope
		if e = rows.Scan(&scope.Type, &scope.ID); e != nil {
			rows.Close()
			return nil, e
		}
		count++
		if scope.Type != "task" || scope.ID != task {
			result = append(result, scope)
		}
	}
	e = rows.Err()
	rows.Close()
	if e != nil {
		return nil, e
	}
	if count > 8 {
		return nil, ErrInvalid
	}
	return result, tx.Commit(ctx)
}
