package broker

import (
	"context"
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
	log      *slog.Logger
}

func NewServer(registry *registry.Registry, log *slog.Logger) *Server {
	if log == nil {
		log = slog.Default()
	}
	return &Server{registry: registry, log: logger.WithComponent(log, "tool-broker")}
}

func (s *Server) ValidateToolCall(_ context.Context, req *toolbrokerv1.ValidateToolCallRequest) (*toolbrokerv1.ValidateToolCallResponse, error) {
	valid, contract, toolErr := s.registry.Validate(req.GetToolCall())
	return &toolbrokerv1.ValidateToolCallResponse{Valid: valid, Contract: contract, Error: toolErr}, nil
}

func (s *Server) ExecuteToolCall(_ context.Context, req *toolbrokerv1.ExecuteToolCallRequest) (*toolbrokerv1.ExecuteToolCallResponse, error) {
	s.log.Info("tool execution requested but not implemented", slog.String("tool_name", req.GetToolCall().GetToolName()), slog.String("tool_call_id", req.GetToolCall().GetToolCallId()))
	return nil, status.Error(codes.Unimplemented, "tool execution routing is not implemented yet")
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
