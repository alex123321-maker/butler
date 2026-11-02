package credentialcontrol

import (
	"context"
	"github.com/jackc/pgx/v5"
)

// StoppedExternalLoginAbsentTx is metadata-only. Its caller must hold the
// original terminal session/run/Input locks against the pinned creator, which
// commits its request/audit atomically under shared session/run locks. Any
// same-call request, even in another run, conservatively prevents this proof.
// FK-bound launches/exchanges cannot exist without a request. No private
// challenge, account field, cookie, provider identity or token is read.
func StoppedExternalLoginAbsentTx(ctx context.Context, tx pgx.Tx, call string) (bool, error) {
	var absent bool
	err := tx.QueryRow(ctx, `SELECT NOT EXISTS(SELECT 1 FROM browser_external_auth_requests WHERE tool_call_id=$1)`, call).Scan(&absent)
	return absent, err
}
