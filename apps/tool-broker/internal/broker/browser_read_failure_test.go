package broker

import (
	"context"
	"errors"
	"path/filepath"
	"strings"
	"testing"

	"github.com/butler/butler/apps/tool-broker/internal/registry"
	"github.com/butler/butler/internal/credentials"
	commonv1 "github.com/butler/butler/internal/gen/common/v1"
	toolbrokerv1 "github.com/butler/butler/internal/gen/toolbroker/v1"
)

type browserReadRejectExecutor struct{ missing, foreign, lost bool }

func (e browserReadRejectExecutor) Execute(_ context.Context, c *toolbrokerv1.ToolCall, _ *toolbrokerv1.ToolContract, _ []credentials.ResolvedSecret) (*toolbrokerv1.ToolResult, error) {
	if e.lost {
		return nil, errors.New("private lost acknowledgement")
	}
	details := `{"read_rejection":"pre_navigation_v1","navigation_dispatched":false,"reason":"page_changed"}`
	if e.missing {
		details = `{"reason":"page_changed"}`
	}
	id := c.ToolCallId
	if e.foreign {
		id = "other"
	}
	return &toolbrokerv1.ToolResult{RunId: c.RunId, ToolCallId: id, ToolName: c.ToolName, Status: "failed", Error: &toolbrokerv1.ToolError{ErrorClass: commonv1.ErrorClass_ERROR_CLASS_POLICY_DENIED, Message: "PRIVATE_RUNTIME_MESSAGE", DetailsJson: details, Retryable: true}}, nil
}

func TestGuardedBrowserReadFailurePreservesUnknownUnlessProvedNotDispatched(t *testing.T) {
	for _, mode := range []string{"proved", "missing_proof", "foreign_call", "lost_ack", "opaque_click"} {
		t.Run(mode, func(t *testing.T) {
			reg, err := registry.Load(filepath.Join("..", "..", "..", "..", "configs", "tools.json"), "local")
			if err != nil {
				t.Fatal(err)
			}
			s := NewServer(reg, browserReadRejectExecutor{missing: mode == "missing_proof", foreign: mode == "foreign_call", lost: mode == "lost_ack"}, nil, nil)
			f := setTestAuthority(s, nil)
			tool, args := "browser.submit_get_form", `{"browser_session_id":"context","selector":"form","fields":[],"observation_version":1,"expected_url":"https://example.com/catalog"}`
			if mode == "opaque_click" {
				tool = "browser.click"
				args = `{"browser_session_id":"context","selector":"a","observation_version":1,"expected_url":"https://example.com/catalog"}`
			}
			result := dispatchForTest(t, s, f, &toolbrokerv1.ToolCall{RunId: "run", ToolCallId: "call", ToolName: tool, ArgsJson: args}, true)
			wantState, wantStatus := "effect_unknown", "effect_unknown"
			if mode == "proved" {
				wantState, wantStatus = "effect_absent", "failed"
			}
			if result.GetEffectState() != wantState || result.GetStatus() != wantStatus || result.GetError().GetRetryable() || strings.Contains(result.GetError().GetMessage(), "PRIVATE_") {
				t.Fatal("rejection changed outcome or leaked runtime data", mode, result.GetStatus(), result.GetEffectState())
			}
		})
	}
}
