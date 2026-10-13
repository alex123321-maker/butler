package generatedtools

import (
	"context"

	"github.com/butler/butler/internal/actionauth"
)

// CollectionAvailable describes the current parent phase without granting a
// candidate source or preparing an action. Collection Prepare/Claim still repeat
// the same full return and current-authority checks immediately before execution.
func (s *Store) CollectionAvailable(ctx context.Context, proposed actionauth.ContextSnapshot) (bool, error) {
	current, err := s.run(ctx, proposed.RunID)
	if err != nil || current.OwnerID != proposed.OwnerID || current.TaskID != proposed.TaskID || current.TaskRevision != proposed.TaskRevision || current.ConstraintsRevision != proposed.ConstraintsRevision || current.LeaseID != proposed.LeaseID || current.LeaseEpoch != proposed.LeaseEpoch {
		return false, ErrAccess
	}
	ctx = runContext(ctx, current)
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return false, err
	}
	defer tx.Rollback(ctx)
	if _, _, err = s.guardRunTx(ctx, tx, current); err != nil {
		return false, err
	}
	// Bound the metadata scan by the existing maximum child width. A tested
	// package alone does not supply the immutable child final/report projection.
	rows, err := tx.Query(ctx, `SELECT child_task_id,digest FROM converter_developer_returns WHERE owner_id=$1 AND parent_task_id=$2 ORDER BY child_task_id LIMIT 64`, current.OwnerID, current.TaskID)
	if err != nil {
		return false, err
	}
	type returned struct{ child, digest string }
	returns := []returned{}
	for rows.Next() {
		var r returned
		if err = rows.Scan(&r.child, &r.digest); err != nil {
			break
		}
		returns = append(returns, r)
	}
	if err == nil {
		err = rows.Err()
	}
	rows.Close()
	if err != nil {
		return false, err
	}
	for _, r := range returns {
		if _, _, err = s.collectionTx(ctx, tx, current, r.child, r.digest); err == nil {
			return true, tx.Commit(ctx)
		}
		if ctx.Err() != nil {
			return false, ctx.Err()
		}
	}
	return false, tx.Commit(ctx)
}
