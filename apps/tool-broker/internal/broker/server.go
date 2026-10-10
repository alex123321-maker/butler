package broker

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"time"

	"github.com/butler/butler/apps/tool-broker/internal/registry"
	"github.com/butler/butler/internal/actionauth"
	"github.com/butler/butler/internal/credentials"
	commonv1 "github.com/butler/butler/internal/gen/common/v1"
	toolbrokerv1 "github.com/butler/butler/internal/gen/toolbroker/v1"
	"github.com/butler/butler/internal/logger"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

type Server struct {
	toolbrokerv1.UnimplementedToolBrokerServiceServer

	registry   *registry.Registry
	executor   RuntimeExecutor
	creds      CredentialResolver
	authorizer *actionauth.Service
	log        *slog.Logger
}

func (s *Server) SetActionAuthorizer(authorizer *actionauth.Service) { s.authorizer = authorizer }

func (s *Server) PrepareToolCall(ctx context.Context, req *toolbrokerv1.PrepareToolCallRequest) (*toolbrokerv1.PrepareToolCallResponse, error) {
	valid, contract, toolErr := s.registry.Validate(req.GetToolCall())
	if !valid {
		return &toolbrokerv1.PrepareToolCallResponse{Error: toolErr}, nil
	}
	if s.authorizer == nil {
		return nil, status.Error(codes.FailedPrecondition, "action authorization is not configured")
	}
	prepared, err := s.authorizer.Prepare(ctx, req.GetToolCall(), contract)
	if err != nil {
		return nil, status.Error(codes.PermissionDenied, err.Error())
	}
	encoded, _ := json.Marshal(prepared.Binding)
	return &toolbrokerv1.PrepareToolCallResponse{Action: &toolbrokerv1.PreparedAction{ActionId: prepared.Binding.ActionID, ActionHash: prepared.Binding.ActionHash, BindingJson: string(encoded), RequiresApproval: prepared.RequiresApproval, Summary: prepared.Summary, ExpiresAt: prepared.ExpiresAt.Format(time.RFC3339Nano)}}, nil
}
func (s *Server) AuthorizeToolCall(ctx context.Context, req *toolbrokerv1.AuthorizeToolCallRequest) (*toolbrokerv1.AuthorizeToolCallResponse, error) {
	valid, contract, toolErr := s.registry.Validate(req.GetToolCall())
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

func (s *Server) ValidateToolCall(_ context.Context, req *toolbrokerv1.ValidateToolCallRequest) (*toolbrokerv1.ValidateToolCallResponse, error) {
	valid, contract, toolErr := s.registry.Validate(req.GetToolCall())
	return &toolbrokerv1.ValidateToolCallResponse{Valid: valid, Contract: contract, Error: toolErr}, nil
}

func (s *Server) ExecuteToolCall(ctx context.Context, req *toolbrokerv1.ExecuteToolCallRequest) (*toolbrokerv1.ExecuteToolCallResponse, error) {
	if s.executor == nil {
		return nil, status.Error(codes.FailedPrecondition, "tool execution routing is not configured")
	}
	valid, contract, toolErr := s.registry.Validate(req.GetToolCall())
	if !valid {
		return &toolbrokerv1.ExecuteToolCallResponse{Result: invalidResult(req.GetToolCall(), toolErr)}, nil
	}
	if s.authorizer == nil {
		return &toolbrokerv1.ExecuteToolCallResponse{Result: authorizationErrorResult(req.GetToolCall(), "action authorization is not configured")}, nil
	}
	grant, err := s.authorizer.Verify(ctx, req.GetToolCall(), contract, req.GetAuthorizationId())
	if err != nil {
		return &toolbrokerv1.ExecuteToolCallResponse{Result: authorizationErrorResult(req.GetToolCall(), err.Error())}, nil
	}
	resolved, err := s.resolveAuthorizedCredentials(ctx, req.GetToolCall(), grant, contract)
	if err != nil {
		s.log.Error("credential resolution denied", slog.String("tool_name", req.GetToolCall().GetToolName()), slog.String("tool_call_id", req.GetToolCall().GetToolCallId()))
		return &toolbrokerv1.ExecuteToolCallResponse{Result: credentialErrorResult(req.GetToolCall(), err)}, nil
	}
	if _, err = s.authorizer.Verify(ctx, req.GetToolCall(), contract, grant.ID); err != nil {
		return &toolbrokerv1.ExecuteToolCallResponse{Result: authorizationErrorResult(req.GetToolCall(), err.Error())}, nil
	}
	if err = s.authorizer.Claim(ctx, grant.ID); err != nil {
		return &toolbrokerv1.ExecuteToolCallResponse{Result: authorizationErrorResult(req.GetToolCall(), "authorization already dispatched, expired or revoked; do not replay")}, nil
	}
	result, err := s.executor.Execute(ctx, req.GetToolCall(), contract, resolved)
	result = redactResult(result, resolved)
	// Dispatch completion is not evidence of the external effect. A runtime
	// failure or timeout retains an unknown outcome and never permits replay.
	outcome := "dispatch_completed"
	if err != nil || result == nil || result.GetStatus() != "completed" {
		outcome = "unknown"
	}
	finishCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 3*time.Second)
	defer cancel()
	if finishErr := s.authorizer.Finish(finishCtx, grant.ID, outcome); finishErr != nil {
		s.log.Error("action dispatch outcome recording failed", slog.String("authorization_id", grant.ID))
		return &toolbrokerv1.ExecuteToolCallResponse{Result: unknownResult(req.GetToolCall(), grant.Binding.ActionID)}, nil
	}
	if err != nil {
		s.log.Error("tool execution outcome unknown", slog.String("tool_name", req.GetToolCall().GetToolName()), slog.String("tool_call_id", req.GetToolCall().GetToolCallId()))
		return &toolbrokerv1.ExecuteToolCallResponse{Result: unknownResult(req.GetToolCall(), grant.Binding.ActionID)}, nil
	}
	if outcome == "unknown" {
		if result == nil {
			result = unknownResult(req.GetToolCall(), grant.Binding.ActionID)
		} else {
			result.Status = "effect_unknown"
			result.Error = &toolbrokerv1.ToolError{ErrorClass: commonv1.ErrorClass_ERROR_CLASS_INTERNAL_ERROR, Message: "external effect outcome is unknown; verify before any new attempt", Retryable: false, DetailsJson: fmt.Sprintf(`{"action_id":%q,"dispatch_outcome":"unknown"}`, grant.Binding.ActionID)}
		}
	}
	s.log.Info("tool execution routed", slog.String("tool_name", req.GetToolCall().GetToolName()), slog.String("tool_call_id", req.GetToolCall().GetToolCallId()), slog.String("runtime_target", contract.GetRuntimeTarget()))
	return &toolbrokerv1.ExecuteToolCallResponse{Result: result}, nil
}

func unknownResult(call *toolbrokerv1.ToolCall, actionID string) *toolbrokerv1.ToolResult {
	result := authorizationErrorResult(call, "external effect outcome is unknown; verify before any new attempt")
	result.Status = "effect_unknown"
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

func (s *Server) resolveCredentials(ctx context.Context, call *toolbrokerv1.ToolCall) ([]credentials.ResolvedSecret, error) {
	if call == nil || len(call.GetCredentialRefs()) == 0 {
		return nil, nil
	}
	if s.creds == nil {
		return nil, fmt.Errorf("credential resolution is not configured")
	}
	return s.creds.ResolveToolCall(ctx, call)
}

func (s *Server) ListTools(_ context.Context, req *toolbrokerv1.ListToolsRequest) (*toolbrokerv1.ListToolsResponse, error) {
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

func runtimeErrorResult(call *toolbrokerv1.ToolCall, contract *toolbrokerv1.ToolContract, err error) *toolbrokerv1.ToolResult {
	runtimeTarget := ""
	if contract != nil {
		runtimeTarget = contract.GetRuntimeTarget()
	}
	return &toolbrokerv1.ToolResult{
		ToolCallId: call.GetToolCallId(),
		RunId:      call.GetRunId(),
		ToolName:   call.GetToolName(),
		Status:     "failed",
		Error: &toolbrokerv1.ToolError{
			ErrorClass: commonv1.ErrorClass_ERROR_CLASS_INTERNAL_ERROR,
			Message:    fmt.Sprintf("runtime execution failed: %v", err),
			Retryable:  false,
			DetailsJson: fmt.Sprintf(`{"runtime_target":%q}`,
				runtimeTarget,
			),
		},
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
