package credentials

import (
	"context"
	"crypto/subtle"
	"time"

	"github.com/butler/butler/internal/actionauth"
)

// CancelExternal withdraws only this attempt. It cannot approve, replay or
// refund it. An exchange already dispatched remains UNKNOWN, never absent.
func (v *Vault) CancelExternal(ctx context.Context, owner, id, digest string) error {
	return v.closeExternal(ctx, owner, id, digest, "cancelled")
}

func (v *Vault) closeExternal(ctx context.Context, owner, id, digest, reason string) error {
	ctx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 5*time.Second)
	defer cancel()
	if reason != "cancelled" && reason != "rejected" {
		return ErrVault
	}
	tx, e := v.Pool.Begin(ctx)
	if e != nil {
		return ErrVault
	}
	defer tx.Rollback(ctx)
	q, e := v.loadExternalTx(ctx, tx, id)
	if e != nil || q.p.OwnerID != owner || subtle.ConstantTimeCompare([]byte(q.digest), []byte(digest)) != 1 {
		return ErrVault
	}
	// Same lock order as callback: canonical Input before immutable request.
	var locked string
	if tx.QueryRow(ctx, `SELECT input_id FROM accepted_inputs WHERE input_id=$1 FOR UPDATE`, q.input).Scan(&locked) != nil || tx.QueryRow(ctx, `SELECT id FROM browser_external_auth_requests WHERE id=$1 FOR UPDATE`, id).Scan(&locked) != nil {
		return ErrVault
	}
	var result, exchanged bool
	if tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM browser_external_auth_results WHERE request_id=$1),EXISTS(SELECT 1 FROM browser_external_auth_exchanges WHERE request_id=$1)`, id).Scan(&result, &exchanged) != nil {
		return ErrVault
	}
	if result {
		return ErrVault
	}
	if _, e = tx.Exec(ctx, `INSERT INTO browser_external_auth_closures(request_id,reason) VALUES($1,$2) ON CONFLICT DO NOTHING`, id, reason); e != nil {
		return ErrVault
	}
	if exchanged {
		if _, e = tx.Exec(ctx, `INSERT INTO browser_external_auth_results(request_id,outcome) VALUES($1,'unknown')`, id); e != nil || actionauth.FinishExternalAuthTx(ctx, tx, id) != nil {
			return ErrVault
		}
	}
	if v.audit(ctx, tx, owner, q.record, id, "external_auth_"+reason) != nil || v.AfterResponse != nil && v.AfterResponse(ctx, tx, q.p.RunID, id) != nil || tx.Commit(ctx) != nil {
		return ErrVault
	}
	return nil
}
