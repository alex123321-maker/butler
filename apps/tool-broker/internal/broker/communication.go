package broker

import (
	"context"
	"encoding/json"
	"github.com/butler/butler/internal/actionauth"
	"github.com/butler/butler/internal/communicationcontrol"
	"github.com/butler/butler/internal/credentials"
	toolbrokerv1 "github.com/butler/butler/internal/gen/toolbroker/v1"
	"google.golang.org/protobuf/proto"
)

type CommunicationExecutor struct {
	Communication communicationcontrol.Service
	Next          RuntimeExecutor
}

func (r *CommunicationExecutor) Execute(ctx context.Context, c *toolbrokerv1.ToolCall, contract *toolbrokerv1.ToolContract, secrets []credentials.ResolvedSecret) (*toolbrokerv1.ToolResult, error) {
	if c == nil {
		return nil, communicationcontrol.ErrDenied
	}
	if !communicationcontrol.IsTool(c.ToolName) {
		if r == nil || r.Next == nil {
			return nil, communicationcontrol.ErrDenied
		}
		return r.Next.Execute(ctx, c, contract, secrets)
	}
	if r == nil || r.Communication == nil || len(secrets) != 0 || len(c.CredentialRefs) != 0 || !proto.Equal(contract, communicationcontrol.Contract(c.ToolName)) {
		return nil, communicationcontrol.ErrDenied
	}
	if _, e := communicationcontrol.DecodeArgs(c.ToolName, c.ArgsJson); e != nil {
		return nil, e
	}
	id := actionauth.DispatchIdentityFromContext(ctx)
	if id.ActionID == "" || id.Fingerprint == "" {
		return nil, communicationcontrol.ErrDenied
	}
	hash, e := actionauth.ContractHash(contract)
	if e != nil {
		return nil, e
	}
	env := communicationcontrol.Envelope{RunID: c.RunId, ToolCallID: c.ToolCallId, EffectID: id.ActionID, Tool: c.ToolName, ArgsJSON: c.ArgsJson, ContractHash: hash}
	receipt, e := r.Communication.ExecuteDispatched(ctx, env)
	// Resolve a lost internal ACK by reading the original durable receipt. This
	// second operation has no permission to publish or resend anything.
	if e != nil {
		env.Lookup = true
		receipt, e = r.Communication.ExecuteDispatched(ctx, env)
	}
	if e != nil {
		return nil, e
	}
	raw, e := json.Marshal(receipt)
	if e != nil {
		return nil, e
	}
	return &toolbrokerv1.ToolResult{RunId: c.RunId, ToolCallId: c.ToolCallId, ToolName: c.ToolName, Status: "completed", ResultJson: string(raw)}, nil
}
