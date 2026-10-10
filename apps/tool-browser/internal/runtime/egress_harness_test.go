package runtime

import (
	"context"
	"net"
	"net/url"
	"os"
	"os/signal"
	"testing"
	"time"

	runtimev1 "github.com/butler/butler/internal/gen/runtime/v1"
	"github.com/butler/butler/internal/security/egress"
	"github.com/butler/butler/internal/security/egress/egresstest"
	"google.golang.org/grpc"
)

// This entry point exists only in a test binary. Broker/authority integration
// starts it as a separate process to exercise the real gRPC server and Chromium
// with the public-destination policy intact. Only its trusted dial seam routes
// the checked public fixture address to the parent's disposable HTTP listener.
func TestBrowserRuntimeEgressHarness(t *testing.T) {
	if os.Getenv("BUTLER_BROWSER_EGRESS_HARNESS") != "1" {
		t.Skip("test-only browser subprocess")
	}
	endpoint, err := url.Parse(os.Getenv("BUTLER_BROWSER_FIXTURE_ENDPOINT"))
	if err != nil || endpoint.Scheme != "http" || endpoint.User != nil || endpoint.Path != "" || endpoint.RawQuery != "" || endpoint.Fragment != "" || endpoint.Hostname() != "127.0.0.1" || endpoint.Port() == "" {
		t.Fatal("disposable loopback fixture endpoint required")
	}
	policy := egress.Policy{
		Resolver: egresstest.Resolver{},
		Dial: func(ctx context.Context, network, _ string) (net.Conn, error) {
			return (&net.Dialer{}).DialContext(ctx, network, endpoint.Host)
		},
	}
	runner := NewPlaywrightRunnerWithPolicy("node", os.Getenv("BUTLER_BROWSER_E2E_SCRIPT"), policy)
	server := NewServer(runner, nil)
	defer server.Close()
	listener, err := net.Listen("tcp", os.Getenv("BUTLER_GRPC_ADDR"))
	if err != nil {
		t.Fatal("fixture grpc listener unavailable")
	}
	defer listener.Close()
	rpc := grpc.NewServer()
	runtimev1.RegisterToolRuntimeServiceServer(rpc, server)
	done := make(chan error, 1)
	go func() { done <- rpc.Serve(listener) }()
	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt)
	defer cancel()
	timer := time.NewTimer(2 * time.Minute)
	defer timer.Stop()
	select {
	case <-ctx.Done():
	case <-timer.C:
		t.Error("browser subprocess fixture timed out")
	case err := <-done:
		if err != nil {
			t.Error("browser subprocess fixture stopped unexpectedly")
		}
	}
	rpc.Stop()
}
