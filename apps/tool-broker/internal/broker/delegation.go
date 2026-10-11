package broker

import (
	"context"
	"encoding/json"

	"github.com/butler/butler/internal/actionauth"
	"github.com/butler/butler/internal/credentials"
	"github.com/butler/butler/internal/delegation"
	toolbrokerv1 "github.com/butler/butler/internal/gen/toolbroker/v1"
	"google.golang.org/protobuf/proto"
)

// DelegationExecutor is an optional Broker-owned control route. Isolated tool
// runtimes never orchestrate children or receive the private control token.
type DelegationExecutor struct {
	Admissions delegation.AdmissionService
	Next       RuntimeExecutor
}

func (r *DelegationExecutor) Execute(ctx context.Context, call *toolbrokerv1.ToolCall, contract *toolbrokerv1.ToolContract, secrets []credentials.ResolvedSecret) (*toolbrokerv1.ToolResult, error) {
	if call == nil {
		return nil, delegation.ErrDenied
	}
	if call.GetToolName() != delegation.AdmissionTool {
		if r == nil || r.Next == nil {
			return nil, delegation.ErrDenied
		}
		return r.Next.Execute(ctx, call, contract, secrets)
	}
	if r == nil || r.Admissions == nil || !proto.Equal(contract, delegation.AdmissionContract()) || len(secrets) != 0 || len(call.GetCredentialRefs()) != 0 {
		return nil, delegation.ErrDenied
	}
	identity := actionauth.DispatchIdentityFromContext(ctx)
	if identity.ActionID == "" || identity.Fingerprint == "" {
		return nil, delegation.ErrDenied
	}
	if _, err := actionauth.ArgumentsHash(call.GetArgsJson()); err != nil {
		return nil, delegation.ErrDenied
	}
	if _, err := delegation.DecodeAdmissionArgs(call.GetArgsJson()); err != nil {
		return nil, delegation.ErrDenied
	}
	hash, err := actionauth.ContractHash(contract)
	if err != nil {
		return nil, err
	}
	receipt, err := r.Admissions.AdmitDispatched(ctx, delegation.AdmissionEnvelope{RunID: call.GetRunId(), ToolCallID: call.GetToolCallId(), EffectID: identity.ActionID, ArgsJSON: call.GetArgsJson(), ContractHash: hash})
	if err != nil {
		return nil, err
	}
	raw, err := json.Marshal(receipt)
	if err != nil {
		return nil, err
	}
	return &toolbrokerv1.ToolResult{RunId: call.GetRunId(), ToolCallId: call.GetToolCallId(), ToolName: call.GetToolName(), Status: "completed", ResultJson: string(raw)}, nil
}
