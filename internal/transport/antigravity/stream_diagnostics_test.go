package antigravity

import (
	"context"
	"io"
	"net/http"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/butler/butler/internal/transport"
)

func TestStreamRejectionsExportOnlyLocalBranch(t *testing.T) {
	for _, tc := range []struct{ name, body, reason string }{
		{"json", "data: {PRIVATE_PAYLOAD\n\n", "stream_json_invalid"},
		{"envelope", "data: {\"error\":{\"message\":\"PRIVATE_PAYLOAD\"}}\n\n", "stream_envelope_invalid"},
		{"usage alias", "data: {\"UsageMetadata\":{}}\n\n", "stream_usage_ambiguous"},
		{"negative usage", "data: {\"usageMetadata\":{\"totalTokenCount\":-1}}\n\n", "stream_usage_invalid"},
		{"model version", "data: {\"modelVersion\":\"PRIVATE PAYLOAD\"}\n\n", "stream_model_version_invalid"},
		{"candidates", "data: {\"candidates\":[{},{}]}\n\n", "stream_candidate_unsupported"},
		{"candidate index", "data: {\"candidates\":[{\"index\":1}]}\n\n", "stream_candidate_unexpected"},
		{"finish", frame(`{"text":"PRIVATE_PAYLOAD"}`, "SAFETY"), "stream_finish_rejected"},
		{"automatic execution", "data: {\"automaticFunctionCallingHistory\":[{}]}\n\n", "stream_automatic_function_execution"},
		{"part", frame(`null`, "STOP"), "stream_part_invalid"},
		{"unsupported part", frame(`{"PRIVATE_PAYLOAD":true}`, "STOP"), "stream_part_unsupported"},
		{"thought", frame(`{"text":"PRIVATE_PAYLOAD","thought":"true"}`, "STOP"), "stream_thought_invalid"},
		{"function ambiguity", frame(`{"text":"PRIVATE_PAYLOAD","functionCall":{}}`, "STOP"), "stream_function_ambiguous"},
		{"content", frame(`{}`, "STOP"), "stream_content_missing"},
		{"terminal", "data: [DONE]\n\n", "stream_terminal_missing"},
		{"late candidate", frame(`{"text":"PRIVATE_PAYLOAD"}`, "STOP") + frame(`{"text":"PRIVATE_PAYLOAD"}`, "STOP"), "stream_candidate_unexpected"},
		{"truncated function", frame(`{"functionCall":{"name":"PRIVATE_PAYLOAD","args":{}}}`, "MAX_TOKENS"), "stream_function_truncated"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			_, err := readStream(strings.NewReader(tc.body), func(string) bool { return true })
			if transport.DiagnosticCode(err) != "provider_protocol_error" || transport.DiagnosticReason(err) != tc.reason || transport.DiagnosticHTTPStatus(err) != 0 {
				t.Fatalf("wrong safe branch: %v / %q", err, transport.DiagnosticReason(err))
			}
			if strings.Contains(err.Error(), "PRIVATE_PAYLOAD") {
				t.Fatal("private content exported")
			}
		})
	}
	for _, input := range []string{"PRIVATE_PAYLOAD", "invalid SSE response JSON PRIVATE_PAYLOAD", "INVALID SSE RESPONSE JSON"} {
		if transport.DiagnosticReason(protocolError(input)) != "" {
			t.Fatal("nonliteral classification")
		}
	}
}

func TestStreamDiagnosticFailureNeverRetriesOrPublishesOutcome(t *testing.T) {
	var calls atomic.Int32
	p := newFixture(t, func(*http.Request) (*http.Response, error) {
		calls.Add(1)
		return &http.Response{StatusCode: 200, Header: http.Header{"Content-Type": []string{"text/event-stream"}}, Body: io.NopCloser(strings.NewReader("data: {PRIVATE_PAYLOAD\n\n"))}, nil
	})
	stream, err := p.StartRun(context.Background(), request("stream-diagnostic"))
	if err != nil {
		t.Fatal(err)
	}
	events := collect(t, stream)
	failure := findError(events)
	if calls.Load() != 1 || failure == nil || failure.Retryable || transport.DiagnosticReason(failure) != "stream_json_invalid" || failure.ProviderDetails != nil {
		t.Fatal("unsafe failed outcome")
	}
	for _, event := range events {
		if event.ToolCall != nil || event.AssistantFinal != nil || event.Usage != nil {
			t.Fatal("failed stream released an outcome")
		}
	}
}
