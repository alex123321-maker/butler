package runtime

import (
	"context"
	"net"
	"net/url"
	"os"
	"testing"
	"time"

	runtimev1 "github.com/butler/butler/internal/gen/runtime/v1"
	"github.com/butler/butler/internal/security/egress"
	"github.com/butler/butler/internal/security/egress/egresstest"
	"google.golang.org/grpc"
)

// This finite test subprocess is the only adapter that maps a synthetic public
// hostname to a loopback fixture. Production constructors and policy are intact.
func TestWebFetchRuntimeHarness(t *testing.T) {
	if os.Getenv("BUTLER_WEBFETCH_RUNTIME_HARNESS") != "1" {
		t.Skip("isolated Broker/runtime fixture subprocess")
	}
	endpoint, err := url.Parse(os.Getenv("BUTLER_WEBFETCH_FIXTURE_ENDPOINT"))
	if err != nil || endpoint.Scheme != "http" || endpoint.Hostname() != "127.0.0.1" || endpoint.Port() == "" || endpoint.Path != "" || endpoint.User != nil || endpoint.RawQuery != "" || endpoint.Fragment != "" {
		t.Fatal("fixed loopback fixture required")
	}
	address := os.Getenv("BUTLER_GRPC_ADDR")
	host, _, err := net.SplitHostPort(address)
	if err != nil || host != "127.0.0.1" {
		t.Fatal("private fixture listener required")
	}
	listener, err := net.Listen("tcp", address)
	if err != nil {
		t.Fatal(err)
	}
	defer listener.Close()
	policy := egress.Policy{Resolver: egresstest.Resolver{}, Dial: func(ctx context.Context, network, _ string) (net.Conn, error) {
		return (&net.Dialer{}).DialContext(ctx, network, endpoint.Host)
	}}
	rpc := grpc.NewServer()
	runtimev1.RegisterToolRuntimeServiceServer(rpc, NewServer(NewProviderChainWithPolicy(policy, NewPlainHTTPProvider(egress.NewClient(policy, 10*time.Second), true)), nil))
	defer rpc.Stop()
	done := make(chan error, 1)
	go func() { done <- rpc.Serve(listener) }()
	select {
	case err := <-done:
		t.Fatal(err)
	case <-time.After(3 * time.Minute):
		t.Fatal("bounded webfetch fixture lifetime expired")
	}
}
