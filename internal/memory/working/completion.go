package working

import "context"

// ClearCompletedRun cannot erase working state belonging to a later run.
// This controller operation uses the stored auxiliary job, not an expired lease.
func (s *Store) ClearCompletedRun(ctx context.Context, sessionKey, runID string) error {
	_, err := s.pool.Exec(ctx, `DELETE FROM memory_working WHERE session_key=$1 AND run_id=$2 AND EXISTS(SELECT 1 FROM runs WHERE run_id=$2 AND current_state='completed')`, sessionKey, runID)
	return err
}
