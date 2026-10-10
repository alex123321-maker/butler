package goal

import (
	"context"
	"encoding/json"
	"github.com/butler/butler/internal/actionauth"
	"time"
)

func (m *Manager) View(ctx context.Context) (View, error) {
	a, err := actor(ctx)
	v := View{Goals: []Record{}, Mandates: []actionauth.Mandate{}, Obligations: []Obligation{}}
	if err != nil {
		return v, err
	}
	rows, err := m.pool.Query(ctx, `SELECT goal_data,source_data,created_at,updated_at FROM owner_goals WHERE owner_id=$1 ORDER BY kind,created_at DESC LIMIT 500`, a.OwnerID)
	if err != nil {
		return v, err
	}
	for rows.Next() {
		var data, source []byte
		var g Record
		if err = rows.Scan(&data, &source, &g.CreatedAt, &g.UpdatedAt); err != nil {
			rows.Close()
			return v, err
		}
		if err = json.Unmarshal(data, &g.GoalPolicy); err != nil {
			rows.Close()
			return v, err
		}
		if err = json.Unmarshal(source, &g.Source); err != nil {
			rows.Close()
			return v, err
		}
		v.Goals = append(v.Goals, g)
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		return v, err
	}
	rows, err = m.pool.Query(ctx, `SELECT mandate_data,revision,status,expires_at,revoked_at FROM action_mandates WHERE owner_id=$1 AND scope_kind='owner_goal' ORDER BY created_at DESC LIMIT 500`, a.OwnerID)
	if err != nil {
		return v, err
	}
	for rows.Next() {
		var data []byte
		var mandate actionauth.Mandate
		var revision int64
		var status string
		var expiry time.Time
		var revoked *time.Time
		if err = rows.Scan(&data, &revision, &status, &expiry, &revoked); err != nil {
			rows.Close()
			return v, err
		}
		if err = json.Unmarshal(data, &mandate); err != nil {
			rows.Close()
			return v, err
		}
		mandate.Revision = revision
		mandate.Status = status
		mandate.ExpiresAt = expiry
		mandate.RevokedAt = revoked
		v.Mandates = append(v.Mandates, mandate)
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		return v, err
	}
	rows, err = m.pool.Query(ctx, `SELECT b.task_id,b.owner_id,b.goal_id,b.goal_revision,b.mandate_id,b.mandate_revision,b.purpose,b.initiative_reason,b.observation_ref,b.policy_revision,t.status,t.revision FROM task_authority_bindings b JOIN tasks t USING(task_id) WHERE b.owner_id=$1 AND t.status NOT IN ('completed','cancelled') ORDER BY t.updated_at DESC LIMIT 500`, a.OwnerID)
	if err != nil {
		return v, err
	}
	for rows.Next() {
		var o Obligation
		if err = rows.Scan(&o.TaskID, &o.OwnerID, &o.GoalID, &o.GoalRevision, &o.MandateID, &o.MandateRevision, &o.Purpose, &o.InitiativeReason, &o.ObservationRef, &o.PolicyRevision, &o.TaskStatus, &o.TaskRevision); err != nil {
			rows.Close()
			return v, err
		}
		v.Obligations = append(v.Obligations, o)
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		return v, err
	}
	for i := range v.Obligations {
		tx, e := m.pool.Begin(ctx)
		if e != nil {
			return v, e
		}
		_, _, e = actionauth.CheckAssignmentTx(ctx, tx, v.Obligations[i].Assignment, time.Now())
		tx.Rollback(ctx)
		v.Obligations[i].AuthorityCurrent = e == nil
	}
	err = m.pool.QueryRow(ctx, `SELECT count(*) FROM owner_authority_jobs WHERE owner_id=$1 AND state='pending'`, a.OwnerID).Scan(&v.PendingPropagation)
	return v, err
}
