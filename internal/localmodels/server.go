package localmodels

import (
	"context"
	"crypto/sha256"
	"crypto/subtle"
	"errors"
	"strings"
	"time"

	pb "github.com/butler/butler/internal/gen/localmodels/v1"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"
)

type Server struct {
	pb.UnimplementedLocalModelsServiceServer
	Manager interface {
		Catalog() *pb.Catalog
		Status(context.Context) *pb.ManagerStatus
		Infer(context.Context, *pb.InferRequest) (*pb.InferResponse, error)
	}
}

func (s *Server) Describe(context.Context, *pb.Empty) (*pb.Catalog, error) {
	return s.Manager.Catalog(), nil
}
func (s *Server) Status(ctx context.Context, _ *pb.Empty) (*pb.ManagerStatus, error) {
	ctx, cancel := context.WithTimeout(ctx, 2*time.Second)
	defer cancel()
	return s.Manager.Status(ctx), nil
}
func (s *Server) Infer(ctx context.Context, r *pb.InferRequest) (*pb.InferResponse, error) {
	response, err := s.Manager.Infer(ctx, r)
	if err == nil {
		return response, nil
	}
	if errors.Is(err, context.Canceled) {
		return nil, status.Error(codes.Canceled, "request canceled")
	}
	if errors.Is(err, context.DeadlineExceeded) {
		return nil, status.Error(codes.DeadlineExceeded, "request deadline exceeded")
	}
	var f *Fault
	if errors.As(err, &f) {
		code := codes.Unavailable
		switch f.Kind {
		case Invalid:
			code = codes.InvalidArgument
		case Resources, OOM, QueueFull:
			code = codes.ResourceExhausted
		case Protocol:
			code = codes.DataLoss
		}
		return nil, status.Error(code, string(f.Kind))
	}
	return nil, status.Error(codes.Unavailable, "local model unavailable")
}

// Authentication runs before any model action. Tokens never enter model input.
func AuthInterceptor(token string) grpc.UnaryServerInterceptor {
	expected := sha256.Sum256([]byte(token))
	return func(ctx context.Context, request any, _ *grpc.UnaryServerInfo, handler grpc.UnaryHandler) (any, error) {
		md, _ := metadata.FromIncomingContext(ctx)
		values := md.Get("authorization")
		if len(values) != 1 || !strings.HasPrefix(values[0], "Bearer ") {
			return nil, status.Error(codes.Unauthenticated, "authentication required")
		}
		supplied := sha256.Sum256([]byte(strings.TrimPrefix(values[0], "Bearer ")))
		if subtle.ConstantTimeCompare(expected[:], supplied[:]) != 1 {
			return nil, status.Error(codes.Unauthenticated, "authentication required")
		}
		return handler(ctx, request)
	}
}
