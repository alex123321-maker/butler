package input

import (
	"context"

	"github.com/jackc/pgx/v5"
)

// CheckCommunicationClaimTx binds kernel communication writes to the caller's
// actual private claim, not merely to some live claim currently owning the run.
func (s *Store) CheckCommunicationClaimTx(ctx context.Context, tx pgx.Tx, runID, inputID string) error {
	r, ok := ctx.Value(claimKey{}).(Record)
	if !ok || r.InputID != inputID || r.ClaimID == "" || r.RunID != "" && r.RunID != runID {
		return ErrClaimLost
	}
	var valid bool
	if tx.QueryRow(ctx, `SELECT TRUE FROM accepted_inputs WHERE input_id=$1 AND claim_id=$2 AND run_id=$3 AND status='running' AND claim_expires_at>clock_timestamp() AND expires_at>clock_timestamp() FOR UPDATE`, inputID, r.ClaimID, runID).Scan(&valid) != nil {
		return ErrClaimLost
	}
	return nil
}
