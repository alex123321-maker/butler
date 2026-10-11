package egress

import (
	"context"
	"io"
	"net"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"
)

// ScopedProxy is a private Chromium proxy, fixed to one public origin. The
// context allowance survives later operations; expiry closes hijacked tunnels.
type ScopedProxy struct {
	server   *http.Server
	listener net.Listener
	policy   Policy
	origin   *url.URL
	expires  time.Time
	counter  *Counter
	inbound  *Counter
	mu       sync.Mutex
	tunnels  map[net.Conn]bool
	requests map[uint64]context.CancelFunc
	epoch    uint64
	next     uint64
	frozen   bool
	closed   bool
	timer    *time.Timer
	window   *time.Timer
}

func NewScopedProxy(policy Policy, origin string, expires time.Time, limit int64) (*ScopedProxy, error) {
	parsed, err := policy.ValidateURL(origin)
	if err != nil || (parsed.Path != "" && parsed.Path != "/") || parsed.RawQuery != "" || !expires.After(time.Now()) {
		return nil, ErrDenied
	}
	policy.AllowedDomains = []string{parsed.Hostname()}
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return nil, ErrDenied
	}
	proxy := &ScopedProxy{listener: listener, policy: policy, origin: parsed, expires: expires, counter: NewCounter(limit), inbound: NewCounter(20 * 1024 * 1024), tunnels: map[net.Conn]bool{}, requests: map[uint64]context.CancelFunc{}}
	proxy.server = &http.Server{Handler: proxy, ReadHeaderTimeout: 5 * time.Second, IdleTimeout: 10 * time.Second, MaxHeaderBytes: 32 * 1024}
	// AfterFunc may call Close before the returned timer has been assigned. The
	// callback reads timer under mu, so initialize it under the same lock.
	proxy.mu.Lock()
	proxy.timer = time.AfterFunc(time.Until(expires), func() { _ = proxy.Close() })
	proxy.mu.Unlock()
	go func() { _ = proxy.server.Serve(listener) }()
	return proxy, nil
}

func (p *ScopedProxy) URL() string   { return "http://" + p.listener.Addr().String() }
func (p *ScopedProxy) Expired() bool { return !time.Now().Before(p.expires) }

// Freeze survives subsequent observations. Cancel requests and close existing
// CONNECT tunnels too; blocking only new connections would leave draft release
// possible through an established browser connection.
func (p *ScopedProxy) Freeze() {
	p.mu.Lock()
	p.freezeLocked()
	p.mu.Unlock()
}
func (p *ScopedProxy) freezeLocked() {
	p.frozen = true
	p.epoch++
	if p.window != nil {
		p.window.Stop()
	}
	for _, cancel := range p.requests {
		cancel()
	}
	for connection := range p.tunnels {
		_ = connection.Close()
	}
}

// AllowUntil is a private runner capability for a separately authorized click.
// It never resets byte allowance or session expiry. The reviewed page route
// stays frozen until its live observation guards have passed.
func (p *ScopedProxy) AllowUntil(deadline time.Time) error {
	p.mu.Lock()
	defer p.mu.Unlock()
	if deadline.After(p.expires) {
		deadline = p.expires
	}
	if p.closed || !deadline.After(time.Now()) {
		return ErrDenied
	}
	if p.window != nil {
		p.window.Stop()
	}
	p.epoch++
	epoch := p.epoch
	p.frozen = false
	p.window = time.AfterFunc(time.Until(deadline), func() {
		p.mu.Lock()
		if p.epoch == epoch {
			p.freezeLocked()
		}
		p.mu.Unlock()
	})
	return nil
}
func (p *ScopedProxy) begin(ctx context.Context) (context.Context, uint64, func(), bool) {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.closed || p.frozen || p.Expired() {
		return ctx, 0, func() {}, false
	}
	ctx, cancel := context.WithDeadline(ctx, p.expires)
	p.next++
	id, epoch := p.next, p.epoch
	p.requests[id] = cancel
	return ctx, epoch, func() {
		cancel()
		p.mu.Lock()
		delete(p.requests, id)
		p.mu.Unlock()
	}, true
}
func (p *ScopedProxy) Close() error {
	p.mu.Lock()
	if p.closed {
		p.mu.Unlock()
		return nil
	}
	p.closed = true
	p.freezeLocked()
	if p.timer != nil {
		p.timer.Stop()
	}
	p.mu.Unlock()
	return p.server.Close()
}
func (p *ScopedProxy) track(connection net.Conn, epoch uint64) bool {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.closed || p.frozen || p.epoch != epoch {
		return false
	}
	p.tunnels[connection] = true
	return true
}
func (p *ScopedProxy) untrack(connection net.Conn) {
	p.mu.Lock()
	delete(p.tunnels, connection)
	p.mu.Unlock()
}

func portFor(origin *url.URL) string {
	if port := origin.Port(); port != "" {
		return port
	}
	if origin.Scheme == "https" {
		return "443"
	}
	return "80"
}

func (p *ScopedProxy) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	ctx, epoch, done, allowed := p.begin(r.Context())
	if !allowed {
		http.Error(w, "proxy unavailable", http.StatusForbidden)
		return
	}
	defer done()
	r = r.WithContext(ctx)
	if r.Method == http.MethodConnect {
		p.connect(w, r, epoch)
		return
	}
	if p.origin.Scheme != "http" || r.URL.Scheme != "http" || !strings.EqualFold(r.URL.Host, p.origin.Host) || r.URL.User != nil {
		http.Error(w, "destination denied", http.StatusForbidden)
		return
	}
	// WebSockets have no Broker action contract; upgrade tunnels are denied.
	if r.Header.Get("Upgrade") != "" {
		http.Error(w, "upgrade denied", http.StatusForbidden)
		return
	}
	request := r.Clone(WithCounter(r.Context(), p.counter))
	request.RequestURI = ""
	request.Header.Del("Proxy-Authorization")
	request.Header.Del("Proxy-Connection")
	request.Header.Del("Connection")
	transport := NewTransport(p.policy)
	defer transport.CloseIdleConnections()
	response, err := transport.RoundTrip(request)
	if err != nil {
		http.Error(w, "outbound request denied", http.StatusForbidden)
		return
	}
	defer response.Body.Close()
	for key, values := range response.Header {
		for _, value := range values {
			w.Header().Add(key, value)
		}
	}
	w.WriteHeader(response.StatusCode)
	_, _ = io.Copy(w, io.LimitReader(response.Body, 20*1024*1024))
}

func (p *ScopedProxy) connect(w http.ResponseWriter, r *http.Request, epoch uint64) {
	if p.origin.Scheme != "https" || !strings.EqualFold(r.Host, net.JoinHostPort(p.origin.Hostname(), portFor(p.origin))) {
		http.Error(w, "destination denied", http.StatusForbidden)
		return
	}
	ctx, cancel := context.WithDeadline(r.Context(), p.expires)
	defer cancel()
	transport := NewTransport(p.policy)
	remote, err := transport.dial(ctx, "tcp", r.Host)
	if err != nil {
		http.Error(w, "destination denied", http.StatusForbidden)
		return
	}
	if !p.track(remote, epoch) {
		_ = remote.Close()
		return
	}
	defer p.untrack(remote)
	defer remote.Close()
	hijacker, ok := w.(http.Hijacker)
	if !ok {
		return
	}
	client, buffer, err := hijacker.Hijack()
	if err != nil {
		return
	}
	if !p.track(client, epoch) {
		_ = client.Close()
		return
	}
	defer p.untrack(client)
	defer client.Close()
	_ = client.SetDeadline(p.expires)
	_ = remote.SetDeadline(p.expires)
	if _, err = buffer.WriteString("HTTP/1.1 200 Connection Established\r\n\r\n"); err != nil {
		return
	}
	if err = buffer.Flush(); err != nil {
		return
	}
	done := make(chan struct{})
	go func() {
		_, _ = io.Copy(&boundedWriter{writer: client, counter: p.inbound}, remote)
		_ = client.Close()
		_ = remote.Close()
		close(done)
	}()
	_, _ = io.Copy(&boundedWriter{writer: remote, counter: p.counter}, buffer)
	_ = remote.Close()
	_ = client.Close()
	<-done
}

type boundedWriter struct {
	writer  io.Writer
	counter *Counter
}

func (w *boundedWriter) Write(bytes []byte) (int, error) {
	if err := w.counter.Take(int64(len(bytes))); err != nil {
		return 0, err
	}
	return w.writer.Write(bytes)
}
