package spaproxy

import (
	"context"
	"crypto/tls"
	"net/http"
	"net/url"
	"strings"
	"sync/atomic"
	"time"

	"github.com/butler/butler/internal/browserspa"
	"github.com/gorilla/websocket"
)

// Only server push or the fixed GraphQL read state machine is supported.
// Arbitrary application data and GraphQL mutations are never forwarded.
func (p *Proxy) webSocket(w http.ResponseWriter, r *http.Request, target *url.URL) {
	protocol := r.Header.Get("Sec-WebSocket-Protocol")
	if r.Method != http.MethodGet || !strings.EqualFold(r.Header.Get("Upgrade"), "websocket") || r.Header.Get("Origin") != p.origin || r.ContentLength != 0 || protocol != "" && protocol != browserspa.SocketProtocol || p.check(target.String()) != nil {
		p.fail("websocket_denied")
		return
	}
	p.mu.Lock()
	p.stats.WebSockets++
	allowed := p.stats.WebSockets <= 4
	p.mu.Unlock()
	if !allowed {
		p.fail("websocket_limit")
		return
	}
	u := *target
	u.Scheme = "wss"
	headers := r.Header.Clone()
	for name := range headers {
		if strings.HasPrefix(strings.ToLower(name), "sec-websocket-") || strings.EqualFold(name, "Connection") || strings.EqualFold(name, "Upgrade") || strings.EqualFold(name, "Proxy-Authorization") || strings.EqualFold(name, "Proxy-Connection") {
			headers.Del(name)
		}
	}
	dialer := websocket.Dialer{NetDialContext: p.transport.DialContext, Proxy: nil, HandshakeTimeout: 3 * time.Second, TLSClientConfig: &tls.Config{MinVersion: tls.VersionTLS12, RootCAs: p.options.Policy.TLSRoots}}
	var protocols []string
	if protocol != "" {
		protocols = []string{protocol}
		dialer.Subprotocols = protocols
	}
	upstream, response, err := dialer.DialContext(context.WithValue(p.ctx, destinationKey{}, target.String()), u.String(), headers)
	if err != nil && response != nil && response.Body != nil {
		response.Body.Close()
	}
	if err != nil {
		p.fail("websocket_lost")
		return
	}
	defer upstream.Close()
	if upstream.Subprotocol() != protocol {
		p.fail("websocket_protocol_denied")
		return
	}
	upstream.SetReadLimit(64 * 1024)
	_ = upstream.SetReadDeadline(p.options.Deadline)
	_ = upstream.SetWriteDeadline(p.options.Deadline)
	upgrade := websocket.Upgrader{ReadBufferSize: 4096, WriteBufferSize: 4096, CheckOrigin: func(r *http.Request) bool { return r.Header.Get("Origin") == p.origin }, HandshakeTimeout: time.Second}
	upgrade.Subprotocols = protocols
	responseHeaders := http.Header{}
	if response != nil {
		for _, cookie := range response.Header.Values("Set-Cookie") {
			responseHeaders.Add("Set-Cookie", cookie)
		}
	}
	client, err := upgrade.Upgrade(w, r, responseHeaders)
	if err != nil {
		p.fail("websocket_lost")
		return
	}
	defer client.Close()
	client.SetReadLimit(64 * 1024)
	_ = client.SetReadDeadline(p.options.Deadline)
	_ = client.SetWriteDeadline(p.options.Deadline)
	done := make(chan struct{})
	var clientClosed atomic.Bool
	socket := &browserspa.SocketPolicy{}
	go func() {
		defer close(done)
		for {
			kind, data, err := client.ReadMessage()
			if err != nil {
				break
			}
			if protocol != browserspa.SocketProtocol || kind != websocket.TextMessage || socket.Client(data) != nil {
				p.fail("client_message_denied")
				break
			}
			if p.check(target.String()) != nil {
				break
			}
			p.mu.Lock()
			p.stats.SentMessages++
			allowed := p.stats.SentMessages <= 128
			p.mu.Unlock()
			if !allowed {
				p.fail("message_limit")
				break
			}
			if upstream.WriteMessage(kind, data) != nil {
				if p.ctx.Err() == nil {
					p.fail("websocket_lost")
				}
				break
			}
		}
		clientClosed.Store(true)
		_ = upstream.Close()
	}()
	for {
		kind, data, err := upstream.ReadMessage()
		if err != nil {
			if !clientClosed.Load() && !websocket.IsCloseError(err, websocket.CloseNormalClosure, websocket.CloseGoingAway) && p.ctx.Err() == nil {
				p.fail("websocket_lost")
			}
			break
		}
		if p.check(target.String()) != nil {
			break
		}
		if protocol == browserspa.SocketProtocol && (kind != websocket.TextMessage || socket.Server(data) != nil) {
			p.fail("server_message_denied")
			break
		}
		p.mu.Lock()
		p.stats.Messages++
		allowed := p.stats.Messages <= 128
		p.mu.Unlock()
		if !allowed {
			p.fail("message_limit")
			break
		}
		if client.WriteMessage(kind, data) != nil {
			break
		}
	}
	_ = client.Close()
	_ = upstream.Close()
	<-done
}
