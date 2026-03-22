package runtime

import (
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

var supportedWebFetchTools = map[string]bool{
	"web.fetch":       true,
	"web.fetch_batch": true,
	"web.extract":     true,
}

type Provider interface {
	Name() string
	Fetch(ctx context.Context, url string, includeHTML bool) (FetchResult, error)
}

type FetchResult struct {
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
}

func NewProviderChain(providers ...Provider) *ProviderChain {
	filtered := make([]Provider, 0, len(providers))
	for _, provider := range providers {
		if provider != nil {
			filtered = append(filtered, provider)
		}
	}
	return &ProviderChain{providers: filtered}
}

func (c *ProviderChain) Fetch(ctx context.Context, rawURL string, includeHTML bool) (FetchResult, error) {
	var lastErr error
	for _, provider := range c.providers {
		result, err := provider.Fetch(ctx, rawURL, includeHTML)
		if err == nil {
			if result.Provider == "" {
				result.Provider = provider.Name()
			}
			return result, nil
		}
		lastErr = err
	}
	if lastErr == nil {
		lastErr = fmt.Errorf("no webfetch providers are configured")
	}
	return FetchResult{}, lastErr
}

type Server struct {
	runtimev1.UnimplementedToolRuntimeServiceServer

	fetcher *ProviderChain
	log     *slog.Logger
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
	return &Server{fetcher: fetcher, log: logger.WithComponent(log, "tool-webfetch-runtime")}
}

func (s *Server) Execute(ctx context.Context, req *runtimev1.ExecuteRequest) (*runtimev1.ExecuteResponse, error) {
	call := req.GetToolCall()
	if call == nil {
		return &runtimev1.ExecuteResponse{Result: failedResult(req, commonv1.ErrorClass_ERROR_CLASS_VALIDATION_ERROR, "tool_call is required")}, nil
	}
	if !supportedWebFetchTools[call.GetToolName()] {
		return &runtimev1.ExecuteResponse{Result: failedResult(req, commonv1.ErrorClass_ERROR_CLASS_VALIDATION_ERROR, "unsupported tool for webfetch runtime")}, nil
	}

	switch call.GetToolName() {
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
		return &runtimev1.ExecuteResponse{Result: failedResult(req, commonv1.ErrorClass_ERROR_CLASS_INTERNAL_ERROR, err.Error())}, nil
	}
	return &runtimev1.ExecuteResponse{Result: completedResult(req, mustJSON(result))}, nil
}

func (s *Server) executeFetchBatch(ctx context.Context, req *runtimev1.ExecuteRequest) (*runtimev1.ExecuteResponse, error) {
	var args fetchBatchArgs
	if err := json.Unmarshal([]byte(req.GetToolCall().GetArgsJson()), &args); err != nil {
		return &runtimev1.ExecuteResponse{Result: failedResult(req, commonv1.ErrorClass_ERROR_CLASS_VALIDATION_ERROR, "args_json must decode into web.fetch_batch args")}, nil
	}
	if len(args.URLs) == 0 {
		return &runtimev1.ExecuteResponse{Result: failedResult(req, commonv1.ErrorClass_ERROR_CLASS_VALIDATION_ERROR, "urls is required")}, nil
	}

	items := make([]map[string]any, 0, len(args.URLs))
	for _, itemURL := range args.URLs {
		result, err := s.fetcher.Fetch(ctx, itemURL, args.IncludeHTML)
		if err != nil {
			items = append(items, map[string]any{"requested_url": itemURL, "error": err.Error()})
			continue
		}
		items = append(items, map[string]any{
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
	if sourceURL != "" {
		result, err := s.fetcher.Fetch(ctx, sourceURL, true)
		if err != nil {
			return &runtimev1.ExecuteResponse{Result: failedResult(req, commonv1.ErrorClass_ERROR_CLASS_INTERNAL_ERROR, err.Error())}, nil
		}
		if text == "" {
			text = strings.TrimSpace(result.ContentText)
		}
		if htmlContent == "" {
			htmlContent = strings.TrimSpace(result.ContentHTML)
		}
	}
	if text == "" && htmlContent == "" {
		return &runtimev1.ExecuteResponse{Result: failedResult(req, commonv1.ErrorClass_ERROR_CLASS_VALIDATION_ERROR, "url, text, or html is required")}, nil
	}
	if text == "" && htmlContent != "" {
		text = extractReadableText(htmlContent)
	}

	payload := map[string]any{
		"url":          sourceURL,
		"content_text": text,
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
	if strings.TrimSpace(baseURL) == "" {
		return nil
	}
	return &SelfHostedProvider{baseURL: strings.TrimRight(strings.TrimSpace(baseURL), "/"), client: client}
}

func (p *SelfHostedProvider) Name() string { return "self_hosted_primary" }

func (p *SelfHostedProvider) Fetch(ctx context.Context, rawURL string, includeHTML bool) (FetchResult, error) {
	requestBody, _ := json.Marshal(map[string]any{"url": rawURL, "include_html": includeHTML})
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, p.baseURL+"/fetch", strings.NewReader(string(requestBody)))
	if err != nil {
		return FetchResult{}, err
	}
	httpReq.Header.Set("Content-Type", "application/json")
	resp, err := p.client.Do(httpReq)
	if err != nil {
		return FetchResult{}, err
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 400 {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 1024))
		return FetchResult{}, fmt.Errorf("self-hosted webfetch returned status %d: %s", resp.StatusCode, strings.TrimSpace(string(body)))
	}

	var payload struct {
		FinalURL    string         `json:"final_url"`
		StatusCode  int            `json:"status_code"`
		MIMEType    string         `json:"mime_type"`
		ContentText string         `json:"content_text"`
		ContentHTML string         `json:"content_html"`
		Metadata    map[string]any `json:"metadata"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&payload); err != nil {
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
	return &JinaReaderProvider{baseURL: strings.TrimRight(strings.TrimSpace(baseURL), "/"), token: strings.TrimSpace(token), client: client}
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
	body, err := io.ReadAll(io.LimitReader(resp.Body, 2*1024*1024))
	if err != nil {
		return FetchResult{}, err
	}
	return FetchResult{
		Provider:     p.Name(),
		RequestedURL: rawURL,
		FinalURL:     rawURL,
		StatusCode:   resp.StatusCode,
		MIMEType:     resp.Header.Get("Content-Type"),
		ContentText:  string(body),
		ContentHTML:  mapHTML(includeHTML, ""),
		Metadata:     map[string]any{},
	}, nil
}

type PlainHTTPProvider struct {
	client  *http.Client
	enabled bool
}

func NewPlainHTTPProvider(client *http.Client, enabled bool) *PlainHTTPProvider {
	return &PlainHTTPProvider{client: client, enabled: enabled}
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
	resp, err := p.client.Do(httpReq)
	if err != nil {
		return FetchResult{}, err
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(io.LimitReader(resp.Body, 2*1024*1024))
	if err != nil {
		return FetchResult{}, err
	}
	htmlContent := string(body)
	mimeType := resp.Header.Get("Content-Type")
	text := htmlContent
	if strings.Contains(strings.ToLower(mimeType), "html") {
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
		Metadata:     map[string]any{},
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

func completedResult(req *runtimev1.ExecuteRequest, resultJSON string) *toolbrokerv1.ToolResult {
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
