// Package scheduled owns durable agent-created jobs and occurrence admission.
// Clock arithmetic is shared with initiatives; execution uses Task/Input owners.
package scheduled

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"time"

	durableinput "github.com/butler/butler/apps/orchestrator/internal/input"
	"github.com/butler/butler/apps/orchestrator/internal/task"
	"github.com/butler/butler/internal/actionauth"
	"github.com/butler/butler/internal/budget"
	"github.com/butler/butler/internal/schedulecontrol"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

var ErrDenied = schedulecontrol.ErrDenied

type Store struct {
	Pool        *pgxpool.Pool
	Tasks       *task.Store
	Inputs      *durableinput.Store
	Budgets     *budget.Controller
	Allowance   budget.ModelAllowance
	SourceGuard func(context.Context, pgx.Tx, string) error
}

// Principal is private verified dispatch metadata. It is never decoded from tool
// arguments, an observer, a child report or a service authentication token.
type Principal struct {
	OwnerID, SessionKey, RunID, SourceTaskID string
	Constraints                              []string
	SourceGoal, ExecutionPolicy              string
	Authority                                *actionauth.Assignment
	Approved                                 bool
}
type job struct {
	schedulecontrol.View
	owner, session, sourceRun, sourceTask, sourceGoal, policy string
	constraints                                               []string
	authority                                                 *actionauth.Assignment
	approved                                                  bool
	expiry                                                    time.Time
	maximum                                                   int
}

const jobColumns = `schedule_id,owner_id,session_key,source_run_id,COALESCE(source_task_id,''),source_goal,source_constraints,execution_policy,authority_json,approved,spec_json,revision,state,reason,next_at,expires_at,maximum_occurrences,occurrence_count`

func scanJob(row interface{ Scan(...any) error }) (job, error) {
	var j job
	var constraints, spec, authority []byte
	err := row.Scan(&j.ID, &j.owner, &j.session, &j.sourceRun, &j.sourceTask, &j.sourceGoal, &constraints, &j.policy, &authority, &j.approved, &spec, &j.Revision, &j.State, &j.Reason, &j.NextAt, &j.expiry, &j.maximum, &j.OccurrenceCount)
	if err != nil {
		return j, err
	}
	if json.Unmarshal(spec, &j.Spec) != nil || json.Unmarshal(constraints, &j.constraints) != nil {
		return j, ErrDenied
	}
	if len(authority) != 0 && string(authority) != "null" {
		if json.Unmarshal(authority, &j.authority) != nil || j.authority == nil {
			return j, ErrDenied
		}
	}
	return j, nil
}
func newID(prefix string) (string, error) {
	var b [16]byte
	if _, err := rand.Read(b[:]); err != nil {
		return "", err
	}
	return prefix + hex.EncodeToString(b[:]), nil
}
func (s *Store) CheckReady(ctx context.Context) error {
	if s == nil || s.Pool == nil || s.Tasks == nil || s.Inputs == nil || s.Budgets == nil || s.SourceGuard == nil {
		return ErrDenied
	}
	_, err := s.Pool.Exec(ctx, `SELECT j.schedule_id,o.occurrence_id,r.effect_id,n.notification_id,m.state_hash FROM scheduled_jobs j,scheduled_occurrences o,schedule_control_receipts r,schedule_reminder_outbox n,schedule_observations m WHERE FALSE`)
	if err != nil {
		return err
	}
	var ready bool
	err = s.Pool.QueryRow(ctx, `SELECT to_regprocedure('butler_task_check_schedule(text)') IS NOT NULL AND EXISTS(SELECT 1 FROM pg_trigger WHERE tgrelid='schedule_control_receipts'::regclass AND tgname='schedule_receipt_immutable' AND tgenabled IN ('O','A')) AND EXISTS(SELECT 1 FROM pg_trigger WHERE tgrelid='schedule_reminder_outbox'::regclass AND tgname='schedule_notice_immutable' AND tgenabled IN ('O','A')) AND EXISTS(SELECT 1 FROM pg_trigger WHERE tgrelid='schedule_observations'::regclass AND tgname='schedule_observation_immutable' AND tgenabled IN ('O','A'))`).Scan(&ready)
	if err != nil {
		return err
	}
	if !ready {
		return ErrDenied
	}
	return nil
}
func (s *Store) ApplyTx(ctx context.Context, tx pgx.Tx, p Principal, tool, effect, hash string, a schedulecontrol.Args) (schedulecontrol.Receipt, error) {
	if p.OwnerID == "" || p.SessionKey == "" || p.RunID == "" || effect == "" || hash == "" {
		return schedulecontrol.Receipt{}, ErrDenied
	}
	var oldHash string
	var raw []byte
	err := tx.QueryRow(ctx, `SELECT request_hash,receipt_json FROM schedule_control_receipts WHERE effect_id=$1 AND owner_id=$2 AND session_key=$3`, effect, p.OwnerID, p.SessionKey).Scan(&oldHash, &raw)
	if err == nil {
		var receipt schedulecontrol.Receipt
		if oldHash != hash || json.Unmarshal(raw, &receipt) != nil {
			return receipt, ErrDenied
		}
		if receipt.Schedule != nil {
			if err = s.projectTx(ctx, tx, p, receipt.Schedule); err != nil {
				return receipt, err
			}
		}
		for i := range receipt.Schedules {
			if err = s.projectTx(ctx, tx, p, &receipt.Schedules[i]); err != nil {
				return receipt, err
			}
			receipt.Schedules[i].Instruction = ""
			receipt.Schedules[i].CompletionCriteria = []string{}
		}
		return receipt, nil
	}
	if !errors.Is(err, pgx.ErrNoRows) {
		return schedulecontrol.Receipt{}, err
	}
	var receipt schedulecontrol.Receipt
	if tool == "schedule.list" {
		limit := a.Limit
		if limit == 0 {
			limit = 5
		}
		if limit > 10 {
			return receipt, ErrDenied
		}
		rows, e := tx.Query(ctx, `SELECT `+jobColumns+` FROM scheduled_jobs WHERE owner_id=$1 AND session_key=$2 AND schedule_id>$3 ORDER BY schedule_id LIMIT $4`, p.OwnerID, p.SessionKey, a.AfterID, limit+1)
		if e != nil {
			return receipt, e
		}
		receipt.Schedules = []schedulecontrol.View{}
		for rows.Next() {
			j, e := scanJob(rows)
			if e != nil {
				rows.Close()
				return receipt, e
			}
			if len(receipt.Schedules) == limit {
				receipt.NextCursor = receipt.Schedules[limit-1].ID
				break
			}
			receipt.Schedules = append(receipt.Schedules, j.View)
		}
		rows.Close()
		if e = rows.Err(); e != nil {
			return receipt, e
		}
		// Do not borrow another connection while the result cursor is open.
		for i := range receipt.Schedules {
			if err = s.projectTx(ctx, tx, p, &receipt.Schedules[i]); err != nil {
				return receipt, err
			}
			receipt.Schedules[i].Instruction = ""
			receipt.Schedules[i].CompletionCriteria = []string{}
		}
	} else {
		var j job
		if tool == "schedule.create" {
			j, err = s.createTx(ctx, tx, p, *a.Job)
		} else {
			j, err = scanJob(tx.QueryRow(ctx, `SELECT `+jobColumns+` FROM scheduled_jobs WHERE schedule_id=$1 AND owner_id=$2 AND session_key=$3 FOR UPDATE`, a.ScheduleID, p.OwnerID, p.SessionKey))
			if err == nil && tool != "schedule.get" {
				j, err = s.changeTx(ctx, tx, p, j, tool, a)
				if err == nil {
					err = s.invalidateInputsTx(ctx, tx, j.ID, j.Revision)
				}
			}
		}
		if err != nil {
			return receipt, err
		}
		j.History, err = s.historyTx(ctx, tx, j.ID)
		if err != nil {
			return receipt, err
		}
		if err = s.projectTx(ctx, tx, p, &j.View); err != nil {
			return receipt, err
		}
		receipt.Schedule = &j.View
	}
	raw, err = json.Marshal(receipt)
	if err != nil {
		return receipt, err
	}
	_, err = tx.Exec(ctx, `INSERT INTO schedule_control_receipts(effect_id,owner_id,session_key,request_hash,receipt_json) VALUES($1,$2,$3,$4,$5)`, effect, p.OwnerID, p.SessionKey, hash, raw)
	return receipt, err
}

func (s *Store) invalidateInputsTx(ctx context.Context, tx pgx.Tx, id string, revision int64) error {
	rows, err := tx.Query(ctx, `WITH RECURSIVE obsolete AS (
 SELECT task_id FROM scheduled_occurrences WHERE schedule_id=$1 AND schedule_revision<>$2 AND task_id IS NOT NULL
 UNION SELECT t.task_id FROM tasks t JOIN obsolete p ON t.parent_task_id=p.task_id
 ) SELECT task_id FROM obsolete ORDER BY task_id`, id, revision)
	if err != nil {
		return err
	}
	ids := []string{}
	for rows.Next() {
		var taskID string
		if err = rows.Scan(&taskID); err != nil {
			rows.Close()
			return err
		}
		ids = append(ids, taskID)
	}
	rows.Close()
	if err = rows.Err(); err != nil {
		return err
	}
	return s.Inputs.InvalidateForScheduleTx(ctx, tx, ids)
}
func (s *Store) validatePrincipalTx(ctx context.Context, tx pgx.Tx, p Principal, spec schedulecontrol.Spec, now time.Time) error {
	if !spec.ExpiresAt.After(now) || spec.ExpiresAt.After(now.Add(30*24*time.Hour)) || p.ExecutionPolicy == "prepare_only" {
		return ErrDenied
	}
	if p.Authority != nil {
		m, g, err := actionauth.CheckAssignmentTx(ctx, tx, *p.Authority, now)
		if err != nil {
			return err
		}
		if (!m.AllowInitiative && !p.Approved) || spec.ExpiresAt.After(m.ExpiresAt) || spec.ExpiresAt.After(m.BudgetDeadline) || spec.ExpiresAt.After(g.ExpiresAt) {
			return ErrDenied
		}
	} else if !p.Approved {
		return ErrDenied
	}
	return nil
}
func (s *Store) createTx(ctx context.Context, tx pgx.Tx, p Principal, spec schedulecontrol.Spec) (job, error) {
	var now time.Time
	if err := tx.QueryRow(ctx, `SELECT clock_timestamp()`).Scan(&now); err != nil {
		return job{}, err
	}
	var count int
	if err := tx.QueryRow(ctx, `SELECT count(*) FROM scheduled_jobs WHERE owner_id=$1 AND state IN ('active','paused','blocked')`, p.OwnerID).Scan(&count); err != nil {
		return job{}, err
	}
	if count >= 100 {
		return job{}, ErrDenied
	}
	if err := s.validatePrincipalTx(ctx, tx, p, spec, now); err != nil {
		return job{}, err
	}
	clock, err := spec.Timing.Schedule(now)
	if err != nil {
		return job{}, err
	}
	next := clock.NextAfter(now)
	if next.IsZero() || !next.Before(spec.ExpiresAt) {
		return job{}, ErrDenied
	}
	// Resolve relative time once, inside the idempotent admission transaction.
	if spec.Timing.AfterSeconds > 0 {
		spec.Timing.At = clock.At
		spec.Timing.AfterSeconds = 0
	}
	id, err := newID("sch_")
	if err != nil {
		return job{}, err
	}
	state := "active"
	if spec.Paused {
		state = "paused"
	}
	if p.Constraints == nil {
		p.Constraints = []string{}
	}
	data, _ := json.Marshal(spec)
	constraints, _ := json.Marshal(p.Constraints)
	authority, _ := json.Marshal(p.Authority)
	return scanJob(tx.QueryRow(ctx, `INSERT INTO scheduled_jobs(schedule_id,owner_id,session_key,source_run_id,source_task_id,source_goal,source_constraints,execution_policy,authority_json,approved,spec_json,state,next_at,expires_at,maximum_occurrences) VALUES($1,$2,$3,$4,NULLIF($5,''),$6,$7,$8,$9,$10,$11,$12,$13,$14,$15) RETURNING `+jobColumns, id, p.OwnerID, p.SessionKey, p.RunID, p.SourceTaskID, p.SourceGoal, constraints, p.ExecutionPolicy, authority, p.Approved, data, state, next, spec.ExpiresAt, spec.MaxOccurrences))
}

// A saved schedule is not a permanent grant to republish deleted/restricted
// source text. Keep identity/status inspectable while withholding its payload.
func (s *Store) projectTx(ctx context.Context, tx pgx.Tx, p Principal, v *schedulecontrol.View) error {
	var run string
	var authority []byte
	if err := tx.QueryRow(ctx, `SELECT source_run_id,authority_json FROM scheduled_jobs WHERE schedule_id=$1`, v.ID).Scan(&run, &authority); err != nil {
		return err
	}
	var original *actionauth.Assignment
	if len(authority) > 0 && json.Unmarshal(authority, &original) != nil {
		return ErrDenied
	}
	if original != nil && !sameScope(original, p.Authority) {
		v.Title = "Authority scope unavailable"
		v.Instruction = ""
		v.CompletionCriteria = []string{}
		v.Reason = "authority_scope_unavailable"
		return nil
	}
	nested, err := tx.Begin(ctx)
	if err != nil {
		return err
	}
	if err = s.SourceGuard(ctx, nested, run); err != nil {
		if rollback := nested.Rollback(ctx); rollback != nil {
			return rollback
		}
		v.Title = "Source unavailable"
		v.Instruction = ""
		v.CompletionCriteria = []string{}
		v.Reason = "source_unavailable"
		return nil
	}
	return nested.Commit(ctx)
}

func sameScope(a, b *actionauth.Assignment) bool {
	return a != nil && b != nil && a.OwnerID == b.OwnerID && a.GoalID == b.GoalID && a.GoalRevision == b.GoalRevision && a.MandateID == b.MandateID && a.MandateRevision == b.MandateRevision && a.PolicyRevision == b.PolicyRevision && a.Purpose == b.Purpose
}
func (s *Store) changeTx(ctx context.Context, tx pgx.Tx, p Principal, j job, tool string, a schedulecontrol.Args) (job, error) {
	if j.Revision != a.ExpectedRevision || j.State == "cancelled" {
		return j, ErrDenied
	}
	if !p.Approved && (j.authority == nil || p.Authority == nil || p.Authority.GoalID != j.authority.GoalID || p.Authority.MandateID != j.authority.MandateID || p.Authority.MandateRevision != j.authority.MandateRevision || p.Authority.PolicyRevision != j.authority.PolicyRevision) {
		return j, ErrDenied
	}
	if tool == "schedule.cancel" {
		return scanJob(tx.QueryRow(ctx, `UPDATE scheduled_jobs SET state='cancelled',reason='cancelled',next_at=NULL,revision=revision+1,updated_at=clock_timestamp() WHERE schedule_id=$1 RETURNING `+jobColumns, j.ID))
	}
	if j.State == "finished" || a.Job == nil || a.Job.MaxOccurrences > j.maximum || a.Job.MaxOccurrences <= j.OccurrenceCount || a.Job.ExpiresAt.After(j.expiry) {
		return j, ErrDenied
	}
	// An ordinary task cannot replace a job that belongs to a different goal or
	// change its original authorization. Explicit approval may narrow its payload.
	if j.authority != nil && (p.Authority == nil || *p.Authority != *j.authority) {
		if p.Authority == nil || p.Authority.OwnerID != j.authority.OwnerID || p.Authority.GoalID != j.authority.GoalID || p.Authority.GoalRevision != j.authority.GoalRevision || p.Authority.MandateID != j.authority.MandateID || p.Authority.MandateRevision != j.authority.MandateRevision || p.Authority.PolicyRevision != j.authority.PolicyRevision {
			return j, ErrDenied
		}
	}
	now := time.Time{}
	if err := tx.QueryRow(ctx, `SELECT clock_timestamp()`).Scan(&now); err != nil {
		return j, err
	}
	if err := s.validatePrincipalTx(ctx, tx, p, *a.Job, now); err != nil {
		return j, err
	}
	clock, err := a.Job.Timing.Schedule(now)
	if err != nil {
		return j, err
	}
	next := clock.NextAfter(now)
	if bytes.Equal(mustJSON(a.Job.Timing), mustJSON(j.Timing)) && j.NextAt != nil {
		next = *j.NextAt
	}
	if next.IsZero() || !next.Before(a.Job.ExpiresAt) {
		return j, ErrDenied
	}
	if a.Job.Timing.AfterSeconds > 0 {
		a.Job.Timing.At = clock.At
		a.Job.Timing.AfterSeconds = 0
	}
	state := "active"
	if a.Job.Paused {
		state = "paused"
	}
	data, _ := json.Marshal(a.Job)
	return scanJob(tx.QueryRow(ctx, `UPDATE scheduled_jobs SET spec_json=$2,state=$3,reason='',next_at=$4,expires_at=$5,revision=revision+1,updated_at=clock_timestamp() WHERE schedule_id=$1 RETURNING `+jobColumns, j.ID, data, state, next, a.Job.ExpiresAt))
}
func (s *Store) historyTx(ctx context.Context, tx pgx.Tx, id string) ([]schedulecontrol.Occurrence, error) {
	rows, err := tx.Query(ctx, `SELECT o.occurrence_id,o.scheduled_at,o.state,o.reason,COALESCE(o.task_id,''),COALESCE(o.input_id,''),COALESCE(r.current_state,i.status,''),COALESCE(t.status,''),COALESCE(f.verdict,''),
 (SELECT CASE WHEN count(*)=0 THEN '' WHEN bool_or(status='delivery_unknown') THEN 'delivery_unknown' WHEN bool_or(status='failed') THEN 'failed' WHEN bool_or(status IN ('pending','sending')) THEN 'pending' WHEN bool_or(status='result_ready') THEN 'result_ready' ELSE 'sent' END
 FROM (SELECT status FROM schedule_reminder_outbox n WHERE n.occurrence_id=o.occurrence_id UNION ALL SELECT status FROM delivery_outbox d WHERE d.run_id=i.run_id) notices)
 FROM scheduled_occurrences o LEFT JOIN accepted_inputs i ON i.input_id=o.input_id LEFT JOIN runs r ON r.run_id=i.run_id LEFT JOIN tasks t ON t.task_id=o.task_id LEFT JOIN run_final_results f ON f.run_id=i.run_id
 WHERE o.schedule_id=$1 ORDER BY o.scheduled_at DESC,o.occurrence_id DESC LIMIT 20`, id)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []schedulecontrol.Occurrence{}
	for rows.Next() {
		var o schedulecontrol.Occurrence
		if err = rows.Scan(&o.ID, &o.ScheduledAt, &o.State, &o.Reason, &o.TaskID, &o.InputID, &o.RunState, &o.TaskStatus, &o.ResultVerdict, &o.DeliveryState); err != nil {
			return nil, err
		}
		out = append(out, o)
	}
	return out, rows.Err()
}
