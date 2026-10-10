package broker

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/butler/butler/apps/tool-broker/internal/registry"
	"github.com/butler/butler/internal/credentials"
	commonv1 "github.com/butler/butler/internal/gen/common/v1"
	toolbrokerv1 "github.com/butler/butler/internal/gen/toolbroker/v1"
)

type schemaResultExecutor struct{ result *toolbrokerv1.ToolResult }

func (e schemaResultExecutor) Execute(context.Context, *toolbrokerv1.ToolCall, *toolbrokerv1.ToolContract, []credentials.ResolvedSecret) (*toolbrokerv1.ToolResult, error) {
	return e.result, nil
}

func TestCompletedRuntimeResultMustSatisfySchemaBeforeEvidence(t *testing.T) {
	for _, tc := range []struct {
		name, raw    string
		runtimeError bool
	}{
		{"invalid_json", "{", false}, {"wrong_field_type", `{"status_code":"success"}`, false},
		{"completed_with_error", `{"status_code":200}`, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			config := map[string]any{"tools": []any{map[string]any{"tool_name": "http.request", "status": "enabled", "input_schema_json": `{"type":"object"}`, "output_schema_json": `{"type":"object","required":["status_code"],"properties":{"status_code":{"type":"integer"}},"additionalProperties":false}`}}}
			raw, err := json.Marshal(config)
			if err != nil {
				t.Fatal(err)
			}
			path := filepath.Join(t.TempDir(), "tools.json")
			if err = os.WriteFile(path, raw, 0600); err != nil {
				t.Fatal(err)
			}
			reg, err := registry.Load(path, "local")
			if err != nil {
				t.Fatal(err)
			}
			runtimeResult := &toolbrokerv1.ToolResult{Status: "completed", ResultJson: tc.raw}
			if tc.runtimeError {
				runtimeResult.Error = &toolbrokerv1.ToolError{ErrorClass: commonv1.ErrorClass_ERROR_CLASS_TOOL_ERROR}
			}
			server := NewServer(reg, schemaResultExecutor{runtimeResult}, nil, nil)
			fixture := setTestAuthority(server, nil)
			result := dispatchForTest(t, server, fixture, &toolbrokerv1.ToolCall{ToolCallId: "call", RunId: "run", ToolName: "http.request", ArgsJson: `{"method":"GET","url":"https://example.com"}`}, true)
			if result.GetStatus() != "effect_unknown" || result.GetEffectState() != "effect_unknown" || result.GetResultJson() != "{}" {
				t.Fatalf("invalid success became confirmed evidence: %+v", result)
			}
		})
	}
}
