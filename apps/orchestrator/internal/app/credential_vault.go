package app

import (
	"context"
	"encoding/json"
	"errors"
	"os"

	"github.com/butler/butler/apps/orchestrator/internal/channel/telegram"
	durableinput "github.com/butler/butler/apps/orchestrator/internal/input"
	runstore "github.com/butler/butler/apps/orchestrator/internal/run"
	"github.com/butler/butler/apps/orchestrator/internal/task"
	"github.com/butler/butler/internal/actionauth"
	"github.com/butler/butler/internal/communicationcontrol"
	"github.com/butler/butler/internal/credentials"
	"github.com/butler/butler/internal/ownership"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"time"
)

type CredentialInteractions struct {
	Vault  *credentials.Vault
	Inputs *durableinput.Store
	Notify func(context.Context, credentials.AccessRequest, string) (string, error)
}

func (c *CredentialInteractions) configureTelegram(channel *telegramCommunication) {
	if c == nil || channel == nil || os.Getenv("BUTLER_PUBLIC_WEB_URL") == "" {
		return
	}
	base := os.Getenv("BUTLER_PUBLIC_WEB_URL")
	c.Notify = func(ctx context.Context, r credentials.AccessRequest, ack string) (string, error) {
		tx, err := c.Vault.Pool.Begin(ctx)
		if err != nil {
			return "", credentials.ErrVault
		}
		defer tx.Rollback(ctx)
		var owner, runID string
		if tx.QueryRow(ctx, `SELECT owner_id,run_id FROM credential_vault_requests WHERE id=$1`, r.ID).Scan(&owner, &runID) != nil || owner != channel.binding.OwnerID {
			return "", credentials.ErrVault
		}
		if channel.policy.check(ctx, tx, runID, channel.policy.binding, communicationcontrol.Args{}) != nil {
			return "", credentials.ErrVault
		}
		if tx.Commit(ctx) != nil {
			return "", credentials.ErrVault
		}
		return channel.client.CredentialNotice(ctx, channel.binding, r, base, ack)
	}
}

func (c *CredentialInteractions) configureLegacyTelegram(channel *telegram.Adapter) {
	if c == nil || channel == nil || c.Notify != nil || os.Getenv("BUTLER_PUBLIC_WEB_URL") == "" {
		return
	}
	base := os.Getenv("BUTLER_PUBLIC_WEB_URL")
	c.Notify = func(ctx context.Context, r credentials.AccessRequest, ack string) (string, error) {
		var session string
		if c.Vault.Pool.QueryRow(ctx, `SELECT q.session_key FROM credential_vault_requests q JOIN sessions s ON s.session_key=q.session_key WHERE q.id=$1 AND q.owner_id=s.user_id`, r.ID).Scan(&session) != nil {
			return "", credentials.ErrVault
		}
		return channel.CredentialNotice(ctx, session, r, base, ack)
	}
}

// Sweep expires secret material and wakes the existing owning Input. Notice
// claims are durable; an ambiguous send/edit is never automatically replayed.
func (c *CredentialInteractions) Sweep(ctx context.Context) error {
	if c == nil {
		return nil
	}
	tx, err := c.Vault.Pool.Begin(ctx)
	if err != nil {
		return credentials.ErrVault
	}
	defer tx.Rollback(ctx)
	if _, err = tx.Exec(ctx, `DELETE FROM credential_vault_otps WHERE expires_at<=now()`); err != nil {
		return credentials.ErrVault
	}
	rows, err := tx.Query(ctx, `UPDATE credential_vault_requests q SET state='expired' WHERE q.state IN ('pending','approved') AND (q.expires_at<=now() OR EXISTS(SELECT 1 FROM tasks t WHERE t.task_id=q.task_id AND (t.revision<>q.task_revision OR t.constraints_revision<>q.constraints_revision OR t.status IN ('cancelled','completed','blocked')))) RETURNING run_id,id`)
	if err != nil {
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
	if rows.Err() != nil {
		rows.Close()
		return credentials.ErrVault
	}
	rows.Close()
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
	if _, err = c.Vault.Pool.Exec(ctx, `UPDATE credential_vault_requests SET notice_state='unknown' WHERE notice_state='sending' AND notice_started_at<now()-interval '1 minute'`); err != nil {
		return credentials.ErrVault
	}
	var r credentials.AccessRequest
	var raw, ack, phase string
	err = c.Vault.Pool.QueryRow(ctx, `UPDATE credential_vault_requests q SET notice_state='sending',notice_phase=q.state,notice_started_at=now() WHERE id=(SELECT x.id FROM credential_vault_requests x WHERE (x.notice_state='pending' AND x.state='pending' AND EXISTS(SELECT 1 FROM accepted_inputs i WHERE i.run_id=x.run_id AND i.checkpoint_json->'interaction'->>'interaction_id'=x.id)) OR (x.notice_state='sent' AND x.notice_phase<>x.state) ORDER BY x.created_at FOR UPDATE OF x SKIP LOCKED LIMIT 1) RETURNING id,kind,record_id,target_origin,browser_session_id,purpose,fields::text,state,expires_at,notice_ack,notice_phase`).Scan(&r.ID, &r.Kind, &r.RecordID, &r.TargetOrigin, &r.BrowserSessionID, &r.Purpose, &raw, &r.State, &r.ExpiresAt, &ack, &phase)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil
	}
	if err != nil || json.Unmarshal([]byte(raw), &r.Fields) != nil {
		return credentials.ErrVault
	}
	if r.RecordID != "" {
		// Display safe account metadata so the owner can identify the requested
		// entry before following the protected form link.
		_ = c.Vault.Pool.QueryRow(ctx, `SELECT title||' · '||alias FROM credential_vault WHERE id=$1 AND owner_id=(SELECT owner_id FROM credential_vault_requests WHERE id=$2)`, r.RecordID, r.ID).Scan(&r.RecordLabel)
	}
	result, sendErr := c.Notify(ctx, r, ack)
	state := "sent"
	if sendErr != nil {
		state = "unknown"
		result = ack
	}
	_, err = c.Vault.Pool.Exec(ctx, `UPDATE credential_vault_requests SET notice_state=$2,notice_ack=$3 WHERE id=$1 AND notice_state='sending' AND notice_phase=$4`, r.ID, state, result, phase)
	return err
}

func credentialInteractions(pool *pgxpool.Pool) *CredentialInteractions {
	v, err := credentials.VaultFromEnvironment(pool)
	if err != nil {
		return nil
	}
	c := &CredentialInteractions{Vault: v, Inputs: durableinput.NewStore(pool)}
	v.AfterResponse = c.readyTx
	return c
}
func (c *CredentialInteractions) readyTx(ctx context.Context, tx pgx.Tx, runID, id string) error {
	var inputID string
	var paused bool
	err := tx.QueryRow(ctx, `SELECT i.input_id,COALESCE(t.status='paused',false) FROM accepted_inputs i JOIN runs r ON r.run_id=i.run_id LEFT JOIN tasks t ON t.task_id=r.task_id WHERE i.run_id=$1 AND i.checkpoint='awaiting_interaction' AND i.checkpoint_json->'interaction'->>'interaction_id'=$2 AND i.status IN ('running','waiting_for_user','blocked') FOR UPDATE OF i`, runID, id).Scan(&inputID, &paused)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil
	}
	if err != nil {
		return credentials.ErrVault
	}
	return c.Inputs.ReadyInteractionTx(ctx, tx, inputID, id, paused)
}
func (c *CredentialInteractions) SuspendRequest(ctx context.Context, runID, callID string) (string, error) {
	if c == nil {
		return "", credentials.ErrVault
	}
	token, ok := ownership.TokenFromContext(ctx)
	if !ok || token.RunID != runID {
		return "", credentials.ErrVault
	}
	tx, err := c.Vault.Pool.Begin(ctx)
	if err != nil {
		return "", credentials.ErrVault
	}
	defer tx.Rollback(ctx)
	if checkCredentialLease(ctx, tx, token) != nil {
		return "", credentials.ErrVault
	}
	var id, taskID, state string
	var rev, cr int64
	if tx.QueryRow(ctx, `SELECT id,task_id,task_revision,constraints_revision,state FROM credential_vault_requests WHERE run_id=$1 AND tool_call_id=$2 AND session_key=$3 AND expires_at>now() FOR UPDATE`, runID, callID, token.SessionKey).Scan(&id, &taskID, &rev, &cr, &state) != nil {
		return "", credentials.ErrVault
	}
	if task.WaitInteractionTx(ctx, tx, taskID, rev, cr) != nil || c.Inputs.SuspendPrivateInteractionTx(ctx, tx, runID, id, "credential_interaction") != nil || runstore.SuspendInteractionTx(ctx, tx, runID) != nil {
		return "", credentials.ErrVault
	}
	if state != "pending" && c.readyTx(ctx, tx, runID, id) != nil {
		return "", credentials.ErrVault
	}
	if tx.Commit(ctx) != nil {
		return "", credentials.ErrVault
	}
	return id, nil
}
func (c *CredentialInteractions) ResumeInteraction(ctx context.Context, runID, id string) (string, error) {
	if c == nil {
		return "", credentials.ErrVault
	}
	token, ok := ownership.TokenFromContext(ctx)
	if !ok || token.RunID != runID {
		return "", credentials.ErrVault
	}
	tx, err := c.Vault.Pool.Begin(ctx)
	if err != nil {
		return "", credentials.ErrVault
	}
	defer tx.Rollback(ctx)
	if checkCredentialLease(ctx, tx, token) != nil {
		return "", credentials.ErrVault
	}
	var owner, taskID, state, fields string
	var rev, cr int64
	if tx.QueryRow(ctx, `SELECT owner_id,task_id,task_revision,constraints_revision,CASE WHEN expires_at<=now() THEN 'expired' ELSE state END,fields::text FROM credential_vault_requests WHERE id=$1 AND run_id=$2 AND session_key=$3 FOR SHARE`, id, runID, token.SessionKey).Scan(&owner, &taskID, &rev, &cr, &state, &fields) != nil || state == "pending" {
		return "", credentials.ErrVault
	}
	if task.ResumeInteractionTx(ctx, tx, taskID, rev, cr) != nil || c.Inputs.CheckInteractionClaimTx(ctx, tx, runID, id) != nil {
		return "", credentials.ErrVault
	}
	if taskID != "" {
		if _, err = actionauth.CheckTaskAuthorityTx(ctx, tx, taskID, owner, nil, time.Now()); err != nil {
			return "", credentials.ErrVault
		}
	}
	if tx.Commit(ctx) != nil {
		return "", credentials.ErrVault
	}
	raw, _ := json.Marshal(map[string]any{"request_id": id, "state": state, "credential_alias": id, "fields": json.RawMessage(fields)})
	return string(raw), nil
}

func checkCredentialLease(ctx context.Context, tx pgx.Tx, t ownership.Token) error {
	var valid bool
	if tx.QueryRow(ctx, `SELECT TRUE FROM session_ownership WHERE session_key=$1 AND run_id=$2 AND lease_id=$3 AND fence_epoch=$4 AND expires_at>now() FOR SHARE`, t.SessionKey, t.RunID, t.LeaseID, t.FenceEpoch).Scan(&valid) != nil {
		return credentials.ErrVault
	}
	return nil
}
