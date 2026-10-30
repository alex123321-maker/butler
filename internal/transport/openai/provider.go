package openai

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"math"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/butler/butler/internal/logger"
	"github.com/butler/butler/internal/transport"
)

const providerName = "openai"

const (
	TransportModeWSFirst = "ws-first"
	TransportModeSSEOnly = "sse-only"
)

func init() {
	transport.RegisterProvider(providerName, func(raw any) (transport.ModelProvider, error) {
		cfg, ok := raw.(Config)
		if !ok {
			return nil, fmt.Errorf("openai: expected Config, got %T", raw)
		}
		return NewProvider(cfg, nil)
	})
}

type Config struct {
	APIKey              string
	Model               string
	BaseURL             string
	ResponsesWSURL      string
	RealtimeURL         string // Deprecated name; this is a Responses WebSocket URL.
	ReasoningEffort     string
	MaximumInputTokens  int
	MaximumOutputTokens int
	TransportMode       string
	Timeout             time.Duration
	Logger              *slog.Logger
}

type Provider struct {
	toolSets     map[string][]transport.ToolDefinition
	config       Config
	httpClient   *http.Client
	capabilities transport.CapabilitySnapshot
	log          *slog.Logger
	mu           sync.Mutex
	sessions     map[string]*responsesSession // runID -> Responses socket
	cancels      map[string]context.CancelFunc
	aliases      map[string]map[string]string
	prefixTokens map[string]map[string]int64
	expiry       map[string]*time.Timer
	instructions map[string]string // runID -> instruction continuity within this run
}

func NewProvider(cfg Config, client *http.Client) (*Provider, error) {
	if strings.TrimSpace(cfg.Model) == "" {
		cfg.Model = "gpt-4o-mini"
	}
	if strings.TrimSpace(cfg.BaseURL) == "" {
		cfg.BaseURL = "https://api.openai.com/v1"
	}
	if cfg.ResponsesWSURL != "" {
		cfg.RealtimeURL = cfg.ResponsesWSURL
	}
	if strings.TrimSpace(cfg.RealtimeURL) == "" {
		cfg.RealtimeURL = deriveResponsesURL(cfg.BaseURL)
	}
	if strings.TrimSpace(cfg.TransportMode) == "" {
		cfg.TransportMode = TransportModeWSFirst
	}
	if cfg.RealtimeURL == "wss://api.openai.com/v1/realtime" {
		cfg.RealtimeURL = deriveResponsesURL(cfg.BaseURL)
	}
	if strings.Contains(cfg.RealtimeURL, "/realtime") {
		return nil, fmt.Errorf("openai: Realtime endpoint cannot serve Responses models")
	}
	if cfg.TransportMode != TransportModeWSFirst && cfg.TransportMode != TransportModeSSEOnly {
		return nil, fmt.Errorf("openai: unsupported transport mode")
	}
	switch cfg.ReasoningEffort {
	case "", "none", "low", "medium", "high", "xhigh", "max":
	default:
		return nil, fmt.Errorf("openai: invalid reasoning effort")
	}
	if cfg.MaximumInputTokens == 0 {
		cfg.MaximumInputTokens = 65536
	}
	if cfg.MaximumOutputTokens == 0 {
		cfg.MaximumOutputTokens = 8192
	}
	if cfg.MaximumInputTokens < 1 || cfg.MaximumOutputTokens < 1 {
		return nil, fmt.Errorf("openai: invalid inference limits")
	}
	if cfg.Timeout <= 0 {
		cfg.Timeout = 60 * time.Second
	}
	if client == nil {
		client = &http.Client{Timeout: cfg.Timeout}
	}
	clientCopy := *client
	clientCopy.CheckRedirect = func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }
	client = &clientCopy
	log := cfg.Logger
	if log == nil {
		log = slog.Default()
	}
	return &Provider{
		config:       cfg,
		httpClient:   client,
		log:          logger.WithComponent(log, "transport-openai"),
		sessions:     make(map[string]*responsesSession),
		instructions: make(map[string]string),
		cancels:      make(map[string]context.CancelFunc), aliases: make(map[string]map[string]string), expiry: make(map[string]*time.Timer), prefixTokens: make(map[string]map[string]int64),
		capabilities: transport.CapabilitySnapshot{
			SupportsStreaming:        true,
			SupportsToolCalls:        true,
			SupportsBatchToolCalls:   false,
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
	if req.Context.ModelName != p.config.Model {
		return nil, fmt.Errorf("openai: request model differs from configured model")
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
	return p.executeRunRequest(ctx, req.Context.RunID, body)
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
	return p.executeRunRequest(ctx, req.RunID, body)
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
	return p.executeRunRequest(ctx, req.RunID, body)
}

func (p *Provider) CancelRun(ctx context.Context, req transport.CancelRunRequest) (*transport.TransportEvent, error) {
	if err := req.Validate(); err != nil {
		return nil, err
	}
	p.cancelRequest(req.RunID)
	p.CloseRun(req.RunID)
	p.clearInstructions(req.RunID)
	event := transport.NewTerminalEvent(req.RunID, transport.EventTypeRunCancelled, providerName)
	return &event, nil
}

func (p *Provider) executeStreamingRequest(ctx context.Context, runID, method, url string, body []byte) (transport.EventStream, error) {
	if err := transport.CheckSerializedInput(ctx, len(body)); err != nil {
		return nil, err
	}
	path := strings.TrimPrefix(url, strings.TrimRight(p.config.BaseURL, "/"))
	httpReq, err := http.NewRequestWithContext(ctx, method, url, bytes.NewReader(body))
	if err != nil {
		p.log.Error("failed to create openai request", slog.String("run_id", runID), slog.String("method", method), slog.String("path", path), slog.String("error", transport.NormalizeError(err, providerName).Error()))
		return nil, transport.NormalizeError(err, providerName)
	}
	if err := transport.CheckInferenceAuthority(ctx); err != nil {
		return nil, err
	}
	p.applyHeaders(httpReq)
	httpReq.Header.Set("Content-Type", "application/json")
	if err := transport.CheckInferenceDispatch(ctx); err != nil {
		return nil, err
	}

	p.log.Info("opening openai stream", slog.String("run_id", runID), slog.String("method", method), slog.String("path", path))
	resp, err := p.httpClient.Do(httpReq)
	if err != nil {
		p.log.Error("openai stream request failed", slog.String("run_id", runID), slog.String("path", path), slog.String("error", transport.NormalizeError(err, providerName).Error()))
		return nil, transport.NormalizeError(err, providerName)
	}
	if err := decodeAPIError(resp); err != nil {
		resp.Body.Close()
		p.log.Error("openai stream request rejected", slog.String("run_id", runID), slog.String("path", path), slog.Int("status_code", resp.StatusCode), slog.String("error", transport.NormalizeError(err, providerName).Error()))
		failure := transport.NormalizeError(err, providerName)
		failure.HTTPStatus = resp.StatusCode
		return nil, failure
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
			if ctx.Err() != nil {
				p.log.Info("openai stream closed", slog.String("run_id", runID), slog.String("path", path), slog.Int("events_emitted", eventsEmitted))
				return
			}
			p.log.Error("openai stream decode failed", slog.String("run_id", runID), slog.String("path", path), slog.String("error", transport.NormalizeError(err, providerName).Error()))
			select {
			case stream <- transport.NewTransportErrorEvent(runID, providerName, err):
			case <-ctx.Done():
			}
			return
		}

		events, stop, err := p.normalizeMessage(runID, message, &state)
		if err != nil {
			p.log.Error("openai event normalization failed", slog.String("run_id", runID), slog.String("path", path), slog.String("error", transport.NormalizeError(err, providerName).Error()))
			select {
			case stream <- transport.NewTransportErrorEvent(runID, providerName, err):
			case <-ctx.Done():
			}
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
	return p.normalizePayload(runID, eventType, payload, state)
}

func (p *Provider) normalizePayload(runID, eventType string, payload map[string]any, state *streamState) ([]transport.TransportEvent, bool, error) {
	switch eventType {
	case "response.created", "response.in_progress":
		if state.started {
			return nil, false, nil
		}
		state.started = true
		state.providerSession = providerSessionFromPayload(payload)
		events := []transport.TransportEvent{transport.NewRunStartedEvent(runID, providerName, p.capabilities, state.providerSession)}
		if state.providerSession != nil {
			events = append(events, transport.NewProviderSessionBoundEvent(runID, providerName, *state.providerSession))
		}
		return events, false, nil
	case "response.output_text.delta":
		delta := stringValue(payload["delta"])
		state.finalText.WriteString(delta)
		return []transport.TransportEvent{transport.NewAssistantDeltaEvent(runID, providerName, transport.AssistantDelta{DeltaType: "text", Content: delta, SequenceNo: intValue(payload["sequence_number"])})}, false, nil
	case "response.output_item.done":
		item := mapValue(payload["item"])
		if stringValue(item["type"]) == "function_call" {
			state.calls = append(state.calls, item)
		}
		return nil, false, nil
	case "response.completed", "response.incomplete", "response.failed":
		response := mapValue(payload["response"])
		usage, err := parseUsage(mapValue(response["usage"]))
		if err != nil {
			return nil, false, err
		}
		ref := providerSessionFromPayload(payload)
		if ref == nil {
			ref = state.providerSession
		}
		if usage != nil && ref != nil && ref.ResponseRef != "" {
			p.mu.Lock()
			if p.prefixTokens[runID] == nil {
				p.prefixTokens[runID] = map[string]int64{}
			}
			p.prefixTokens[runID][ref.ResponseRef] = int64(usage.InputTokens) + int64(usage.OutputTokens)
			p.mu.Unlock()
		}
		if eventType != "response.completed" || stringValue(response["status"]) != "completed" {
			event := transport.NewTransportErrorEvent(runID, providerName, transport.ProviderProtocolError(providerName))
			event.Usage = usage
			event.ProviderSessionRef = ref
			p.clearInstructions(runID)
			return []transport.TransportEvent{event}, true, nil
		}
		calls := state.calls
		if output, ok := response["output"].([]any); ok {
			calls = nil
			for _, raw := range output {
				item := mapValue(raw)
				if stringValue(item["type"]) == "function_call" {
					calls = append(calls, item)
				}
			}
		}
		var proposed []transport.ToolCallRequest
		seen := map[string]bool{}
		for _, item := range calls {
			id, name, args := stringValue(item["call_id"]), stringValue(item["name"]), stringValue(item["arguments"])
			if id == "" || name == "" || seen[id] || !json.Valid([]byte(args)) {
				event := transport.NewTransportErrorEvent(runID, providerName, transport.ProviderProtocolError(providerName))
				event.Usage = usage
				return []transport.TransportEvent{event}, true, nil
			}
			seen[id] = true
			proposed = append(proposed, transport.ToolCallRequest{ToolCallRef: id, ToolName: p.resolveToolName(runID, name), ArgsJSON: args})
		}
		var event transport.TransportEvent
		switch len(proposed) {
		case 0:
			text := extractOutputText(response)
			if text == "" {
				text = state.finalText.String()
			}
			if strings.TrimSpace(text) == "" {
				event = transport.NewTransportErrorEvent(runID, providerName, transport.ProviderProtocolError(providerName))
			} else {
				event = transport.NewAssistantFinalEvent(runID, providerName, transport.AssistantFinal{Content: text, FinishReason: "completed", Usage: usage})
			}
			p.expireInstructions(runID)
		case 1:
			event = transport.NewToolCallRequestedEvent(runID, providerName, proposed[0])
		default:
			event = transport.NewTransportErrorEvent(runID, providerName, &transport.Error{Type: transport.ErrorTypeCapabilityMismatch, ProviderName: providerName, Message: "openai returned unsupported parallel tools", SafeReason: "batch_tool_calls_unsupported"})
		}
		event.Usage = usage
		event.ProviderSessionRef = ref
		if model := stringValue(response["model"]); model != "" {
			event.ProviderModelVersion = &model
		}
		if tier, ok := response["service_tier"].(string); ok {
			switch tier {
			case "default", "priority", "flex", "scale", "ultrafast":
				event.ProviderServiceTier = &tier
			}
		}
		done := transport.NewTerminalEvent(runID, transport.EventTypeRunCompleted, providerName)
		done.ProviderSessionRef = ref
		return []transport.TransportEvent{event, done}, true, nil
	case "error":
		p.clearInstructions(runID)
		return []transport.TransportEvent{transport.NewTransportErrorEvent(runID, providerName, errorFromPayload(payload))}, true, nil
	default:
		// Reasoning, annotations and argument fragments are not executable proposals.
		return nil, false, nil
	}
}

func (p *Provider) startRunBody(req transport.StartRunRequest) ([]byte, error) {
	instructions, input := splitInstructions(req.InputItems)
	payload := map[string]any{
		"model":  req.Context.ModelName,
		"input":  input,
		"stream": req.StreamingEnabled,
	}
	if instructions != "" {
		payload["instructions"] = instructions
		p.setInstructions(req.Context.RunID, instructions)
	} else {
		p.clearInstructions(req.Context.RunID)
	}
	p.rememberTools(req.Context.RunID, req.ToolDefinitions)
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
	return p.encodeRequest(payload)
}

func (p *Provider) continueRunBody(req transport.ContinueRunRequest) ([]byte, error) {
	if req.ToolSet != nil {
		p.rememberTools(req.RunID, req.ToolSet.Definitions)
	}
	instructions, input := splitInstructions(req.InputItems)
	payload := map[string]any{
		"model":  p.config.Model,
		"input":  input,
		"stream": true,
	}
	if instructions == "" {
		instructions = p.instructionsFor(req.RunID)
	} else {
		p.setInstructions(req.RunID, instructions)
	}
	if instructions != "" {
		payload["instructions"] = instructions
	}
	if req.ProviderSessionRef != nil && req.ProviderSessionRef.ResponseRef != "" {
		payload["previous_response_id"] = req.ProviderSessionRef.ResponseRef
	}
	if defs, ok := p.currentTools(req.RunID); ok {
		payload["tools"] = encodeTools(defs)
	}
	if options := mergeOptions(req.TransportOptionsJSON, req.TransportOptions); len(options) > 0 {
		for key, value := range options {
			payload[key] = value
		}
	}
	return p.encodeRequest(payload)
}

func (p *Provider) submitToolResultBody(req transport.SubmitToolResultRequest) ([]byte, error) {
	if req.ToolSet != nil {
		p.rememberTools(req.RunID, req.ToolSet.Definitions)
	}
	input := []map[string]any{{
		"type":    "function_call_output",
		"call_id": req.ToolCallRef,
		"output":  req.ToolResultJSON,
	}}
	payload := map[string]any{
		"model":  p.config.Model,
		"stream": true,
		"input":  input,
	}
	if instructions := p.instructionsFor(req.RunID); instructions != "" {
		payload["instructions"] = instructions
	}
	if req.ProviderSessionRef != nil && req.ProviderSessionRef.ResponseRef != "" {
		payload["previous_response_id"] = req.ProviderSessionRef.ResponseRef
	}
	if defs, ok := p.currentTools(req.RunID); ok {
		payload["tools"] = encodeTools(defs)
	}
	if options := mergeOptions(req.TransportOptionsJSON, req.TransportOptions); len(options) > 0 {
		for key, value := range options {
			payload[key] = value
		}
	}
	return p.encodeRequest(payload)
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
	started         bool
	calls           []map[string]any
	providerSession *transport.ProviderSessionRef
	finalText       strings.Builder
}

func providerSessionFromPayload(payload map[string]any) *transport.ProviderSessionRef {
	session := mapValue(payload["session"])
	sessionID := stringValue(session["id"])
	response := mapValue(payload["response"])
	responseID := stringValue(response["id"])
	if responseID == "" {
		responseID = stringValue(payload["response_id"])
	}
	if responseID == "" && sessionID == "" {
		return nil
	}
	now := time.Now().UTC()
	return &transport.ProviderSessionRef{
		ProviderName: providerName,
		SessionRef:   sessionID,
		ResponseRef:  responseID,
		CreatedAt:    now,
		LastUsedAt:   now,
	}
}

func encodeInputItems(items []transport.InputItem) []map[string]any {
	encoded := make([]map[string]any, 0, len(items))
	for _, item := range items {
		contentType := contentTypeForRole(item.Role)
		entry := map[string]any{
			"role": item.Role,
			"content": []map[string]any{{
				"type": contentType,
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

// contentTypeForRole returns the correct content type for the OpenAI Responses
// API: "input_text" for user/system messages, "output_text" for assistant messages.
func contentTypeForRole(role string) string {
	if strings.EqualFold(strings.TrimSpace(role), "assistant") {
		return "output_text"
	}
	return "input_text"
}

func encodeTools(tools []transport.ToolDefinition) []map[string]any {
	encoded := make([]map[string]any, 0, len(tools))
	for _, tool := range tools {
		entry := map[string]any{
			"type":        "function",
			"name":        toolAlias(tool.Name),
			"strict":      false,
			"description": "Butler tool " + tool.Name + ". " + tool.Description,
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
	return transport.ProviderHTTPError(resp)
}

func errorFromPayload(payload map[string]any) error {
	if status, ok := payload["status"].(float64); ok && status >= 300 && status <= 599 && math.Trunc(status) == status {
		failure := transport.NormalizeError(&transport.HTTPStatusError{StatusCode: int(status)}, providerName)
		failure.HTTPStatus = int(status)
		return failure
	}
	return transport.ProviderProtocolError(providerName)
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

func parseUsage(payload map[string]any) (*transport.Usage, error) {
	if len(payload) == 0 {
		return nil, nil
	}
	count := func(key string) (int, error) {
		n, ok := payload[key].(float64)
		if !ok || math.IsNaN(n) || n < 0 || n >= float64(int(^uint(0)>>1)) || math.Trunc(n) != n {
			return 0, transport.ProviderProtocolError(providerName)
		}
		return int(n), nil
	}
	in, err := count("input_tokens")
	if err != nil {
		return nil, err
	}
	out, err := count("output_tokens")
	if err != nil {
		return nil, err
	}
	total, err := count("total_tokens")
	if err != nil {
		return nil, err
	}
	usage := &transport.Usage{InputTokens: in, OutputTokens: out, TotalTokens: total}
	if usage.Validate() != nil || total-in != out {
		return nil, transport.ProviderProtocolError(providerName)
	}
	for _, spec := range []struct {
		parent, key string
		dst         **int
		max         int
	}{{"input_tokens_details", "cached_tokens", &usage.CachedInputTokens, in}, {"output_tokens_details", "reasoning_tokens", &usage.OutputReasoningTokens, out}} {
		if v, ok := mapValue(payload[spec.parent])[spec.key]; ok {
			n, ok := v.(float64)
			if !ok || n < 0 || n > float64(spec.max) || math.Trunc(n) != n {
				return nil, transport.ProviderProtocolError(providerName)
			}
			value := int(n)
			*spec.dst = &value
		}
	}
	details := mapValue(payload["input_tokens_details"])
	writes := mapValue(details["cache_write_tokens_details"])
	for _, spec := range []struct {
		value any
		dst   **int
	}{{details["cache_write_tokens"], &usage.CacheWriteInputTokens}, {writes["ttl_30m_tokens"], &usage.CacheWrite30mTokens}, {writes["ttl_12h_tokens"], &usage.CacheWrite12hTokens}} {
		if spec.value == nil {
			continue
		}
		n, ok := spec.value.(float64)
		if !ok || n < 0 || n > float64(in) || math.Trunc(n) != n {
			return nil, transport.ProviderProtocolError(providerName)
		}
		value := int(n)
		*spec.dst = &value
	}
	if usage.Validate() != nil {
		return nil, transport.ProviderProtocolError(providerName)
	}
	return usage, nil
}

func mergeOptions(raw string, options map[string]any) map[string]any {
	merged := make(map[string]any)
	if strings.TrimSpace(raw) != "" {
		_ = json.Unmarshal([]byte(raw), &merged)
	}
	for key, value := range options {
		merged[key] = value
	}
	for key := range merged {
		switch key {
		case "reasoning", "text", "tool_choice", "parallel_tool_calls", "max_output_tokens", "service_tier", "store":
		default:
			delete(merged, key)
		}
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

func splitInstructions(items []transport.InputItem) (string, []map[string]any) {
	instructions, input := splitInstructionItems(items)
	return instructions, encodeInputItems(input)
}

func (p *Provider) setInstructions(runID, instructions string) {
	runID = strings.TrimSpace(runID)
	if runID == "" {
		return
	}
	trimmed := strings.TrimSpace(instructions)
	p.mu.Lock()
	defer p.mu.Unlock()
	if trimmed == "" {
		delete(p.instructions, runID)
		return
	}
	p.instructions[runID] = trimmed
}

func (p *Provider) instructionsFor(runID string) string {
	runID = strings.TrimSpace(runID)
	if runID == "" {
		return ""
	}
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.instructions[runID]
}

func (p *Provider) clearInstructions(runID string) {
	runID = strings.TrimSpace(runID)
	if runID == "" {
		return
	}
	p.mu.Lock()
	defer p.mu.Unlock()
	delete(p.toolSets, runID)
	delete(p.instructions, runID)
	delete(p.aliases, runID)
	delete(p.prefixTokens, runID)
	if timer := p.expiry[runID]; timer != nil {
		timer.Stop()
		delete(p.expiry, runID)
	}
}
