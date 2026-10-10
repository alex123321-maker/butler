package goal

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/butler/butler/apps/orchestrator/internal/task"
	"github.com/butler/butler/internal/actionauth"
	"github.com/butler/butler/internal/owneridentity"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

type Manager struct {
	pool     *pgxpool.Pool
	controls *task.Controller
	tasks    *task.Store
}

func New(pool *pgxpool.Pool, tasks *task.Store, controls *task.Controller) *Manager {
	return &Manager{pool: pool, tasks: tasks, controls: controls}
}
func (m *Manager) SetControls(controls *task.Controller) { m.controls = controls }
func newID(prefix string) string {
	var b [16]byte
	if _, err := rand.Read(b[:]); err != nil {
		panic("secure owner authority identity unavailable")
	}
	return prefix + "_" + hex.EncodeToString(b[:])
}
func fingerprint(v any) string {
	b, _ := json.Marshal(v)
	h := sha256.Sum256(b)
	return hex.EncodeToString(h[:])
}
func actor(ctx context.Context) (owneridentity.Actor, error) {
	a, ok := owneridentity.FromContext(ctx)
	if !ok || len(a.DeliveryID) > 512 {
		return a, ErrUnauthorized
	}
	return a, nil
}

func beginDecision(ctx context.Context, pool *pgxpool.Pool, a owneridentity.Actor, hash string) (pgx.Tx, *Receipt, error) {
	tx, err := pool.Begin(ctx)
	if err != nil {
		return nil, nil, err
	}
	if _, err = tx.Exec(ctx, `SELECT pg_advisory_xact_lock(hashtextextended($1,0))`, fingerprint([]string{a.OwnerID, a.Channel, a.DeliveryID})); err != nil {
		tx.Rollback(ctx)
		return nil, nil, err
	}
	var r Receipt
	var oldHash string
	err = tx.QueryRow(ctx, `SELECT e.event_id,e.subject_id,e.revision,e.request_hash,COALESCE(j.state,'applied') FROM owner_authority_events e LEFT JOIN owner_authority_jobs j USING(event_id) WHERE e.owner_id=$1 AND e.source=$2 AND e.delivery_id=$3`, a.OwnerID, a.Channel, a.DeliveryID).Scan(&r.EventID, &r.SubjectID, &r.Revision, &oldHash, &r.Propagation)
	if err == nil {
		if hash != oldHash {
			tx.Rollback(ctx)
			return nil, nil, ErrConflict
		}
		return tx, &r, nil
	}
	if !errors.Is(err, pgx.ErrNoRows) {
		tx.Rollback(ctx)
		return nil, nil, err
	}
	return tx, nil, nil
}

func saveDecision(ctx context.Context, tx pgx.Tx, a owneridentity.Actor, kind, id string, revision int64, hash string, snapshot any, propagate, revoked bool) (Receipt, error) {
	r := Receipt{EventID: newID("authority"), SubjectID: id, Revision: revision, Propagation: "applied"}
	data, _ := json.Marshal(snapshot)
	_, err := tx.Exec(ctx, `INSERT INTO owner_authority_events(event_id,owner_id,source,delivery_id,subject_kind,subject_id,revision,request_hash,snapshot) VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9::jsonb)`, r.EventID, a.OwnerID, a.Channel, a.DeliveryID, kind, id, revision, hash, string(data))
	if err != nil {
		return r, err
	}
	if propagate {
		control := "pause"
		if revoked {
			control = "stop"
		}
		_, err = tx.Exec(ctx, `INSERT INTO owner_authority_jobs(event_id,owner_id,subject_kind,subject_id,current_revision,control_kind) VALUES($1,$2,$3,$4,$5,$6)`, r.EventID, a.OwnerID, kind, id, revision, control)
		r.Propagation = "pending"
	}
	return r, err
}

func validateGoal(g actionauth.GoalPolicy, now time.Time) error {
	if strings.TrimSpace(g.Title) == "" || len(g.Title) > 8192 || len(g.Constraints) > 64 || g.Purpose == "" || len(g.Purpose) > 128 || strings.ContainsAny(g.Purpose, "*? \n\r") || (g.Status == "active" || g.Status == "proposed") && !g.ExpiresAt.After(now) || g.ExpiresAt.After(now.Add(366*24*time.Hour)) {
		return ErrInvalid
	}
	switch g.Kind {
	case "objective", "preference", "constraint":
		if g.Status != "active" && g.Status != "paused" && g.Status != "revoked" {
			return ErrInvalid
		}
	case "assumption":
		if g.Status != "proposed" {
			return ErrInvalid
		}
	default:
		return ErrInvalid
	}
	for _, c := range g.Constraints {
		if strings.TrimSpace(c) == "" || len(c) > 8192 {
			return ErrInvalid
		}
	}
	return nil
}

func (m *Manager) PutGoal(ctx context.Context, req GoalRequest) (Receipt, error) {
	a, err := actor(ctx)
	if err != nil {
		return Receipt{}, err
	}
	g := req.Goal
	if g.OwnerID != "" && g.OwnerID != a.OwnerID {
		return Receipt{}, ErrUnauthorized
	}
	hash := fingerprint(req)
	tx, replay, err := beginDecision(ctx, m.pool, a, hash)
	if err != nil {
		return Receipt{}, err
	}
	defer tx.Rollback(ctx)
	if replay != nil {
		return *replay, tx.Commit(ctx)
	}
	g.ExpiresAt = g.ExpiresAt.UTC().Truncate(time.Microsecond)
	g.Policies = nil
	if err = validateGoal(g, time.Now()); err != nil {
		return Receipt{}, err
	}
	if g.ID == "" {
		if req.ExpectedRevision != 0 {
			return Receipt{}, ErrConflict
		}
		g.ID = newID("goal")
	} else {
		// Lock the complete mandate set in its stable order, then the goal. Mutation
		// never takes session/task/worker locks while retaining these authority rows.
		rows, e := tx.Query(ctx, `SELECT mandate_id FROM action_mandates WHERE goal_id=$1 ORDER BY mandate_id FOR UPDATE`, g.ID)
		if e != nil {
			return Receipt{}, e
		}
		for rows.Next() {
			var id string
			if e = rows.Scan(&id); e != nil {
				rows.Close()
				return Receipt{}, e
			}
		}
		e = rows.Err()
		rows.Close()
		if e != nil {
			return Receipt{}, e
		}
		var old actionauth.GoalPolicy
		var data []byte
		var owner, kind, status string
		var revision int64
		err = tx.QueryRow(ctx, `SELECT owner_id,kind,status,revision,goal_data FROM owner_goals WHERE goal_id=$1 FOR UPDATE`, g.ID).Scan(&owner, &kind, &status, &revision, &data)
		if err != nil {
			return Receipt{}, ErrConflict
		}
		if owner != a.OwnerID {
			return Receipt{}, ErrUnauthorized
		}
		if revision != req.ExpectedRevision || kind != g.Kind || status == "revoked" {
			return Receipt{}, ErrConflict
		}
		if json.Unmarshal(data, &old) != nil {
			return Receipt{}, ErrInvalid
		}
		if (g.Kind == "preference" || g.Kind == "constraint") && g.Purpose != old.Purpose {
			return Receipt{}, ErrConflict
		}
	}
	if req.ParentRef != "" {
		var kind, owner string
		if err = tx.QueryRow(ctx, `SELECT kind,owner_id FROM owner_goals WHERE goal_id=$1`, req.ParentRef).Scan(&kind, &owner); err != nil || kind != "assumption" || owner != a.OwnerID {
			return Receipt{}, ErrInvalid
		}
	}
	g.OwnerID = a.OwnerID
	g.Revision = req.ExpectedRevision + 1
	source := Source{a.OwnerID, a.Channel, a.DeliveryID, req.ParentRef}
	data, _ := json.Marshal(g)
	src, _ := json.Marshal(source)
	_, err = tx.Exec(ctx, `INSERT INTO owner_goals(goal_id,owner_id,kind,status,revision,expires_at,goal_data,source_data) VALUES($1,$2,$3,$4,$5,$6,$7::jsonb,$8::jsonb) ON CONFLICT(goal_id) DO UPDATE SET status=EXCLUDED.status,revision=EXCLUDED.revision,expires_at=EXCLUDED.expires_at,goal_data=EXCLUDED.goal_data,source_data=EXCLUDED.source_data,updated_at=clock_timestamp()`, g.ID, g.OwnerID, g.Kind, g.Status, g.Revision, g.ExpiresAt, string(data), string(src))
	if err != nil {
		return Receipt{}, err
	}
	if _, err = tx.Exec(ctx, `INSERT INTO owner_purpose_policies(owner_id,purpose) VALUES($1,$2) ON CONFLICT DO NOTHING`, a.OwnerID, g.Purpose); err != nil {
		return Receipt{}, err
	}
	var policyRevision int64
	isPolicy := g.Kind == "preference" || g.Kind == "constraint"
	if isPolicy {
		if err = tx.QueryRow(ctx, `UPDATE owner_purpose_policies SET revision=revision+1 WHERE owner_id=$1 AND purpose=$2 RETURNING revision`, a.OwnerID, g.Purpose).Scan(&policyRevision); err != nil {
			return Receipt{}, err
		}
	}
	r, err := saveDecision(ctx, tx, a, "goal", g.ID, g.Revision, hash, Record{GoalPolicy: g, Source: source}, req.ExpectedRevision > 0 || isPolicy, g.Status == "revoked" && !isPolicy)
	if err != nil {
		return r, err
	}
	if isPolicy {
		if _, err = tx.Exec(ctx, `UPDATE owner_authority_jobs SET subject_kind='policy',subject_id=$2,current_revision=$3 WHERE event_id=$1`, r.EventID, g.Purpose, policyRevision); err != nil {
			return r, err
		}
	}
	return r, tx.Commit(ctx)
}

func (m *Manager) PutMandate(ctx context.Context, req MandateRequest) (Receipt, error) {
	a, err := actor(ctx)
	if err != nil {
		return Receipt{}, err
	}
	mandate := req.Mandate
	if mandate.OwnerID != "" && mandate.OwnerID != a.OwnerID {
		return Receipt{}, ErrUnauthorized
	}
	hash := fingerprint(req)
	tx, replay, err := beginDecision(ctx, m.pool, a, hash)
	if err != nil {
		return Receipt{}, err
	}
	defer tx.Rollback(ctx)
	if replay != nil {
		return *replay, tx.Commit(ctx)
	}
	mandate.ExpiresAt = mandate.ExpiresAt.UTC().Truncate(time.Microsecond)
	if mandate.ID == "" {
		if req.ExpectedRevision != 0 {
			return Receipt{}, ErrConflict
		}
		mandate.ID = newID("mandate")
		mandate.BudgetDeadline = mandate.ExpiresAt
	} else {
		var data []byte
		var owner, scope, status string
		var rev int64
		err = tx.QueryRow(ctx, `SELECT owner_id,scope_kind,status,revision,mandate_data FROM action_mandates WHERE mandate_id=$1 FOR UPDATE`, mandate.ID).Scan(&owner, &scope, &status, &rev, &data)
		if err != nil || scope != "owner_goal" || rev != req.ExpectedRevision || status == "revoked" {
			return Receipt{}, ErrConflict
		}
		if owner != a.OwnerID {
			return Receipt{}, ErrUnauthorized
		}
		var old actionauth.Mandate
		if json.Unmarshal(data, &old) != nil {
			return Receipt{}, ErrInvalid
		}
		// Revision changes do not create fresh allowance or widen an immutable
		// budget period. A new owner grant requires a separately visible mandate.
		if old.GoalID != mandate.GoalID || old.Budget != mandate.Budget {
			return Receipt{}, ErrConflict
		}
		mandate.BudgetDeadline = old.BudgetDeadline
	}
	var owner, kind, status, purpose string
	var goalRevision int64
	var expiry time.Time
	var goalData []byte
	err = tx.QueryRow(ctx, `SELECT owner_id,kind,status,revision,expires_at,goal_data FROM owner_goals WHERE goal_id=$1 FOR UPDATE`, mandate.GoalID).Scan(&owner, &kind, &status, &goalRevision, &expiry, &goalData)
	if err != nil || owner != a.OwnerID || kind != "objective" || mandate.Status == "active" && (status != "active" || !expiry.After(time.Now()) || goalRevision != mandate.GoalRevision) {
		return Receipt{}, actionauth.ErrOwnerAuthority
	}
	var g actionauth.GoalPolicy
	if json.Unmarshal(goalData, &g) != nil {
		return Receipt{}, ErrInvalid
	}
	purpose = g.Purpose
	if mandate.Status == "active" && (mandate.Purpose != purpose || mandate.ExpiresAt.After(expiry)) {
		return Receipt{}, ErrInvalid
	}
	mandate.OwnerID = a.OwnerID
	mandate.ScopeKind = "owner_goal"
	mandate.TaskID = ""
	mandate.TaskRevision = 0
	mandate.ConstraintsRevision = 0
	mandate.Revision = req.ExpectedRevision + 1
	mandate.Source = map[string]string{"author": a.OwnerID, "channel": a.Channel, "delivery_id": a.DeliveryID}
	if err = actionauth.ValidateOwnerMandate(mandate, time.Now()); err != nil {
		return Receipt{}, err
	}
	data, _ := json.Marshal(mandate)
	_, err = tx.Exec(ctx, `INSERT INTO action_mandates(mandate_id,owner_id,task_id,revision,mandate_data,expires_at,scope_kind,goal_id,goal_revision,status,revoked_at) VALUES($1,$2,'',$3,$4::jsonb,$5,'owner_goal',$6,$7,$8,CASE WHEN $8='revoked' THEN clock_timestamp() ELSE NULL END) ON CONFLICT(mandate_id) DO UPDATE SET revision=EXCLUDED.revision,mandate_data=EXCLUDED.mandate_data,expires_at=EXCLUDED.expires_at,goal_revision=EXCLUDED.goal_revision,status=EXCLUDED.status,revoked_at=EXCLUDED.revoked_at`, mandate.ID, mandate.OwnerID, mandate.Revision, string(data), mandate.ExpiresAt, mandate.GoalID, mandate.GoalRevision, mandate.Status)
	if err != nil {
		return Receipt{}, err
	}
	r, err := saveDecision(ctx, tx, a, "mandate", mandate.ID, mandate.Revision, hash, mandate, req.ExpectedRevision > 0, mandate.Status == "revoked")
	if err != nil {
		return r, err
	}
	return r, tx.Commit(ctx)
}

// CheckAdmissionTx is a read/lock boundary for an already trusted scheduler.
// It requires initiative permission. Callers lock ownership/session first, then
// insert new task/input rows in the same transaction. It never acquires tasks.
func (m *Manager) CheckAdmissionTx(ctx context.Context, tx pgx.Tx, req AdmissionRequest, at time.Time) (actionauth.Assignment, error) {
	return m.checkTaskAdmissionTx(ctx, tx, req, at, true)
}

// requireInitiative is service-owned: only CreateOwnerTask may choose false,
// after proving an authenticated owner request. It is never an ingress field.
func (m *Manager) checkTaskAdmissionTx(ctx context.Context, tx pgx.Tx, req AdmissionRequest, at time.Time, requireInitiative bool) (actionauth.Assignment, error) {
	a := actionauth.Assignment{OwnerID: req.OwnerID, GoalID: req.GoalID, Purpose: req.Purpose, InitiativeReason: req.InitiativeReason, ObservationRef: req.ObservationRef}
	if req.OwnerID == "" || req.GoalID == "" || req.Purpose == "" || strings.TrimSpace(req.InitiativeReason) == "" || strings.TrimSpace(req.ObservationRef) == "" || len(req.InitiativeReason) > 8192 || len(req.ObservationRef) > 512 {
		return a, ErrInvalid
	}
	rows, err := tx.Query(ctx, `SELECT mandate_id,revision,goal_revision FROM action_mandates WHERE goal_id=$1 AND owner_id=$2 AND scope_kind='owner_goal' AND status='active' AND revoked_at IS NULL AND expires_at>$3 ORDER BY mandate_id`, req.GoalID, req.OwnerID, at)
	if err != nil {
		return a, err
	}
	var candidates []actionauth.Assignment
	for rows.Next() {
		c := a
		if err = rows.Scan(&c.MandateID, &c.MandateRevision, &c.GoalRevision); err != nil {
			rows.Close()
			return a, err
		}
		candidates = append(candidates, c)
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		return a, err
	}
	if req.MandateID == "" {
		if len(candidates) != 1 {
			return a, ErrConflict
		}
		a = candidates[0]
	} else {
		for _, c := range candidates {
			if c.MandateID == req.MandateID {
				a = c
			}
		}
		if a.MandateID == "" {
			return a, actionauth.ErrOwnerAuthority
		}
	}
	if err = tx.QueryRow(ctx, `SELECT revision FROM owner_purpose_policies WHERE owner_id=$1 AND purpose=$2`, a.OwnerID, a.Purpose).Scan(&a.PolicyRevision); err != nil {
		return a, err
	}
	mandate, _, err := actionauth.CheckAssignmentTx(ctx, tx, a, at)
	if err != nil {
		return a, err
	}
	if requireInitiative && !mandate.AllowInitiative {
		return a, actionauth.ErrOwnerAuthority
	}
	return a, nil
}

func (m *Manager) ReadTaskAuthority(ctx context.Context, taskID string) (actionauth.Assignment, actionauth.Mandate, actionauth.GoalPolicy, error) {
	tx, err := m.pool.Begin(ctx)
	if err != nil {
		return actionauth.Assignment{}, actionauth.Mandate{}, actionauth.GoalPolicy{}, err
	}
	defer tx.Rollback(ctx)
	a, err := actionauth.GetTaskAssignment(ctx, tx, taskID)
	if err != nil || a.TaskID == "" {
		return a, actionauth.Mandate{}, actionauth.GoalPolicy{}, err
	}
	mandate, g, err := actionauth.CheckAssignmentTx(ctx, tx, a, time.Now())
	return a, mandate, g, err
}

func (m *Manager) String() string { return fmt.Sprintf("owner goal authority") }
