package egress

import (
	"context"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"net/netip"
	"net/url"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

type resolverFunc func(context.Context, string) ([]net.IPAddr, error)

func (f resolverFunc) LookupIPAddr(ctx context.Context, host string) ([]net.IPAddr, error) {
	return f(ctx, host)
}

func TestInternalAndAmbiguousResourcesAreDenied(t *testing.T) {
	for _, target := range []string{"http://127.0.0.1/", "http://[::1]/", "http://[::ffff:127.0.0.1]/", "http://169.254.169.254/latest/meta-data/", "http://10.0.0.4/", "http://100.64.1.2/", "file:///etc/passwd", "http://user:synthetic-secret@example.com/", "http://example.com:2375/", "http://example.com/#secret"} {
		if _, err := (Policy{}).ValidateURL(target); err == nil {
			t.Fatalf("accepted %s", target)
		}
	}
	for _, ip := range []string{"0.0.0.0", "127.1.2.3", "172.16.1.2", "192.168.2.3", "::1", "fc00::1", "2001:db8::1", "64:ff9b::7f00:1", "224.1.2.3", "192.0.2.1"} {
		if PublicAddress(netip.MustParseAddr(ip)) {
			t.Fatalf("accepted %s", ip)
		}
	}
}

func TestDNSMixedAnswersAndRebindingNeverDialPrivateNetwork(t *testing.T) {
	var calls, dials atomic.Int32
	policy := Policy{Resolver: resolverFunc(func(context.Context, string) ([]net.IPAddr, error) {
		n := calls.Add(1)
		if n == 1 {
			return []net.IPAddr{{IP: net.ParseIP("93.184.216.34")}}, nil
		}
		return []net.IPAddr{{IP: net.ParseIP("93.184.216.34")}, {IP: net.ParseIP("127.0.0.1")}}, nil
	}), Dial: func(context.Context, string, string) (net.Conn, error) { dials.Add(1); return nil, ErrDenied }}
	if _, err := policy.Resolve(context.Background(), "external.test"); err != nil {
		t.Fatal(err)
	}
	client := NewClient(policy, time.Second)
	if _, err := client.Get("http://external.test"); err == nil {
		t.Fatal("mixed DNS accepted")
	}
	if dials.Load() != 0 {
		t.Fatal("dial occurred after changed private DNS answer")
	}
}

func TestAllowedPublicFixtureRedirectAndByteCaps(t *testing.T) {
	var requests atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests.Add(1)
		if r.URL.Path == "/redirect" {
			http.Redirect(w, r, "http://127.0.0.1/private", 302)
			return
		}
		_, _ = io.WriteString(w, "allowed")
	}))
	defer server.Close()
	endpoint, _ := url.Parse(server.URL)
	policy := Policy{Resolver: resolverFunc(func(context.Context, string) ([]net.IPAddr, error) {
		return []net.IPAddr{{IP: net.ParseIP("93.184.216.34")}}, nil
	}), Dial: func(ctx context.Context, network, address string) (net.Conn, error) {
		if address != "93.184.216.34:80" {
			t.Fatal("not pinned")
		}
		return (&net.Dialer{}).DialContext(ctx, network, endpoint.Host)
	}}
	client := NewClient(policy, time.Second)
	response, err := client.Get("http://external.test/")
	if err != nil {
		t.Fatal(err)
	}
	_ = response.Body.Close()
	response, err = client.Get("http://external.test/redirect")
	if err != nil || response.StatusCode != 302 {
		t.Fatal("redirect followed")
	}
	_ = response.Body.Close()
	request, _ := http.NewRequest("POST", "http://external.test/", strings.NewReader(strings.Repeat("x", int(MaxRequestBytes))))
	if _, err = client.Do(request); err == nil {
		t.Fatal("body plus headers exceeded allowance")
	}
	if requests.Load() != 2 {
		t.Fatalf("unexpected recipient effects: %d", requests.Load())
	}
}
