package jobs

import (
	"context"
	"github.com/butler/butler/internal/owneridentity"
)

// ClaimOwner is the private authenticated presentation adapter's exact selector.
// The ordinary worker queues and full independent admission proof are unchanged.
func (s *Store) ClaimOwner(ctx context.Context, id, kind, purpose string) (Job, bool, error) {
	actor, ok := owneridentity.FromContext(ctx)
	if !ok || len(id) < 8 || len(id) > 512 || kind == "" || len(purpose) < 1 || len(purpose) > 512 {
		return Job{}, false, ErrClaim
	}
	return s.claimSelectedOwner(ctx, kind, purpose, id, actor.OwnerID)
}
