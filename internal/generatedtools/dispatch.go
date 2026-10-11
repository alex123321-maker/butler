package generatedtools

import (
	"context"
	"encoding/json"

	"github.com/butler/butler/internal/actionauth"
	converterv1 "github.com/butler/butler/internal/gen/converter/v1"
	toolbrokerv1 "github.com/butler/butler/internal/gen/toolbroker/v1"
	"github.com/butler/butler/internal/memory/catalog"
	"github.com/jackc/pgx/v5"
)

func (s *Store) dispatchTx(ctx context.Context, tx pgx.Tx, c actionauth.ContextSnapshot, call *toolbrokerv1.ToolCall, contract *toolbrokerv1.ToolContract) error {
	d := actionauth.DispatchIdentityFromContext(ctx)
	if call == nil || contract == nil || d.ActionID == "" || !digestName.MatchString(d.Fingerprint) || call.GetRunId() != c.RunID || len(call.GetCredentialRefs()) != 0 {
		return ErrAccess
	}
	var raw []byte
	var fingerprint, state, authState, outcome string
	var b actionauth.ActionBinding
	// The intent is immutable; recheck its current bound source/catalog state
	// before taking effect locks. Publication after RPC cannot use an old grant.
	e := tx.QueryRow(ctx, `SELECT binding FROM action_intents WHERE run_id=$1 AND tool_call_id=$2`, c.RunID, call.GetToolCallId()).Scan(&raw)
	if e != nil || json.Unmarshal(raw, &b) != nil || s.CheckActionTx(ctx, tx, b) != nil {
		return ErrAccess
	}
	e = tx.QueryRow(ctx, `SELECT e.fingerprint,e.state,a.state,d.outcome FROM action_intents i JOIN action_effects e ON e.action_id=i.effect_id JOIN action_effect_dispatches d ON d.execution_action_id=i.action_id JOIN action_authorizations a ON a.authorization_id=d.authorization_id WHERE e.action_id=$1 AND i.run_id=$2 AND i.tool_call_id=$3 AND e.authorization_id=d.authorization_id FOR SHARE OF e,a,d`, d.ActionID, c.RunID, call.GetToolCallId()).Scan(&fingerprint, &state, &authState, &outcome)
	if e != nil || fingerprint != d.Fingerprint || state != "executing" || authState != "inflight" || outcome != "executing" || b.OwnerID != c.OwnerID || b.TaskID != c.TaskID || b.TaskRevision != c.TaskRevision || b.ConstraintsRevision != c.ConstraintsRevision || b.ToolName != call.GetToolName() {
		return ErrAccess
	}
	argsHash, e := actionauth.ArgumentsHash(call.GetArgsJson())
	if e != nil || argsHash != b.ArgsHash {
		return ErrAccess
	}
	contractHash, e := actionauth.ContractHash(contract)
	if e != nil || contractHash != b.ContractHash {
		return ErrAccess
	}
	return nil
}
func dataEnvelope(s catalog.Source) *converterv1.SourceData {
	return &converterv1.SourceData{SourceId: s.ID, Revision: s.Revision, AccessRevision: s.AccessRevision, ContentHash: s.Hash, Body: []byte(s.Body), BodyHash: HashBytes([]byte(s.Body)), OriginJson: encoded(s.Origin)}
}
func proof(ctx context.Context, call *toolbrokerv1.ToolCall) *converterv1.ExecutionProof {
	d := actionauth.DispatchIdentityFromContext(ctx)
	return &converterv1.ExecutionProof{RunId: call.GetRunId(), ToolCallId: call.GetToolCallId(), ActionId: d.ActionID, ActionFingerprint: d.Fingerprint}
}
