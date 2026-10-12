package antigravity

import (
	"context"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/butler/butler/internal/transport"
)

func TestCancelledIncompleteProviderTurnHasNoObservedUsage(t *testing.T) {
	started := make(chan struct{})
	p := newFixture(t, func(req *http.Request) (*http.Response, error) {
		close(started)
		<-req.Context().Done()
		return nil, req.Context().Err()
	})
	stream, err := p.StartRun(context.Background(), request("cancelled-usage"))
	if err != nil {
		t.Fatal(err)
	}
	select {
	case <-started:
	case <-time.After(2 * time.Second):
		t.Fatal("fixture provider did not start")
	}
	if _, err = p.CancelRun(context.Background(), transport.CancelRunRequest{RunID: "cancelled-usage"}); err != nil {
		t.Fatal(err)
	}
	for _, event := range collect(t, stream) {
		if event.Usage != nil || event.ToolCall != nil || event.AssistantFinal != nil {
			t.Fatal("cancelled incomplete request published a measured final or proposal")
		}
	}
}

func TestToolTurnUsageSurvivesTieredAndHighContinuation(t *testing.T) {
	for _, high := range []bool{false, true} {
		t.Run(map[bool]string{false: "tiered", true: "high"}[high], func(t *testing.T) {
			calls := 0
			p := newFixture(t, func(*http.Request) (*http.Response, error) {
				calls++
				part := `{"functionCall":{"name":"butler_tool_0","args":{"url":"https://example.com"}}}`
				if calls == 2 {
					part = `{"text":"done"}`
				}
				if high {
					return response(cloudFrame(part, "STOP")), nil
				}
				return response(frame(part, "STOP")), nil
			})
			req := request("usage-turn")
			if high {
				p.config.AuthSource = cloudAuthSource{project: "discovered-project"}
				req.Context.ModelName = FlashHighModel
			}
			stream, err := p.StartRun(context.Background(), req)
			if err != nil {
				t.Fatal(err)
			}
			var ref *transport.ProviderSessionRef
			var call *transport.ToolCallRequest
			observations := 0
			for _, event := range collect(t, stream) {
				if event.ProviderSessionRef != nil {
					ref = event.ProviderSessionRef
				}
				if event.ToolCall != nil {
					call = event.ToolCall
					if event.Usage == nil || *event.Usage != (transport.Usage{InputTokens: 17, OutputTokens: 14, TotalTokens: 31}) {
						t.Fatal("known tool turn usage was lost")
					}
				}
				if event.Usage != nil {
					observations++
				}
			}
			if call == nil || ref == nil || observations != 1 {
				t.Fatal("missing proposal or usage duplicated within tool turn")
			}
			stream, err = p.SubmitToolResult(context.Background(), transport.SubmitToolResultRequest{RunID: req.Context.RunID, ProviderSessionRef: ref, ToolCallRef: call.ToolCallRef, ToolResultJSON: `{"status":"failed","error":{"code":"fixture_tool_failed"}}`})
			if err != nil {
				t.Fatal(err)
			}
			observations = 0
			for _, event := range collect(t, stream) {
				if event.Usage != nil {
					observations++
					if event.AssistantFinal == nil || event.AssistantFinal.Usage == nil || *event.Usage != *event.AssistantFinal.Usage {
						t.Fatal("final compatibility usage differs")
					}
				}
			}
			if observations != 1 || calls != 2 {
				t.Fatal("one complete observation required per actual request")
			}
		})
	}
}

func TestToolTurnUsageMissingIncompleteAndInvalidRemainUnobserved(t *testing.T) {
	complete := frame(`{"functionCall":{"name":"butler_tool_0","args":{"url":"https://example.com"}}}`, "STOP")
	usage := `"usageMetadata":{"promptTokenCount":17,"candidatesTokenCount":14,"totalTokenCount":31}`
	cases := []struct {
		name, sse string
		fail      bool
	}{
		{"missing", strings.Replace(complete, ","+usage, "", 1), false},
		{"partial", strings.Replace(complete, usage, `"usageMetadata":{"totalTokenCount":31}`, 1), false},
		{"provisional_then_terminal_without_usage", strings.Replace(complete, `"finishReason":"STOP"`, `"finishReason":""`, 1) + strings.Replace(frame(`{"text":"finished"}`, "STOP"), ","+usage, "", 1), false},
		{"aliased_count", strings.Replace(complete, `"candidatesTokenCount":14`, `"candidatesTokenCount":14,"candidate\u017fTokenCount":0`, 1), true},
		{"aliased_metadata", strings.Replace(complete, usage, usage+`,"u\u017fageMetadata":{"promptTokenCount":1,"candidatesTokenCount":1,"totalTokenCount":2}`, 1), true},
		{"negative", strings.Replace(complete, `"promptTokenCount":17`, `"promptTokenCount":-1`, 1), true},
		{"overflow_decode", strings.Replace(complete, `"promptTokenCount":17`, `"promptTokenCount":9223372036854775808`, 1), true},
		{"overflow_sum", strings.Replace(strings.Replace(complete, `"promptTokenCount":17`, `"promptTokenCount":9223372036854775807`, 1), `"candidatesTokenCount":14`, `"candidatesTokenCount":1`, 1), true},
		{"inconsistent", strings.Replace(complete, `"totalTokenCount":31`, `"totalTokenCount":1`, 1), true},
		{"incomplete", strings.Replace(complete, `"finishReason":"STOP"`, `"finishReason":""`, 1), true},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			p := newFixture(t, func(*http.Request) (*http.Response, error) { return response(c.sse), nil })
			stream, err := p.StartRun(context.Background(), request("unobserved"))
			if err != nil {
				t.Fatal(err)
			}
			events := collect(t, stream)
			if (findError(events) != nil) != c.fail {
				t.Fatal("unexpected safe terminal result")
			}
			for _, event := range events {
				if event.Usage != nil || event.AssistantFinal != nil && event.AssistantFinal.Usage != nil {
					t.Fatal("unknown/invalid/incomplete usage became measured zero or valid observation")
				}
				if c.fail && event.ToolCall != nil {
					t.Fatal("invalid/incomplete proposal escaped")
				}
			}
		})
	}
}

func TestCompleteInvalidToolProposalRetainsUsageWithoutGrantingExecution(t *testing.T) {
	p := newFixture(t, func(*http.Request) (*http.Response, error) {
		return response(frame(`{"functionCall":{"name":"foreign_function","args":{}}}`, "STOP")), nil
	})
	stream, err := p.StartRun(context.Background(), request("invalid-tool"))
	if err != nil {
		t.Fatal(err)
	}
	known := 0
	for _, event := range collect(t, stream) {
		if event.ToolCall != nil {
			t.Fatal("foreign function released")
		}
		if event.TransportError != nil && event.Usage != nil {
			known++
		}
	}
	if known != 1 {
		t.Fatal("complete failed proposal lost independently observed request usage")
	}
}

func TestToolTurnUsageOnlyTrailerPreservesOneConsistentCompleteObservation(t *testing.T) {
	const usage = `"usageMetadata":{"promptTokenCount":17,"candidatesTokenCount":14,"totalTokenCount":31}`
	for _, high := range []bool{false, true} {
		for _, mode := range []string{"identical", "conflicting", "late_complete"} {
			name := map[bool]string{false: "tiered", true: "high"}[high] + "/" + mode
			t.Run(name, func(t *testing.T) {
				first := frame(`{"functionCall":{"name":"butler_tool_0","args":{"url":"https://example.com"}}}`, "STOP")
				last := "data: {" + usage + "}\n\n"
				if mode == "conflicting" {
					last = strings.Replace(last, `"promptTokenCount":17`, `"promptTokenCount":18`, 1)
					last = strings.Replace(last, `"totalTokenCount":31`, `"totalTokenCount":32`, 1)
				}
				if mode == "late_complete" {
					first = strings.Replace(first, ","+usage, "", 1)
				}
				if high {
					wrap := func(sse string) string {
						return "data: {\"response\":" + strings.TrimSpace(strings.TrimPrefix(sse, "data: ")) + "}\n\n"
					}
					first, last = wrap(first), wrap(last)
				}
				p := newFixture(t, func(*http.Request) (*http.Response, error) { return response(first + last), nil })
				req := request("trailer-usage")
				if high {
					p.config.AuthSource = cloudAuthSource{project: "discovered-project"}
					req.Context.ModelName = FlashHighModel
				}
				stream, err := p.StartRun(context.Background(), req)
				if err != nil {
					t.Fatal(err)
				}
				events := collect(t, stream)
				observations, proposals := 0, 0
				for _, event := range events {
					if event.ToolCall != nil {
						proposals++
					}
					if event.Usage != nil {
						observations++
						if *event.Usage != (transport.Usage{InputTokens: 17, OutputTokens: 14, TotalTokens: 31}) {
							t.Fatal("complete usage overwritten")
						}
					}
				}
				if mode == "conflicting" {
					if findError(events) == nil || proposals != 0 || observations != 0 {
						t.Fatal("conflicting complete trailer released a proposal or unambiguous observation")
					}
				} else if findError(events) != nil || proposals != 1 || observations != 1 {
					t.Fatal("identical/late complete trailer lost or duplicated one observation")
				}
			})
		}
	}
}

func TestToolTurnUsageHighWrapperAliasesCannotReplaceCompleteResponse(t *testing.T) {
	complete := strings.TrimSpace(strings.TrimPrefix(frame(`{"functionCall":{"name":"butler_tool_0","args":{"url":"https://example.com"}}}`, "STOP"), "data: "))
	changed := strings.Replace(strings.Replace(complete, `"promptTokenCount":17`, `"promptTokenCount":18`, 1), `"totalTokenCount":31`, `"totalTokenCount":32`, 1)
	for _, alias := range []string{"Response", `re\u017fponse`} {
		t.Run(alias, func(t *testing.T) {
			sse := `data: {"response":` + complete + `,"` + alias + `":` + changed + "}\n\n"
			p := newFixture(t, func(*http.Request) (*http.Response, error) { return response(sse), nil })
			p.config.AuthSource = cloudAuthSource{project: "discovered-project"}
			req := request("aliased-high-wrapper")
			req.Context.ModelName = FlashHighModel
			stream, err := p.StartRun(context.Background(), req)
			if err != nil {
				t.Fatal(err)
			}
			events := collect(t, stream)
			if failure := findError(events); failure == nil || failure.Type != transport.ErrorTypeProviderProtocolError {
				t.Fatal("aliased High wrapper was accepted")
			}
			for _, event := range events {
				if event.Usage != nil || event.ToolCall != nil || event.AssistantFinal != nil {
					t.Fatal("alternate wrapper replaced a complete observation/proposal")
				}
			}
		})
	}
}
