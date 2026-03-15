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
	"golang.org/x/net/html"
)

const (
	maxBodyBytes        int64 = 64 * 1024
	defaultMaxDownload  int64 = 10 * 1024 * 1024 // 10 MB
	maxAllowedDownload  int64 = 50 * 1024 * 1024 // 50 MB
	maxHTMLParseContent int64 = 2 * 1024 * 1024  // 2 MB
)

// supportedTools lists all HTTP tools the runtime can execute.
var supportedHTTPTools = map[string]bool{
	"http.request":    true,
	"http.download":   true,
	"http.parse_html": true,
}

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
	Auth    *authArgs         `json:"auth"`
}

type downloadArgs struct {
	URL     string `json:"url"`
	MaxSize int64  `json:"max_size,omitempty"`
}

type parseHTMLArgs struct {
	URL      string `json:"url,omitempty"`
	Content  string `json:"content,omitempty"`
	Selector string `json:"selector"`
}

type authArgs struct {
	Type   string `json:"type"`
	Alias  string `json:"alias"`
	Field  string `json:"field"`
	Header string `json:"header"`
	Scheme string `json:"scheme"`
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

	toolName := call.GetToolName()
	if !supportedHTTPTools[toolName] {
		return &runtimev1.ExecuteResponse{Result: failedResult(req, commonv1.ErrorClass_ERROR_CLASS_VALIDATION_ERROR, "unsupported tool for http runtime", mustJSON(map[string]any{"tool_name": toolName}))}, nil
	}

	switch toolName {
	case "http.request":
		return s.executeRequest(ctx, req)
	case "http.download":
		return s.executeDownload(ctx, req)
	case "http.parse_html":
		return s.executeParseHTML(ctx, req)
	default:
		return &runtimev1.ExecuteResponse{Result: failedResult(req, commonv1.ErrorClass_ERROR_CLASS_VALIDATION_ERROR, "unsupported tool for http runtime", mustJSON(map[string]any{"tool_name": toolName}))}, nil
	}
}

// ---------------------------------------------------------------------------
// http.request
// ---------------------------------------------------------------------------

func (s *Server) executeRequest(ctx context.Context, req *runtimev1.ExecuteRequest) (*runtimev1.ExecuteResponse, error) {
	call := req.GetToolCall()
	contract := req.GetContract()

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
	if err := applyResolvedAuth(httpReq, args.Auth, req.GetResolvedCredentials()); err != nil {
		return &runtimev1.ExecuteResponse{Result: failedResult(req, commonv1.ErrorClass_ERROR_CLASS_CREDENTIAL_ERROR, err.Error(), mustJSON(map[string]any{"tool_name": call.GetToolName()}))}, nil
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

// ---------------------------------------------------------------------------
// http.download
// ---------------------------------------------------------------------------

func (s *Server) executeDownload(ctx context.Context, req *runtimev1.ExecuteRequest) (*runtimev1.ExecuteResponse, error) {
	call := req.GetToolCall()
	contract := req.GetContract()

	var args downloadArgs
	if err := json.Unmarshal([]byte(call.GetArgsJson()), &args); err != nil {
		return &runtimev1.ExecuteResponse{Result: failedResult(req, commonv1.ErrorClass_ERROR_CLASS_VALIDATION_ERROR, "args_json must decode into download args", "{}")}, nil
	}
	parsedURL, err := url.Parse(strings.TrimSpace(args.URL))
	if err != nil || parsedURL.Scheme == "" || parsedURL.Host == "" {
		return &runtimev1.ExecuteResponse{Result: failedResult(req, commonv1.ErrorClass_ERROR_CLASS_VALIDATION_ERROR, "url is invalid", "{}")}, nil
	}
	if !domainAllowed(parsedURL.Hostname(), contract.GetAllowedDomains()) {
		return &runtimev1.ExecuteResponse{Result: failedResult(req, commonv1.ErrorClass_ERROR_CLASS_POLICY_DENIED, "target domain is not allowed", mustJSON(map[string]any{"domain": parsedURL.Hostname()}))}, nil
	}

	maxSize := args.MaxSize
	if maxSize <= 0 {
		maxSize = defaultMaxDownload
	}
	if maxSize > maxAllowedDownload {
		maxSize = maxAllowedDownload
	}

	requestCtx := ctx
	if timeoutMS := req.GetContext().GetTimeoutMs(); timeoutMS > 0 {
		var cancel context.CancelFunc
		requestCtx, cancel = context.WithTimeout(ctx, time.Duration(timeoutMS)*time.Millisecond)
		defer cancel()
	}
	httpReq, err := http.NewRequestWithContext(requestCtx, http.MethodGet, parsedURL.String(), nil)
	if err != nil {
		return &runtimev1.ExecuteResponse{Result: failedResult(req, commonv1.ErrorClass_ERROR_CLASS_INTERNAL_ERROR, fmt.Sprintf("build request: %v", err), "{}")}, nil
	}

	resp, err := s.httpClient.Do(httpReq)
	if err != nil {
		return &runtimev1.ExecuteResponse{Result: failedResult(req, commonv1.ErrorClass_ERROR_CLASS_INTERNAL_ERROR, fmt.Sprintf("download request failed: %v", err), mustJSON(map[string]any{"domain": parsedURL.Hostname()}))}, nil
	}
	defer resp.Body.Close()

	body, truncated, err := readBody(resp.Body, maxSize)
	if err != nil {
		return &runtimev1.ExecuteResponse{Result: failedResult(req, commonv1.ErrorClass_ERROR_CLASS_INTERNAL_ERROR, fmt.Sprintf("read download body: %v", err), "{}")}, nil
	}

	contentType := resp.Header.Get("Content-Type")
	resultJSON := mustJSON(map[string]any{
		"status_code":  resp.StatusCode,
		"content_type": contentType,
		"size":         len(body),
		"truncated":    truncated,
		"body":         body,
	})
	s.log.Info("http download executed", slog.String("tool_call_id", call.GetToolCallId()), slog.String("url", parsedURL.String()), slog.Int("status_code", resp.StatusCode), slog.Int("size", len(body)))
	return &runtimev1.ExecuteResponse{Result: &toolbrokerv1.ToolResult{ToolCallId: call.GetToolCallId(), RunId: call.GetRunId(), ToolName: call.GetToolName(), Status: "completed", ResultJson: resultJSON, FinishedAt: time.Now().UTC().Format(time.RFC3339Nano)}}, nil
}

// ---------------------------------------------------------------------------
// http.parse_html
// ---------------------------------------------------------------------------

func (s *Server) executeParseHTML(ctx context.Context, req *runtimev1.ExecuteRequest) (*runtimev1.ExecuteResponse, error) {
	call := req.GetToolCall()
	contract := req.GetContract()

	var args parseHTMLArgs
	if err := json.Unmarshal([]byte(call.GetArgsJson()), &args); err != nil {
		return &runtimev1.ExecuteResponse{Result: failedResult(req, commonv1.ErrorClass_ERROR_CLASS_VALIDATION_ERROR, "args_json must decode into parse_html args", "{}")}, nil
	}
	if strings.TrimSpace(args.Selector) == "" {
		return &runtimev1.ExecuteResponse{Result: failedResult(req, commonv1.ErrorClass_ERROR_CLASS_VALIDATION_ERROR, "selector is required", "{}")}, nil
	}

	var htmlContent string

	if strings.TrimSpace(args.Content) != "" {
		// Content provided inline.
		htmlContent = args.Content
	} else if strings.TrimSpace(args.URL) != "" {
		// Fetch HTML from URL.
		parsedURL, err := url.Parse(strings.TrimSpace(args.URL))
		if err != nil || parsedURL.Scheme == "" || parsedURL.Host == "" {
			return &runtimev1.ExecuteResponse{Result: failedResult(req, commonv1.ErrorClass_ERROR_CLASS_VALIDATION_ERROR, "url is invalid", "{}")}, nil
		}
		if !domainAllowed(parsedURL.Hostname(), contract.GetAllowedDomains()) {
			return &runtimev1.ExecuteResponse{Result: failedResult(req, commonv1.ErrorClass_ERROR_CLASS_POLICY_DENIED, "target domain is not allowed", mustJSON(map[string]any{"domain": parsedURL.Hostname()}))}, nil
		}
		requestCtx := ctx
		if timeoutMS := req.GetContext().GetTimeoutMs(); timeoutMS > 0 {
			var cancel context.CancelFunc
			requestCtx, cancel = context.WithTimeout(ctx, time.Duration(timeoutMS)*time.Millisecond)
			defer cancel()
		}
		httpReq, err := http.NewRequestWithContext(requestCtx, http.MethodGet, parsedURL.String(), nil)
		if err != nil {
			return &runtimev1.ExecuteResponse{Result: failedResult(req, commonv1.ErrorClass_ERROR_CLASS_INTERNAL_ERROR, fmt.Sprintf("build request: %v", err), "{}")}, nil
		}
		resp, err := s.httpClient.Do(httpReq)
		if err != nil {
			return &runtimev1.ExecuteResponse{Result: failedResult(req, commonv1.ErrorClass_ERROR_CLASS_INTERNAL_ERROR, fmt.Sprintf("fetch html: %v", err), mustJSON(map[string]any{"domain": parsedURL.Hostname()}))}, nil
		}
		defer resp.Body.Close()
		body, _, err := readBody(resp.Body, maxHTMLParseContent)
		if err != nil {
			return &runtimev1.ExecuteResponse{Result: failedResult(req, commonv1.ErrorClass_ERROR_CLASS_INTERNAL_ERROR, fmt.Sprintf("read html body: %v", err), "{}")}, nil
		}
		htmlContent = body
	} else {
		return &runtimev1.ExecuteResponse{Result: failedResult(req, commonv1.ErrorClass_ERROR_CLASS_VALIDATION_ERROR, "url or content is required", "{}")}, nil
	}

	matches := extractTextBySelector(htmlContent, args.Selector)
	resultJSON := mustJSON(map[string]any{
		"selector": args.Selector,
		"matches":  matches,
		"count":    len(matches),
	})
	s.log.Info("http parse_html executed", slog.String("tool_call_id", call.GetToolCallId()), slog.String("selector", args.Selector), slog.Int("matches", len(matches)))
	return &runtimev1.ExecuteResponse{Result: &toolbrokerv1.ToolResult{ToolCallId: call.GetToolCallId(), RunId: call.GetRunId(), ToolName: call.GetToolName(), Status: "completed", ResultJson: resultJSON, FinishedAt: time.Now().UTC().Format(time.RFC3339Nano)}}, nil
}

// ---------------------------------------------------------------------------
// HTML parsing helpers
// ---------------------------------------------------------------------------

// extractTextBySelector finds elements matching a simple tag or tag.class or
// tag#id selector and returns their text content. This is intentionally simple —
// no full CSS selector engine dependency.
func extractTextBySelector(htmlContent, selector string) []string {
	tag, id, class := parseSimpleSelector(selector)
	if tag == "" && id == "" && class == "" {
		return nil
	}

	doc, err := html.Parse(strings.NewReader(htmlContent))
	if err != nil {
		return nil
	}

	var matches []string
	var walk func(*html.Node)
	walk = func(n *html.Node) {
		if n.Type == html.ElementNode && matchNode(n, tag, id, class) {
			text := collectText(n)
			if text != "" {
				matches = append(matches, text)
			}
		}
		for c := n.FirstChild; c != nil; c = c.NextSibling {
			walk(c)
		}
	}
	walk(doc)
	return matches
}

// parseSimpleSelector supports: "tag", ".class", "#id", "tag.class", "tag#id".
func parseSimpleSelector(sel string) (tag, id, class string) {
	sel = strings.TrimSpace(sel)
	if sel == "" {
		return
	}
	if idx := strings.Index(sel, "#"); idx >= 0 {
		tag = sel[:idx]
		id = sel[idx+1:]
		return
	}
	if idx := strings.Index(sel, "."); idx >= 0 {
		tag = sel[:idx]
		class = sel[idx+1:]
		return
	}
	tag = sel
	return
}

// matchNode checks whether an HTML element matches the tag/id/class criteria.
func matchNode(n *html.Node, tag, id, class string) bool {
	if tag != "" && n.Data != tag {
		return false
	}
	if id != "" {
		found := false
		for _, a := range n.Attr {
			if a.Key == "id" && a.Val == id {
				found = true
				break
			}
		}
		if !found {
			return false
		}
	}
	if class != "" {
		found := false
		for _, a := range n.Attr {
			if a.Key == "class" {
				for _, c := range strings.Fields(a.Val) {
					if c == class {
						found = true
						break
					}
				}
				break
			}
		}
		if !found {
			return false
		}
	}
	return true
}

// collectText concatenates all text nodes inside an element.
func collectText(n *html.Node) string {
	var sb strings.Builder
	var walk func(*html.Node)
	walk = func(node *html.Node) {
		if node.Type == html.TextNode {
			sb.WriteString(node.Data)
		}
		for c := node.FirstChild; c != nil; c = c.NextSibling {
			walk(c)
		}
	}
	walk(n)
	return strings.TrimSpace(sb.String())
}

// ---------------------------------------------------------------------------
// Shared helpers
// ---------------------------------------------------------------------------

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

func flattenHeaders(headers http.Header) map[string]string {
	result := make(map[string]string, len(headers))
	for key, values := range headers {
		result[key] = strings.Join(values, ", ")
	}
	return result
}

func applyResolvedAuth(req *http.Request, auth *authArgs, resolved []*runtimev1.ResolvedCredential) error {
	if auth == nil {
		return nil
	}
	if !strings.EqualFold(strings.TrimSpace(auth.Type), "credential_ref") {
		return fmt.Errorf("unsupported auth type %q", auth.Type)
	}
	match := findResolvedCredential(resolved, auth.Alias, auth.Field)
	if match == nil {
		return fmt.Errorf("resolved credential %s/%s was not provided", strings.TrimSpace(auth.Alias), strings.TrimSpace(auth.Field))
	}
	header := strings.TrimSpace(auth.Header)
	if header == "" {
		header = "Authorization"
	}
	scheme := strings.TrimSpace(auth.Scheme)
	value := match.GetValue()
	if strings.EqualFold(header, "Authorization") {
		if scheme == "" && strings.EqualFold(strings.TrimSpace(auth.Field), "token") {
			scheme = "Bearer"
		}
		if scheme != "" {
			value = scheme + " " + value
		}
	}
	req.Header.Set(header, value)
	return nil
}

func findResolvedCredential(resolved []*runtimev1.ResolvedCredential, alias, field string) *runtimev1.ResolvedCredential {
	alias = strings.TrimSpace(alias)
	field = strings.TrimSpace(field)
	for _, item := range resolved {
		if item == nil {
			continue
		}
		if strings.EqualFold(strings.TrimSpace(item.GetAlias()), alias) && strings.EqualFold(strings.TrimSpace(item.GetField()), field) {
			return item
		}
	}
	return nil
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
