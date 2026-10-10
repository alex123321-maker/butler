// Package egresstest supplies trusted resolver/dial seams for local fixtures.
// It is imported exclusively by tests; production policy remains enabled.
package egresstest

import (
	"context"
	"github.com/butler/butler/internal/security/egress"
	"net"
	"net/http"
	"net/http/httptest"
	"net/url"
	"time"
)

const Origin = "http://external.test"

type Resolver struct{}

func (Resolver) LookupIPAddr(context.Context, string) ([]net.IPAddr, error) {
	return []net.IPAddr{{IP: net.ParseIP("93.184.216.34")}}, nil
}

func Policy(server *httptest.Server) egress.Policy {
	endpoint, _ := url.Parse(server.URL)
	return egress.Policy{Resolver: Resolver{}, Dial: func(ctx context.Context, network, address string) (net.Conn, error) {
		return (&net.Dialer{}).DialContext(ctx, network, endpoint.Host)
	}}
}
func Client(server *httptest.Server) *http.Client {
	return egress.NewClient(Policy(server), 15*time.Second)
}
