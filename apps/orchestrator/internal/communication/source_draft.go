package communication

import (
	"context"
	"strings"

	"github.com/butler/butler/internal/ownership"
)

// CheckPublicationSources is a read-only draft check, never authorization or
// source export. The dispatched publisher independently repeats this lookup
// under its own current grant/claim/transaction and all export guards.
func (s *Store) CheckPublicationSources(ctx context.Context, run string, refs []string) (bool, error) {
	if len(refs) == 0 {
		return true, nil
	}
	if len(refs) > 64 || s == nil || s.Pool == nil {
		return false, ErrDenied
	}
	ctx, release, err := ownership.Begin(ctx)
	if err != nil {
		return false, err
	}
	defer release()
	tx, err := s.Pool.Begin(ctx)
	if err != nil {
		return false, err
	}
	defer tx.Rollback(context.Background())
	p, err := s.principalTx(ctx, tx, run)
	if err != nil {
		return false, err
	}
	var yes bool
	if tx.QueryRow(ctx, `SELECT TRUE FROM sessions WHERE session_key=$1 AND user_id=$2 FOR UPDATE`, p.SessionKey, p.OwnerID).Scan(&yes) != nil {
		return false, ErrDenied
	}
	if err = s.guard(ctx, tx, p); err != nil {
		return false, err
	}
	for _, ref := range refs {
		var selected bool
		if err = tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM memory_run_sources WHERE run_id=$1 AND source_id=$2)`, run, strings.TrimPrefix(ref, "source:")).Scan(&selected); err != nil {
			return false, err
		}
		if !selected {
			return false, nil
		}
	}
	return true, nil
}
