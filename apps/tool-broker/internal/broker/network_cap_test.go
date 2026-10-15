package broker

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/butler/butler/apps/tool-broker/internal/registry"
	"github.com/butler/butler/internal/credentials"
	commonv1 "github.com/butler/butler/internal/gen/common/v1"
	toolbrokerv1 "github.com/butler/butler/internal/gen/toolbroker/v1"
)

type cappedExecutor struct{ calls int }

func (e *cappedExecutor) Execute(context.Context, *toolbrokerv1.ToolCall, *toolbrokerv1.ToolContract, []credentials.ResolvedSecret) (*toolbrokerv1.ToolResult, error) {
	e.calls++
	return &toolbrokerv1.ToolResult{Status: "completed"}, nil
}

func TestNetworkCapDeniesAllBrokerAdmissionPathsBeforeDispatch(t *testing.T) {
	path := filepath.Join(t.TempDir(), "tools.json")
	if err := os.WriteFile(path, []byte(`{"tools":[{"tool_name":"http.request","tool_class":"http","input_schema_json":"{\"type\":\"object\"}","allowed_domains":["a.example"]}]}`), 0600); err != nil {
		t.Fatal(err)
	}
	reg, err := registry.LoadWithOptions(path, "unused", registry.LoadOptions{ClassAllowedDomains: map[string][]string{"http": {"b.example"}}})
	if err != nil {
		t.Fatal(err)
	}
	executor := &cappedExecutor{}
	server := NewServer(reg, executor, nil, nil)
	call := &toolbrokerv1.ToolCall{ToolName: "http.request", ArgsJson: `{"url":"https://outside.invalid/"}`}
	validated, err := server.ValidateToolCall(context.Background(), &toolbrokerv1.ValidateToolCallRequest{ToolCall: call})
	if err != nil || validated.GetValid() {
		t.Fatal("validation accepted", err)
	}
	prepared, err := server.PrepareToolCall(context.Background(), &toolbrokerv1.PrepareToolCallRequest{ToolCall: call})
	if err != nil || prepared.GetError().GetErrorClass() != commonv1.ErrorClass_ERROR_CLASS_POLICY_DENIED {
		t.Fatal("prepare did not deny", err, prepared)
	}
	authorized, err := server.AuthorizeToolCall(context.Background(), &toolbrokerv1.AuthorizeToolCallRequest{ToolCall: call})
	if err != nil || authorized.GetError().GetErrorClass() != commonv1.ErrorClass_ERROR_CLASS_POLICY_DENIED {
		t.Fatal("authorize did not deny", err, authorized)
	}
	result, err := server.ExecuteToolCall(context.Background(), &toolbrokerv1.ExecuteToolCallRequest{ToolCall: call})
	if err != nil || result.GetResult().GetError().GetErrorClass() != commonv1.ErrorClass_ERROR_CLASS_POLICY_DENIED || executor.calls != 0 {
		t.Fatal("dispatch reached runtime", err, result)
	}
}
