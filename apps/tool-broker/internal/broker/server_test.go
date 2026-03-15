package broker

import (
	"context"
	"net"
	"os"
	"path/filepath"
	"testing"

	"github.com/butler/butler/apps/tool-broker/internal/registry"
	"github.com/butler/butler/apps/tool-broker/internal/runtimeclient"
	"github.com/butler/butler/internal/credentials"
	runtimev1 "github.com/butler/butler/internal/gen/runtime/v1"
	toolbrokerv1 "github.com/butler/butler/internal/gen/toolbroker/v1"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

func TestServerListAndGet(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	path := filepath.Join(dir, "tools.json")
	if err := os.WriteFile(path, []byte(`{"tools":[{"tool_name":"browser.navigate","tool_class":"browser","input_schema_json":"{\"type\":\"object\"}","status":"enabled"}]}`), 0o600); err != nil {
		t.Fatalf("write registry file: %v", err)
	}
	reg, err := registry.Load(path, "local")
	if err != nil {
		t.Fatalf("registry.Load returned error: %v", err)
	}
	server := NewServer(reg, nil, nil, nil)

	if _, err := server.ListTools(context.Background(), &toolbrokerv1.ListToolsRequest{}); err != nil {
		t.Fatalf("ListTools returned error: %v", err)
	}

	_, err = server.GetToolContract(context.Background(), &toolbrokerv1.GetToolContractRequest{ToolName: "missing"})
	if status.Code(err) != codes.NotFound {
		t.Fatalf("expected not found error, got %v", err)
	}
}

func TestExecuteToolCallRoutesToRuntime(t *testing.T) {
	t.Parallel()

	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	grpcServer := grpc.NewServer()
	runtimev1.RegisterToolRuntimeServiceServer(grpcServer, runtimeStub{})
	go func() {
		_ = grpcServer.Serve(listener)
	}()
	defer grpcServer.Stop()

	dir := t.TempDir()
	path := filepath.Join(dir, "tools.json")
	registryJSON := `{"tools":[{"tool_name":"browser.navigate","tool_class":"browser","runtime_target":"` + listener.Addr().String() + `","input_schema_json":"{\"type\":\"object\",\"required\":[\"url\"],\"properties\":{\"url\":{\"type\":\"string\"}},\"additionalProperties\":false}","status":"enabled"}]}`
	if err := os.WriteFile(path, []byte(registryJSON), 0o600); err != nil {
		t.Fatalf("write registry file: %v", err)
	}
	reg, err := registry.Load(path, "local")
	if err != nil {
		t.Fatalf("registry.Load returned error: %v", err)
	}

	server := NewServer(reg, runtimeclient.New(nil), nil, nil)

	resp, err := server.ExecuteToolCall(context.Background(), &toolbrokerv1.ExecuteToolCallRequest{ToolCall: &toolbrokerv1.ToolCall{ToolCallId: "tool-1", RunId: "run-1", ToolName: "browser.navigate", ArgsJson: `{"url":"https://example.com"}`}})
	if err != nil {
		t.Fatalf("ExecuteToolCall returned error: %v", err)
	}
	if resp.GetResult().GetStatus() != "completed" {
		t.Fatalf("expected completed result, got %+v", resp.GetResult())
	}
}

func TestExecuteToolCallResolvesCredentials(t *testing.T) {
	t.Parallel()

	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	grpcServer := grpc.NewServer()
	runtimev1.RegisterToolRuntimeServiceServer(grpcServer, credentialRuntimeStub{t: t})
	go func() {
		_ = grpcServer.Serve(listener)
	}()
	defer grpcServer.Stop()

	dir := t.TempDir()
	path := filepath.Join(dir, "tools.json")
	registryJSON := `{"tools":[{"tool_name":"http.request","tool_class":"http","runtime_target":"` + listener.Addr().String() + `","supports_credential_refs":true,"input_schema_json":"{\"type\":\"object\",\"required\":[\"method\",\"url\"],\"properties\":{\"method\":{\"type\":\"string\"},\"url\":{\"type\":\"string\"}},\"additionalProperties\":true}","status":"enabled"}]}`
	if err := os.WriteFile(path, []byte(registryJSON), 0o600); err != nil {
		t.Fatalf("write registry file: %v", err)
	}
	reg, err := registry.Load(path, "local")
	if err != nil {
		t.Fatalf("registry.Load returned error: %v", err)
	}

	server := NewServer(reg, runtimeclient.New(nil), stubCredentialResolver{}, nil)
	resp, err := server.ExecuteToolCall(context.Background(), &toolbrokerv1.ExecuteToolCallRequest{ToolCall: &toolbrokerv1.ToolCall{ToolCallId: "tool-1", RunId: "run-1", ToolName: "http.request", ArgsJson: `{"method":"GET","url":"https://example.com"}`, CredentialRefs: []*toolbrokerv1.CredentialRef{{Type: "credential_ref", Alias: "github", Field: "token"}}}})
	if err != nil {
		t.Fatalf("ExecuteToolCall returned error: %v", err)
	}
	if resp.GetResult().GetStatus() != "completed" {
		t.Fatalf("expected completed result, got %+v", resp.GetResult())
	}
}

type runtimeStub struct {
	runtimev1.UnimplementedToolRuntimeServiceServer
}

func (runtimeStub) Execute(_ context.Context, req *runtimev1.ExecuteRequest) (*runtimev1.ExecuteResponse, error) {
	if len(req.GetResolvedCredentials()) != 0 {
		return nil, status.Error(codes.Internal, "unexpected resolved credentials")
	}
	return &runtimev1.ExecuteResponse{Result: &toolbrokerv1.ToolResult{ToolCallId: req.GetToolCall().GetToolCallId(), RunId: req.GetToolCall().GetRunId(), ToolName: req.GetToolCall().GetToolName(), Status: "completed", ResultJson: `{"ok":true}`}}, nil
}

type credentialRuntimeStub struct {
	runtimev1.UnimplementedToolRuntimeServiceServer
	t *testing.T
}

func (s credentialRuntimeStub) Execute(_ context.Context, req *runtimev1.ExecuteRequest) (*runtimev1.ExecuteResponse, error) {
	if len(req.GetResolvedCredentials()) != 1 || req.GetResolvedCredentials()[0].GetValue() != "secret-token" {
		s.t.Fatalf("unexpected resolved credentials: %+v", req.GetResolvedCredentials())
	}
	return &runtimev1.ExecuteResponse{Result: &toolbrokerv1.ToolResult{ToolCallId: req.GetToolCall().GetToolCallId(), RunId: req.GetToolCall().GetRunId(), ToolName: req.GetToolCall().GetToolName(), Status: "completed", ResultJson: `{"ok":true}`}}, nil
}

type stubCredentialResolver struct{}

func (stubCredentialResolver) ResolveToolCall(context.Context, *toolbrokerv1.ToolCall) ([]credentials.ResolvedSecret, error) {
	return []credentials.ResolvedSecret{{Alias: "github", Field: "token", Value: "secret-token"}}, nil
}
