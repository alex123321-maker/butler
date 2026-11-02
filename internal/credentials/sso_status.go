package credentials

import (
	"context"
	"encoding/json"

	"github.com/butler/butler/internal/browsersso"
)

func (v *Vault) ssoStatus(ctx context.Context, p Principal, id string) (AccessRequest, error) {
	tx, e := v.Pool.Begin(ctx)
	if e != nil {
		return AccessRequest{}, ErrVault
	}
	defer tx.Rollback(ctx)
	if currentTx(ctx, tx, p) != nil {
		return AccessRequest{}, ErrVault
	}
	var r AccessRequest
	var scope browsersso.Scope
	var raw string
	if tx.QueryRow(ctx, `SELECT id,record_id,purpose,scope::text,scope_digest,context_epoch,live_state,expires_at FROM credential_sso_state WHERE id=$1 AND owner_id=$2 AND run_id=$3 AND session_key=$4 AND task_id=$5 AND task_revision=$6 AND constraints_revision=$7`, id, p.OwnerID, p.RunID, p.SessionKey, p.TaskID, p.TaskRevision, p.ConstraintsRevision).Scan(&r.ID, &r.RecordID, &r.Purpose, &raw, &r.ScopeDigest, &r.ContextEpoch, &r.State, &r.ExpiresAt) != nil || json.Unmarshal([]byte(raw), &scope) != nil || scope.Validate() != nil || tx.Commit(ctx) != nil {
		return AccessRequest{}, ErrVault
	}
	r.Kind = "sso_login"
	r.SSOScope = &scope
	r.TargetOrigin = browsersso.Origin(scope.RPURL)
	r.BrowserSessionID = scope.Browser
	r.Fields = []string{"username", "password"}
	return r, nil
}
