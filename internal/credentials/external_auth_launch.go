package credentials

import (
	"context"
	"github.com/butler/butler/internal/externalauth"
)

// ExternalLaunch recovers the same private unspent ticket after an owner page
// reload. It cannot approve a pending decision or extend/mint a capability.
func (v *Vault) ExternalLaunch(ctx context.Context, owner, id, digest string) (externalauth.PrivateLaunch, error) {
	tx, e := v.Pool.Begin(ctx)
	if e != nil {
		return externalauth.PrivateLaunch{}, ErrVault
	}
	defer tx.Rollback(ctx)
	q, e := v.loadExternalTx(ctx, tx, id)
	if e != nil || q.p.OwnerID != owner || q.state != "approved" || digest != q.digest {
		return externalauth.PrivateLaunch{}, ErrVault
	}
	var launched bool
	if tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM browser_external_auth_launches WHERE request_id=$1)`, id).Scan(&launched) != nil || launched || tx.Commit(ctx) != nil {
		return externalauth.PrivateLaunch{}, ErrVault
	}
	return v.RespondExternal(ctx, owner, id, q.record, true, externalauth.Consent, digest)
}
