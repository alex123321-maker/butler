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

// Only this test binary maps the reviewed public fixture origin to its one
// loopback recipient. Production HTTP runtime/SSRF/credential behavior is intact.
func TestExperienceHTTPRuntimeHarness(t *testing.T) {
	if os.Getenv("BUTLER_EXPERIENCE_HTTP_HARNESS") != "1" {
		t.Skip("isolated test-only HTTP runtime process")
	}
	endpoint, err := url.Parse(os.Getenv("BUTLER_EXPERIENCE_RECIPIENT_ENDPOINT"))
	if err != nil || endpoint.Scheme != "http" || endpoint.Hostname() != "127.0.0.1" || endpoint.Port() == "" || endpoint.Path != "" || endpoint.User != nil || endpoint.RawQuery != "" || endpoint.Fragment != "" {
		t.Fatal("fixed loopback recipient required")
	}
	address := os.Getenv("BUTLER_GRPC_ADDR")
	host, _, err := net.SplitHostPort(address)
	if err != nil || host != "127.0.0.1" {
		t.Fatal("private fixture RPC listener required")
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
	runtimev1.RegisterToolRuntimeServiceServer(rpc, NewServer(egress.NewClient(policy, 10*time.Second), nil))
	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt)
	defer cancel()
	timer := time.NewTimer(3 * time.Minute)
	defer timer.Stop()
	done := make(chan error, 1)
	go func() { done <- rpc.Serve(listener) }()
	select {
	case <-ctx.Done():
		rpc.GracefulStop()
		<-done
	case err := <-done:
		if err != nil {
			t.Fatal(err)
		}
	case <-timer.C:
		rpc.Stop()
		<-done
		t.Fatal("bounded experience HTTP fixture expired")
	}
}
