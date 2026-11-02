package app

import (
	"context"
	"encoding/json"
	"errors"

	"github.com/butler/butler/internal/browsersso"
	"github.com/butler/butler/internal/credentials"
	"github.com/jackc/pgx/v5"
)

// Wake only the canonical already-suspended Input. Readiness grants no new
// action and ResumeInteraction still checks the current claim/lease/authority.
func (c *CredentialInteractions) sweepSSO(ctx context.Context) error {
	tx, e := c.Vault.Pool.Begin(ctx)
	if e != nil {
		return credentials.ErrVault
	}
	defer tx.Rollback(ctx)
	rows, e := tx.Query(ctx, `SELECT q.run_id,q.id FROM credential_sso_state q JOIN accepted_inputs i ON i.run_id=q.run_id AND i.checkpoint_json->'interaction'->>'interaction_id'=q.id WHERE q.live_state<>'pending' AND i.checkpoint='awaiting_interaction' AND i.status IN ('running','waiting_for_user','blocked') FOR UPDATE OF i SKIP LOCKED`)
	if e != nil {
		return credentials.ErrVault
	}
	type wait struct{ run, id string }
	var waits []wait
	for rows.Next() {
		var w wait
		if rows.Scan(&w.run, &w.id) != nil {
			rows.Close()
			return credentials.ErrVault
		}
		waits = append(waits, w)
	}
	rows.Close()
	if rows.Err() != nil {
		return credentials.ErrVault
	}
	for _, w := range waits {
		if c.readyTx(ctx, tx, w.run, w.id) != nil {
			return credentials.ErrVault
		}
	}
	if tx.Commit(ctx) != nil {
		return credentials.ErrVault
	}
	if c.Notify == nil {
		return nil
	}
	if _, e = c.Vault.Pool.Exec(ctx, `UPDATE credential_sso_notices SET notice_state='unknown' WHERE notice_state='sending' AND notice_started_at<now()-interval '1 minute'`); e != nil {
		return credentials.ErrVault
	}
	var r credentials.AccessRequest
	var raw, ack, phase string
	e = c.Vault.Pool.QueryRow(ctx, `WITH chosen AS (
 SELECT n.request_id,v.live_state FROM credential_sso_notices n JOIN credential_sso_state v ON v.id=n.request_id
 WHERE (n.notice_state='pending' AND v.live_state='pending' AND EXISTS(SELECT 1 FROM accepted_inputs i WHERE i.run_id=v.run_id AND i.checkpoint_json->'interaction'->>'interaction_id'=v.id)) OR (n.notice_state='sent' AND n.notice_phase<>v.live_state)
 ORDER BY v.created_at FOR UPDATE OF n SKIP LOCKED LIMIT 1
 ), claimed AS (
 UPDATE credential_sso_notices n SET notice_state='sending',notice_phase=c.live_state,notice_started_at=now() FROM chosen c WHERE n.request_id=c.request_id RETURNING n.*
 ) SELECT q.id,q.record_id,q.purpose,q.scope::text,q.scope_digest,q.context_epoch,q.live_state,q.expires_at,n.notice_ack,n.notice_phase FROM claimed n JOIN credential_sso_state q ON q.id=n.request_id`).Scan(&r.ID, &r.RecordID, &r.Purpose, &raw, &r.ScopeDigest, &r.ContextEpoch, &r.State, &r.ExpiresAt, &ack, &phase)
	if errors.Is(e, pgx.ErrNoRows) {
		return nil
	}
	if e != nil {
		return credentials.ErrVault
	}
	var scope browsersso.Scope
	if json.Unmarshal([]byte(raw), &scope) != nil || scope.Validate() != nil {
		return credentials.ErrVault
	}
	r.Kind = "sso_login"
	r.SSOScope = &scope
	r.TargetOrigin = browsersso.Origin(scope.RPURL)
	r.BrowserSessionID = scope.Browser
	r.Fields = []string{"username", "password"}
	_ = c.Vault.Pool.QueryRow(ctx, `SELECT c.title||' · '||c.alias FROM credential_vault c JOIN credential_sso_requests q ON q.record_id=c.id AND q.owner_id=c.owner_id WHERE q.id=$1`, r.ID).Scan(&r.RecordLabel)
	result, sendErr := c.Notify(ctx, r, ack)
	state := "sent"
	if sendErr != nil {
		state = "unknown"
		result = ack
	}
	_, e = c.Vault.Pool.Exec(ctx, `UPDATE credential_sso_notices SET notice_state=$2,notice_ack=$3 WHERE request_id=$1 AND notice_state='sending' AND notice_phase=$4`, r.ID, state, result, phase)
	return e
}
