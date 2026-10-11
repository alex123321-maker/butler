package broker

import (
	"context"
	"encoding/json"

	"github.com/butler/butler/internal/actionauth"
	"github.com/butler/butler/internal/credentials"
	"github.com/butler/butler/internal/delegation"
	toolbrokerv1 "github.com/butler/butler/internal/gen/toolbroker/v1"
	"github.com/butler/butler/internal/generatedtools"
	"google.golang.org/protobuf/proto"
)

// Optional private control route for the distinct reviewed developer contract.
// Initial context-only DelegationExecutor and runtimes are unchanged.
type ConverterDelegationExecutor struct {
	Admissions delegation.AdmissionService
	Next       RuntimeExecutor
}

func (r *ConverterDelegationExecutor) Execute(ctx context.Context, call *toolbrokerv1.ToolCall, contract *toolbrokerv1.ToolContract, secrets []credentials.ResolvedSecret) (*toolbrokerv1.ToolResult, error) {
	if call == nil {
		return nil, delegation.ErrDenied
	}
	if call.GetToolName() != generatedtools.DeveloperAdmissionTool {
		if r == nil || r.Next == nil {
			return nil, delegation.ErrDenied
		}
		return r.Next.Execute(ctx, call, contract, secrets)
	}
	if r == nil || r.Admissions == nil || !proto.Equal(contract, generatedtools.DeveloperAdmissionContract()) || len(secrets) != 0 || len(call.GetCredentialRefs()) != 0 {
		return nil, delegation.ErrDenied
	}
	identity := actionauth.DispatchIdentityFromContext(ctx)
	if identity.ActionID == "" || identity.Fingerprint == "" {
		return nil, delegation.ErrDenied
	}
	if _, err := generatedtools.DecodeDeveloperArgs(call.GetArgsJson()); err != nil {
		return nil, err
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
	return &toolbrokerv1.ToolResult{RunId: call.RunId, ToolCallId: call.ToolCallId, ToolName: call.ToolName, Status: "completed", ResultJson: string(raw)}, nil
}
