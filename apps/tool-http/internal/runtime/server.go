package runtime

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/url"
	"strings"
	"time"

	commonv1 "github.com/butler/butler/internal/gen/common/v1"
	runtimev1 "github.com/butler/butler/internal/gen/runtime/v1"
	toolbrokerv1 "github.com/butler/butler/internal/gen/toolbroker/v1"
	"github.com/butler/butler/internal/logger"
)

const maxBodyBytes = 64 * 1024

type Server struct {
	runtimev1.UnimplementedToolRuntimeServiceServer

	httpClient *http.Client
	log        *slog.Logger
}

type requestArgs struct {
	Method  string            `json:"method"`
	URL     string            `json:"url"`
	Headers map[string]string `json:"headers"`
	Body    string            `json:"body"`
}

func NewServer(httpClient *http.Client, log *slog.Logger) *Server {
	if httpClient == nil {
		httpClient = &http.Client{Timeout: 15 * time.Second}
	}
	if log == nil {
		log = slog.Default()
	}
	return &Server{httpClient: httpClient, log: logger.WithComponent(log, "tool-http-runtime")}
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
	if call.GetToolName() != "http.request" {
		return &runtimev1.ExecuteResponse{Result: failedResult(req, commonv1.ErrorClass_ERROR_CLASS_VALIDATION_ERROR, "unsupported tool for http runtime", `{"tool_name":"`+call.GetToolName()+`"}`)}, nil
	}

	var args requestArgs
	if err := json.Unmarshal([]byte(call.GetArgsJson()), &args); err != nil {
		return &runtimev1.ExecuteResponse{Result: failedResult(req, commonv1.ErrorClass_ERROR_CLASS_VALIDATION_ERROR, "args_json must decode into http request args", "{}")}, nil
	}
	method := strings.ToUpper(strings.TrimSpace(args.Method))
	if method == "" {
		method = http.MethodGet
	}
	if method != http.MethodGet && method != http.MethodPost && method != http.MethodPut && method != http.MethodDelete && method != http.MethodPatch {
		return &runtimev1.ExecuteResponse{Result: failedResult(req, commonv1.ErrorClass_ERROR_CLASS_VALIDATION_ERROR, "http method is not allowed", `{"method":"`+method+`"}`)}, nil
	}
	parsedURL, err := url.Parse(strings.TrimSpace(args.URL))
	if err != nil || parsedURL.Scheme == "" || parsedURL.Host == "" {
		return &runtimev1.ExecuteResponse{Result: failedResult(req, commonv1.ErrorClass_ERROR_CLASS_VALIDATION_ERROR, "url is invalid", "{}")}, nil
	}
	if !domainAllowed(parsedURL.Hostname(), contract.GetAllowedDomains()) {
		return &runtimev1.ExecuteResponse{Result: failedResult(req, commonv1.ErrorClass_ERROR_CLASS_POLICY_DENIED, "target domain is not allowed", `{"domain":"`+parsedURL.Hostname()+`"}`)}, nil
	}

	requestCtx := ctx
	if timeoutMS := req.GetContext().GetTimeoutMs(); timeoutMS > 0 {
		var cancel context.CancelFunc
		requestCtx, cancel = context.WithTimeout(ctx, time.Duration(timeoutMS)*time.Millisecond)
		defer cancel()
	}
	httpReq, err := http.NewRequestWithContext(requestCtx, method, parsedURL.String(), bytes.NewBufferString(args.Body))
	if err != nil {
		return &runtimev1.ExecuteResponse{Result: failedResult(req, commonv1.ErrorClass_ERROR_CLASS_INTERNAL_ERROR, fmt.Sprintf("build request: %v", err), "{}")}, nil
	}
	for key, value := range args.Headers {
		httpReq.Header.Set(key, value)
	}

	resp, err := s.httpClient.Do(httpReq)
	if err != nil {
		return &runtimev1.ExecuteResponse{Result: failedResult(req, commonv1.ErrorClass_ERROR_CLASS_INTERNAL_ERROR, fmt.Sprintf("execute request: %v", err), mustJSON(map[string]any{"domain": parsedURL.Hostname()}))}, nil
	}
	defer resp.Body.Close()
	body, truncated, err := readBody(resp.Body, maxBodyBytes)
	if err != nil {
		return &runtimev1.ExecuteResponse{Result: failedResult(req, commonv1.ErrorClass_ERROR_CLASS_INTERNAL_ERROR, fmt.Sprintf("read response body: %v", err), "{}")}, nil
	}

	resultJSON := mustJSON(map[string]any{
		"status_code": resp.StatusCode,
		"headers":     flattenHeaders(resp.Header),
		"body":        body,
		"truncated":   truncated,
	})
	s.log.Info("http request executed", slog.String("tool_call_id", call.GetToolCallId()), slog.String("method", method), slog.String("url", parsedURL.String()), slog.Int("status_code", resp.StatusCode))
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

func flattenHeaders(headers http.Header) map[string]string {
	result := make(map[string]string, len(headers))
	for key, values := range headers {
		result[key] = strings.Join(values, ", ")
	}
	return result
}

func readBody(reader io.Reader, maxBytes int64) (string, bool, error) {
	limited := io.LimitReader(reader, maxBytes+1)
	body, err := io.ReadAll(limited)
	if err != nil {
		return "", false, err
	}
	truncated := int64(len(body)) > maxBytes
	if truncated {
		body = body[:maxBytes]
	}
	return string(body), truncated, nil
}

func mustJSON(value any) string {
	encoded, err := json.Marshal(value)
	if err != nil {
		return "{}"
	}
	return string(encoded)
}
