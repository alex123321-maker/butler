package orchestrator

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	sessionv1 "github.com/butler/butler/internal/gen/session/v1"
	"github.com/butler/butler/internal/security/content"
	"github.com/butler/butler/internal/transport"
)

func TestCommunicationWidgetAnswerSurvivesApprovalProviderRebuild(t *testing.T) {
	owner := content.CurrentOwnerMessage("Ask one question, then publish the answer", "input:original")
	initial := preparedRun{InputItems: []transport.InputItem{owner}}
	prepared := interactionResponsePreparation(initial, initial, "interaction:one", `{"answer":{"option_ids":["b"]},"preference_data":true}`)
	prepared = interactionResponsePreparation(prepared, prepared, "interaction:one", `{"answer":{"option_ids":["b"]},"preference_data":true}`)
	if len(prepared.InputItems) != 2 || content.ItemOrigin(prepared.InputItems[1]).IsInstruction() {
		t.Fatal("response duplicated/promoted")
	}
	continuation := approvalContinuation{Version: 1, Prepared: prepared, ToolCall: &transport.ToolCallRequest{ToolCallRef: "publish", ToolName: "communication.message", ArgsJSON: `{}`}}
	raw, _ := json.Marshal(continuation)
	var recovered approvalContinuation
	if json.Unmarshal(raw, &recovered) != nil {
		t.Fatal("saved context invalid")
	}
	provider := &mockProvider{}
	s := newFixtureService(&memorySessionRepo{}, &memoryLeaseManager{}, newMemoryRunManager(), &memoryTranscriptStore{}, provider, Config{}, nil)
	ctx := context.WithValue(context.Background(), recoveredApprovalKey{}, recovered)
	_, err := s.resumeProviderWithToolResult(ctx, &sessionv1.RunRecord{RunId: "run", SessionKey: "web:question"}, "publish", `{"status":"completed"}`)
	if err != nil {
		t.Fatal(err)
	}
	// The actual provider request after an exact approval still has the choice.
	requestRaw, _ := json.Marshal(provider.startRequests)
	if !strings.Contains(string(requestRaw), "preference_data") || !strings.Contains(string(requestRaw), "interaction:one") {
		t.Fatal("approval replay lost the response", string(requestRaw))
	}
}
