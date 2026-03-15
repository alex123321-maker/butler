package runtime

import (
	"context"
	"encoding/json"
	"log/slog"
	"time"

	commonv1 "github.com/butler/butler/internal/gen/common/v1"
	runtimev1 "github.com/butler/butler/internal/gen/runtime/v1"
	toolbrokerv1 "github.com/butler/butler/internal/gen/toolbroker/v1"
	"github.com/butler/butler/internal/logger"
)

type Server struct {
	runtimev1.UnimplementedToolRuntimeServiceServer

	inspector SystemInspector
	log       *slog.Logger
}

func NewServer(inspector SystemInspector, log *slog.Logger) *Server {
	if log == nil {
		log = slog.Default()
	}
	return &Server{inspector: inspector, log: logger.WithComponent(log, "tool-doctor-runtime")}
}

func (s *Server) Execute(ctx context.Context, req *runtimev1.ExecuteRequest) (*runtimev1.ExecuteResponse, error) {
	call := req.GetToolCall()
	if call == nil {
		return &runtimev1.ExecuteResponse{Result: failedResult(req, commonv1.ErrorClass_ERROR_CLASS_VALIDATION_ERROR, "tool_call is required", "{}")}, nil
	}
	if call.GetToolName() != "doctor.check_system" && call.GetToolName() != "doctor.check_database" && call.GetToolName() != "doctor.check_container" && call.GetToolName() != "doctor.check_provider" {
		return &runtimev1.ExecuteResponse{Result: failedResult(req, commonv1.ErrorClass_ERROR_CLASS_VALIDATION_ERROR, "unsupported tool for doctor runtime", mustJSON(map[string]any{"tool_name": call.GetToolName()}))}, nil
	}
	if s.inspector == nil {
		return &runtimev1.ExecuteResponse{Result: failedResult(req, commonv1.ErrorClass_ERROR_CLASS_INTERNAL_ERROR, "doctor inspector is not configured", "{}")}, nil
	}
	var report SystemReport
	switch call.GetToolName() {
	case "doctor.check_system":
		report = s.inspector.CheckSystem(ctx)
	case "doctor.check_database":
		report = s.inspector.CheckDatabase(ctx)
	case "doctor.check_container":
		report = s.inspector.CheckContainer(ctx)
	case "doctor.check_provider":
		report = s.inspector.CheckProvider(ctx)
	}
	resultJSON := mustJSON(report)
	s.log.Info("doctor check executed", slog.String("tool_call_id", call.GetToolCallId()), slog.String("tool_name", call.GetToolName()), slog.String("status", string(report.Status)))
	return &runtimev1.ExecuteResponse{Result: &toolbrokerv1.ToolResult{ToolCallId: call.GetToolCallId(), RunId: call.GetRunId(), ToolName: call.GetToolName(), Status: "completed", ResultJson: resultJSON, FinishedAt: time.Now().UTC().Format(time.RFC3339Nano)}}, nil
}

func failedResult(req *runtimev1.ExecuteRequest, class commonv1.ErrorClass, message, detailsJSON string) *toolbrokerv1.ToolResult {
	call := req.GetToolCall()
	result := &toolbrokerv1.ToolResult{Status: "failed", FinishedAt: time.Now().UTC().Format(time.RFC3339Nano), Error: &toolbrokerv1.ToolError{ErrorClass: class, Message: message, Retryable: false, DetailsJson: detailsJSON}}
	if call != nil {
		result.ToolCallId = call.GetToolCallId()
		result.RunId = call.GetRunId()
		result.ToolName = call.GetToolName()
	}
	return result
}

func mustJSON(value any) string {
	encoded, err := json.Marshal(value)
	if err != nil {
		return "{}"
	}
	return string(encoded)
}
