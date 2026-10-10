package egress

import (
	"bytes"
	"context"
	"crypto/tls"
	"io"
	"net"
	"net/http"
	"time"
)

type Transport struct {
	policy Policy
	base   *http.Transport
}

func NewClient(policy Policy, timeout time.Duration) *http.Client {
	return &http.Client{Transport: NewTransport(policy), Timeout: timeout, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}
}

// SecureClient does not inherit a supplied unguarded transport/proxy/redirect
// policy. Explicit trusted test seams use NewClient with an injected Policy.
func SecureClient(client *http.Client) *http.Client {
	if client != nil {
		if _, ok := client.Transport.(*Transport); ok {
			copy := *client
			copy.CheckRedirect = func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }
			return &copy
		}
	}
	timeout := 15 * time.Second
	if client != nil && client.Timeout > 0 {
		timeout = client.Timeout
	}
	return NewClient(Policy{}, timeout)
}

func NewTransport(policy Policy) *Transport {
	t := &Transport{policy: policy}
	t.base = &http.Transport{Proxy: nil, DialContext: t.dial, TLSClientConfig: &tls.Config{MinVersion: tls.VersionTLS12}, TLSHandshakeTimeout: 10 * time.Second, ResponseHeaderTimeout: 15 * time.Second, IdleConnTimeout: 30 * time.Second, MaxResponseHeaderBytes: 32 * 1024, MaxIdleConns: 32, MaxIdleConnsPerHost: 4, DisableKeepAlives: true, ForceAttemptHTTP2: true}
	return t
}

func (t *Transport) dial(ctx context.Context, network, address string) (net.Conn, error) {
	host, port, err := net.SplitHostPort(address)
	if err != nil {
		return nil, ErrDenied
	}
	addresses, err := t.policy.Resolve(ctx, host)
	if err != nil {
		return nil, err
	}
	dial := t.policy.Dial
	if dial == nil {
		dial = (&net.Dialer{Timeout: 10 * time.Second, KeepAlive: 30 * time.Second}).DialContext
	}
	for _, ip := range addresses {
		conn, err := dial(ctx, network, net.JoinHostPort(ip.String(), port))
		if err == nil {
			return conn, nil
		}
	}
	return nil, ErrDenied
}

func (t *Transport) RoundTrip(request *http.Request) (*http.Response, error) {
	if request == nil || request.URL == nil {
		return nil, ErrDenied
	}
	if _, err := t.policy.ValidateURL(request.URL.String()); err != nil {
		return nil, err
	}
	if request.Host != "" && request.Host != request.URL.Host {
		return nil, ErrDenied
	}
	size, err := requestSize(request, MaxRequestBytes)
	if err != nil {
		return nil, err
	}
	if counter := counterFrom(request.Context()); counter != nil {
		if err := counter.Take(size); err != nil {
			return nil, err
		}
	}
	response, err := t.base.RoundTrip(request)
	if err != nil {
		return nil, ErrDenied
	} // No URL, headers, body or token in errors.
	return response, nil
}

func (t *Transport) CloseIdleConnections() { t.base.CloseIdleConnections() }

type countingWriter struct{ count, limit int64 }

func (w *countingWriter) Write(p []byte) (int, error) {
	w.count += int64(len(p))
	if w.count > w.limit {
		return 0, ErrLimit
	}
	return len(p), nil
}

// Count the actual HTTP/1 request representation before the first network
// effect, including resolved headers. Bodies are restored, never logged.
func CheckRequestBytes(request *http.Request, limit int64) error {
	_, err := requestSize(request, limit)
	return err
}

func requestSize(request *http.Request, limit int64) (int64, error) {
	var body []byte
	if request.Body != nil {
		var err error
		body, err = io.ReadAll(io.LimitReader(request.Body, limit+1))
		_ = request.Body.Close()
		if err != nil || int64(len(body)) > limit {
			return 0, ErrLimit
		}
		request.Body = io.NopCloser(bytes.NewReader(body))
	}
	clone := request.Clone(request.Context())
	if request.Body != nil {
		clone.Body = io.NopCloser(bytes.NewReader(body))
	}
	writer := &countingWriter{limit: limit}
	if err := clone.Write(writer); err != nil {
		return 0, ErrLimit
	}
	return writer.count, nil
}
