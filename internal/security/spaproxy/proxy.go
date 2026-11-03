// Package spaproxy is a terminal, one-operation HTTPS query relay. Unlike an
// opaque CONNECT tunnel, it checks every worker/page request and WS frame.
package spaproxy

import (
	"bytes"
	"context"
	"crypto/tls"
	"io"
	"log"
	"net"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/butler/butler/internal/browserspa"
	"github.com/butler/butler/internal/security/egress"
)

// Options is trusted runtime construction, never a model-selected authority,
// proxy, certificate, deadline, private address or callback.
type Options struct {
	URL      string
	Deadline time.Time
	Policy   egress.Policy
	Check    func(context.Context, string) error
	Allows   func(string) bool
}

type Stats = browserspa.NetworkStats

type Proxy struct {
	ctx       context.Context
	cancel    context.CancelFunc
	options   Options
	origin    string
	host      string
	url       string
	SPKI      string
	transport *egress.Transport
	server    *http.Server
	tlsServer *http.Server
	listener  net.Listener
	tlsListen net.Listener
	outbound  *egress.Counter
	inbound   *egress.Counter
	traffic   *egress.Counter
	mu        sync.Mutex
	closed    bool
	failure   string
	stats     Stats
	active    int
	connects  int
	conns     map[net.Conn]bool
	closeOnce sync.Once
}

func New(ctx context.Context, o Options) (*Proxy, error) {
	u, err := o.Policy.ValidateURL(o.URL)
	if err != nil || u.Scheme != "https" || o.Check == nil || o.Allows == nil || !o.Allows(o.URL) || !o.Deadline.After(time.Now()) || o.Deadline.After(time.Now().Add(30*time.Second)) || o.Check(ctx, o.URL) != nil {
		return nil, browserspa.ErrDenied
	}
	cert, pin, err := localCertificate(u.Hostname(), o.Deadline)
	if err != nil {
		return nil, browserspa.ErrDenied
	}
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return nil, browserspa.ErrDenied
	}
	inner, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		listener.Close()
		return nil, browserspa.ErrDenied
	}
	owned, cancel := context.WithDeadline(ctx, o.Deadline)
	p := &Proxy{ctx: owned, cancel: cancel, options: o, origin: u.Scheme + "://" + u.Host, host: u.Host, SPKI: pin, listener: listener, tlsListen: inner, outbound: egress.NewCounter(egress.MaxRequestBytes), inbound: egress.NewCounter(20 * 1024 * 1024), traffic: egress.NewCounter(egress.MaxRequestBytes), conns: map[net.Conn]bool{}}
	p.url = "http://" + listener.Addr().String()
	policy := o.Policy
	dial := policy.Dial
	if dial == nil {
		dial = (&net.Dialer{Timeout: 3 * time.Second}).DialContext
	}
	policy.Dial = func(ctx context.Context, network, address string) (net.Conn, error) {
		target, _ := ctx.Value(destinationKey{}).(string)
		if target == "" || p.check(target) != nil {
			return nil, browserspa.ErrDenied
		}
		conn, err := dial(ctx, network, address)
		if err != nil {
			return nil, browserspa.ErrDenied
		}
		if !p.track(conn) {
			conn.Close()
			return nil, browserspa.ErrDenied
		}
		return &guardedConn{Conn: conn, proxy: p, target: target}, nil
	}
	p.transport = egress.NewTransport(policy)
	newServer := func(handler http.Handler) *http.Server {
		s := &http.Server{Handler: handler, ReadHeaderTimeout: 3 * time.Second, IdleTimeout: time.Second, MaxHeaderBytes: 32 * 1024, ErrorLog: log.New(io.Discard, "", 0), BaseContext: func(net.Listener) context.Context { return owned }}
		s.SetKeepAlivesEnabled(false)
		return s
	}
	p.server = newServer(http.HandlerFunc(p.connect))
	p.tlsServer = newServer(http.HandlerFunc(p.serveOrigin))
	go func() { _ = p.server.Serve(listener) }()
	go func() {
		_ = p.tlsServer.Serve(tls.NewListener(inner, &tls.Config{Certificates: []tls.Certificate{cert}, MinVersion: tls.VersionTLS12, NextProtos: []string{"http/1.1"}}))
	}()
	go func() {
		ticker := time.NewTicker(100 * time.Millisecond)
		defer ticker.Stop()
		for {
			select {
			case <-owned.Done():
				p.Close()
				return
			case <-ticker.C:
				if p.check() != nil {
					return
				}
			}
		}
	}()
	return p, nil
}

func (p *Proxy) URL() string { return p.url }
func (p *Proxy) Snapshot() (Stats, string) {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.stats, p.failure
}
func (p *Proxy) fail(reason string) error {
	p.mu.Lock()
	if !p.closed && p.failure == "" {
		p.failure = reason
	}
	p.mu.Unlock()
	p.Close()
	return browserspa.ErrDenied
}
func (p *Proxy) check(targets ...string) error {
	p.mu.Lock()
	closed := p.closed
	p.mu.Unlock()
	if closed || p.ctx.Err() != nil {
		return browserspa.ErrDenied
	}
	target := p.options.URL
	if len(targets) > 0 {
		target = targets[0]
	}
	if p.options.Check(p.ctx, target) != nil {
		return p.fail("authority_denied")
	}
	return nil
}
func (p *Proxy) track(conn net.Conn) bool {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.closed || len(p.conns) >= 96 {
		return false
	}
	p.conns[conn] = true
	return true
}
func (p *Proxy) untrack(conn net.Conn) {
	p.mu.Lock()
	delete(p.conns, conn)
	p.mu.Unlock()
}
func (p *Proxy) Close() {
	p.closeOnce.Do(func() {
		p.mu.Lock()
		p.closed = true
		connections := make([]net.Conn, 0, len(p.conns))
		for c := range p.conns {
			connections = append(connections, c)
		}
		p.mu.Unlock()
		p.cancel()
		for _, c := range connections {
			_ = c.Close()
		}
		_ = p.listener.Close()
		_ = p.tlsListen.Close()
		_ = p.server.Close()
		_ = p.tlsServer.Close()
		p.transport.CloseIdleConnections()
	})
}

// All browser network requests, including worker main-script updates, terminate
// inside this relay; no arbitrary encrypted CONNECT is forwarded to an origin.
func (p *Proxy) connect(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodConnect || r.Host != p.host && r.Host != p.host+":443" {
		p.fail("destination_denied")
		return
	}
	if p.check() != nil {
		return
	}
	p.mu.Lock()
	p.connects++
	allowed := !p.closed && p.connects <= 128
	p.mu.Unlock()
	if !allowed {
		p.fail("connection_limit")
		return
	}
	local, err := (&net.Dialer{Timeout: time.Second}).DialContext(p.ctx, "tcp", p.tlsListen.Addr().String())
	if err != nil {
		p.fail("relay_unavailable")
		return
	}
	defer local.Close()
	if !p.track(local) {
		p.fail("connection_limit")
		return
	}
	defer p.untrack(local)
	hijacker, ok := w.(http.Hijacker)
	if !ok {
		p.fail("relay_unavailable")
		return
	}
	client, buffered, err := hijacker.Hijack()
	if err != nil {
		return
	}
	defer client.Close()
	if !p.track(client) {
		p.fail("connection_limit")
		return
	}
	defer p.untrack(client)
	_ = client.SetDeadline(p.options.Deadline)
	_ = local.SetDeadline(p.options.Deadline)
	if _, err = buffered.WriteString("HTTP/1.1 200 Connection Established\r\n\r\n"); err != nil || buffered.Flush() != nil {
		return
	}
	done := make(chan struct{})
	go func() {
		_, _ = io.Copy(client, local)
		_ = client.Close()
		close(done)
	}()
	_, _ = io.Copy(local, buffered)
	_ = local.Close()
	<-done
}

func (p *Proxy) serveOrigin(w http.ResponseWriter, r *http.Request) {
	if p.check() != nil {
		return
	}
	u := *r.URL
	u.Scheme, u.Host = "https", r.Host
	if r.Host != p.host || u.User != nil || !p.options.Allows(u.String()) || p.check(u.String()) != nil {
		p.fail("destination_denied")
		return
	}
	if _, err := p.options.Policy.ValidateURL(u.String()); err != nil {
		p.fail("destination_denied")
		return
	}
	p.mu.Lock()
	p.stats.Requests++
	p.active++
	allowed := !p.closed && p.stats.Requests <= 128 && p.active <= 16
	p.mu.Unlock()
	defer func() { p.mu.Lock(); p.active--; p.mu.Unlock() }()
	if !allowed {
		p.fail("request_limit")
		return
	}
	if r.Header.Get("Upgrade") != "" {
		p.webSocket(w, r, &u)
		return
	}
	if r.Method == http.MethodPost {
		body, err := io.ReadAll(http.MaxBytesReader(w, r.Body, 64*1024))
		if err != nil || strings.ToLower(strings.TrimSpace(strings.Split(r.Header.Get("Content-Type"), ";")[0])) != "application/json" || browserspa.QueryBody(body) != nil {
			p.fail("query_denied")
			return
		}
		p.mu.Lock()
		if p.closed {
			p.mu.Unlock()
			return
		}
		p.stats.Queries++
		p.stats.PendingQueries++
		allowed := p.stats.Queries <= 4
		p.mu.Unlock()
		defer func() { p.mu.Lock(); p.stats.PendingQueries--; p.mu.Unlock() }()
		if !allowed {
			p.fail("query_limit")
			return
		}
		r.Body = io.NopCloser(bytes.NewReader(body))
		r.ContentLength = int64(len(body))
	} else if r.Method != http.MethodGet && r.Method != http.MethodHead || r.ContentLength != 0 || len(r.TransferEncoding) > 0 {
		p.fail("method_denied")
		return
	}
	request := r.Clone(context.WithValue(p.ctx, destinationKey{}, u.String()))
	request.URL, request.RequestURI = &u, ""
	request.Header = r.Header.Clone()
	// Bound processed body bytes as well as wire traffic. Reject a server that
	// ignores identity rather than passing compressed expansion to Chromium.
	request.Header.Set("Accept-Encoding", "identity")
	for _, name := range []string{"Proxy-Authorization", "Proxy-Connection", "Connection", "Upgrade", "Keep-Alive", "TE", "Trailer"} {
		request.Header.Del(name)
	}
	response, err := p.transport.RoundTrip(request)
	if err != nil {
		p.fail("response_lost")
		return
	}
	defer response.Body.Close()
	if encoding := strings.TrimSpace(response.Header.Get("Content-Encoding")); encoding != "" && !strings.EqualFold(encoding, "identity") {
		p.fail("content_encoding_denied")
		return
	}
	if p.check(u.String()) != nil || response.StatusCode == http.StatusSwitchingProtocols {
		p.fail("response_denied")
		return
	}
	for key, values := range response.Header {
		if strings.EqualFold(key, "Connection") || strings.EqualFold(key, "Transfer-Encoding") || strings.EqualFold(key, "Keep-Alive") {
			continue
		}
		for _, value := range values {
			w.Header().Add(key, value)
		}
	}
	w.WriteHeader(response.StatusCode)
	// Flush every bounded chunk, not just SSE; fetch/ReadableStream is supported.
	buffer := make([]byte, 16*1024)
	for {
		n, err := response.Body.Read(buffer)
		if n > 0 {
			if p.check(u.String()) != nil {
				return
			}
			if _, e := w.Write(buffer[:n]); e != nil {
				return
			}
			if http.NewResponseController(w).Flush() != nil {
				return
			}
		}
		if err == io.EOF {
			if r.Method == http.MethodPost {
				p.mu.Lock()
				p.stats.CompletedQueries++
				p.mu.Unlock()
			}
			return
		}
		if err != nil {
			p.fail("response_lost")
			return
		}
	}
}

type guardedConn struct {
	net.Conn
	proxy  *Proxy
	target string
}

type destinationKey struct{}

func (p *Proxy) takeTraffic(size int64, inbound bool) error {
	p.mu.Lock()
	if p.closed {
		p.mu.Unlock()
		return browserspa.ErrDenied
	}
	counter := p.outbound
	if inbound {
		counter = p.inbound
	}
	err := counter.Take(size)
	if err == nil {
		err = p.traffic.Take(size)
	}
	if err == nil {
		p.stats.MeteredBytes += size
	}
	p.mu.Unlock()
	if err != nil {
		return p.fail("traffic_limit")
	}
	return nil
}

func (c *guardedConn) Write(data []byte) (int, error) {
	if c.proxy.check(c.target) != nil {
		return 0, browserspa.ErrDenied
	}
	if err := c.proxy.takeTraffic(int64(len(data)), false); err != nil {
		return 0, err
	}
	return c.Conn.Write(data)
}
func (c *guardedConn) Read(data []byte) (int, error) {
	if c.proxy.check(c.target) != nil {
		return 0, browserspa.ErrDenied
	}
	n, err := c.Conn.Read(data)
	if n > 0 {
		if e := c.proxy.takeTraffic(int64(n), true); e != nil {
			return 0, e
		}
	}
	return n, err
}
func (c *guardedConn) Close() error {
	c.proxy.untrack(c.Conn)
	return c.Conn.Close()
}
