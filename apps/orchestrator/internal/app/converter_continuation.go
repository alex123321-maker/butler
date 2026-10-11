package app

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"sync"
	"time"

	durableinput "github.com/butler/butler/apps/orchestrator/internal/input"
	"github.com/butler/butler/apps/orchestrator/internal/task"
	"github.com/butler/butler/internal/budget"
	"github.com/butler/butler/internal/generatedtools"
	"github.com/butler/butler/internal/memory/catalog"
	"github.com/butler/butler/internal/owneridentity"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// ConverterContinuations schedules only two previously accepted technical
// phases of the original task. It owns no model/runtime or owner authority.
type ConverterContinuations struct {
	pool      *pgxpool.Pool
	tasks     *task.Store
	inputs    *durableinput.Store
	sources   *catalog.Store
	budgets   *budget.Controller
	allowance budget.ModelAllowance
	profile   generatedtools.Profile
	mu        sync.Mutex
	after     string
}

var errContinuationParentBusy = errors.New("continuation parent is still owned")

func NewConverterContinuations(pool *pgxpool.Pool, tasks *task.Store, inputs *durableinput.Store, sources *catalog.Store, budgets *budget.Controller, a budget.ModelAllowance, profile generatedtools.Profile) (*ConverterContinuations, error) {
	if pool == nil || tasks == nil || inputs == nil || sources == nil || budgets == nil || profile.Validate() != nil || a.InputTokens <= 0 || a.OutputTokens != budgets.Policy.OutputTokens {
		return nil, generatedtools.ErrAccess
	}
	var ready bool
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if pool.QueryRow(ctx, `WITH owned AS (SELECT to_regclass(format('%I.%I',current_schema(),'converter_parent_continuations')) AS oid), required(name,kind,nullable) AS (
 SELECT n,'pg_catalog.text',FALSE FROM unnest(ARRAY['continuation_id','child_task_id','owner_id','parent_task_id','purpose','intent_hash','state','ready_hash']) n
 UNION ALL SELECT n,'pg_catalog.int8',FALSE FROM unnest(ARRAY['parent_revision','constraints_revision']) n
 UNION ALL SELECT 'intent_json','pg_catalog.jsonb',FALSE UNION ALL SELECT 'ready_json','pg_catalog.jsonb',TRUE
 UNION ALL SELECT 'input_id','pg_catalog.text',TRUE UNION ALL SELECT n,'pg_catalog.timestamptz',FALSE FROM unnest(ARRAY['deadline','created_at']) n
 ) SELECT owned.oid IS NOT NULL AND NOT EXISTS(SELECT 1 FROM required r LEFT JOIN pg_attribute a ON a.attrelid=owned.oid AND a.attname=r.name AND NOT a.attisdropped WHERE a.attname IS NULL OR a.atttypid<>to_regtype(r.kind) OR a.attnotnull=r.nullable)
 AND EXISTS(SELECT 1 FROM pg_trigger t WHERE t.tgrelid=owned.oid AND t.tgname='converter_continuation_guard' AND NOT t.tgisinternal AND t.tgenabled='O' AND t.tgfoid=to_regprocedure(format('%I.converter_continuation_immutable()',current_schema()))) FROM owned`).Scan(&ready) != nil || !ready {
		return nil, generatedtools.ErrAccess
	}
	return &ConverterContinuations{pool: pool, tasks: tasks, inputs: inputs, sources: sources, budgets: budgets, allowance: a, profile: profile}, nil
}

// EnableContinuations is startup-only composition, never a model/catalog flag.
func (s *ConverterDelegation) EnableContinuations(c *ConverterContinuations) error {
	if s == nil || c == nil || s.pool != c.pool || s.reader.profile != c.profile {
		return generatedtools.ErrAccess
	}
	s.reader.continuations = true
	return nil
}

func (s *ConverterContinuations) Sweep(ctx context.Context) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	rows, err := s.pool.Query(ctx, `SELECT continuation_id,parent_task_id FROM converter_parent_continuations WHERE state='waiting' AND continuation_id>$1 ORDER BY continuation_id LIMIT 4`, s.after)
	if err != nil {
		return err
	}
	type item struct{ id, parent string }
	var list []item
	for rows.Next() {
		var i item
		if err = rows.Scan(&i.id, &i.parent); err != nil {
			break
		}
		list = append(list, i)
	}
	if err == nil {
		err = rows.Err()
	}
	rows.Close()
	if err != nil {
		return err
	}
	if len(list) == 0 {
		s.after = ""
	}
	var errs []error
	for _, i := range list {
		s.after = i.id
		if _, err = s.Enqueue(ctx, i.parent, i.id); err != nil {
			errs = append(errs, err)
		}
	}
	return errors.Join(errs...)
}

// Enqueue uses the same tree/ownership/session barrier as controls. A queued
// input and its once-only receipt commit together. No read body is granted.
func (s *ConverterContinuations) Enqueue(ctx context.Context, parentID, id string) (bool, error) {
	if _, owner := owneridentity.FromContext(ctx); owner {
		return false, generatedtools.ErrAccess
	}
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return false, err
	}
	defer tx.Rollback(ctx)
	parent, err := s.lockParentTx(ctx, tx, parentID)
	if err != nil {
		if errors.Is(err, errContinuationParentBusy) {
			return false, nil
		}
		return false, err
	}
	c, err := generatedtools.ReadContinuationTx(ctx, tx, id)
	if err != nil || c.Intent.ParentTaskID != parent.TaskID || c.Intent.OwnerID != parent.OwnerID || c.Intent.ParentRevision != parent.Revision || c.Intent.ConstraintsRevision != parent.ConstraintsRevision {
		return false, generatedtools.ErrAccess
	}
	if c.State == "queued" {
		return false, tx.Commit(ctx)
	}
	var idle bool
	if tx.QueryRow(ctx, `SELECT NOT EXISTS(SELECT 1 FROM session_ownership WHERE session_key=$1 AND expires_at>clock_timestamp()) AND NOT EXISTS(SELECT 1 FROM runs WHERE task_id=$2 AND current_state NOT IN ('completed','failed','cancelled','timed_out'))`, parent.SessionKey, parent.TaskID).Scan(&idle) != nil || !idle {
		return false, nil
	}
	var originalDone bool
	if tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM accepted_inputs WHERE input_id=$1 AND run_id=$2 AND status='completed')`, c.Intent.OriginalInputID, c.Intent.OriginalRunID).Scan(&originalDone) != nil || !originalDone {
		return false, nil
	}
	var returned bool
	if tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM converter_developer_returns WHERE child_task_id=$1)`, c.Intent.ChildTaskID).Scan(&returned) != nil || !returned {
		return false, nil
	}
	snapshot, projection, err := generatedtools.CheckContinuationTx(ctx, tx, s.sources, c)
	if err != nil {
		return false, fmt.Errorf("continuation facts: %w", err)
	}
	if snapshot.Permission.RuntimePin != generatedtools.Hash(s.profile) {
		return false, generatedtools.ErrAccess
	}
	ready := generatedtools.ContinuationReady{ReturnHash: generatedtools.Hash(projection), PriorRunID: c.Intent.OriginalRunID, PriorInputID: c.Intent.OriginalInputID}
	if c.Intent.Purpose == generatedtools.ActivatedCatalog {
		var firstInput, runID string
		if tx.QueryRow(ctx, `SELECT i.input_id,i.run_id FROM converter_parent_continuations first JOIN accepted_inputs i ON i.input_id=first.input_id JOIN runs r ON r.run_id=i.run_id WHERE first.child_task_id=$1 AND first.purpose='candidate_return' AND first.state='queued' AND i.status='completed' AND r.current_state='completed' FOR SHARE OF first,i,r`, c.Intent.ChildTaskID).Scan(&firstInput, &runID) != nil {
			return false, nil
		}
		ready.PriorInputID, ready.PriorRunID = firstInput, runID
		if tx.QueryRow(ctx, `SELECT i.action_id,ce.action_id,ce.generation FROM converter_events ce JOIN action_intents i ON i.effect_id=ce.action_id WHERE ce.owner_id=$1 AND ce.digest=$2 AND ce.kind='converters.activate' AND ce.run_id=$3 ORDER BY ce.event_id DESC LIMIT 1`, parent.OwnerID, projection.Candidate.Digest, runID).Scan(&ready.ActivationActionID, &ready.ActivationEffectID, &ready.Generation) != nil {
			return false, nil
		}
	}
	ready.Sources, err = generatedtools.RunSourcePinsTx(ctx, tx, ready.PriorRunID)
	if err != nil || len(ready.Sources) == 0 {
		return false, generatedtools.ErrAccess
	}
	c.Ready, c.State = ready, "queued"
	if _, _, err = generatedtools.CheckContinuationTx(ctx, tx, s.sources, c); err != nil {
		return false, err
	}
	if err = s.checkSourcesTx(ctx, tx, c, projection); err != nil {
		return false, fmt.Errorf("continuation full privacy: %w", err)
	}
	if err = s.budgets.CheckToolTx(ctx, tx, budget.ToolIdentity{RunID: c.Intent.OriginalRunID, TaskRevision: c.Intent.ParentRevision, ConstraintsRevision: c.Intent.ConstraintsRevision, ToolName: "converters.continue", Effect: "prepare"}); err != nil {
		return false, err
	}
	if err = s.budgets.CheckTaskContinuationTx(ctx, tx, parent.TaskID, parent.OwnerID, s.allowance); err != nil {
		return false, fmt.Errorf("continuation finite model preflight: %w", err)
	}
	// All waits precede this last source/authority/time check and queue admission.
	if _, _, err = generatedtools.CheckContinuationTx(ctx, tx, s.sources, c); err != nil {
		return false, err
	}
	if err = s.checkSourcesTx(ctx, tx, c, projection); err != nil {
		return false, err
	}
	inputID, err := s.inputs.AcceptConverterContinuationTx(ctx, tx, durableinput.ConverterContinuationRequest{ID: id, TaskID: parent.TaskID, OwnerID: parent.OwnerID, SessionKey: parent.SessionKey, Deadline: c.Intent.Deadline})
	if err != nil {
		return false, err
	}
	raw, _ := json.Marshal(ready)
	tag, err := tx.Exec(ctx, `UPDATE converter_parent_continuations SET state='queued',input_id=$2,ready_hash=$3,ready_json=$4::jsonb WHERE continuation_id=$1 AND state='waiting'`, id, inputID, generatedtools.Hash(ready), raw)
	if err != nil || tag.RowsAffected() != 1 {
		return false, generatedtools.ErrAccess
	}
	return true, tx.Commit(ctx)
}

func (s *ConverterContinuations) lockParentTx(ctx context.Context, tx pgx.Tx, id string) (task.Record, error) {
	var root, key string
	if tx.QueryRow(ctx, `WITH RECURSIVE a AS(SELECT task_id,parent_task_id FROM tasks WHERE task_id=$1 UNION SELECT t.task_id,t.parent_task_id FROM tasks t JOIN a ON t.task_id=a.parent_task_id) SELECT task_id FROM a WHERE parent_task_id IS NULL`, id).Scan(&root) != nil {
		return task.Record{}, generatedtools.ErrAccess
	}
	if _, err := tx.Exec(ctx, `SELECT pg_advisory_xact_lock(hashtextextended('butler-task-tree:'||$1,0))`, root); err != nil {
		return task.Record{}, err
	}
	if tx.QueryRow(ctx, `SELECT session_key FROM tasks WHERE task_id=$1`, id).Scan(&key) != nil {
		return task.Record{}, generatedtools.ErrAccess
	}
	// Do not take exclusive session/run locks while a worker is live. Besides
	// refusing concurrent continuation, this prevents maintenance from holding
	// the session while that worker's source verifier upgrades its own run lock.
	var active bool
	if err := tx.QueryRow(ctx, `SELECT expires_at>clock_timestamp() FROM session_ownership WHERE session_key=$1 FOR SHARE`, key).Scan(&active); err != nil && !errors.Is(err, pgx.ErrNoRows) {
		return task.Record{}, err
	} else if active {
		return task.Record{}, errContinuationParentBusy
	}
	if _, err := tx.Exec(ctx, `SELECT session_key FROM sessions WHERE session_key=$1 FOR UPDATE`, key); err != nil {
		return task.Record{}, err
	}
	if _, err := tx.Exec(ctx, `SELECT run_id FROM runs WHERE task_id=$1 ORDER BY run_id FOR SHARE`, id); err != nil {
		return task.Record{}, err
	}
	p, err := s.tasks.GetTx(ctx, tx, id)
	if err != nil || p.Status != "active" {
		return p, generatedtools.ErrAccess
	}
	return p, nil
}

func (s *ConverterContinuations) checkSourcesTx(ctx context.Context, tx pgx.Tx, c generatedtools.Continuation, p generatedtools.DeveloperReturn) error {
	return generatedtools.CheckContinuationSourcesTx(ctx, tx, s.sources, c, p)
}
