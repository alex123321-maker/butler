package openai

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
	"time"

	"github.com/butler/butler/internal/logger"
	"github.com/butler/butler/internal/transport"
)

const providerName = "openai"

type Config struct {
	APIKey  string
	Model   string
	BaseURL string
	Timeout time.Duration
	Logger  *slog.Logger
}

type Provider struct {
	config       Config
	httpClient   *http.Client
	capabilities transport.CapabilitySnapshot
	log          *slog.Logger
}

func NewProvider(cfg Config, client *http.Client) (*Provider, error) {
	if strings.TrimSpace(cfg.Model) == "" {
		cfg.Model = "gpt-5-mini"
	}
	if strings.TrimSpace(cfg.BaseURL) == "" {
		cfg.BaseURL = "https://api.openai.com/v1"
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
		log:        logger.WithComponent(log, "transport-openai"),
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
	p.log.Info("starting openai run",
		slog.String("run_id", req.Context.RunID),
		slog.String("model", req.Context.ModelName),
		slog.Bool("streaming_enabled", req.StreamingEnabled),
	)
	body, err := p.startRunBody(req)
	if err != nil {
		return nil, err
	}
	return p.executeStreamingRequest(ctx, req.Context.RunID, http.MethodPost, p.endpoint("/responses"), body)
}

func (p *Provider) ContinueRun(ctx context.Context, req transport.ContinueRunRequest) (transport.EventStream, error) {
	if err := req.Validate(); err != nil {
		return nil, err
	}
	p.log.Info("continuing openai run", slog.String("run_id", req.RunID))
	body, err := p.continueRunBody(req)
	if err != nil {
		return nil, err
	}
	return p.executeStreamingRequest(ctx, req.RunID, http.MethodPost, p.endpoint("/responses"), body)
}

func (p *Provider) SubmitToolResult(ctx context.Context, req transport.SubmitToolResultRequest) (transport.EventStream, error) {
	if err := req.Validate(); err != nil {
		return nil, err
	}
	p.log.Info("submitting tool result to openai",
		slog.String("run_id", req.RunID),
		slog.String("tool_call_ref", req.ToolCallRef),
	)
	body, err := p.submitToolResultBody(req)
	if err != nil {
		return nil, err
	}
	return p.executeStreamingRequest(ctx, req.RunID, http.MethodPost, p.endpoint("/responses"), body)
}

func (p *Provider) CancelRun(ctx context.Context, req transport.CancelRunRequest) (*transport.TransportEvent, error) {
	if err := req.Validate(); err != nil {
		return nil, err
	}
	p.log.Info("cancelling openai run", slog.String("run_id", req.RunID))
	responseRef := ""
	if req.ProviderSessionRef != nil {
		responseRef = strings.TrimSpace(req.ProviderSessionRef.ResponseRef)
	}
	if responseRef == "" {
		event := transport.NewTerminalEvent(req.RunID, transport.EventTypeRunCancelled, providerName)
		return &event, nil
	}

	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, p.endpoint("/responses/"+responseRef+"/cancel"), nil)
	if err != nil {
		return nil, transport.NormalizeError(err, providerName)
	}
	p.applyHeaders(httpReq)

	resp, err := p.httpClient.Do(httpReq)
	if err != nil {
		p.log.Error("openai cancel request failed", slog.String("run_id", req.RunID), slog.String("error", err.Error()))
		return nil, transport.NormalizeError(err, providerName)
	}
	defer resp.Body.Close()
	if err := decodeAPIError(resp); err != nil {
		p.log.Error("openai cancel request rejected", slog.String("run_id", req.RunID), slog.Int("status_code", resp.StatusCode), slog.String("error", err.Error()))
		return nil, transport.NormalizeError(err, providerName)
	}
	event := transport.NewTerminalEvent(req.RunID, transport.EventTypeRunCancelled, providerName)
	p.log.Info("openai run cancelled", slog.String("run_id", req.RunID), slog.Int("status_code", resp.StatusCode))
	return &event, nil
}

func (p *Provider) executeStreamingRequest(ctx context.Context, runID, method, url string, body []byte) (transport.EventStream, error) {
	path := strings.TrimPrefix(url, strings.TrimRight(p.config.BaseURL, "/"))
	httpReq, err := http.NewRequestWithContext(ctx, method, url, bytes.NewReader(body))
	if err != nil {
		p.log.Error("failed to create openai request", slog.String("run_id", runID), slog.String("method", method), slog.String("path", path), slog.String("error", err.Error()))
		return nil, transport.NormalizeError(err, providerName)
	}
	p.applyHeaders(httpReq)
	httpReq.Header.Set("Content-Type", "application/json")

	p.log.Info("opening openai stream", slog.String("run_id", runID), slog.String("method", method), slog.String("path", path))
	resp, err := p.httpClient.Do(httpReq)
	if err != nil {
		p.log.Error("openai stream request failed", slog.String("run_id", runID), slog.String("path", path), slog.String("error", err.Error()))
		return nil, transport.NormalizeError(err, providerName)
	}
	if err := decodeAPIError(resp); err != nil {
		resp.Body.Close()
		p.log.Error("openai stream request rejected", slog.String("run_id", runID), slog.String("path", path), slog.Int("status_code", resp.StatusCode), slog.String("error", err.Error()))
		return nil, transport.NormalizeError(err, providerName)
	}
	p.log.Info("openai stream opened", slog.String("run_id", runID), slog.String("path", path), slog.Int("status_code", resp.StatusCode))

	stream := make(chan transport.TransportEvent, 32)
	go p.streamResponse(ctx, runID, path, resp.Body, stream)
	return stream, nil
}

func (p *Provider) streamResponse(ctx context.Context, runID, path string, body io.ReadCloser, stream chan<- transport.TransportEvent) {
	defer close(stream)
	defer body.Close()

	decoder := newSSEDecoder(body)
	state := streamState{}
	eventsEmitted := 0
	for {
		message, err := decoder.Next(ctx)
		if err != nil {
			if err == io.EOF || ctx.Err() != nil {
				p.log.Info("openai stream closed", slog.String("run_id", runID), slog.String("path", path), slog.Int("events_emitted", eventsEmitted))
				return
			}
			p.log.Error("openai stream decode failed", slog.String("run_id", runID), slog.String("path", path), slog.String("error", err.Error()))
			stream <- transport.NewTransportErrorEvent(runID, providerName, err)
			return
		}

		events, stop, err := p.normalizeMessage(runID, message, &state)
		if err != nil {
			p.log.Error("openai event normalization failed", slog.String("run_id", runID), slog.String("path", path), slog.String("error", err.Error()))
			stream <- transport.NewTransportErrorEvent(runID, providerName, err)
			return
		}
		for _, event := range events {
			select {
			case <-ctx.Done():
				p.log.Info("openai stream cancelled", slog.String("run_id", runID), slog.String("path", path), slog.Int("events_emitted", eventsEmitted))
				return
			case stream <- event:
				eventsEmitted++
			}
		}
		if stop {
			p.log.Info("openai stream completed", slog.String("run_id", runID), slog.String("path", path), slog.Int("events_emitted", eventsEmitted))
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
		return nil, false, fmt.Errorf("decode openai event %q: %w", message.Event, err)
	}

	eventType := message.Event
	if eventType == "" {
		eventType, _ = payload["type"].(string)
	}

	switch eventType {
	case "response.created", "response.in_progress":
		ref := providerSessionFromPayload(payload)
		state.providerSession = ref
		events := []transport.TransportEvent{transport.NewRunStartedEvent(runID, providerName, p.capabilities, ref)}
		if ref != nil {
			events = append(events, transport.NewProviderSessionBoundEvent(runID, providerName, *ref))
		}
		return events, false, nil
	case "response.output_text.delta":
		delta := stringValue(payload["delta"])
		state.finalText.WriteString(delta)
		return []transport.TransportEvent{transport.NewAssistantDeltaEvent(runID, providerName, transport.AssistantDelta{
			DeltaType:  "text",
			Content:    delta,
			SequenceNo: intValue(payload["sequence_number"]),
		})}, false, nil
	case "response.output_item.done":
		item, _ := payload["item"].(map[string]any)
		if stringValue(item["type"]) != "function_call" {
			return nil, false, nil
		}
		return []transport.TransportEvent{transport.NewToolCallRequestedEvent(runID, providerName, transport.ToolCallRequest{
			ToolCallRef: stringValue(item["call_id"]),
			ToolName:    stringValue(item["name"]),
			ArgsJSON:    coalesceJSONString(item["arguments"], "{}"),
			SequenceNo:  intValue(payload["sequence_number"]),
		})}, false, nil
	case "response.completed":
		response, _ := payload["response"].(map[string]any)
		if ref := providerSessionFromPayload(payload); ref != nil {
			state.providerSession = ref
		}
		finalText := strings.TrimSpace(extractOutputText(response))
		if finalText == "" {
			finalText = strings.TrimSpace(state.finalText.String())
		}
		finalEvent := transport.NewAssistantFinalEvent(runID, providerName, transport.AssistantFinal{
			Content:      finalText,
			FinishReason: stringValue(response["status"]),
			Usage:        usageFromMap(mapValue(response["usage"])),
		})
		if state.providerSession != nil {
			finalEvent.ProviderSessionRef = state.providerSession
		}
		completed := transport.NewTerminalEvent(runID, transport.EventTypeRunCompleted, providerName)
		if state.providerSession != nil {
			completed.ProviderSessionRef = state.providerSession
		}
		return []transport.TransportEvent{finalEvent, completed}, true, nil
	case "response.failed", "error":
		providerErr := errorFromPayload(payload)
		errorEvent := transport.NewTransportErrorEvent(runID, providerName, providerErr)
		if state.providerSession != nil {
			errorEvent.ProviderSessionRef = state.providerSession
		}
		return []transport.TransportEvent{errorEvent}, true, nil
	default:
		warning := transport.NewTransportWarningEvent(runID, providerName, "unhandled openai stream event", map[string]any{"event_type": eventType})
		if state.providerSession != nil {
			warning.ProviderSessionRef = state.providerSession
		}
		return []transport.TransportEvent{warning}, false, nil
	}
}

func (p *Provider) startRunBody(req transport.StartRunRequest) ([]byte, error) {
	payload := map[string]any{
		"model":  req.Context.ModelName,
		"input":  encodeInputItems(req.InputItems),
		"stream": req.StreamingEnabled,
	}
	if req.Context.ProviderSessionRef != nil && req.Context.ProviderSessionRef.ResponseRef != "" {
		payload["previous_response_id"] = req.Context.ProviderSessionRef.ResponseRef
	}
	if len(req.ToolDefinitions) > 0 {
		payload["tools"] = encodeTools(req.ToolDefinitions)
	}
	if options := mergeOptions(req.TransportOptionsJSON, req.TransportOptions); len(options) > 0 {
		for key, value := range options {
			payload[key] = value
		}
	}
	return json.Marshal(payload)
}

func (p *Provider) continueRunBody(req transport.ContinueRunRequest) ([]byte, error) {
	payload := map[string]any{
		"model":  p.config.Model,
		"input":  encodeInputItems(req.InputItems),
		"stream": true,
	}
	if req.ProviderSessionRef != nil && req.ProviderSessionRef.ResponseRef != "" {
		payload["previous_response_id"] = req.ProviderSessionRef.ResponseRef
	}
	if options := mergeOptions(req.TransportOptionsJSON, req.TransportOptions); len(options) > 0 {
		for key, value := range options {
			payload[key] = value
		}
	}
	return json.Marshal(payload)
}

func (p *Provider) submitToolResultBody(req transport.SubmitToolResultRequest) ([]byte, error) {
	payload := map[string]any{
		"model":  p.config.Model,
		"stream": true,
		"input": []map[string]any{{
			"type":    "function_call_output",
			"call_id": req.ToolCallRef,
			"output":  json.RawMessage(req.ToolResultJSON),
		}},
	}
	if req.ProviderSessionRef != nil && req.ProviderSessionRef.ResponseRef != "" {
		payload["previous_response_id"] = req.ProviderSessionRef.ResponseRef
	}
	if options := mergeOptions(req.TransportOptionsJSON, req.TransportOptions); len(options) > 0 {
		for key, value := range options {
			payload[key] = value
		}
	}
	return json.Marshal(payload)
}

func (p *Provider) applyHeaders(req *http.Request) {
	if strings.TrimSpace(p.config.APIKey) != "" {
		req.Header.Set("Authorization", "Bearer "+p.config.APIKey)
	}
	if req.Header.Get("Accept") == "" {
		req.Header.Set("Accept", "text/event-stream")
	}
}

func (p *Provider) endpoint(path string) string {
	return strings.TrimRight(p.config.BaseURL, "/") + path
}

type sseDecoder struct {
	scanner *bufio.Scanner
}

type sseMessage struct {
	Event string
	Data  string
}

func newSSEDecoder(reader io.Reader) *sseDecoder {
	scanner := bufio.NewScanner(reader)
	buffer := make([]byte, 0, 64*1024)
	scanner.Buffer(buffer, 1024*1024)
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

type streamState struct {
	providerSession *transport.ProviderSessionRef
	finalText       strings.Builder
}

func providerSessionFromPayload(payload map[string]any) *transport.ProviderSessionRef {
	response := mapValue(payload["response"])
	responseID := stringValue(response["id"])
	if responseID == "" {
		responseID = stringValue(payload["response_id"])
	}
	if responseID == "" {
		return nil
	}
	now := time.Now().UTC()
	return &transport.ProviderSessionRef{
		ProviderName: providerName,
		ResponseRef:  responseID,
		CreatedAt:    now,
		LastUsedAt:   now,
	}
}

func encodeInputItems(items []transport.InputItem) []map[string]any {
	encoded := make([]map[string]any, 0, len(items))
	for _, item := range items {
		entry := map[string]any{
			"role": item.Role,
			"content": []map[string]any{{
				"type": "input_text",
				"text": item.Content,
			}},
		}
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
		entry := map[string]any{
			"type":        "function",
			"name":        tool.Name,
			"description": tool.Description,
		}
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
	code := stringValue(errPayload["code"])
	return &transport.Error{
		Type:            transport.ErrorTypeProviderProtocolError,
		Message:         message,
		ProviderName:    providerName,
		ProviderCode:    code,
		ProviderDetails: errPayload,
	}
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
	return &transport.Usage{
		InputTokens:  intValue(payload["input_tokens"]),
		OutputTokens: intValue(payload["output_tokens"]),
		TotalTokens:  intValue(payload["total_tokens"]),
	}
}

func mergeOptions(raw string, options map[string]any) map[string]any {
	merged := make(map[string]any)
	if strings.TrimSpace(raw) != "" {
		_ = json.Unmarshal([]byte(raw), &merged)
	}
	for key, value := range options {
		merged[key] = value
	}
	return merged
}

func mapValue(value any) map[string]any {
	if value == nil {
		return map[string]any{}
	}
	if mapped, ok := value.(map[string]any); ok {
		return mapped
	}
	return map[string]any{}
}

func stringValue(value any) string {
	if value == nil {
		return ""
	}
	if s, ok := value.(string); ok {
		return s
	}
	return fmt.Sprintf("%v", value)
}

func intValue(value any) int {
	switch typed := value.(type) {
	case float64:
		return int(typed)
	case int:
		return typed
	case int32:
		return int(typed)
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
