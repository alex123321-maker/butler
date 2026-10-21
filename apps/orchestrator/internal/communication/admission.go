package communication

import (
	"context"
	"encoding/json"
	"slices"

	"github.com/butler/butler/internal/ownership"
)

// AdmitRun is a composition hook, not a model/owner API. All identity comes
// from the owned run and its actual private Input claim. No startup caller is
// installed. Recovery may reuse only the same immutable admission profile.
func (s *Store) AdmitRun(ctx context.Context, runID string, quiet bool, methods []string) error {
	t, ok := ownership.TokenFromContext(ctx)
	if !ok || t.RunID != runID {
		return ErrDenied
	}
	tx, err := s.Pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(context.Background())
	var p Principal
	err = tx.QueryRow(ctx, `SELECT s.user_id,r.session_key,r.run_id,i.input_id,COALESCE(r.task_id,''),COALESCE(r.task_revision,0),COALESCE(t.constraints_revision,0) FROM runs r JOIN sessions s USING(session_key) JOIN accepted_inputs i ON i.run_id=r.run_id LEFT JOIN tasks t ON t.task_id=r.task_id WHERE r.run_id=$1 FOR UPDATE OF s`, runID).Scan(&p.OwnerID, &p.SessionKey, &p.RunID, &p.InputID, &p.TaskID, &p.TaskRevision, &p.ConstraintsRevision)
	if err != nil {
		return ErrDenied
	}
	var exists bool
	if err = tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM communication_admissions WHERE input_id=$1)`, p.InputID).Scan(&exists); err != nil {
		return err
	}
	if exists {
		if err = s.guard(ctx, tx, p); err != nil {
			return err
		}
		var raw []byte
		var oldQuiet bool
		if err = tx.QueryRow(ctx, `SELECT methods,quiet FROM communication_admissions WHERE input_id=$1`, p.InputID).Scan(&raw, &oldQuiet); err != nil {
			return err
		}
		var old []string
		if json.Unmarshal(raw, &old) != nil {
			return ErrDenied
		}
		if p.TaskID != "" {
			var mode string
			if tx.QueryRow(ctx, `SELECT result_delivery FROM tasks WHERE task_id=$1`, p.TaskID).Scan(&mode) != nil {
				return ErrDenied
			}
			quiet = quiet || mode == "verified_task"
		}
		if oldQuiet != quiet || !slices.Equal(old, methods) {
			return ErrConflict
		}
	} else if err = s.AdmitTx(ctx, tx, p, quiet, methods); err != nil {
		return err
	}
	return tx.Commit(ctx)
}
