package jobs

import "context"

// ClaimPurpose selects one owned worker queue without stealing another purpose
// in the same kind. Selection never replaces the subsequent admission proof.
func (s *Store) ClaimPurpose(ctx context.Context, kind, purpose string) (Job, bool, error) {
	if kind == "" || len(purpose) < 1 || len(purpose) > 512 {
		return Job{}, false, ErrClaim
	}
	return s.claimSelected(ctx, kind, purpose)
}
