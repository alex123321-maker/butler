package runtime

import (
	"context"
	"encoding/json"
	"log/slog"
	"net/url"
	"strings"
	"time"

	commonv1 "github.com/butler/butler/internal/gen/common/v1"
	runtimev1 "github.com/butler/butler/internal/gen/runtime/v1"
	toolbrokerv1 "github.com/butler/butler/internal/gen/toolbroker/v1"
	"github.com/butler/butler/internal/logger"
)

const maxSnapshotTextLen = 12000

type Runner interface {
	Run(context.Context, Request) (Result, error)
}

type Server struct {
	runtimev1.UnimplementedToolRuntimeServiceServer

	runner Runner
	log    *slog.Logger
}

type Request struct {
	ToolName string `json:"tool_name"`
	URL      string `json:"url"`
}

type Result struct {
	FinalURL string `json:"final_url"`
	Title    string `json:"title"`
	Text     string `json:"text,omitempty"`
}

func NewServer(runner Runner, log *slog.Logger) *Server {
	if log == nil {
		log = slog.Default()
	}
	return &Server{runner: runner, log: logger.WithComponent(log, "tool-browser-runtime")}
}

func (s *Server) Execute(ctx context.Context, req *runtimev1.ExecuteRequest) (*runtimev1.ExecuteResponse, error) {
	call := req.GetToolCall()
	contract := req.GetContract()
	if call == nil {
		return &runtimev1.ExecuteResponse{Result: failedResult(req, commonv1.ErrorClass_ERROR_CLASS_VALIDATION_ERROR, "tool_call is required", "{}")}, nil
	}
	if contract == nil {
		return &runtimev1.ExecuteResponse{Result: failedResult(req, commonv1.ErrorClass_ERROR_CLASS_VALIDATION_ERROR, "tool contract is required", "{}")}, nil
	}
	if s.runner == nil {
		return &runtimev1.ExecuteResponse{Result: failedResult(req, commonv1.ErrorClass_ERROR_CLASS_INTERNAL_ERROR, "browser runner is not configured", "{}")}, nil
	}

	var args Request
	if err := json.Unmarshal([]byte(call.GetArgsJson()), &args); err != nil {
		return &runtimev1.ExecuteResponse{Result: failedResult(req, commonv1.ErrorClass_ERROR_CLASS_VALIDATION_ERROR, "args_json must decode into browser request args", "{}")}, nil
	}
	args.ToolName = call.GetToolName()
	if args.ToolName != "browser.navigate" && args.ToolName != "browser.snapshot" {
		return &runtimev1.ExecuteResponse{Result: failedResult(req, commonv1.ErrorClass_ERROR_CLASS_VALIDATION_ERROR, "unsupported tool for browser runtime", mustJSON(map[string]any{"tool_name": args.ToolName}))}, nil
	}
	parsedURL, err := url.Parse(strings.TrimSpace(args.URL))
	if err != nil || parsedURL.Scheme == "" || parsedURL.Host == "" {
		return &runtimev1.ExecuteResponse{Result: failedResult(req, commonv1.ErrorClass_ERROR_CLASS_VALIDATION_ERROR, "url is invalid", "{}")}, nil
	}
	if !domainAllowed(parsedURL.Hostname(), contract.GetAllowedDomains()) {
		return &runtimev1.ExecuteResponse{Result: failedResult(req, commonv1.ErrorClass_ERROR_CLASS_POLICY_DENIED, "target domain is not allowed", mustJSON(map[string]any{"domain": parsedURL.Hostname()}))}, nil
	}

	runCtx := ctx
	if timeoutMS := req.GetContext().GetTimeoutMs(); timeoutMS > 0 {
		var cancel context.CancelFunc
		runCtx, cancel = context.WithTimeout(ctx, time.Duration(timeoutMS)*time.Millisecond)
		defer cancel()
	}
	result, err := s.runner.Run(runCtx, args)
	if err != nil {
		return &runtimev1.ExecuteResponse{Result: failedResult(req, commonv1.ErrorClass_ERROR_CLASS_INTERNAL_ERROR, err.Error(), mustJSON(map[string]any{"tool_name": args.ToolName}))}, nil
	}
	result.Text = truncate(result.Text, maxSnapshotTextLen)
	resultJSON := mustJSON(map[string]any{"final_url": result.FinalURL, "title": result.Title, "text": result.Text})
	s.log.Info("browser request executed", slog.String("tool_call_id", call.GetToolCallId()), slog.String("tool_name", call.GetToolName()), slog.String("url", args.URL))
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

func domainAllowed(host string, allowed []string) bool {
	host = strings.ToLower(strings.TrimSpace(host))
	if host == "" || len(allowed) == 0 {
		return false
	}
	for _, candidate := range allowed {
		candidate = strings.ToLower(strings.TrimSpace(candidate))
		if candidate == "" {
			continue
		}
		if host == candidate || strings.HasSuffix(host, "."+candidate) {
			return true
		}
	}
	return false
}

func truncate(value string, limit int) string {
	if limit <= 0 || len(value) <= limit {
		return value
	}
	return value[:limit]
}

func mustJSON(value any) string {
	encoded, err := json.Marshal(value)
	if err != nil {
		return "{}"
	}
	return string(encoded)
}
