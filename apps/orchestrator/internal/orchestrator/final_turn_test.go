package orchestrator

import (
	"context"
	"encoding/json"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/butler/butler/internal/delegation"
	runv1 "github.com/butler/butler/internal/gen/run/v1"
	sessionv1 "github.com/butler/butler/internal/gen/session/v1"
	toolbrokerv1 "github.com/butler/butler/internal/gen/toolbroker/v1"
	"github.com/butler/butler/internal/ingress"
	"github.com/butler/butler/internal/memory/transcript"
	"github.com/butler/butler/internal/security/content"
	"github.com/butler/butler/internal/transport"
)

type nilFinalContinuationProvider struct{ *mockProvider }

func (p nilFinalContinuationProvider) SubmitToolResult(_ context.Context, request transport.SubmitToolResultRequest) (transport.EventStream, error) {
	p.submittedResults = append(p.submittedResults, request)
	return nil, nil
}

func TestExecuteFinalBelongsToLastModelTurn(t *testing.T) {
	for _, scenario := range []string{"fresh_final", "fresh_partial_final", "missing_final", "nil_final"} {
		t.Run(scenario, func(t *testing.T) {
			finalJSON := `{"receipt":"exact-final"}`
			provider := &mockProvider{startEvents: []transport.TransportEvent{
				transport.NewAssistantFinalEvent("", "openai", transport.AssistantFinal{Content: "Earlier tool-turn answer", FinishReason: "max_tokens"}),
				transport.NewToolCallRequestedEvent("", "openai", transport.ToolCallRequest{ToolCallRef: "call", ToolName: "http.request", ArgsJSON: `{"url":"https://example.invalid"}`}),
			}, submitEvents: []transport.TransportEvent{transport.NewAssistantDeltaEvent("", "openai", transport.AssistantDelta{Content: "Intermediate streaming text", SequenceNo: 1})}}
			missing := scenario == "missing_final" || scenario == "nil_final"
			if !missing {
				finish := "stop"
				if scenario == "fresh_partial_final" {
					finish = "max_tokens"
				}
				provider.submitEvents = append(provider.submitEvents, transport.NewAssistantFinalEvent("", "openai", transport.AssistantFinal{Content: finalJSON, FinishReason: finish}), transport.NewTerminalEvent("", transport.EventTypeRunCompleted, "openai"))
			}
			transcript := &memoryTranscriptStore{}
			tools := &mockToolExecutor{result: &toolbrokerv1.ToolResult{Status: "completed", ResultJson: `{"receipt":"observed-effect"}`}}
			var bridge transport.ModelProvider = provider
			if scenario == "nil_final" {
				bridge = nilFinalContinuationProvider{provider}
			}
			service := newFixtureService(&memorySessionRepo{}, &memoryLeaseManager{}, newMemoryRunManager(), transcript, bridge, Config{ProviderName: "openai", ModelName: "fixture-model", Tools: tools, ActionAuthorizer: &mockApprovalChecker{}}, nil)
			result, err := service.Execute(context.Background(), ingress.InputEvent{EventID: scenario, IdempotencyKey: scenario, EventType: runv1.InputEventType_INPUT_EVENT_TYPE_USER_MESSAGE, SessionKey: "final-turn:" + scenario, Source: "web", PayloadJSON: `{"text":"Finish the exact action"}`, CreatedAt: time.Now().UTC()})
			if len(transcript.toolCalls) != 1 || transcript.toolCalls[0].ResultJSON != `{"receipt":"observed-effect"}` {
				t.Fatal("last-turn validation lost the confirmed tool observation")
			}
			if missing {
				if err == nil || result != nil || !strings.Contains(err.Error(), "without assistant_final") || len(transcript.messages) != 1 {
					t.Fatal("earlier turn supplied a missing terminal answer", err)
				}
				return
			}
			state := "final"
			if scenario == "fresh_partial_final" {
				state = "partial"
			}
			if err != nil || result.AssistantResponse != finalJSON || result.AnswerState != state || len(transcript.messages) != 2 || transcript.messages[1].Content != finalJSON {
				t.Fatal("final body or finish reason came from the wrong turn", err)
			}
		})
	}
}

func TestRecoveryDelegationReceiptsKeepExactIDsAsData(t *testing.T) {
	receipts := []delegation.AdmissionReceipt{{TaskID: "child:first", InputID: "input:first", State: "queued", ContractRevision: 1}, {TaskID: "child:second", InputID: "", State: "waiting", ContractRevision: 1}}
	finished := time.Now()
	store := &memoryTranscriptStore{}
	for i, receipt := range receipts {
		raw, _ := json.Marshal(receipt)
		store.toolCalls = append(store.toolCalls, transcript.ToolCall{RunID: "run", ToolCallID: []string{"first", "second"}[i], ToolName: delegation.AdmissionTool, ArgsJSON: `{}`, Status: "completed", ResultJSON: string(raw), ErrorJSON: `{}`, FinishedAt: &finished})
	}
	provider := &mockProvider{}
	service := newFixtureService(nil, nil, nil, store, provider, Config{ProviderName: "fixture", ModelName: "fixture"}, nil)
	ctx := context.WithValue(context.Background(), recoveredApprovalKey{}, approvalContinuation{Prepared: preparedRun{InputItems: []transport.InputItem{content.CurrentOwnerMessage("Project the two existing receipts; do not admit again", "input:parent")}}, ToolCall: &transport.ToolCallRequest{ToolCallRef: "second", ToolName: delegation.AdmissionTool, ArgsJSON: `{}`}})
	current := &sessionv1.RunRecord{RunId: "run", SessionKey: "session", ProviderSessionRef: `{"provider_name":"fixture","opaque_ref":"lost"}`}
	if _, err := service.resumeProviderWithToolResult(ctx, current, "second", recordedToolResultEnvelope(store.toolCalls[1])); err != nil {
		t.Fatal(err)
	}
	if len(provider.startRequests) != 1 || len(provider.submittedResults) != 0 || provider.startRequests[0].Context.ProviderSessionRef != nil || len(provider.startRequests[0].InputItems) != 3 {
		t.Fatal("recovery reused lost function state or omitted an actual receipt")
	}
	for i, item := range provider.startRequests[0].InputItems[1:] {
		var envelope struct {
			Data struct {
				AlreadyExecuted bool   `json:"already_executed"`
				ToolCallID      string `json:"tool_call_id"`
				ToolName        string `json:"tool_name"`
				Result          struct {
					Status string                      `json:"status"`
					Result delegation.AdmissionReceipt `json:"result"`
				} `json:"result"`
			} `json:"data"`
		}
		origin := content.ItemOrigin(item)
		if json.Unmarshal([]byte(item.Content), &envelope) != nil || item.Role != "user" || origin.IsInstruction() || origin.Kind != content.ToolOutput || !envelope.Data.AlreadyExecuted || envelope.Data.ToolName != delegation.AdmissionTool || envelope.Data.ToolCallID != store.toolCalls[i].ToolCallID || envelope.Data.Result.Status != "completed" || !reflect.DeepEqual(envelope.Data.Result.Result, receipts[i]) {
			t.Fatal("recovered receipt changed identity, shape or instruction authority")
		}
	}
}
