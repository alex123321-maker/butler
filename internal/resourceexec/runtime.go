package resourceexec

import (
	"context"
	"crypto/subtle"
	"encoding/json"
	"sync"
	"time"

	"github.com/butler/butler/internal/actionauth"
	commonv1 "github.com/butler/butler/internal/gen/common/v1"
	runtimev1 "github.com/butler/butler/internal/gen/runtime/v1"
	toolbrokerv1 "github.com/butler/butler/internal/gen/toolbroker/v1"
	"google.golang.org/grpc/metadata"
)

type RuntimeBackend interface {
	Execute(context.Context, *runtimev1.ExecuteRequest) (*runtimev1.ExecuteResponse, error)
}
type lane struct {
	mu    sync.Mutex
	fence int64
}

// Runtime wraps the actual browser server, not a second orchestration loop.
// Register this wrapper instead of the raw backend when resource arbitration is
// enabled. The backend retains all original origin/page/credential checks.
type Runtime struct {
	runtimev1.UnimplementedToolRuntimeServiceServer
	backend   RuntimeBackend
	authority RuntimeAuthority
	token     string
	mu        sync.Mutex
	lanes     map[string]*lane
}

func NewRuntime(backend RuntimeBackend, authority RuntimeAuthority, brokerToken string) (*Runtime, error) {
	if backend == nil || authority == nil || !validServiceToken(brokerToken) {
		return nil, ErrDenied
	}
	return &Runtime{backend: backend, authority: authority, token: brokerToken, lanes: map[string]*lane{}}, nil
}
func (r *Runtime) Execute(ctx context.Context, req *runtimev1.ExecuteRequest) (*runtimev1.ExecuteResponse, error) {
	fail := func() (*runtimev1.ExecuteResponse, error) {
		call := req.GetToolCall()
		return &runtimev1.ExecuteResponse{Result: &toolbrokerv1.ToolResult{RunId: call.GetRunId(), ToolCallId: call.GetToolCallId(), ToolName: call.GetToolName(), Status: "effect_unknown", Error: &toolbrokerv1.ToolError{ErrorClass: commonv1.ErrorClass_ERROR_CLASS_POLICY_DENIED, Message: "guarded browser dispatch denied or its outcome is unknown", Retryable: false}}}, nil
	}
	values := metadata.ValueFromIncomingContext(ctx, "authorization")
	tickets := metadata.ValueFromIncomingContext(ctx, ticketHeader)
	if len(values) != 1 || subtle.ConstantTimeCompare([]byte(values[0]), []byte("Bearer "+r.token)) != 1 || len(tickets) != 1 || !Protected(req.GetToolCall().GetToolName()) {
		return fail()
	}
	request, e := runtimeRequest(req, tickets[0])
	if e != nil {
		return fail()
	}
	spec, e := r.authority.Inspect(ctx, request)
	if e != nil || spec.Lane == "" || spec.Fence < 1 {
		return fail()
	}
	r.mu.Lock()
	l := r.lanes[spec.Lane]
	if l == nil && len(r.lanes) < 1024 {
		l = &lane{}
		r.lanes[spec.Lane] = l
	}
	r.mu.Unlock()
	if l == nil || !l.mu.TryLock() {
		return fail()
	}
	defer l.mu.Unlock()
	if spec.Fence <= l.fence {
		return fail()
	}
	// Inspect is not admission. Repeat online authority AFTER the local lane
	// lock, then atomically consume the one-use ticket before the concrete backend.
	admitted, e := r.authority.Begin(ctx, request)
	if e != nil || admitted.Lane != spec.Lane || admitted.Fence != spec.Fence || admitted.Deadline.After(spec.Deadline) || !admitted.Deadline.After(time.Now()) {
		return fail()
	}
	l.fence = admitted.Fence
	runCtx, cancel := context.WithDeadline(context.WithValue(ctx, runtimeSpecKey{}, admitted), admitted.Deadline)
	defer cancel()
	response, runErr := r.backend.Execute(runCtx, req)
	result := response.GetResult()
	completed := runErr == nil && result != nil && result.GetStatus() == "completed" && result.GetError() == nil && result.GetRunId() == request.RunID && result.GetToolCallId() == request.ToolCallID && result.GetToolName() == request.ToolName && runCtx.Err() == nil
	// A correlated closed pre-input rejection completes the lane, not an
	// external effect. Preserve its proof for Broker/PG's independent verifier.
	// Lost transport/finish, changed authority and in-flight errors stay unknown.
	rejected := runErr == nil && runCtx.Err() == nil && (localPreparationRejected(req, result) || navigationRejected(req, result))
	// Hash the actual local response; never send its content or resolved secrets
	// back to the resource service. "completed" means the guarded lane finished,
	// not verified external success or permission to replay an effect.
	encoded, _ := json.Marshal(response)
	receipt := Receipt{RuntimeRequest: request, Completed: completed || rejected, Digest: hash(string(encoded))}
	if completed {
		var observation struct {
			SessionID string    `json:"browser_session_id"`
			ExpiresAt time.Time `json:"expires_at"`
		}
		if json.Unmarshal([]byte(result.GetResultJson()), &observation) != nil {
			return fail()
		}
		receipt.SessionID, receipt.SessionExpiresAt = observation.SessionID, observation.ExpiresAt
	}
	finishCtx, cancelFinish := context.WithTimeout(context.WithoutCancel(ctx), 3*time.Second)
	defer cancelFinish()
	if e = r.authority.Finish(finishCtx, receipt); e != nil || !(completed || rejected) {
		return fail()
	}
	if confirmed, ok := r.backend.(interface {
		ConfirmRuntime(context.Context, RuntimeSpec) error
	}); ok && !rejected {
		if e = confirmed.ConfirmRuntime(finishCtx, admitted); e != nil {
			return fail()
		}
	}
	return response, nil
}

func localPreparationRejected(req *runtimev1.ExecuteRequest, result *toolbrokerv1.ToolResult) bool {
	c, call := req.GetContract(), req.GetToolCall()
	if c.GetToolName() != call.GetToolName() || c.GetEffectContract().GetRetrySafety() != "unsafe" || c.GetEffectContract().GetVerificationMethod() != "local_browser_preparation_v1" {
		return false
	}
	b := actionauth.ActionBinding{RunID: call.GetRunId(), ToolCallID: call.GetToolCallId(), ToolName: call.GetToolName(), ContractRevision: c.GetContractRevision(), Effect: actionauth.EffectPrepare}
	return actionauth.BrowserPreparationRejection(b, result) != ""
}

func navigationRejected(req *runtimev1.ExecuteRequest, result *toolbrokerv1.ToolResult) bool {
	c, call := req.GetContract(), req.GetToolCall()
	if c.GetToolName() != call.GetToolName() || c.GetEffectContract().GetRetrySafety() != "read_only" || c.GetEffectContract().GetVerificationMethod() != actionauth.BrowserGETVerification || c.GetSupportsCredentialRefs() || len(call.GetCredentialRefs()) != 0 {
		return false
	}
	return actionauth.BrowserReadRejection(actionauth.ActionBinding{RunID: call.GetRunId(), ToolCallID: call.GetToolCallId(), ToolName: call.GetToolName(), ContractRevision: c.GetContractRevision(), Effect: actionauth.EffectRead}, result) != ""
}
