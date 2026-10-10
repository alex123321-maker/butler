package runtime

import (
	"context"
	"net"
	"net/url"
	"os"
	"os/signal"
	"testing"
	"time"

	"github.com/butler/butler/internal/actionauth"
	"github.com/butler/butler/internal/extensions"
	runtimev1 "github.com/butler/butler/internal/gen/runtime/v1"
	"github.com/butler/butler/internal/security/egress"
	"github.com/butler/butler/internal/security/egress/egresstest"
	"google.golang.org/grpc"
)

// Only a test binary contains this entry point. The production interpreter,
// dedicated RPC authentication and public destination policy remain unchanged.
func TestExtensionRuntimeEgressHarness(t *testing.T) {
	if os.Getenv("BUTLER_EXTENSION_EGRESS_HARNESS") != "1" {
		t.Skip("test-only runtime subprocess")
	}
	endpoint, err := url.Parse(os.Getenv("BUTLER_EXTENSION_FIXTURE_ENDPOINT"))
	if err != nil || endpoint.Scheme != "http" || endpoint.Hostname() != "127.0.0.1" || endpoint.Port() == "" || endpoint.Path != "" || endpoint.User != nil || endpoint.RawQuery != "" || endpoint.Fragment != "" {
		t.Fatal("disposable loopback fixture required")
	}
	profiles, err := extensions.ParseProfiles(os.Getenv("BUTLER_EXTENSION_PROFILES_JSON"))
	if err != nil {
		t.Fatal("fixture profile invalid")
	}
	token, err := actionauth.ReadServiceToken(os.Getenv("BUTLER_EXTENSION_RUNTIME_TOKEN_FILE"))
	if err != nil {
		t.Fatal("dedicated fixture token unavailable")
	}
	policy := egress.Policy{Resolver: egresstest.Resolver{}, Dial: func(ctx context.Context, network, _ string) (net.Conn, error) {
		return (&net.Dialer{}).DialContext(ctx, network, endpoint.Host)
	}}
	listener, err := net.Listen("tcp", os.Getenv("BUTLER_GRPC_ADDR"))
	if err != nil {
		t.Fatal("fixture listener unavailable")
	}
	defer listener.Close()
	rpc := grpc.NewServer(grpc.UnaryInterceptor(extensions.RuntimeAuthentication(token)))
	runtimev1.RegisterToolRuntimeServiceServer(rpc, NewServerWithEgress(profiles, policy))
	done := make(chan error, 1)
	go func() { done <- rpc.Serve(listener) }()
	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt)
	defer cancel()
	timer := time.NewTimer(3 * time.Minute)
	defer timer.Stop()
	select {
	case <-ctx.Done():
	case <-timer.C:
		t.Error("extension harness timed out")
	case err := <-done:
		if err != nil {
			t.Error("extension harness unexpectedly stopped")
		}
	}
	rpc.Stop()
}
