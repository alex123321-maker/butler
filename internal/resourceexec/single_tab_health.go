package resourceexec

import (
	"context"
	"crypto/subtle"
	"net"
	"strconv"
	"time"

	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/credentials/insecure"
	healthv1 "google.golang.org/grpc/health/grpc_health_v1"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"
)

// A direct literal-loopback route is the supported topology. No resolver,
// proxy, fallback or bearer transport across an unauthenticated LAN is allowed.
func ValidSingleTabTarget(target string) bool {
	host, port, err := net.SplitHostPort(target)
	ip := net.ParseIP(host)
	n, pe := strconv.Atoi(port)
	return err == nil && ip != nil && ip.IsLoopback() && pe == nil && n > 0 && n <= 65535
}

type singleTabHealth struct {
	healthv1.UnimplementedHealthServer
	target, token string
	host          func(context.Context) (string, error)
}

// Host is read from the actual guarded extension outside database transactions.
// This probe grants no execution ticket and calls no browser action.
func NewSingleTabHealth(target, token string, host func(context.Context) (string, error)) (healthv1.HealthServer, error) {
	if !ValidSingleTabTarget(target) || !validServiceToken(token) || host == nil {
		return nil, ErrDenied
	}
	return &singleTabHealth{target: target, token: token, host: host}, nil
}

func (s *singleTabHealth) Check(ctx context.Context, request *healthv1.HealthCheckRequest) (*healthv1.HealthCheckResponse, error) {
	auth := metadata.ValueFromIncomingContext(ctx, "authorization")
	if len(auth) != 1 || subtle.ConstantTimeCompare([]byte(auth[0]), []byte("Bearer "+s.token)) != 1 {
		return nil, status.Error(codes.Unauthenticated, "private guarded readiness authentication required")
	}
	ctx, cancel := context.WithTimeout(ctx, 2*time.Second)
	defer cancel()
	host, err := s.host(ctx)
	if err != nil || host == "" || len(host) > 128 || request.GetService() != SingleTabReadinessService(s.target, host) {
		return nil, status.Error(codes.FailedPrecondition, "exact current guarded runtime required")
	}
	return &healthv1.HealthCheckResponse{Status: healthv1.HealthCheckResponse_SERVING}, nil
}

func CheckSingleTabReadiness(ctx context.Context, target, host, token string) error {
	if !ValidSingleTabTarget(target) || host == "" || len(host) > 128 || !validServiceToken(token) {
		return ErrDenied
	}
	ctx, cancel := context.WithTimeout(ctx, 3*time.Second)
	defer cancel()
	conn, err := grpc.NewClient(target, grpc.WithTransportCredentials(insecure.NewCredentials()), grpc.WithNoProxy())
	if err != nil {
		return ErrDenied
	}
	defer conn.Close()
	ctx = metadata.AppendToOutgoingContext(ctx, "authorization", "Bearer "+token)
	r, err := healthv1.NewHealthClient(conn).Check(ctx, &healthv1.HealthCheckRequest{Service: SingleTabReadinessService(target, host)})
	if err != nil || r.GetStatus() != healthv1.HealthCheckResponse_SERVING {
		return ErrDenied
	}
	return nil
}
