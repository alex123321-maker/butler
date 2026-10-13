package runtime

import (
	"context"
	"errors"
	"net"
	"strings"
	"testing"
	"time"

	"github.com/butler/butler/internal/extensions"
	runtimev1 "github.com/butler/butler/internal/gen/runtime/v1"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"
)

func TestExtensionHarnessLifetimeBounds(t *testing.T) {
	for raw, expected := range map[string]time.Duration{"": 3 * time.Minute, "1": time.Millisecond, "180000": 3 * time.Minute, "2400000": 40 * time.Minute} {
		got, err := extensionHarnessLifetime(raw)
		if err != nil || got != expected {
			t.Fatal("valid bounded lifetime rejected")
		}
	}
	for _, raw := range []string{"0", "2400001", "-1", "+1", "01", "1.0", "1e3", " 1", "1 ", "1\n", "99999999999999999999", "１"} {
		if _, err := extensionHarnessLifetime(raw); err == nil {
			t.Fatal("invalid or unbounded lifetime accepted")
		}
	}
}

// Uses the real service and its private authentication interceptor; no HTTP
// request is sent because the deliberately incomplete probe is denied by policy.
func TestExtensionHarnessRPCLifecycle(t *testing.T) {
	for _, mode := range []string{"timer", "cancel"} {
		t.Run(mode, func(t *testing.T) {
			listener, err := net.Listen("tcp", "127.0.0.1:0")
			if err != nil {
				t.Fatal("loopback listener unavailable")
			}
			defer listener.Close()
			token := strings.Repeat("synthetic-runtime-", 3)
			rpc := grpc.NewServer(grpc.UnaryInterceptor(extensions.RuntimeAuthentication(token)))
			defer rpc.Stop()
			runtimev1.RegisterToolRuntimeServiceServer(rpc, NewServer(nil))
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			done := make(chan error, 1)
			go func() { done <- serveExtensionHarness(ctx, rpc, listener, 1500*time.Millisecond) }()
			connection, err := grpc.NewClient(listener.Addr().String(), grpc.WithTransportCredentials(insecure.NewCredentials()))
			if err != nil {
				t.Fatal("loopback RPC client unavailable")
			}
			defer connection.Close()
			client := runtimev1.NewToolRuntimeServiceClient(connection)
			probe := func(authenticated bool) codes.Code {
				call, cancelCall := context.WithTimeout(context.Background(), time.Second)
				defer cancelCall()
				if authenticated {
					call = metadata.NewOutgoingContext(call, metadata.Pairs("authorization", "Bearer "+token))
				}
				_, err := client.ProbeExtension(call, &runtimev1.ProbeExtensionRequest{})
				return status.Code(err)
			}
			if probe(false) != codes.Unauthenticated || probe(true) != codes.PermissionDenied {
				t.Fatal("live runtime authentication or policy boundary failed")
			}
			if mode == "cancel" {
				cancel()
			}
			select {
			case err := <-done:
				if mode == "timer" && !errors.Is(err, errExtensionHarnessTimeout) || mode == "cancel" && err != nil {
					t.Fatal("wrong runtime lifecycle result")
				}
			case <-time.After(3 * time.Second):
				t.Fatal("runtime did not stop within its bound")
			}
			if code := probe(true); code != codes.Unavailable && code != codes.DeadlineExceeded {
				t.Fatal("stopped runtime still accepts RPC")
			}
		})
	}
}
