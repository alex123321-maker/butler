package communication

import (
	"context"
	"encoding/json"
	"errors"
	"github.com/butler/butler/internal/communicationcontrol"
	"github.com/butler/butler/internal/ownership"
	"github.com/jackc/pgx/v5"
)

// Explicit is read from the kernel-pinned Input, never provider/model metadata.
func (s *Store) Explicit(ctx context.Context, runID string) (bool, error) {
	var yes bool
	e := s.Pool.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM communication_admissions a JOIN accepted_inputs i USING(input_id) WHERE i.run_id=$1 AND a.schema_epoch=1)`, runID).Scan(&yes)
	return yes, e
}
func (s *Store) principalTx(ctx context.Context, tx pgx.Tx, runID string) (Principal, error) {
	var p Principal
	p.RunID = runID
	e := tx.QueryRow(ctx, `SELECT i.input_id,i.owner_id,i.session_key,COALESCE(r.task_id,''),COALESCE(r.task_revision,0),COALESCE(t.constraints_revision,0) FROM accepted_inputs i JOIN runs r USING(run_id) LEFT JOIN tasks t ON t.task_id=r.task_id WHERE i.run_id=$1`, runID).Scan(&p.InputID, &p.OwnerID, &p.SessionKey, &p.TaskID, &p.TaskRevision, &p.ConstraintsRevision)
	if e != nil {
		return p, ErrDenied
	}
	return p, nil
}

// BeforeFinal durably spends the single correction before inference. A crash
// after that spend cannot start another correction. A final is internal DATA.
func (s *Store) BeforeFinal(ctx context.Context, runID, body string) (string, error) {
	ctx, release, e := ownership.Begin(ctx)
	if e != nil {
		return "", e
	}
	defer release()
	tx, e := s.Pool.Begin(ctx)
	if e != nil {
		return "", e
	}
	defer tx.Rollback(context.Background())
	p, e := s.principalTx(ctx, tx, runID)
	if e != nil {
		return "", e
	}
	var yes bool
	if tx.QueryRow(ctx, `SELECT TRUE FROM sessions WHERE session_key=$1 FOR UPDATE`, p.SessionKey).Scan(&yes) != nil {
		return "", ErrDenied
	}
	if e = s.guard(ctx, tx, p); e != nil {
		return "", e
	}
	var state string
	var corrections int
	var quiet bool
	if tx.QueryRow(ctx, `SELECT obligation_state,correction_count,quiet FROM communication_admissions WHERE input_id=$1 FOR UPDATE`, p.InputID).Scan(&state, &corrections, &quiet) != nil {
		return "", ErrDenied
	}
	var missingObservation bool
	if e = tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM scheduled_occurrences o JOIN scheduled_jobs j USING(schedule_id) WHERE o.input_id=$1 AND COALESCE((j.spec_json->>'monitor')::boolean,FALSE) AND NOT EXISTS(SELECT 1 FROM schedule_observations n WHERE n.occurrence_id=o.occurrence_id))`, p.InputID).Scan(&missingObservation); e != nil {
		return "", e
	}
	if missingObservation && state != "closed_with_failure" {
		state = "open"
		quiet = false
	}
	if state == "response_committed" {
		return "ready", tx.Commit(ctx)
	}
	if state == "waiting_for_user" {
		return "waiting", tx.Commit(ctx)
	}
	if state != "open" {
		return "failed", tx.Commit(ctx)
	}
	if quiet {
		// An optional question temporarily opens even a quiet/verified_task
		// obligation. Its answer must not force a forbidden/unnecessary result,
		// but Completion still needs an explicitly satisfied obligation.
		if _, e = tx.Exec(ctx, `UPDATE communication_admissions SET obligation_state='response_committed' WHERE input_id=$1`, p.InputID); e != nil {
			return "", e
		}
		return "ready", tx.Commit(ctx)
	}
	if corrections == 0 {
		metadata := `{"kind":"attempt_final","visibility":"internal","published":false}`
		_, e = tx.Exec(ctx, `INSERT INTO messages(message_id,session_key,run_id,role,content,metadata) VALUES($1,$2,$3,'system',$4,$5::jsonb)`, "attempt:"+runID+":0", p.SessionKey, runID, body, metadata)
		if e != nil {
			return "", e
		}
		_, e = tx.Exec(ctx, `UPDATE communication_admissions SET correction_count=1 WHERE input_id=$1`, p.InputID)
		if e != nil {
			return "", e
		}
		_, e = tx.Exec(ctx, `UPDATE accepted_inputs SET checkpoint='communication_correction_started',updated_at=clock_timestamp() WHERE input_id=$1 AND status='running'`, p.InputID)
		if e != nil {
			return "", e
		}
		return "correct", tx.Commit(ctx)
	}
	// System failure uses a fixed safe body and stable effect identity. It cannot
	// publish a raw model final or choose recipients. At most one kernel notice
	// is allowed in addition to the agent quota, without authorizing another tool.
	args := communicationcontrol.Args{SchemaVersion: 1, Purpose: "blocked", Text: "Butler не смог подготовить ответ на запрос. Выполнение остановлено; требуется проверка состояния поручения."}
	raw, _ := json.Marshal(args)
	notice, e := tx.Begin(ctx) // A refused notification must not leave an endless correction.
	if e != nil {
		return "", e
	}
	_, e = s.applyTx(ctx, notice, p, communicationcontrol.Envelope{RunID: runID, ToolCallID: "system:missing-response", EffectID: "system:missing-response:" + p.InputID, Tool: "communication.message", ArgsJSON: string(raw)}, true)
	if e == nil {
		e = notice.Commit(ctx)
	} else {
		_ = notice.Rollback(ctx)
		if errors.Is(e, ErrDenied) {
			e = s.eventTx(ctx, tx, p.OwnerID, p.SessionKey, p.InputID, "obligation.failure_notice_denied")
		}
	}
	if e != nil {
		return "", e
	}
	_, e = tx.Exec(ctx, `UPDATE communication_admissions SET obligation_state='closed_with_failure' WHERE input_id=$1`, p.InputID)
	if e != nil {
		return "", e
	}
	return "failed", tx.Commit(ctx)
}

// FinalizeTx is joined to the existing completion transaction. No automatic
// notification is produced for explicit attempts; accepted publications own it.
func (s *Store) FinalizeTx(ctx context.Context, tx pgx.Tx, runID string) (bool, error) {
	var state, inputID string
	e := tx.QueryRow(ctx, `SELECT a.obligation_state,a.input_id FROM communication_admissions a JOIN accepted_inputs i USING(input_id) WHERE i.run_id=$1 FOR UPDATE OF a`, runID).Scan(&state, &inputID)
	if errors.Is(e, pgx.ErrNoRows) {
		return false, nil
	}
	if e != nil {
		return false, e
	}
	if state != "response_committed" {
		return true, ErrDenied
	}
	if s.SourceGuard == nil || s.Inputs == nil || s.Inputs.CheckCommunicationClaimTx(ctx, tx, runID, inputID) != nil {
		return true, ErrDenied
	}
	return true, s.SourceGuard(ctx, tx, runID)
}
