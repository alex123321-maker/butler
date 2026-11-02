package credentials

import (
	"context"
	"encoding/json"
	"strings"
	"time"

	"github.com/butler/butler/internal/browsersso"
	"github.com/jackc/pgx/v5"
)

// SSORequest is an owner-facing conditional consent. Only the separately
// approved SSO executor can use its deferred references under a one-call claim.
type SSORequest struct {
	ID           string           `json:"id"`
	Kind         string           `json:"kind"`
	RecordID     string           `json:"record_id"`
	RecordLabel  string           `json:"record_label"`
	Purpose      string           `json:"purpose"`
	Scope        browsersso.Scope `json:"sso_scope"`
	ScopeDigest  string           `json:"scope_digest"`
	ContextEpoch int64            `json:"context_epoch"`
	State        string           `json:"state"`
	ExpiresAt    time.Time        `json:"expires_at"`
}

// The displayed digest belongs to one request, not a reusable same-scope card.
func ssoConsentDigest(p Principal, id, callID, recordID, purpose string, s browsersso.Scope, epoch, revision int64) (string, error) {
	scope, e := s.Digest(epoch, revision)
	if e != nil {
		return "", ErrVault
	}
	raw, e := json.Marshal([]any{browsersso.Consent, p.OwnerID, p.SessionKey, p.RunID, p.TaskID, p.TaskRevision, p.ConstraintsRevision, id, callID, recordID, purpose, scope})
	if e != nil {
		return "", ErrVault
	}
	return digestString(string(raw)), nil
}

// ssoPage pins the current RP generation; caller must already pin Run/Task.
// It does not authorize the future IdP navigation or release any credentials.
func ssoPage(ctx context.Context, tx pgx.Tx, p Principal, s browsersso.Scope) (int64, time.Time, error) {
	var epoch int64
	var expiry time.Time
	if tx.QueryRow(ctx, `SELECT b.epoch,b.expires_at FROM server_browser_sessions b WHERE b.browser_session_id=$1 AND b.run_id=$2 AND b.owner_id=$3 AND b.task_id=$4 AND b.page_mode AND b.mode='agent' AND b.expires_at>clock_timestamp() AND b.page_origin=$5 FOR SHARE`, s.Browser, p.RunID, p.OwnerID, p.TaskID, browsersso.Origin(s.RPURL)).Scan(&epoch, &expiry) != nil {
		return 0, time.Time{}, ErrVault
	}
	var ready bool
	if tx.QueryRow(ctx, `SELECT TRUE FROM server_browser_pages WHERE browser_session_id=$1 AND page_id=$2 AND document_id=$3 AND observation_version=$4 AND observed_epoch=$5 AND observed_url=$6 AND state='ready' AND runtime_version>0 FOR SHARE`, s.Browser, s.RPPage.ID, s.RPPage.DocumentID, s.RPPage.Version, epoch, s.RPURL).Scan(&ready) != nil || !ready {
		return 0, time.Time{}, ErrVault
	}
	return epoch, expiry, nil
}

func (v *Vault) CreateSSORequest(ctx context.Context, p Principal, callID, recordID, purpose string, scope browsersso.Scope) (SSORequest, error) {
	if scope.Validate() != nil || callID == "" || len(callID) > 256 || strings.TrimSpace(purpose) == "" || len(purpose) > 500 {
		return SSORequest{}, ErrVault
	}
	tx, e := v.Pool.Begin(ctx)
	if e != nil {
		return SSORequest{}, ErrVault
	}
	defer tx.Rollback(ctx)
	if currentTx(ctx, tx, p) != nil {
		return SSORequest{}, ErrVault
	}
	epoch, expiry, e := ssoPage(ctx, tx, p, scope)
	if e != nil {
		return SSORequest{}, ErrVault
	}
	var revision int64
	var label string
	if tx.QueryRow(ctx, `SELECT revision,title||' · '||alias FROM credential_vault WHERE id=$1 AND owner_id=$2 AND status='active' FOR SHARE`, recordID, p.OwnerID).Scan(&revision, &label) != nil {
		return SSORequest{}, ErrVault
	}
	id := randomID("ssoq_")
	digest, e := ssoConsentDigest(p, id, callID, recordID, purpose, scope, epoch, revision)
	if e != nil {
		return SSORequest{}, ErrVault
	}
	raw, _ := json.Marshal(scope)
	r := SSORequest{Kind: "sso_login", RecordID: recordID, RecordLabel: label, Purpose: purpose, Scope: scope, ScopeDigest: digest, ContextEpoch: epoch}
	// A repeat may read its exact immutable request, never alter it or its TTL.
	e = tx.QueryRow(ctx, `INSERT INTO credential_sso_requests(id,owner_id,session_key,run_id,task_id,task_revision,constraints_revision,tool_call_id,record_id,record_revision,browser_session_id,rp_page_id,context_epoch,scope,scope_digest,purpose,expires_at) VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14::jsonb,$15,$16,LEAST($17,clock_timestamp()+interval '10 minutes')) ON CONFLICT(run_id,tool_call_id) DO NOTHING RETURNING id,state,expires_at`, id, p.OwnerID, p.SessionKey, p.RunID, p.TaskID, p.TaskRevision, p.ConstraintsRevision, callID, recordID, revision, scope.Browser, scope.RPPage.ID, epoch, string(raw), digest, purpose, expiry).Scan(&r.ID, &r.State, &r.ExpiresAt)
	if e == pgx.ErrNoRows {
		e = tx.QueryRow(ctx, `SELECT id,state,expires_at,scope_digest FROM credential_sso_requests WHERE run_id=$1 AND tool_call_id=$2 AND owner_id=$3 AND session_key=$4 AND task_id=$5 AND task_revision=$6 AND constraints_revision=$7 AND record_id=$8 AND record_revision=$9 AND scope=$10::jsonb AND purpose=$11 AND expires_at>clock_timestamp()`, p.RunID, callID, p.OwnerID, p.SessionKey, p.TaskID, p.TaskRevision, p.ConstraintsRevision, recordID, revision, string(raw), purpose).Scan(&r.ID, &r.State, &r.ExpiresAt, &r.ScopeDigest)
		check, err := ssoConsentDigest(p, r.ID, callID, recordID, purpose, scope, epoch, revision)
		if e != nil || err != nil || check != r.ScopeDigest || tx.Commit(ctx) != nil {
			return SSORequest{}, ErrVault
		}
		return r, nil
	}
	if e != nil {
		return SSORequest{}, ErrVault
	}
	if _, e = tx.Exec(ctx, `INSERT INTO credential_sso_notices(request_id) VALUES($1)`, r.ID); e != nil {
		return SSORequest{}, ErrVault
	}
	if v.audit(ctx, tx, p.OwnerID, recordID, r.ID, "sso_requested") != nil || tx.Commit(ctx) != nil {
		return SSORequest{}, ErrVault
	}
	return r, nil
}

func (v *Vault) SSORequests(ctx context.Context, owner string) ([]SSORequest, error) {
	rows, e := v.Pool.Query(ctx, `SELECT q.id,q.record_id,c.title||' · '||c.alias,q.purpose,q.scope::text,q.scope_digest,q.context_epoch,q.live_state,q.expires_at FROM credential_sso_state q JOIN credential_vault c ON c.id=q.record_id AND c.owner_id=q.owner_id WHERE q.owner_id=$1 AND q.live_state='pending' ORDER BY q.created_at LIMIT 50`, owner)
	if e != nil {
		return nil, ErrVault
	}
	defer rows.Close()
	out := []SSORequest{}
	for rows.Next() {
		var r SSORequest
		var raw string
		r.Kind = "sso_login"
		if rows.Scan(&r.ID, &r.RecordID, &r.RecordLabel, &r.Purpose, &raw, &r.ScopeDigest, &r.ContextEpoch, &r.State, &r.ExpiresAt) != nil || json.Unmarshal([]byte(raw), &r.Scope) != nil || r.Scope.Validate() != nil {
			return nil, ErrVault
		}
		out = append(out, r)
	}
	if rows.Err() != nil {
		return nil, ErrVault
	}
	return out, nil
}

// OwnerRequests combines bounded metadata for the protected UI only. Model
// request-status and the old secret metadata/resolution path stay unchanged.
func (v *Vault) OwnerRequests(ctx context.Context, owner string) ([]AccessRequest, error) {
	out, e := v.Requests(ctx, owner)
	if e != nil {
		return nil, e
	}
	sso, e := v.SSORequests(ctx, owner)
	if e != nil {
		return nil, e
	}
	for _, r := range sso {
		scope := r.Scope
		out = append(out, AccessRequest{ID: r.ID, Kind: r.Kind, RecordID: r.RecordID, RecordLabel: r.RecordLabel, Purpose: r.Purpose, SSOScope: &scope, ScopeDigest: r.ScopeDigest, ContextEpoch: r.ContextEpoch, TargetOrigin: browsersso.Origin(scope.RPURL), BrowserSessionID: scope.Browser, Fields: []string{"username", "password"}, State: r.State, ExpiresAt: r.ExpiresAt})
	}
	return out, nil
}

// RespondSSO records a protected owner decision only. It does not release fields,
// dispatch POST, claim the request or authorize an existing browser tool.
func (v *Vault) RespondSSO(ctx context.Context, owner, id, recordID string, allow bool, consent, digest string) error {
	tx, e := v.Pool.Begin(ctx)
	if e != nil {
		return ErrVault
	}
	defer tx.Rollback(ctx)
	var p Principal
	var scope browsersso.Scope
	var raw, selected, storedDigest, state, callID, purpose string
	var epoch, revision int64
	var expiry time.Time
	if tx.QueryRow(ctx, `SELECT owner_id,session_key,run_id,task_id,task_revision,constraints_revision,record_id,record_revision,scope::text,scope_digest,context_epoch,expires_at,tool_call_id,purpose FROM credential_sso_requests WHERE id=$1 AND owner_id=$2`, id, owner).Scan(&p.OwnerID, &p.SessionKey, &p.RunID, &p.TaskID, &p.TaskRevision, &p.ConstraintsRevision, &selected, &revision, &raw, &storedDigest, &epoch, &expiry, &callID, &purpose) != nil || !expiry.After(time.Now()) || json.Unmarshal([]byte(raw), &scope) != nil || scope.Validate() != nil || currentTx(ctx, tx, p) != nil {
		return ErrVault
	}
	if allow {
		if consent != browsersso.Consent || digest != storedDigest || recordID != selected {
			return ErrVault
		}
		current, _, e := ssoPage(ctx, tx, p, scope)
		if e != nil || current != epoch {
			return ErrVault
		}
		var active bool
		if tx.QueryRow(ctx, `SELECT TRUE FROM credential_vault WHERE id=$1 AND owner_id=$2 AND revision=$3 AND status='active' FOR SHARE`, selected, owner, revision).Scan(&active) != nil || !active {
			return ErrVault
		}
		check, e := ssoConsentDigest(p, id, callID, selected, purpose, scope, epoch, revision)
		if e != nil || check != storedDigest {
			return ErrVault
		}
	}
	if tx.QueryRow(ctx, `SELECT state FROM credential_sso_requests WHERE id=$1 AND owner_id=$2 FOR UPDATE`, id, owner).Scan(&state) != nil {
		return ErrVault
	}
	// A competing answer/transaction can hold this row past its TTL. Re-read
	// database time after acquiring the lock, not just before waiting for it.
	var live bool
	if tx.QueryRow(ctx, `SELECT expires_at>clock_timestamp() FROM credential_sso_requests WHERE id=$1`, id).Scan(&live) != nil || !live {
		return ErrVault
	}
	target := "denied"
	if allow {
		target = "approved"
	}
	if state == target {
		return tx.Commit(ctx)
	}
	if state != "pending" {
		return ErrVault
	}
	if e = tx.QueryRow(ctx, `UPDATE credential_sso_requests SET state=$2 WHERE id=$1 AND expires_at>clock_timestamp() RETURNING id`, id, target).Scan(&id); e != nil {
		return ErrVault
	}
	if v.AfterResponse != nil && v.AfterResponse(ctx, tx, p.RunID, id) != nil {
		return ErrVault
	}
	if v.audit(ctx, tx, owner, selected, id, "sso_"+target) != nil || tx.Commit(ctx) != nil {
		return ErrVault
	}
	return nil
}
