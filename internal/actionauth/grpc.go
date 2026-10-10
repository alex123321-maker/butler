package actionauth

import (
	"context"
	"crypto/subtle"
	"strings"

	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"
)

// ExecutionUnaryInterceptor protects the private prepare/authorize/dispatch
// boundary. Knowing a run_id is not evidence of being its Butler worker.
func ExecutionUnaryInterceptor(token string) grpc.UnaryServerInterceptor {
	return func(ctx context.Context, request any, info *grpc.UnaryServerInfo, handler grpc.UnaryHandler) (any, error) {
		switch {
		case strings.HasSuffix(info.FullMethod, "/PrepareToolCall"), strings.HasSuffix(info.FullMethod, "/AuthorizeToolCall"), strings.HasSuffix(info.FullMethod, "/ExecuteToolCall"):
			values := metadata.ValueFromIncomingContext(ctx, "authorization")
			if len(token) < 32 || len(values) != 1 || subtle.ConstantTimeCompare([]byte(values[0]), []byte("Bearer "+token)) != 1 {
				return nil, status.Error(codes.Unauthenticated, "private action service authentication required")
			}
		}
		return handler(ctx, request)
	}
}

type ServiceTokenCredentials struct{ Token string }

func (c ServiceTokenCredentials) GetRequestMetadata(context.Context, ...string) (map[string]string, error) {
	return map[string]string{"authorization": "Bearer " + c.Token}, nil
}

// The baseline backend network has no TLS. The service token remains private
// to the trusted backend; an external deployment must use encrypted transport.
func (ServiceTokenCredentials) RequireTransportSecurity() bool { return false }
