package goal

import (
	"context"
	"errors"
	"fmt"
	"github.com/butler/butler/apps/orchestrator/internal/task"
	"github.com/butler/butler/internal/owneridentity"
	"github.com/jackc/pgx/v5"
	"time"
)

// Sweep performs bounded, repeatable branch cancellation after authority denial
// is already committed. No authority rows are retained while taking ownership.
// Multiple workers may retry a job; Controller's durable delivery identity and
// revision CAS make this safe without keeping a pool connection during Apply.
func (m *Manager) Sweep(ctx context.Context) error {
	ctx, cancel := context.WithTimeout(ctx, 8*time.Second)
	defer cancel()
	if m.controls == nil || m.tasks == nil {
		return fmt.Errorf("owner authority propagation unavailable")
	}
	if err := m.queueExpired(ctx); err != nil {
		return err
	}
	var eventID, owner, kind, id, control string
	var revision int64
	err := m.pool.QueryRow(ctx, `SELECT event_id,owner_id,subject_kind,subject_id,current_revision,control_kind FROM owner_authority_jobs WHERE state='pending' ORDER BY updated_at,event_id LIMIT 1`).Scan(&eventID, &owner, &kind, &id, &revision, &control)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil
		}
		return err
	}
	rows, err := m.pool.Query(ctx, `SELECT b.task_id FROM task_authority_bindings b JOIN tasks t USING(task_id) JOIN owner_goals g ON g.goal_id=b.goal_id JOIN action_mandates a ON a.mandate_id=b.mandate_id JOIN owner_purpose_policies p ON p.owner_id=b.owner_id AND p.purpose=b.purpose WHERE b.owner_id=$1 AND t.status NOT IN ('completed','cancelled') AND ($4='stop' OR t.status<>'paused') AND (($2='goal' AND b.goal_id=$3 AND (b.goal_revision<>g.revision OR g.status<>'active' OR g.expires_at<=clock_timestamp())) OR ($2='mandate' AND b.mandate_id=$3 AND (b.mandate_revision<>a.revision OR a.status<>'active' OR a.expires_at<=clock_timestamp())) OR ($2='policy' AND b.purpose=$3 AND b.policy_revision<>p.revision)) ORDER BY t.parent_task_id NULLS FIRST,b.task_id LIMIT 32`, owner, kind, id, control)
	if err != nil {
		return m.jobFailed(ctx, eventID, err)
	}
	var ids []string
	for rows.Next() {
		var taskID string
		if err = rows.Scan(&taskID); err != nil {
			rows.Close()
			return m.jobFailed(ctx, eventID, err)
		}
		ids = append(ids, taskID)
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		return m.jobFailed(ctx, eventID, err)
	}
	for _, taskID := range ids {
		r, err := m.tasks.Get(ctx, taskID)
		if err != nil {
			return m.jobFailed(ctx, eventID, err)
		}
		if r.Status == "completed" || r.Status == "cancelled" || control == "pause" && r.Status == "paused" {
			continue
		}
		// This proof is derived only from a committed authenticated decision (or
		// system expiry of one), never from model text. It can only reduce authority.
		proof := owneridentity.Actor{OwnerID: owner, Channel: "web", DeliveryID: "authority-job:" + eventID + ":" + taskID + fmt.Sprintf(":%d", r.Revision)}
		_, err = m.controls.Apply(owneridentity.WithActor(ctx, proof), task.ControlRequest{TaskID: taskID, ExpectedRevision: r.Revision, Kind: control, Instruction: "Durable owner goal or mandate changed; old branch authority is invalid."})
		if err != nil && !errors.Is(err, task.ErrConflict) {
			return m.jobFailed(ctx, eventID, err)
		}
	}
	if len(ids) == 32 {
		_, err = m.pool.Exec(ctx, `UPDATE owner_authority_jobs SET updated_at=clock_timestamp(),attempts=attempts+1 WHERE event_id=$1`, eventID)
		return err
	}
	// Re-read after controls: a concurrent job/control may have changed a branch.
	var pending bool
	err = m.pool.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM task_authority_bindings b JOIN tasks t USING(task_id) JOIN owner_goals g ON g.goal_id=b.goal_id JOIN action_mandates a ON a.mandate_id=b.mandate_id JOIN owner_purpose_policies p ON p.owner_id=b.owner_id AND p.purpose=b.purpose WHERE b.owner_id=$1 AND t.status NOT IN ('completed','cancelled') AND ($4='stop' OR t.status<>'paused') AND (($2='goal' AND b.goal_id=$3 AND (b.goal_revision<>g.revision OR g.status<>'active' OR g.expires_at<=clock_timestamp())) OR ($2='mandate' AND b.mandate_id=$3 AND (b.mandate_revision<>a.revision OR a.status<>'active' OR a.expires_at<=clock_timestamp())) OR ($2='policy' AND b.purpose=$3 AND b.policy_revision<>p.revision)))`, owner, kind, id, control).Scan(&pending)
	if err != nil {
		return m.jobFailed(ctx, eventID, err)
	}
	if pending {
		return nil
	}
	_, err = m.pool.Exec(ctx, `UPDATE owner_authority_jobs SET state='applied',last_error='',attempts=attempts+1,updated_at=clock_timestamp() WHERE event_id=$1`, eventID)
	return err
}

func (m *Manager) jobFailed(ctx context.Context, id string, cause error) error {
	// Persist a bounded classification; database/provider errors may contain
	// sensitive request material and must not become model-visible job text.
	_, err := m.pool.Exec(ctx, `UPDATE owner_authority_jobs SET attempts=attempts+1,last_error='branch cancellation unavailable',updated_at=clock_timestamp() WHERE event_id=$1`, id)
	if err != nil {
		return err
	}
	return fmt.Errorf("owner authority propagation pending: %w", cause)
}

func (m *Manager) queueExpired(ctx context.Context) error {
	if err := m.expirePolicies(ctx); err != nil {
		return err
	}
	_, err := m.pool.Exec(ctx, `WITH expired AS (
 SELECT owner_id,'goal'::text kind,goal_id id,revision FROM owner_goals WHERE status='active' AND expires_at<=clock_timestamp()
 UNION ALL SELECT owner_id,'mandate',mandate_id,revision FROM action_mandates WHERE scope_kind='owner_goal' AND status='active' AND expires_at<=clock_timestamp()
 ), added AS (
 INSERT INTO owner_authority_events(event_id,owner_id,source,delivery_id,subject_kind,subject_id,revision,request_hash,snapshot)
 SELECT 'expiry:'||kind||':'||id||':'||revision,owner_id,'system_expiry',kind||':'||id||':'||revision,kind,id,revision,'deadline',jsonb_build_object('status','expired') FROM expired
 ON CONFLICT(owner_id,source,delivery_id) DO NOTHING RETURNING *
 ) INSERT INTO owner_authority_jobs(event_id,owner_id,subject_kind,subject_id,current_revision,control_kind)
 SELECT event_id,owner_id,subject_kind,subject_id,revision,'stop' FROM added ON CONFLICT(event_id) DO NOTHING`)
	return err
}

func (m *Manager) expirePolicies(ctx context.Context) error {
	tx, err := m.pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)
	rows, err := tx.Query(ctx, `SELECT goal_id,owner_id,revision,goal_data->>'purpose' FROM owner_goals WHERE kind IN ('preference','constraint') AND status='active' AND expires_at<=clock_timestamp() ORDER BY goal_id LIMIT 16 FOR UPDATE SKIP LOCKED`)
	if err != nil {
		return err
	}
	type expired struct {
		id, owner, purpose string
		revision           int64
	}
	var items []expired
	for rows.Next() {
		var e expired
		if err = rows.Scan(&e.id, &e.owner, &e.revision, &e.purpose); err != nil {
			rows.Close()
			return err
		}
		items = append(items, e)
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		return err
	}
	// Goals above are independent data rows, not task-locking authority. Lock the
	// affected purpose rows in canonical order before updating any of them.
	if len(items) > 0 {
		rows, err = tx.Query(ctx, `SELECT owner_id,purpose FROM owner_purpose_policies WHERE (owner_id,purpose) IN(SELECT owner_id,goal_data->>'purpose' FROM owner_goals WHERE kind IN ('preference','constraint') AND status='active' AND expires_at<=clock_timestamp()) ORDER BY owner_id,purpose FOR UPDATE`)
		if err != nil {
			return err
		}
		rows.Close()
	}
	for _, e := range items {
		var revision int64
		if err = tx.QueryRow(ctx, `UPDATE owner_purpose_policies SET revision=revision+1 WHERE owner_id=$1 AND purpose=$2 RETURNING revision`, e.owner, e.purpose).Scan(&revision); err != nil {
			return err
		}
		if _, err = tx.Exec(ctx, `UPDATE owner_goals SET status='paused',revision=revision+1,goal_data=goal_data||jsonb_build_object('status','paused','revision',revision+1),updated_at=clock_timestamp() WHERE goal_id=$1`, e.id); err != nil {
			return err
		}
		id := fmt.Sprintf("expiry:policy:%s:%d", e.id, e.revision)
		if _, err = tx.Exec(ctx, `INSERT INTO owner_authority_events(event_id,owner_id,source,delivery_id,subject_kind,subject_id,revision,request_hash,snapshot) VALUES($1,$2,'system_expiry',$1,'goal',$3,$4,'deadline','{"status":"expired"}')`, id, e.owner, e.id, e.revision+1); err != nil {
			return err
		}
		if _, err = tx.Exec(ctx, `INSERT INTO owner_authority_jobs(event_id,owner_id,subject_kind,subject_id,current_revision,control_kind) VALUES($1,$2,'policy',$3,$4,'pause')`, id, e.owner, e.purpose, revision); err != nil {
			return err
		}
	}
	return tx.Commit(ctx)
}
