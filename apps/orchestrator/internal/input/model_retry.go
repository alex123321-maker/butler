package input

import (
	"context"
	"encoding/json"
	"errors"
	"time"

	"github.com/butler/butler/internal/ownership"
	"github.com/jackc/pgx/v5"
)

var ErrModelRetryDenied = errors.New("durable model retry denied")

type modelRetryRecord struct {
	Count             int       `json:"count"`
	Reason            string    `json:"reason"`
	PriorOperationID  string    `json:"prior_operation_id"`
	State             string    `json:"state"`
	NotBefore         time.Time `json:"not_before"`
	ClaimedAt         time.Time `json:"claimed_at"`
	ExecutionDeadline time.Time `json:"execution_deadline,omitempty"`
}

// The private accepted claim and live execution fence precede the durable CAS.
// No caller/model can choose a retry counter, deadline, prior run or owner.
func (s *Store) modelRetryTx(ctx context.Context, runID string, fn func(pgx.Tx, Record, string) error) error {
	claim, ok := ctx.Value(claimKey{}).(Record)
	token, owned := ownership.TokenFromContext(ctx)
	if !ok || !owned || token.RunID != runID || token.SessionKey != claim.SessionKey || ctx.Err() != nil {
		return ErrModelRetryDenied
	}
	ctx, release, err := ownership.Begin(ctx)
	if err != nil {
		return err
	}
	defer release()
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)
	var taskID, state, owner string
	var revision int64
	err = tx.QueryRow(ctx, `SELECT COALESCE(r.task_id,''),COALESCE(r.task_revision,0),r.current_state,s.user_id FROM runs r JOIN sessions s USING(session_key) WHERE r.run_id=$1 AND r.session_key=$2 FOR SHARE OF r`, runID, claim.SessionKey).Scan(&taskID, &revision, &state, &owner)
	if err != nil || owner != claim.OwnerID || state != "model_running" && state != "awaiting_model_resume" {
		return ErrModelRetryDenied
	}
	if taskID != "" {
		var current int64
		var status string
		if tx.QueryRow(ctx, `SELECT revision,status FROM tasks WHERE task_id=$1 FOR SHARE`, taskID).Scan(&current, &status) != nil || current != revision || status != "active" {
			return ErrModelRetryDenied
		}
	}
	var payload string
	if tx.QueryRow(ctx, `SELECT checkpoint_json::text FROM accepted_inputs WHERE input_id=$1 AND claim_id=$2 AND run_id=$3 AND owner_id=$4 AND status='running' AND claim_expires_at>clock_timestamp() AND expires_at>clock_timestamp() FOR UPDATE`, claim.InputID, claim.ClaimID, runID, owner).Scan(&payload) != nil {
		return ErrClaimLost
	}
	var uncertain bool
	const known = `WITH RECURSIVE ancestors AS(SELECT task_id,parent_task_id FROM tasks WHERE task_id=$2 UNION SELECT t.task_id,t.parent_task_id FROM tasks t JOIN ancestors a ON t.task_id=a.parent_task_id), tree AS(SELECT task_id FROM ancestors WHERE parent_task_id IS NULL UNION SELECT t.task_id FROM tasks t JOIN tree p ON t.parent_task_id=p.task_id) SELECT EXISTS(SELECT 1 FROM tool_calls c JOIN runs r ON r.run_id=c.run_id WHERE (r.run_id=$1 OR r.task_id IN(SELECT task_id FROM tree)) AND (c.finished_at IS NULL OR c.status='effect_unknown')) OR EXISTS(SELECT 1 FROM action_intents i JOIN runs r ON r.run_id=i.run_id JOIN action_effects e ON e.action_id=i.effect_id WHERE (r.run_id=$1 OR r.task_id IN(SELECT task_id FROM tree)) AND e.state IN('executing','effect_unknown','reconciling'))`
	if tx.QueryRow(ctx, known, runID, taskID).Scan(&uncertain) != nil || uncertain {
		return ErrModelRetryDenied
	}
	if err = fn(tx, claim, payload); err != nil {
		return err
	}
	return tx.Commit(ctx)
}

func (s *Store) ClaimModelRetry(ctx context.Context, runID, priorOperationID string) error {
	return s.claimModelRetry(ctx, runID, priorOperationID, "model_turn_timeout", nil)
}

// This distinct owning-kernel entry never accepts a reason from model/RPC DATA.
// The original worker deadline is retained, not replaced by the next attempt.
func (s *Store) ClaimAuthorityRetry(ctx context.Context, runID, priorOperationID string, deadline time.Time) error {
	if deadline.IsZero() {
		return ErrModelRetryDenied
	}
	deadline = deadline.UTC().Truncate(time.Microsecond)
	return s.claimModelRetry(ctx, runID, priorOperationID, "authority_observation_timeout", &deadline)
}
func (s *Store) claimModelRetry(ctx context.Context, runID, priorOperationID, reason string, deadline *time.Time) error {
	if priorOperationID == "" {
		return ErrModelRetryDenied
	}
	return s.modelRetryTx(ctx, runID, func(tx pgx.Tx, claim Record, payload string) error {
		var saved map[string]json.RawMessage
		if json.Unmarshal([]byte(payload), &saved) != nil {
			return ErrModelRetryDenied
		}
		if _, exists := saved["model_retry"]; exists {
			return ErrModelRetryDenied
		}
		var valid bool
		query := `SELECT checkpoint='model_budget_reserved' AND checkpoint_json->'checkpoint_data'->>'operation_id'=$2
   AND (($5='model_turn_timeout' AND checkpoint_json->'checkpoint_data'->>'deadline_origin'='model_turn') OR
    ($5='authority_observation_timeout' AND $6::timestamptz>clock_timestamp()+interval '2 seconds' AND $6::timestamptz<=expires_at))
   AND expires_at>clock_timestamp()+interval '2 seconds' AND attempts<max_attempts
   AND EXISTS(SELECT 1 FROM execution_model_attempts a JOIN execution_budget_reservations b ON b.reservation_id=a.reservation_id WHERE a.operation_id=$2 AND a.run_id=$3 AND b.owner_id=$4)
   AND $2=(SELECT operation_id FROM execution_model_attempts WHERE run_id=$3 ORDER BY created_at DESC,operation_id DESC LIMIT 1) FROM accepted_inputs WHERE input_id=$1`
		if tx.QueryRow(ctx, query, claim.InputID, priorOperationID, runID, claim.OwnerID, reason, deadline).Scan(&valid) != nil || !valid {
			return ErrModelRetryDenied
		}
		_, err := tx.Exec(ctx, `WITH stamp AS MATERIALIZED(SELECT clock_timestamp() AS at) UPDATE accepted_inputs SET checkpoint='model_retry_pending',not_before=stamp.at+interval '2 seconds',checkpoint_json=checkpoint_json||jsonb_build_object('model_retry',jsonb_strip_nulls(jsonb_build_object('count',1,'reason',$3::text,'prior_operation_id',$2::text,'state','claimed','claimed_at',stamp.at,'not_before',stamp.at+interval '2 seconds','execution_deadline',$4::timestamptz))),updated_at=stamp.at FROM stamp WHERE input_id=$1`, claim.InputID, priorOperationID, reason, deadline)
		return err
	})
}

func (s *Store) ValidateModelRetry(ctx context.Context, runID, priorOperationID string) error {
	return s.modelRetryTx(ctx, runID, func(tx pgx.Tx, claim Record, payload string) error {
		var valid bool
		if tx.QueryRow(ctx, `SELECT checkpoint='model_retry_pending' AND checkpoint_json->'model_retry'->>'count'='1' AND checkpoint_json->'model_retry'->>'reason' IN ('model_turn_timeout','authority_observation_timeout') AND (checkpoint_json->'model_retry'->>'reason'<>'authority_observation_timeout' OR (checkpoint_json->'model_retry'->>'execution_deadline')::timestamptz>clock_timestamp()) AND checkpoint_json->'model_retry'->>'state'='claimed' AND checkpoint_json->'model_retry'->>'prior_operation_id'=$2 AND (checkpoint_json->'model_retry'->>'not_before')::timestamptz=(checkpoint_json->'model_retry'->>'claimed_at')::timestamptz+interval '2 seconds' AND (checkpoint_json->'model_retry'->>'not_before')::timestamptz<=clock_timestamp() AND $2=(SELECT operation_id FROM execution_model_attempts WHERE run_id=$3 ORDER BY created_at DESC,operation_id DESC LIMIT 1) FROM accepted_inputs WHERE input_id=$1`, claim.InputID, priorOperationID, runID).Scan(&valid) != nil || !valid {
			return ErrModelRetryDenied
		}
		return nil
	})
}

// An admitted retry can start once. A crash after its reservation or this marker
// cannot issue another fresh session from the claimed marker. Later ordinary
// turns retain the exhausted count and do not relabel their new operations.
func (s *Store) MarkModelRetryStarted(ctx context.Context, runID, operationID string) error {
	if _, ok := ctx.Value(claimKey{}).(Record); !ok {
		return nil
	}
	return s.modelRetryTx(ctx, runID, func(tx pgx.Tx, claim Record, payload string) error {
		var saved struct {
			Retry *modelRetryRecord `json:"model_retry"`
		}
		if json.Unmarshal([]byte(payload), &saved) != nil {
			return ErrModelRetryDenied
		}
		if saved.Retry == nil {
			return ErrModelRetryDenied
		}
		if saved.Retry.Count != 1 || (saved.Retry.Reason != "model_turn_timeout" && saved.Retry.Reason != "authority_observation_timeout") {
			return ErrModelRetryDenied
		}
		if saved.Retry.State == "started" {
			return nil
		}
		var valid bool
		if tx.QueryRow(ctx, `SELECT checkpoint='model_retry_pending' AND (checkpoint_json->'model_retry'->>'reason'<>'authority_observation_timeout' OR (checkpoint_json->'model_retry'->>'execution_deadline')::timestamptz>clock_timestamp()) AND checkpoint_json->'model_retry'->>'state'='claimed' AND checkpoint_json->'model_retry'->>'prior_operation_id'<>$2 AND (checkpoint_json->'model_retry'->>'not_before')::timestamptz<=clock_timestamp() AND $2=(SELECT operation_id FROM execution_model_attempts WHERE run_id=$3 ORDER BY created_at DESC,operation_id DESC LIMIT 1) AND EXISTS(SELECT 1 FROM execution_model_attempts a JOIN execution_budget_reservations b ON b.reservation_id=a.reservation_id WHERE a.operation_id=$2 AND a.run_id=$3 AND b.owner_id=$4) FROM accepted_inputs WHERE input_id=$1`, claim.InputID, operationID, runID, claim.OwnerID).Scan(&valid) != nil || !valid {
			return ErrModelRetryDenied
		}
		_, err := tx.Exec(ctx, `UPDATE accepted_inputs SET checkpoint_json=jsonb_set(checkpoint_json,'{model_retry}',checkpoint_json->'model_retry'||jsonb_build_object('state','started','retry_operation_id',$2::text,'started_at',clock_timestamp())),updated_at=clock_timestamp() WHERE input_id=$1`, claim.InputID, operationID)
		return err
	})
}
