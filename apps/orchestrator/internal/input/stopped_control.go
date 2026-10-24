package input

import (
	"context"
	"errors"
	"slices"

	"github.com/butler/butler/internal/communicationcontrol"
	"github.com/butler/butler/internal/owneridentity"
	"github.com/jackc/pgx/v5"
)

var ErrControlConflict = errors.New("stopped input control conflict")

// CloseStoppedTx is a terminal acknowledgement of failure, never a retry. The
// original checkpoint, errors, spending, model attempts and unknowns are kept.
// Communication owns the audit event in the caller's same transaction.
func (s *Store) CloseStoppedTx(ctx context.Context, tx pgx.Tx, id, checkpoint, code string, acknowledged []string) (Record, bool, error) {
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
	if len(acknowledged) > 64 {
		return r, false, ErrControlConflict
	}
	rows, err := tx.Query(ctx, `SELECT action_id,state,COALESCE(initial_binding->>'effect',''),COALESCE(initial_binding->>'tool_name','') FROM action_effects WHERE initial_binding->>'run_id'=$1 ORDER BY action_id FOR SHARE`, r.RunID)
	if err != nil {
		return r, false, err
	}
	var uncertain []string
	executing := false
	unsupportedUnknown := false
	for rows.Next() {
		var actionID, state, effect, toolName string
		if err = rows.Scan(&actionID, &state, &effect, &toolName); err != nil {
			break
		}
		if effect != "read" {
			executing = executing || state == "executing"
			if state == "effect_unknown" {
				uncertain = append(uncertain, actionID)
				unsupportedUnknown = unsupportedUnknown || effect != "prepare" || !communicationcontrol.IsTool(toolName)
			}
		}
	}
	if err == nil {
		err = rows.Err()
	}
	rows.Close()
	if err != nil {
		return r, false, err
	}
	// Abandoning this Input is not effect reconciliation. Explicit owner IDs
	// acknowledge exactly the retained unknown set; executing writes still deny.
	// No grant, journal state, receipt, observation or reservation is changed.
	wanted := slices.Clone(acknowledged)
	slices.Sort(wanted)
	if executing || unsupportedUnknown || !slices.Equal(wanted, uncertain) {
		return r, false, ErrControlConflict
	}
	if r.Status == "failed" {
		return r, false, nil
	}
	_, err = tx.Exec(ctx, `UPDATE accepted_inputs SET status='failed',updated_at=clock_timestamp() WHERE input_id=$1`, id)
	return r, true, err
}
