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

type diagnosticExecutor struct {
	wrongCall        bool
	transportFailure bool
	oversized        bool
	internal         bool
}

func (e diagnosticExecutor) Execute(_ context.Context, call *toolbrokerv1.ToolCall, _ *toolbrokerv1.ToolContract, _ []credentials.ResolvedSecret) (*toolbrokerv1.ToolResult, error) {
	if e.transportFailure {
		return nil, errors.New("fixture connection lost")
	}
	id := call.ToolCallId
	if e.wrongCall {
		id = "another-call"
	}
	message := "bounded read diagnostic"
	if e.oversized {
		message = strings.Repeat("x", 513)
	}
	class := commonv1.ErrorClass_ERROR_CLASS_VALIDATION_ERROR
	if e.internal {
		class = commonv1.ErrorClass_ERROR_CLASS_INTERNAL_ERROR
	}
	return &toolbrokerv1.ToolResult{RunId: call.RunId, ToolCallId: id, ToolName: call.ToolName, Status: "failed", Error: &toolbrokerv1.ToolError{ErrorClass: class, Message: message, DetailsJson: `{"reason":"unsupported_content"}`, Retryable: true}}, nil
}

func TestReadFailureDiagnosticsDoNotConfirmEffectsOrPermitRetry(t *testing.T) {
	for _, tc := range []struct {
		name, method                          string
		wrongCall, transportFailure, preserve bool
		oversized, internal                   bool
	}{
		{name: "explicit read failure", method: "GET", preserve: true},
		{name: "write failure", method: "POST"},
		{name: "uncorrelated read failure", method: "GET", wrongCall: true},
		{name: "lost read response", method: "GET", transportFailure: true},
		{name: "oversized diagnostic", method: "GET", oversized: true},
		{name: "internal read failure", method: "GET", internal: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			reg, err := registry.Load(filepath.Join("..", "..", "..", "..", "configs", "tools.json"), "local")
			if err != nil {
				t.Fatal(err)
			}
			s := NewServer(reg, diagnosticExecutor{wrongCall: tc.wrongCall, transportFailure: tc.transportFailure, oversized: tc.oversized, internal: tc.internal}, nil, nil)
			f := setTestAuthority(s, nil)
			r := dispatchForTest(t, s, f, &toolbrokerv1.ToolCall{RunId: "run", ToolCallId: "call", ToolName: "http.request", ArgsJson: `{"method":"` + tc.method + `","url":"https://example.com"}`}, true)
			if r.GetEffectState() != "effect_unknown" || r.GetError().GetRetryable() {
				t.Fatal("failure confirmed effect or permitted retry")
			}
			if tc.preserve {
				if r.GetStatus() != "failed" || r.GetError().GetMessage() != "bounded read diagnostic" || r.GetError().GetDetailsJson() != `{"reason":"unsupported_content"}` {
					t.Fatal("correlated read diagnostic lost", r)
				}
			} else if r.GetStatus() != "effect_unknown" || r.GetError().GetMessage() == "bounded read diagnostic" {
				t.Fatal("write/transport/uncorrelated outcome was relaxed", r)
			}
		})
	}
}
