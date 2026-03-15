package runtime

import (
	"context"
	"encoding/json"
	"fmt"
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

// supportedTools lists all browser tools the runtime can execute.
var supportedTools = map[string]bool{
	"browser.navigate":              true,
	"browser.snapshot":              true,
	"browser.click":                 true,
	"browser.fill":                  true,
	"browser.type":                  true,
	"browser.wait_for":              true,
	"browser.extract_text":          true,
	"browser.set_cookie":            true,
	"browser.restore_storage_state": true,
}

// toolsRequiringURL lists tools that require a valid URL.
var toolsRequiringURL = map[string]bool{
	"browser.navigate":   true,
	"browser.snapshot":   true,
	"browser.set_cookie": true,
}

// toolsSupportingCredentialRef lists tools whose value/text field
// may contain a credential_ref that is resolved by the broker.
var toolsSupportingCredentialRef = map[string]bool{
	"browser.fill": true,
	"browser.type": true,
}

type Runner interface {
	Run(context.Context, Request) (Result, error)
}

type Server struct {
	runtimev1.UnimplementedToolRuntimeServiceServer

	runner Runner
	log    *slog.Logger
}

// Request represents a browser tool invocation passed to the Playwright runner.
type Request struct {
	ToolName     string        `json:"tool_name"`
	URL          string        `json:"url,omitempty"`
	Selector     string        `json:"selector,omitempty"`
	Value        string        `json:"value,omitempty"`
	Text         string        `json:"text,omitempty"`
	Timeout      int64         `json:"timeout,omitempty"`
	Cookie       *Cookie       `json:"cookie,omitempty"`
	StorageState *StorageState `json:"storage_state,omitempty"`
}

// Cookie represents a browser cookie for browser.set_cookie.
type Cookie struct {
	Name     string `json:"name"`
	Value    string `json:"value"`
	Domain   string `json:"domain"`
	Path     string `json:"path,omitempty"`
	Secure   bool   `json:"secure,omitempty"`
	HTTPOnly bool   `json:"http_only,omitempty"`
}

// StorageState captures cookies and local storage for browser.restore_storage_state.
type StorageState struct {
	Cookies []Cookie        `json:"cookies,omitempty"`
	Origins []OriginStorage `json:"origins,omitempty"`
}

// OriginStorage holds local storage entries for a single origin.
type OriginStorage struct {
	Origin       string         `json:"origin"`
	LocalStorage []StorageEntry `json:"local_storage,omitempty"`
}

// StorageEntry is a single key/value local storage entry.
type StorageEntry struct {
	Name  string `json:"name"`
	Value string `json:"value"`
}

// Result represents normalized output from the Playwright runner.
type Result struct {
	FinalURL string `json:"final_url,omitempty"`
	Title    string `json:"title,omitempty"`
	Text     string `json:"text,omitempty"`
	Matched  bool   `json:"matched,omitempty"`
	OK       bool   `json:"ok,omitempty"`
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

	if !supportedTools[args.ToolName] {
		return &runtimev1.ExecuteResponse{Result: failedResult(req, commonv1.ErrorClass_ERROR_CLASS_VALIDATION_ERROR, "unsupported tool for browser runtime", mustJSON(map[string]any{"tool_name": args.ToolName}))}, nil
	}

	// Validate URL for tools that need it.
	if toolsRequiringURL[args.ToolName] {
		parsedURL, err := url.Parse(strings.TrimSpace(args.URL))
		if err != nil || parsedURL.Scheme == "" || parsedURL.Host == "" {
			return &runtimev1.ExecuteResponse{Result: failedResult(req, commonv1.ErrorClass_ERROR_CLASS_VALIDATION_ERROR, "url is invalid", "{}")}, nil
		}
		if !domainAllowed(parsedURL.Hostname(), contract.GetAllowedDomains()) {
			return &runtimev1.ExecuteResponse{Result: failedResult(req, commonv1.ErrorClass_ERROR_CLASS_POLICY_DENIED, "target domain is not allowed", mustJSON(map[string]any{"domain": parsedURL.Hostname()}))}, nil
		}
	}

	// Validate selector for tools that require it.
	if err := validateSelectorRequired(args); err != nil {
		return &runtimev1.ExecuteResponse{Result: failedResult(req, commonv1.ErrorClass_ERROR_CLASS_VALIDATION_ERROR, err.Error(), "{}")}, nil
	}

	// Resolve credential_ref values for fill/type tools.
	if toolsSupportingCredentialRef[args.ToolName] {
		resolveCredentialValue(&args, req.GetResolvedCredentials())
	}

	// Validate cookie for set_cookie.
	if args.ToolName == "browser.set_cookie" && (args.Cookie == nil || args.Cookie.Name == "" || args.Cookie.Domain == "") {
		return &runtimev1.ExecuteResponse{Result: failedResult(req, commonv1.ErrorClass_ERROR_CLASS_VALIDATION_ERROR, "cookie with name and domain is required", "{}")}, nil
	}

	// Validate storage_state for restore_storage_state.
	if args.ToolName == "browser.restore_storage_state" && args.StorageState == nil {
		return &runtimev1.ExecuteResponse{Result: failedResult(req, commonv1.ErrorClass_ERROR_CLASS_VALIDATION_ERROR, "storage_state is required", "{}")}, nil
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

	resultJSON := buildResultJSON(args.ToolName, result)
	s.log.Info("browser request executed", slog.String("tool_call_id", call.GetToolCallId()), slog.String("tool_name", call.GetToolName()))
	return &runtimev1.ExecuteResponse{Result: &toolbrokerv1.ToolResult{ToolCallId: call.GetToolCallId(), RunId: call.GetRunId(), ToolName: call.GetToolName(), Status: "completed", ResultJson: resultJSON, FinishedAt: time.Now().UTC().Format(time.RFC3339Nano)}}, nil
}

// validateSelectorRequired returns an error if the tool requires a selector but none is provided.
func validateSelectorRequired(args Request) error {
	switch args.ToolName {
	case "browser.click", "browser.fill", "browser.type", "browser.wait_for", "browser.extract_text":
		if strings.TrimSpace(args.Selector) == "" {
			return fmt.Errorf("selector is required for %s", args.ToolName)
		}
	}
	return nil
}

// resolveCredentialValue replaces the value/text field with the resolved credential
// when the model passes a credential_ref placeholder. The broker resolves the secret
// and places it in resolved_credentials; this function applies it to the request.
func resolveCredentialValue(args *Request, resolved []*runtimev1.ResolvedCredential) {
	if len(resolved) == 0 {
		return
	}
	// For browser.fill, the secret goes into Value; for browser.type, into Text.
	// Use the first resolved credential — the broker provides exactly the needed secret.
	cred := resolved[0]
	if cred == nil || cred.GetValue() == "" {
		return
	}
	switch args.ToolName {
	case "browser.fill":
		args.Value = cred.GetValue()
	case "browser.type":
		args.Text = cred.GetValue()
	}
}

// buildResultJSON produces a normalized JSON result for the given tool.
func buildResultJSON(toolName string, result Result) string {
	switch toolName {
	case "browser.navigate", "browser.snapshot":
		return mustJSON(map[string]any{"final_url": result.FinalURL, "title": result.Title, "text": result.Text})
	case "browser.click":
		return mustJSON(map[string]any{"ok": result.OK, "title": result.Title})
	case "browser.fill", "browser.type":
		return mustJSON(map[string]any{"ok": result.OK})
	case "browser.wait_for":
		return mustJSON(map[string]any{"matched": result.Matched})
	case "browser.extract_text":
		return mustJSON(map[string]any{"text": result.Text})
	case "browser.set_cookie", "browser.restore_storage_state":
		return mustJSON(map[string]any{"ok": result.OK})
	default:
		return mustJSON(map[string]any{"ok": result.OK, "text": result.Text})
	}
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
	if host == "" {
		return false
	}
	// Empty allowlist means unrestricted — operators populate the list to restrict domains.
	if len(allowed) == 0 {
		return true
	}
	for _, candidate := range allowed {
		candidate = strings.ToLower(strings.TrimSpace(candidate))
		if candidate == "" {
			continue
		}
		if candidate == "*" || host == candidate || strings.HasSuffix(host, "."+candidate) {
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
