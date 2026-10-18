package scheduled

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"time"

	"github.com/butler/butler/apps/orchestrator/internal/deliveryoutbox"
	"github.com/butler/butler/apps/orchestrator/internal/task"
	"github.com/butler/butler/internal/actionauth"
	"github.com/butler/butler/internal/budget"
	runv1 "github.com/butler/butler/internal/gen/run/v1"
	"github.com/butler/butler/internal/ingress"
	"github.com/jackc/pgx/v5"
)

func digest(raw string) string { sum := sha256.Sum256([]byte(raw)); return hex.EncodeToString(sum[:]) }

// Sweep performs only bounded transactional admission, never provider/tool work.
// There is no second run queue: Task and Input commit with each occurrence.
func (s *Store) Sweep(ctx context.Context) error {
	rows, err := s.Pool.Query(ctx, `SELECT schedule_id FROM scheduled_jobs WHERE state='active' AND (next_at<=clock_timestamp() OR expires_at<=clock_timestamp()) ORDER BY next_at,schedule_id LIMIT 50`)
	if err != nil {
		return err
	}
	ids := []string{}
	for rows.Next() {
		var id string
		if err = rows.Scan(&id); err != nil {
			rows.Close()
			return err
		}
		ids = append(ids, id)
	}
	rows.Close()
	if err = rows.Err(); err != nil {
		return err
	}
	var result error
	for _, id := range ids {
		result = errors.Join(result, s.advance(ctx, id))
	}
	return result
}
func (s *Store) sourceTx(ctx context.Context, tx pgx.Tx, j job) error {
	if s.SourceGuard == nil {
		return ErrDenied
	}
	if j.sourceTask != "" {
		var valid bool
		if err := tx.QueryRow(ctx, `SELECT owner_id=$2 AND session_key=$3 AND status NOT IN ('paused','waiting_for_user','cancelled') AND constraints_json=$4::jsonb AND execution_policy=$5 AND goal=$6 AND NOT COALESCE((working_json #>> '{scratch,privacy_redacted}')::boolean,FALSE) FROM tasks WHERE task_id=$1 FOR SHARE`, j.sourceTask, j.owner, j.session, mustJSON(j.constraints), j.policy, j.sourceGoal).Scan(&valid); err != nil || !valid {
			return ErrDenied
		}
	}
	return s.SourceGuard(ctx, tx, j.sourceRun)
}
func mustJSON(v any) []byte { raw, _ := json.Marshal(v); return raw }
func (s *Store) authorityTx(ctx context.Context, tx pgx.Tx, j job, now time.Time) (*budget.MandateScope, error) {
	if !j.expiry.After(now) {
		return nil, ErrDenied
	}
	if j.authority == nil {
		if !j.approved {
			return nil, ErrDenied
		}
		return nil, nil
	}
	m, _, err := actionauth.CheckAssignmentTx(ctx, tx, *j.authority, now)
	if err != nil || (!m.AllowInitiative && !j.approved) {
		return nil, ErrDenied
	}
	b := m.Budget
	return &budget.MandateScope{ID: m.ID, OwnerID: j.owner, Privacy: m.Privacy, Deadline: m.BudgetDeadline, Limits: budget.Units{ModelTurns: b.ModelTurns, Tokens: b.Tokens, CostMicros: b.CostMicros, ExternalCalls: b.ExternalCalls, Children: b.Children, ActiveMillis: b.ActiveMillis, EgressBytes: b.EgressBytes, ProviderInputBytes: b.ProviderInputBytes}}, nil
}
func (s *Store) advance(ctx context.Context, id string) error {
	// Hints choose locks only. Every value is read again under its owned lock.
	hint, err := scanJob(s.Pool.QueryRow(ctx, `SELECT `+jobColumns+` FROM scheduled_jobs WHERE schedule_id=$1`, id))
	if err != nil {
		return err
	}
	tx, err := s.Pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(context.Background())
	var owner, channel string
	if err = tx.QueryRow(ctx, `SELECT user_id,channel FROM sessions WHERE session_key=$1 FOR UPDATE`, hint.session).Scan(&owner, &channel); err != nil {
		return err
	}
	if owner != hint.owner || (channel != "web" && channel != "telegram") {
		return ErrDenied
	}
	// Source tasks precede job and authority locks, matching dispatch/control.
	if hint.sourceTask != "" {
		var source string
		if err = tx.QueryRow(ctx, `SELECT task_id FROM tasks WHERE task_id=$1 FOR SHARE`, hint.sourceTask).Scan(&source); err != nil {
			return err
		}
	}
	j, err := scanJob(tx.QueryRow(ctx, `SELECT `+jobColumns+` FROM scheduled_jobs WHERE schedule_id=$1 FOR UPDATE`, id))
	if err != nil {
		return err
	}
	if j.session != hint.session || j.owner != owner || j.sourceTask != hint.sourceTask || j.State != "active" {
		return tx.Commit(ctx)
	}
	var now time.Time
	if err = tx.QueryRow(ctx, `SELECT clock_timestamp()`).Scan(&now); err != nil {
		return err
	}
	finish := func(state, reason string) error {
		_, e := tx.Exec(ctx, `UPDATE scheduled_jobs SET state=$2,reason=$3,next_at=CASE WHEN $2='finished' THEN NULL ELSE next_at END,updated_at=clock_timestamp() WHERE schedule_id=$1`, id, state, reason)
		if e != nil {
			return e
		}
		return tx.Commit(ctx)
	}
	if !j.expiry.After(now) {
		return finish("finished", "expired")
	}
	if j.NextAt == nil || j.NextAt.After(now) {
		return tx.Commit(ctx)
	}
	clock, err := j.Timing.Schedule(now)
	if err != nil {
		return err
	}
	clock.NextAt = *j.NextAt
	tick, due := clock.Due(now)
	if !due {
		return ErrDenied
	}
	occurrenceID := "occ_" + digest(id+":"+string(mustJSON(j.Revision))+":"+tick.SlotKey)
	record := func(state, reason, taskID, inputID string) error {
		_, e := tx.Exec(ctx, `INSERT INTO scheduled_occurrences(occurrence_id,schedule_id,schedule_revision,slot_key,scheduled_at,state,reason,missed_count,task_id,input_id) VALUES($1,$2,$3,$4,$5,$6,$7,$8,NULLIF($9,''),NULLIF($10,''))`, occurrenceID, id, j.Revision, tick.SlotKey, tick.ScheduledAt, state, reason, tick.MissedCount, taskID, inputID)
		return e
	}
	block := func(reason string) error {
		if e := record("blocked", reason, "", ""); e != nil {
			return e
		}
		return finish("blocked", reason)
	}
	// A Catalog SQL guard may raise an exception. Roll back only that check so
	// the same transaction can durably retain a bounded blocked decision.
	checkPhase := func(fn func() error) error {
		if _, e := tx.Exec(ctx, "SAVEPOINT schedule_check"); e != nil {
			return e
		}
		if e := fn(); e != nil {
			_, rollback := tx.Exec(ctx, "ROLLBACK TO SAVEPOINT schedule_check")
			if rollback != nil {
				return rollback
			}
			_, _ = tx.Exec(ctx, "RELEASE SAVEPOINT schedule_check")
			return e
		}
		_, e := tx.Exec(ctx, "RELEASE SAVEPOINT schedule_check")
		return e
	}
	if err = checkPhase(func() error { return s.sourceTx(ctx, tx, j) }); err != nil {
		return block("source_changed")
	}
	var scope *budget.MandateScope
	err = checkPhase(func() error { var e error; scope, e = s.authorityTx(ctx, tx, j, now); return e })
	if err != nil {
		return block("authority_unavailable")
	}
	if err = tx.QueryRow(ctx, `SELECT clock_timestamp()`).Scan(&now); err != nil {
		return err
	}
	if !j.expiry.After(now) {
		return finish("finished", "expired")
	}
	if err = checkPhase(func() error { _, e := s.authorityTx(ctx, tx, j, now); return e }); err != nil {
		return block("authority_unavailable")
	}
	if j.Timing.MissedPolicy == "skip" && now.Sub(tick.ScheduledAt) > time.Duration(j.Timing.GraceSeconds)*time.Second {
		tick.ShouldObserve = false
	}
	// Nonterminal input/unknown effect prevents overlapping work. Successful
	// attempts may remain unverified tasks and must not stop the next occurrence.
	var busy, unknown bool
	if err = tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM scheduled_occurrences o JOIN accepted_inputs i ON i.input_id=o.input_id WHERE o.schedule_id=$1 AND i.status NOT IN ('completed','failed','cancelled','expired')),
 EXISTS(SELECT 1 FROM scheduled_occurrences o JOIN runs r ON r.task_id=o.task_id JOIN action_intents i ON i.run_id=r.run_id JOIN action_effects e ON e.action_id=i.effect_id WHERE o.schedule_id=$1 AND e.state IN ('effect_unknown','reconciling'))`, id).Scan(&busy, &unknown); err != nil {
		return err
	}
	if unknown {
		return block("effect_unknown")
	}
	state, reason := "skipped", "missed_window"
	if busy {
		reason = "previous_occurrence_active"
	}
	if tick.ShouldObserve && !busy {
		if j.Mode == "agent_task" {
			admissionDay := now.UTC().Format("2006-01-02")
			if err = checkPhase(func() error { return s.Budgets.CheckScheduledRootAdmissionTx(ctx, tx, j.owner, scope, s.Allowance) }); err != nil {
				return block("execution_budget_unavailable")
			}
			// A lock wait may cross expiry, grace or authority boundaries. Re-read the
			// DB clock and all held policy/source guards immediately before admission.
			if err = tx.QueryRow(ctx, `SELECT clock_timestamp()`).Scan(&now); err != nil {
				return err
			}
			if now.UTC().Format("2006-01-02") != admissionDay {
				return tx.Commit(ctx)
			}
			if err = checkPhase(func() error { _, e := s.authorityTx(ctx, tx, j, now); return e }); err != nil {
				return block("authority_unavailable")
			}
			if err = checkPhase(func() error { return s.sourceTx(ctx, tx, j) }); err != nil {
				return block("source_changed")
			}
			if j.Timing.MissedPolicy == "skip" && now.Sub(tick.ScheduledAt) > time.Duration(j.Timing.GraceSeconds)*time.Second {
				reason = "missed_window"
			} else {
				var assignment *actionauth.Assignment
				if j.authority != nil {
					copy := *j.authority
					copy.TaskID = ""
					copy.InitiativeReason = "Scheduled owner job " + id
					copy.ObservationRef = occurrenceID
					assignment = &copy
				}
				created, e := s.Tasks.CreateTx(ctx, tx, task.CreateRequest{SessionKey: j.session, OwnerID: j.owner, Goal: j.Instruction, Source: map[string]any{"type": "scheduled_job", "schedule_id": id, "occurrence_id": occurrenceID}, Constraints: j.constraints, CompletionCriteria: j.CompletionCriteria, InitialExecutionPolicy: j.policy, Authority: assignment})
				if e != nil {
					return e
				}
				payload := mustJSON(map[string]any{"user_id": j.owner, "task_id": created.TaskID, "text": j.Instruction, "schedule_id": id, "occurrence_id": occurrenceID})
				event := ingress.InputEvent{EventID: occurrenceID, IdempotencyKey: occurrenceID, EventType: runv1.InputEventType_INPUT_EVENT_TYPE_SCHEDULED_INTERNAL_EVENT, SessionKey: j.session, Source: "schedule", PayloadJSON: string(payload), CreatedAt: now}
				input, accepted, e := s.Inputs.AcceptTx(ctx, tx, event, j.owner, created.TaskID)
				if e != nil {
					return e
				}
				if !accepted {
					return ErrDenied
				}
				if err = record("admitted", "ordinary_task_admitted", created.TaskID, input.InputID); err != nil {
					return err
				}
				state = "admitted"
				reason = "ordinary_task_admitted"
			}
		} else {
			if err = record("notified", "reminder_ready", "", ""); err != nil {
				return err
			}
			status := "pending"
			if channel == "web" {
				status = "result_ready"
			}
			for i, chunk := range deliveryoutbox.Split(j.Instruction, 4096) {
				_, err = tx.Exec(ctx, `INSERT INTO schedule_reminder_outbox(notification_id,occurrence_id,schedule_id,schedule_revision,owner_id,session_key,channel,chunk_index,content,status,deadline_at) VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11)`, occurrenceID+":"+string(mustJSON(i)), occurrenceID, id, j.Revision, j.owner, j.session, channel, i, chunk, status, now.Add(24*time.Hour))
				if err != nil {
					return err
				}
			}
			state = "notified"
			reason = "reminder_ready"
		}
	}
	if state == "skipped" {
		if err = record(state, reason, "", ""); err != nil {
			return err
		}
	}
	var next *time.Time
	if !tick.NextAt.IsZero() {
		copy := tick.NextAt
		next = &copy
	}
	jobState := "active"
	if next == nil || !next.Before(j.ExpiresAt) || j.OccurrenceCount+1 >= j.MaxOccurrences {
		jobState = "finished"
		next = nil
	}
	_, err = tx.Exec(ctx, `UPDATE scheduled_jobs SET state=$2,reason=$3,next_at=$4,occurrence_count=occurrence_count+1,updated_at=clock_timestamp() WHERE schedule_id=$1`, id, jobState, reason, next)
	if err != nil {
		return err
	}
	return tx.Commit(ctx)
}
