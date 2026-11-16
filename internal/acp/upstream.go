package acp

import (
	"bufio"
	"context"
	"io"
	"net"
	"net/http"
	"net/netip"
	"net/url"
	"strings"
	"time"

	"github.com/butler/butler/internal/security/egress"
)

// The upstream is trusted operator routing, never a model-selected destination.
// Only already checked public IP:443 targets enter CONNECT; Google TLS remains
// end to end and neither proxy credentials nor target DNS authority are added.
func googleUpstreamDial(raw string) (egress.DialFunc, error) {
	if raw == "" {
		return nil, nil
	}
	u, err := url.Parse(raw)
	if err != nil || u.Scheme != "http" || u.User != nil || u.Hostname() == "" || u.Port() == "" || u.Path != "" || u.RawQuery != "" || u.Fragment != "" || strings.ContainsAny(u.Hostname(), "%\\") {
		return nil, ErrProcess
	}
	return func(ctx context.Context, network, address string) (net.Conn, error) {
		host, port, err := net.SplitHostPort(address)
		ip, parseErr := netip.ParseAddr(host)
		if err != nil || parseErr != nil || !egress.PublicAddress(ip) || port != "443" || (network != "tcp" && network != "tcp4" && network != "tcp6") {
			return nil, egress.ErrDenied
		}
		conn, err := (&net.Dialer{Timeout: 5 * time.Second}).DialContext(ctx, "tcp", u.Host)
		if err != nil {
			return nil, ErrProcess
		}
		ok := false
		defer func() {
			if !ok {
				conn.Close()
			}
		}()
		deadline := time.Now().Add(8 * time.Second)
		if parent, exists := ctx.Deadline(); exists && parent.Before(deadline) {
			deadline = parent
		}
		if conn.SetDeadline(deadline) != nil {
			return nil, ErrProcess
		}
		stop := context.AfterFunc(ctx, func() { conn.Close() })
		defer stop()
		request := &http.Request{Method: http.MethodConnect, URL: &url.URL{Opaque: address}, Host: address}
		if request.Write(conn) != nil {
			return nil, ErrProcess
		}
		limited := &io.LimitedReader{R: conn, N: 32 << 10}
		reader := bufio.NewReader(limited)
		response, err := http.ReadResponse(reader, request)
		if err != nil || response.StatusCode != http.StatusOK || ctx.Err() != nil {
			return nil, ErrProcess
		}
		limited.N = 1<<63 - 1
		if conn.SetDeadline(time.Time{}) != nil {
			return nil, ErrProcess
		}
		ok = true
		return &upstreamConn{Conn: conn, reader: reader}, nil
	}, nil
}

type upstreamConn struct {
	net.Conn
	reader *bufio.Reader
}

func (conn *upstreamConn) Read(b []byte) (int, error) { return conn.reader.Read(b) }
