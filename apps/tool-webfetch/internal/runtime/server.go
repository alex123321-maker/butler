package runtime

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"net/url"
	"reflect"
	"strings"
	"time"

	commonv1 "github.com/butler/butler/internal/gen/common/v1"
	runtimev1 "github.com/butler/butler/internal/gen/runtime/v1"
	toolbrokerv1 "github.com/butler/butler/internal/gen/toolbroker/v1"
	"github.com/butler/butler/internal/logger"
	"github.com/butler/butler/internal/memory/sanitize"
	"github.com/butler/butler/internal/security/egress"
	"github.com/butler/butler/internal/security/redact"
	"golang.org/x/net/html"
)

var supportedWebFetchTools = map[string]bool{
	"web.search":      true,
	"web.fetch":       true,
	"web.fetch_batch": true,
	"web.extract":     true,
}

type Provider interface {
	Name() string
	Fetch(ctx context.Context, url string, includeHTML bool) (FetchResult, error)
}

type FetchResult struct {
	ObservedAt   string         `json:"observed_at"`
	Links        []DocumentLink `json:"links,omitempty"`
	Provider     string         `json:"provider"`
	RequestedURL string         `json:"requested_url"`
	FinalURL     string         `json:"final_url"`
	StatusCode   int            `json:"status_code"`
	MIMEType     string         `json:"mime_type"`
	ContentText  string         `json:"content_text"`
	ContentHTML  string         `json:"content_html,omitempty"`
	Metadata     map[string]any `json:"metadata,omitempty"`
}

type ProviderChain struct {
	providers []Provider
	policy    egress.Policy
}

func NewProviderChainWithPolicy(policy egress.Policy, providers ...Provider) *ProviderChain {
	chain := NewProviderChain(providers...)
	chain.policy = policy
	return chain
}

func NewProviderChain(providers ...Provider) *ProviderChain {
	filtered := make([]Provider, 0, len(providers))
	for _, provider := range providers {
		if provider == nil {
			continue
		}
		// Optional constructors return typed nil pointers. Converting one to
		// Provider keeps a non-nil interface and must not install it as fallback.
		value := reflect.ValueOf(provider)
		if value.Kind() == reflect.Pointer && value.IsNil() {
			continue
		}
		filtered = append(filtered, provider)
	}
	return &ProviderChain{providers: filtered}
}

func (c *ProviderChain) Fetch(ctx context.Context, rawURL string, includeHTML bool) (FetchResult, error) {
	parsed, err := c.policy.ValidateURL(rawURL)
	if err != nil {
		return FetchResult{}, egress.ErrDenied
	}
	if _, err := c.policy.Resolve(ctx, parsed.Hostname()); err != nil {
		return FetchResult{}, egress.ErrDenied
	}
	var lastErr error
	for _, provider := range c.providers {
		result, err := provider.Fetch(ctx, rawURL, includeHTML)
		if err == nil {
			if _, err := textMIME(result.MIMEType, []byte(result.ContentText)); err != nil {
				return FetchResult{}, err
			}
			if result.Provider == "" {
				result.Provider = provider.Name()
			}
			result.ObservedAt = time.Now().UTC().Format(time.RFC3339)
			if result.ContentHTML != "" {
				result.Links = documentLinks(result.ContentHTML, result.FinalURL)
			}
			return sanitizeFetchResult(result), nil
		}
		if errors.Is(err, errUnsupportedContent) || errors.Is(err, errResponseLimit) {
			return FetchResult{}, err
		}
		lastErr = err
	}
	if lastErr == nil {
		lastErr = fmt.Errorf("no webfetch providers are configured")
	}
	return FetchResult{}, fmt.Errorf("webfetch providers unavailable")
}

type Server struct {
	runtimev1.UnimplementedToolRuntimeServiceServer

	searchClient *http.Client
	fetcher      *ProviderChain
	log          *slog.Logger
}

type fetchArgs struct {
	URL         string `json:"url"`
	IncludeHTML bool   `json:"include_html,omitempty"`
}

type fetchBatchArgs struct {
	URLs        []string `json:"urls"`
	IncludeHTML bool     `json:"include_html,omitempty"`
}

type extractArgs struct {
	URL         string `json:"url,omitempty"`
	Text        string `json:"text,omitempty"`
	HTML        string `json:"html,omitempty"`
	IncludeHTML bool   `json:"include_html,omitempty"`
}

func NewServer(fetcher *ProviderChain, log *slog.Logger) *Server {
	if fetcher == nil {
		fetcher = NewProviderChain()
	}
	if log == nil {
		log = slog.Default()
	}
	return &Server{searchClient: egress.NewClient(fetcher.policy, 20*time.Second), fetcher: fetcher, log: logger.WithComponent(log, "tool-webfetch-runtime")}
}

func (s *Server) Execute(ctx context.Context, req *runtimev1.ExecuteRequest) (*runtimev1.ExecuteResponse, error) {
	ctx = egress.WithCounter(ctx, egress.NewCounter(egress.MaxRequestBytes))
	call := req.GetToolCall()
	if call == nil {
		return &runtimev1.ExecuteResponse{Result: failedResult(req, commonv1.ErrorClass_ERROR_CLASS_VALIDATION_ERROR, "tool_call is required")}, nil
	}
	if !supportedWebFetchTools[call.GetToolName()] {
		return &runtimev1.ExecuteResponse{Result: failedResult(req, commonv1.ErrorClass_ERROR_CLASS_VALIDATION_ERROR, "unsupported tool for webfetch runtime")}, nil
	}

	switch call.GetToolName() {
	case "web.search":
		return s.executeSearch(ctx, req)
	case "web.fetch":
		return s.executeFetch(ctx, req)
	case "web.fetch_batch":
		return s.executeFetchBatch(ctx, req)
	case "web.extract":
		return s.executeExtract(ctx, req)
	default:
		return &runtimev1.ExecuteResponse{Result: failedResult(req, commonv1.ErrorClass_ERROR_CLASS_VALIDATION_ERROR, "unsupported tool for webfetch runtime")}, nil
	}
}

func (s *Server) executeFetch(ctx context.Context, req *runtimev1.ExecuteRequest) (*runtimev1.ExecuteResponse, error) {
	var args fetchArgs
	if err := json.Unmarshal([]byte(req.GetToolCall().GetArgsJson()), &args); err != nil {
		return &runtimev1.ExecuteResponse{Result: failedResult(req, commonv1.ErrorClass_ERROR_CLASS_VALIDATION_ERROR, "args_json must decode into web.fetch args")}, nil
	}
	if strings.TrimSpace(args.URL) == "" {
		return &runtimev1.ExecuteResponse{Result: failedResult(req, commonv1.ErrorClass_ERROR_CLASS_VALIDATION_ERROR, "url is required")}, nil
	}

	result, err := s.fetcher.Fetch(ctx, args.URL, args.IncludeHTML)
	if err != nil {
		return &runtimev1.ExecuteResponse{Result: fetchFailedResult(req, err)}, nil
	}
	return &runtimev1.ExecuteResponse{Result: completedResult(req, mustJSON(result))}, nil
}

func (s *Server) executeFetchBatch(ctx context.Context, req *runtimev1.ExecuteRequest) (*runtimev1.ExecuteResponse, error) {
	var args fetchBatchArgs
	if err := json.Unmarshal([]byte(req.GetToolCall().GetArgsJson()), &args); err != nil {
		return &runtimev1.ExecuteResponse{Result: failedResult(req, commonv1.ErrorClass_ERROR_CLASS_VALIDATION_ERROR, "args_json must decode into web.fetch_batch args")}, nil
	}
	if len(args.URLs) == 0 || len(args.URLs) > 8 {
		return &runtimev1.ExecuteResponse{Result: failedResult(req, commonv1.ErrorClass_ERROR_CLASS_VALIDATION_ERROR, "urls must contain 1 to 8 entries")}, nil
	}

	items := make([]map[string]any, 0, len(args.URLs))
	for _, itemURL := range args.URLs {
		result, err := s.fetcher.Fetch(ctx, itemURL, args.IncludeHTML)
		if err != nil {
			items = append(items, map[string]any{"error": "web resource unavailable or denied"})
			continue
		}
		items = append(items, map[string]any{
			"observed_at":   result.ObservedAt,
			"links":         result.Links,
			"provider":      result.Provider,
			"requested_url": result.RequestedURL,
			"final_url":     result.FinalURL,
			"status_code":   result.StatusCode,
			"mime_type":     result.MIMEType,
			"content_text":  result.ContentText,
			"content_html":  result.ContentHTML,
			"metadata":      result.Metadata,
		})
	}

	return &runtimev1.ExecuteResponse{Result: completedResult(req, mustJSON(map[string]any{"items": items, "count": len(items)}))}, nil
}

func (s *Server) executeExtract(ctx context.Context, req *runtimev1.ExecuteRequest) (*runtimev1.ExecuteResponse, error) {
	var args extractArgs
	if err := json.Unmarshal([]byte(req.GetToolCall().GetArgsJson()), &args); err != nil {
		return &runtimev1.ExecuteResponse{Result: failedResult(req, commonv1.ErrorClass_ERROR_CLASS_VALIDATION_ERROR, "args_json must decode into web.extract args")}, nil
	}

	text := strings.TrimSpace(args.Text)
	htmlContent := strings.TrimSpace(args.HTML)
	sourceURL := strings.TrimSpace(args.URL)
	var fetched *FetchResult
	if sourceURL != "" {
		result, err := s.fetcher.Fetch(ctx, sourceURL, true)
		if err != nil {
			return &runtimev1.ExecuteResponse{Result: fetchFailedResult(req, err)}, nil
		}
		if result.StatusCode < 200 || result.StatusCode >= 300 {
			failure := failedResult(req, commonv1.ErrorClass_ERROR_CLASS_VALIDATION_ERROR, "web resource did not return a successful HTTP status")
			failure.Error.DetailsJson = mustJSON(map[string]any{"reason": "http_status", "status_code": result.StatusCode})
			return &runtimev1.ExecuteResponse{Result: failure}, nil
		}
		fetched = &result
		if text == "" {
			text = strings.TrimSpace(result.ContentText)
		}
		if htmlContent == "" {
			htmlContent = strings.TrimSpace(result.ContentHTML)
		}
	}
	if text == "" && htmlContent == "" && fetched == nil {
		return &runtimev1.ExecuteResponse{Result: failedResult(req, commonv1.ErrorClass_ERROR_CLASS_VALIDATION_ERROR, "url, text, or html is required")}, nil
	}
	if text == "" && htmlContent != "" {
		text = extractReadableText(htmlContent)
	}

	payload := map[string]any{
		"url":          sourceURL,
		"content_text": text,
	}
	if fetched != nil {
		payload["status_code"] = fetched.StatusCode
		payload["final_url"] = fetched.FinalURL
		payload["mime_type"] = fetched.MIMEType
		payload["provider"] = fetched.Provider
		payload["metadata"] = fetched.Metadata
	}
	if args.IncludeHTML {
		payload["content_html"] = htmlContent
	}
	return &runtimev1.ExecuteResponse{Result: completedResult(req, mustJSON(payload))}, nil
}

type SelfHostedProvider struct {
	baseURL string
	client  *http.Client
}

func NewSelfHostedProvider(baseURL string, client *http.Client) *SelfHostedProvider {
	endpoint, err := url.Parse(strings.TrimSpace(baseURL))
	if err != nil || endpoint.Host == "" || endpoint.User != nil || (endpoint.Scheme != "http" && endpoint.Scheme != "https") || endpoint.RawQuery != "" || endpoint.Fragment != "" {
		return nil
	}
	if client == nil {
		client = &http.Client{Timeout: 30 * time.Second}
	}
	copyClient := *client
	copyClient.CheckRedirect = func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }
	return &SelfHostedProvider{baseURL: strings.TrimRight(endpoint.String(), "/"), client: &copyClient}
}

func (p *SelfHostedProvider) Name() string { return "self_hosted_primary" }

func (p *SelfHostedProvider) Fetch(ctx context.Context, rawURL string, includeHTML bool) (FetchResult, error) {
	requestBody, _ := json.Marshal(map[string]any{"url": rawURL, "include_html": includeHTML})
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, p.baseURL+"/fetch", strings.NewReader(string(requestBody)))
	if err != nil {
		return FetchResult{}, err
	}
	httpReq.Header.Set("Content-Type", "application/json")
	if err := egress.TakeRequest(ctx, httpReq); err != nil {
		return FetchResult{}, egress.ErrLimit
	}
	resp, err := p.client.Do(httpReq)
	if err != nil {
		return FetchResult{}, err
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return FetchResult{}, fmt.Errorf("self-hosted webfetch returned status %d", resp.StatusCode)
	}

	var payload struct {
		FinalURL    string         `json:"final_url"`
		StatusCode  int            `json:"status_code"`
		MIMEType    string         `json:"mime_type"`
		ContentText string         `json:"content_text"`
		ContentHTML string         `json:"content_html"`
		Metadata    map[string]any `json:"metadata"`
	}
	body, truncated, err := readContent(resp.Body)
	if err != nil {
		return FetchResult{}, err
	}
	if truncated {
		return FetchResult{}, errResponseLimit
	}
	if err := json.Unmarshal(body, &payload); err != nil {
		return FetchResult{}, err
	}
	return FetchResult{
		Provider:     p.Name(),
		RequestedURL: rawURL,
		FinalURL:     firstNonEmpty(payload.FinalURL, rawURL),
		StatusCode:   payload.StatusCode,
		MIMEType:     payload.MIMEType,
		ContentText:  payload.ContentText,
		ContentHTML:  payload.ContentHTML,
		Metadata:     payload.Metadata,
	}, nil
}

type JinaReaderProvider struct {
	baseURL string
	token   string
	client  *http.Client
}

func NewJinaReaderProvider(baseURL, token string, client *http.Client) *JinaReaderProvider {
	if strings.TrimSpace(baseURL) == "" {
		return nil
	}
	return &JinaReaderProvider{baseURL: strings.TrimRight(strings.TrimSpace(baseURL), "/"), token: strings.TrimSpace(token), client: egress.SecureClient(client)}
}

func (p *JinaReaderProvider) Name() string { return "jina_reader_fallback" }

func (p *JinaReaderProvider) Fetch(ctx context.Context, rawURL string, includeHTML bool) (FetchResult, error) {
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodGet, p.baseURL+"/"+strings.TrimPrefix(rawURL, "https://"), nil)
	if err != nil {
		return FetchResult{}, err
	}
	if p.token != "" {
		httpReq.Header.Set("Authorization", "Bearer "+p.token)
	}
	resp, err := p.client.Do(httpReq)
	if err != nil {
		return FetchResult{}, err
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 400 {
		return FetchResult{}, fmt.Errorf("jina reader returned status %d", resp.StatusCode)
	}
	body, truncated, err := readContent(resp.Body)
	if err != nil {
		return FetchResult{}, err
	}
	if _, err := textMIME(resp.Header.Get("Content-Type"), body); err != nil {
		return FetchResult{}, err
	}
	text, err := contentText(body, truncated)
	if err != nil {
		return FetchResult{}, err
	}
	return FetchResult{
		Provider:     p.Name(),
		RequestedURL: rawURL,
		FinalURL:     rawURL,
		StatusCode:   resp.StatusCode,
		MIMEType:     resp.Header.Get("Content-Type"),
		ContentText:  redact.Values(sanitize.Text(text), p.token),
		ContentHTML:  mapHTML(includeHTML, ""),
		Metadata:     map[string]any{"truncated": truncated},
	}, nil
}

type PlainHTTPProvider struct {
	client  *http.Client
	enabled bool
}

func NewPlainHTTPProvider(client *http.Client, enabled bool) *PlainHTTPProvider {
	return &PlainHTTPProvider{client: egress.SecureClient(client), enabled: enabled}
}

func (p *PlainHTTPProvider) Name() string { return "plain_http_fallback" }

func (p *PlainHTTPProvider) Fetch(ctx context.Context, rawURL string, includeHTML bool) (FetchResult, error) {
	if !p.enabled {
		return FetchResult{}, fmt.Errorf("plain http fallback is disabled")
	}
	if _, err := url.ParseRequestURI(strings.TrimSpace(rawURL)); err != nil {
		return FetchResult{}, err
	}
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodGet, rawURL, nil)
	if err != nil {
		return FetchResult{}, err
	}
	httpReq.Header.Set("User-Agent", "Butler/1.0 (public web research)")
	resp, err := p.client.Do(httpReq)
	if err != nil {
		return FetchResult{}, err
	}
	defer resp.Body.Close()
	body, truncated, err := readContent(resp.Body)
	if err != nil {
		return FetchResult{}, err
	}
	mimeType := resp.Header.Get("Content-Type")
	kind, err := textMIME(mimeType, body)
	if err != nil {
		return FetchResult{}, err
	}
	text, err := contentText(body, truncated)
	if err != nil {
		return FetchResult{}, err
	}
	if mimeType == "" {
		mimeType = kind
	}
	htmlContent := ""
	if kind == "text/html" || kind == "application/xhtml+xml" {
		htmlContent = text
		text = extractReadableText(htmlContent)
	}
	return FetchResult{
		Provider:     p.Name(),
		RequestedURL: rawURL,
		FinalURL:     firstNonEmpty(resp.Request.URL.String(), rawURL),
		StatusCode:   resp.StatusCode,
		MIMEType:     mimeType,
		ContentText:  text,
		ContentHTML:  mapHTML(includeHTML, htmlContent),
		Links:        documentLinks(htmlContent, firstNonEmpty(resp.Request.URL.String(), rawURL)),
		Metadata:     map[string]any{"truncated": truncated},
	}, nil
}

func extractReadableText(htmlContent string) string {
	doc, err := html.Parse(strings.NewReader(htmlContent))
	if err != nil {
		return strings.TrimSpace(htmlContent)
	}
	var builder strings.Builder
	var walk func(*html.Node)
	walk = func(node *html.Node) {
		if node.Type == html.ElementNode && (node.Data == "script" || node.Data == "style") {
			return
		}
		if node.Type == html.TextNode {
			text := strings.TrimSpace(node.Data)
			if text != "" {
				if builder.Len() > 0 {
					builder.WriteString(" ")
				}
				builder.WriteString(text)
			}
		}
		for child := node.FirstChild; child != nil; child = child.NextSibling {
			walk(child)
		}
	}
	walk(doc)
	return strings.TrimSpace(builder.String())
}

func mapHTML(include bool, htmlContent string) string {
	if include {
		return htmlContent
	}
	return ""
}

// Provider responses are observations, including metadata and returned URLs.
// Operator service credentials and cookies never become model-visible fields.
func sanitizeFetchResult(result FetchResult) FetchResult {
	encoded, err := json.Marshal(result)
	if err != nil {
		return FetchResult{Provider: result.Provider}
	}
	var cleaned FetchResult
	if json.Unmarshal([]byte(sanitize.JSON(string(encoded))), &cleaned) != nil {
		return FetchResult{Provider: result.Provider}
	}
	return cleaned
}

func completedResult(req *runtimev1.ExecuteRequest, resultJSON string) *toolbrokerv1.ToolResult {
	// JSON escaping and optional HTML can expand a bounded HTTP response beyond
	// the private gRPC response limit. Refuse explicitly rather than losing ACK.
	if len(resultJSON) > 3*1024*1024 {
		return failedResult(req, commonv1.ErrorClass_ERROR_CLASS_VALIDATION_ERROR, "web resource output exceeds the supported size limit")
	}
	call := req.GetToolCall()
	result := &toolbrokerv1.ToolResult{
		Status:     "completed",
		ResultJson: resultJSON,
		FinishedAt: time.Now().UTC().Format(time.RFC3339Nano),
	}
	if call != nil {
		result.ToolCallId = call.GetToolCallId()
		result.RunId = call.GetRunId()
		result.ToolName = call.GetToolName()
	}
	return result
}

func fetchFailedResult(req *runtimev1.ExecuteRequest, err error) *toolbrokerv1.ToolResult {
	if errors.Is(err, errUnsupportedContent) || errors.Is(err, errResponseLimit) {
		result := failedResult(req, commonv1.ErrorClass_ERROR_CLASS_VALIDATION_ERROR, err.Error())
		reason := "unsupported_content"
		if errors.Is(err, errResponseLimit) {
			reason = "response_limit"
		}
		result.Error.DetailsJson = mustJSON(map[string]any{"reason": reason})
		return result
	}
	return failedResult(req, commonv1.ErrorClass_ERROR_CLASS_INTERNAL_ERROR, "web resource unavailable or denied")
}

func failedResult(req *runtimev1.ExecuteRequest, class commonv1.ErrorClass, message string) *toolbrokerv1.ToolResult {
	call := req.GetToolCall()
	result := &toolbrokerv1.ToolResult{
		Status:     "failed",
		FinishedAt: time.Now().UTC().Format(time.RFC3339Nano),
		Error:      &toolbrokerv1.ToolError{ErrorClass: class, Message: message, Retryable: false, DetailsJson: "{}"},
	}
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

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return value
		}
	}
	return ""
}
