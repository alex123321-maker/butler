package githubcopilot

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

const providerName = "github-copilot"

type Config struct {
	Model      string
	Timeout    time.Duration
	Logger     *slog.Logger
	AuthSource providerauth.GitHubCopilotTokenSource
}

type Provider struct {
	config       Config
	httpClient   *http.Client
	capabilities transport.CapabilitySnapshot
	log          *slog.Logger
	mu           sync.Mutex
	sessions     map[string]*conversationSession
	active       map[string]context.CancelFunc
}

type conversationSession struct {
	ID               string
	Model            string
	Messages         []chatMessage
	Tools            []transport.ToolDefinition
	PendingToolCalls []transport.ToolCallRequest
	CreatedAt        time.Time
	LastUsedAt       time.Time
}

type chatMessage struct {
	Role      string
	Content   string
	ToolCalls []toolCall
	ToolCall  string
}

type toolCall struct {
	ID            string
	Name          string
	ArgumentsJSON string
}

type sseDecoder struct{ scanner *bufio.Scanner }
type sseMessage struct{ Data string }

type streamState struct {
	sessionID string
	text      strings.Builder
	toolCalls map[int]*toolCall
	finish    string
	usage     *transport.Usage
}

var copilotHeaders = map[string]string{
	"User-Agent":             "GitHubCopilotChat/0.35.0",
	"Editor-Version":         "vscode/1.107.0",
	"Editor-Plugin-Version":  "copilot-chat/0.35.0",
	"Copilot-Integration-Id": "vscode-chat",
	"Openai-Intent":          "conversation-edits",
}

func init() {
	transport.RegisterProvider(providerName, func(raw any) (transport.ModelProvider, error) {
		cfg, ok := raw.(Config)
		if !ok {
			return nil, fmt.Errorf("github-copilot: expected Config, got %T", raw)
		}
		return NewProvider(cfg, nil)
	})
}

func NewProvider(cfg Config, client *http.Client) (*Provider, error) {
	if strings.TrimSpace(cfg.Model) == "" {
		cfg.Model = "gpt-4o"
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
		log:        logger.WithComponent(log, "transport-github-copilot"),
		sessions:   make(map[string]*conversationSession),
		active:     make(map[string]context.CancelFunc),
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

func (p *Provider) Name() string { return providerName }

func (p *Provider) Capabilities(_ context.Context, _ transport.TransportRunContext) (transport.CapabilitySnapshot, error) {
	return p.capabilities, nil
}

func (p *Provider) StartRun(ctx context.Context, req transport.StartRunRequest) (transport.EventStream, error) {
	if err := req.Validate(); err != nil {
		return nil, err
	}
	sessionID := strings.TrimSpace(req.Context.SessionKey)
	if sessionID == "" {
		sessionID = req.Context.RunID
	}
	session := p.session(sessionID)
	session.Model = req.Context.ModelName
	session.Tools = append([]transport.ToolDefinition(nil), req.ToolDefinitions...)
	p.appendInputItems(session, req.InputItems)
	return p.execute(ctx, req.Context.RunID, session, "user")
}

func (p *Provider) ContinueRun(ctx context.Context, req transport.ContinueRunRequest) (transport.EventStream, error) {
	if err := req.Validate(); err != nil {
		return nil, err
	}
	session, err := p.existingSession(sessionIDFromRef(req.ProviderSessionRef))
	if err != nil {
		return nil, err
	}
	p.appendInputItems(session, req.InputItems)
	return p.execute(ctx, req.RunID, session, "agent")
}

func (p *Provider) SubmitToolResult(ctx context.Context, req transport.SubmitToolResultRequest) (transport.EventStream, error) {
	if err := req.Validate(); err != nil {
		return nil, err
	}
	session, err := p.existingSession(sessionIDFromRef(req.ProviderSessionRef))
	if err != nil {
		return nil, err
	}
	session.Messages = append(session.Messages, chatMessage{Role: "tool", Content: req.ToolResultJSON, ToolCall: req.ToolCallRef})
	if len(session.PendingToolCalls) > 0 {
		next := session.PendingToolCalls[0]
		session.PendingToolCalls = session.PendingToolCalls[1:]
		return localToolCallStream(req.RunID, session.ID, next), nil
	}
	return p.execute(ctx, req.RunID, session, "agent")
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

func (p *Provider) execute(parent context.Context, runID string, session *conversationSession, initiator string) (transport.EventStream, error) {
	if session == nil {
		return nil, fmt.Errorf("github copilot session is not available")
	}
	if p.config.AuthSource == nil {
		return nil, fmt.Errorf("github copilot auth source is not configured")
	}
	auth, err := p.config.AuthSource.ResolveGitHubCopilot(parent)
	if err != nil {
		return nil, transport.NormalizeError(err, providerName)
	}
	body, err := p.requestBody(session)
	if err != nil {
		return nil, err
	}
	ctx, cancel := context.WithCancel(parent)
	p.registerCancel(runID, cancel)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, strings.TrimRight(auth.BaseURL, "/")+"/chat/completions", bytes.NewReader(body))
	if err != nil {
		p.clearCancel(runID)
		cancel()
		return nil, transport.NormalizeError(err, providerName)
	}
	req.Header.Set("Authorization", "Bearer "+auth.AccessToken)
	req.Header.Set("Accept", "text/event-stream")
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Initiator", initiator)
	for key, value := range copilotHeaders {
		if req.Header.Get(key) == "" {
			req.Header.Set(key, value)
		}
	}
	resp, err := p.httpClient.Do(req)
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
	go p.streamResponse(ctx, runID, session, resp.Body, stream)
	return stream, nil
}

func (p *Provider) streamResponse(ctx context.Context, runID string, session *conversationSession, body io.ReadCloser, stream chan<- transport.TransportEvent) {
	defer close(stream)
	defer body.Close()
	defer p.clearCancel(runID)

	ref := &transport.ProviderSessionRef{ProviderName: providerName, SessionRef: session.ID, CreatedAt: session.CreatedAt, LastUsedAt: time.Now().UTC()}
	stream <- transport.NewRunStartedEvent(runID, providerName, p.capabilities, ref)
	stream <- transport.NewProviderSessionBoundEvent(runID, providerName, *ref)

	decoder := newSSEDecoder(body)
	state := streamState{sessionID: session.ID, toolCalls: make(map[int]*toolCall)}
	for {
		message, err := decoder.Next(ctx)
		if err != nil {
			if err == io.EOF || ctx.Err() != nil {
				return
			}
			stream <- transport.NewTransportErrorEvent(runID, providerName, err)
			return
		}
		stop, err := p.handleChunk(runID, session, stateRef(ref, &state), message, stream)
		if err != nil {
			stream <- transport.NewTransportErrorEvent(runID, providerName, err)
			return
		}
		if stop {
			return
		}
	}
}

func (p *Provider) handleChunk(runID string, session *conversationSession, state *streamState, message sseMessage, stream chan<- transport.TransportEvent) (bool, error) {
	trimmed := strings.TrimSpace(message.Data)
	if trimmed == "" || trimmed == "[DONE]" {
		return trimmed == "[DONE]", nil
	}
	var payload map[string]any
	if err := json.Unmarshal([]byte(trimmed), &payload); err != nil {
		return false, err
	}
	if usage := mapValue(payload["usage"]); len(usage) > 0 {
		state.usage = &transport.Usage{InputTokens: intValue(usage["prompt_tokens"]), OutputTokens: intValue(usage["completion_tokens"]), TotalTokens: intValue(usage["total_tokens"])}
	}
	choices, _ := payload["choices"].([]any)
	if len(choices) == 0 {
		return false, nil
	}
	choice := mapValue(choices[0])
	delta := mapValue(choice["delta"])
	if content := stringValue(delta["content"]); content != "" {
		state.text.WriteString(content)
		stream <- transport.NewAssistantDeltaEvent(runID, providerName, transport.AssistantDelta{DeltaType: "text", Content: content, SequenceNo: len(state.text.String())})
	}
	if rawToolCalls, ok := delta["tool_calls"].([]any); ok {
		for _, raw := range rawToolCalls {
			call := mapValue(raw)
			index := intValue(call["index"])
			stateCall := state.toolCalls[index]
			if stateCall == nil {
				stateCall = &toolCall{}
				state.toolCalls[index] = stateCall
			}
			if id := stringValue(call["id"]); id != "" {
				stateCall.ID = id
			}
			function := mapValue(call["function"])
			if name := stringValue(function["name"]); name != "" {
				stateCall.Name = name
			}
			if args := stringValue(function["arguments"]); args != "" {
				stateCall.ArgumentsJSON += args
			}
		}
	}
	finishReason := stringValue(choice["finish_reason"])
	if finishReason == "" {
		return false, nil
	}
	assistantText := strings.TrimSpace(state.text.String())
	if len(state.toolCalls) > 0 || finishReason == "tool_calls" {
		calls := flattenToolCalls(state.toolCalls)
		toolDefs := make([]toolCall, 0, len(calls))
		pending := make([]transport.ToolCallRequest, 0, len(calls))
		for i, call := range calls {
			args := coalesceJSONString(call.ArgumentsJSON, "{}")
			toolDefs = append(toolDefs, toolCall{ID: call.ID, Name: call.Name, ArgumentsJSON: args})
			pending = append(pending, transport.ToolCallRequest{ToolCallRef: call.ID, ToolName: call.Name, ArgsJSON: args, SequenceNo: i + 1})
		}
		session.Messages = append(session.Messages, chatMessage{Role: "assistant", Content: assistantText, ToolCalls: toolDefs})
		session.PendingToolCalls = append(session.PendingToolCalls, pending...)
		session.LastUsedAt = time.Now().UTC()
		first := session.PendingToolCalls[0]
		session.PendingToolCalls = session.PendingToolCalls[1:]
		stream <- transport.NewToolCallRequestedEvent(runID, providerName, first)
		return true, nil
	}
	session.Messages = append(session.Messages, chatMessage{Role: "assistant", Content: assistantText})
	session.LastUsedAt = time.Now().UTC()
	final := transport.NewAssistantFinalEvent(runID, providerName, transport.AssistantFinal{Content: assistantText, FinishReason: finishReason, Usage: state.usage})
	final.ProviderSessionRef = &transport.ProviderSessionRef{ProviderName: providerName, SessionRef: session.ID, CreatedAt: session.CreatedAt, LastUsedAt: session.LastUsedAt}
	stream <- final
	completed := transport.NewTerminalEvent(runID, transport.EventTypeRunCompleted, providerName)
	completed.ProviderSessionRef = final.ProviderSessionRef
	stream <- completed
	return true, nil
}

func (p *Provider) requestBody(session *conversationSession) ([]byte, error) {
	payload := map[string]any{
		"model":          firstNonEmpty(session.Model, p.config.Model),
		"stream":         true,
		"messages":       encodeMessages(session.Messages),
		"stream_options": map[string]any{"include_usage": true},
	}
	if len(session.Tools) > 0 {
		payload["tools"] = encodeTools(session.Tools)
		payload["tool_choice"] = "auto"
	}
	return json.Marshal(payload)
}

func (p *Provider) session(sessionID string) *conversationSession {
	p.mu.Lock()
	defer p.mu.Unlock()
	session := p.sessions[sessionID]
	if session != nil {
		session.LastUsedAt = time.Now().UTC()
		return session
	}
	now := time.Now().UTC()
	session = &conversationSession{ID: sessionID, CreatedAt: now, LastUsedAt: now}
	p.sessions[sessionID] = session
	return session
}

func (p *Provider) existingSession(sessionID string) (*conversationSession, error) {
	if strings.TrimSpace(sessionID) == "" {
		return nil, fmt.Errorf("provider session ref is required for github copilot continuation")
	}
	p.mu.Lock()
	defer p.mu.Unlock()
	session := p.sessions[sessionID]
	if session == nil {
		return nil, fmt.Errorf("github copilot session %s was not found", sessionID)
	}
	session.LastUsedAt = time.Now().UTC()
	return session, nil
}

func (p *Provider) appendInputItems(session *conversationSession, items []transport.InputItem) {
	for _, item := range items {
		role := strings.TrimSpace(item.Role)
		if role == "" {
			role = "user"
		}
		session.Messages = append(session.Messages, chatMessage{Role: role, Content: item.Content})
	}
	session.LastUsedAt = time.Now().UTC()
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

func localToolCallStream(runID, sessionID string, call transport.ToolCallRequest) transport.EventStream {
	stream := make(chan transport.TransportEvent, 2)
	go func() {
		defer close(stream)
		stream <- transport.NewProviderSessionBoundEvent(runID, providerName, transport.ProviderSessionRef{ProviderName: providerName, SessionRef: sessionID, CreatedAt: time.Now().UTC(), LastUsedAt: time.Now().UTC()})
		stream <- transport.NewToolCallRequestedEvent(runID, providerName, call)
	}()
	return stream
}

func newSSEDecoder(reader io.Reader) *sseDecoder {
	scanner := bufio.NewScanner(reader)
	scanner.Buffer(make([]byte, 0, 64*1024), 1024*1024)
	return &sseDecoder{scanner: scanner}
}

func (d *sseDecoder) Next(ctx context.Context) (sseMessage, error) {
	var dataLines []string
	for d.scanner.Scan() {
		select {
		case <-ctx.Done():
			return sseMessage{}, ctx.Err()
		default:
		}
		line := d.scanner.Text()
		if line == "" {
			if len(dataLines) == 0 {
				continue
			}
			return sseMessage{Data: strings.Join(dataLines, "\n")}, nil
		}
		if strings.HasPrefix(line, "data:") {
			dataLines = append(dataLines, strings.TrimSpace(strings.TrimPrefix(line, "data:")))
		}
	}
	if err := d.scanner.Err(); err != nil {
		return sseMessage{}, err
	}
	if len(dataLines) > 0 {
		return sseMessage{Data: strings.Join(dataLines, "\n")}, nil
	}
	return sseMessage{}, io.EOF
}

func encodeMessages(messages []chatMessage) []map[string]any {
	encoded := make([]map[string]any, 0, len(messages))
	for _, message := range messages {
		switch message.Role {
		case "assistant":
			entry := map[string]any{"role": "assistant", "content": message.Content}
			if len(message.ToolCalls) > 0 {
				toolCalls := make([]map[string]any, 0, len(message.ToolCalls))
				for _, call := range message.ToolCalls {
					toolCalls = append(toolCalls, map[string]any{"id": call.ID, "type": "function", "function": map[string]any{"name": call.Name, "arguments": call.ArgumentsJSON}})
				}
				entry["tool_calls"] = toolCalls
			}
			encoded = append(encoded, entry)
		case "tool":
			encoded = append(encoded, map[string]any{"role": "tool", "content": message.Content, "tool_call_id": message.ToolCall})
		default:
			encoded = append(encoded, map[string]any{"role": message.Role, "content": message.Content})
		}
	}
	return encoded
}

func encodeTools(tools []transport.ToolDefinition) []map[string]any {
	encoded := make([]map[string]any, 0, len(tools))
	for _, tool := range tools {
		entry := map[string]any{"type": "function", "function": map[string]any{"name": tool.Name, "description": tool.Description}}
		if strings.TrimSpace(tool.SchemaJSON) != "" {
			var schema any
			if err := json.Unmarshal([]byte(tool.SchemaJSON), &schema); err == nil {
				entry["function"].(map[string]any)["parameters"] = schema
			}
		}
		encoded = append(encoded, entry)
	}
	return encoded
}

func flattenToolCalls(values map[int]*toolCall) []*toolCall {
	indices := make([]int, 0, len(values))
	for index := range values {
		indices = append(indices, index)
	}
	for i := 0; i < len(indices); i++ {
		for j := i + 1; j < len(indices); j++ {
			if indices[j] < indices[i] {
				indices[i], indices[j] = indices[j], indices[i]
			}
		}
	}
	ordered := make([]*toolCall, 0, len(indices))
	for _, index := range indices {
		ordered = append(ordered, values[index])
	}
	return ordered
}

func stateRef(ref *transport.ProviderSessionRef, state *streamState) *streamState {
	_ = ref
	return state
}

func sessionIDFromRef(ref *transport.ProviderSessionRef) string {
	if ref == nil {
		return ""
	}
	return strings.TrimSpace(ref.SessionRef)
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
	default:
		return 0
	}
}

func coalesceJSONString(value, fallback string) string {
	trimmed := strings.TrimSpace(value)
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

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if trimmed := strings.TrimSpace(value); trimmed != "" {
			return trimmed
		}
	}
	return ""
}
