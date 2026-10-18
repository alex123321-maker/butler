package broker

import (
	"context"
	"encoding/json"

	"github.com/butler/butler/internal/actionauth"
	"github.com/butler/butler/internal/credentials"
	toolbrokerv1 "github.com/butler/butler/internal/gen/toolbroker/v1"
	"github.com/butler/butler/internal/memory/factcontrol"
	"github.com/butler/butler/internal/memory/readcontrol"
	"google.golang.org/protobuf/proto"
)

type MemoryFactExecutor struct {
	Facts readcontrol.Service
	Next  RuntimeExecutor
}

func (r *MemoryFactExecutor) Execute(ctx context.Context, call *toolbrokerv1.ToolCall, contract *toolbrokerv1.ToolContract, secrets []credentials.ResolvedSecret) (*toolbrokerv1.ToolResult, error) {
	if call == nil {
		return nil, factcontrol.ErrDenied
	}
	if call.ToolName != factcontrol.Tool {
		if r == nil || r.Next == nil {
			return nil, factcontrol.ErrDenied
		}
		return r.Next.Execute(ctx, call, contract, secrets)
	}
	if r == nil || r.Facts == nil || !proto.Equal(contract, factcontrol.Contract()) || len(secrets) > 0 || len(call.CredentialRefs) > 0 {
		return nil, factcontrol.ErrDenied
	}
	if _, err := factcontrol.DecodeArgs(call.ArgsJson); err != nil {
		return nil, err
	}
	id := actionauth.DispatchIdentityFromContext(ctx)
	if id.ActionID == "" || id.Fingerprint == "" {
		return nil, factcontrol.ErrDenied
	}
	hash, err := actionauth.ContractHash(contract)
	if err != nil {
		return nil, err
	}
	out, err := r.Facts.ReadDispatched(ctx, readcontrol.Envelope{RunID: call.RunId, ToolCallID: call.ToolCallId, EffectID: id.ActionID, Tool: call.ToolName, ArgsJSON: call.ArgsJson, ContractHash: hash})
	if err != nil {
		return nil, factcontrol.ErrDenied
	}
	raw, err := json.Marshal(out)
	if err != nil {
		return nil, err
	}
	if _, err = readcontrol.DecodeResult(raw); err != nil {
		return nil, err
	}
	return &toolbrokerv1.ToolResult{RunId: call.RunId, ToolCallId: call.ToolCallId, ToolName: call.ToolName, Status: "completed", ResultJson: string(raw)}, nil
}
