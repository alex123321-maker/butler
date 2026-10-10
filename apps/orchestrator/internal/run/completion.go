package run

import (
	"context"
	"fmt"
	"github.com/butler/butler/internal/ownership"
	"github.com/jackc/pgx/v5"
	"time"
)

type CompletionState struct {
	TaskID       string
	TaskRevision int64
	State        string
}

func (r *PostgresRepository) LockCompletionTx(ctx context.Context, tx pgx.Tx, runID, sessionKey string) (CompletionState, error) {
	token, ok := ownership.TokenFromContext(ctx)
	if !ok || token.RunID != runID || token.SessionKey != sessionKey {
		return CompletionState{}, ownership.ErrStale
	}
	var s CompletionState
	var key, lease string
	err := tx.QueryRow(ctx, `SELECT session_key,COALESCE(lease_id,''),current_state,COALESCE(task_id,''),COALESCE(task_revision,0) FROM runs WHERE run_id=$1 FOR UPDATE`, runID).Scan(&key, &lease, &s.State, &s.TaskID, &s.TaskRevision)
	if err != nil {
		return s, err
	}
	if key != sessionKey || lease != token.LeaseID || s.State != "finalizing" {
		return s, fmt.Errorf("%w: run is not owned finalizing execution", ownership.ErrStale)
	}
	return s, nil
}

func (r *PostgresRepository) CompleteTx(ctx context.Context, tx pgx.Tx, runID string, at time.Time) error {
	token, ok := ownership.TokenFromContext(ctx)
	if !ok || token.RunID != runID {
		return ownership.ErrStale
	}
	tag, err := tx.Exec(ctx, `UPDATE runs SET status='completed',current_state='completed',finished_at=$2,updated_at=$2 WHERE run_id=$1 AND current_state='finalizing' AND session_key=$3 AND lease_id=$4`, runID, at, token.SessionKey, token.LeaseID)
	if err != nil {
		return err
	}
	if tag.RowsAffected() != 1 {
		return fmt.Errorf("completion state changed")
	}
	_, err = tx.Exec(ctx, `INSERT INTO run_state_transitions(run_id,from_state,to_state,triggered_by,metadata_json,transitioned_at) VALUES($1,'finalizing','completed','atomic_result','{}',$2)`, runID, at)
	return err
}
