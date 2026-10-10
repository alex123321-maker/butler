package actionauth

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

type PostgresStore struct{ pool *pgxpool.Pool }

func NewPostgresStore(pool *pgxpool.Pool) *PostgresStore { return &PostgresStore{pool: pool} }
func (s *PostgresStore) PutIntent(ctx context.Context, i Intent) error {
	encoded, err := json.Marshal(i.Binding)
	if err != nil {
		return err
	}
	var hash string
	err = s.pool.QueryRow(ctx, `INSERT INTO action_intents(action_id,run_id,tool_call_id,action_hash,binding,requires_approval,expires_at)
 VALUES($1,$2,$3,$4,$5::jsonb,$6,$7) ON CONFLICT(action_id) DO UPDATE SET action_id=EXCLUDED.action_id RETURNING action_hash`, i.Binding.ActionID, i.Binding.RunID, i.Binding.ToolCallID, i.Binding.ActionHash, string(encoded), i.RequiresApproval, i.ExpiresAt).Scan(&hash)
	if err != nil {
		return err
	}
	if hash != i.Binding.ActionHash {
		return fmt.Errorf("tool call identity already binds a different action")
	}
	return nil
}
func (s *PostgresStore) GetIntent(ctx context.Context, id string) (Intent, error) {
	var i Intent
	var encoded string
	err := s.pool.QueryRow(ctx, `SELECT binding::text,requires_approval,expires_at FROM action_intents WHERE action_id=$1`, id).Scan(&encoded, &i.RequiresApproval, &i.ExpiresAt)
	if err != nil {
		return i, err
	}
	err = json.Unmarshal([]byte(encoded), &i.Binding)
	return i, err
}
func (s *PostgresStore) PutGrant(ctx context.Context, g Grant) (Grant, error) {
	encoded, err := json.Marshal(g)
	if err != nil {
		return Grant{}, err
	}
	var stored string
	// Only an unused grant may be refreshed after worker takeover. Once dispatch
	// begins, even a timeout cannot turn this action into a reusable permission.
	err = s.pool.QueryRow(ctx, `INSERT INTO action_authorizations(authorization_id,action_id,action_hash,grant_data,mandate_id,mandate_revision,expires_at,state)
 VALUES($1,$2,$3,$4::jsonb,NULLIF($5,''),$6,$7,'authorized') ON CONFLICT(action_id) DO UPDATE
 SET authorization_id=EXCLUDED.authorization_id,grant_data=EXCLUDED.grant_data,mandate_id=EXCLUDED.mandate_id,mandate_revision=EXCLUDED.mandate_revision,expires_at=EXCLUDED.expires_at
 WHERE action_authorizations.state='authorized' AND action_authorizations.action_hash=EXCLUDED.action_hash RETURNING grant_data::text`, g.ID, g.Binding.ActionID, g.Binding.ActionHash, string(encoded), g.MandateID, g.MandateRevision, g.ExpiresAt).Scan(&stored)
	if err != nil {
		return Grant{}, fmt.Errorf("action authorization cannot be refreshed after dispatch")
	}
	err = json.Unmarshal([]byte(stored), &g)
	return g, err
}
func (s *PostgresStore) GetGrant(ctx context.Context, id string) (Grant, error) {
	var g Grant
	var encoded string
	var state string
	err := s.pool.QueryRow(ctx, `SELECT grant_data::text,state FROM action_authorizations WHERE authorization_id=$1`, id).Scan(&encoded, &state)
	if err != nil {
		return g, err
	}
	err = json.Unmarshal([]byte(encoded), &g)
	g.State = state
	return g, err
}
func (s *PostgresStore) ClaimGrant(ctx context.Context, id string) error {
	tag, err := s.pool.Exec(ctx, `UPDATE action_authorizations a SET state='inflight',dispatched_at=NOW()
 WHERE authorization_id=$1 AND state='authorized' AND expires_at>NOW()
 AND (mandate_id IS NULL OR EXISTS(SELECT 1 FROM action_mandates m WHERE m.mandate_id=a.mandate_id AND m.revision=a.mandate_revision AND m.revoked_at IS NULL AND m.expires_at>NOW()))`, id)
	if err != nil {
		return err
	}
	if tag.RowsAffected() != 1 {
		return fmt.Errorf("authorization already dispatched, expired or revoked; do not replay the action")
	}
	return nil
}
func (s *PostgresStore) FinishGrant(ctx context.Context, id, state string) error {
	if state != "dispatch_completed" && state != "unknown" {
		return fmt.Errorf("unsupported dispatch outcome")
	}
	tag, err := s.pool.Exec(ctx, `UPDATE action_authorizations SET state=$2,finished_at=NOW() WHERE authorization_id=$1 AND state='inflight'`, id, state)
	if err != nil {
		return err
	}
	if tag.RowsAffected() != 1 {
		return fmt.Errorf("dispatch outcome cannot be recorded")
	}
	return nil
}
func (s *PostgresStore) GetMandate(ctx context.Context, id string) (Mandate, error) {
	var m Mandate
	var encoded string
	err := s.pool.QueryRow(ctx, `SELECT mandate_data::text,revision,revoked_at FROM action_mandates WHERE mandate_id=$1`, id).Scan(&encoded, &m.Revision, &m.RevokedAt)
	if err != nil {
		return m, err
	}
	revision, revoked := m.Revision, m.RevokedAt
	err = json.Unmarshal([]byte(encoded), &m)
	m.Revision, m.RevokedAt = revision, revoked
	return m, err
}
func (s *PostgresStore) PutMandate(ctx context.Context, m Mandate) error {
	if err := ValidateMandate(m, time.Now().UTC()); err != nil {
		return err
	}
	encoded, err := json.Marshal(m)
	if err != nil {
		return err
	}
	tag, err := s.pool.Exec(ctx, `INSERT INTO action_mandates(mandate_id,owner_id,task_id,revision,mandate_data,expires_at) VALUES($1,$2,$3,$4,$5::jsonb,$6)
 ON CONFLICT(mandate_id) DO UPDATE SET revision=EXCLUDED.revision,mandate_data=EXCLUDED.mandate_data,expires_at=EXCLUDED.expires_at
 WHERE action_mandates.owner_id=EXCLUDED.owner_id AND action_mandates.task_id=EXCLUDED.task_id AND action_mandates.revision+1=EXCLUDED.revision AND action_mandates.revoked_at IS NULL`, m.ID, m.OwnerID, m.TaskID, m.Revision, string(encoded), m.ExpiresAt)
	if err != nil {
		return err
	}
	if tag.RowsAffected() != 1 {
		return fmt.Errorf("mandate owner or revision is incompatible")
	}
	return nil
}
func (s *PostgresStore) RevokeMandate(ctx context.Context, id, owner string, revision int64) error {
	tag, err := s.pool.Exec(ctx, `UPDATE action_mandates SET revoked_at=NOW(),revision=revision+1 WHERE mandate_id=$1 AND owner_id=$2 AND revision=$3 AND revoked_at IS NULL`, id, owner, revision)
	if err != nil {
		return err
	}
	if tag.RowsAffected() != 1 {
		return fmt.Errorf("mandate owner or revision is incompatible")
	}
	return nil
}
