package resourceexec

import (
	"context"
	"net"
	"strings"
	"sync/atomic"
	"testing"

	"google.golang.org/grpc"
	healthv1 "google.golang.org/grpc/health/grpc_health_v1"
)

func TestSingleTabReadinessExactPrivateRoute(t *testing.T) {
	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	token := strings.Repeat("private-fixture-", 3)
	var current atomic.Bool
	current.Store(true)
	h, err := NewSingleTabHealth(l.Addr().String(), token, func(context.Context) (string, error) {
		if !current.Load() {
			return "", ErrDenied
		}
		return "fixed-host", nil
	})
	if err != nil {
		t.Fatal(err)
	}
	s := grpc.NewServer()
	healthv1.RegisterHealthServer(s, h)
	go s.Serve(l)
	defer s.Stop()
	defer l.Close()
	for _, scenario := range []string{"current", "wrong_host", "wrong_token", "unavailable_extension"} {
		t.Run(scenario, func(t *testing.T) {
			host, auth := "fixed-host", token
			if scenario == "wrong_host" {
				host = "replacement-host"
			}
			if scenario == "wrong_token" {
				auth = strings.Repeat("wrong-", 8)
			}
			if scenario == "unavailable_extension" {
				current.Store(false)
				defer func() { current.Store(true) }()
			}
			err := CheckSingleTabReadiness(context.Background(), l.Addr().String(), host, auth)
			if (err == nil) != (scenario == "current") {
				t.Fatal("readiness boundary mismatch", err)
			}
		})
	}
	for _, target := range []string{"localhost:19090", "192.168.1.2:19090", "dns:///127.0.0.1:19090", "127.0.0.1:0", "127.0.0.1:65536"} {
		if ValidSingleTabTarget(target) {
			t.Fatal("unsafe target accepted", target)
		}
	}
	for _, c := range SingleTabContracts(l.Addr().String()) {
		if !ExactSingleTabContract(c) {
			t.Fatal("owned contract denied")
		}
		c.OutputSchemaJson = `{"type":"object"}`
		if ExactSingleTabContract(c) {
			t.Fatal("modified output contract accepted")
		}
	}
}
