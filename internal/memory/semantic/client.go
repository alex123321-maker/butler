package semantic

import (
	"context"
	"io"
	"os"
	"strings"

	pb "github.com/butler/butler/internal/gen/localmodels/v1"
	"google.golang.org/grpc"
	"google.golang.org/grpc/metadata"
)

// AuthenticatedBackend reads a private service token at the trusted composition
// root. It never places credential bytes in input, error text or a model item.
type AuthenticatedBackend struct {
	client pb.LocalModelsServiceClient
	token  string
}

func NewAuthenticatedBackend(conn grpc.ClientConnInterface, tokenFile string) (*AuthenticatedBackend, error) {
	info, err := os.Stat(tokenFile)
	if err != nil || !info.Mode().IsRegular() || info.Size() > 4096 {
		return nil, ErrUnavailable
	}
	file, err := os.Open(tokenFile)
	if err != nil {
		return nil, ErrUnavailable
	}
	defer file.Close()
	raw, err := io.ReadAll(io.LimitReader(file, 4097))
	if err != nil {
		return nil, ErrUnavailable
	}
	if len(raw) > 4096 {
		return nil, ErrUnavailable
	}
	token := strings.TrimSpace(string(raw))
	if token == "" || strings.ContainsAny(token, "\r\n") {
		return nil, ErrUnavailable
	}
	return &AuthenticatedBackend{client: pb.NewLocalModelsServiceClient(conn), token: token}, nil
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
