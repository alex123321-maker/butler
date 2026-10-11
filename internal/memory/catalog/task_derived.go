package catalog

import (
	"context"
	"encoding/json"

	"github.com/butler/butler/internal/owneridentity"
	"github.com/butler/butler/internal/ownership"
	"github.com/jackc/pgx/v5"
)

// CreateTaskDerivedTx publishes fresh DATA in the current worker's own task.
// The scope is derived from its live run, never a caller-supplied task/scope or
// an owner/job shortcut. Complete current parent edges remain mandatory. This
// does not grant the parent task access to the child body; report acceptance
// must separately establish that exact current source permission.
func (s *Store) CreateTaskDerivedTx(ctx context.Context, tx pgx.Tx, parents []SourceRef, purpose, title, body string, locator json.RawMessage) (Source, error) {
	token, owned := ownership.TokenFromContext(ctx)
	_, owner := owneridentity.FromContext(ctx)
	_, auxiliary := ctx.Value(checkerKey{}).(AccessChecker)
	if tx == nil || !owned || owner || auxiliary || token.RunID == "" || token.SessionKey == "" {
		return Source{}, ErrDenied
	}
	// Keep a failed publication atomic even when its caller records an error
	// and commits the outer transaction. No additional pooled connection is used.
	publication, err := tx.Begin(ctx)
	if err != nil {
		return Source{}, err
	}
	defer publication.Rollback(ctx)
	if _, err = s.AccessTx(ctx, publication, parents, purpose); err != nil {
		return Source{}, err
	}
	var taskID, own string
	// AccessTx already holds the live ownership/run/task rows. This read takes
	// no new earlier locks after parent-source locks and checks their exact pin.
	err = publication.QueryRow(ctx, `SELECT r.task_id,s.user_id FROM runs r JOIN sessions s USING(session_key) JOIN tasks t ON t.task_id=r.task_id WHERE r.run_id=$1 AND r.session_key=$2 AND t.owner_id=s.user_id AND t.session_key=r.session_key AND t.revision=r.task_revision AND t.status='active'`, token.RunID, token.SessionKey).Scan(&taskID, &own)
	if err != nil || taskID == "" || own == "" {
		return Source{}, ErrDenied
	}
	source, err := s.CreateDerivedTx(ctx, publication, parents, purpose, title, body, locator)
	if err != nil {
		return Source{}, err
	}
	if source.OwnerID != own {
		return Source{}, ErrDenied
	}
	// The row is fresh and invisible outside this transaction. Retarget only it;
	// an existing parent/source scope can never be changed through this method.
	if _, err = publication.Exec(ctx, `UPDATE memory_sources SET scope_type='task',scope_id=$2 WHERE source_id=$1 AND owner_id=$3`, source.ID, taskID, own); err != nil {
		return Source{}, err
	}
	current, err := s.AccessTx(ctx, publication, []SourceRef{refForTaskDerived(source)}, purpose)
	if err != nil || len(current) != 1 || current[0].ScopeType != "task" || current[0].ScopeID != taskID {
		return Source{}, ErrDenied
	}
	if err = publication.Commit(ctx); err != nil {
		return Source{}, err
	}
	return current[0], nil
}

func refForTaskDerived(s Source) SourceRef {
	return SourceRef{ID: s.ID, Revision: s.Revision, AccessRevision: s.AccessRevision, Hash: s.Hash}
}
