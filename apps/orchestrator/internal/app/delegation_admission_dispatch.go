package app

import (
	"context"
	"encoding/json"
	"fmt"
	"reflect"
	"time"

	"github.com/butler/butler/internal/actionauth"
	"github.com/butler/butler/internal/delegation"
	"github.com/butler/butler/internal/ownership"
	"github.com/jackc/pgx/v5"
)

type admissionQuery interface {
	QueryRow(context.Context, string, ...any) pgx.Row
}

func (r *delegationAdmissionReader) readDispatch(ctx context.Context, q admissionQuery, e delegation.AdmissionEnvelope, args delegation.AdmissionArgs, lock bool) (delegationAdmissionProof, error) {
	grant, err := readAdmissionDispatch(ctx, q, e, delegation.AdmissionTool, r.hash, lock)
	return delegationAdmissionProof{envelope: e, grant: grant, args: args}, err
}

func readAdmissionDispatch(ctx context.Context, q admissionQuery, e delegation.AdmissionEnvelope, expectedTool, expectedHash string, lock bool) (actionauth.Grant, error) {
	return readAdmissionDispatchEffect(ctx, q, e, expectedTool, expectedHash, actionauth.EffectPrepare, lock)
}

func readAdmissionDispatchEffect(ctx context.Context, q admissionQuery, e delegation.AdmissionEnvelope, expectedTool, expectedHash string, expectedEffect actionauth.Effect, lock bool) (actionauth.Grant, error) {
	return readAdmissionDispatchRevision(ctx, q, e, expectedTool, expectedHash, "1", expectedEffect, lock)
}

func readAdmissionDispatchRevision(ctx context.Context, q admissionQuery, e delegation.AdmissionEnvelope, expectedTool, expectedHash, expectedRevision string, expectedEffect actionauth.Effect, lock bool) (actionauth.Grant, error) {
	return readOwnedDispatch(ctx, q, e, expectedTool, expectedHash, expectedRevision, expectedEffect, lock, false)
}
func readOwnedDispatch(ctx context.Context, q admissionQuery, e delegation.AdmissionEnvelope, expectedTool, expectedHash, expectedRevision string, expectedEffect actionauth.Effect, lock, allowConversation bool) (actionauth.Grant, error) {
	var grant actionauth.Grant
	var binding actionauth.ActionBinding
	var grantRaw, bindingRaw []byte
	var state, effectState, effectID, executionID, effectAuthorization, outcome, actionHash, fingerprint, grantID, grantHash, actionID, effectFingerprint string
	var grantExpires, intentExpires, now time.Time
	suffix := ""
	if lock {
		// Outcome recording and interrupted-effect reconciliation take the
		// journal row before authorization/dispatch. Stabilize it in that same
		// order; a multi-table lock alone can acquire authorization first.
		var effect string
		if err := q.QueryRow(ctx, `SELECT e.action_id FROM action_effects e JOIN action_intents i ON i.effect_id=e.action_id WHERE i.run_id=$1 AND i.tool_call_id=$2 AND e.action_id=$3 FOR SHARE OF e`, e.RunID, e.ToolCallID, e.EffectID).Scan(&effect); err != nil {
			return actionauth.Grant{}, delegation.ErrDenied
		}
		suffix = " FOR SHARE OF i,a,e,d"
	}
	err := q.QueryRow(ctx, `SELECT i.binding,a.grant_data,a.state,e.state,i.effect_id,d.execution_action_id,e.authorization_id,d.outcome,i.action_hash,i.effect_fingerprint,a.expires_at,i.expires_at,clock_timestamp(),a.authorization_id,a.action_hash,i.action_id,e.fingerprint
 FROM action_intents i JOIN action_authorizations a USING(action_id)
 JOIN action_effects e ON e.action_id=i.effect_id JOIN action_effect_dispatches d ON d.authorization_id=a.authorization_id AND d.action_id=e.action_id
 WHERE i.run_id=$1 AND i.tool_call_id=$2`+suffix, e.RunID, e.ToolCallID).Scan(&bindingRaw, &grantRaw, &state, &effectState, &effectID, &executionID, &effectAuthorization, &outcome, &actionHash, &fingerprint, &grantExpires, &intentExpires, &now, &grantID, &grantHash, &actionID, &effectFingerprint)
	if err != nil || json.Unmarshal(bindingRaw, &binding) != nil || json.Unmarshal(grantRaw, &grant) != nil {
		return actionauth.Grant{}, delegation.ErrDenied
	}
	// A SELECT target expression can be evaluated before its row lock waits.
	// Read the database clock again once all dispatch rows have been acquired.
	if q.QueryRow(ctx, `SELECT clock_timestamp()`).Scan(&now) != nil {
		return actionauth.Grant{}, delegation.ErrDenied
	}
	argsHash, err := actionauth.ArgumentsHash(e.ArgsJSON)
	_, expectedFingerprint, fpErr := actionauth.EffectIdentity(binding, "")
	if err != nil || fpErr != nil || fingerprint != expectedFingerprint || effectFingerprint != fingerprint || state != "inflight" || effectState != string(actionauth.StateExecuting) || outcome != "executing" || effectID != e.EffectID || executionID != binding.ActionID || actionID != binding.ActionID || grant.ID != effectAuthorization || grant.ID != grantID || grantHash != actionHash || binding.ActionHash != actionHash || actionauth.BindCredentialMetadata(binding, binding.CredentialMetadataHash).ActionHash != actionHash || !reflect.DeepEqual(binding, grant.Binding) || binding.ArgsHash != argsHash || binding.ContractHash != expectedHash || e.ContractHash != expectedHash || expectedRevision == "" || binding.ContractRevision != expectedRevision || binding.ToolName != expectedTool || binding.RunID != e.RunID || binding.ToolCallID != e.ToolCallID || (!allowConversation && binding.TaskID == "") || binding.TaskID != "" && (binding.TaskRevision < 1 || binding.ConstraintsRevision < 1) || binding.TaskID == "" && (binding.TaskRevision != 0 || binding.ConstraintsRevision != 0) || binding.OwnerID == "" || binding.Effect != expectedEffect || binding.Resource != "tool:"+expectedTool || len(binding.CredentialRefs) != 0 || binding.ExtensionID != "" || grant.LeaseID == "" || grant.LeaseEpoch < 1 || expectedEffect != actionauth.EffectRead && grant.ApprovalID == "" && grant.MandateID == "" || grant.MandateID != "" && (grant.MandateID != binding.MandateID || grant.MandateRevision != binding.MandateRevision) || !grantExpires.Equal(grant.ExpiresAt.UTC().Truncate(time.Microsecond)) || !grantExpires.After(now) || !intentExpires.After(now) {
		return actionauth.Grant{}, delegation.ErrDenied
	}
	grant.State = state
	if lock {
		token, ok := ownership.TokenFromContext(ctx)
		if !ok || token.RunID != binding.RunID || token.LeaseID != grant.LeaseID || token.FenceEpoch != grant.LeaseEpoch {
			return actionauth.Grant{}, delegation.ErrDenied
		}
		var valid bool
		if grant.AutonomyMode < 1 || expectedEffect != actionauth.EffectRead && grant.AutonomyMode < 2 || grant.AutonomyMode > 4 {
			return actionauth.Grant{}, delegation.ErrDenied
		}
		if err = q.QueryRow(ctx, `SELECT TRUE FROM session_ownership o JOIN runs r ON r.run_id=o.run_id JOIN sessions s ON s.session_key=r.session_key WHERE o.session_key=$1 AND o.run_id=$2 AND o.lease_id=$3 AND o.fence_epoch=$4 AND o.expires_at>clock_timestamp() AND r.session_key=o.session_key AND COALESCE(r.task_id,'')=$5 AND COALESCE(r.task_revision,0)=$6 AND s.user_id=$7 AND r.autonomy_mode=$8 AND r.current_state IN ('model_running','tool_pending','tool_running','awaiting_model_resume') AND r.status=r.current_state`, token.SessionKey, token.RunID, token.LeaseID, token.FenceEpoch, binding.TaskID, binding.TaskRevision, binding.OwnerID, fmt.Sprintf("mode_%d", grant.AutonomyMode-1)).Scan(&valid); err != nil {
			return actionauth.Grant{}, delegation.ErrDenied
		}
	}
	return grant, nil
}
