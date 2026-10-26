package task

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/butler/butler/internal/actionauth"
	"github.com/butler/butler/internal/budget"
	"github.com/butler/butler/internal/owneridentity"
	"github.com/butler/butler/internal/ownership"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

type Store struct {
	pool                      *pgxpool.Pool
	authorityReads            *pgxpool.Pool
	budgets                   *budget.Controller
	budgetMandate             string
	verifiedResultSourceGuard func(context.Context, pgx.Tx, string) error
}

// SetVerifiedResultSourceGuard installs the owning Catalog's same-transaction
// historical source check. It grants no completed run execution authority.
func (s *Store) SetVerifiedResultSourceGuard(guard func(context.Context, pgx.Tx, string) error) {
	s.verifiedResultSourceGuard = guard
}

func (s *Store) SetBudgets(c *budget.Controller, mandateID string) {
	s.budgets = c
	s.budgetMandate = mandateID
}

func NewStore(pool *pgxpool.Pool) *Store { return NewStoreWithAuthorityReads(pool, pool) }

func NewStoreWithAuthorityReads(pool, reads *pgxpool.Pool) *Store {
	return &Store{pool: pool, authorityReads: reads}
}

const columns = `task_id, session_key, owner_id, COALESCE(parent_task_id,''), goal, source_json,
 status, revision, constraints_revision, execution_policy, constraints_json, completion_criteria_json, working_json,
 working_revision, COALESCE(last_run_id,''), COALESCE(completion_result_id,''), legacy, COALESCE(to_jsonb(tasks)->>'result_delivery','attempt'), created_at, updated_at, finished_at`

type scanner interface{ Scan(...any) error }

func scan(row scanner) (Record, error) {
	var r Record
	var source, constraints, criteria, working []byte
	err := row.Scan(&r.TaskID, &r.SessionKey, &r.OwnerID, &r.ParentTaskID, &r.Goal, &source, &r.Status, &r.Revision, &r.ConstraintsRevision, &r.ExecutionPolicy, &constraints, &criteria, &working, &r.WorkingRevision, &r.LastRunID, &r.CompletionResultID, &r.Legacy, &r.ResultDelivery, &r.CreatedAt, &r.UpdatedAt, &r.FinishedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return Record{}, ErrNotFound
	}
	if err != nil {
		return Record{}, err
	}
	for _, pair := range []struct {
		data   []byte
		target any
	}{{source, &r.Source}, {constraints, &r.Constraints}, {criteria, &r.CompletionCriteria}, {working, &r.Working}} {
		if err := json.Unmarshal(pair.data, pair.target); err != nil {
			return Record{}, fmt.Errorf("decode task state: %w", err)
		}
	}
	return r, nil
}

func (s *Store) Get(ctx context.Context, id string) (Record, error) {
	return scan(s.pool.QueryRow(ctx, `SELECT `+columns+` FROM tasks WHERE task_id=$1`, id))
}

func (s *Store) GetTx(ctx context.Context, tx pgx.Tx, id string) (Record, error) {
	return scan(tx.QueryRow(ctx, `SELECT `+columns+` FROM tasks WHERE task_id=$1`, id))
}

func (s *Store) GetByRun(ctx context.Context, runID string) (Record, error) {
	return scan(s.pool.QueryRow(ctx, `SELECT `+columns+` FROM tasks WHERE task_id=(SELECT task_id FROM runs WHERE run_id=$1)`, runID))
}

func (s *Store) List(ctx context.Context, sessionKey string, limit int) ([]Record, error) {
	if limit <= 0 || limit > 200 {
		limit = 50
	}
	rows, err := s.pool.Query(ctx, `SELECT `+columns+` FROM tasks WHERE ($1='' OR session_key=$1) ORDER BY updated_at DESC, task_id LIMIT $2`, sessionKey, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	items := make([]Record, 0)
	for rows.Next() {
		item, err := scan(rows)
		if err != nil {
			return nil, err
		}
		items = append(items, item)
	}
	return items, rows.Err()
}

func (s *Store) Create(ctx context.Context, req CreateRequest) (Record, error) {
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return Record{}, err
	}
	defer tx.Rollback(ctx)
	record, err := s.CreateTx(ctx, tx, req)
	if err != nil {
		return record, err
	}
	return record, tx.Commit(ctx)
}

// CreateTx admits a task inside an owned transaction. Existing session and
// parent rows are locked before goal/mandate authority. A caller that already
// checked authority may insert only a new root (never lock an existing parent).
func (s *Store) CreateTx(ctx context.Context, tx pgx.Tx, req CreateRequest) (Record, error) {
	return s.createTx(ctx, tx, req, false)
}

// Only the owning delegation manager can admit a separate child session.
// Public owner/task creation retains its existing same-session contract.
func (s *Store) createTx(ctx context.Context, tx pgx.Tx, req CreateRequest, delegated bool) (Record, error) {

	if err := ValidateCreate(req); err != nil {
		return Record{}, err
	}
	if err := validateStrings(req.Constraints); err != nil {
		return Record{}, err
	}
	if req.Source == nil {
		req.Source = map[string]any{}
	}
	if req.Constraints == nil {
		req.Constraints = []string{}
	}
	if req.ParentTaskID != "" {
		if _, err := lockTreeTx(ctx, tx, req.ParentTaskID); err != nil {
			return Record{}, err
		}
	}
	var owner, channel string
	if err := tx.QueryRow(ctx, `SELECT user_id,channel FROM sessions WHERE session_key=$1 FOR SHARE`, req.SessionKey).Scan(&owner, &channel); err != nil {
		return Record{}, fmt.Errorf("%w: task session does not exist", ErrInvalid)
	}
	if owner != req.OwnerID {
		return Record{}, fmt.Errorf("%w: session owner mismatch", ErrInvalid)
	}
	if len(req.Source) == 0 {
		req.Source = map[string]any{"type": "owner_request", "channel": channel, "session_key": req.SessionKey}
	}
	policy := PolicyNormal
	if req.InitialExecutionPolicy != "" {
		policy = req.InitialExecutionPolicy
	}
	if req.ParentTaskID != "" {
		parent, err := scan(tx.QueryRow(ctx, `SELECT `+columns+` FROM tasks WHERE task_id=$1 FOR SHARE`, req.ParentTaskID))
		if err != nil {
			return Record{}, err
		}
		if parent.OwnerID != req.OwnerID || (!delegated && parent.SessionKey != req.SessionKey) || parent.Status != "active" {
			return Record{}, ErrConflict
		}
		policy = parent.ExecutionPolicy
		if req.ResultDelivery != "" && req.ResultDelivery != parent.ResultDelivery {
			return Record{}, ErrConflict
		}
		req.ResultDelivery = parent.ResultDelivery
		if req.InitialExecutionPolicy != "" && req.InitialExecutionPolicy != policy {
			return Record{}, ErrConflict
		}
		a, err := actionauth.CheckTaskAuthorityTx(ctx, tx, parent.TaskID, parent.OwnerID, nil, time.Now())
		if err != nil {
			return Record{}, err
		}
		if a.TaskID != "" {
			a.TaskID = ""
			req.Authority = &a
		} else if req.Authority != nil {
			return Record{}, ErrConflict
		}
		for _, constraint := range parent.Constraints {
			req.Constraints = appendUnique(req.Constraints, constraint)
		}
	}
	if req.Authority != nil {
		if req.Authority.OwnerID != owner {
			return Record{}, ErrConflict
		}
		if _, _, err := actionauth.CheckAssignmentTx(ctx, tx, *req.Authority, time.Now()); err != nil {
			return Record{}, err
		}
		req.Source["owner_authority"] = req.Authority
	}
	id, err := newID("task")
	if err != nil {
		return Record{}, err
	}
	if req.ParentTaskID != "" && s.budgets != nil {
		if err = s.budgets.ReserveChildTx(ctx, tx, req.ParentTaskID, s.budgetMandate, id); err != nil {
			return Record{}, err
		}
	}
	source, err := json.Marshal(req.Source)
	if err != nil {
		return Record{}, fmt.Errorf("%w: source is not JSON", ErrInvalid)
	}
	constraints, _ := json.Marshal(req.Constraints)
	criteria, _ := json.Marshal(req.CompletionCriteria)
	query := `INSERT INTO tasks(task_id,session_key,owner_id,parent_task_id,goal,source_json,constraints_json,completion_criteria_json,execution_policy)
 VALUES($1,$2,$3,NULLIF($4,''),$5,$6::jsonb,$7::jsonb,$8::jsonb,$9) RETURNING ` + columns
	// Legacy default creation remains compatible with old schemas. Explicit
	// mode/inheritance needs actual069; a missing schema rolls back admission.
	if req.ResultDelivery == "verified_task" {
		ready, err := verifiedDeliveryReady(ctx, tx)
		if err != nil || !ready {
			return Record{}, ErrInvalid
		}
		query = `INSERT INTO tasks(task_id,session_key,owner_id,parent_task_id,goal,source_json,constraints_json,completion_criteria_json,execution_policy,result_delivery)
 VALUES($1,$2,$3,NULLIF($4,''),$5,$6::jsonb,$7::jsonb,$8::jsonb,$9,'verified_task') RETURNING ` + columns
	}
	record, err := scan(tx.QueryRow(ctx, query, id, req.SessionKey, req.OwnerID, req.ParentTaskID, strings.TrimSpace(req.Goal), source, constraints, criteria, policy))
	if err != nil {
		return Record{}, err
	}
	if req.Authority != nil {
		a := *req.Authority
		a.TaskID = id
		if err = BindAuthorityTx(ctx, tx, a); err != nil {
			return Record{}, err
		}
		// A budget or insert lock may have crossed a purpose/hours deadline.
		// These authority rows are already held; recheck the DB clock before commit.
		var at time.Time
		if err = tx.QueryRow(ctx, `SELECT clock_timestamp()`).Scan(&at); err != nil {
			return Record{}, err
		}
		if _, _, err = actionauth.CheckAssignmentTx(ctx, tx, a, at); err != nil {
			return Record{}, err
		}
	}
	return record, nil
}

// AttachRun pins the exact revision before execution. It rejects task attempts
// after a pause, cancellation, completion or concurrent status revision.
func (s *Store) AttachRun(ctx context.Context, taskID string, revision int64, runID, sessionKey string) error {
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
	record, err := scan(tx.QueryRow(ctx, `SELECT `+columns+` FROM tasks WHERE task_id=$1 FOR UPDATE`, taskID))
	if err != nil {
		return err
	}
	if record.Revision != revision || record.Status != "active" || record.SessionKey != sessionKey {
		return ErrConflict
	}
	tag, err := tx.Exec(ctx, `UPDATE runs SET task_id=$1,task_revision=$2 WHERE run_id=$3 AND session_key=$4 AND (task_id IS NULL OR (task_id=$1 AND task_revision=$2))`, taskID, revision, runID, sessionKey)
	if err != nil {
		return err
	}
	if tag.RowsAffected() != 1 {
		return ErrConflict
	}
	if _, err = actionauth.CheckTaskAuthorityTx(ctx, tx, record.TaskID, record.OwnerID, nil, time.Now()); err != nil {
		return err
	}
	_, err = tx.Exec(ctx, `UPDATE tasks SET last_run_id=$2,updated_at=NOW() WHERE task_id=$1`, taskID, runID)
	if err != nil {
		return err
	}
	return tx.Commit(ctx)
}

func (s *Store) SaveWorking(ctx context.Context, id string, revision int64, state WorkingState) (Record, error) {
	ctx, release, err := ownership.Begin(ctx)
	if err != nil {
		return Record{}, err
	}
	defer release()

	if state.Plan == nil {
		state.Plan = []string{}
	}
	if state.PendingActions == nil {
		state.PendingActions = []string{}
	}
	if state.Scratch == nil {
		state.Scratch = map[string]any{}
	}
	data, err := json.Marshal(state)
	if err != nil {
		return Record{}, fmt.Errorf("%w: working state is not JSON", ErrInvalid)
	}
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return Record{}, err
	}
	defer tx.Rollback(ctx)
	if err = guardTaskWrite(ctx, tx, id); err != nil {
		return Record{}, err
	}
	record, err := scan(tx.QueryRow(ctx, `UPDATE tasks SET working_json=$3::jsonb,working_revision=working_revision+1,updated_at=NOW()
 WHERE task_id=$1 AND working_revision=$2 AND status NOT IN ('completed','cancelled') RETURNING `+columns, id, revision, data))
	if errors.Is(err, ErrNotFound) {
		return Record{}, ErrConflict
	}
	if err != nil {
		return record, err
	}
	return record, tx.Commit(ctx)
}

func (s *Store) CheckpointRun(ctx context.Context, runID string, revision int64, state WorkingState) (Record, error) {
	ctx, release, err := ownership.Begin(ctx)
	if err != nil {
		return Record{}, err
	}
	defer release()

	data, err := json.Marshal(state)
	if err != nil {
		return Record{}, err
	}
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return Record{}, err
	}
	defer tx.Rollback(ctx)
	var id string
	if err = tx.QueryRow(ctx, `SELECT task_id FROM runs WHERE run_id=$1 FOR SHARE`, runID).Scan(&id); err != nil {
		return Record{}, err
	}
	if err = guardTaskWrite(ctx, tx, id); err != nil {
		return Record{}, err
	}
	record, err := scan(tx.QueryRow(ctx, `UPDATE tasks SET working_json=$3::jsonb,working_revision=working_revision+1,updated_at=NOW()
 WHERE working_revision=$2 AND status='active' AND EXISTS(SELECT 1 FROM runs WHERE run_id=$1 AND runs.task_id=tasks.task_id AND runs.task_revision=tasks.revision) RETURNING `+columns, runID, revision, data))
	if errors.Is(err, ErrNotFound) {
		return Record{}, ErrConflict
	}
	if err != nil {
		return record, err
	}
	return record, tx.Commit(ctx)
}

// Transition is a trusted owner control operation, never a model finish event.
func (s *Store) Transition(ctx context.Context, id string, revision int64, status, resultID string) (Record, error) {
	hint, err := s.Get(ctx, id)
	if err != nil {
		return Record{}, err
	}
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return Record{}, err
	}
	defer tx.Rollback(ctx)
	if hint.ResultDelivery == "verified_task" {
		a, ok := owneridentity.FromContext(ctx)
		_, worker := ownership.TokenFromContext(ctx)
		if !ok || worker || a.OwnerID != hint.OwnerID || (a.SessionKey != "" && a.SessionKey != hint.SessionKey) {
			return Record{}, ErrUnauthorized
		}
		if _, err = tx.Exec(ctx, `SELECT session_key FROM sessions WHERE session_key=$1 AND user_id=$2 FOR SHARE`, hint.SessionKey, hint.OwnerID); err != nil {
			return Record{}, err
		}
	}
	record, err := scan(tx.QueryRow(ctx, `SELECT `+columns+` FROM tasks WHERE task_id=$1 FOR UPDATE`, id))
	if err != nil {
		return Record{}, err
	}
	if record.ResultDelivery == "verified_task" && record.Status == "completed" && status == "completed" && record.CompletionResultID == resultID && record.Revision == revision+1 {
		return record, tx.Commit(ctx)
	}
	if record.Revision != revision {
		return Record{}, ErrConflict
	}
	if status != "cancelled" && status != "paused" {
		if _, err = actionauth.CheckTaskAuthorityTx(ctx, tx, id, record.OwnerID, nil, time.Now()); err != nil {
			return Record{}, err
		}
	}
	if err := ValidateTransition(record.Status, status); err != nil {
		return Record{}, err
	}
	if status == "completed" {
		result, err := scanResult(tx.QueryRow(ctx, `SELECT `+resultColumns+` FROM task_results WHERE task_id=$1 AND result_id=$2`, id, resultID))
		if err != nil {
			return Record{}, err
		}
		if result.Verdict != "verified" {
			return Record{}, ErrInvalid
		}
		if err = ValidateVerification(record, result); err != nil {
			return Record{}, err
		}
	}
	stored, err := scan(tx.QueryRow(ctx, `UPDATE tasks SET status=$3,revision=revision+1,completion_result_id=CASE WHEN $3='completed' THEN NULLIF($4,'') ELSE completion_result_id END,
 finished_at=CASE WHEN $3 IN ('completed','cancelled') THEN NOW() ELSE NULL END,updated_at=NOW() WHERE task_id=$1 AND revision=$2 RETURNING `+columns, id, revision, status, resultID))
	if err != nil {
		return Record{}, err
	}
	if err = s.queueVerifiedNotificationTx(ctx, tx, record, stored); err != nil {
		return Record{}, err
	}
	if err = tx.Commit(ctx); err != nil {
		return Record{}, err
	}
	return stored, nil
}

const resultColumns = `result_id,task_id,task_revision,COALESCE(run_id,''),summary,evidence_refs_json,verified_criteria_json,verdict,verifier,created_at`

func scanResult(row scanner) (Result, error) {
	var result Result
	var evidence, criteria []byte
	err := row.Scan(&result.ResultID, &result.TaskID, &result.TaskRevision, &result.RunID, &result.Summary, &evidence, &criteria, &result.Verdict, &result.Verifier, &result.CreatedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return Result{}, ErrNotFound
	}
	if err != nil {
		return Result{}, err
	}
	if err = json.Unmarshal(evidence, &result.EvidenceRefs); err != nil {
		return Result{}, err
	}
	if err = json.Unmarshal(criteria, &result.VerifiedCriteria); err != nil {
		return Result{}, err
	}
	return result, nil
}

func (s *Store) SaveVerifiedResult(ctx context.Context, result Result) (Result, error) {
	actor, ok := owneridentity.FromContext(ctx)
	_, worker := ownership.TokenFromContext(ctx)
	if !ok || worker {
		return Result{}, ErrUnauthorized
	}
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return Result{}, err
	}
	defer tx.Rollback(ctx)
	record, err := scan(tx.QueryRow(ctx, `SELECT `+columns+` FROM tasks WHERE task_id=$1 FOR UPDATE`, result.TaskID))
	if err != nil {
		return Result{}, err
	}
	if actor.OwnerID != record.OwnerID || (actor.SessionKey != "" && actor.SessionKey != record.SessionKey) {
		return Result{}, ErrUnauthorized
	}
	if record.Status == "completed" || record.Status == "cancelled" {
		return Result{}, ErrConflict
	}
	if _, err = actionauth.CheckTaskAuthorityTx(ctx, tx, record.TaskID, record.OwnerID, nil, time.Now()); err != nil {
		return Result{}, err
	}
	if err = ValidateVerification(record, result); err != nil {
		return Result{}, err
	}
	if result.RunID != "" {
		var belongs bool
		if err = tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM runs WHERE run_id=$1 AND task_id=$2)`, result.RunID, result.TaskID).Scan(&belongs); err != nil {
			return Result{}, err
		}
		if !belongs {
			return Result{}, ErrInvalid
		}
	}
	result.ResultID, err = newID("result")
	if err != nil {
		return Result{}, err
	}
	evidence, _ := json.Marshal(result.EvidenceRefs)
	criteria, _ := json.Marshal(result.VerifiedCriteria)
	stored, err := scanResult(tx.QueryRow(ctx, `INSERT INTO task_results(result_id,task_id,task_revision,run_id,summary,evidence_refs_json,verified_criteria_json,verdict,verifier)
 VALUES($1,$2,$3,NULLIF($4,''),$5,$6::jsonb,$7::jsonb,'verified','owner') RETURNING `+resultColumns, result.ResultID, result.TaskID, result.TaskRevision, result.RunID, result.Summary, evidence, criteria))
	if err != nil {
		return Result{}, err
	}
	if err = tx.Commit(ctx); err != nil {
		return Result{}, err
	}
	return stored, nil
}

func (s *Store) Results(ctx context.Context, id string) ([]Result, error) {
	rows, err := s.pool.Query(ctx, `SELECT `+resultColumns+` FROM task_results WHERE task_id=$1 ORDER BY created_at,result_id`, id)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	results := make([]Result, 0)
	for rows.Next() {
		result, err := scanResult(rows)
		if err != nil {
			return nil, err
		}
		results = append(results, result)
	}
	return results, rows.Err()
}

func (s *Store) RunIDs(ctx context.Context, id string) ([]string, error) {
	rows, err := s.pool.Query(ctx, `SELECT run_id FROM runs WHERE task_id=$1 ORDER BY started_at,run_id`, id)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	ids := make([]string, 0)
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			return nil, err
		}
		ids = append(ids, id)
	}
	return ids, rows.Err()
}

func (s *Store) SaveRunOutcome(ctx context.Context, runID, state, note string) error {
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
	var id string
	var revision int64
	err = tx.QueryRow(ctx, `SELECT task_id,task_revision FROM runs WHERE run_id=$1 AND task_id IS NOT NULL`, runID).Scan(&id, &revision)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil
	}
	if err != nil {
		return err
	}
	var owner string
	if err = tx.QueryRow(ctx, `SELECT owner_id FROM tasks WHERE task_id=$1 AND revision=$2 FOR UPDATE`, id, revision).Scan(&owner); err != nil {
		return ErrConflict
	}
	if _, err = actionauth.CheckTaskAuthorityTx(ctx, tx, id, owner, nil, time.Now()); err != nil {
		return err
	}
	if strings.TrimSpace(note) != "" {
		_, err = tx.Exec(ctx, `INSERT INTO task_results(result_id,task_id,task_revision,run_id,summary,verdict,verifier)
 VALUES($1,$2,$3,$4,$5,'unverified','model_run') ON CONFLICT(result_id) DO NOTHING`, "run:"+runID, id, revision, runID, note)
		if err != nil {
			return err
		}
	}
	// A failed attempt blocks this task only; successful model final leaves all
	// work intact. No transition can reopen an already cancelled/completed task.
	_, err = tx.Exec(ctx, `UPDATE tasks SET status=CASE WHEN $3 IN ('failed','timed_out') AND status='active' THEN 'blocked' ELSE status END,
 revision=revision+CASE WHEN $3 IN ('failed','timed_out') AND status='active' THEN 1 ELSE 0 END,updated_at=NOW()
 WHERE task_id=$1 AND revision=$2`, id, revision, state)
	if err != nil {
		return err
	}
	return tx.Commit(ctx)
}

func newID(prefix string) (string, error) {
	var raw [16]byte
	if _, err := rand.Read(raw[:]); err != nil {
		return "", err
	}
	return prefix + "_" + hex.EncodeToString(raw[:]), nil
}
