package acp

import (
	"context"
	"io"
	"net/http"
	"strings"
	"testing"

	"github.com/butler/butler/internal/security/egress"
)

type dnsFixtureTransport struct{ body string }

func (fixture dnsFixtureTransport) RoundTrip(request *http.Request) (*http.Response, error) {
	return &http.Response{StatusCode: 200, Body: io.NopCloser(strings.NewReader(fixture.body)), Header: http.Header{}, Request: request}, nil
}

func TestGoogleDoHRetainsPrivateAndFakeIPDenial(t *testing.T) {
	for _, address := range []string{"127.0.0.1", "169.254.169.254", "198.18.0.67", "192.168.0.5", "8.8.8.8"} {
		resolver := &googleDNSResolver{client: &http.Client{Transport: dnsFixtureTransport{body: `{"Status":0,"Answer":[{"type":1,"data":"` + address + `"}]}`}}}
		policy := egress.Policy{AllowedDomains: []string{"oauth2.googleapis.com"}, Resolver: resolver}
		_, err := policy.Resolve(context.Background(), "oauth2.googleapis.com")
		if (err == nil) != (address == "8.8.8.8") {
			t.Fatal("DoH bypassed checked public-address policy", address, err)
		}
	}
	for _, body := range []string{`{"Status":0,"Status":0}`, `{"Status":0}`, `{"Status":3,"Answer":[{"type":1,"data":"8.8.8.8"}]}`} {
		resolver := &googleDNSResolver{client: &http.Client{Transport: dnsFixtureTransport{body: body}}}
		if _, err := resolver.LookupIPAddr(context.Background(), "oauth2.googleapis.com"); err == nil {
			t.Fatal("invalid DoH metadata accepted")
		}
	}
}
