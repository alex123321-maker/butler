package communication

import (
	"context"

	"github.com/butler/butler/internal/owneridentity"
)

// ConfigureQuestionLimit is for trusted operator startup after validating the
// exact owner's channel bindings. It is not installed as an HTTP/model tool.
// PostgreSQL's trigger enforces the current policy even for older writers.
func (s *Store) ConfigureQuestionLimit(ctx context.Context, owner string, limit int) error {
	if s == nil || s.Pool == nil || owner == "" || limit < 0 || limit > 64 {
		return ErrDenied
	}
	if _, fromOwnerEndpoint := owneridentity.FromContext(ctx); fromOwnerEndpoint {
		return ErrDenied
	}
	if limit == 0 {
		limit = 16
	}
	tag, err := s.Pool.Exec(ctx, `INSERT INTO communication_owner_limits(owner_id,max_open_interactions) SELECT $1,$2 WHERE EXISTS(SELECT 1 FROM communication_bindings WHERE owner_id=$1 AND enabled AND verified) ON CONFLICT(owner_id) DO UPDATE SET max_open_interactions=EXCLUDED.max_open_interactions,version=communication_owner_limits.version+1,updated_at=clock_timestamp() WHERE communication_owner_limits.max_open_interactions<>EXCLUDED.max_open_interactions`, owner, limit)
	if err != nil {
		return ErrDenied
	}
	if tag.RowsAffected() == 0 {
		var current int
		if s.Pool.QueryRow(ctx, `SELECT max_open_interactions FROM communication_owner_limits WHERE owner_id=$1`, owner).Scan(&current) != nil || current != limit {
			return ErrDenied
		}
	}
	return nil
}
