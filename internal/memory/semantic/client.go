package semantic

import (
	"context"
	"os"
	"path/filepath"
	"strings"

	"github.com/butler/butler/internal/budget"
	pb "github.com/butler/butler/internal/gen/localmodels/v1"
	"github.com/butler/butler/internal/security/workspace"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/metadata"
)

// AuthenticatedBackend reads a private service token at the trusted composition
// root. It never places credential bytes in input, error text or a model item.
type AuthenticatedBackend struct {
	client pb.LocalModelsServiceClient
	token  string
}

func NewAuthenticatedBackend(conn grpc.ClientConnInterface, tokenFile string) (*AuthenticatedBackend, error) {
	info, err := os.Lstat(tokenFile)
	if err != nil || !info.Mode().IsRegular() || info.Mode().Perm()&0077 != 0 || info.Size() > 4096 {
		return nil, ErrUnavailable
	}
	raw, err := workspace.ReadRegular(filepath.Dir(tokenFile), filepath.Base(tokenFile), 4096)
	if err != nil {
		return nil, ErrUnavailable
	}
	if len(raw) > 4096 {
		return nil, ErrUnavailable
	}
	token := strings.TrimSpace(string(raw))
	if len(token) < 32 || len(token) > 256 || strings.ContainsAny(token, "\r\n") {
		return nil, ErrUnavailable
	}
	return &AuthenticatedBackend{client: pb.NewLocalModelsServiceClient(conn), token: token}, nil
}

// NewPinnedBackend is the production composition path. It dials only the
// operator-pinned internal target; token bytes stay in RPC metadata. Docker's
// private local-inference network is the trust boundary, not public gRPC TLS.
func NewPinnedBackend(target budget.LocalTarget, tokenFile string) (*AuthenticatedBackend, func(), error) {
	if target.Endpoint() != "local-models:9090" || target.Identity() == "" {
		return nil, nil, ErrUnavailable
	}
	conn, err := grpc.NewClient(target.Endpoint(), grpc.WithTransportCredentials(insecure.NewCredentials()), grpc.WithDefaultCallOptions(grpc.MaxCallRecvMsgSize(8<<20), grpc.MaxCallSendMsgSize(5<<20)))
	if err != nil {
		return nil, nil, ErrUnavailable
	}
	backend, err := NewAuthenticatedBackend(conn, tokenFile)
	if err != nil {
		_ = conn.Close()
		return nil, nil, err
	}
	return backend, func() { _ = conn.Close() }, nil
}
func (b *AuthenticatedBackend) ctx(ctx context.Context) context.Context {
	md, _ := metadata.FromOutgoingContext(ctx)
	md = md.Copy()
	md.Set("authorization", "Bearer "+b.token)
	return metadata.NewOutgoingContext(ctx, md)
}
func (b *AuthenticatedBackend) Describe(ctx context.Context, in *pb.Empty) (*pb.Catalog, error) {
	return b.client.Describe(b.ctx(ctx), in)
}
func (b *AuthenticatedBackend) Infer(ctx context.Context, in *pb.InferRequest) (*pb.InferResponse, error) {
	return b.client.Infer(b.ctx(ctx), in)
}
