package runtime

import (
	"context"
	"errors"
	"net"
	"net/url"
	"os"
	"os/signal"
	"strconv"
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
	lifetime, err := extensionHarnessLifetime(os.Getenv("BUTLER_EXTENSION_EGRESS_HARNESS_LIFETIME_MS"))
	if err != nil {
		t.Fatal("bounded fixture harness lifetime required")
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
	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt)
	defer cancel()
	if err := serveExtensionHarness(ctx, rpc, listener, lifetime); err != nil {
		t.Error(err)
	}
}

var errExtensionHarnessTimeout = errors.New("extension harness timed out")

// Test-only subprocess configuration. An absent override retains the original
// three-minute bound; the prospective fixture can pin at most forty minutes.
func extensionHarnessLifetime(raw string) (time.Duration, error) {
	if raw == "" {
		return 3 * time.Minute, nil
	}
	if len(raw) > 7 || raw[0] == '0' {
		return 0, errors.New("invalid fixture harness lifetime")
	}
	for _, digit := range raw {
		if digit < '0' || digit > '9' {
			return 0, errors.New("invalid fixture harness lifetime")
		}
	}
	millis, err := strconv.ParseInt(raw, 10, 64)
	if err != nil || millis < 1 || millis > (40*time.Minute).Milliseconds() {
		return 0, errors.New("invalid fixture harness lifetime")
	}
	return time.Duration(millis) * time.Millisecond, nil
}

func serveExtensionHarness(ctx context.Context, rpc *grpc.Server, listener net.Listener, lifetime time.Duration) error {
	defer rpc.Stop()
	done := make(chan error, 1)
	go func() { done <- rpc.Serve(listener) }()
	timer := time.NewTimer(lifetime)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return nil
	case <-timer.C:
		return errExtensionHarnessTimeout
	case err := <-done:
		if err != nil {
			return errors.New("extension harness unexpectedly stopped")
		}
		return nil
	}
}
