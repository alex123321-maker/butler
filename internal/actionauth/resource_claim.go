package actionauth

import (
	"context"

	"github.com/jackc/pgx/v5"
)

// DispatchClaimGuard is trusted deployment composition. It may add an exact
// resource claim to the same transaction after the finite budget reservation.
// CheckAdmissionTx locks the complete authority/source set before the ordinary
// dispatch guard, effect and budget locks. ClaimActionTx may repeat those held
// checks, then acquire the complete sorted resource set; no new authority set.
// It must not call a remote service or borrow another database connection.
type DispatchClaimGuard interface {
	CheckAdmissionTx(context.Context, pgx.Tx, Grant) error
	ClaimActionTx(context.Context, pgx.Tx, Grant, string) error
}

func (s *PostgresStore) SetDispatchClaimGuard(g DispatchClaimGuard) { s.dispatchClaimGuard = g }
