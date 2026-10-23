package input

import (
	"context"
	"errors"

	"github.com/butler/butler/internal/owneridentity"
	"github.com/jackc/pgx/v5"
)

var ErrControlConflict = errors.New("stopped input control conflict")

// CloseStoppedTx is a terminal acknowledgement of failure, never a retry. The
// original checkpoint, errors, spending, model attempts and unknowns are kept.
// Communication owns the audit event in the caller's same transaction.
func (s *Store) CloseStoppedTx(ctx context.Context, tx pgx.Tx, id, checkpoint, code string) (Record, bool, error) {
	a, ok := owneridentity.FromContext(ctx)
	if !ok || a.OwnerID == "" || a.DeliveryID == "" {
		return Record{}, false, ErrNotFound
	}
	r, err := scan(tx.QueryRow(ctx, `SELECT `+columns+` FROM accepted_inputs WHERE input_id=$1 AND owner_id=$2`, id, a.OwnerID))
	if err != nil {
		return Record{}, false, err
	}
	if r.TaskID != "" || r.RunID == "" {
		return r, false, ErrControlConflict
	}
	if r.Status == "failed" && r.Checkpoint == checkpoint && code != "" && r.ErrorCode == code && r.ClaimID == "" && r.ClaimExpiresAt == nil {
		return r, false, nil
	}
	key, runID := r.SessionKey, r.RunID
	// Match worker order: lease proof before session/run/input. A live owner is
	// never interrupted; an expired lease row remains stable through this close.
	var live bool
	err = tx.QueryRow(ctx, `SELECT lease_id<>'' AND expires_at>clock_timestamp() FROM session_ownership WHERE session_key=$1 FOR SHARE`, r.SessionKey).Scan(&live)
	if err != nil && !errors.Is(err, pgx.ErrNoRows) {
		return r, false, err
	}
	if live {
		return r, false, ErrControlConflict
	}
	var yes bool
	if err = tx.QueryRow(ctx, `SELECT TRUE FROM sessions WHERE session_key=$1 AND user_id=$2 FOR UPDATE`, r.SessionKey, a.OwnerID).Scan(&yes); err != nil {
		return r, false, err
	}
	if err = tx.QueryRow(ctx, `SELECT TRUE FROM runs WHERE run_id=$1 AND session_key=$2 AND task_id IS NULL AND current_state<>'completed' FOR UPDATE`, r.RunID, r.SessionKey).Scan(&yes); err != nil {
		return r, false, err
	}
	r, err = scan(tx.QueryRow(ctx, `SELECT `+columns+` FROM accepted_inputs WHERE input_id=$1 AND owner_id=$2 FOR UPDATE`, id, a.OwnerID))
	if err != nil {
		return r, false, err
	}
	if r.SessionKey != key || r.RunID != runID || r.TaskID != "" || r.Checkpoint != checkpoint || code == "" || r.ErrorCode != code || r.ClaimID != "" || r.ClaimExpiresAt != nil || (r.Status != "blocked" && r.Status != "failed") {
		return r, false, ErrControlConflict
	}
	rows, err := tx.Query(ctx, `SELECT state,COALESCE(initial_binding->>'effect','') FROM action_effects WHERE initial_binding->>'run_id'=$1 ORDER BY action_id FOR SHARE`, r.RunID)
	if err != nil {
		return r, false, err
	}
	uncertain := false
	for rows.Next() {
		var state, effect string
		if err = rows.Scan(&state, &effect); err != nil {
			break
		}
		uncertain = uncertain || ((state == "executing" || state == "effect_unknown") && effect != "read")
	}
	if err == nil {
		err = rows.Err()
	}
	rows.Close()
	if err != nil {
		return r, false, err
	}
	if uncertain {
		return r, false, ErrControlConflict
	}
	if r.Status == "failed" {
		return r, false, nil
	}
	_, err = tx.Exec(ctx, `UPDATE accepted_inputs SET status='failed',updated_at=clock_timestamp() WHERE input_id=$1`, id)
	return r, true, err
}
