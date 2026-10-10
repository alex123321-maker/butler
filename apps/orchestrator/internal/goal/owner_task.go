package goal

import (
	"context"
	"errors"
	"regexp"
	"time"

	"github.com/butler/butler/apps/orchestrator/internal/task"
	"github.com/jackc/pgx/v5"
)

var projectDigest = regexp.MustCompile("^[a-f0-9]{64}$")
var projectReference = regexp.MustCompile("^[a-zA-Z0-9][a-zA-Z0-9_.:/-]{0,199}$")

type TaskRequest struct {
	SessionKey         string   `json:"session_key"`
	GoalID             string   `json:"goal_id"`
	MandateID          string   `json:"mandate_id"`
	Purpose            string   `json:"purpose"`
	ProjectRef         string   `json:"project_ref,omitempty"`
	ProjectRevision    string   `json:"project_revision,omitempty"`
	Goal               string   `json:"goal"`
	Constraints        []string `json:"constraints"`
	CompletionCriteria []string `json:"completion_criteria"`
	InitiativeReason   string   `json:"initiative_reason"`
	ObservationRef     string   `json:"observation_ref"`
}

// CreateOwnerTask is an authenticated manual admission surface. Automated
// initiative coordinators instead use CheckAdmissionTx + task.CreateTx in their
// own atomic candidate/input/budget transaction.
func (m *Manager) CreateOwnerTask(ctx context.Context, req TaskRequest) (task.Record, error) {
	if (req.ProjectRef == "") != (req.ProjectRevision == "") || (req.ProjectRef != "" && !projectReference.MatchString(req.ProjectRef)) || (req.ProjectRevision != "" && !projectDigest.MatchString(req.ProjectRevision)) {
		return task.Record{}, ErrInvalid
	}
	a, err := actor(ctx)
	if err != nil {
		return task.Record{}, err
	}
	tx, err := m.pool.Begin(ctx)
	if err != nil {
		return task.Record{}, err
	}
	defer tx.Rollback(ctx)
	// Same delivery creates one task. Advisory key is independent of authority
	// and never acquired by dispatch/result paths.
	if _, err = tx.Exec(ctx, `SELECT pg_advisory_xact_lock(hashtextextended($1,0))`, fingerprint([]string{"task", a.OwnerID, a.Channel, a.DeliveryID})); err != nil {
		return task.Record{}, err
	}
	var oldHash, taskID string
	err = tx.QueryRow(ctx, `SELECT source_json->>'owner_request_hash',task_id FROM tasks WHERE owner_id=$1 AND source_json->>'owner_delivery_id'=$2`, a.OwnerID, a.Channel+":"+a.DeliveryID).Scan(&oldHash, &taskID)
	if err == nil {
		if oldHash != fingerprint(req) {
			return task.Record{}, ErrConflict
		}
		return m.tasks.GetTx(ctx, tx, taskID)
	}
	if !errors.Is(err, pgx.ErrNoRows) {
		return task.Record{}, err
	}
	var owner string
	if _, err = tx.Exec(ctx, `INSERT INTO session_ownership(session_key) VALUES($1) ON CONFLICT(session_key) DO NOTHING`, req.SessionKey); err != nil {
		return task.Record{}, err
	}
	if err = tx.QueryRow(ctx, `SELECT session_key FROM session_ownership WHERE session_key=$1 FOR SHARE`, req.SessionKey).Scan(&taskID); err != nil {
		return task.Record{}, err
	}
	if err = tx.QueryRow(ctx, `SELECT user_id FROM sessions WHERE session_key=$1 FOR SHARE`, req.SessionKey).Scan(&owner); err != nil || owner != a.OwnerID {
		return task.Record{}, ErrUnauthorized
	}
	// Explicit owner work needs the same current exact assignment, but is not a
	// spontaneous initiative. Only this authenticated surface may skip that flag.
	assignment, err := m.checkTaskAdmissionTx(ctx, tx, AdmissionRequest{OwnerID: a.OwnerID, GoalID: req.GoalID, MandateID: req.MandateID, Purpose: req.Purpose, ObservationRef: req.ObservationRef, InitiativeReason: req.InitiativeReason}, time.Now(), false)
	if err != nil {
		return task.Record{}, err
	}
	source := map[string]any{"type": "owner_goal_task", "owner_delivery_id": a.Channel + ":" + a.DeliveryID, "owner_request_hash": fingerprint(req), "channel": a.Channel}
	if req.ProjectRef != "" {
		source["project_ref"], source["project_revision"] = req.ProjectRef, req.ProjectRevision
	}
	r, err := m.tasks.CreateTx(ctx, tx, task.CreateRequest{SessionKey: req.SessionKey, OwnerID: a.OwnerID, Goal: req.Goal, Constraints: req.Constraints, CompletionCriteria: req.CompletionCriteria, Authority: &assignment, Source: source})
	if err != nil {
		return r, err
	}
	return r, tx.Commit(ctx)
}
