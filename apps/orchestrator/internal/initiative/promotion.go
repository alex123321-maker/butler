package initiative

import (
	"context"
	"encoding/json"
	"errors"
	"time"

	"github.com/butler/butler/apps/orchestrator/internal/goal"
	durableinput "github.com/butler/butler/apps/orchestrator/internal/input"
	"github.com/butler/butler/apps/orchestrator/internal/task"
	"github.com/butler/butler/internal/actionauth"
	"github.com/butler/butler/internal/budget"
	runv1 "github.com/butler/butler/internal/gen/run/v1"
	"github.com/butler/butler/internal/ingress"
)

type Promoter struct {
	Store     *Store
	Goals     *goal.Manager
	Tasks     *task.Store
	Inputs    *durableinput.Store
	Budgets   *budget.Controller
	Allowance budget.ModelAllowance
}

// Promote atomically admits an ordinary task and ordered input. Observation
// content cannot supply authority, tools, credentials, model or budget policy.
// All locks on existing session/source/candidate rows precede authority/budget.
func (p *Promoter) Promote(ctx context.Context, id, ownerID string) (Candidate, error) {
	if p.Store == nil || p.Goals == nil || p.Tasks == nil || p.Inputs == nil || p.Budgets == nil {
		return Candidate{}, ErrInvalid
	}
	hint, err := p.Store.Get(ctx, id, ownerID)
	if err != nil {
		return hint, err
	}
	sourceHint, err := p.Store.GetSource(ctx, hint.SourceID, ownerID)
	if err != nil {
		return Candidate{}, err
	}
	tx, err := p.Store.pool.Begin(ctx)
	if err != nil {
		return Candidate{}, err
	}
	defer tx.Rollback(ctx)
	var storedOwner string
	if err = tx.QueryRow(ctx, `SELECT user_id FROM sessions WHERE session_key=$1 FOR UPDATE`, sourceHint.SessionKey).Scan(&storedOwner); err != nil {
		return Candidate{}, err
	}
	if storedOwner != ownerID {
		return Candidate{}, ErrConflict
	}
	source, err := scanSource(tx.QueryRow(ctx, `SELECT source_json,revision,enabled FROM initiative_sources WHERE source_id=$1 AND owner_id=$2 FOR UPDATE`, hint.SourceID, ownerID))
	if err != nil {
		return Candidate{}, err
	}
	if source.SessionKey != sourceHint.SessionKey {
		return Candidate{}, ErrConflict
	}
	c, err := scanCandidate(tx.QueryRow(ctx, `SELECT `+candidateColumns+` FROM initiatives WHERE initiative_id=$1 AND owner_id=$2 FOR UPDATE`, id, ownerID))
	if err != nil {
		return Candidate{}, err
	}
	var now time.Time
	if err = tx.QueryRow(ctx, `SELECT clock_timestamp()`).Scan(&now); err != nil {
		return Candidate{}, err
	}
	if c.TaskID != "" || (c.Status != "ready" && c.Status != "deferred") || c.NotBefore.After(now) {
		return c, tx.Commit(ctx)
	}
	finish := func(status, reason string, next time.Time) (Candidate, error) {
		c.Status, c.Reason, c.NotBefore = status, reason, next
		_, e := tx.Exec(ctx, `UPDATE initiatives SET status=$2,reason=$3,not_before=$4,updated_at=clock_timestamp() WHERE initiative_id=$1`, c.ID, status, reason, next)
		if e != nil {
			return c, e
		}
		return c, tx.Commit(ctx)
	}
	if !source.Enabled || source.Revision != c.SourceRevision || source.GoalID != c.GoalID || source.MandateID != c.MandateID {
		return finish("rejected", "source_changed", now)
	}
	// The installed source's latest durable condition is rechecked after locking
	// its row. A healthy state, newer resource revision, or old observation makes
	// the earlier candidate obsolete instead of manufacturing another task.
	var encoded []byte
	if err = tx.QueryRow(ctx, `SELECT event_json FROM initiative_events WHERE source_id=$1 AND verdict NOT IN ('duplicate_evidence','empty_check','stale_observation') ORDER BY observed_at DESC,recorded_at DESC,event_id DESC LIMIT 1`, source.ID).Scan(&encoded); err != nil {
		return c, err
	}
	var latest Event
	if json.Unmarshal(encoded, &latest) != nil || latest.Validate() != nil {
		return c, ErrInvalid
	}
	if latest.fingerprint(source) != c.Fingerprint || latest.Kind == "project_healthy" || c.LastObservedAt.Before(now.Add(-24*time.Hour)) {
		return finish("rejected", "condition_obsolete", now)
	}
	current, err := p.Store.currentConditionTx(ctx, tx, source, latest)
	if err != nil {
		return c, err
	}
	if !current {
		return finish("rejected", "condition_obsolete", now)
	}
	status, reason, next := Decide(source, latest, c.Occurrences, now)
	if status != "ready" {
		return finish(status, reason, next)
	}
	var lastPromoted *time.Time
	if err = tx.QueryRow(ctx, `SELECT last_promoted_at FROM initiative_sources WHERE source_id=$1`, source.ID).Scan(&lastPromoted); err != nil {
		return c, err
	}
	if lastPromoted != nil && now.Before(lastPromoted.Add(time.Duration(source.CooldownSeconds)*time.Second)) {
		return finish("deferred", "source_cooldown", lastPromoted.Add(time.Duration(source.CooldownSeconds)*time.Second))
	}
	day := now.UTC().Truncate(24 * time.Hour)
	if _, err = tx.Exec(ctx, `INSERT INTO initiative_attention_days(owner_id,day) VALUES($1,$2) ON CONFLICT DO NOTHING`, ownerID, day); err != nil {
		return c, err
	}
	var attentionUsed int
	if err = tx.QueryRow(ctx, `SELECT admitted_tasks FROM initiative_attention_days WHERE owner_id=$1 AND day=$2 FOR UPDATE`, ownerID, day).Scan(&attentionUsed); err != nil {
		return c, err
	}
	if attentionUsed >= 20 {
		return finish("deferred", "attention_budget_exhausted", day.Add(24*time.Hour))
	}
	a, err := p.Goals.CheckAdmissionTx(ctx, tx, goal.AdmissionRequest{OwnerID: ownerID, GoalID: source.GoalID, MandateID: source.MandateID, Purpose: source.Purpose, ObservationRef: c.ObservationRef, InitiativeReason: "Useful condition observed for configured project " + source.ProjectRef}, now)
	if err != nil {
		if errors.Is(err, actionauth.ErrOwnerAuthority) || errors.Is(err, goal.ErrConflict) || errors.Is(err, goal.ErrInvalid) {
			return finish("proposed", "authority_unavailable", now)
		}
		return c, err
	}
	m, _, err := actionauth.CheckAssignmentTx(ctx, tx, a, now)
	if err != nil {
		return c, err
	}
	b := m.Budget
	ms := budget.MandateScope{ID: m.ID, OwnerID: ownerID, Privacy: m.Privacy, Deadline: m.BudgetDeadline, Limits: budget.Units{ModelTurns: b.ModelTurns, Tokens: b.Tokens, CostMicros: b.CostMicros, ExternalCalls: b.ExternalCalls, Children: b.Children, ActiveMillis: b.ActiveMillis, EgressBytes: b.EgressBytes, ProviderInputBytes: b.ProviderInputBytes}}
	if err = p.Budgets.CheckRootAdmissionTx(ctx, tx, ownerID, ms, p.Allowance); err != nil {
		var exhausted *budget.ExhaustedError
		var loop *budget.LoopError
		switch {
		case errors.As(err, &exhausted):
			if exhausted.Kind == "owner_day" {
				return finish("deferred", "owner_day_budget_exhausted", now.UTC().Truncate(24*time.Hour).Add(24*time.Hour))
			}
			return finish("proposed", "execution_budget_exhausted", now)
		case errors.Is(err, budget.ErrUnknownCost):
			return finish("proposed", "model_cost_unknown", now)
		case errors.As(err, &loop):
			return finish("proposed", "model_privacy_denied", now)
		case errors.Is(err, budget.ErrConflict), errors.Is(err, budget.ErrInvalid):
			return finish("proposed", "budget_policy_unavailable", now)
		default:
			return c, err
		}
	}
	// Budget contention can cross an expiry or active-hours boundary. Recheck
	// time against PostgreSQL after those waits, reusing the held authority rows.
	if err = tx.QueryRow(ctx, `SELECT clock_timestamp()`).Scan(&now); err != nil {
		return c, err
	}
	if _, _, err = actionauth.CheckAssignmentTx(ctx, tx, a, now); err != nil {
		if errors.Is(err, actionauth.ErrOwnerAuthority) {
			return finish("proposed", "authority_unavailable", now)
		}
		return c, err
	}
	if current, err = p.Store.currentConditionTx(ctx, tx, source, latest); err != nil {
		return c, err
	} else if !current {
		return finish("rejected", "condition_obsolete", now)
	}
	record, err := p.Tasks.CreateTx(ctx, tx, task.CreateRequest{Authority: &a, InitialExecutionPolicy: source.ExecutionPolicy, SessionKey: source.SessionKey, OwnerID: ownerID, Goal: source.TaskGoal, Source: map[string]any{"type": "initiative", "initiative_id": c.ID, "observation_ref": c.ObservationRef, "project_ref": source.ProjectRef, "project_revision": source.ProjectRevision}, CompletionCriteria: source.CompletionCriteria})
	if err != nil {
		return c, err
	}
	payload, _ := json.Marshal(map[string]any{"user_id": ownerID, "task_id": record.TaskID, "text": source.TaskGoal, "initiative_id": c.ID, "observation_ref": c.ObservationRef})
	event := ingress.InputEvent{EventID: c.ID, EventType: runv1.InputEventType_INPUT_EVENT_TYPE_SCHEDULED_INTERNAL_EVENT, SessionKey: source.SessionKey, Source: "initiative", PayloadJSON: string(payload), CreatedAt: now, IdempotencyKey: c.ID}
	receipt, created, err := p.Inputs.AcceptTx(ctx, tx, event, ownerID, record.TaskID)
	if err != nil {
		return c, err
	}
	if !created {
		return c, ErrConflict
	}
	c.Status, c.Reason, c.TaskID, c.InputID = "promoted", "ordinary_task_admitted", record.TaskID, receipt.InputID
	if _, err = tx.Exec(ctx, `UPDATE initiatives SET status=$2,reason=$3,task_id=$4,input_id=$5,updated_at=clock_timestamp() WHERE initiative_id=$1`, c.ID, c.Status, c.Reason, c.TaskID, c.InputID); err != nil {
		return c, err
	}
	if _, err = tx.Exec(ctx, `UPDATE initiative_sources SET last_promoted_at=$2 WHERE source_id=$1`, source.ID, now); err != nil {
		return c, err
	}
	if _, err = tx.Exec(ctx, `UPDATE initiative_attention_days SET admitted_tasks=admitted_tasks+1 WHERE owner_id=$1 AND day=$2`, ownerID, day); err != nil {
		return c, err
	}
	return c, tx.Commit(ctx)
}

// Ready is bounded and deterministic; a busy project cannot starve older work.
func (s *Store) Ready(ctx context.Context, limit int) ([]Candidate, error) {
	if limit < 1 || limit > 100 {
		limit = 20
	}
	rows, err := s.pool.Query(ctx, `SELECT `+candidateColumns+` FROM initiatives WHERE status IN ('ready','deferred') AND not_before<=clock_timestamp() ORDER BY not_before,importance DESC,initiative_id LIMIT $1`, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	result := []Candidate{}
	for rows.Next() {
		c, err := scanCandidate(rows)
		if err != nil {
			return nil, err
		}
		result = append(result, c)
	}
	return result, rows.Err()
}
