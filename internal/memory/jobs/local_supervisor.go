package jobs

import "context"

// checkLocalSupervisor repeats the complete source/claim proof of an admitted
// local call. This private job checker only SELECTs/checks/locks: it cannot bind
// live-run pins or invoke an independently supplied caller checker. There is no
// business state to publish, so observation releases its locks with bounded
// rollback rather than requiring a synchronous COMMIT inside the 200ms proof.
// Actual allowance admission and derivative publication keep their commits.
func (s *Store) checkLocalSupervisor(ctx context.Context, j Job) error {
	ctx = s.Context(ctx, j)
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)
	if err = s.catalog.CheckObjectsTx(ctx, tx, nil, j.Purpose); err != nil {
		return err
	}
	return tx.Rollback(ctx)
}
