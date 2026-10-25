package input

import (
	"context"
	"encoding/json"
	"errors"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
)

// This startup-only collaborator is composed by the execution kernel. No public
// endpoint or model field can supply a stopped-attempt proof or finalizer.
type StoppedFinalizer interface {
	FinalizeStoppedTx(context.Context, pgx.Tx, StoppedAttempt) error
}

type StoppedAttempt struct {
	InputID, RunID, OwnerID, SessionKey, Classification, Reason string
}

// SweepStopped never dispatches a tool, calls a model or acquires a worker claim.
// Failure ends the attempt; an uncertain action stays in its original journal.
func (s *Store) SweepStopped(ctx context.Context) error {
	if s.StoppedFinalizer == nil || s.CommunicationEpoch != 1 {
		return nil
	}
	rows, err := s.pool.Query(ctx, `SELECT i.input_id FROM accepted_inputs i
 JOIN sessions ss ON ss.session_key=i.session_key AND ss.user_id=i.owner_id
 JOIN runs r ON r.run_id=i.run_id AND r.session_key=i.session_key AND r.task_id IS NOT DISTINCT FROM i.task_id
 LEFT JOIN input_stopped_attempts f ON f.input_id=i.input_id
 WHERE (i.communication_ingress_epoch=1 OR EXISTS(SELECT 1 FROM communication_admissions c WHERE c.input_id=i.input_id AND c.owner_id=i.owner_id AND c.session_key=i.session_key AND c.schema_epoch=1)) AND i.error_code<>''
 AND (i.status='blocked' OR (i.status='failed' AND (f.input_id IS NULL OR f.classification='uncertain')))
 AND COALESCE(f.checked_at,i.updated_at)<clock_timestamp()-interval '5 seconds'
 AND (i.claim_expires_at IS NULL OR i.claim_expires_at<=clock_timestamp())
 AND EXISTS(SELECT 1 FROM session_ownership l WHERE l.session_key=i.session_key AND (l.lease_id='' OR l.expires_at<=clock_timestamp()))
 ORDER BY CASE WHEN i.status='blocked' THEN 0 WHEN f.input_id IS NULL THEN 1 ELSE 2 END,COALESCE(f.checked_at,i.updated_at),i.input_id LIMIT 8`)
	if err != nil {
		return err
	}
	var ids []string
	for rows.Next() {
		var id string
		if err = rows.Scan(&id); err != nil {
			break
		}
		ids = append(ids, id)
	}
	if err == nil {
		err = rows.Err()
	}
	rows.Close()
	if err != nil {
		return err
	}
	var failures error
	for _, id := range ids {
		check, cancel := context.WithTimeout(ctx, 750*time.Millisecond)
		err = s.finalizeStopped(check, id)
		cancel()
		var database *pgconn.PgError
		if errors.Is(err, pgx.ErrNoRows) || (errors.As(err, &database) && database.Code == "55P03") {
			continue
		}
		if err != nil {
			failures = errors.Join(failures, err)
		}
	}
	return failures
}

func (s *Store) finalizeStopped(ctx context.Context, id string) error {
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)
	var stop StoppedAttempt
	var payload string
	// NOWAIT never queues a maintenance lock behind a live worker/controller.
	// Holding the ownership row fences lease acquisition until failure commits.
	err = tx.QueryRow(ctx, `SELECT i.input_id,i.run_id,i.owner_id,i.session_key,jsonb_build_object('checkpoint_data',jsonb_build_object('tool_call_id',i.checkpoint_json->'checkpoint_data'->>'tool_call_id','effect_id',i.checkpoint_json->'checkpoint_data'->>'effect_id'))::text
 FROM accepted_inputs i JOIN sessions ss ON ss.session_key=i.session_key AND ss.user_id=i.owner_id
 JOIN runs r ON r.run_id=i.run_id AND r.session_key=i.session_key AND r.task_id IS NOT DISTINCT FROM i.task_id
 JOIN session_ownership l ON l.session_key=i.session_key
 LEFT JOIN input_stopped_attempts f ON f.input_id=i.input_id
 WHERE i.input_id=$1 AND (i.communication_ingress_epoch=1 OR EXISTS(SELECT 1 FROM communication_admissions c WHERE c.input_id=i.input_id AND c.owner_id=i.owner_id AND c.session_key=i.session_key AND c.schema_epoch=1)) AND i.error_code<>''
 AND (i.status='blocked' OR (i.status='failed' AND (f.input_id IS NULL OR f.classification='uncertain')))
 AND COALESCE(f.checked_at,i.updated_at)<clock_timestamp()-interval '5 seconds'
 AND (i.claim_expires_at IS NULL OR i.claim_expires_at<=clock_timestamp())
 AND (l.lease_id='' OR l.expires_at<=clock_timestamp()) AND r.current_state<>'completed'
 FOR UPDATE OF l,ss,r,i NOWAIT`, id).Scan(&stop.InputID, &stop.RunID, &stop.OwnerID, &stop.SessionKey, &payload)
	if err != nil {
		return err
	}
	var waiting bool
	err = tx.QueryRow(ctx, `SELECT (SELECT status='blocked' FROM accepted_inputs WHERE input_id=$2) AND (EXISTS(SELECT 1 FROM approvals WHERE run_id=$1 AND status='pending' AND expires_at>clock_timestamp())
 OR EXISTS(SELECT 1 FROM communication_interactions WHERE run_id=$1 AND state='open' AND (expires_at IS NULL OR expires_at>clock_timestamp()))
 OR EXISTS(SELECT 1 FROM credential_vault_requests WHERE run_id=$1 AND state IN ('pending','approved') AND expires_at>clock_timestamp()))`, stop.RunID, id).Scan(&waiting)
	if err != nil || waiting {
		return err
	}
	stop.Classification, stop.Reason = "known", "no_uncertainty"
	knownCalls, knownEffects := map[string]bool{}, map[string]bool{}
	rows, err := tx.Query(ctx, `SELECT e.action_id,e.state,COALESCE(e.initial_binding->>'owner_id',''),
 COALESCE(a.tool_call_id,CASE WHEN e.initial_binding->>'run_id'=$1 THEN e.initial_binding->>'tool_call_id' END,'')
 FROM action_effects e LEFT JOIN action_intents a ON a.effect_id=e.action_id AND a.run_id=$1
 WHERE e.initial_binding->>'run_id'=$1 OR a.run_id=$1 FOR SHARE OF e NOWAIT`, stop.RunID)
	if err != nil {
		return err
	}
	for rows.Next() {
		var effect, state, owner, call string
		if err = rows.Scan(&effect, &state, &owner, &call); err != nil {
			break
		}
		known := owner == stop.OwnerID && (state == "prepared" || state == "allowed" || state == "effect_confirmed" || state == "effect_absent")
		knownEffects[effect] = known
		if known && call != "" {
			knownCalls[call] = true
		}
		if !known {
			stop.Classification, stop.Reason = "uncertain", "journal_uncertain"
		}
	}
	if err == nil {
		err = rows.Err()
	}
	rows.Close()
	if err != nil {
		return err
	}
	rows, err = tx.Query(ctx, `SELECT tool_call_id,status,finished_at IS NOT NULL AND (result IS NOT NULL OR error IS NOT NULL) FROM tool_calls WHERE run_id=$1 FOR SHARE NOWAIT`, stop.RunID)
	if err != nil {
		return err
	}
	for rows.Next() {
		var call, status string
		var finished bool
		if err = rows.Scan(&call, &status, &finished); err != nil {
			break
		}
		known := knownCalls[call] || (finished && status == "validation_rejected")
		knownCalls[call] = known
		if !known && stop.Classification != "uncertain" {
			stop.Classification, stop.Reason = "uncertain", "missing_receipt"
		}
	}
	if err == nil {
		err = rows.Err()
	}
	rows.Close()
	if err != nil {
		return err
	}
	var checkpoint struct {
		Data struct {
			Call   string `json:"tool_call_id"`
			Effect string `json:"effect_id"`
		} `json:"checkpoint_data"`
	}
	var dispatching bool
	if err = tx.QueryRow(ctx, `SELECT checkpoint='tool_dispatching' FROM accepted_inputs WHERE input_id=$1`, id).Scan(&dispatching); err != nil {
		return err
	}
	if dispatching && (json.Unmarshal([]byte(payload), &checkpoint) != nil || (!knownCalls[checkpoint.Data.Call] && !knownEffects[checkpoint.Data.Effect])) && stop.Classification != "uncertain" {
		stop.Classification, stop.Reason = "uncertain", "missing_receipt"
	}
	_, err = tx.Exec(ctx, `INSERT INTO input_stopped_attempts(input_id,run_id,owner_id,session_key,classification,reason) VALUES($1,$2,$3,$4,$5,$6)
 ON CONFLICT(input_id) DO UPDATE SET classification=EXCLUDED.classification,reason=EXCLUDED.reason,checked_at=clock_timestamp()`, stop.InputID, stop.RunID, stop.OwnerID, stop.SessionKey, stop.Classification, stop.Reason)
	if err != nil {
		return err
	}
	if _, err = tx.Exec(ctx, `UPDATE accepted_inputs SET status='failed',claim_id='',claim_expires_at=NULL,updated_at=CASE WHEN status='failed' THEN updated_at ELSE clock_timestamp() END WHERE input_id=$1`, id); err != nil {
		return err
	}
	if err = s.StoppedFinalizer.FinalizeStoppedTx(ctx, tx, stop); err != nil {
		return err
	}
	return tx.Commit(ctx)
}
