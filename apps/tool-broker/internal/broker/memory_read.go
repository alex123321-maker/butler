package broker

import (
	"context"
	"encoding/json"

	"github.com/butler/butler/internal/actionauth"
	"github.com/butler/butler/internal/credentials"
	toolbrokerv1 "github.com/butler/butler/internal/gen/toolbroker/v1"
	"github.com/butler/butler/internal/memory/readcontrol"
	"google.golang.org/protobuf/proto"
)

type MemoryReadExecutor struct {
	Reads readcontrol.Service
	Next  RuntimeExecutor
}

func (r *MemoryReadExecutor) Execute(ctx context.Context, call *toolbrokerv1.ToolCall, contract *toolbrokerv1.ToolContract, secrets []credentials.ResolvedSecret) (*toolbrokerv1.ToolResult, error) {
	if call == nil {
		return nil, readcontrol.ErrDenied
	}
	if !readcontrol.IsTool(call.ToolName) {
		if r == nil || r.Next == nil {
			return nil, readcontrol.ErrDenied
		}
		return r.Next.Execute(ctx, call, contract, secrets)
	}
	if r == nil || r.Reads == nil || !proto.Equal(contract, readcontrol.Contract(call.ToolName)) || len(secrets) > 0 || len(call.CredentialRefs) > 0 {
		return nil, readcontrol.ErrDenied
	}
	if _, err := readcontrol.DecodeArgs(call.ToolName, call.ArgsJson); err != nil {
		return nil, err
	}
	id := actionauth.DispatchIdentityFromContext(ctx)
	if id.ActionID == "" || id.Fingerprint == "" {
		return nil, readcontrol.ErrDenied
	}
	hash, err := actionauth.ContractHash(contract)
	if err != nil {
		return nil, err
	}
	result, err := r.Reads.ReadDispatched(ctx, readcontrol.Envelope{RunID: call.RunId, ToolCallID: call.ToolCallId, EffectID: id.ActionID, Tool: call.ToolName, ArgsJSON: call.ArgsJson, ContractHash: hash})
	if err != nil {
		return nil, readcontrol.ErrDenied
	}
	raw, err := json.Marshal(result)
	if err != nil {
		return nil, readcontrol.ErrDenied
	}
	if _, err = readcontrol.DecodeResult(raw); err != nil {
		return nil, err
	}
	return &toolbrokerv1.ToolResult{RunId: call.RunId, ToolCallId: call.ToolCallId, ToolName: call.ToolName, Status: "completed", ResultJson: string(raw)}, nil
}
