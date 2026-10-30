package openai

import (
	"context"
	"encoding/json"
	"github.com/butler/butler/internal/transport"
	"github.com/gorilla/websocket"
	"net/http"
	"strings"
	"sync"
)

// A socket is a run-local optimization, never Butler session history.
type responsesSession struct {
	conn       *websocket.Conn
	mu         sync.Mutex
	busy       bool
	closed     bool
	sessionKey string
}

func deriveResponsesURL(base string) string {
	base = strings.TrimRight(base, "/")
	base = strings.Replace(strings.Replace(base, "https://", "wss://", 1), "http://", "ws://", 1)
	if !strings.HasSuffix(base, "/responses") {
		base += "/responses"
	}
	return base
}

func (p *Provider) executeRunRequest(parent context.Context, runID string, body []byte) (transport.EventStream, error) {
	if err := transport.CheckSerializedInput(parent, len(body)); err != nil {
		return nil, err
	}
	if err := transport.CheckInferenceAuthority(parent); err != nil {
		return nil, err
	}
	if err := p.checkContextAllowance(runID, body); err != nil {
		return nil, err
	}
	ctx, cancel := context.WithTimeout(parent, p.config.Timeout)
	p.mu.Lock()
	if _, active := p.cancels[runID]; active {
		p.mu.Unlock()
		cancel()
		return nil, transport.ProviderProtocolError(providerName)
	}
	if timer := p.expiry[runID]; timer != nil {
		timer.Stop()
		delete(p.expiry, runID)
	}
	p.cancels[runID] = cancel
	p.mu.Unlock()
	var stream transport.EventStream
	var err error
	if p.config.TransportMode == TransportModeSSEOnly {
		stream, err = p.executeStreamingRequest(ctx, runID, http.MethodPost, p.endpoint("/responses"), body)
	} else {
		var fallback bool
		stream, fallback, err = p.executeResponsesWS(ctx, runID, body)
		if fallback && ctx.Err() == nil {
			stream, err = p.executeStreamingRequest(ctx, runID, http.MethodPost, p.endpoint("/responses"), body)
			if err == nil {
				stream = prependEventStream(ctx, transport.NewTransportWarningEvent(runID, providerName, "Responses WebSocket unsupported; using HTTP SSE", nil), stream)
			}
		}
	}
	if err != nil {
		cancel()
		p.finishRequest(runID)
		return nil, err
	}
	result := make(chan transport.TransportEvent, 32)
	go func() {
		defer close(result)
		defer cancel()
		defer p.finishRequest(runID)
		for event := range stream {
			select {
			case result <- event:
			case <-ctx.Done():
				return
			}
		}
	}()
	return result, nil
}

func (p *Provider) finishRequest(runID string) { p.mu.Lock(); delete(p.cancels, runID); p.mu.Unlock() }
func (p *Provider) cancelRequest(runID string) {
	p.mu.Lock()
	cancel := p.cancels[runID]
	p.mu.Unlock()
	if cancel != nil {
		cancel()
	}
}

func (p *Provider) executeResponsesWS(ctx context.Context, runID string, body []byte) (transport.EventStream, bool, error) {
	var request map[string]any
	if json.Unmarshal(body, &request) != nil {
		return nil, false, transport.ProviderProtocolError(providerName)
	}
	delete(request, "stream")
	delete(request, "background")
	request["type"] = "response.create"
	encoded, err := json.Marshal(request)
	if err != nil {
		return nil, false, err
	}
	if err = transport.CheckSerializedInput(ctx, len(encoded)); err != nil {
		return nil, false, err
	}
	p.mu.Lock()
	session := p.sessions[runID]
	p.mu.Unlock()
	if session == nil {
		headers := http.Header{}
		if p.config.APIKey != "" {
			headers.Set("Authorization", "Bearer "+p.config.APIKey)
		}
		dialer := websocket.Dialer{Proxy: http.ProxyFromEnvironment, HandshakeTimeout: p.config.Timeout}
		if base, ok := p.httpClient.Transport.(*http.Transport); ok {
			dialer.TLSClientConfig = base.TLSClientConfig
			dialer.NetDialContext = base.DialContext
			dialer.Proxy = base.Proxy
		}
		conn, resp, dialErr := dialer.DialContext(ctx, p.config.RealtimeURL, headers)
		if dialErr != nil {
			if resp != nil {
				defer resp.Body.Close()
				status := resp.StatusCode
				if status == 404 || status == 405 || status == 426 || status == 501 {
					return nil, true, transport.ProviderProtocolError(providerName)
				}
				failure := transport.NormalizeError(transport.ProviderHTTPError(resp), providerName)
				failure.HTTPStatus = status
				return nil, false, failure
			}
			return nil, false, transport.NormalizeError(dialErr, providerName)
		}
		conn.SetReadLimit(8 * 1024 * 1024)
		key, _ := transport.SessionKeyFromContext(ctx)
		session = &responsesSession{conn: conn, sessionKey: key}
		p.mu.Lock()
		p.sessions[runID] = session
		p.mu.Unlock()
	}
	session.mu.Lock()
	if session.closed || session.busy {
		session.mu.Unlock()
		return nil, false, transport.ProviderProtocolError(providerName)
	}
	session.busy = true
	session.mu.Unlock()
	if err = transport.CheckInferenceAuthority(ctx); err == nil {
		err = transport.CheckInferenceDispatch(ctx)
	}
	if err != nil {
		p.CloseRun(runID)
		return nil, false, err
	}
	if deadline, ok := ctx.Deadline(); ok {
		_ = session.conn.SetWriteDeadline(deadline)
		_ = session.conn.SetReadDeadline(deadline)
	}
	// Never replay after a frame write: generation and spending may have occurred.
	if err = session.conn.WriteMessage(websocket.TextMessage, encoded); err != nil {
		p.CloseRun(runID)
		return nil, false, transport.NormalizeError(err, providerName)
	}
	stream := make(chan transport.TransportEvent, 32)
	go func() {
		defer close(stream)
		defer func() { session.mu.Lock(); session.busy = false; session.mu.Unlock() }()
		stop := context.AfterFunc(ctx, func() { session.closeConn() })
		defer stop()
		state := streamState{}
		for frames := 0; frames < 100000; frames++ {
			var payload map[string]any
			if err := session.conn.ReadJSON(&payload); err != nil {
				p.CloseRun(runID)
				select {
				case stream <- transport.NewTransportErrorEvent(runID, providerName, err):
				case <-ctx.Done():
				}
				return
			}
			events, done, err := p.normalizePayload(runID, stringValue(payload["type"]), payload, &state)
			if err != nil {
				p.CloseRun(runID)
				events = []transport.TransportEvent{transport.NewTransportErrorEvent(runID, providerName, err)}
				done = true
			}
			if done {
				stop()
				session.mu.Lock()
				session.busy = false
				session.mu.Unlock()
			}
			for _, event := range events {
				select {
				case stream <- event:
				case <-ctx.Done():
					p.CloseRun(runID)
					return
				}
			}
			if done {
				return
			}
		}
		p.CloseRun(runID)
		select {
		case stream <- transport.NewTransportErrorEvent(runID, providerName, transport.ProviderProtocolError(providerName)):
		case <-ctx.Done():
		}
	}()
	return stream, false, nil
}

func prependEventStream(ctx context.Context, first transport.TransportEvent, remainder transport.EventStream) transport.EventStream {
	stream := make(chan transport.TransportEvent, 32)
	go func() {
		defer close(stream)
		select {
		case stream <- first:
		case <-ctx.Done():
			return
		}
		for event := range remainder {
			select {
			case stream <- event:
			case <-ctx.Done():
				return
			}
		}
	}()
	return stream
}

func (s *responsesSession) closeConn() {
	s.mu.Lock()
	defer s.mu.Unlock()
	if !s.closed {
		s.closed = true
		_ = s.conn.Close()
	}
}
func (p *Provider) CloseRun(runID string) {
	p.mu.Lock()
	s := p.sessions[runID]
	delete(p.sessions, runID)
	p.mu.Unlock()
	if s != nil {
		s.closeConn()
	}
}
func (p *Provider) CloseSession(key string) {
	p.mu.Lock()
	var ids []string
	for id, s := range p.sessions {
		if s.sessionKey == key {
			ids = append(ids, id)
		}
	}
	p.mu.Unlock()
	for _, id := range ids {
		p.cancelRequest(id)
		p.CloseRun(id)
		p.clearInstructions(id)
	}
}
func (p *Provider) CloseAllSessions() {
	p.mu.Lock()
	var ids []string
	for id := range p.sessions {
		ids = append(ids, id)
	}
	for _, cancel := range p.cancels {
		cancel()
	}
	for _, timer := range p.expiry {
		timer.Stop()
	}
	clear(p.expiry)
	clear(p.instructions)
	clear(p.toolSets)
	clear(p.aliases)
	clear(p.prefixTokens)
	p.mu.Unlock()
	for _, id := range ids {
		p.CloseRun(id)
	}
}

func splitInstructionItems(items []transport.InputItem) (string, []transport.InputItem) {
	var instructions []string
	var input []transport.InputItem
	for _, item := range items {
		if item.Role == "system" || item.Role == "developer" {
			instructions = append(instructions, item.Content)
		} else {
			input = append(input, item)
		}
	}
	return strings.Join(instructions, "\n\n"), input
}
