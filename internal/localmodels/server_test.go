package localmodels

import (
	"context"
	"net"
	"strings"
	"testing"

	pb "github.com/butler/butler/internal/gen/localmodels/v1"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"
	"google.golang.org/grpc/test/bufconn"
)

func TestAuthenticatedGRPCAndSafeErrorStates(t *testing.T) {
	const token = "private-test-token-32-characters-long"
	b := &fakeBackend{fn: func(context.Context, *pb.ModelProfile, *pb.InferRequest, string, uint32) (*pb.InferResponse, error) {
		return nil, fault(Protocol)
	}}
	m := newManager(t, Config{}, []*pb.ModelProfile{profile()}, b, &fakeTelemetry{used: 17})
	listener := bufconn.Listen(1 << 20)
	server := grpc.NewServer(grpc.UnaryInterceptor(AuthInterceptor(token)))
	pb.RegisterLocalModelsServiceServer(server, &Server{Manager: m})
	go func() { _ = server.Serve(listener) }()
	t.Cleanup(server.Stop)
	connection, err := grpc.NewClient("passthrough:///buffer", grpc.WithContextDialer(func(context.Context, string) (net.Conn, error) { return listener.Dial() }), grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		t.Fatal(err)
	}
	defer connection.Close()
	client := pb.NewLocalModelsServiceClient(connection)
	if _, err := client.Describe(context.Background(), &pb.Empty{}); status.Code(err) != codes.Unauthenticated {
		t.Fatalf("anonymous access: %v", err)
	}
	ctx := metadata.AppendToOutgoingContext(context.Background(), "authorization", "Bearer "+token)
	catalog, err := client.Describe(ctx, &pb.Empty{})
	if err != nil || len(catalog.Models) != 1 {
		t.Fatalf("authenticated catalog: %v %v", catalog, err)
	}
	r := request("safe")
	r.GetEmbed().Texts[0].Text = "secret input must not appear in errors"
	_, err = client.Infer(ctx, r)
	if status.Code(err) != codes.DataLoss || strings.Contains(err.Error(), "secret") {
		t.Fatalf("unsafe/fake-success response: %v", err)
	}
	r.ProfileId = "missing"
	_, err = client.Infer(ctx, r)
	if status.Code(err) != codes.Unavailable {
		t.Fatalf("unknown model: %v", err)
	}
	r = request("invalid")
	r.GetEmbed().Dimensions = 128
	_, err = client.Infer(ctx, r)
	if status.Code(err) != codes.InvalidArgument {
		t.Fatalf("unconfigured dimension: %v", err)
	}
}
