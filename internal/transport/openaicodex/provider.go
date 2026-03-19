package openaicodex

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net"
	"net/http"
	"regexp"
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
	config        Config
	httpClient    *http.Client
	capabilities  transport.CapabilitySnapshot
	log           *slog.Logger
	mu            sync.Mutex
	active        map[string]context.CancelFunc
	toolAliases   map[string]map[string]string
	instructions  map[string]string
	lastToolCalls map[string]lastToolCall // runID -> last emitted tool call
}

// lastToolCall stores the most recently emitted tool call for a run so the
// stateless Codex API can receive the function_call item alongside the
// function_call_output when submitting tool results.
type lastToolCall struct {
	CallID    string
	Name      string
	Arguments string
}

var invalidToolNameChars = regexp.MustCompile(`[^a-zA-Z0-9_-]+`)

type streamState struct {
	providerSession  *transport.ProviderSessionRef
	finalText        strings.Builder
	sessionID        string
	pendingToolCalls map[string]pendingToolCall
}

type pendingToolCall struct {
	ID        string
	CallID    string
	Name      string
	Arguments string
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
		client = newStreamingHTTPClient(cfg.Timeout)
	}
	log := cfg.Logger
	if log == nil {
		log = slog.Default()
	}
	return &Provider{
		config:        cfg,
		httpClient:    client,
		log:           logger.WithComponent(log, "transport-openai-codex"),
		active:        make(map[string]context.CancelFunc),
		toolAliases:   make(map[string]map[string]string),
		instructions:  make(map[string]string),
		lastToolCalls: make(map[string]lastToolCall),
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

func newStreamingHTTPClient(responseHeaderTimeout time.Duration) *http.Client {
	transport := http.DefaultTransport.(*http.Transport).Clone()
	transport.ResponseHeaderTimeout = responseHeaderTimeout
	transport.DialContext = (&net.Dialer{Timeout: 30 * time.Second, KeepAlive: 30 * time.Second}).DialContext
	return &http.Client{Transport: transport}
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
	p.registerToolAliases(req.Context.RunID, req.ToolDefinitions)
	p.storeInstructions(req.Context.RunID, req.InputItems)
	body, err := p.startRunBody(req)
	if err != nil {
		p.clearRunState(req.Context.RunID)
		return nil, err
	}
	return p.executeStreamingRequest(ctx, req.Context.RunID, sessionIDFromStart(req), body)
}

func (p *Provider) ContinueRun(ctx context.Context, req transport.ContinueRunRequest) (transport.EventStream, error) {
	if err := req.Validate(); err != nil {
		return nil, err
	}
	p.storeInstructions(req.RunID, req.InputItems)
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
	p.clearRunState(req.RunID)
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
	case "response.output_item.added":
		item := mapValue(payload["item"])
		if stringValue(item["type"]) != "function_call" {
			return nil, false, nil
		}
		state.storePendingToolCall(pendingToolCall{
			ID:        stringValue(item["id"]),
			CallID:    stringValue(item["call_id"]),
			Name:      stringValue(item["name"]),
			Arguments: coalesceJSONString(item["arguments"], ""),
		})
		return nil, false, nil
	case "response.function_call_arguments.delta":
		return nil, false, nil
	case "response.output_item.done":
		item := mapValue(payload["item"])
		if stringValue(item["type"]) != "function_call" {
			return nil, false, nil
		}
		pending := pendingToolCall{
			ID:        stringValue(item["id"]),
			CallID:    stringValue(item["call_id"]),
			Name:      stringValue(item["name"]),
			Arguments: coalesceJSONString(item["arguments"], ""),
		}
		state.storePendingToolCall(pending)
		callID := pending.CallID
		if callID == "" {
			callID = pending.ID
		}
		argsJSON := coalesceJSONString(item["arguments"], "{}")
		p.storeLastToolCall(runID, lastToolCall{CallID: callID, Name: pending.Name, Arguments: argsJSON})
		return []transport.TransportEvent{transport.NewToolCallRequestedEvent(runID, providerName, transport.ToolCallRequest{ToolCallRef: callID, ToolName: p.resolveToolName(runID, pending.Name), ArgsJSON: argsJSON, SequenceNo: intValue(payload["sequence_number"])})}, true, nil
	case "response.function_call_arguments.done":
		pending := state.pendingToolCall(stringValue(payload["item_id"]))
		callID := stringValue(payload["call_id"])
		if callID == "" {
			callID = pending.CallID
		}
		if callID == "" {
			callID = pending.ID
		}
		name := stringValue(payload["name"])
		if name == "" {
			name = pending.Name
		}
		argsJSON := coalesceJSONString(payload["arguments"], pending.Arguments)
		if strings.TrimSpace(argsJSON) == "" {
			argsJSON = "{}"
		}
		p.storeLastToolCall(runID, lastToolCall{CallID: callID, Name: name, Arguments: argsJSON})
		return []transport.TransportEvent{transport.NewToolCallRequestedEvent(runID, providerName, transport.ToolCallRequest{ToolCallRef: callID, ToolName: p.resolveToolName(runID, name), ArgsJSON: argsJSON, SequenceNo: intValue(payload["sequence_number"])})}, true, nil
	case "response.completed", "response.done":
		defer p.clearRunState(runID)
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
		defer p.clearRunState(runID)
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
	instructions, input := splitInstructions(req.InputItems)
	payload := map[string]any{"model": req.Context.ModelName, "input": input, "stream": req.StreamingEnabled, "store": false}
	if instructions != "" {
		payload["instructions"] = instructions
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
	instructions, input := splitInstructions(req.InputItems)
	payload := map[string]any{"model": p.config.Model, "input": input, "stream": true, "store": false}
	if instructions != "" {
		payload["instructions"] = instructions
	}
	if sessionID := sessionIDFromRef(req.ProviderSessionRef); sessionID != "" {
		payload["prompt_cache_key"] = sessionID
	}
	mergeOptions(payload, req.TransportOptionsJSON, req.TransportOptions)
	return json.Marshal(payload)
}

func (p *Provider) submitToolResultBody(req transport.SubmitToolResultRequest) ([]byte, error) {
	// The Codex API does not support previous_response_id, so each request is
	// stateless.  We replay the function_call item alongside the
	// function_call_output so the API can match the result to the call.
	input := make([]map[string]any, 0, 2)
	if call, ok := p.lastToolCallFor(req.RunID); ok && call.CallID == req.ToolCallRef {
		input = append(input, map[string]any{
			"type":      "function_call",
			"call_id":   call.CallID,
			"name":      call.Name,
			"arguments": call.Arguments,
		})
	}
	input = append(input, map[string]any{
		"type":    "function_call_output",
		"call_id": req.ToolCallRef,
		"output":  strings.TrimSpace(req.ToolResultJSON),
	})
	payload := map[string]any{
		"model":  p.config.Model,
		"store":  false,
		"stream": true,
		"input":  input,
	}
	if instructions := p.instructionsFor(req.RunID); instructions != "" {
		payload["instructions"] = instructions
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

func (s *streamState) storePendingToolCall(call pendingToolCall) {
	if strings.TrimSpace(call.ID) == "" {
		return
	}
	if s.pendingToolCalls == nil {
		s.pendingToolCalls = make(map[string]pendingToolCall)
	}
	existing := s.pendingToolCalls[call.ID]
	if strings.TrimSpace(call.CallID) == "" {
		call.CallID = existing.CallID
	}
	if strings.TrimSpace(call.Name) == "" {
		call.Name = existing.Name
	}
	if strings.TrimSpace(call.Arguments) == "" {
		call.Arguments = existing.Arguments
	}
	s.pendingToolCalls[call.ID] = call
}

func (s *streamState) pendingToolCall(itemID string) pendingToolCall {
	if s.pendingToolCalls == nil {
		return pendingToolCall{}
	}
	return s.pendingToolCalls[strings.TrimSpace(itemID)]
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
		entry := map[string]any{"role": item.Role, "content": []map[string]any{{"type": contentTypeForRole(item.Role), "text": item.Content}}}
		if strings.TrimSpace(item.Name) != "" {
			entry["name"] = item.Name
		}
		encoded = append(encoded, entry)
	}
	return encoded
}

func splitInstructions(items []transport.InputItem) (string, []map[string]any) {
	encoded := make([]map[string]any, 0, len(items))
	instructions := make([]string, 0, 1)
	for _, item := range items {
		if strings.EqualFold(item.Role, "system") {
			content := strings.TrimSpace(item.Content)
			if content != "" {
				instructions = append(instructions, content)
			}
			continue
		}
		entry := map[string]any{"role": item.Role, "content": []map[string]any{{"type": contentTypeForRole(item.Role), "text": item.Content}}}
		if strings.TrimSpace(item.Name) != "" {
			entry["name"] = item.Name
		}
		encoded = append(encoded, entry)
	}
	return strings.Join(instructions, "\n\n"), encoded
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
	aliases := buildToolAliases(tools)
	encoded := make([]map[string]any, 0, len(tools))
	for _, tool := range tools {
		entry := map[string]any{"type": "function", "name": aliases[tool.Name], "description": tool.Description}
		if strings.TrimSpace(tool.SchemaJSON) != "" {
			var schema any
			if err := json.Unmarshal([]byte(tool.SchemaJSON), &schema); err == nil {
				normalizeSchema(schema)
				entry["parameters"] = schema
			}
		}
		encoded = append(encoded, entry)
	}
	return encoded
}

func (p *Provider) registerToolAliases(runID string, tools []transport.ToolDefinition) {
	if strings.TrimSpace(runID) == "" {
		return
	}
	aliases := buildToolAliases(tools)
	p.mu.Lock()
	defer p.mu.Unlock()
	if len(aliases) == 0 {
		delete(p.toolAliases, runID)
		return
	}
	reverse := make(map[string]string, len(aliases))
	for original, alias := range aliases {
		reverse[alias] = original
	}
	p.toolAliases[runID] = reverse
}

func (p *Provider) clearToolAliases(runID string) {
	p.mu.Lock()
	defer p.mu.Unlock()
	delete(p.toolAliases, runID)
}

func (p *Provider) storeInstructions(runID string, items []transport.InputItem) {
	if strings.TrimSpace(runID) == "" {
		return
	}
	instructions, _ := splitInstructions(items)
	if instructions == "" {
		return
	}
	p.mu.Lock()
	defer p.mu.Unlock()
	p.instructions[runID] = instructions
}

func (p *Provider) instructionsFor(runID string) string {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.instructions[runID]
}

func (p *Provider) clearRunState(runID string) {
	p.mu.Lock()
	defer p.mu.Unlock()
	delete(p.toolAliases, runID)
	delete(p.instructions, runID)
	delete(p.lastToolCalls, runID)
}

func (p *Provider) storeLastToolCall(runID string, call lastToolCall) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.lastToolCalls[runID] = call
}

func (p *Provider) lastToolCallFor(runID string) (lastToolCall, bool) {
	p.mu.Lock()
	defer p.mu.Unlock()
	call, ok := p.lastToolCalls[runID]
	return call, ok
}

func (p *Provider) resolveToolName(runID, name string) string {
	p.mu.Lock()
	defer p.mu.Unlock()
	if aliases := p.toolAliases[runID]; aliases != nil {
		if original := aliases[name]; original != "" {
			return original
		}
	}
	return name
}

func buildToolAliases(tools []transport.ToolDefinition) map[string]string {
	if len(tools) == 0 {
		return nil
	}
	aliases := make(map[string]string, len(tools))
	used := make(map[string]struct{}, len(tools))
	for _, tool := range tools {
		name := strings.TrimSpace(tool.Name)
		if name == "" {
			continue
		}
		alias := sanitizeToolName(name)
		base := alias
		idx := 2
		for {
			if _, exists := used[alias]; !exists {
				break
			}
			alias = fmt.Sprintf("%s_%d", base, idx)
			idx++
		}
		used[alias] = struct{}{}
		aliases[name] = alias
	}
	return aliases
}

func sanitizeToolName(name string) string {
	cleaned := invalidToolNameChars.ReplaceAllString(strings.TrimSpace(name), "_")
	cleaned = strings.Trim(cleaned, "_")
	if cleaned == "" {
		return "tool"
	}
	return cleaned
}

func normalizeSchema(value any) {
	switch node := value.(type) {
	case map[string]any:
		if strings.EqualFold(stringValue(node["type"]), "array") {
			if _, ok := node["items"]; !ok {
				node["items"] = map[string]any{}
			}
		}
		if strings.EqualFold(stringValue(node["type"]), "object") {
			if _, ok := node["properties"]; !ok {
				node["properties"] = map[string]any{}
			}
		}
		for _, child := range node {
			normalizeSchema(child)
		}
	case []any:
		for _, child := range node {
			normalizeSchema(child)
		}
	}
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
	if value == nil {
		return fallback
	}
	if text, ok := value.(string); ok {
		trimmed := strings.TrimSpace(text)
		if trimmed == "" {
			return fallback
		}
		if json.Valid([]byte(trimmed)) {
			return trimmed
		}
		encoded, err := json.Marshal(trimmed)
		if err != nil {
			return fallback
		}
		return string(encoded)
	}
	encoded, err := json.Marshal(value)
	if err != nil {
		return fallback
	}
	trimmed := strings.TrimSpace(string(encoded))
	if trimmed == "" {
		return fallback
	}
	return trimmed
}
