package communication

import (
	"context"

	"github.com/butler/butler/apps/orchestrator/internal/input"
	"github.com/butler/butler/apps/orchestrator/internal/run"
	"github.com/jackc/pgx/v5"
)

// A kernel-only transaction collaborator. The stop record and failed Input are
// the proof; this does not manufacture owner acknowledgement of an unknown effect.
func (s *Store) FinalizeStoppedTx(ctx context.Context, tx pgx.Tx, stop input.StoppedAttempt) error {
	var valid bool
	err := tx.QueryRow(ctx, `SELECT TRUE FROM input_stopped_attempts f
 JOIN accepted_inputs i ON i.input_id=f.input_id AND i.run_id=f.run_id AND i.owner_id=f.owner_id AND i.session_key=f.session_key
 JOIN sessions ss ON ss.session_key=i.session_key AND ss.user_id=i.owner_id
 JOIN runs r ON r.run_id=i.run_id AND r.session_key=i.session_key
 WHERE f.input_id=$1 AND f.run_id=$2 AND f.owner_id=$3 AND f.session_key=$4 AND f.classification=$5 AND f.reason=$6
 AND i.status='failed' AND i.claim_id='' AND i.claim_expires_at IS NULL AND r.current_state<>'completed'
 AND NOT EXISTS(SELECT 1 FROM session_ownership l WHERE l.session_key=i.session_key AND l.lease_id<>'' AND l.expires_at>clock_timestamp())`, stop.InputID, stop.RunID, stop.OwnerID, stop.SessionKey, stop.Classification, stop.Reason).Scan(&valid)
	if err != nil || !valid {
		return ErrDenied
	}
	if err = run.FailStoppedKernelTx(ctx, tx, stop.InputID); err != nil {
		return err
	}
	if _, err = tx.Exec(ctx, `UPDATE communication_admissions SET obligation_state='closed_with_failure' WHERE input_id=$1 AND obligation_state IN ('open','waiting_for_user')`, stop.InputID); err != nil {
		return err
	}
	kind := "input.stopped_failed"
	if stop.Classification == "uncertain" {
		kind = "input.stopped_uncertain"
	}
	_, err = tx.Exec(ctx, `INSERT INTO communication_events(owner_id,session_key,aggregate_id,event_type)
 SELECT $1,$2,$3,$4 WHERE NOT EXISTS(SELECT 1 FROM communication_events WHERE owner_id=$1 AND session_key=$2 AND aggregate_id=$3 AND event_type=$4)`, stop.OwnerID, stop.SessionKey, stop.InputID, kind)
	return err
}
