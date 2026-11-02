package antigravity

import (
	"context"
	"net/http"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/butler/butler/internal/transport"
)

func TestInvalidFunctionDiagnosticNeverReleasesProposalOrPrivateBytes(t *testing.T) {
	for _, tc := range []struct{ name, part, reason string }{
		{"malformed", `{"functionCall":{"name":"butler_tool_0","args":{},"PRIVATE_FIELD":true}}`, "native_function_malformed"},
		{"not_selected", `{"functionCall":{"name":"PRIVATE_FUNCTION","args":{"PRIVATE":"SECRET"}}}`, "native_function_not_selected"},
		{"missing", `{"functionCall":{"name":"butler_tool_0","args":{}}}`, "native_arguments_missing_field"},
		{"unknown", `{"functionCall":{"name":"butler_tool_0","args":{"url":"https://example.com","PRIVATE_FIELD":"SECRET"}}}`, "native_arguments_unknown_field"},
		{"type", `{"functionCall":{"name":"butler_tool_0","args":{"url":42}}}`, "native_arguments_wrong_type"},
		{"object", `{"functionCall":{"name":"butler_tool_0","args":[]}}`, "native_arguments_wrong_type"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var requests atomic.Int32
			p := newFixture(t, func(*http.Request) (*http.Response, error) {
				requests.Add(1)
				return response(frame(tc.part, "STOP")), nil
			})
			stream, err := p.StartRun(context.Background(), request("diagnostic"))
			if err != nil {
				t.Fatal(err)
			}
			events := collect(t, stream)
			failure := findError(events)
			if transport.DiagnosticCode(failure) != "invalid_tool_request" || transport.DiagnosticReason(failure) != tc.reason || requests.Load() != 1 {
				t.Fatal("fixed diagnostic or no-retry contract lost")
			}
			if strings.Contains(failure.Error(), "PRIVATE") || strings.Contains(failure.Error(), "SECRET") {
				t.Fatal("provider bytes leaked")
			}
			for _, e := range events {
				if e.ToolCall != nil || e.ToolCallBatch != nil || e.AssistantFinal != nil {
					t.Fatal("rejected proposal escaped")
				}
			}
		})
	}
}

func TestArgumentFailureReasonRejectsUnknownTextAndWrongErrorType(t *testing.T) {
	for _, reason := range []string{"native_function_malformed", "native_function_not_selected", "native_arguments_missing_field", "native_arguments_unknown_field", "native_arguments_wrong_type", "native_arguments_union_mismatch", "native_arguments_enum_mismatch", "native_arguments_array_bounds", "native_arguments_invalid"} {
		if transport.DiagnosticReason(&transport.Error{Type: transport.ErrorTypeInvalidToolRequest, SafeReason: reason}) != reason {
			t.Fatal("fixed reason lost")
		}
		if transport.DiagnosticReason(&transport.Error{Type: transport.ErrorTypeProviderTimeout, SafeReason: reason}) != "" {
			t.Fatal("reason borrowed across type")
		}
	}
	if transport.DiagnosticReason(&transport.Error{Type: transport.ErrorTypeInvalidToolRequest, SafeReason: "PRIVATE"}) != "" {
		t.Fatal("unrecognized reason exported")
	}
}
