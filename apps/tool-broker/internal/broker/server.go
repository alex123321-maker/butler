package broker

import (
	"context"
	"fmt"
	"log/slog"

	"github.com/butler/butler/apps/tool-broker/internal/registry"
	toolbrokerv1 "github.com/butler/butler/internal/gen/toolbroker/v1"
	"github.com/butler/butler/internal/logger"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

type Server struct {
	toolbrokerv1.UnimplementedToolBrokerServiceServer

	registry *registry.Registry
	executor RuntimeExecutor
	log      *slog.Logger
}

type RuntimeExecutor interface {
	Execute(context.Context, *toolbrokerv1.ToolCall, *toolbrokerv1.ToolContract) (*toolbrokerv1.ToolResult, error)
}

func NewServer(registry *registry.Registry, executor RuntimeExecutor, log *slog.Logger) *Server {
	if log == nil {
		log = slog.Default()
	}
	return &Server{registry: registry, executor: executor, log: logger.WithComponent(log, "tool-broker")}
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
	result, err := s.executor.Execute(ctx, req.GetToolCall(), contract)
	if err != nil {
		s.log.Error("tool execution failed", slog.String("tool_name", req.GetToolCall().GetToolName()), slog.String("tool_call_id", req.GetToolCall().GetToolCallId()), slog.String("error", err.Error()))
		return &toolbrokerv1.ExecuteToolCallResponse{Result: runtimeErrorResult(req.GetToolCall(), contract, err)}, nil
	}
	s.log.Info("tool execution routed", slog.String("tool_name", req.GetToolCall().GetToolName()), slog.String("tool_call_id", req.GetToolCall().GetToolCallId()), slog.String("runtime_target", contract.GetRuntimeTarget()))
	return &toolbrokerv1.ExecuteToolCallResponse{Result: result}, nil
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
			ErrorClass: 1,
			Message:    fmt.Sprintf("runtime execution failed: %v", err),
			Retryable:  false,
			DetailsJson: fmt.Sprintf(`{"runtime_target":%q}`,
				runtimeTarget,
			),
		},
	}
}
