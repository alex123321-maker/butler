package input

import (
	"context"
	"fmt"

	"github.com/butler/butler/internal/owneridentity"
)

// StoppedObservation is content-free evidence of an already relinquished
// attempt. It never grants takeover, clears a blocker or supplies a final.
type StoppedObservation struct {
	InputID, InputStatus, RunID, RunState, SessionKey, TaskID string
	TaskRevision, ConstraintsRevision                         int64
	Attempts                                                  int
	Checkpoint, InputHash, Reason                             string
	InputRevision                                             int64
}

func (s *Store) ObserveStopped(ctx context.Context, run string) (StoppedObservation, error) {
	var o StoppedObservation
	a, ok := owneridentity.FromContext(ctx)
	if !ok || a.OwnerID == "" || a.DeliveryID == "" || s == nil || run == "" {
		return o, fmt.Errorf("explicit owner stopped-attempt observation required")
	}
	err := s.pool.QueryRow(ctx, `SELECT i.input_id,i.status,r.run_id,r.current_state,i.session_key,t.task_id,r.task_revision,t.constraints_revision,i.attempts,i.checkpoint,h.row_hash,h.revision,i.error_code
FROM accepted_inputs i JOIN runs r ON r.run_id=i.run_id JOIN tasks t ON t.task_id=i.task_id
JOIN memory_experience_input_heads h ON h.input_id=i.input_id AND h.alive
WHERE r.run_id=$1 AND i.owner_id=$2 AND t.owner_id=$2 AND i.session_key=r.session_key AND t.session_key=r.session_key
AND r.task_id=t.task_id AND r.task_revision=t.revision AND COALESCE(t.parent_task_id,'')='' AND t.status IN ('active','blocked')
AND i.status IN ('blocked','failed') AND i.attempts>0 AND i.claim_id='' AND i.claim_expires_at IS NULL
AND (r.current_state IN ('failed','timed_out') OR (i.status='blocked' AND i.error_code='effect_unknown' AND r.current_state='tool_running'))
AND NOT EXISTS(SELECT 1 FROM session_ownership l WHERE l.session_key=i.session_key AND l.lease_id<>'' AND l.expires_at>clock_timestamp())
AND NOT EXISTS(SELECT 1 FROM accepted_inputs x WHERE x.session_key=i.session_key AND x.status='running')`, run, a.OwnerID).Scan(&o.InputID, &o.InputStatus, &o.RunID, &o.RunState, &o.SessionKey, &o.TaskID, &o.TaskRevision, &o.ConstraintsRevision, &o.Attempts, &o.Checkpoint, &o.InputHash, &o.InputRevision, &o.Reason)
	if err != nil {
		return StoppedObservation{}, fmt.Errorf("current stopped-attempt evidence unavailable")
	}
	return o, nil
}
