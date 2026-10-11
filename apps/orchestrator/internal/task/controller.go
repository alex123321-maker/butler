package task

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"github.com/butler/butler/internal/actionauth"
	"sort"
	"strings"
	"time"

	runv1 "github.com/butler/butler/internal/gen/run/v1"
	"github.com/butler/butler/internal/ingress"
	"github.com/butler/butler/internal/owneridentity"
	"github.com/butler/butler/internal/ownership"
	"github.com/jackc/pgx/v5"
)

var ErrUnauthorized = errors.New("authenticated task owner required")

type ControlLeases interface {
	RevokeForControlTx(context.Context, pgx.Tx, []string) error
}
type ControlRuns interface {
	LockForControlTx(context.Context, pgx.Tx, []string) error
	CancelForControlTx(context.Context, pgx.Tx, []string, string) error
}
type ControlApprovals interface {
	LockForControlTx(context.Context, pgx.Tx, []string) error
	InvalidateForControlTx(context.Context, pgx.Tx, []string, string, string) error
}
type ControlInputs interface {
	InvalidateForControlTx(context.Context, pgx.Tx, []string, string) (bool, error)
	AdmitControlTx(context.Context, pgx.Tx, ingress.InputEvent, string, string, string, string, string) (string, error)
	ResolveAmbiguousForControlTx(context.Context, pgx.Tx, []string, string) error
}
type ControlTranscript interface {
	AppendOwnerControlTx(context.Context, pgx.Tx, string, string, string, map[string]any) error
}
type Controller struct {
	store      *Store
	authority  *ownership.Authority
	leases     ControlLeases
	runs       ControlRuns
	approvals  ControlApprovals
	inputs     ControlInputs
	transcript ControlTranscript
}

func NewController(store *Store, authority *ownership.Authority, leases ControlLeases, runs ControlRuns, approvals ControlApprovals, inputs ControlInputs, transcript ControlTranscript) *Controller {
	return &Controller{store: store, authority: authority, leases: leases, runs: runs, approvals: approvals, inputs: inputs, transcript: transcript}
}

// ResolveTarget never guesses between active owner tasks. Channels share the
// authenticated owner identity; a task reference is not proof of ownership.
func (c *Controller) ResolveTarget(ctx context.Context, id string) (Record, error) {
	actor, ok := owneridentity.FromContext(ctx)
	if !ok {
		return Record{}, ErrUnauthorized
	}
	if id != "" {
		r, err := c.store.Get(ctx, id)
		if err != nil {
			return r, err
		}
		if r.OwnerID != actor.OwnerID {
			return Record{}, ErrUnauthorized
		}
		return r, nil
	}
	rows, err := c.store.pool.Query(ctx, `SELECT `+columns+` FROM tasks WHERE owner_id=$1 AND status NOT IN ('completed','cancelled') AND parent_task_id IS NULL ORDER BY created_at LIMIT 2`, actor.OwnerID)
	if err != nil {
		return Record{}, err
	}
	defer rows.Close()
	var matches []Record
	for rows.Next() {
		r, err := scan(rows)
		if err != nil {
			return r, err
		}
		matches = append(matches, r)
	}
	if err = rows.Err(); err != nil {
		return Record{}, err
	}
	if len(matches) != 1 {
		return Record{}, ErrConflict
	}
	return matches[0], nil
}

func (c *Controller) DeliveryTarget(ctx context.Context) (string, error) {
	actor, ok := owneridentity.FromContext(ctx)
	if !ok {
		return "", ErrUnauthorized
	}
	var id string
	err := c.store.pool.QueryRow(ctx, `SELECT task_id FROM task_control_events WHERE owner_id=$1 AND source=$2 AND delivery_id=$3`, actor.OwnerID, actor.Channel, actor.DeliveryID).Scan(&id)
	if errors.Is(err, pgx.ErrNoRows) {
		return "", nil
	}
	return id, err
}

// ControlGuard is a trusted adapter's current-source check. It uses only the
// caller transaction; it must not acquire another pool connection or do IPC.
type ControlGuard func(context.Context, pgx.Tx) error

func (c *Controller) Apply(ctx context.Context, req ControlRequest) (ControlResult, error) {
	return c.ApplyWithGuard(ctx, req, nil)
}

func (c *Controller) ApplyWithGuard(ctx context.Context, req ControlRequest, guard ControlGuard) (ControlResult, error) {
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	var result ControlResult
	actor, ok := owneridentity.FromContext(ctx)
	if !ok {
		return result, ErrUnauthorized
	}
	if err := ValidateControl(req); err != nil {
		return result, err
	}
	release, err := c.authority.BeginMutation(ctx)
	if err != nil {
		return result, err
	}
	defer release()
	tx, err := c.store.pool.Begin(ctx)
	if err != nil {
		return result, err
	}
	defer func() {
		cleanup, done := context.WithTimeout(context.Background(), 5*time.Second)
		defer done()
		_ = tx.Rollback(cleanup)
	}()
	initial, err := scan(tx.QueryRow(ctx, `SELECT `+columns+` FROM tasks WHERE task_id=$1`, req.TaskID))
	if err != nil {
		return result, err
	}
	if initial.OwnerID != actor.OwnerID {
		return result, ErrUnauthorized
	}
	// Stabilize the entire branch before collecting its independent execution
	// sessions. Every child admission takes this tree barrier before session locks.
	if _, err = lockTreeTx(ctx, tx, initial.TaskID); err != nil {
		return result, err
	}
	rows, err := tx.Query(ctx, `WITH RECURSIVE tree AS (
 SELECT task_id,session_key FROM tasks WHERE task_id=$1
 UNION SELECT t.task_id,t.session_key FROM tasks t JOIN tree b ON t.parent_task_id=b.task_id
) SELECT DISTINCT session_key FROM tree ORDER BY session_key`, initial.TaskID)
	if err != nil {
		return result, err
	}
	sessions := []string{}
	for rows.Next() {
		var key string
		if err = rows.Scan(&key); err != nil {
			rows.Close()
			return result, err
		}
		sessions = append(sessions, key)
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		return result, err
	}
	if actor.SessionKey != "" && actor.SessionKey != initial.SessionKey {
		sessions = append(sessions, actor.SessionKey)
	}
	sort.Strings(sessions)
	for _, key := range sessions {
		if _, err = tx.Exec(ctx, `INSERT INTO session_ownership(session_key) VALUES($1) ON CONFLICT(session_key) DO NOTHING`, key); err != nil {
			return result, err
		}
		var locked string
		if err = tx.QueryRow(ctx, `SELECT session_key FROM session_ownership WHERE session_key=$1 FOR UPDATE`, key).Scan(&locked); err != nil {
			return result, err
		}
	}
	for _, key := range sessions {
		var owner string
		if err = tx.QueryRow(ctx, `SELECT user_id FROM sessions WHERE session_key=$1 FOR UPDATE`, key).Scan(&owner); err != nil {
			return result, err
		}
		if owner != actor.OwnerID {
			return result, ErrUnauthorized
		}
	}
	var oldRequest, oldResult []byte
	err = tx.QueryRow(ctx, `SELECT request_json,result_json FROM task_control_events WHERE owner_id=$1 AND source=$2 AND delivery_id=$3`, actor.OwnerID, actor.Channel, actor.DeliveryID).Scan(&oldRequest, &oldResult)
	if err == nil {
		if canonicalJSON(string(oldRequest)) != canonicalJSON(ControlFingerprint(req)) {
			return result, ErrConflict
		}
		if err = json.Unmarshal(oldResult, &result); err != nil {
			return result, err
		}
		if guard != nil {
			if err = guard(ctx, tx); err != nil {
				return ControlResult{}, err
			}
		}
		return result, tx.Commit(ctx)
	}
	if !errors.Is(err, pgx.ErrNoRows) {
		return result, err
	}
	rows, err = tx.Query(ctx, `WITH RECURSIVE tree AS(SELECT task_id FROM tasks WHERE task_id=$1 UNION ALL SELECT t.task_id FROM tasks t JOIN tree ON t.parent_task_id=tree.task_id) SELECT t.task_id FROM tasks t JOIN tree USING(task_id) WHERE t.status NOT IN ('completed','cancelled') ORDER BY t.task_id`, req.TaskID)
	if err != nil {
		return result, err
	}
	ids := []string{}
	for rows.Next() {
		var id string
		if err = rows.Scan(&id); err != nil {
			rows.Close()
			return result, err
		}
		ids = append(ids, id)
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		return result, err
	}
	if len(ids) == 0 {
		return result, ErrConflict
	}
	// Linearization with Broker claim: ownership first, then approval/run rows,
	// then task rows. No RPC or external operation is performed in this transaction.
	if err = c.approvals.LockForControlTx(ctx, tx, ids); err != nil {
		return result, err
	}
	if err = c.runs.LockForControlTx(ctx, tx, ids); err != nil {
		return result, err
	}
	records := make(map[string]Record, len(ids))
	for _, id := range ids {
		r, err := scan(tx.QueryRow(ctx, `SELECT `+columns+` FROM tasks WHERE task_id=$1 FOR UPDATE`, id))
		if err != nil {
			return result, err
		}
		records[id] = r
	}
	root := records[req.TaskID]
	if root.OwnerID != actor.OwnerID || root.Status == "completed" || root.Status == "cancelled" || (req.ExpectedRevision != 0 && root.Revision != req.ExpectedRevision) {
		return result, ErrConflict
	}
	if guard != nil {
		if err = guard(ctx, tx); err != nil {
			return ControlResult{}, err
		}
	}
	// Ownership rows are already locked. No durable revocation precedes the
	// adapter's source proof; a denied guard rolls back the complete operation.
	if err = c.leases.RevokeForControlTx(ctx, tx, ids); err != nil {
		return result, err
	}
	status := "active"
	switch req.Kind {
	case ControlStop:
		status = "cancelled"
	case ControlPause:
		status = "paused"
	case ControlResume:
		if root.Status != "paused" && root.Status != "blocked" && root.Status != "waiting_for_user" {
			return result, ErrConflict
		}
	case ControlStatus:
		status = req.Status
		if err = ValidateTransition(root.Status, status); err != nil {
			return result, err
		}
	}
	if status == "completed" {
		verified, err := scanResult(tx.QueryRow(ctx, `SELECT `+resultColumns+` FROM task_results WHERE task_id=$1 AND result_id=$2`, root.TaskID, req.ResultID))
		if err != nil {
			return result, err
		}
		if verified.Verdict != "verified" {
			return result, ErrInvalid
		}
		if err = ValidateVerification(root, verified); err != nil {
			return result, err
		}
	}
	if status == "active" {
		var inactiveAncestor bool
		err = tx.QueryRow(ctx, `WITH RECURSIVE parents AS(SELECT parent_task_id FROM tasks WHERE task_id=$1 UNION ALL SELECT t.parent_task_id FROM tasks t JOIN parents p ON t.task_id=p.parent_task_id) SELECT EXISTS(SELECT 1 FROM tasks WHERE task_id IN(SELECT parent_task_id FROM parents) AND status<>'active')`, root.TaskID).Scan(&inactiveAncestor)
		if err != nil {
			return result, err
		}
		if inactiveAncestor {
			return result, ErrConflict
		}
		if _, err = actionauth.CheckTaskAuthorityTx(ctx, tx, root.TaskID, root.OwnerID, nil, time.Now()); err != nil {
			return result, err
		}
	}
	controlID, err := newID("control")
	if err != nil {
		return result, err
	}
	uncertain, err := c.inputs.InvalidateForControlTx(ctx, tx, ids, controlID)
	if err != nil {
		return result, err
	}
	if err = c.inputs.ResolveAmbiguousForControlTx(ctx, tx, sessions, actor.OwnerID); err != nil {
		return result, err
	}
	if err = c.approvals.InvalidateForControlTx(ctx, tx, ids, controlID, actor.OwnerID); err != nil {
		return result, err
	}
	if err = c.runs.CancelForControlTx(ctx, tx, ids, controlID); err != nil {
		return result, err
	}
	result = ControlResult{ControlID: controlID, AffectedTaskIDs: ids, EffectBoundary: "previous_external_effects_preserved", AppliedAt: time.Now().UTC()}
	if uncertain {
		result.EffectBoundary = "dispatch_may_have_committed_reconciliation_required"
	}
	requestedJSON := ControlFingerprint(req)
	_, err = tx.Exec(ctx, `INSERT INTO task_control_events(control_id,task_id,owner_id,source,delivery_id,kind,request_json,result_json,revision_before,revision_after) VALUES($1,$2,$3,$4,$5,$6,$7::jsonb,'{}',$8::bigint,$8::bigint+1)`, controlID, root.TaskID, actor.OwnerID, actor.Channel, actor.DeliveryID, req.Kind, requestedJSON, root.Revision)
	if err != nil {
		return result, err
	}
	for _, id := range ids {
		r := records[id]
		nextStatus := status
		if id != root.TaskID && status != "cancelled" {
			nextStatus = "paused"
		}
		if id == root.TaskID {
			if req.Goal != nil {
				r.Goal = *req.Goal
			}
			if req.Constraints != nil {
				r.Constraints = append([]string{}, (*req.Constraints)...)
			}
			if req.CompletionCriteria != nil {
				r.CompletionCriteria = append([]string{}, (*req.CompletionCriteria)...)
			}
			if req.ExecutionPolicy != "" {
				r.ExecutionPolicy = req.ExecutionPolicy
			}
		}
		if req.Kind == ControlAmend && req.Instruction != "" {
			r.Constraints = appendUnique(r.Constraints, "Owner amendment: "+req.Instruction)
		}
		if req.ExecutionPolicy == PolicyPrepareOnly {
			r.ExecutionPolicy = PolicyPrepareOnly
		}
		// A child cannot weaken any currently active ancestor restriction.
		var restricted bool
		if err = tx.QueryRow(ctx, `WITH RECURSIVE parents AS(SELECT parent_task_id FROM tasks WHERE task_id=$1 UNION ALL SELECT t.parent_task_id FROM tasks t JOIN parents p ON t.task_id=p.parent_task_id) SELECT EXISTS(SELECT 1 FROM tasks WHERE task_id IN(SELECT parent_task_id FROM parents) AND execution_policy='prepare_only')`, id).Scan(&restricted); err != nil {
			return result, err
		}
		if restricted {
			r.ExecutionPolicy = PolicyPrepareOnly
		}
		oldWorking, _ := json.Marshal(records[id].Working)
		_, err = tx.Exec(ctx, `INSERT INTO task_control_targets(control_id,task_id,revision_before,revision_after,previous_working_json) VALUES($1,$2,$3::bigint,$3::bigint+1,$4::jsonb)`, controlID, id, r.Revision, oldWorking)
		if err != nil {
			return result, err
		}
		constraints, _ := json.Marshal(r.Constraints)
		criteria, _ := json.Marshal(r.CompletionCriteria)
		source := r.Source
		if source == nil {
			source = map[string]any{}
		}
		source["last_owner_control"] = map[string]any{"control_id": controlID, "channel": actor.Channel, "delivery_id": actor.DeliveryID, "instruction": req.Instruction}
		sourceJSON, _ := json.Marshal(source)
		updated, err := scan(tx.QueryRow(ctx, `UPDATE tasks SET goal=$2,status=$3,revision=revision+1,constraints_revision=constraints_revision+1,execution_policy=$4,constraints_json=$5::jsonb,completion_criteria_json=$6::jsonb,source_json=$7::jsonb,working_json='{"plan":[],"pending_actions":[],"scratch":{}}',working_revision=working_revision+1,completion_result_id=CASE WHEN $3='completed' THEN NULLIF($8,'') ELSE completion_result_id END,finished_at=CASE WHEN $3 IN('completed','cancelled') THEN clock_timestamp() ELSE NULL END,updated_at=clock_timestamp() WHERE task_id=$1 RETURNING `+columns, id, r.Goal, nextStatus, r.ExecutionPolicy, constraints, criteria, sourceJSON, req.ResultID))
		if err != nil {
			return result, err
		}
		if id == root.TaskID {
			result.Task = updated
		}
	}
	instruction := strings.TrimSpace(req.Instruction)
	if instruction == "" {
		instruction = "Owner control: " + req.Kind
	}
	payload, _ := json.Marshal(map[string]any{"task_id": root.TaskID, "user_id": actor.OwnerID, "text": instruction, "control_id": controlID, "task_revision": result.Task.Revision})
	event := ingress.InputEvent{EventID: controlID, IdempotencyKey: controlID, SessionKey: root.SessionKey, Source: actor.Channel, EventType: runv1.InputEventType_INPUT_EVENT_TYPE_USER_MESSAGE, PayloadJSON: string(payload)}
	result.InputID, err = c.inputs.AdmitControlTx(ctx, tx, event, actor.OwnerID, root.TaskID, "completed", "owner_control_applied", "")
	if err != nil {
		return result, err
	}
	if err = c.transcript.AppendOwnerControlTx(ctx, tx, root.SessionKey, controlID, instruction, map[string]any{"kind": "owner_control", "control_id": controlID, "source": actor.Channel, "task_id": root.TaskID, "task_revision": result.Task.Revision}); err != nil {
		return result, err
	}
	if status == "active" && (req.Kind == ControlAmend || req.Kind == ControlResume) {
		// Continuation uses the task's actual delivery channel, while protected task
		// context retains the authenticated amendment's original channel provenance.
		var channel string
		if err = tx.QueryRow(ctx, `SELECT channel FROM sessions WHERE session_key=$1`, root.SessionKey).Scan(&channel); err != nil {
			return result, err
		}
		event.EventID = controlID + ":continue"
		event.IdempotencyKey = event.EventID
		event.Source = channel
		result.ContinuationInputID, err = c.inputs.AdmitControlTx(ctx, tx, event, actor.OwnerID, root.TaskID, "queued", "accepted", "")
		if err != nil {
			return result, err
		}
	}
	resultJSON, _ := json.Marshal(result)
	_, err = tx.Exec(ctx, `UPDATE task_control_events SET result_json=$2::jsonb WHERE control_id=$1`, controlID, resultJSON)
	if err != nil {
		return result, err
	}
	return result, tx.Commit(ctx)
}
func canonicalJSON(value string) string {
	var object any
	if json.Unmarshal([]byte(value), &object) != nil {
		return value
	}
	data, _ := json.Marshal(object)
	return string(data)
}

func (c *Controller) History(ctx context.Context, id string) ([]ControlResult, error) {
	rows, err := c.store.pool.Query(ctx, `SELECT result_json FROM task_control_events WHERE task_id=$1 ORDER BY applied_at,control_id`, id)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	result := []ControlResult{}
	for rows.Next() {
		var data []byte
		var item ControlResult
		if err = rows.Scan(&data); err != nil {
			return nil, err
		}
		if err = json.Unmarshal(data, &item); err != nil {
			return nil, fmt.Errorf("decode task control: %w", err)
		}
		result = append(result, item)
	}
	return result, rows.Err()
}
