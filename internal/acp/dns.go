package acp

import (
	"context"
	"crypto/tls"
	"encoding/json"
	"io"
	"net"
	"net/http"
	"net/url"
	"time"

	"github.com/butler/butler/internal/jsonstrict"
	"github.com/butler/butler/internal/security/egress"
)

type googleDNSResolver struct{ client *http.Client }

func newGoogleDNSResolver() *googleDNSResolver {
	return newGoogleDNSResolverVia(nil)
}

func newGoogleDNSResolverVia(upstream egress.DialFunc) *googleDNSResolver {
	transport := &http.Transport{Proxy: nil, TLSClientConfig: &tls.Config{MinVersion: tls.VersionTLS12}, TLSHandshakeTimeout: 5 * time.Second, ResponseHeaderTimeout: 5 * time.Second, MaxResponseHeaderBytes: 16 << 10}
	transport.DialContext = func(ctx context.Context, network, address string) (net.Conn, error) {
		if address != "dns.google:443" {
			return nil, egress.ErrDenied
		}
		if upstream != nil {
			return upstream(ctx, "tcp", "8.8.8.8:443")
		}
		return (&net.Dialer{Timeout: 5 * time.Second}).DialContext(ctx, "tcp", "8.8.8.8:443")
	}
	return &googleDNSResolver{client: &http.Client{Transport: transport, Timeout: 8 * time.Second, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}}
}

func (resolver *googleDNSResolver) LookupIPAddr(ctx context.Context, host string) ([]net.IPAddr, error) {
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, "https://dns.google/resolve?name="+url.QueryEscape(host)+"&type=A", nil)
	if err != nil {
		return nil, egress.ErrDenied
	}
	response, err := resolver.client.Do(request)
	if err != nil {
		return nil, egress.ErrDenied
	}
	defer response.Body.Close()
	raw, err := io.ReadAll(io.LimitReader(response.Body, (64<<10)+1))
	var checked map[string]any
	if err != nil || response.StatusCode != 200 || len(raw) > 64<<10 || jsonstrict.Decode(raw, &checked) != nil {
		return nil, egress.ErrDenied
	}
	var result struct {
		Status int `json:"Status"`
		Answer []struct {
			Type int    `json:"type"`
			Data string `json:"data"`
		} `json:"Answer"`
	}
	if json.Unmarshal(raw, &result) != nil || result.Status != 0 || len(result.Answer) > 128 {
		return nil, egress.ErrDenied
	}
	addresses := []net.IPAddr{}
	for _, answer := range result.Answer {
		if answer.Type != 1 {
			continue
		}
		ip := net.ParseIP(answer.Data)
		if ip == nil || ip.To4() == nil {
			return nil, egress.ErrDenied
		}
		addresses = append(addresses, net.IPAddr{IP: ip})
	}
	if len(addresses) == 0 {
		return nil, egress.ErrDenied
	}
	return addresses, nil
}
