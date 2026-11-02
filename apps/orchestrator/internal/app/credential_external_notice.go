package app

import (
	"context"
	"encoding/json"
	"errors"

	"github.com/butler/butler/internal/credentials"
	"github.com/butler/butler/internal/externalauth"
	"github.com/jackc/pgx/v5"
)

// Notice delivery is separate from consent, authentication and Input delivery.
// A lost channel acknowledgement is retained and never automatically resent.
func (c *CredentialInteractions) notifyExternal(ctx context.Context) error {
	if c.Notify == nil {
		return nil
	}
	var exists bool
	if c.Vault.Pool.QueryRow(ctx, `SELECT to_regclass('browser_external_auth_notices') IS NOT NULL`).Scan(&exists) != nil {
		return credentials.ErrVault
	}
	if !exists {
		return nil
	}
	if _, e := c.Vault.Pool.Exec(ctx, `UPDATE browser_external_auth_notices SET notice_state='unknown' WHERE notice_state='sending' AND notice_started_at<clock_timestamp()-interval '1 minute'`); e != nil {
		return credentials.ErrVault
	}
	var r credentials.AccessRequest
	var raw, ack, phase string
	e := c.Vault.Pool.QueryRow(ctx, `WITH chosen AS (
 SELECT n.request_id,v.live_state FROM browser_external_auth_notices n JOIN browser_external_auth_state v ON v.id=n.request_id
 WHERE (n.notice_state='pending' AND v.live_state='pending' AND EXISTS(SELECT 1 FROM accepted_inputs i WHERE i.input_id=v.input_id AND i.run_id=v.run_id AND i.checkpoint='awaiting_interaction' AND i.checkpoint_json->'interaction'->>'interaction_id'=v.id)) OR (n.notice_state='sent' AND n.notice_phase<>v.live_state)
 ORDER BY v.created_at FOR UPDATE OF n SKIP LOCKED LIMIT 1
 ), claimed AS (
 UPDATE browser_external_auth_notices n SET notice_state='sending',notice_phase=c.live_state,notice_started_at=clock_timestamp() FROM chosen c WHERE n.request_id=c.request_id RETURNING n.*
 ) SELECT q.id,q.record_id,q.purpose,q.scope::text,q.scope_digest,q.context_epoch,q.live_state,q.expires_at,n.notice_ack,n.notice_phase,c.title||' · '||c.alias FROM claimed n JOIN browser_external_auth_state q ON q.id=n.request_id JOIN credential_vault c ON c.id=q.record_id AND c.owner_id=q.owner_id`).Scan(&r.ID, &r.RecordID, &r.Purpose, &raw, &r.ScopeDigest, &r.ContextEpoch, &r.State, &r.ExpiresAt, &ack, &phase, &r.RecordLabel)
	if errors.Is(e, pgx.ErrNoRows) {
		return nil
	}
	var scope externalauth.Scope
	if e != nil || json.Unmarshal([]byte(raw), &scope) != nil || scope.Validate() != nil {
		return credentials.ErrVault
	}
	r.Kind = "external_login"
	r.ExternalScope = &scope
	r.TargetOrigin = scope.RPURL
	r.BrowserSessionID = scope.Browser
	r.Fields = []string{}
	result, sendErr := c.Notify(ctx, r, ack)
	state := "sent"
	if sendErr != nil {
		state = "unknown"
		result = ack
	}
	_, e = c.Vault.Pool.Exec(ctx, `UPDATE browser_external_auth_notices SET notice_state=$2,notice_ack=$3 WHERE request_id=$1 AND notice_state='sending' AND notice_phase=$4`, r.ID, state, result, phase)
	return e
}
