package broker

import (
	"context"
	"encoding/json"

	"github.com/butler/butler/internal/actionauth"
	"github.com/butler/butler/internal/credentials"
	toolbrokerv1 "github.com/butler/butler/internal/gen/toolbroker/v1"
	"github.com/butler/butler/internal/singletabcontrol"
	"google.golang.org/protobuf/proto"
)

// GuardedTabRequestExecutor routes a local owning control, never to Chrome or
// a legacy bind runtime. The Broker's exact dispatch and budgets precede it.
type GuardedTabRequestExecutor struct {
	Requests singletabcontrol.RequestService
	Next     RuntimeExecutor
}

func (r *GuardedTabRequestExecutor) Execute(ctx context.Context, call *toolbrokerv1.ToolCall, contract *toolbrokerv1.ToolContract, secrets []credentials.ResolvedSecret) (*toolbrokerv1.ToolResult, error) {
	if call == nil {
		return nil, singletabcontrol.ErrDenied
	}
	if call.GetToolName() != singletabcontrol.BindTool {
		if r == nil || r.Next == nil {
			return nil, singletabcontrol.ErrDenied
		}
		return r.Next.Execute(ctx, call, contract, secrets)
	}
	if r == nil || r.Requests == nil || !proto.Equal(contract, singletabcontrol.RequestContract()) || len(secrets) != 0 || len(call.GetCredentialRefs()) != 0 || !singletabcontrol.ValidRequestArgs(call.GetArgsJson()) {
		return nil, singletabcontrol.ErrDenied
	}
	identity := actionauth.DispatchIdentityFromContext(ctx)
	if identity.ActionID == "" || identity.Fingerprint == "" {
		return nil, singletabcontrol.ErrDenied
	}
	hash, err := actionauth.ContractHash(contract)
	if err != nil {
		return nil, err
	}
	receipt, err := r.Requests.RequestDispatched(ctx, singletabcontrol.RequestEnvelope{RunID: call.GetRunId(), ToolCallID: call.GetToolCallId(), EffectID: identity.ActionID, ArgsJSON: call.GetArgsJson(), ContractHash: hash})
	if err != nil {
		return nil, err
	}
	raw, err := json.Marshal(receipt)
	if err != nil {
		return nil, err
	}
	return &toolbrokerv1.ToolResult{RunId: call.GetRunId(), ToolCallId: call.GetToolCallId(), ToolName: call.GetToolName(), Status: "completed", ResultJson: string(raw)}, nil
}
