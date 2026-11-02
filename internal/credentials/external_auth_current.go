package credentials

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/hex"
	"encoding/json"
	"time"

	"github.com/butler/butler/internal/actionauth"
	"github.com/butler/butler/internal/externalauth"
	"github.com/butler/butler/internal/jsonstrict"
	"github.com/jackc/pgx/v5"
)

func (v *Vault) loadExternalTx(ctx context.Context, tx pgx.Tx, id string) (externalRequest, error) {
	var q externalRequest
	if !validExternalID(id) {
		return q, ErrVault
	}
	var raw string
	e := tx.QueryRow(ctx, `SELECT id,owner_id,session_key,run_id,input_id,task_id,task_revision,constraints_revision,tool_call_id,source_action_id,record_id,record_revision,context_epoch,profile_fingerprint,scope::text,scope_digest,purpose,state,created_at,expires_at FROM browser_external_auth_requests WHERE id=$1`, id).Scan(&q.id, &q.p.OwnerID, &q.p.SessionKey, &q.p.RunID, &q.input, &q.p.TaskID, &q.p.TaskRevision, &q.p.ConstraintsRevision, &q.call, &q.source, &q.record, &q.revision, &q.epoch, &q.profile, &raw, &q.digest, &q.purpose, &q.state, &q.created, &q.expires)
	if e != nil || jsonstrict.Decode([]byte(raw), &q.scope) != nil || q.scope.Validate() != nil || q.digest != externalDigest(q) {
		return externalRequest{}, ErrVault
	}
	return q, nil
}

// No worker lease is borrowed during an owner/browser callback. The immutable
// canonical Input wait, current Task/mandate/account, source page fence, exact
// dispatched request and operator profile are checked on the same transaction.
func (v *Vault) externalCurrentTx(ctx context.Context, tx pgx.Tx, q externalRequest, commit bool) error {
	if currentTx(ctx, tx, q.p) != nil || q.p.TaskID == "" {
		return ErrVault
	}
	var b actionauth.ActionBinding
	var raw string
	if tx.QueryRow(ctx, `SELECT i.binding::text FROM action_intents i JOIN action_effects e ON e.action_id=i.effect_id JOIN action_effect_dispatches d ON d.execution_action_id=i.action_id AND d.authorization_id=e.authorization_id WHERE i.action_id=$1 AND i.run_id=$2 AND i.tool_call_id=$3 AND e.state='effect_confirmed' AND d.outcome='dispatch_completed' FOR SHARE OF i,e,d`, q.source, q.p.RunID, q.call).Scan(&raw) != nil || jsonstrict.Decode([]byte(raw), &b) != nil || b.ToolName != externalauth.RequestTool || b.OwnerID != q.p.OwnerID || b.TaskID != q.p.TaskID || b.TaskRevision != q.p.TaskRevision || b.ConstraintsRevision != q.p.ConstraintsRevision || b.Effect != actionauth.EffectPrepare || len(b.CredentialRefs) != 0 {
		return ErrVault
	}
	args, _ := json.Marshal(externalauth.Request{RecordID: q.record, Purpose: q.purpose, Scope: q.scope})
	h, e := actionauth.ArgumentsHash(string(args))
	if e != nil || h != b.ArgsHash || b.GoalID == "" || b.MandateID == "" || b.MandateRevision <= 0 || b.ExternalAuth == nil || *b.ExternalAuth != q.scope || actionauth.CheckTaskActionTx(ctx, tx, b, time.Now()) != nil {
		return ErrVault
	}
	// Consent is not a substitute for the mandate's commit effect/tool/path
	// permission. Check all destinations, including the fixed provider backend.
	if commit {
		b.Effect = actionauth.EffectCommit
		if actionauth.CheckTaskActionTx(ctx, tx, b, time.Now()) != nil {
			return ErrVault
		}
	}
	var valid bool
	if tx.QueryRow(ctx, `SELECT TRUE FROM accepted_inputs WHERE input_id=$1 AND run_id=$2 AND session_key=$3 AND owner_id=$4 AND task_id=$5 AND checkpoint='awaiting_interaction' AND checkpoint_json->'interaction'->>'interaction_id'=$6 AND status IN ('running','waiting_for_user','blocked') AND expires_at>clock_timestamp() FOR UPDATE`, q.input, q.p.RunID, q.p.SessionKey, q.p.OwnerID, q.p.TaskID, q.id).Scan(&valid) != nil || !valid {
		return ErrVault
	}
	p, e := v.externalProfileTx(ctx, tx)
	if e != nil || p.ClientFingerprint != q.profile || !p.Matches(q.scope) {
		return ErrVault
	}
	if tx.QueryRow(ctx, `SELECT TRUE FROM credential_vault WHERE id=$1 AND owner_id=$2 AND revision=$3 AND status='active' FOR SHARE`, q.record, q.p.OwnerID, q.revision).Scan(&valid) != nil || !valid {
		return ErrVault
	}
	if tx.QueryRow(ctx, `SELECT TRUE FROM server_browser_sessions b JOIN server_browser_pages p ON p.browser_session_id=b.browser_session_id WHERE b.browser_session_id=$1 AND b.owner_id=$2 AND b.run_id=$3 AND b.task_id=$4 AND b.epoch=$5 AND b.page_mode AND b.mode='agent' AND b.expires_at>clock_timestamp() AND p.page_id=$6 AND p.document_id=$7 AND p.observation_version=$8 AND p.observed_epoch=b.epoch AND p.observed_url=$9 AND p.state='ready' AND p.runtime_version>0 FOR SHARE OF b,p`, q.scope.Browser, q.p.OwnerID, q.p.RunID, q.p.TaskID, q.epoch, q.scope.RPPage.ID, q.scope.RPPage.DocumentID, q.scope.RPPage.Version, q.scope.RPURL).Scan(&valid) != nil || !valid {
		return ErrVault
	}
	// Recheck expiry against database time after all potentially blocking locks.
	var lockedID string
	if tx.QueryRow(ctx, `SELECT id FROM browser_external_auth_requests WHERE id=$1 FOR UPDATE`, q.id).Scan(&lockedID) != nil || lockedID != q.id {
		return ErrVault
	}
	var at time.Time
	if tx.QueryRow(ctx, `SELECT clock_timestamp()`).Scan(&at) != nil || !q.expires.After(at) || actionauth.CheckTaskActionTx(ctx, tx, b, at) != nil {
		return ErrVault
	}
	var closed bool
	if tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM browser_external_auth_closures WHERE request_id=$1)`, q.id).Scan(&closed) != nil || closed {
		return ErrVault
	}
	if tx.QueryRow(ctx, `SELECT expires_at>clock_timestamp() FROM accepted_inputs WHERE input_id=$1`, q.input).Scan(&valid) != nil || !valid {
		return ErrVault
	}
	return nil
}

func (v *Vault) matchExternalIdentityTx(ctx context.Context, tx pgx.Tx, q externalRequest, identity externalauth.VerifiedIdentity) error {
	if identity.Subject == "" || identity.Email == "" || !identity.AuthoritativeEmail || !identity.ExpiresAt.After(time.Now()) {
		return ErrVault
	}
	var claimed bool
	if v.externalCurrentTx(ctx, tx, q, true) != nil || tx.QueryRow(ctx, `SELECT TRUE FROM browser_external_auth_exchanges WHERE request_id=$1 FOR SHARE`, q.id).Scan(&claimed) != nil || !claimed {
		return ErrVault
	}
	var index string
	if tx.QueryRow(ctx, `SELECT login_index FROM credential_vault WHERE id=$1 AND owner_id=$2 AND revision=$3 AND status='active' FOR SHARE`, q.record, q.p.OwnerID, q.revision).Scan(&index) != nil || subtle.ConstantTimeCompare([]byte(index), []byte(v.index(q.p.OwnerID, identity.Email))) != 1 {
		return ErrVault
	}
	// An email match can establish a first binding; subsequent authentications
	// must have the same stable provider subject even after record edits.
	subject := v.externalSubjectIndex(q.p.OwnerID, q.profile, q.record, identity.Subject)
	var retained string
	e := tx.QueryRow(ctx, `INSERT INTO browser_external_auth_identities(owner_id,record_id,profile_fingerprint,subject_index) VALUES($1,$2,$3,$4) ON CONFLICT DO NOTHING RETURNING subject_index`, q.p.OwnerID, q.record, q.profile, subject).Scan(&retained)
	if e == pgx.ErrNoRows {
		e = tx.QueryRow(ctx, `SELECT subject_index FROM browser_external_auth_identities WHERE owner_id=$1 AND record_id=$2 AND profile_fingerprint=$3 FOR SHARE`, q.p.OwnerID, q.record, q.profile).Scan(&retained)
	}
	if e != nil || subtle.ConstantTimeCompare([]byte(subject), []byte(retained)) != 1 {
		return ErrVault
	}
	return nil
}

func (v *Vault) externalSubjectIndex(owner, profile, record, subject string) string {
	m := hmac.New(sha256.New, []byte(v.key))
	m.Write([]byte("external-google-subject-v1\x00" + owner + "\x00" + profile + "\x00" + record + "\x00" + subject))
	return hex.EncodeToString(m.Sum(nil))
}
