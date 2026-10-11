package bridge

import (
	"context"
	"encoding/json"
	"strings"

	"github.com/butler/butler/apps/browser-bridge/internal/protocol"
	runtimev1 "github.com/butler/butler/internal/gen/runtime/v1"
	toolbrokerv1 "github.com/butler/butler/internal/gen/toolbroker/v1"
	"github.com/butler/butler/internal/resourceexec"
)

// GuardedBackend has no raw control HTTP or secret resolution entry point.
// Register only the authenticated resourceexec.Runtime around this backend.
type GuardedBackend struct{ dispatcher *Dispatcher }

func NewGuardedBackend(d *Dispatcher) *GuardedBackend { return &GuardedBackend{d} }
func (b *GuardedBackend) Execute(ctx context.Context, req *runtimev1.ExecuteRequest) (*runtimev1.ExecuteResponse, error) {
	spec, ok := resourceexec.SpecFromContext(ctx)
	call := req.GetToolCall()
	if !ok || spec.SingleTab == nil || spec.DispatchID == "" || call == nil || len(req.GetResolvedCredentials()) != 0 || !resourceexec.ExactSingleTabContract(req.GetContract()) {
		return nil, resourceexec.ErrDenied
	}
	var args struct {
		SessionID   string `json:"session_id"`
		Observation string `json:"observation_id"`
	}
	if json.Unmarshal([]byte(call.GetArgsJson()), &args) != nil || args.SessionID != spec.SingleTab.ID || args.Observation != spec.Observation {
		return nil, resourceexec.ErrDenied
	}
	action := strings.TrimPrefix(call.GetToolName(), "single_tab.")
	if action != "status" && action != "click" {
		return nil, resourceexec.ErrDenied
	}
	params := protocol.GuardedParams{ActionDispatchParams: protocol.ActionDispatchParams{SingleTabSessionID: spec.SingleTab.ID, BoundTabRef: spec.SingleTab.TabRef, ActionType: action, ArgsJSON: call.GetArgsJson()}, BrowserInstance: spec.SingleTab.BrowserInstance, AllowedOrigin: spec.SingleTab.Origin, DispatchID: spec.DispatchID, ResourceID: spec.Lane, ResourceFence: spec.Fence, Deadline: spec.Deadline}
	var result protocol.GuardedResult
	if e := b.dispatcher.request(ctx, "action.guarded", params, &result); e != nil {
		return nil, resourceexec.ErrDenied
	}
	if result.DispatchID != spec.DispatchID || result.ResourceFence != spec.Fence || result.SingleTabSessionID != spec.SingleTab.ID || result.SessionStatus != "ACTIVE" {
		return nil, resourceexec.ErrDenied
	}
	var observation struct {
		OK      bool   `json:"ok"`
		ID      string `json:"observation_id"`
		Version int64  `json:"observation_version"`
	}
	if json.Unmarshal([]byte(result.ResultJSON), &observation) != nil || !observation.OK || observation.Version < 1 || (action == "status" && len(observation.ID) != 36) || (action == "click" && observation.ID != "") {
		return nil, resourceexec.ErrDenied
	}
	// Reconstruct the allowlisted observation. Native metadata/page text/errors
	// cannot be reflected into Broker/model results from a malformed response.
	encoded, _ := json.Marshal(observation)
	return &runtimev1.ExecuteResponse{Result: &toolbrokerv1.ToolResult{RunId: call.GetRunId(), ToolCallId: call.GetToolCallId(), ToolName: call.GetToolName(), Status: "completed", ResultJson: string(encoded)}}, nil
}
func (b *GuardedBackend) ConfirmRuntime(ctx context.Context, spec resourceexec.RuntimeSpec) error {
	if spec.SingleTab == nil {
		return resourceexec.ErrDenied
	}
	var result struct {
		Acknowledged bool `json:"acknowledged"`
	}
	params := protocol.GuardedAck{BoundTabRef: spec.SingleTab.TabRef, DispatchID: spec.DispatchID, ResourceID: spec.Lane, ResourceFence: spec.Fence}
	if b.dispatcher.request(ctx, "action.guarded.ack", params, &result) != nil || !result.Acknowledged {
		return resourceexec.ErrDenied
	}
	return nil
}
