package broker

import (
	"context"
	"encoding/json"
	"github.com/butler/butler/internal/actionauth"
	"github.com/butler/butler/internal/credentials"
	toolbrokerv1 "github.com/butler/butler/internal/gen/toolbroker/v1"
	"github.com/butler/butler/internal/schedulecontrol"
	"google.golang.org/protobuf/proto"
)

type ScheduleExecutor struct {
	Schedules schedulecontrol.Service
	Next      RuntimeExecutor
}

func (r *ScheduleExecutor) Execute(ctx context.Context, call *toolbrokerv1.ToolCall, contract *toolbrokerv1.ToolContract, secrets []credentials.ResolvedSecret) (*toolbrokerv1.ToolResult, error) {
	if call == nil {
		return nil, schedulecontrol.ErrDenied
	}
	if !schedulecontrol.IsTool(call.GetToolName()) {
		if r == nil || r.Next == nil {
			return nil, schedulecontrol.ErrDenied
		}
		return r.Next.Execute(ctx, call, contract, secrets)
	}
	if r == nil || r.Schedules == nil || !proto.Equal(contract, schedulecontrol.Contract(call.GetToolName())) || len(secrets) != 0 || len(call.GetCredentialRefs()) != 0 {
		return nil, schedulecontrol.ErrDenied
	}
	if _, err := schedulecontrol.DecodeArgs(call.GetToolName(), call.GetArgsJson()); err != nil {
		return nil, err
	}
	id := actionauth.DispatchIdentityFromContext(ctx)
	if id.ActionID == "" || id.Fingerprint == "" {
		return nil, schedulecontrol.ErrDenied
	}
	hash, err := actionauth.ContractHash(contract)
	if err != nil {
		return nil, err
	}
	receipt, err := r.Schedules.ExecuteDispatched(ctx, schedulecontrol.Envelope{RunID: call.GetRunId(), ToolCallID: call.GetToolCallId(), Tool: call.GetToolName(), EffectID: id.ActionID, ArgsJSON: call.GetArgsJson(), ContractHash: hash})
	if err != nil {
		return nil, err
	}
	raw, err := json.Marshal(receipt)
	if err != nil {
		return nil, err
	}
	return &toolbrokerv1.ToolResult{RunId: call.GetRunId(), ToolCallId: call.GetToolCallId(), ToolName: call.GetToolName(), Status: "completed", ResultJson: string(raw)}, nil
}
