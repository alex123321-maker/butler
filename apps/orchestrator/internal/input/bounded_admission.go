package input

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/butler/butler/internal/delegation"
	runv1 "github.com/butler/butler/internal/gen/run/v1"
	"github.com/butler/butler/internal/ingress"
	"github.com/jackc/pgx/v5"
)

// AcceptBoundedTx is only a kernel admission seam. Caller holds the child's
// session lock before task/owner authority and publishes its contract atomically.
// Execution still passes through ordinary ordered claims, leases and the Broker.
func (s *Store) AcceptBoundedTx(ctx context.Context, tx pgx.Tx, req delegation.InputRequest) (string, error) {
	var now time.Time
	if err := tx.QueryRow(ctx, `SELECT clock_timestamp()`).Scan(&now); err != nil {
		return "", err
	}
	if req.OwnerID == "" || req.SessionKey == "" || req.TaskID == "" || req.EventID == "" || !delegation.ValidSource(req.ContractSource) || !req.Deadline.After(now) || req.Deadline.After(now.Add(24*time.Hour)) || req.MaxAttempts < 1 || req.MaxAttempts > 3 {
		return "", fmt.Errorf("invalid bounded child admission")
	}
	payload, err := json.Marshal(struct {
		TaskID         string               `json:"task_id"`
		ContractSource delegation.SourceRef `json:"delegation_source"`
	}{req.TaskID, req.ContractSource})
	if err != nil {
		return "", err
	}
	event := ingress.InputEvent{EventID: req.EventID, EventType: runv1.InputEventType_INPUT_EVENT_TYPE_SCHEDULED_INTERNAL_EVENT, SessionKey: req.SessionKey, Source: "delegation", PayloadJSON: string(payload), CreatedAt: now, IdempotencyKey: req.EventID}
	r, created, err := s.AcceptTx(ctx, tx, event, req.OwnerID, req.TaskID)
	if err != nil {
		return "", err
	}
	if !created {
		if !r.ExpiresAt.Equal(req.Deadline.Truncate(time.Microsecond)) || r.MaxAttempts != req.MaxAttempts {
			return "", fmt.Errorf("bounded child identity conflict")
		}
		return r.InputID, nil
	}
	tag, err := tx.Exec(ctx, `UPDATE accepted_inputs SET expires_at=$2,max_attempts=$3 WHERE input_id=$1 AND status='queued'`, r.InputID, req.Deadline, req.MaxAttempts)
	if err != nil {
		return "", err
	}
	if tag.RowsAffected() != 1 {
		return "", ErrClaimLost
	}
	return r.InputID, nil
}
