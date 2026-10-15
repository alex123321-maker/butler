package orchestrator

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	runv1 "github.com/butler/butler/internal/gen/run/v1"
	"github.com/butler/butler/internal/ingress"
	"github.com/butler/butler/internal/transport"
)

// The core carries an exact structured proposal through authorization. Textual
// action names, quotes and negation must never rewrite it into a different tool.
func TestOwnerTextCannotRewriteStatusIntoCapture(t *testing.T) {
	for _, text := range []string{"Не делай скриншот, только проверь статус вкладки.", "Что означает screenshot? Проверь статус.", "Проверь статус вкладки"} {
		t.Run(text, func(t *testing.T) {
			provider := &mockProvider{startEvents: []transport.TransportEvent{transport.NewToolCallRequestedEvent("", "openai", transport.ToolCallRequest{ToolCallRef: "status", ToolName: "single_tab.status", ArgsJSON: `{"session_id":"approved-tab"}`})}, submitEvents: []transport.TransportEvent{transport.NewAssistantFinalEvent("", "openai", transport.AssistantFinal{Content: "status checked", FinishReason: "completed"})}}
			tools := &mockToolExecutor{}
			service := newFixtureService(&memorySessionRepo{}, &memoryLeaseManager{}, newMemoryRunManager(), &memoryTranscriptStore{}, provider, Config{ProviderName: "openai", ModelName: "fixture", Tools: tools, ActionAuthorizer: &mockApprovalChecker{}}, nil)
			payload, _ := json.Marshal(map[string]string{"text": text})
			_, err := service.Execute(context.Background(), ingress.InputEvent{EventID: "negative-capture", EventType: runv1.InputEventType_INPUT_EVENT_TYPE_USER_MESSAGE, SessionKey: "web:session:negative-capture", Source: "web", PayloadJSON: string(payload), CreatedAt: time.Now().UTC(), IdempotencyKey: "negative-capture"})
			if err != nil {
				t.Fatal(err)
			}
			if len(tools.calls) != 1 || tools.calls[0].GetToolName() != "single_tab.status" || tools.calls[0].GetArgsJson() != `{"session_id":"approved-tab"}` {
				t.Fatalf("proposal was rewritten: %+v", tools.calls)
			}
		})
	}
}
