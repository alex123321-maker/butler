package credentials

import (
	"context"
	"encoding/json"
	"github.com/butler/butler/internal/browserauth"
	toolbrokerv1 "github.com/butler/butler/internal/gen/toolbroker/v1"
)

func loginArgs(call *toolbrokerv1.ToolCall) (browserauth.Args, bool) {
	if call == nil || call.GetToolCallId() == "" || call.GetToolName() != browserauth.Tool || len(call.CredentialRefs) != 2 {
		return browserauth.Args{}, false
	}
	a, e := browserauth.Parse(call.ArgsJson)
	if e != nil {
		return a, false
	}
	fields := map[string]bool{}
	for _, r := range call.CredentialRefs {
		if r.GetAlias() != a.UsernameRef.Alias || r.GetType() != "credential_ref" || fields[r.GetField()] || (r.GetField() != "username" && r.GetField() != "password") {
			return a, false
		}
		fields[r.GetField()] = true
	}
	return a, len(fields) == 2
}

func (v *Vault) loginCallCurrent(ctx context.Context, call *toolbrokerv1.ToolCall) bool {
	a, ok := loginArgs(call)
	if !ok {
		return false
	}
	raw, _ := json.Marshal(a.Scope)
	var valid bool
	e := v.Pool.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM credential_vault_requests WHERE id=$1 AND kind='login' AND run_id=$2 AND login_scope=$3::jsonb AND (login_claimed_call='' OR login_claimed_call=$4) AND fields='["username","password"]'::jsonb)`, a.UsernameRef.Alias, call.RunId, string(raw), call.ToolCallId).Scan(&valid)
	return e == nil && valid
}
