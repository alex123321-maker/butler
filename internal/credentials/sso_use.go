package credentials

import (
	"context"
	"encoding/json"
	"net/url"
	"strings"

	"github.com/butler/butler/internal/browsersso"
	toolbrokerv1 "github.com/butler/butler/internal/gen/toolbroker/v1"
	"github.com/jackc/pgx/v5"
)

func ssoArgs(call *toolbrokerv1.ToolCall) (browsersso.Args, bool) {
	if call == nil || call.ToolName != browsersso.Tool || call.ToolCallId == "" || len(call.CredentialRefs) != 2 {
		return browsersso.Args{}, false
	}
	a, e := browsersso.Parse(call.ArgsJson)
	if e != nil {
		return a, false
	}
	fields := map[string]bool{}
	for _, r := range call.CredentialRefs {
		if r == nil || r.Type != "credential_ref" || r.Alias != a.UsernameRef.Alias || r.Field != "username" && r.Field != "password" || fields[r.Field] {
			return a, false
		}
		fields[r.Field] = true
	}
	return a, len(fields) == 2
}

func (v *Vault) ssoRecord(ctx context.Context, alias string) (Record, error) {
	var scope browsersso.Scope
	var raw, hash string
	var epoch, revision int64
	if v.Pool.QueryRow(ctx, `SELECT q.scope::text,q.scope_digest,q.context_epoch,q.record_revision FROM credential_sso_requests q JOIN credential_vault c ON c.id=q.record_id AND c.owner_id=q.owner_id WHERE q.id=$1 AND q.state='approved' AND q.expires_at>clock_timestamp() AND c.revision=q.record_revision AND c.status='active'`, alias).Scan(&raw, &hash, &epoch, &revision) != nil || json.Unmarshal([]byte(raw), &scope) != nil || scope.Validate() != nil {
		return Record{}, ErrVault
	}
	u, _ := url.Parse(scope.IdPURL)
	return Record{Alias: alias, Status: StatusActive, SecretType: "username_password", TargetType: "browser", ApprovalPolicy: ApprovalPolicyConfirmOnMutation, SecretRef: "vault://" + alias, AllowedDomains: []string{u.Hostname()}, AllowedTools: []string{browsersso.Tool}, MetadataJSON: string(mustJSONBytes(map[string]any{"kind": "sso_login", "sso_scope": scope, "scope_digest": hash, "context_epoch": epoch, "revision": revision}))}, nil
}

// The immutable owner decision and live request/context/account are checked for
// every field. Claiming one call is separate from acknowledging a POST effect.
func (v *Vault) ssoUse(ctx context.Context, tx pgx.Tx, call *toolbrokerv1.ToolCall, claim bool) (Principal, string, int64, error) {
	a, ok := ssoArgs(call)
	if !ok {
		return Principal{}, "", 0, ErrVault
	}
	var p Principal
	var id, raw, hash, state string
	var revision, epoch int64
	if tx.QueryRow(ctx, `SELECT owner_id,session_key,run_id,task_id,task_revision,constraints_revision,record_id,record_revision,scope::text,scope_digest,context_epoch,state FROM credential_sso_requests WHERE id=$1 AND run_id=$2 AND state='approved' AND expires_at>clock_timestamp()`, a.UsernameRef.Alias, call.RunId).Scan(&p.OwnerID, &p.SessionKey, &p.RunID, &p.TaskID, &p.TaskRevision, &p.ConstraintsRevision, &id, &revision, &raw, &hash, &epoch, &state) != nil || hash != a.Digest || currentTx(ctx, tx, p) != nil {
		return p, "", 0, ErrVault
	}
	wanted, _ := json.Marshal(a.Scope)
	var stored browsersso.Scope
	if json.Unmarshal([]byte(raw), &stored) != nil || string(mustJSONBytes(stored)) != string(wanted) {
		return p, "", 0, ErrVault
	}
	current, _, e := ssoPage(ctx, tx, p, a.Scope)
	if e != nil || current != epoch {
		return p, "", 0, ErrVault
	}
	var active bool
	if tx.QueryRow(ctx, `SELECT TRUE FROM credential_vault WHERE id=$1 AND owner_id=$2 AND revision=$3 AND status='active' FOR SHARE`, id, p.OwnerID, revision).Scan(&active) != nil || !active {
		return p, "", 0, ErrVault
	}
	argsHash := digestString(string(mustJSONBytes(a)))
	if claim {
		if _, e = tx.Exec(ctx, `INSERT INTO credential_sso_uses(request_id,run_id,tool_call_id,args_hash) SELECT id,run_id,$2,$3 FROM credential_sso_requests WHERE id=$1 AND state='approved' AND expires_at>clock_timestamp() ON CONFLICT DO NOTHING`, a.UsernameRef.Alias, call.ToolCallId, argsHash); e != nil {
			return p, "", 0, ErrVault
		}
	}
	var valid bool
	if tx.QueryRow(ctx, `SELECT q.expires_at>clock_timestamp() AND (NOT EXISTS(SELECT 1 FROM credential_sso_uses u WHERE u.request_id=q.id) OR EXISTS(SELECT 1 FROM credential_sso_uses u WHERE u.request_id=q.id AND u.run_id=$2 AND u.tool_call_id=$3 AND u.args_hash=$4)) FROM credential_sso_requests q WHERE q.id=$1`, a.UsernameRef.Alias, call.RunId, call.ToolCallId, argsHash).Scan(&valid) != nil || !valid {
		return p, "", 0, ErrVault
	}
	if claim {
		if tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM credential_sso_uses WHERE request_id=$1 AND run_id=$2 AND tool_call_id=$3 AND args_hash=$4)`, a.UsernameRef.Alias, call.RunId, call.ToolCallId, argsHash).Scan(&valid) != nil || !valid {
			return p, "", 0, ErrVault
		}
	}
	return p, id, revision, nil
}

func (v *Vault) checkSSOCall(ctx context.Context, call *toolbrokerv1.ToolCall) bool {
	tx, e := v.Pool.Begin(ctx)
	if e != nil {
		return false
	}
	defer tx.Rollback(ctx)
	_, _, _, e = v.ssoUse(ctx, tx, call, false)
	return e == nil && tx.Commit(ctx) == nil
}
func (v *Vault) resolveSSOField(ctx context.Context, call *toolbrokerv1.ToolCall, record Record, field string) (string, error) {
	a, ok := ssoArgs(call)
	if !ok || record.Alias != a.UsernameRef.Alias || field != "username" && field != "password" {
		return "", ErrVault
	}
	tx, e := v.Pool.Begin(ctx)
	if e != nil {
		return "", ErrVault
	}
	defer tx.Rollback(ctx)
	p, id, revision, e := v.ssoUse(ctx, tx, call, true)
	if e != nil {
		return "", ErrVault
	}
	column := "username_cipher"
	if field == "password" {
		column = "password_cipher"
	}
	var cipher string
	if tx.QueryRow(ctx, `SELECT `+column+` FROM credential_vault WHERE id=$1 AND owner_id=$2 AND revision=$3 AND status='active'`, id, p.OwnerID, revision).Scan(&cipher) != nil {
		return "", ErrVault
	}
	value, e := v.open(p.OwnerID, id, field, cipher)
	if e != nil || v.audit(ctx, tx, p.OwnerID, id, record.Alias, "sso_released_"+field) != nil || tx.Commit(ctx) != nil {
		return "", ErrVault
	}
	return value, nil
}

func isSSOAlias(alias string) bool { return strings.HasPrefix(alias, "ssoq_") }
