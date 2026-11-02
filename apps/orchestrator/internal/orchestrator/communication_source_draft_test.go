package orchestrator

import (
	"context"
	"log/slog"
	"strconv"
	"strings"
	"testing"

	commonv1 "github.com/butler/butler/internal/gen/common/v1"
	sessionv1 "github.com/butler/butler/internal/gen/session/v1"
	"github.com/butler/butler/internal/transport"
)

type sourceDraftKernel struct {
	communicationKernelFixture
	selected bool
	checks   int
}

func (k *sourceDraftKernel) CheckPublicationSources(context.Context, string, []string) (bool, error) {
	k.checks++
	return k.selected, nil
}

func TestCommunicationSourceDraftRejectsBeforeBrokerAndRepairs(t *testing.T) {
	history := &memoryTranscriptStore{}
	provider := &mockProvider{}
	tools := &mockToolExecutor{}
	kernel := &sourceDraftKernel{}
	runs := newMemoryRunManager()
	current := &sessionv1.RunRecord{RunId: "r", SessionKey: "chat", CurrentState: commonv1.RunState_RUN_STATE_MODEL_RUNNING}
	runs.records["r"] = current
	s := newFixtureService(nil, nil, runs, history, provider, Config{Communication: kernel, Tools: tools}, nil)
	r := &transport.ToolCallRequest{ToolCallRef: "bad-source-draft", ToolName: "communication.message", ArgsJSON: `{"schema_version":1,"purpose":"result","text":"Verified login reference and RP link","source_refs":["message:invented-input-event"]}`}
	_, _, err := s.handleToolCall(context.Background(), slog.Default(), current, "lease", r)
	if err != nil {
		t.Fatal(err)
	}
	if len(tools.calls) != 0 || len(history.toolCalls) != 1 || history.toolCalls[0].Status != "validation_rejected" || len(provider.submittedResults) != 1 || !strings.Contains(provider.submittedResults[0].ToolResultJSON, `"dispatched":false`) {
		t.Fatal("unselected draft reached Broker or lost correction")
	}
	r.ArgsJSON = `{"schema_version":1,"purpose":"result","text":"Verified login reference and RP link","source_refs":[]}`
	if feedback, err := s.communicationSourceFeedback(context.Background(), current, r); err != nil || feedback != "" || kernel.checks != 1 {
		t.Fatal("repair required a new login or inferred source", err)
	}
	r.ArgsJSON = `{"schema_version":1,"purpose":"result","text":"Supported selected fact","source_refs":["document:selected"]}`
	kernel.selected = true
	if feedback, err := s.communicationSourceFeedback(context.Background(), current, r); err != nil || feedback != "" {
		t.Fatal("selected source refused", err)
	}
	kernel.selected = false
	r.ArgsJSON = `{"schema_version":1,"purpose":"result","text":"Verified login reference and RP link","source_refs":["message:invented-input-event"]}`
	for attempt := 1; attempt < 4; attempt++ {
		r.ToolCallRef = "bad-source-draft-" + strconv.Itoa(attempt)
		if _, _, err = s.handleToolCall(context.Background(), slog.Default(), runs.records["r"], "lease", r); err != nil {
			t.Fatal(err)
		}
	}
	if _, _, err = s.handleToolCall(context.Background(), slog.Default(), runs.records["r"], "lease", r); err == nil || len(provider.submittedResults) != 4 || len(tools.calls) != 0 {
		t.Fatal("unbounded source correction or actual dispatch")
	}
}
