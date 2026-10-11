package input

import (
	"context"
	"encoding/json"
	"time"

	runv1 "github.com/butler/butler/internal/gen/run/v1"
	"github.com/butler/butler/internal/generatedtools"
	"github.com/butler/butler/internal/ingress"
	"github.com/butler/butler/internal/owneridentity"
	"github.com/butler/butler/internal/ownership"
	"github.com/jackc/pgx/v5"
)

type ConverterContinuationRequest struct {
	ID, TaskID, OwnerID, SessionKey string
	Deadline                        time.Time
}
type ConverterContinuationBinding struct {
	TaskID string `json:"task_id"`
	ID     string `json:"continuation_id"`
}

// This transaction seam is used only by the owning055 scheduler. It writes a
// reference-only ordered receipt, not an owner message or source grant.
func (s *Store) AcceptConverterContinuationTx(ctx context.Context, tx pgx.Tx, q ConverterContinuationRequest) (string, error) {
	var now time.Time
	if tx.QueryRow(ctx, `SELECT clock_timestamp()`).Scan(&now) != nil || q.ID == "" || q.TaskID == "" || q.OwnerID == "" || q.SessionKey == "" || !q.Deadline.After(now) || q.Deadline.After(now.Add(24*time.Hour)) {
		return "", generatedtools.ErrAccess
	}
	if _, actor := owneridentity.FromContext(ctx); actor {
		return "", generatedtools.ErrAccess
	}
	payload, _ := json.Marshal(ConverterContinuationBinding{q.TaskID, q.ID})
	event := ingress.InputEvent{EventID: "converter-return:" + q.ID, EventType: runv1.InputEventType_INPUT_EVENT_TYPE_SCHEDULED_INTERNAL_EVENT, SessionKey: q.SessionKey, Source: "converter-return", PayloadJSON: string(payload), CreatedAt: now, IdempotencyKey: "converter-return:" + q.ID}
	r, created, err := s.AcceptTx(ctx, tx, event, q.OwnerID, q.TaskID)
	if err != nil {
		return "", err
	}
	if !created {
		if !r.ExpiresAt.Equal(q.Deadline.Truncate(time.Microsecond)) || r.MaxAttempts != 3 {
			return "", generatedtools.ErrAccess
		}
		return r.InputID, nil
	}
	tag, err := tx.Exec(ctx, `UPDATE accepted_inputs SET expires_at=$2,max_attempts=3 WHERE input_id=$1 AND status='queued'`, r.InputID, q.Deadline)
	if err != nil || tag.RowsAffected() != 1 {
		return "", generatedtools.ErrAccess
	}
	return r.InputID, nil
}

// VerifyConverterContinuationTx is separate from the context-only delegated
// verifier. It requires the exact private queue claim and current worker fence.
// Caller supplies the055 identity, never a general system/owner bypass.
func (s *Store) VerifyConverterContinuationTx(ctx context.Context, tx pgx.Tx, event ingress.InputEvent, c generatedtools.Continuation) error {
	claim, ok := ctx.Value(claimKey{}).(Record)
	tok, owned := ownership.TokenFromContext(ctx)
	v := c.Intent
	if !ok || !owned || c.State != "queued" || claim.InputID != c.InputID || claim.ClaimID == "" || claim.OwnerID != v.OwnerID || claim.TaskID != v.ParentTaskID || tok.SessionKey != claim.SessionKey || event.SessionKey != claim.SessionKey || event.Source != "converter-return" || event.EventID != "converter-return:"+v.ID || event.IdempotencyKey != claim.InputID {
		return generatedtools.ErrAccess
	}
	if _, actor := owneridentity.FromContext(ctx); actor {
		return generatedtools.ErrAccess
	}
	var leaseExpires, claimExpires, inputExpires time.Time
	if tx.QueryRow(ctx, `SELECT expires_at FROM session_ownership WHERE session_key=$1 AND run_id=$2 AND lease_id=$3 AND fence_epoch=$4 FOR SHARE`, tok.SessionKey, tok.RunID, tok.LeaseID, tok.FenceEpoch).Scan(&leaseExpires) != nil {
		return generatedtools.ErrAccess
	}
	var yes bool
	if tx.QueryRow(ctx, `SELECT TRUE FROM sessions WHERE session_key=$1 AND user_id=$2 FOR SHARE`, tok.SessionKey, v.OwnerID).Scan(&yes) != nil {
		return generatedtools.ErrAccess
	}
	if tx.QueryRow(ctx, `SELECT TRUE FROM runs WHERE run_id=$1 AND session_key=$2 AND idempotency_key=$3 AND input_event_id=$4 AND task_id=$5 AND task_revision=$6 AND lease_id=$7 AND current_state NOT IN ('completed','failed','cancelled','timed_out','finalizing') FOR SHARE`, tok.RunID, tok.SessionKey, claim.InputID, event.EventID, v.ParentTaskID, v.ParentRevision, tok.LeaseID).Scan(&yes) != nil {
		return generatedtools.ErrAccess
	}
	if tx.QueryRow(ctx, `SELECT TRUE FROM tasks WHERE task_id=$1 AND session_key=$2 AND owner_id=$3 AND revision=$4 AND constraints_revision=$5 AND status='active' FOR SHARE`, v.ParentTaskID, tok.SessionKey, v.OwnerID, v.ParentRevision, v.ConstraintsRevision).Scan(&yes) != nil {
		return generatedtools.ErrAccess
	}
	var raw []byte
	if tx.QueryRow(ctx, `SELECT event_json,claim_expires_at,expires_at FROM accepted_inputs WHERE input_id=$1 AND claim_id=$2 AND owner_id=$3 AND task_id=$4 AND run_id=$5 AND source='converter-return' AND status='running' FOR SHARE`, c.InputID, claim.ClaimID, v.OwnerID, v.ParentTaskID, tok.RunID).Scan(&raw, &claimExpires, &inputExpires) != nil {
		return generatedtools.ErrAccess
	}
	var original ingress.InputEvent
	var binding ConverterContinuationBinding
	if json.Unmarshal(raw, &original) != nil || original.EventID != event.EventID || original.SessionKey != event.SessionKey || original.Source != event.Source || original.EventType != event.EventType || original.PayloadJSON != event.PayloadJSON || !original.CreatedAt.Equal(event.CreatedAt) || original.IdempotencyKey != original.EventID || generatedtools.Decode([]byte(original.PayloadJSON), &binding) != nil || binding.TaskID != v.ParentTaskID || binding.ID != v.ID {
		return generatedtools.ErrAccess
	}
	var now time.Time
	if tx.QueryRow(ctx, `SELECT clock_timestamp()`).Scan(&now) != nil || !leaseExpires.After(now) || !claimExpires.After(now) || !inputExpires.After(now) || !v.Deadline.After(now) {
		return generatedtools.ErrAccess
	}
	return nil
}
