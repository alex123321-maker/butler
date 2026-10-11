package broker

import (
	"context"
	"encoding/json"

	"github.com/butler/butler/internal/actionauth"
	"github.com/butler/butler/internal/credentials"
	toolbrokerv1 "github.com/butler/butler/internal/gen/toolbroker/v1"
	"github.com/butler/butler/internal/memory/candidatecontrol"
	"google.golang.org/protobuf/proto"
)

// MemoryCandidateExecutor follows the Broker's actual immutable dispatch and
// budget reservation. Only the owning service can publish a neutral candidate.
type MemoryCandidateExecutor struct {
	Candidates candidatecontrol.Service
	Next       RuntimeExecutor
}

func (r *MemoryCandidateExecutor) Execute(ctx context.Context, call *toolbrokerv1.ToolCall, contract *toolbrokerv1.ToolContract, secrets []credentials.ResolvedSecret) (*toolbrokerv1.ToolResult, error) {
	if call == nil {
		return nil, candidatecontrol.ErrDenied
	}
	if call.GetToolName() != candidatecontrol.Tool {
		if r == nil || r.Next == nil {
			return nil, candidatecontrol.ErrDenied
		}
		return r.Next.Execute(ctx, call, contract, secrets)
	}
	if r == nil || r.Candidates == nil || !proto.Equal(contract, candidatecontrol.Contract()) || len(secrets) != 0 || len(call.GetCredentialRefs()) != 0 {
		return nil, candidatecontrol.ErrDenied
	}
	if _, err := candidatecontrol.DecodeArgs(call.GetArgsJson()); err != nil {
		return nil, err
	}
	id := actionauth.DispatchIdentityFromContext(ctx)
	if id.ActionID == "" || id.Fingerprint == "" {
		return nil, candidatecontrol.ErrDenied
	}
	hash, err := actionauth.ContractHash(contract)
	if err != nil {
		return nil, err
	}
	receipt, err := r.Candidates.ProposeDispatched(ctx, candidatecontrol.Envelope{RunID: call.GetRunId(), ToolCallID: call.GetToolCallId(), EffectID: id.ActionID, ArgsJSON: call.GetArgsJson(), ContractHash: hash})
	if err != nil {
		return nil, err
	}
	raw, err := json.Marshal(receipt)
	if err != nil {
		return nil, err
	}
	return &toolbrokerv1.ToolResult{RunId: call.GetRunId(), ToolCallId: call.GetToolCallId(), ToolName: call.GetToolName(), Status: "completed", ResultJson: string(raw)}, nil
}
