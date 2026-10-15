package runtime

import (
	"context"
	"net"
	"net/url"
	"os"
	"os/signal"
	"strconv"
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
	lifetime, err := experienceHTTPHarnessLifetime(os.Getenv("BUTLER_EXPERIENCE_HTTP_HARNESS_LIFETIME_MS"))
	if err != nil {
		t.Fatal("finite fixture process lifetime required")
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
	timer := time.NewTimer(lifetime)
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

func experienceHTTPHarnessLifetime(raw string) (time.Duration, error) {
	if raw == "" {
		return 3 * time.Minute, nil
	}
	millis, err := strconv.ParseInt(raw, 10, 64)
	if err != nil || len(raw) > 7 || raw[0] == '0' || strconv.FormatInt(millis, 10) != raw || millis < 1 || millis > int64((40*time.Minute)/time.Millisecond) {
		return 0, strconv.ErrRange
	}
	return time.Duration(millis) * time.Millisecond, nil
}

func TestExperienceHTTPHarnessLifetimeIsFiniteAndLegacyDefaultUnchanged(t *testing.T) {
	for _, tc := range []struct {
		raw  string
		want time.Duration
	}{{"", 3 * time.Minute}, {"1", time.Millisecond}, {"180000", 3 * time.Minute}, {"2400000", 40 * time.Minute}} {
		if got, err := experienceHTTPHarnessLifetime(tc.raw); err != nil || got != tc.want {
			t.Fatal(tc.raw, got, err)
		}
	}
	for _, raw := range []string{"0", "01", "-1", "+1", " 1", "1 ", "1.0", "2400001", "99999999999999999999999"} {
		if _, err := experienceHTTPHarnessLifetime(raw); err == nil {
			t.Fatal("unbounded or ambiguous fixture lifetime", raw)
		}
	}
}
