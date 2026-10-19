package broker

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"time"

	"github.com/butler/butler/apps/tool-broker/internal/registry"
	"github.com/butler/butler/internal/actionauth"
	"github.com/butler/butler/internal/budget"
	"github.com/butler/butler/internal/credentials"
	commonv1 "github.com/butler/butler/internal/gen/common/v1"
	toolbrokerv1 "github.com/butler/butler/internal/gen/toolbroker/v1"
	"github.com/butler/butler/internal/logger"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

type Server struct {
	toolbrokerv1.UnimplementedToolBrokerServiceServer

	registry          *registry.Registry
	executor          RuntimeExecutor
	creds             CredentialResolver
	authorizer        *actionauth.Service
	log               *slog.Logger
	budgetTimeout     time.Duration
	budgets           *budget.Controller
	scopedCatalog     ScopedCatalog
	resourceAdmission ResourceAdmission
	browserControl    BrowserControl
}
type BrowserControl interface {
	Before(context.Context, *toolbrokerv1.ToolCall) error
	Record(context.Context, *toolbrokerv1.ToolCall, actionauth.Grant, *toolbrokerv1.ToolResult) error
}

func (s *Server) SetBrowserControl(control BrowserControl) { s.browserControl = control }

type ScopedCatalog interface {
	ListForRun(context.Context, string, string, int) ([]*toolbrokerv1.ToolContract, error)
}

func (s *Server) SetScopedCatalog(c ScopedCatalog) { s.scopedCatalog = c }

// SetCatalogExecutor is startup wiring for finite broker-owned local catalog operations.
func (s *Server) SetCatalogExecutor(e RuntimeExecutor) { s.executor = e }

func (s *Server) SetActionAuthorizer(authorizer *actionauth.Service) { s.authorizer = authorizer }
func (s *Server) SetBudgetTimeout(d time.Duration)                   { s.budgetTimeout = d }
func (s *Server) SetBudgets(c *budget.Controller)                    { s.budgets = c }

func (s *Server) PrepareToolCall(ctx context.Context, req *toolbrokerv1.PrepareToolCallRequest) (*toolbrokerv1.PrepareToolCallResponse, error) {
	valid, contract, toolErr := s.registry.ValidateContext(ctx, req.GetToolCall())
	if !valid {
		return &toolbrokerv1.PrepareToolCallResponse{Error: toolErr}, nil
	}
	if s.authorizer == nil {
		return nil, status.Error(codes.FailedPrecondition, "action authorization is not configured")
	}
	prepared, err := s.authorizer.PrepareWithLogicalRef(ctx, req.GetToolCall(), contract, req.GetLogicalActionRef())
	if err != nil {
		return nil, status.Error(codes.PermissionDenied, err.Error())
	}
	encoded, _ := json.Marshal(prepared.Binding)
	return &toolbrokerv1.PrepareToolCallResponse{Action: &toolbrokerv1.PreparedAction{ActionId: prepared.Binding.ActionID, EffectId: prepared.EffectID, ActionHash: prepared.Binding.ActionHash, BindingJson: string(encoded), RequiresApproval: prepared.RequiresApproval, Summary: prepared.Summary, ExpiresAt: prepared.ExpiresAt.Format(time.RFC3339Nano)}}, nil
}
func (s *Server) AuthorizeToolCall(ctx context.Context, req *toolbrokerv1.AuthorizeToolCallRequest) (*toolbrokerv1.AuthorizeToolCallResponse, error) {
	valid, contract, toolErr := s.registry.ValidateContext(ctx, req.GetToolCall())
	if !valid {
		return &toolbrokerv1.AuthorizeToolCallResponse{Error: toolErr}, nil
	}
	if s.authorizer == nil {
		return nil, status.Error(codes.FailedPrecondition, "action authorization is not configured")
	}
	grant, err := s.authorizer.Authorize(ctx, req.GetToolCall(), contract, req.GetActionId(), req.GetApprovalId(), req.GetMandateId())
	if err != nil {
		return nil, status.Error(codes.PermissionDenied, err.Error())
	}
	return &toolbrokerv1.AuthorizeToolCallResponse{AuthorizationId: grant.ID}, nil
}

type RuntimeExecutor interface {
	Execute(context.Context, *toolbrokerv1.ToolCall, *toolbrokerv1.ToolContract, []credentials.ResolvedSecret) (*toolbrokerv1.ToolResult, error)
}

type CredentialResolver interface {
	ResolveToolCall(context.Context, *toolbrokerv1.ToolCall) ([]credentials.ResolvedSecret, error)
}

func NewServer(registry *registry.Registry, executor RuntimeExecutor, creds CredentialResolver, log *slog.Logger) *Server {
	if log == nil {
		log = slog.Default()
	}
	return &Server{registry: registry, executor: executor, creds: creds, log: logger.WithComponent(log, "tool-broker")}
}

func (s *Server) ValidateToolCall(ctx context.Context, req *toolbrokerv1.ValidateToolCallRequest) (*toolbrokerv1.ValidateToolCallResponse, error) {
	valid, contract, toolErr := s.registry.ValidateContext(ctx, req.GetToolCall())
	return &toolbrokerv1.ValidateToolCallResponse{Valid: valid, Contract: contract, Error: toolErr}, nil
}

func (s *Server) ExecuteToolCall(ctx context.Context, req *toolbrokerv1.ExecuteToolCallRequest) (*toolbrokerv1.ExecuteToolCallResponse, error) {
	if s.executor == nil {
		return nil, status.Error(codes.FailedPrecondition, "tool execution routing is not configured")
	}
	valid, contract, toolErr := s.registry.ValidateContext(ctx, req.GetToolCall())
	if !valid {
		return &toolbrokerv1.ExecuteToolCallResponse{Result: invalidResult(req.GetToolCall(), toolErr)}, nil
	}
	if s.authorizer == nil {
		return &toolbrokerv1.ExecuteToolCallResponse{Result: authorizationErrorResult(req.GetToolCall(), "action authorization is not configured")}, nil
	}
	if s.browserControl != nil {
		if err := s.browserControl.Before(ctx, req.GetToolCall()); err != nil {
			return &toolbrokerv1.ExecuteToolCallResponse{Result: authorizationErrorResult(req.GetToolCall(), "browser is under owner control, unavailable or requires a fresh snapshot; no action dispatched")}, nil
		}
	}
	grant, err := s.authorizer.Verify(ctx, req.GetToolCall(), contract, req.GetAuthorizationId())
	if err != nil {
		if recorded, e := s.authorizer.RecordedEffect(ctx, req.GetAuthorizationId(), req.GetToolCall()); e == nil {
			result := authorizationErrorResult(req.GetToolCall(), "effect already dispatched; execution permission is spent")
			if recorded.State == actionauth.StateExecuting || recorded.State == actionauth.StateUnknown {
				result = unknownResult(req.GetToolCall(), recorded.ActionID)
			}
			result.ActionId = recorded.ActionID
			result.EffectState = string(recorded.State)
			if recorded.State == actionauth.StateExecuting {
				result.EffectState = string(actionauth.StateUnknown)
			}
			result.EvidenceRefs = recorded.EvidenceRefs
			return &toolbrokerv1.ExecuteToolCallResponse{Result: result}, nil
		}
		return &toolbrokerv1.ExecuteToolCallResponse{Result: authorizationErrorResult(req.GetToolCall(), err.Error())}, nil
	}
	effect, err := s.authorizer.EffectForGrant(ctx, grant)
	if err != nil {
		return &toolbrokerv1.ExecuteToolCallResponse{Result: authorizationErrorResult(req.GetToolCall(), "durable effect journal is unavailable")}, nil
	}
	resolved, err := s.resolveAuthorizedCredentials(ctx, req.GetToolCall(), grant, contract)
	if err != nil {
		s.log.Error("credential resolution denied", slog.String("tool_name", req.GetToolCall().GetToolName()), slog.String("tool_call_id", req.GetToolCall().GetToolCallId()))
		return &toolbrokerv1.ExecuteToolCallResponse{Result: credentialErrorResult(req.GetToolCall(), err)}, nil
	}
	if _, err = s.authorizer.Verify(ctx, req.GetToolCall(), contract, grant.ID); err != nil {
		return &toolbrokerv1.ExecuteToolCallResponse{Result: authorizationErrorResult(req.GetToolCall(), err.Error())}, nil
	}
	ctx, err = budget.WithCanonicalArguments(ctx, req.GetToolCall().GetArgsJson())
	if err != nil {
		return &toolbrokerv1.ExecuteToolCallResponse{Result: authorizationErrorResult(req.GetToolCall(), "tool arguments cannot be normalized")}, nil
	}
	if s.resourceAdmission != nil {
		ctx, err = s.resourceAdmission.PrepareContext(ctx, req.GetToolCall(), contract, grant)
		if err != nil {
			return &toolbrokerv1.ExecuteToolCallResponse{Result: authorizationErrorResult(req.GetToolCall(), "current guarded resource admission is required")}, nil
		}
	}
	if err = s.authorizer.Claim(ctx, grant.ID); err != nil {
		var exhausted *budget.ExhaustedError
		var loop *budget.LoopError
		reason := ""
		if errors.As(err, &exhausted) {
			reason = "budget_" + exhausted.Dimension + "_" + exhausted.Kind
		} else if errors.As(err, &loop) {
			reason = loop.Reason
		}
		if reason != "" {
			result := authorizationErrorResult(req.GetToolCall(), reason)
			result.Status = "execution_stopped"
			data, _ := json.Marshal(map[string]string{"reason": reason})
			result.Error.DetailsJson = string(data)
			return &toolbrokerv1.ExecuteToolCallResponse{Result: result}, nil
		}
		return &toolbrokerv1.ExecuteToolCallResponse{Result: authorizationErrorResult(req.GetToolCall(), "authorization already dispatched, expired or revoked; do not replay")}, nil
	}
	dispatchCtx := actionauth.WithDispatchIdentity(ctx, actionauth.DispatchIdentity{ActionID: effect.ActionID, IdempotencyKey: effect.IdempotencyKey, Fingerprint: effect.Fingerprint})
	// The grant includes the earlier owner-policy/active-hours bound. Persisted
	// budget periods remain immutable and cannot widen this call's authority.
	dispatchCtx, cancelAuthority := context.WithDeadline(dispatchCtx, grant.ExpiresAt)
	defer cancelAuthority()
	if s.budgets != nil {
		deadline, deadlineErr := s.budgets.ToolDeadline(ctx, grant.Binding.OwnerID, effect.ActionID)
		if deadlineErr != nil || !deadline.After(time.Now()) {
			return &toolbrokerv1.ExecuteToolCallResponse{Result: unknownResult(req.GetToolCall(), effect.ActionID)}, nil
		}
		var cancel context.CancelFunc
		dispatchCtx, cancel = context.WithDeadline(dispatchCtx, deadline)
		defer cancel()
	}
	if s.budgetTimeout > 0 {
		var cancel context.CancelFunc
		dispatchCtx, cancel = context.WithTimeout(dispatchCtx, s.budgetTimeout)
		defer cancel()
	}
	result, err := s.executor.Execute(dispatchCtx, req.GetToolCall(), contract, resolved)
	if result != nil && result.GetStatus() == "completed" {
		if schemaErr := registry.ValidateArgs(contract.GetOutputSchemaJson(), result.GetResultJson()); schemaErr != nil || result.GetError() != nil {
			result.Status = "failed"
			result.ResultJson = "{}"
			result.Error = &toolbrokerv1.ToolError{ErrorClass: commonv1.ErrorClass_ERROR_CLASS_VALIDATION_ERROR, Message: "runtime result does not satisfy tool output schema", Retryable: false}
			err = fmt.Errorf("runtime result schema invalid")
		}
	}
	result = redactResult(result, resolved)
	if err == nil && s.browserControl != nil {
		if controlErr := s.browserControl.Record(ctx, req.GetToolCall(), grant, result); controlErr != nil {
			err = controlErr
		}
	}
	finishCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 3*time.Second)
	defer cancel()
	recorded, finishErr := s.authorizer.Outcome(finishCtx, grant, result, err != nil)
	if finishErr != nil {
		s.log.Error("action dispatch outcome recording failed", slog.String("authorization_id", grant.ID))
		return &toolbrokerv1.ExecuteToolCallResponse{Result: unknownResult(req.GetToolCall(), effect.ActionID)}, nil
	}
	if recorded.State == actionauth.StateUnknown {
		if result == nil {
			result = unknownResult(req.GetToolCall(), effect.ActionID)
		} else if err == nil && grant.Binding.Effect == actionauth.EffectRead && reportableReadFailure(result, contract, grant.Binding) {
			// An explicit failed read has useful bounded runtime diagnostics (for
			// example HTTP status or unsupported MIME). Keep them while retaining
			// the journal's uncertain effect state; this grants no retry or success.
			result.Error.Retryable = false
		} else {
			result.Status = "effect_unknown"
			result.Error = &toolbrokerv1.ToolError{ErrorClass: commonv1.ErrorClass_ERROR_CLASS_INTERNAL_ERROR, Message: "external effect outcome is unknown; verify before any new attempt", Retryable: false, DetailsJson: fmt.Sprintf(`{"action_id":%q,"dispatch_outcome":%q}`, effect.ActionID, recorded.DispatchOutcome)}
		}
	}
	result.ActionId = recorded.ActionID
	result.RunId = req.GetToolCall().GetRunId()
	result.ToolCallId = req.GetToolCall().GetToolCallId()
	result.ToolName = req.GetToolCall().GetToolName()
	result.EffectState = string(recorded.State)
	result.EvidenceRefs = append([]string(nil), recorded.EvidenceRefs...)
	s.log.Info("tool execution routed", slog.String("tool_name", req.GetToolCall().GetToolName()), slog.String("tool_call_id", req.GetToolCall().GetToolCallId()), slog.String("runtime_target", contract.GetRuntimeTarget()))
	return &toolbrokerv1.ExecuteToolCallResponse{Result: result}, nil
}

func reportableReadFailure(result *toolbrokerv1.ToolResult, contract *toolbrokerv1.ToolContract, binding actionauth.ActionBinding) bool {
	if contract.GetToolClass() != "http" && contract.GetToolClass() != "webfetch" {
		return false
	}
	failure := result.GetError()
	return result.GetStatus() == "failed" && failure != nil && failure.GetErrorClass() == commonv1.ErrorClass_ERROR_CLASS_VALIDATION_ERROR &&
		len(failure.GetMessage()) <= 512 && len(failure.GetDetailsJson()) <= 1024 &&
		(result.GetResultJson() == "" || result.GetResultJson() == "{}") &&
		result.GetRunId() == binding.RunID && result.GetToolCallId() == binding.ToolCallID && result.GetToolName() == binding.ToolName
}

func unknownResult(call *toolbrokerv1.ToolCall, actionID string) *toolbrokerv1.ToolResult {
	result := authorizationErrorResult(call, "external effect outcome is unknown; verify before any new attempt")
	result.Status = "effect_unknown"
	result.ActionId = actionID
	result.EffectState = string(actionauth.StateUnknown)
	result.Error.DetailsJson = fmt.Sprintf(`{"action_id":%q,"dispatch_outcome":"unknown"}`, actionID)
	return result
}

func (s *Server) resolveAuthorizedCredentials(ctx context.Context, call *toolbrokerv1.ToolCall, grant actionauth.Grant, contract *toolbrokerv1.ToolContract) ([]credentials.ResolvedSecret, error) {
	if len(call.GetCredentialRefs()) == 0 {
		return nil, nil
	}
	resolver, ok := s.creds.(interface {
		ResolveAuthorizedToolCall(context.Context, *toolbrokerv1.ToolCall, credentials.ActionProof) ([]credentials.ResolvedSecret, error)
	})
	if !ok {
		return nil, fmt.Errorf("credential action authorization is not configured")
	}
	return resolver.ResolveAuthorizedToolCall(ctx, call, credentials.ActionProof{RunID: call.GetRunId(), ToolCallID: call.GetToolCallId(), ToolName: call.GetToolName(), ArgsJSON: call.GetArgsJson(), MetadataHash: grant.Binding.CredentialMetadataHash, Approved: grant.ApprovalID != "", Revalidate: func(checkCtx context.Context) error {
		_, err := s.authorizer.Verify(checkCtx, call, contract, grant.ID)
		return err
	}})
}

func authorizationErrorResult(call *toolbrokerv1.ToolCall, message string) *toolbrokerv1.ToolResult {
	return &toolbrokerv1.ToolResult{ToolCallId: call.GetToolCallId(), RunId: call.GetRunId(), ToolName: call.GetToolName(), Status: "failed", Error: &toolbrokerv1.ToolError{ErrorClass: commonv1.ErrorClass_ERROR_CLASS_VALIDATION_ERROR, Message: message, Retryable: false}}
}

func (s *Server) ListTools(ctx context.Context, req *toolbrokerv1.ListToolsRequest) (*toolbrokerv1.ListToolsResponse, error) {
	if req.GetRunId() != "" {
		if s.scopedCatalog == nil {
			return nil, status.Error(codes.FailedPrecondition, "current task catalog is unavailable")
		}
		tools, err := s.scopedCatalog.ListForRun(ctx, req.GetRunId(), req.GetQuery(), int(req.GetLimit()))
		if err != nil {
			return nil, status.Error(codes.PermissionDenied, "current task catalog is unavailable")
		}
		filtered := tools[:0]
		for _, tool := range tools {
			if req.GetToolClass() == "" || req.GetToolClass() == tool.GetToolClass() {
				filtered = append(filtered, tool)
			}
		}
		return &toolbrokerv1.ListToolsResponse{Tools: filtered}, nil
	}
	tools := s.registry.List(req.GetToolClass(), req.GetIncludeDisabled())
	return &toolbrokerv1.ListToolsResponse{Tools: tools}, nil
}

func (s *Server) GetToolContract(_ context.Context, req *toolbrokerv1.GetToolContractRequest) (*toolbrokerv1.GetToolContractResponse, error) {
	contract, ok := s.registry.Get(req.GetToolName())
	if !ok {
		return nil, status.Error(codes.NotFound, "tool contract not found")
	}
	return &toolbrokerv1.GetToolContractResponse{Contract: contract}, nil
}

func invalidResult(call *toolbrokerv1.ToolCall, toolErr *toolbrokerv1.ToolError) *toolbrokerv1.ToolResult {
	return &toolbrokerv1.ToolResult{
		ToolCallId: call.GetToolCallId(),
		RunId:      call.GetRunId(),
		ToolName:   call.GetToolName(),
		Status:     "failed",
		Error:      toolErr,
	}
}

func credentialErrorResult(call *toolbrokerv1.ToolCall, err error) *toolbrokerv1.ToolResult {
	return &toolbrokerv1.ToolResult{
		ToolCallId: call.GetToolCallId(),
		RunId:      call.GetRunId(),
		ToolName:   call.GetToolName(),
		Status:     "failed",
		Error: &toolbrokerv1.ToolError{
			ErrorClass: commonv1.ErrorClass_ERROR_CLASS_CREDENTIAL_ERROR,
			Message:    "credential action authorization or resolution denied",
			Retryable:  false,
		},
	}
}
