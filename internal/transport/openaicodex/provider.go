package openaicodex

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/butler/butler/internal/logger"
	"github.com/butler/butler/internal/providerauth"
	"github.com/butler/butler/internal/transport"
)

const providerName = "openai-codex"

type Config struct {
	Model      string
	BaseURL    string
	Timeout    time.Duration
	Logger     *slog.Logger
	AuthSource providerauth.OpenAICodexTokenSource
}

type Provider struct {
	config       Config
	httpClient   *http.Client
	capabilities transport.CapabilitySnapshot
	log          *slog.Logger
	mu           sync.Mutex
	active       map[string]context.CancelFunc
}

type streamState struct {
	providerSession *transport.ProviderSessionRef
	finalText       strings.Builder
	sessionID       string
}

type sseDecoder struct {
	scanner *bufio.Scanner
}

type sseMessage struct {
	Event string
	Data  string
}

func init() {
	transport.RegisterProvider(providerName, func(raw any) (transport.ModelProvider, error) {
		cfg, ok := raw.(Config)
		if !ok {
			return nil, fmt.Errorf("openai-codex: expected Config, got %T", raw)
		}
		return NewProvider(cfg, nil)
	})
}

func NewProvider(cfg Config, client *http.Client) (*Provider, error) {
	if strings.TrimSpace(cfg.Model) == "" {
		cfg.Model = "gpt-5.1-codex"
	}
	if strings.TrimSpace(cfg.BaseURL) == "" {
		cfg.BaseURL = "https://chatgpt.com/backend-api"
	}
	if cfg.Timeout <= 0 {
		cfg.Timeout = 60 * time.Second
	}
	if client == nil {
		client = &http.Client{Timeout: cfg.Timeout}
	}
	log := cfg.Logger
	if log == nil {
		log = slog.Default()
	}
	return &Provider{
		config:     cfg,
		httpClient: client,
		log:        logger.WithComponent(log, "transport-openai-codex"),
		active:     make(map[string]context.CancelFunc),
		capabilities: transport.CapabilitySnapshot{
			SupportsStreaming:        true,
			SupportsToolCalls:        true,
			SupportsBatchToolCalls:   true,
			SupportsStatefulSessions: true,
			SupportsCancel:           true,
			SupportsUsageMetadata:    true,
		},
	}, nil
}

func (p *Provider) Name() string {
	return providerName
}

func (p *Provider) Capabilities(_ context.Context, _ transport.TransportRunContext) (transport.CapabilitySnapshot, error) {
	return p.capabilities, nil
}

func (p *Provider) StartRun(ctx context.Context, req transport.StartRunRequest) (transport.EventStream, error) {
	if err := req.Validate(); err != nil {
		return nil, err
	}
	body, err := p.startRunBody(req)
	if err != nil {
		return nil, err
	}
	return p.executeStreamingRequest(ctx, req.Context.RunID, sessionIDFromStart(req), body)
}

func (p *Provider) ContinueRun(ctx context.Context, req transport.ContinueRunRequest) (transport.EventStream, error) {
	if err := req.Validate(); err != nil {
		return nil, err
	}
	body, err := p.continueRunBody(req)
	if err != nil {
		return nil, err
	}
	return p.executeStreamingRequest(ctx, req.RunID, sessionIDFromRef(req.ProviderSessionRef), body)
}

func (p *Provider) SubmitToolResult(ctx context.Context, req transport.SubmitToolResultRequest) (transport.EventStream, error) {
	if err := req.Validate(); err != nil {
		return nil, err
	}
	body, err := p.submitToolResultBody(req)
	if err != nil {
		return nil, err
	}
	return p.executeStreamingRequest(ctx, req.RunID, sessionIDFromRef(req.ProviderSessionRef), body)
}

func (p *Provider) CancelRun(_ context.Context, req transport.CancelRunRequest) (*transport.TransportEvent, error) {
	if err := req.Validate(); err != nil {
		return nil, err
	}
	p.mu.Lock()
	cancel := p.active[req.RunID]
	delete(p.active, req.RunID)
	p.mu.Unlock()
	if cancel != nil {
		cancel()
	}
	event := transport.NewTerminalEvent(req.RunID, transport.EventTypeRunCancelled, providerName)
	if req.ProviderSessionRef != nil {
		event.ProviderSessionRef = req.ProviderSessionRef
	}
	return &event, nil
}

func (p *Provider) executeStreamingRequest(parent context.Context, runID, sessionID string, body []byte) (transport.EventStream, error) {
	ctx, cancel := context.WithCancel(parent)
	p.registerCancel(runID, cancel)
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, p.endpoint("/codex/responses"), bytes.NewReader(body))
	if err != nil {
		p.clearCancel(runID)
		cancel()
		return nil, transport.NormalizeError(err, providerName)
	}
	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("Accept", "text/event-stream")
	if err := p.applyHeaders(ctx, sessionID, httpReq); err != nil {
		p.clearCancel(runID)
		cancel()
		return nil, transport.NormalizeError(err, providerName)
	}
	resp, err := p.httpClient.Do(httpReq)
	if err != nil {
		p.clearCancel(runID)
		cancel()
		return nil, transport.NormalizeError(err, providerName)
	}
	if err := decodeAPIError(resp); err != nil {
		p.clearCancel(runID)
		cancel()
		return nil, transport.NormalizeError(err, providerName)
	}
	stream := make(chan transport.TransportEvent, 32)
	go p.streamResponse(ctx, runID, sessionID, resp.Body, stream)
	return stream, nil
}

func (p *Provider) streamResponse(ctx context.Context, runID, sessionID string, body io.ReadCloser, stream chan<- transport.TransportEvent) {
	defer close(stream)
	defer body.Close()
	defer p.clearCancel(runID)

	decoder := newSSEDecoder(body)
	state := streamState{sessionID: sessionID}
	for {
		message, err := decoder.Next(ctx)
		if err != nil {
			if err == io.EOF || ctx.Err() != nil {
				return
			}
			stream <- transport.NewTransportErrorEvent(runID, providerName, err)
			return
		}
		events, stop, err := p.normalizeMessage(runID, message, &state)
		if err != nil {
			stream <- transport.NewTransportErrorEvent(runID, providerName, err)
			return
		}
		for _, event := range events {
			select {
			case <-ctx.Done():
				return
			case stream <- event:
			}
		}
		if stop {
			return
		}
	}
}

func (p *Provider) normalizeMessage(runID string, message sseMessage, state *streamState) ([]transport.TransportEvent, bool, error) {
	trimmed := strings.TrimSpace(message.Data)
	if trimmed == "" || trimmed == "[DONE]" {
		return nil, trimmed == "[DONE]", nil
	}
	var payload map[string]any
	if err := json.Unmarshal([]byte(trimmed), &payload); err != nil {
		return nil, false, fmt.Errorf("decode openai codex event %q: %w", message.Event, err)
	}
	eventType := message.Event
	if eventType == "" {
		eventType = stringValue(payload["type"])
	}
	return p.normalizePayload(runID, eventType, payload, state)
}

func (p *Provider) normalizePayload(runID, eventType string, payload map[string]any, state *streamState) ([]transport.TransportEvent, bool, error) {
	switch eventType {
	case "response.created", "response.in_progress":
		ref := providerSessionFromPayload(state.sessionID, payload)
		state.providerSession = ref
		events := []transport.TransportEvent{transport.NewRunStartedEvent(runID, providerName, p.capabilities, ref)}
		if ref != nil {
			events = append(events, transport.NewProviderSessionBoundEvent(runID, providerName, *ref))
		}
		return events, false, nil
	case "response.output_text.delta", "response.text.delta":
		delta := stringValue(payload["delta"])
		state.finalText.WriteString(delta)
		return []transport.TransportEvent{transport.NewAssistantDeltaEvent(runID, providerName, transport.AssistantDelta{DeltaType: "text", Content: delta, SequenceNo: intValue(payload["sequence_number"])})}, false, nil
	case "response.output_item.done":
		item := mapValue(payload["item"])
		if stringValue(item["type"]) != "function_call" {
			return nil, false, nil
		}
		return []transport.TransportEvent{transport.NewToolCallRequestedEvent(runID, providerName, transport.ToolCallRequest{ToolCallRef: stringValue(item["call_id"]), ToolName: stringValue(item["name"]), ArgsJSON: coalesceJSONString(item["arguments"], "{}"), SequenceNo: intValue(payload["sequence_number"])})}, true, nil
	case "response.function_call_arguments.done":
		return []transport.TransportEvent{transport.NewToolCallRequestedEvent(runID, providerName, transport.ToolCallRequest{ToolCallRef: stringValue(payload["call_id"]), ToolName: stringValue(payload["name"]), ArgsJSON: coalesceJSONString(payload["arguments"], "{}"), SequenceNo: intValue(payload["sequence_number"])})}, true, nil
	case "response.completed", "response.done":
		response := mapValue(payload["response"])
		status := stringValue(response["status"])
		if status == "failed" {
			return []transport.TransportEvent{transport.NewTransportErrorEvent(runID, providerName, errorFromPayload(map[string]any{"error": response["error"]}))}, true, nil
		}
		if ref := providerSessionFromPayload(state.sessionID, payload); ref != nil {
			state.providerSession = ref
		}
		finalText := strings.TrimSpace(extractOutputText(response))
		if finalText == "" {
			finalText = strings.TrimSpace(state.finalText.String())
		}
		finalEvent := transport.NewAssistantFinalEvent(runID, providerName, transport.AssistantFinal{Content: finalText, FinishReason: status, Usage: usageFromMap(mapValue(response["usage"]))})
		if state.providerSession != nil {
			finalEvent.ProviderSessionRef = state.providerSession
		}
		completed := transport.NewTerminalEvent(runID, transport.EventTypeRunCompleted, providerName)
		if state.providerSession != nil {
			completed.ProviderSessionRef = state.providerSession
		}
		return []transport.TransportEvent{finalEvent, completed}, true, nil
	case "response.failed", "error":
		return []transport.TransportEvent{transport.NewTransportErrorEvent(runID, providerName, errorFromPayload(payload))}, true, nil
	default:
		warning := transport.NewTransportWarningEvent(runID, providerName, "unhandled openai codex event", map[string]any{"event_type": eventType})
		if state.providerSession != nil {
			warning.ProviderSessionRef = state.providerSession
		}
		return []transport.TransportEvent{warning}, false, nil
	}
}

func (p *Provider) startRunBody(req transport.StartRunRequest) ([]byte, error) {
	payload := map[string]any{"model": req.Context.ModelName, "input": encodeInputItems(req.InputItems), "stream": req.StreamingEnabled}
	if req.Context.ProviderSessionRef != nil && req.Context.ProviderSessionRef.ResponseRef != "" {
		payload["previous_response_id"] = req.Context.ProviderSessionRef.ResponseRef
	}
	if sessionID := sessionIDFromStart(req); sessionID != "" {
		payload["prompt_cache_key"] = sessionID
	}
	if len(req.ToolDefinitions) > 0 {
		payload["tools"] = encodeTools(req.ToolDefinitions)
	}
	mergeOptions(payload, req.TransportOptionsJSON, req.TransportOptions)
	return json.Marshal(payload)
}

func (p *Provider) continueRunBody(req transport.ContinueRunRequest) ([]byte, error) {
	payload := map[string]any{"model": p.config.Model, "input": encodeInputItems(req.InputItems), "stream": true}
	if req.ProviderSessionRef != nil && req.ProviderSessionRef.ResponseRef != "" {
		payload["previous_response_id"] = req.ProviderSessionRef.ResponseRef
	}
	if sessionID := sessionIDFromRef(req.ProviderSessionRef); sessionID != "" {
		payload["prompt_cache_key"] = sessionID
	}
	mergeOptions(payload, req.TransportOptionsJSON, req.TransportOptions)
	return json.Marshal(payload)
}

func (p *Provider) submitToolResultBody(req transport.SubmitToolResultRequest) ([]byte, error) {
	payload := map[string]any{
		"model":  p.config.Model,
		"stream": true,
		"input":  []map[string]any{{"type": "function_call_output", "call_id": req.ToolCallRef, "output": json.RawMessage(req.ToolResultJSON)}},
	}
	if req.ProviderSessionRef != nil && req.ProviderSessionRef.ResponseRef != "" {
		payload["previous_response_id"] = req.ProviderSessionRef.ResponseRef
	}
	if sessionID := sessionIDFromRef(req.ProviderSessionRef); sessionID != "" {
		payload["prompt_cache_key"] = sessionID
	}
	mergeOptions(payload, req.TransportOptionsJSON, req.TransportOptions)
	return json.Marshal(payload)
}

func (p *Provider) applyHeaders(ctx context.Context, sessionID string, req *http.Request) error {
	if p.config.AuthSource == nil {
		return fmt.Errorf("openai codex auth source is not configured")
	}
	auth, err := p.config.AuthSource.ResolveOpenAICodex(ctx)
	if err != nil {
		return err
	}
	req.Header.Set("Authorization", "Bearer "+auth.AccessToken)
	req.Header.Set("chatgpt-account-id", auth.AccountID)
	req.Header.Set("OpenAI-Beta", "responses=experimental")
	req.Header.Set("originator", "butler")
	if sessionID != "" {
		req.Header.Set("conversation_id", sessionID)
		req.Header.Set("session_id", sessionID)
	}
	return nil
}

func (p *Provider) endpoint(path string) string {
	return strings.TrimRight(p.config.BaseURL, "/") + path
}

func (p *Provider) registerCancel(runID string, cancel context.CancelFunc) {
	p.mu.Lock()
	defer p.mu.Unlock()
	if existing := p.active[runID]; existing != nil {
		existing()
	}
	p.active[runID] = cancel
}

func (p *Provider) clearCancel(runID string) {
	p.mu.Lock()
	defer p.mu.Unlock()
	delete(p.active, runID)
}

func sessionIDFromStart(req transport.StartRunRequest) string {
	if trimmed := strings.TrimSpace(req.Context.SessionKey); trimmed != "" {
		return trimmed
	}
	return sessionIDFromRef(req.Context.ProviderSessionRef)
}

func sessionIDFromRef(ref *transport.ProviderSessionRef) string {
	if ref == nil {
		return ""
	}
	return strings.TrimSpace(ref.SessionRef)
}

func providerSessionFromPayload(sessionID string, payload map[string]any) *transport.ProviderSessionRef {
	response := mapValue(payload["response"])
	responseID := stringValue(response["id"])
	if responseID == "" {
		responseID = stringValue(payload["response_id"])
	}
	if responseID == "" && strings.TrimSpace(sessionID) == "" {
		return nil
	}
	now := time.Now().UTC()
	return &transport.ProviderSessionRef{ProviderName: providerName, SessionRef: strings.TrimSpace(sessionID), ResponseRef: responseID, CreatedAt: now, LastUsedAt: now}
}

func newSSEDecoder(reader io.Reader) *sseDecoder {
	scanner := bufio.NewScanner(reader)
	scanner.Buffer(make([]byte, 0, 64*1024), 1024*1024)
	return &sseDecoder{scanner: scanner}
}

func (d *sseDecoder) Next(ctx context.Context) (sseMessage, error) {
	message := sseMessage{}
	var dataLines []string
	for d.scanner.Scan() {
		select {
		case <-ctx.Done():
			return sseMessage{}, ctx.Err()
		default:
		}
		line := d.scanner.Text()
		if line == "" {
			if len(dataLines) == 0 && message.Event == "" {
				continue
			}
			message.Data = strings.Join(dataLines, "\n")
			return message, nil
		}
		if strings.HasPrefix(line, "event:") {
			message.Event = strings.TrimSpace(strings.TrimPrefix(line, "event:"))
			continue
		}
		if strings.HasPrefix(line, "data:") {
			dataLines = append(dataLines, strings.TrimSpace(strings.TrimPrefix(line, "data:")))
		}
	}
	if err := d.scanner.Err(); err != nil {
		return sseMessage{}, err
	}
	if len(dataLines) > 0 || message.Event != "" {
		message.Data = strings.Join(dataLines, "\n")
		return message, nil
	}
	return sseMessage{}, io.EOF
}

func encodeInputItems(items []transport.InputItem) []map[string]any {
	encoded := make([]map[string]any, 0, len(items))
	for _, item := range items {
		entry := map[string]any{"role": item.Role, "content": []map[string]any{{"type": "input_text", "text": item.Content}}}
		if strings.TrimSpace(item.Name) != "" {
			entry["name"] = item.Name
		}
		encoded = append(encoded, entry)
	}
	return encoded
}

func encodeTools(tools []transport.ToolDefinition) []map[string]any {
	encoded := make([]map[string]any, 0, len(tools))
	for _, tool := range tools {
		entry := map[string]any{"type": "function", "name": tool.Name, "description": tool.Description}
		if strings.TrimSpace(tool.SchemaJSON) != "" {
			var schema any
			if err := json.Unmarshal([]byte(tool.SchemaJSON), &schema); err == nil {
				entry["parameters"] = schema
			}
		}
		encoded = append(encoded, entry)
	}
	return encoded
}

func decodeAPIError(resp *http.Response) error {
	if resp.StatusCode >= 200 && resp.StatusCode < 300 {
		return nil
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)
	message := strings.TrimSpace(string(body))
	code := ""
	var payload map[string]any
	if json.Unmarshal(body, &payload) == nil {
		if errPayload, ok := payload["error"].(map[string]any); ok {
			message = stringValue(errPayload["message"])
			code = stringValue(errPayload["code"])
			payload = errPayload
		}
	}
	return &transport.HTTPStatusError{StatusCode: resp.StatusCode, Message: message, Code: code, Details: payload}
}

func errorFromPayload(payload map[string]any) error {
	errPayload := mapValue(payload["error"])
	message := stringValue(errPayload["message"])
	if message == "" {
		message = stringValue(payload["message"])
	}
	return &transport.Error{Type: transport.ErrorTypeProviderProtocolError, Message: message, ProviderName: providerName, ProviderCode: stringValue(errPayload["code"]), ProviderDetails: errPayload}
}

func extractOutputText(response map[string]any) string {
	if text := stringValue(response["output_text"]); text != "" {
		return text
	}
	output, _ := response["output"].([]any)
	var builder strings.Builder
	for _, rawItem := range output {
		item := mapValue(rawItem)
		if stringValue(item["type"]) != "message" {
			continue
		}
		content, _ := item["content"].([]any)
		for _, rawContent := range content {
			contentItem := mapValue(rawContent)
			if stringValue(contentItem["type"]) == "output_text" {
				builder.WriteString(stringValue(contentItem["text"]))
			}
		}
	}
	return builder.String()
}

func usageFromMap(payload map[string]any) *transport.Usage {
	if len(payload) == 0 {
		return nil
	}
	return &transport.Usage{InputTokens: intValue(payload["input_tokens"]), OutputTokens: intValue(payload["output_tokens"]), TotalTokens: intValue(payload["total_tokens"])}
}

func mergeOptions(payload map[string]any, raw string, options map[string]any) {
	if strings.TrimSpace(raw) != "" {
		var parsed map[string]any
		if json.Unmarshal([]byte(raw), &parsed) == nil {
			for key, value := range parsed {
				payload[key] = value
			}
		}
	}
	for key, value := range options {
		payload[key] = value
	}
}

func mapValue(value any) map[string]any {
	if mapped, ok := value.(map[string]any); ok {
		return mapped
	}
	return map[string]any{}
}

func stringValue(value any) string {
	if value == nil {
		return ""
	}
	if typed, ok := value.(string); ok {
		return typed
	}
	return fmt.Sprintf("%v", value)
}

func intValue(value any) int {
	switch typed := value.(type) {
	case float64:
		return int(typed)
	case int:
		return typed
	case int64:
		return int(typed)
	default:
		return 0
	}
}

func coalesceJSONString(value any, fallback string) string {
	text := strings.TrimSpace(stringValue(value))
	if text == "" {
		return fallback
	}
	if json.Valid([]byte(text)) {
		return text
	}
	encoded, err := json.Marshal(text)
	if err != nil {
		return fallback
	}
	return string(encoded)
}
