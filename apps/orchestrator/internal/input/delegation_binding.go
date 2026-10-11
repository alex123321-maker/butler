package input

import (
	"context"
	"encoding/json"
	"io"
	"strings"
	"time"

	"github.com/butler/butler/internal/delegation"
	"github.com/butler/butler/internal/ingress"
	"github.com/butler/butler/internal/owneridentity"
	"github.com/butler/butler/internal/ownership"
)

// VerifyDelegatedInput is private claim verification, not owner admission. Its
// original event/source is a refs-only kernel record. All current locks use one
// connection in ownership→session→run→task→input order, with a final DB clock.
func (s *Store) VerifyDelegatedInput(ctx context.Context, event ingress.InputEvent, taskID string, source delegation.SourceRef) error {
	claim, ok := ctx.Value(claimKey{}).(Record)
	token, owned := ownership.TokenFromContext(ctx)
	if !ok || !owned || s == nil || s.pool == nil || claim.InputID == "" || claim.ClaimID == "" || claim.OwnerID == "" || claim.TaskID != taskID || token.SessionKey != claim.SessionKey || event.SessionKey != claim.SessionKey || event.Source != "delegation" || event.EventID != "delegation:"+taskID || event.IdempotencyKey != claim.InputID || !delegation.ValidSource(source) {
		return delegation.ErrDenied
	}
	if _, actor := owneridentity.FromContext(ctx); actor {
		return delegation.ErrDenied
	}
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return delegation.ErrDenied
	}
	defer tx.Rollback(ctx)
	var leaseExpires, claimExpires, inputExpires time.Time
	if tx.QueryRow(ctx, `SELECT expires_at FROM session_ownership WHERE session_key=$1 AND run_id=$2 AND lease_id=$3 AND fence_epoch=$4 FOR SHARE`, token.SessionKey, token.RunID, token.LeaseID, token.FenceEpoch).Scan(&leaseExpires) != nil {
		return delegation.ErrDenied
	}
	var owner string
	if tx.QueryRow(ctx, `SELECT user_id FROM sessions WHERE session_key=$1 FOR SHARE`, token.SessionKey).Scan(&owner) != nil || owner != claim.OwnerID {
		return delegation.ErrDenied
	}
	var runTask, runLease string
	var revision int64
	if tx.QueryRow(ctx, `SELECT task_id,task_revision,lease_id FROM runs WHERE run_id=$1 AND session_key=$2 AND idempotency_key=$3 AND input_event_id=$4 AND current_state NOT IN ('completed','failed','cancelled','timed_out','finalizing') FOR SHARE`, token.RunID, token.SessionKey, claim.InputID, event.EventID).Scan(&runTask, &revision, &runLease) != nil || runTask != taskID || runLease != token.LeaseID {
		return delegation.ErrDenied
	}
	var current bool
	if tx.QueryRow(ctx, `SELECT owner_id=$2 AND revision=$3 AND status='active' FROM tasks WHERE task_id=$1 FOR SHARE`, taskID, owner, revision).Scan(&current) != nil || !current {
		return delegation.ErrDenied
	}
	var raw []byte
	if tx.QueryRow(ctx, `SELECT event_json,claim_expires_at,expires_at FROM accepted_inputs WHERE input_id=$1 AND claim_id=$2 AND owner_id=$3 AND session_key=$4 AND task_id=$5 AND source='delegation' AND run_id=$6 AND status='running' FOR SHARE`, claim.InputID, claim.ClaimID, owner, token.SessionKey, taskID, token.RunID).Scan(&raw, &claimExpires, &inputExpires) != nil {
		return delegation.ErrDenied
	}
	var original ingress.InputEvent
	if json.Unmarshal(raw, &original) != nil || original.EventID != event.EventID || original.Source != event.Source || original.SessionKey != event.SessionKey || original.EventType != event.EventType || !original.CreatedAt.Equal(event.CreatedAt) || original.PayloadJSON != event.PayloadJSON || original.IdempotencyKey != original.EventID {
		return delegation.ErrDenied
	}
	var binding struct {
		TaskID string               `json:"task_id"`
		Source delegation.SourceRef `json:"delegation_source"`
	}
	decoder := json.NewDecoder(strings.NewReader(original.PayloadJSON))
	decoder.DisallowUnknownFields()
	if decoder.Decode(&binding) != nil || decoder.Decode(new(any)) != io.EOF || binding.TaskID != taskID || binding.Source != source {
		return delegation.ErrDenied
	}
	var now time.Time
	if tx.QueryRow(ctx, `SELECT clock_timestamp()`).Scan(&now) != nil || !leaseExpires.After(now) || !claimExpires.After(now) || !inputExpires.After(now) {
		return delegation.ErrDenied
	}
	if tx.Commit(ctx) != nil {
		return delegation.ErrDenied
	}
	return nil
}
