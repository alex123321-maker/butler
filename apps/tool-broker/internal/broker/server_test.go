package broker

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/butler/butler/apps/tool-broker/internal/registry"
	toolbrokerv1 "github.com/butler/butler/internal/gen/toolbroker/v1"
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
	server := NewServer(reg, nil)

	if _, err := server.ListTools(context.Background(), &toolbrokerv1.ListToolsRequest{}); err != nil {
		t.Fatalf("ListTools returned error: %v", err)
	}

	_, err = server.GetToolContract(context.Background(), &toolbrokerv1.GetToolContractRequest{ToolName: "missing"})
	if status.Code(err) != codes.NotFound {
		t.Fatalf("expected not found error, got %v", err)
	}
}

func TestExecuteToolCallReturnsUnimplemented(t *testing.T) {
	t.Parallel()

	server := NewServer(&registry.Registry{}, nil)
	_, err := server.ExecuteToolCall(context.Background(), &toolbrokerv1.ExecuteToolCallRequest{ToolCall: &toolbrokerv1.ToolCall{ToolName: "browser.navigate"}})
	if status.Code(err) != codes.Unimplemented {
		t.Fatalf("expected unimplemented, got %v", err)
	}
}
