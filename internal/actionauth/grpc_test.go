package actionauth

import (
	"context"
	"strings"
	"testing"

	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"
)

func TestExecutionRPCRequiresPrivateCallerAuthority(t *testing.T) {
	token := strings.Repeat("t", 48)
	interceptor := ExecutionUnaryInterceptor(token)
	for _, method := range []string{"PrepareToolCall", "AuthorizeToolCall", "ExecuteToolCall"} {
		info := &grpc.UnaryServerInfo{FullMethod: "/butler.toolbroker.v1.ToolBrokerService/" + method}
		calls := 0
		handler := func(context.Context, any) (any, error) { calls++; return nil, nil }
		for _, credential := range []string{"", "Bearer wrong"} {
			ctx := metadata.NewIncomingContext(context.Background(), metadata.Pairs("authorization", credential))
			if _, err := interceptor(ctx, nil, info, handler); status.Code(err) != codes.Unauthenticated {
				t.Fatal("execution RPC accepted untrusted caller")
			}
		}
		if calls != 0 {
			t.Fatal("execution handler reached before private caller authentication")
		}
		ctx := metadata.NewIncomingContext(context.Background(), metadata.Pairs("authorization", "Bearer "+token))
		if _, err := interceptor(ctx, nil, info, handler); err != nil || calls != 1 {
			t.Fatal("trusted caller rejected")
		}
	}
}
