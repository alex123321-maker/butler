package workspaceexec

import (
	"context"
	"crypto/subtle"
	"net"
	"strconv"
	"time"

	"github.com/butler/butler/internal/actionauth"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/credentials/insecure"
	healthv1 "google.golang.org/grpc/health/grpc_health_v1"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"
)

const UnixSocket = "/run/butler-workspace/runtime.sock"
const UnixTarget = "unix://" + UnixSocket

// Fixed private Unix socket or literal loopback; no DNS resolver, proxy or
// service bearer over an unauthenticated LAN is supported.
func ValidTarget(target string) bool {
	if target == UnixTarget {
		return true
	}
	return ValidLoopback(target)
}
func ValidLoopback(target string) bool {
	host, port, err := net.SplitHostPort(target)
	ip := net.ParseIP(host)
	n, e := strconv.Atoi(port)
	return err == nil && ip != nil && ip.IsLoopback() && e == nil && n > 0 && n <= 65535
}
func ReadinessService(p Profile) string {
	hashes := []string{p.ID, p.Target, "workspace-runtime-v1"}
	for _, c := range Contracts(p.Target) {
		h, err := actionauth.ContractHash(c)
		if err != nil {
			return ""
		}
		hashes = append(hashes, h)
	}
	return "butler.workspace/" + bodyDigest([]byte(joinHashes(hashes)))
}
func joinHashes(v []string) string {
	raw := ""
	for _, h := range v {
		raw += strconv.Itoa(len(h)) + ":" + h
	}
	return raw
}

type runtimeHealth struct {
	healthv1.UnimplementedHealthServer
	runtime *Runtime
}

func NewHealth(r *Runtime) (healthv1.HealthServer, error) {
	if r == nil || r.dispatch == nil || !ValidTarget(r.dispatch.profile.Target) {
		return nil, ErrDenied
	}
	return &runtimeHealth{runtime: r}, nil
}
func (h *runtimeHealth) Check(ctx context.Context, req *healthv1.HealthCheckRequest) (*healthv1.HealthCheckResponse, error) {
	auth := metadata.ValueFromIncomingContext(ctx, "authorization")
	if len(auth) != 1 || subtle.ConstantTimeCompare([]byte(auth[0]), []byte("Bearer "+h.runtime.token)) != 1 {
		return nil, status.Error(codes.Unauthenticated, "private workspace readiness authentication required")
	}
	ctx, stop := context.WithTimeout(ctx, 2*time.Second)
	defer stop()
	if req.GetService() != ReadinessService(h.runtime.dispatch.profile) || h.runtime.dispatch.CheckReady(ctx) != nil || h.runtime.files.CheckRoot() != nil {
		return nil, status.Error(codes.FailedPrecondition, "exact installed workspace runtime is unavailable")
	}
	return &healthv1.HealthCheckResponse{Status: healthv1.HealthCheckResponse_SERVING}, nil
}
func CheckReadiness(ctx context.Context, p Profile, token string) error {
	if p.ID == "" || len(p.ID) > 128 || !ValidTarget(p.Target) || len(token) < 32 || len(token) > 4096 {
		return ErrDenied
	}
	ctx, stop := context.WithTimeout(ctx, 3*time.Second)
	defer stop()
	conn, err := grpc.NewClient(p.Target, grpc.WithTransportCredentials(insecure.NewCredentials()), grpc.WithNoProxy())
	if err != nil {
		return ErrDenied
	}
	defer conn.Close()
	ctx = metadata.AppendToOutgoingContext(ctx, "authorization", "Bearer "+token)
	r, err := healthv1.NewHealthClient(conn).Check(ctx, &healthv1.HealthCheckRequest{Service: ReadinessService(p)})
	if err != nil || r.GetStatus() != healthv1.HealthCheckResponse_SERVING {
		return ErrDenied
	}
	return nil
}
