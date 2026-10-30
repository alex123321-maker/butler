package credentials

import (
	"context"
	"crypto/subtle"
	"encoding/json"
	"github.com/butler/butler/internal/browserauth"
	"net/url"
	"strings"
	"time"

	toolbrokerv1 "github.com/butler/butler/internal/gen/toolbroker/v1"
	"github.com/jackc/pgx/v5"
)

type Principal struct {
	OwnerID, SessionKey, RunID, TaskID string
	TaskRevision, ConstraintsRevision  int64
}
type AccessRequest struct {
	LoginScope       *browserauth.Scope `json:"login_scope,omitempty"`
	ID               string             `json:"id"`
	Kind             string             `json:"kind"`
	RecordID         string             `json:"record_id"`
	RecordLabel      string             `json:"record_label,omitempty"`
	TargetOrigin     string             `json:"target_origin"`
	BrowserSessionID string             `json:"browser_session_id"`
	Purpose          string             `json:"purpose"`
	Fields           []string           `json:"fields"`
	State            string             `json:"state"`
	ExpiresAt        time.Time          `json:"expires_at"`
}

// currentTx pins owner/session/task versions; caller identity is never model input.
func currentTx(ctx context.Context, tx pgx.Tx, p Principal) error {
	var owner string
	if tx.QueryRow(ctx, `SELECT s.user_id FROM sessions s JOIN runs r USING(session_key) WHERE r.run_id=$1 AND s.session_key=$2 AND s.user_id=$3 AND COALESCE(r.task_id,'')=$4 AND r.current_state NOT IN ('completed','failed','cancelled','timed_out') FOR SHARE OF s,r`, p.RunID, p.SessionKey, p.OwnerID, p.TaskID).Scan(&owner) != nil {
		return ErrVault
	}
	if p.TaskID != "" {
		var id string
		if tx.QueryRow(ctx, `SELECT task_id FROM tasks WHERE task_id=$1 AND owner_id=$2 AND revision=$3 AND constraints_revision=$4 AND status NOT IN ('completed','cancelled','paused','blocked') FOR SHARE`, p.TaskID, p.OwnerID, p.TaskRevision, p.ConstraintsRevision).Scan(&id) != nil {
			return ErrVault
		}
	}
	return nil
}
func (v *Vault) CreateRequest(ctx context.Context, p Principal, callID, kind, recordID, target, browser, purpose string, fields []string, parents ...string) (AccessRequest, error) {
	return v.createRequest(ctx, p, callID, kind, recordID, target, browser, purpose, fields, parents, nil)
}

func (v *Vault) CreateLoginRequest(ctx context.Context, p Principal, callID, recordID, purpose string, scope browserauth.Scope) (AccessRequest, error) {
	if scope.Validate() != nil {
		return AccessRequest{}, ErrVault
	}
	return v.createRequest(ctx, p, callID, "login", recordID, scope.ExpectedURL, scope.Browser, purpose, []string{"username", "password"}, nil, &scope)
}

func (v *Vault) createRequest(ctx context.Context, p Principal, callID, kind, recordID, target, browser, purpose string, fields []string, parents []string, loginScope *browserauth.Scope) (AccessRequest, error) {
	origin, err := Origin(target)
	if err != nil || browser == "" || len(browser) > 256 || strings.TrimSpace(purpose) == "" || len(purpose) > 500 || len(fields) == 0 || len(fields) > 2 {
		return AccessRequest{}, ErrVault
	}
	if kind != "access" && kind != "create" && kind != "otp" && kind != "login" || (kind == "login") != (loginScope != nil) {
		return AccessRequest{}, ErrVault
	}
	seen := map[string]bool{}
	for _, f := range fields {
		if (f != "username" && f != "password" && f != "otp") || seen[f] || ((kind == "otp") != (f == "otp")) {
			return AccessRequest{}, ErrVault
		}
		seen[f] = true
	}
	tx, err := v.Pool.Begin(ctx)
	if err != nil {
		return AccessRequest{}, ErrVault
	}
	defer tx.Rollback(ctx)
	if currentTx(ctx, tx, p) != nil {
		return AccessRequest{}, ErrVault
	}
	var rev int64
	parentID := ""
	if len(parents) > 1 {
		return AccessRequest{}, ErrVault
	}
	if len(parents) == 1 {
		parentID = parents[0]
	}
	if kind == "access" || kind == "login" {
		if tx.QueryRow(ctx, `SELECT revision FROM credential_vault WHERE id=$1 AND owner_id=$2 AND status='active' FOR SHARE`, recordID, p.OwnerID).Scan(&rev) != nil {
			return AccessRequest{}, ErrVault
		}
	} else if recordID != "" {
		return AccessRequest{}, ErrVault
	}
	if parentID != "" {
		if kind != "otp" || tx.QueryRow(ctx, `SELECT p.record_id,p.record_revision FROM credential_vault_requests p JOIN credential_vault c ON c.id=p.record_id AND c.owner_id=p.owner_id WHERE p.id=$1 AND p.owner_id=$2 AND p.run_id=$3 AND p.browser_session_id=$4 AND p.target_origin=$5 AND p.session_key=$6 AND p.task_id=$7 AND p.task_revision=$8 AND p.constraints_revision=$9 AND p.state='approved' AND p.kind IN ('access','create') AND p.fields ? 'password' AND p.expires_at>now() AND c.status='active' AND c.revision=p.record_revision FOR SHARE OF p,c`, parentID, p.OwnerID, p.RunID, browser, origin, p.SessionKey, p.TaskID, p.TaskRevision, p.ConstraintsRevision).Scan(&recordID, &rev) != nil {
			return AccessRequest{}, ErrVault
		}
	}
	raw, _ := json.Marshal(fields)
	scopeJSON := []byte(`{}`)
	if loginScope != nil {
		scopeJSON, _ = json.Marshal(loginScope)
	}
	var r AccessRequest
	var encoded string
	err = tx.QueryRow(ctx, `INSERT INTO credential_vault_requests(id,owner_id,session_key,run_id,task_id,task_revision,constraints_revision,tool_call_id,kind,record_id,record_revision,target_origin,browser_session_id,purpose,fields,expires_at,login_scope) VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14,$15::jsonb,now()+interval '10 minutes',$16::jsonb) ON CONFLICT(run_id,tool_call_id) DO UPDATE SET tool_call_id=EXCLUDED.tool_call_id WHERE credential_vault_requests.kind=EXCLUDED.kind AND credential_vault_requests.record_id=EXCLUDED.record_id AND credential_vault_requests.target_origin=EXCLUDED.target_origin AND credential_vault_requests.browser_session_id=EXCLUDED.browser_session_id AND credential_vault_requests.fields=EXCLUDED.fields AND credential_vault_requests.purpose=EXCLUDED.purpose AND credential_vault_requests.login_scope=EXCLUDED.login_scope RETURNING id,kind,record_id,target_origin,browser_session_id,purpose,fields::text,state,expires_at`, randomID("creq_"), p.OwnerID, p.SessionKey, p.RunID, p.TaskID, p.TaskRevision, p.ConstraintsRevision, callID, kind, recordID, rev, origin, browser, purpose, string(raw), string(scopeJSON)).Scan(&r.ID, &r.Kind, &r.RecordID, &r.TargetOrigin, &r.BrowserSessionID, &r.Purpose, &encoded, &r.State, &r.ExpiresAt)
	if err != nil || json.Unmarshal([]byte(encoded), &r.Fields) != nil {
		return AccessRequest{}, ErrVault
	}
	r.LoginScope = loginScope
	if parentID != "" {
		if err = tx.QueryRow(ctx, `UPDATE credential_vault_requests q SET parent_request_id=$2,expires_at=LEAST(q.expires_at,p.expires_at) FROM credential_vault_requests p WHERE q.id=$1 AND p.id=$2 AND (q.parent_request_id='' OR q.parent_request_id=$2) RETURNING q.expires_at`, r.ID, parentID).Scan(&r.ExpiresAt); err != nil {
			return AccessRequest{}, ErrVault
		}
	}
	if v.audit(ctx, tx, p.OwnerID, recordID, r.ID, "requested") != nil || tx.Commit(ctx) != nil {
		return AccessRequest{}, ErrVault
	}
	return r, nil
}
func (v *Vault) Requests(ctx context.Context, owner string) ([]AccessRequest, error) {
	rows, err := v.Pool.Query(ctx, `SELECT q.id,q.kind,q.record_id,q.target_origin,q.browser_session_id,q.purpose,q.fields::text,q.state,q.expires_at,COALESCE(c.title||' · '||c.alias,''),q.login_scope::text FROM credential_vault_requests q LEFT JOIN credential_vault c ON c.id=q.record_id AND c.owner_id=q.owner_id WHERE q.owner_id=$1 AND q.state='pending' AND q.expires_at>now() ORDER BY q.created_at LIMIT 50`, owner)
	if err != nil {
		return nil, ErrVault
	}
	defer rows.Close()
	out := []AccessRequest{}
	for rows.Next() {
		var r AccessRequest
		var raw, scope string
		if rows.Scan(&r.ID, &r.Kind, &r.RecordID, &r.TargetOrigin, &r.BrowserSessionID, &r.Purpose, &raw, &r.State, &r.ExpiresAt, &r.RecordLabel, &scope) != nil || json.Unmarshal([]byte(raw), &r.Fields) != nil {
			return nil, ErrVault
		}
		if r.Kind == "login" {
			r.LoginScope = &browserauth.Scope{}
			if json.Unmarshal([]byte(scope), r.LoginScope) != nil || r.LoginScope.Validate() != nil {
				return nil, ErrVault
			}
		}
		out = append(out, r)
	}
	if rows.Err() != nil {
		return nil, ErrVault
	}
	return out, nil
}
func (v *Vault) Respond(ctx context.Context, owner, id, recordID string, allow bool, otp string, consent ...string) error {
	tx, err := v.Pool.Begin(ctx)
	if err != nil {
		return ErrVault
	}
	defer tx.Rollback(ctx)
	var p Principal
	var kind, state, currentRecord string
	var expiry time.Time
	var rev int64
	if tx.QueryRow(ctx, `SELECT owner_id,session_key,run_id,task_id,task_revision,constraints_revision,kind,state,record_id,record_revision,expires_at FROM credential_vault_requests WHERE id=$1 AND owner_id=$2 FOR UPDATE`, id, owner).Scan(&p.OwnerID, &p.SessionKey, &p.RunID, &p.TaskID, &p.TaskRevision, &p.ConstraintsRevision, &kind, &state, &currentRecord, &rev, &expiry) != nil || !expiry.After(time.Now()) || currentTx(ctx, tx, p) != nil {
		return ErrVault
	}
	// Old open tabs render unfamiliar request kinds as preparation-only access.
	// Such a generic answer must never upgrade into authority to submit a form.
	if kind == "login" && allow && (len(consent) != 1 || consent[0] != browserauth.Consent) {
		return ErrVault
	}
	if len(consent) > 1 || (kind != "login" && len(consent) == 1 && consent[0] != "") {
		return ErrVault
	}
	// A lost HTTP response or a second channel may repeat the same answer.
	// Replays never wake or grant twice, and cannot replace an accepted choice.
	if state != "pending" {
		if !allow && state == "denied" && otp == "" {
			return tx.Commit(ctx)
		}
		if allow && state == "approved" {
			if kind != "otp" && currentRecord == recordID && otp == "" {
				return tx.Commit(ctx)
			}
			var cipher string
			if kind == "otp" && recordID == "" && tx.QueryRow(ctx, `SELECT cipher FROM credential_vault_otps WHERE request_id=$1 AND expires_at>now()`, id).Scan(&cipher) == nil {
				prior, e := v.open(owner, id, "otp", cipher)
				if e == nil && subtle.ConstantTimeCompare([]byte(prior), []byte(otp)) == 1 {
					return tx.Commit(ctx)
				}
			}
		}
		return ErrVault
	}
	targetState := "denied"
	if allow {
		targetState = "approved"
		if kind == "otp" {
			if len(otp) < 4 || len(otp) > 32 || recordID != "" {
				return ErrVault
			}
			cipher, e := v.seal(owner, id, "otp", otp)
			if e != nil {
				return ErrVault
			}
			if _, err = tx.Exec(ctx, `INSERT INTO credential_vault_otps(request_id,cipher,expires_at) VALUES($1,$2,LEAST($3,now()+interval '2 minutes'))`, id, cipher, expiry); err != nil {
				return ErrVault
			}
		} else {
			if recordID == "" {
				return ErrVault
			}
			var selectedRev int64
			if tx.QueryRow(ctx, `SELECT revision FROM credential_vault WHERE id=$1 AND owner_id=$2 AND status='active' FOR SHARE`, recordID, owner).Scan(&selectedRev) != nil {
				return ErrVault
			}
			if kind == "login" && (recordID != currentRecord || selectedRev != rev) {
				return ErrVault
			}
			if kind == "access" && recordID == currentRecord && selectedRev != rev {
				return ErrVault
			}
			rev = selectedRev
			currentRecord = recordID
		}
	}
	if _, err = tx.Exec(ctx, `UPDATE credential_vault_requests SET state=$2,record_id=$3,record_revision=$4,expires_at=CASE WHEN kind='otp' AND $2='approved' THEN LEAST(expires_at,now()+interval '2 minutes') ELSE expires_at END WHERE id=$1`, id, targetState, currentRecord, rev); err != nil {
		return ErrVault
	}
	if v.AfterResponse != nil && v.AfterResponse(ctx, tx, p.RunID, id) != nil {
		return ErrVault
	}
	if v.audit(ctx, tx, owner, currentRecord, id, targetState) != nil || tx.Commit(ctx) != nil {
		return ErrVault
	}
	return nil
}

// Metadata identifies an approved request, never a reusable account capability.
func (v *Vault) GetByAlias(ctx context.Context, alias string) (Record, error) {
	var r Record
	var target, kind, scope string
	var rev int64
	if v.Pool.QueryRow(ctx, `SELECT q.id,q.target_origin,q.kind,q.record_revision,q.login_scope::text FROM credential_vault_requests q LEFT JOIN credential_vault c ON c.id=q.record_id AND c.owner_id=q.owner_id WHERE q.id=$1 AND q.state='approved' AND q.expires_at>now() AND (q.kind='otp' OR (c.status='active' AND c.revision=q.record_revision))`, alias).Scan(&r.Alias, &target, &kind, &rev, &scope) != nil {
		return Record{}, ErrVault
	}
	// Metadata is re-read immediately before dispatch, after private resolution.
	// A revoked SMS route/device or expired OTP must invalidate that check even
	// when a separate action approval exists and account metadata is unchanged.
	if kind == "otp" && !otpSourceCurrent(ctx, v.Pool, alias) {
		return Record{}, ErrVault
	}
	u, err := url.Parse(target)
	if err != nil || u.Hostname() == "" {
		return Record{}, ErrVault
	}
	parsedHost := strings.ToLower(u.Hostname())
	r.Status = StatusActive
	r.SecretType = "username_password"
	r.TargetType = "browser"
	r.ApprovalPolicy = ApprovalPolicyConfirmOnMutation
	r.SecretRef = "vault://" + r.Alias
	r.AllowedDomains = []string{parsedHost}
	r.AllowedTools = []string{"browser.prepare_fill", "browser.prepare_type"}
	r.MetadataJSON = string(mustJSONBytes(map[string]any{"revision": rev, "kind": kind}))
	if kind == "login" {
		var binding browserauth.Scope
		if json.Unmarshal([]byte(scope), &binding) != nil || binding.Validate() != nil {
			return Record{}, ErrVault
		}
		r.AllowedTools = []string{browserauth.Tool}
		r.MetadataJSON = string(mustJSONBytes(map[string]any{"revision": rev, "kind": kind, "login_scope": binding}))
	}
	return r, nil
}
func mustJSONBytes(v any) []byte { b, _ := json.Marshal(v); return b }
func (v *Vault) RequestStatus(ctx context.Context, p Principal, id string) (AccessRequest, error) {
	tx, err := v.Pool.Begin(ctx)
	if err != nil {
		return AccessRequest{}, ErrVault
	}
	defer tx.Rollback(ctx)
	if currentTx(ctx, tx, p) != nil {
		return AccessRequest{}, ErrVault
	}
	var r AccessRequest
	var raw, scope string
	if tx.QueryRow(ctx, `SELECT id,kind,record_id,target_origin,browser_session_id,purpose,fields::text,CASE WHEN expires_at<=now() THEN 'expired' ELSE state END,expires_at,login_scope::text FROM credential_vault_requests WHERE id=$1 AND owner_id=$2 AND run_id=$3 AND task_revision=$4 AND constraints_revision=$5`, id, p.OwnerID, p.RunID, p.TaskRevision, p.ConstraintsRevision).Scan(&r.ID, &r.Kind, &r.RecordID, &r.TargetOrigin, &r.BrowserSessionID, &r.Purpose, &raw, &r.State, &r.ExpiresAt, &scope) != nil || json.Unmarshal([]byte(raw), &r.Fields) != nil {
		return r, ErrVault
	}
	if r.Kind == "login" {
		r.LoginScope = &browserauth.Scope{}
		if json.Unmarshal([]byte(scope), r.LoginScope) != nil || r.LoginScope.Validate() != nil {
			return AccessRequest{}, ErrVault
		}
	}
	return r, nil
}

// ResolveSecretRef deliberately refuses unscoped access, including env://.
func (v *Vault) ResolveSecretRef(context.Context, string) (string, error) { return "", ErrVault }

func (v *Vault) CheckGrantedCall(ctx context.Context, call *toolbrokerv1.ToolCall) bool {
	if call == nil || (call.ToolName != "browser.prepare_fill" && call.ToolName != "browser.prepare_type" && call.ToolName != browserauth.Tool) || len(call.CredentialRefs) == 0 {
		return false
	}
	if call.ToolName == browserauth.Tool && !v.loginCallCurrent(ctx, call) {
		return false
	}
	var a struct {
		Browser string `json:"browser_session_id"`
		URL     string `json:"expected_url"`
	}
	if json.Unmarshal([]byte(call.ArgsJson), &a) != nil {
		return false
	}
	origin, err := Origin(a.URL)
	if err != nil {
		return false
	}
	for _, ref := range call.CredentialRefs {
		var valid bool
		err = v.Pool.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM credential_vault_requests q JOIN runs r ON r.run_id=q.run_id JOIN sessions s ON s.session_key=r.session_key LEFT JOIN tasks t ON t.task_id=q.task_id LEFT JOIN credential_vault c ON c.id=q.record_id AND c.owner_id=q.owner_id WHERE q.id=$1 AND q.run_id=$2 AND q.fields ? $3 AND q.browser_session_id=$4 AND q.target_origin=$5 AND q.state='approved' AND q.expires_at>now() AND s.user_id=q.owner_id AND r.current_state IN ('tool_pending','awaiting_approval','tool_running') AND ((q.kind='login')=$7) AND (q.task_id='' OR (t.owner_id=q.owner_id AND t.revision=q.task_revision AND t.constraints_revision=q.constraints_revision AND t.status='active')) AND ((q.kind='otp' AND EXISTS(SELECT 1 FROM credential_vault_otps o WHERE o.request_id=q.id AND o.expires_at>now() AND (o.claimed_call='' OR o.claimed_call=$6))) OR (q.kind<>'otp' AND c.status='active' AND c.revision=q.record_revision)))`, ref.GetAlias(), call.RunId, ref.GetField(), a.Browser, origin, call.ToolCallId, call.ToolName == browserauth.Tool).Scan(&valid)
		if err != nil || !valid {
			return false
		}
		if ref.GetField() == "otp" && !otpSourceCurrent(ctx, v.Pool, ref.GetAlias()) {
			return false
		}
	}
	return true
}
func (v *Vault) ResolveField(ctx context.Context, call *toolbrokerv1.ToolCall, record Record, field string) (string, error) {
	var args struct {
		SessionID   string `json:"browser_session_id"`
		ExpectedURL string `json:"expected_url"`
	}
	if call == nil || json.Unmarshal([]byte(call.GetArgsJson()), &args) != nil || (call.GetToolName() != "browser.prepare_fill" && call.GetToolName() != "browser.prepare_type" && call.GetToolName() != browserauth.Tool) {
		return "", ErrVault
	}
	origin, err := Origin(args.ExpectedURL)
	if err != nil {
		return "", ErrVault
	}
	tx, err := v.Pool.Begin(ctx)
	if err != nil {
		return "", ErrVault
	}
	defer tx.Rollback(ctx)
	if call.GetToolName() == browserauth.Tool {
		a, ok := loginArgs(call)
		if !ok {
			return "", ErrVault
		}
		raw, _ := json.Marshal(a.Scope)
		var id string
		if tx.QueryRow(ctx, `UPDATE credential_vault_requests SET login_claimed_call=$2 WHERE id=$1 AND kind='login' AND state='approved' AND expires_at>now() AND login_scope=$3::jsonb AND (login_claimed_call='' OR login_claimed_call=$2) RETURNING id`, record.Alias, call.ToolCallId, string(raw)).Scan(&id) != nil {
			return "", ErrVault
		}
	}
	var p Principal
	var id, kind, state, raw string
	var revision int64
	var expires time.Time
	if tx.QueryRow(ctx, `SELECT owner_id,session_key,run_id,task_id,task_revision,constraints_revision,record_id,record_revision,kind,state,fields::text,expires_at FROM credential_vault_requests WHERE id=$1 AND target_origin=$2 AND browser_session_id=$3 FOR UPDATE`, record.Alias, origin, args.SessionID).Scan(&p.OwnerID, &p.SessionKey, &p.RunID, &p.TaskID, &p.TaskRevision, &p.ConstraintsRevision, &id, &revision, &kind, &state, &raw, &expires) != nil || state != "approved" || p.RunID != call.GetRunId() || !expires.After(time.Now()) || currentTx(ctx, tx, p) != nil {
		return "", ErrVault
	}
	if (kind == "login") != (call.ToolName == browserauth.Tool) {
		return "", ErrVault
	}
	var fields []string
	if json.Unmarshal([]byte(raw), &fields) != nil {
		return "", ErrVault
	}
	allowed := false
	for _, f := range fields {
		allowed = allowed || f == field
	}
	if !allowed {
		return "", ErrVault
	}
	var cipher string
	if kind == "otp" {
		if !otpSourceCurrent(ctx, tx, record.Alias) {
			return "", ErrVault
		}
		if field != "otp" || call.GetToolCallId() == "" || tx.QueryRow(ctx, `UPDATE credential_vault_otps SET claimed_call=$2 WHERE request_id=$1 AND expires_at>now() AND (claimed_call='' OR claimed_call=$2) RETURNING cipher`, record.Alias, call.GetToolCallId()).Scan(&cipher) != nil {
			return "", ErrVault
		}
		id = record.Alias
	} else {
		column := "username_cipher"
		if field == "password" {
			column = "password_cipher"
		} else if field != "username" {
			return "", ErrVault
		}
		if tx.QueryRow(ctx, `SELECT `+column+` FROM credential_vault WHERE id=$1 AND owner_id=$2 AND revision=$3 AND status='active' FOR SHARE`, id, p.OwnerID, revision).Scan(&cipher) != nil {
			return "", ErrVault
		}
	}
	value, err := v.open(p.OwnerID, id, field, cipher)
	if err != nil {
		return "", ErrVault
	}
	if v.audit(ctx, tx, p.OwnerID, id, record.Alias, "released_"+field) != nil || tx.Commit(ctx) != nil {
		return "", ErrVault
	}
	return value, nil
}
