package openai

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"

	"github.com/butler/butler/internal/transport"
	"github.com/gorilla/websocket"
)

// realtimeSession represents a persistent WebSocket connection to the
// OpenAI Realtime API. Sessions are keyed by session key (e.g.
// "telegram:chat:123") and can be reused across multiple runs.
type realtimeSession struct {
	conn       *websocket.Conn
	sessionKey string
	sessionRef string // provider-side session ID (from session.created)
	mu         sync.Mutex
	createdAt  time.Time
	lastUsedAt time.Time
	closed     bool
}

func deriveRealtimeURL(baseURL string) string {
	trimmed := strings.TrimRight(strings.TrimSpace(baseURL), "/")
	if trimmed == "" {
		return "wss://api.openai.com/v1/realtime"
	}
	if strings.HasPrefix(trimmed, "wss://") || strings.HasPrefix(trimmed, "ws://") {
		if strings.HasSuffix(trimmed, "/realtime") {
			return trimmed
		}
		return trimmed + "/realtime"
	}
	if strings.HasPrefix(trimmed, "https://") {
		trimmed = "wss://" + strings.TrimPrefix(trimmed, "https://")
	} else if strings.HasPrefix(trimmed, "http://") {
		trimmed = "ws://" + strings.TrimPrefix(trimmed, "http://")
	}
	if strings.HasSuffix(trimmed, "/v1") {
		return trimmed + "/realtime"
	}
	if strings.HasSuffix(trimmed, "/realtime") {
		return trimmed
	}
	return trimmed + "/realtime"
}

// pingInterval controls the WebSocket keepalive ping frequency.
const pingInterval = 30 * time.Second

// pongDeadline is the time allowed for a pong response before the
// connection is considered dead.
const pongDeadline = 10 * time.Second

func (p *Provider) executeRunRequest(ctx context.Context, runID, modelName string, sseBody []byte, realtimeMessages func(context.Context) ([]map[string]any, error)) (transport.EventStream, error) {
	if !p.prefersWebSocket() {
		return p.executeStreamingRequest(ctx, runID, http.MethodPost, p.endpoint("/responses"), sseBody)
	}

	messages, err := realtimeMessages(ctx)
	if err != nil {
		return nil, err
	}
	wsStream, err := p.executeRealtimeRequest(ctx, runID, modelName, messages)
	if err != nil {
		return p.fallbackToSSE(ctx, runID, sseBody, err)
	}
	return p.wrapRealtimeFallback(ctx, runID, wsStream, sseBody), nil
}

func (p *Provider) fallbackToSSE(ctx context.Context, runID string, sseBody []byte, cause error) (transport.EventStream, error) {
	p.log.Warn("openai websocket unavailable, falling back to sse",
		slog.String("run_id", runID),
		slog.String("error", cause.Error()),
	)
	stream, err := p.executeStreamingRequest(ctx, runID, http.MethodPost, p.endpoint("/responses"), sseBody)
	if err != nil {
		return nil, err
	}
	warning := transport.NewTransportWarningEvent(runID, providerName, "openai websocket unavailable; falling back to http sse", map[string]any{"cause": cause.Error()})
	return prependEventStream(ctx, warning, stream), nil
}

func (p *Provider) wrapRealtimeFallback(ctx context.Context, runID string, wsStream transport.EventStream, sseBody []byte) transport.EventStream {
	stream := make(chan transport.TransportEvent, 32)
	go func() {
		defer close(stream)
		meaningfulEvents := 0
		for event := range wsStream {
			if event.EventType != transport.EventTypeTransportWarning && event.EventType != transport.EventTypeProviderSessionBound && event.EventType != transport.EventTypeRunStarted {
				meaningfulEvents++
			}
			if event.EventType == transport.EventTypeTransportError && meaningfulEvents == 0 {
				fallback, err := p.fallbackToSSE(ctx, runID, sseBody, event.TransportError)
				if err == nil {
					for fallbackEvent := range fallback {
						select {
						case <-ctx.Done():
							return
						case stream <- fallbackEvent:
						}
					}
					return
				}
			}
			select {
			case <-ctx.Done():
				return
			case stream <- event:
			}
		}
	}()
	return stream
}

func prependEventStream(ctx context.Context, first transport.TransportEvent, remainder transport.EventStream) transport.EventStream {
	stream := make(chan transport.TransportEvent, 32)
	go func() {
		defer close(stream)
		select {
		case <-ctx.Done():
			return
		case stream <- first:
		}
		for event := range remainder {
			select {
			case <-ctx.Done():
				return
			case stream <- event:
			}
		}
	}()
	return stream
}

func (p *Provider) executeRealtimeRequest(ctx context.Context, runID, modelName string, messages []map[string]any) (transport.EventStream, error) {
	sessionKey := p.sessionKeyForRun(ctx, runID)

	// Try to reuse an existing session for this session key.
	session := p.getSessionByKey(sessionKey)
	if session != nil {
		p.log.Info("reusing existing openai realtime session",
			slog.String("run_id", runID),
			slog.String("session_key", sessionKey),
			slog.String("session_ref", session.sessionRef),
			slog.String("session_age", time.Since(session.createdAt).Round(time.Second).String()),
		)
		if err := p.sendMessagesToSession(session, messages); err != nil {
			// Connection is dead — remove and fall through to create a new one.
			p.log.Warn("openai realtime session send failed, creating new session",
				slog.String("run_id", runID),
				slog.String("session_key", sessionKey),
				slog.String("error", err.Error()),
			)
			p.removeSession(sessionKey, session)
			session.closeConn()
		} else {
			// Successfully sent messages on existing connection.
			session.mu.Lock()
			session.lastUsedAt = time.Now().UTC()
			session.mu.Unlock()

			p.registerRunSession(runID, session)
			stream := make(chan transport.TransportEvent, 32)
			go p.streamRealtime(ctx, runID, sessionKey, session, stream)
			return stream, nil
		}
	}

	// Establish a new connection.
	conn, path, err := p.dialRealtime(runID, modelName)
	if err != nil {
		return nil, err
	}

	now := time.Now().UTC()
	session = &realtimeSession{
		conn:       conn,
		sessionKey: sessionKey,
		createdAt:  now,
		lastUsedAt: now,
	}

	// Start ping/pong health monitor.
	p.setupPingPong(session)

	p.storeSession(sessionKey, session)
	p.registerRunSession(runID, session)

	p.log.Info("created new openai realtime session",
		slog.String("run_id", runID),
		slog.String("session_key", sessionKey),
		slog.String("path", path),
	)

	if err := p.sendMessagesToSession(session, messages); err != nil {
		p.removeSession(sessionKey, session)
		p.unregisterRunSession(runID, session)
		session.closeConn()
		return nil, transport.NormalizeError(err, providerName)
	}

	stream := make(chan transport.TransportEvent, 32)
	go p.streamRealtime(ctx, runID, sessionKey, session, stream)
	return stream, nil
}

func (p *Provider) sendMessagesToSession(session *realtimeSession, messages []map[string]any) error {
	for _, message := range messages {
		if err := session.sendJSON(message); err != nil {
			return err
		}
	}
	return nil
}

func (p *Provider) dialRealtime(runID, modelName string) (*websocket.Conn, string, error) {
	realtimeURL := p.realtimeEndpoint(modelName)
	parsed, err := url.Parse(realtimeURL)
	if err != nil {
		return nil, "", transport.NormalizeError(err, providerName)
	}

	headers := http.Header{}
	if strings.TrimSpace(p.config.APIKey) != "" {
		headers.Set("Authorization", "Bearer "+p.config.APIKey)
	}
	headers.Set("OpenAI-Beta", "realtime=v1")
	headers.Set("User-Agent", "butler-openai-transport")

	p.log.Info("opening openai realtime websocket",
		slog.String("run_id", runID),
		slog.String("path", parsed.Path),
	)

	dialer := websocket.Dialer{
		// Use the provider's HTTP client TLS config when available.
		TLSClientConfig: nil,
	}
	conn, _, err := dialer.Dial(realtimeURL, headers)
	if err != nil {
		p.log.Warn("openai realtime websocket dial failed",
			slog.String("run_id", runID),
			slog.String("path", parsed.Path),
			slog.String("error", err.Error()),
		)
		return nil, parsed.Path, transport.NormalizeError(err, providerName)
	}
	return conn, parsed.Path, nil
}

// setupPingPong configures the WebSocket connection for keepalive
// ping/pong monitoring. A background goroutine sends pings at
// regular intervals; the pong handler updates the read deadline.
func (p *Provider) setupPingPong(session *realtimeSession) {
	session.mu.Lock()
	conn := session.conn
	session.mu.Unlock()
	if conn == nil {
		return
	}

	conn.SetPongHandler(func(string) error {
		session.mu.Lock()
		c := session.conn
		session.mu.Unlock()
		if c != nil {
			return c.SetReadDeadline(time.Now().Add(pingInterval + pongDeadline))
		}
		return nil
	})

	go func() {
		ticker := time.NewTicker(pingInterval)
		defer ticker.Stop()
		for range ticker.C {
			session.mu.Lock()
			c := session.conn
			closed := session.closed
			session.mu.Unlock()
			if c == nil || closed {
				return
			}
			if err := c.WriteControl(websocket.PingMessage, nil, time.Now().Add(pongDeadline)); err != nil {
				p.log.Debug("openai realtime ping failed",
					slog.String("session_key", session.sessionKey),
					slog.String("error", err.Error()),
				)
				return
			}
		}
	}()
}

func (p *Provider) streamRealtime(ctx context.Context, runID, sessionKey string, session *realtimeSession, stream chan<- transport.TransportEvent) {
	defer close(stream)
	defer p.unregisterRunSession(runID, session)
	// NOTE: we do NOT close the session connection here — the session may
	// be reused by subsequent runs. The session is only closed when the
	// session key entry is removed or the provider detects connection loss.

	go func() {
		<-ctx.Done()
		// When the run context is cancelled, we must unblock ReadJSON.
		// Set a short read deadline so the read goroutine exits promptly.
		session.mu.Lock()
		c := session.conn
		session.mu.Unlock()
		if c != nil {
			_ = c.SetReadDeadline(time.Now().Add(100 * time.Millisecond))
		}
	}()

	state := streamState{}
	for {
		message, err := session.receiveJSON()
		if err != nil {
			if ctx.Err() != nil {
				return
			}
			// Connection lost — remove session so it's not reused.
			p.log.Warn("openai realtime connection lost",
				slog.String("run_id", runID),
				slog.String("session_key", sessionKey),
				slog.String("error", err.Error()),
			)
			p.removeSession(sessionKey, session)
			session.closeConn()
			stream <- transport.NewTransportErrorEvent(runID, providerName, &transport.Error{
				Type:         transport.ErrorTypeStatefulSessionLost,
				Message:      fmt.Sprintf("openai realtime session lost: %v", err),
				Retryable:    true,
				ProviderName: providerName,
			})
			return
		}
		eventType := stringValue(message["type"])

		// Capture provider session ref from session.created events.
		if eventType == "session.created" || eventType == "session.updated" {
			if sessionObj, ok := message["session"].(map[string]any); ok {
				if id := stringValue(sessionObj["id"]); id != "" {
					session.mu.Lock()
					session.sessionRef = id
					session.mu.Unlock()
					p.log.Info("openai realtime session bound",
						slog.String("run_id", runID),
						slog.String("session_key", sessionKey),
						slog.String("session_ref", id),
					)
				}
			}
		}

		events, stop, normalizeErr := p.normalizePayload(runID, eventType, message, &state)
		if normalizeErr != nil {
			stream <- transport.NewTransportErrorEvent(runID, providerName, normalizeErr)
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
			p.log.Info("openai realtime run completed",
				slog.String("run_id", runID),
				slog.String("session_key", sessionKey),
			)
			return
		}
	}
}

// --- Session pool management ---

// sessionKeyForRun returns the session key associated with a run.
// It looks at the run context set by the current request. If not
// available, falls back to runID as a one-off session key.
func (p *Provider) sessionKeyForRun(ctx context.Context, runID string) string {
	if key, ok := ctx.Value(sessionKeyContextKey).(string); ok && key != "" {
		return key
	}
	return runID
}

type contextKey string

const sessionKeyContextKey contextKey = "transport_session_key"

// WithSessionKey returns a context annotated with the session key for
// WebSocket session pooling.
func WithSessionKey(ctx context.Context, sessionKey string) context.Context {
	return context.WithValue(ctx, sessionKeyContextKey, sessionKey)
}

// storeSession saves a session into the pool, keyed by session key.
// If an existing session is already present, the old one is closed.
func (p *Provider) storeSession(sessionKey string, session *realtimeSession) {
	p.mu.Lock()
	defer p.mu.Unlock()
	if old, exists := p.sessions[sessionKey]; exists && old != session {
		go old.closeConn()
	}
	p.sessions[sessionKey] = session
}

// getSessionByKey retrieves a live session from the pool.
func (p *Provider) getSessionByKey(sessionKey string) *realtimeSession {
	p.mu.Lock()
	defer p.mu.Unlock()
	session := p.sessions[sessionKey]
	if session == nil {
		return nil
	}
	session.mu.Lock()
	closed := session.closed
	session.mu.Unlock()
	if closed {
		delete(p.sessions, sessionKey)
		return nil
	}
	return session
}

// removeSession removes a session from the pool if it matches the
// provided session pointer (avoids removing a replacement session).
func (p *Provider) removeSession(sessionKey string, session *realtimeSession) {
	p.mu.Lock()
	defer p.mu.Unlock()
	if current, exists := p.sessions[sessionKey]; exists && current == session {
		delete(p.sessions, sessionKey)
	}
}

// registerRunSession associates a runID with a session so that
// CancelRun can find the active connection.
func (p *Provider) registerRunSession(runID string, session *realtimeSession) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.activeWS[runID] = session
}

// unregisterRunSession removes the runID→session mapping if it still
// points to the provided session.
func (p *Provider) unregisterRunSession(runID string, session *realtimeSession) {
	p.mu.Lock()
	defer p.mu.Unlock()
	if current := p.activeWS[runID]; current == session {
		delete(p.activeWS, runID)
	}
}

// activeRealtimeSession returns the session currently associated with
// a runID, or nil if none.
func (p *Provider) activeRealtimeSession(runID string) *realtimeSession {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.activeWS[runID]
}

func (p *Provider) prefersWebSocket() bool {
	return strings.TrimSpace(strings.ToLower(p.config.TransportMode)) != TransportModeSSEOnly
}

func (p *Provider) realtimeEndpoint(modelName string) string {
	endpoint := strings.TrimRight(p.config.RealtimeURL, "/")
	if endpoint == "" {
		endpoint = "wss://api.openai.com/v1/realtime"
	}
	parsed, err := url.Parse(endpoint)
	if err != nil {
		return endpoint
	}
	query := parsed.Query()
	if strings.TrimSpace(modelName) != "" {
		query.Set("model", modelName)
	}
	parsed.RawQuery = query.Encode()
	return parsed.String()
}

// CloseSession explicitly closes and removes the persistent session
// for a given session key. This is intended for graceful shutdown
// or when a Butler session ends.
func (p *Provider) CloseSession(sessionKey string) {
	p.mu.Lock()
	session, exists := p.sessions[sessionKey]
	if exists {
		delete(p.sessions, sessionKey)
	}
	p.mu.Unlock()
	if session != nil {
		p.log.Info("closing openai realtime session",
			slog.String("session_key", sessionKey),
			slog.String("session_ref", session.sessionRef),
		)
		session.closeConn()
	}
}

// CloseAllSessions closes all persistent WebSocket sessions.
// Intended for provider shutdown.
func (p *Provider) CloseAllSessions() {
	p.mu.Lock()
	sessions := make([]*realtimeSession, 0, len(p.sessions))
	for key, session := range p.sessions {
		sessions = append(sessions, session)
		delete(p.sessions, key)
	}
	p.mu.Unlock()
	for _, session := range sessions {
		session.closeConn()
	}
	p.log.Info("closed all openai realtime sessions",
		slog.Int("count", len(sessions)),
	)
}

// startRunRealtimeMessages converts a StartRunRequest into the sequence of
// OpenAI Realtime API client events required to initiate a response:
//
//  1. session.update  — configure the session (model, tools, instructions from
//     the first system message if present)
//  2. conversation.item.create — one event per user/assistant input item
//  3. response.create — trigger response generation
//
// The HTTP Responses API body shape is NOT forwarded here; Realtime uses its
// own event-driven protocol.
func (p *Provider) startRunRealtimeMessages(req transport.StartRunRequest) ([]map[string]any, error) {
	var events []map[string]any

	// 1. session.update — configure model and tools.
	sessionConfig := map[string]any{
		"modalities": []string{"text"},
	}
	if len(req.ToolDefinitions) > 0 {
		sessionConfig["tools"] = encodeRealtimeTools(req.ToolDefinitions)
		sessionConfig["tool_choice"] = "auto"
	}
	events = append(events, map[string]any{
		"type":    "session.update",
		"session": sessionConfig,
	})

	// 2. conversation.item.create — one per input item (skipping system messages
	// which are folded into session.update as instructions if present).
	for _, item := range req.InputItems {
		if strings.EqualFold(item.Role, "system") {
			sessionConfig["instructions"] = item.Content
			continue
		}
		events = append(events, map[string]any{
			"type": "conversation.item.create",
			"item": map[string]any{
				"type": "message",
				"role": item.Role,
				"content": []map[string]any{{
					"type": "input_text",
					"text": item.Content,
				}},
			},
		})
	}

	// 3. response.create — trigger the model response.
	events = append(events, map[string]any{"type": "response.create"})
	return events, nil
}

// continueRunRealtimeMessages converts a ContinueRunRequest into Realtime
// events: inject input items then request a new response.
func (p *Provider) continueRunRealtimeMessages(req transport.ContinueRunRequest) ([]map[string]any, error) {
	var events []map[string]any

	for _, item := range req.InputItems {
		events = append(events, map[string]any{
			"type": "conversation.item.create",
			"item": map[string]any{
				"type": "message",
				"role": item.Role,
				"content": []map[string]any{{
					"type": "input_text",
					"text": item.Content,
				}},
			},
		})
	}
	events = append(events, map[string]any{"type": "response.create"})
	return events, nil
}

// submitToolResultRealtimeMessages converts a SubmitToolResultRequest into
// Realtime events: inject the function_call_output item then request a
// new response.
func (p *Provider) submitToolResultRealtimeMessages(req transport.SubmitToolResultRequest) ([]map[string]any, error) {
	item := map[string]any{
		"type":    "function_call_output",
		"call_id": req.ToolCallRef,
		"output":  req.ToolResultJSON,
	}
	return []map[string]any{
		{"type": "conversation.item.create", "item": item},
		{"type": "response.create"},
	}, nil
}

// encodeRealtimeTools converts ToolDefinitions to the Realtime API tool format.
// The Realtime API uses the same function-call schema as the Chat Completions API.
func encodeRealtimeTools(tools []transport.ToolDefinition) []map[string]any {
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

func (s *realtimeSession) sendJSON(value any) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.conn == nil || s.closed {
		return fmt.Errorf("websocket connection is closed")
	}
	return s.conn.WriteJSON(value)
}

func (s *realtimeSession) receiveJSON() (map[string]any, error) {
	// ReadJSON does not need the send lock — gorilla/websocket supports
	// one concurrent reader and one concurrent writer.
	s.mu.Lock()
	conn := s.conn
	closed := s.closed
	s.mu.Unlock()
	if conn == nil || closed {
		return nil, fmt.Errorf("websocket connection is closed")
	}
	var payload map[string]any
	if err := conn.ReadJSON(&payload); err != nil {
		return nil, err
	}
	return payload, nil
}

func (s *realtimeSession) closeConn() {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.conn != nil && !s.closed {
		s.closed = true
		_ = s.conn.WriteMessage(
			websocket.CloseMessage,
			websocket.FormatCloseMessage(websocket.CloseNormalClosure, ""),
		)
		_ = s.conn.Close()
		s.conn = nil
	}
}
