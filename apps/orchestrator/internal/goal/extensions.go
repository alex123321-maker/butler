package goal

import (
	"context"
	"time"

	"github.com/butler/butler/internal/actionauth"
	"github.com/jackc/pgx/v5"
)

// CheckExtensionAttachmentTx is a private admission gate for a trusted extension
// installer. The installer verifies manifest bytes/source and acquires any
// ownership/session locks first. It must persist attachment on this transaction
// and independently authorize every eventual runtime effect through Broker.
// Passing a manifest in a model message does not make its bytes trusted.
func (m *Manager) CheckExtensionAttachmentTx(ctx context.Context, tx pgx.Tx, taskID, ownerID string, reviewed actionauth.ExtensionPermission, at time.Time) error {
	var owner, status, policy string
	if err := tx.QueryRow(ctx, `SELECT owner_id,status,execution_policy FROM tasks WHERE task_id=$1 FOR SHARE`, taskID).Scan(&owner, &status, &policy); err != nil || owner != ownerID || status != "active" {
		return actionauth.ErrOwnerAuthority
	}
	// Enabling a capability changes executable state; a prepare-only task may
	// propose it, but cannot turn its attachment permission into an effect.
	if err := actionauth.CheckExecutionPolicy(actionauth.ContextSnapshot{ExecutionPolicy: policy}, actionauth.EffectUnknown); err != nil {
		return err
	}
	a, err := actionauth.CheckTaskAuthorityTx(ctx, tx, taskID, ownerID, nil, at)
	if err != nil || a.TaskID == "" {
		return actionauth.ErrOwnerAuthority
	}
	mandate, _, err := actionauth.CheckAssignmentTx(ctx, tx, a, at)
	if err != nil {
		return err
	}
	denied, _, err := actionauth.ReadGoalDenialsTx(ctx, tx, a.GoalID, a.OwnerID, at)
	if err != nil {
		return err
	}
	for _, tool := range reviewed.Tools {
		for _, deny := range denied {
			if tool == deny {
				return actionauth.ErrOwnerAuthority
			}
		}
	}
	return actionauth.CheckExtensionAttachment(mandate, reviewed.Source, reviewed.ExtensionID, reviewed.ManifestHash, reviewed.Tools, at)
}
