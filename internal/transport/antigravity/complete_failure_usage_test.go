package antigravity

import (
	"context"
	"io"
	"net/http"
	"strings"
	"testing"

	"github.com/butler/butler/internal/transport"
)

// A complete rejected request can have measured cost. It never leaves a
// pending executable proposal or exposes rejected model parts in its error.
func TestToolTurnUsageTruncatedFunctionFailureRetainsOnlyCompleteMeasurements(t *testing.T) {
	const part = `{"functionCall":{"name":"butler_tool_0","args":{"url":"https://example.com/private-proposal"}},"thoughtSignature":"private-signature"}`
	const counts = `"usageMetadata":{"promptTokenCount":17,"candidatesTokenCount":14,"totalTokenCount":31}`
	complete := frame(part, "MAX_TOKENS")
	known := &transport.Usage{InputTokens: 17, OutputTokens: 14, TotalTokens: 31}
	zero := &transport.Usage{}
	for _, high := range []bool{false, true} {
		wrap := func(sse string) string {
			if !high {
				return sse
			}
			// Preserve a missing frame terminator; wrapping must not repair SSE.
			ending := sse[len(strings.TrimRight(sse, "\n")):]
			return "data: {\"response\":" + strings.TrimSpace(strings.TrimPrefix(sse, "data: ")) + "}" + ending
		}
		for _, tc := range []struct {
			name, sse string
			usage     *transport.Usage
			version   bool
			readError bool
		}{
			{"known", complete, known, true, false},
			{"true_zero", strings.Replace(complete, counts, `"usageMetadata":{"promptTokenCount":0,"candidatesTokenCount":0,"totalTokenCount":0}`, 1), zero, true, false},
			{"missing_usage", strings.Replace(complete, ","+counts, "", 1), nil, true, false},
			{"partial_usage", strings.Replace(complete, counts, `"usageMetadata":{"totalTokenCount":31}`, 1), nil, true, false},
			{"missing_version", strings.Replace(complete, `"modelVersion":"gemini-3.8-flash",`, "", 1), known, false, false},
			{"aliased_metadata", strings.Replace(complete, counts, counts+`,"u\u017fageMetadata":{"promptTokenCount":1,"candidatesTokenCount":1,"totalTokenCount":2}`, 1), nil, false, false},
			{"aliased_count", strings.Replace(complete, `"candidatesTokenCount":14`, `"candidatesTokenCount":14,"candidate\u017fTokenCount":0`, 1), nil, false, false},
			{"negative_count", strings.Replace(complete, `"promptTokenCount":17`, `"promptTokenCount":-1`, 1), nil, false, false},
			{"invalid_version", strings.Replace(complete, `"gemini-3.8-flash"`, `"PRIVATE_VERSION"`, 1), nil, false, false},
			{"incomplete_frame", strings.TrimSuffix(complete, "\n\n"), nil, false, false},
			{"incomplete_candidate", strings.Replace(complete, `"MAX_TOKENS"`, `""`, 1), nil, false, false},
			{"reader_failure_after_terminal", complete, nil, false, true},
		} {
			t.Run(map[bool]string{false: "tiered", true: "high"}[high]+"/"+tc.name, func(t *testing.T) {
				wire := wrap(tc.sse)
				calls := 0
				p := newFixture(t, func(*http.Request) (*http.Response, error) {
					calls++
					resp := response(wire)
					if tc.readError {
						resp.Body = io.NopCloser(io.MultiReader(strings.NewReader(wire), incompleteFailureReader{}))
					}
					return resp, nil
				})
				req := request("truncated-failure")
				if high {
					p.config.AuthSource = cloudAuthSource{project: "discovered-project"}
					req.Context.ModelName = FlashHighModel
				}
				stream, err := p.StartRun(context.Background(), req)
				if err != nil {
					t.Fatal(err)
				}
				failed := 0
				for _, event := range collect(t, stream) {
					if event.ToolCall != nil || event.ToolCallBatch != nil || event.AssistantFinal != nil || event.EventType == transport.EventTypeRunCompleted {
						t.Fatal("rejected proposal released an executable or successful outcome")
					}
					if event.TransportError == nil {
						if event.Usage != nil || event.ProviderModelVersion != nil {
							t.Fatal("measurement published before the complete failed outcome")
						}
						continue
					}
					failed++
					if event.TransportError.Type != transport.ErrorTypeProviderProtocolError || strings.Contains(event.TransportError.Error(), "private-") || strings.Contains(event.TransportError.Error(), "PRIVATE_") {
						t.Fatal("unsafe or incorrect failure class")
					}
					if (event.Usage != nil) != (tc.usage != nil) || event.Usage != nil && *event.Usage != *tc.usage || (event.ProviderModelVersion != nil) != tc.version || event.ProviderModelVersion != nil && *event.ProviderModelVersion != "gemini-3.8-flash" {
						t.Fatal("complete failed-request measurement differs")
					}
				}
				if failed != 1 || calls != 1 {
					t.Fatal("failed request duplicated, retried or lost its terminal")
				}
				if _, err = p.SubmitToolResult(context.Background(), transport.SubmitToolResultRequest{RunID: req.Context.RunID, ToolCallRef: "forged-after-failure", ToolResultJSON: `{}`}); err == nil || calls != 1 {
					t.Fatal("failed proposal left executable continuation state")
				}
			})
		}
		for _, mode := range []string{"conflicting_usage", "conflicting_version", "provisional_only"} {
			t.Run(map[bool]string{false: "tiered", true: "high"}[high]+"/"+mode, func(t *testing.T) {
				first := complete
				last := `data: {"usageMetadata":{"promptTokenCount":18,"candidatesTokenCount":14,"totalTokenCount":32}}` + "\n\n"
				version := false
				if mode == "conflicting_version" {
					last = `data: {"modelVersion":"gemini-other"}` + "\n\n"
				}
				if mode == "provisional_only" {
					first = strings.Replace(first, `"MAX_TOKENS"`, `""`, 1)
					last = strings.Replace(frame(`{"text":""}`, "MAX_TOKENS"), ","+counts, "", 1)
					version = true
				}
				answer, err := readStreamProtocol(strings.NewReader(wrap(first)+wrap(last)), func(string) bool { return true }, high)
				if err == nil || answer.Usage != nil || (answer.ModelVersion != nil) != version || answer.Text != "" || len(answer.Parts) != 0 || answer.FinishReason != "" {
					t.Fatal("conflicting/provisional measurements or rejected model bodies escaped")
				}
			})
		}
	}
}

type incompleteFailureReader struct{}

func (incompleteFailureReader) Read([]byte) (int, error) { return 0, io.ErrUnexpectedEOF }

func TestToolTurnUsageTruncatedFunctionFailureParserReturnsNoModelBody(t *testing.T) {
	answer, err := readStream(strings.NewReader(frame(`{"functionCall":{"name":"butler_tool_0","args":{"url":"PRIVATE_BODY"}},"thoughtSignature":"PRIVATE_SIGNATURE"}`, "MAX_TOKENS")), func(string) bool { return true })
	if err == nil || answer.Usage == nil || answer.ModelVersion == nil || answer.Text != "" || len(answer.Parts) != 0 || answer.FinishReason != "" {
		t.Fatal("complete rejected proposal must return only typed measurement metadata")
	}
}
