package singletab

import (
	"context"
	"encoding/json"
	"time"

	durableinput "github.com/butler/butler/apps/orchestrator/internal/input"
	"github.com/butler/butler/internal/ownership"
	"github.com/butler/butler/internal/resourceexec"
	"github.com/butler/butler/internal/resources"
	"github.com/butler/butler/internal/singletabcontrol"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// SelectionCheckpoints owns input/run/task lifecycle; the selection library
// owns choices. No model-supplied binding, native tab reference or secret is kept
// in the continuation, and no browser operation is performed by this adapter.
type SelectionCheckpoints struct {
	pool    *pgxpool.Pool
	inputs  *durableinput.Store
	sources resourceexec.SourceGuard
	host    string
}

func NewSelectionCheckpoints(pool *pgxpool.Pool, inputs *durableinput.Store, sources resourceexec.SourceGuard, host string) (*SelectionCheckpoints, error) {
	if pool == nil || inputs == nil || sources == nil || host == "" {
		return nil, singletabcontrol.ErrDenied
	}
	return &SelectionCheckpoints{pool, inputs, sources, host}, nil
}

func (s *SelectionCheckpoints) SuspendSelectionTx(ctx context.Context, tx pgx.Tx, b resources.Binding, selection singletabcontrol.Selection) error {
	// Controller already holds ownership, authority, source and choice locks.
	// Failure of the real input claim rolls back all newly published choices.
	payload, _ := json.Marshal(map[string]any{"kind": "guarded_tab_selection", "version": 1, "approval_id": selection.ApprovalID})
	if err := s.inputs.SuspendApprovalTx(ctx, tx, b.RunID, selection.ApprovalID, string(payload)); err != nil {
		return err
	}
	tag, err := tx.Exec(ctx, `UPDATE runs SET status='awaiting_approval',current_state='awaiting_approval',updated_at=clock_timestamp() WHERE run_id=$1 AND session_key=$2 AND task_id=$3 AND task_revision=$4 AND current_state='tool_pending'`, b.RunID, b.SessionKey, b.TaskID, b.TaskRevision)
	if err != nil || tag.RowsAffected() != 1 {
		return singletabcontrol.ErrDenied
	}
	tag, err = tx.Exec(ctx, `UPDATE tasks SET status='waiting_for_user',updated_at=clock_timestamp() WHERE task_id=$1 AND last_run_id=$2 AND revision=$3 AND constraints_revision=$4 AND status='active'`, b.TaskID, b.RunID, b.TaskRevision, b.ConstraintsRevision)
	if err != nil || tag.RowsAffected() != 1 {
		return singletabcontrol.ErrDenied
	}
	_, err = tx.Exec(ctx, `INSERT INTO run_state_transitions(run_id,from_state,to_state,triggered_by,metadata_json,transitioned_at) VALUES($1,'tool_pending','awaiting_approval','orchestrator',$2::jsonb,clock_timestamp())`, b.RunID, payload)
	if err != nil {
		return err
	}
	token, ok := ownership.TokenFromContext(ctx)
	var live bool
	if !ok || tx.QueryRow(ctx, `SELECT run_id=$2 AND lease_id=$3 AND fence_epoch=$4 AND expires_at>clock_timestamp() AND $5::timestamptz>clock_timestamp() FROM session_ownership WHERE session_key=$1`, b.SessionKey, b.RunID, token.LeaseID, b.LeaseEpoch, selection.ExpiresAt).Scan(&live) != nil || !live || s.sources(ctx, tx, b.RunID) != nil {
		return singletabcontrol.ErrDenied
	}
	return err
}

func (s *SelectionCheckpoints) ResolveSelectionTx(ctx context.Context, tx pgx.Tx, b resources.Binding, id string, revoked bool) error {
	// This hook runs only after the current waiting-owner/source guard and the
	// exact selected or revoked state are stabilized. It wakes the same input;
	// task activation belongs to the freshly leased recovery worker below.
	state := "selected"
	if revoked {
		state = "revoked"
	}
	var valid bool
	if tx.QueryRow(ctx, `SELECT state=$3 AND run_id=$2 AND owner_id=$4 AND task_id=$5 AND task_revision=$6 AND constraints_revision=$7 FROM guarded_single_tab_requests WHERE approval_id=$1`, id, b.RunID, state, b.OwnerID, b.TaskID, b.TaskRevision, b.ConstraintsRevision).Scan(&valid) != nil || !valid {
		return singletabcontrol.ErrDenied
	}
	return s.inputs.ReadySelectionTx(ctx, tx, b.RunID, id)
}

// ResumeSelection runs after Session Service acquires a fresh fence and after
// source preparation is reloaded. Its successful result is local selection DATA,
// not authorization or evidence that a browser action has executed.
func (s *SelectionCheckpoints) ResumeSelection(ctx context.Context, runID, id string) (string, error) {
	token, ok := ownership.TokenFromContext(ctx)
	if !ok || token.RunID != runID {
		return "", singletabcontrol.ErrDenied
	}
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return "", err
	}
	defer tx.Rollback(context.Background())
	var b resources.Binding
	var live bool
	err = tx.QueryRow(ctx, `SELECT run_id=$2 AND lease_id=$3 AND fence_epoch=$4 AND expires_at>clock_timestamp(),expires_at FROM session_ownership WHERE session_key=$1 FOR UPDATE`, token.SessionKey, runID, token.LeaseID, token.FenceEpoch).Scan(&live, &b.Deadline)
	if err != nil || !live {
		return "", singletabcontrol.ErrDenied
	}
	var expiry time.Time
	err = tx.QueryRow(ctx, `SELECT owner_id,session_key,run_id,task_id,task_revision,constraints_revision,expires_at FROM guarded_single_tab_requests WHERE approval_id=$1 AND run_id=$2 AND session_key=$3 AND browser_instance=$4`, id, runID, token.SessionKey, s.host).Scan(&b.OwnerID, &b.SessionKey, &b.RunID, &b.TaskID, &b.TaskRevision, &b.ConstraintsRevision, &expiry)
	if err != nil {
		return "", singletabcontrol.ErrDenied
	}
	if expiry.Before(b.Deadline) {
		b.Deadline = expiry
	}
	b.LeaseEpoch = token.FenceEpoch
	var valid bool
	if tx.QueryRow(ctx, `SELECT user_id=$2 FROM sessions WHERE session_key=$1 FOR SHARE`, b.SessionKey, b.OwnerID).Scan(&valid) != nil || !valid {
		return "", singletabcontrol.ErrDenied
	}
	if tx.QueryRow(ctx, `SELECT session_key=$2 AND task_id=$3 AND task_revision=$4 AND current_state IN ('awaiting_approval','awaiting_model_resume','model_running') AND status NOT IN ('completed','failed','cancelled','timed_out') FROM runs WHERE run_id=$1 FOR UPDATE`, runID, b.SessionKey, b.TaskID, b.TaskRevision).Scan(&valid) != nil || !valid {
		return "", singletabcontrol.ErrDenied
	}
	if tx.QueryRow(ctx, `SELECT owner_id=$2 AND session_key=$3 AND revision=$4 AND constraints_revision=$5 AND last_run_id=$6 AND status IN ('waiting_for_user','active') FROM tasks WHERE task_id=$1 FOR UPDATE`, b.TaskID, b.OwnerID, b.SessionKey, b.TaskRevision, b.ConstraintsRevision, runID).Scan(&valid) != nil || !valid {
		return "", singletabcontrol.ErrDenied
	}
	if s.sources(ctx, tx, runID) != nil {
		return "", singletabcontrol.ErrDenied
	}
	// Activate only inside this transaction so denied/expired/revoked selection
	// cannot leave a task active. WorkerGuard repeats full current task authority.
	if _, err = tx.Exec(ctx, `UPDATE tasks SET status='active',updated_at=clock_timestamp() WHERE task_id=$1`, b.TaskID); err != nil {
		return "", err
	}
	if (resources.WorkerGuard{}).CheckTx(ctx, tx, b) != nil {
		return "", singletabcontrol.ErrDenied
	}
	var sessionID string
	if tx.QueryRow(ctx, `SELECT session_id FROM guarded_single_tab_selections WHERE request_id=$1`, id).Scan(&sessionID) != nil {
		return "", singletabcontrol.ErrDenied
	}
	if _, err = singletabcontrol.Guard(s.host)(ctx, tx, b, sessionID); err != nil {
		return "", err
	}
	if s.inputs.CheckSelectionClaimTx(ctx, tx, runID, id) != nil {
		return "", singletabcontrol.ErrDenied
	}
	if (resources.WorkerGuard{}).CheckTx(ctx, tx, b) != nil || s.sources(ctx, tx, runID) != nil {
		return "", singletabcontrol.ErrDenied
	}
	return sessionID, tx.Commit(ctx)
}
