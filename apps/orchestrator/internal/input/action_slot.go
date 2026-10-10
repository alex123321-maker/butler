package input

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/butler/butler/internal/ownership"
)

type actionSlot struct {
	Reference           string `json:"reference"`
	RunID               string `json:"run_id"`
	ToolCallID          string `json:"tool_call_id"`
	TaskID              string `json:"task_id"`
	TaskRevision        int64  `json:"task_revision"`
	ConstraintsRevision int64  `json:"constraints_revision"`
	ToolName            string `json:"tool_name"`
	ArgsHash            string `json:"args_hash"`
}

// EnsureActionSlot persists a fresh random slot before Broker Prepare. Existing
// slots survive replacement of checkpoint_data, lease takeover and decisions.
// Two explicit proposals with identical arguments still receive different slots.
func (s *Store) EnsureActionSlot(ctx context.Context, runID, callID, tool, argsHash string) (string, error) {
	return s.ensureActionSlot(ctx, runID, callID, tool, argsHash, "")
}

// LinkActionSlot is an explicit trusted cross-run continuation operation. It is
// not exposed to model tools or automatically selected by similar arguments.
func (s *Store) LinkActionSlot(ctx context.Context, runID, callID, tool, argsHash, parentRef string) (string, error) {
	if parentRef == "" {
		return "", fmt.Errorf("explicit parent action reference required")
	}
	return s.ensureActionSlot(ctx, runID, callID, tool, argsHash, parentRef)
}
func (s *Store) ensureActionSlot(ctx context.Context, runID, callID, tool, argsHash, parentRef string) (string, error) {
	claim, ok := ctx.Value(claimKey{}).(Record)
	if !ok || runID == "" || callID == "" || tool == "" || len(argsHash) != 64 {
		return "", fmt.Errorf("trusted input claim and exact proposal required")
	}
	token, ok := ownership.TokenFromContext(ctx)
	if !ok || token.RunID != runID || token.SessionKey != claim.SessionKey {
		return "", fmt.Errorf("live action-slot execution ownership required")
	}
	ctx, release, err := ownership.Begin(ctx)
	if err != nil {
		return "", err
	}
	defer release()
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return "", err
	}
	defer tx.Rollback(ctx)
	var slot actionSlot
	var pinned int64
	var status, owner string
	err = tx.QueryRow(ctx, `SELECT task_id,task_revision FROM runs WHERE run_id=$1 FOR SHARE`, runID).Scan(&slot.TaskID, &pinned)
	if err != nil {
		return "", err
	}
	err = tx.QueryRow(ctx, `SELECT revision,constraints_revision,status,owner_id FROM tasks WHERE task_id=$1 FOR SHARE`, slot.TaskID).Scan(&slot.TaskRevision, &slot.ConstraintsRevision, &status, &owner)
	if err != nil || slot.TaskRevision != pinned || owner != claim.OwnerID || (status != "active" && status != "waiting_for_user") {
		return "", fmt.Errorf("task action-slot authority changed")
	}
	slot.RunID, slot.ToolCallID, slot.ToolName, slot.ArgsHash = runID, callID, tool, argsHash
	var payload string
	err = tx.QueryRow(ctx, `SELECT checkpoint_json::text FROM accepted_inputs WHERE input_id=$1 AND claim_id=$2 AND run_id=$3 AND status='running' AND claim_expires_at>clock_timestamp() FOR UPDATE`, claim.InputID, claim.ClaimID, runID).Scan(&payload)
	if err != nil {
		return "", ErrClaimLost
	}
	var saved struct {
		Slots map[string]actionSlot `json:"action_slots"`
	}
	if json.Unmarshal([]byte(payload), &saved) != nil {
		return "", fmt.Errorf("invalid action-slot checkpoint")
	}
	if old, exists := saved.Slots[callID]; exists {
		slot.Reference = old.Reference
		if old != slot || (parentRef != "" && parentRef != old.Reference) {
			return "", fmt.Errorf("action slot identity or task versions changed")
		}
		return old.Reference, tx.Commit(ctx)
	}
	if parentRef != "" {
		var raw string
		err = tx.QueryRow(ctx, `SELECT item.value::text FROM accepted_inputs i CROSS JOIN LATERAL jsonb_each(COALESCE(i.checkpoint_json->'action_slots','{}'::jsonb)) item WHERE i.owner_id=$1 AND item.value->>'reference'=$2 LIMIT 1`, owner, parentRef).Scan(&raw)
		var parent actionSlot
		if err != nil || json.Unmarshal([]byte(raw), &parent) != nil {
			return "", fmt.Errorf("trusted parent action slot unavailable")
		}
		slot.Reference = parentRef
		parent.RunID, parent.ToolCallID = runID, callID
		if parent != slot {
			return "", fmt.Errorf("cross-run link requires unchanged task and proposal")
		}
	} else {
		slot.Reference, err = randomID("slot-")
		if err != nil {
			return "", err
		}
	}
	encoded, err := json.Marshal(slot)
	if err != nil {
		return "", err
	}
	_, err = tx.Exec(ctx, `UPDATE accepted_inputs SET checkpoint_json=checkpoint_json||jsonb_build_object('action_slots',COALESCE(checkpoint_json->'action_slots','{}'::jsonb)||jsonb_build_object($2::text,$3::jsonb)),updated_at=clock_timestamp() WHERE input_id=$1`, claim.InputID, callID, string(encoded))
	if err != nil {
		return "", err
	}
	return slot.Reference, tx.Commit(ctx)
}
