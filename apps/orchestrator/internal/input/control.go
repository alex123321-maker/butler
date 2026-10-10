package input

import (
	"context"
	"encoding/json"

	"github.com/butler/butler/internal/ingress"
	"github.com/jackc/pgx/v5"
)

func (s *Store) InvalidateForControlTx(ctx context.Context, tx pgx.Tx, taskIDs []string, controlID string) (bool, error) {
	var uncertain bool
	err := tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM accepted_inputs WHERE task_id=ANY($1) AND checkpoint IN ('tool_dispatching','tool_result_recorded'))`, taskIDs).Scan(&uncertain)
	if err != nil {
		return false, err
	}
	_, err = tx.Exec(ctx, `UPDATE accepted_inputs SET status='cancelled',claim_id='',claim_expires_at=NULL,error_code='owner_control',error_message=$2,updated_at=clock_timestamp() WHERE task_id=ANY($1) AND status NOT IN ('completed','failed','expired','cancelled')`, taskIDs, controlID)
	return uncertain, err
}
func (s *Store) ResolveAmbiguousForControlTx(ctx context.Context, tx pgx.Tx, sessions []string, owner string) error {
	_, err := tx.Exec(ctx, `UPDATE accepted_inputs SET status='cancelled',error_message='target clarified by authenticated owner',updated_at=clock_timestamp() WHERE session_key=ANY($1) AND owner_id=$2 AND status='waiting_for_user' AND error_code='task_target_required'`, sessions, owner)
	return err
}
func (s *Store) ResolveConversationAmbiguity(ctx context.Context, session, owner string) error {
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)
	var storedOwner string
	if err = tx.QueryRow(ctx, `SELECT user_id FROM sessions WHERE session_key=$1 FOR UPDATE`, session).Scan(&storedOwner); err != nil {
		return err
	}
	if storedOwner != owner {
		return ErrClaimLost
	}
	if err = s.ResolveAmbiguousForControlTx(ctx, tx, []string{session}, owner); err != nil {
		return err
	}
	return tx.Commit(ctx)
}

// Admission session rows are already exclusively held by the controller.
func (s *Store) AdmitControlTx(ctx context.Context, tx pgx.Tx, event ingress.InputEvent, owner, taskID, status, checkpoint, code string) (string, error) {
	if checkpoint == "owner_control_applied" {
		if _, err := tx.Exec(ctx, `UPDATE accepted_inputs SET status='cancelled',error_message='target resolved by owner control',updated_at=clock_timestamp() WHERE session_key=$1 AND owner_id=$2 AND status='waiting_for_user' AND error_code='task_target_required'`, event.SessionKey, owner); err != nil {
			return "", err
		}
	}
	id, err := randomID("input-")
	if err != nil {
		return "", err
	}
	data, err := json.Marshal(event)
	if err != nil {
		return "", err
	}
	_, err = tx.Exec(ctx, `INSERT INTO accepted_inputs(input_id,session_key,event_id,dedup_key,source,owner_id,event_json,task_id,status,checkpoint,error_code) VALUES($1,$2,$3,$4,$5,$6,$7::jsonb,NULLIF($8,''),$9,$10,$11)`, id, event.SessionKey, event.EventID, event.IdempotencyKey, event.Source, owner, data, taskID, status, checkpoint, code)
	return id, err
}
