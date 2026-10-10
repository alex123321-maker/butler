package orchestrator

import (
	"context"
	"errors"
	"log/slog"
	"strings"
	"testing"
	"time"

	commonv1 "github.com/butler/butler/internal/gen/common/v1"
	sessionv1 "github.com/butler/butler/internal/gen/session/v1"
	"github.com/butler/butler/internal/memory/transcript"
	"github.com/butler/butler/internal/transport"
)

func TestModelStreamReadHonorsExecutionDeadline(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Millisecond)
	defer cancel()
	stream := make(chan transport.TransportEvent)
	service := &Service{}
	done := make(chan error, 1)
	go func() {
		_, _, err := service.consumeModelStream(ctx, slog.Default(), &sessionv1.RunRecord{RunId: "run", CurrentState: commonv1.RunState_RUN_STATE_MODEL_RUNNING}, "session", "lease", stream)
		done <- err
	}()
	select {
	case err := <-done:
		if !errors.Is(err, context.DeadlineExceeded) {
			t.Fatalf("deadline not observed: %v", err)
		}
	case <-time.After(time.Second):
		t.Fatal("idle provider stream ignored execution deadline")
	}
}

func TestRecoveryReplaysEveryKnownActionAndThenUsesLiveTransport(t *testing.T) {
	finished := time.Now()
	store := &memoryTranscriptStore{toolCalls: []transcript.ToolCall{
		{RunID: "other", ToolCallID: "unrelated", Status: "completed", FinishedAt: &finished},
		{RunID: "run", ToolCallID: "first", ToolName: "http.request", ArgsJSON: `{"url":"https://example.invalid/one"}`, Status: "completed", ResultJSON: `{"receipt":"first-effect"}`, FinishedAt: &finished},
		{RunID: "run", ToolCallID: "second", ToolName: "http.request", ArgsJSON: `{}`, Status: "completed", ResultJSON: `{"receipt":"second-effect"}`, FinishedAt: &finished},
	}}
	provider := &mockProvider{}
	service := newFixtureService(nil, nil, nil, store, provider, Config{ProviderName: "fixture", ModelName: "fixture"}, nil)
	ctx := context.WithValue(context.Background(), recoveredApprovalKey{}, approvalContinuation{Prepared: preparedRun{InputItems: []transport.InputItem{{Role: "user", Content: "finish both actions"}}}, ToolCall: &transport.ToolCallRequest{ToolCallRef: "second", ToolName: "http.request", ArgsJSON: `{}`}})
	current := &sessionv1.RunRecord{RunId: "run", SessionKey: "session", ProviderSessionRef: `{"provider_name":"fixture","opaque_ref":"live"}`}
	if _, err := service.resumeProviderWithToolResult(ctx, current, "second", `{"status":"completed","result":{"receipt":"second-effect"}}`); err != nil {
		t.Fatal(err)
	}
	if len(provider.startRequests) != 1 || provider.startRequests[0].Context.ProviderSessionRef != nil {
		t.Fatal("recovery did not start a fresh Butler-owned provider context")
	}
	items := provider.startRequests[0].InputItems
	if len(items) != 3 || !strings.Contains(items[1].Content, "first-effect") || !strings.Contains(items[2].Content, "second-effect") {
		t.Fatalf("lost or duplicated known action history: %#v", items)
	}
	if items[1].Role != "user" || items[2].Role != "user" {
		t.Fatal("recorded tool observations were promoted to system authority")
	}
	if _, err := service.resumeProviderWithToolResult(ctx, current, "new-live-call", `{"status":"completed"}`); err != nil {
		t.Fatal(err)
	}
	if len(provider.startRequests) != 1 || len(provider.submittedResults) != 1 || provider.submittedResults[0].ToolCallRef != "new-live-call" {
		t.Fatal("new live result discarded the current provider's private context")
	}
	store.toolCalls[1].Status = "effect_unknown"
	if _, err := service.resumeProviderWithToolResult(ctx, current, "second", `{}`); !errors.Is(err, ErrActionOutcomeUnknown) {
		t.Fatalf("uncertain earlier action resumed model: %v", err)
	}
}

func TestRecordedRecoveryPreservesFailedToolEvidence(t *testing.T) {
	call := transcript.ToolCall{Status: "failed", ResultJSON: `{}`, ErrorJSON: `{"error_class":"ERROR_CLASS_EXTERNAL","message":"rejected before submission","retryable":false}`}
	encoded := recordedToolResultEnvelope(call)
	if !strings.Contains(encoded, `"status":"failed"`) || !strings.Contains(encoded, "rejected before submission") || !strings.Contains(encoded, `"retryable":false`) {
		t.Fatalf("known failure evidence lost: %s", encoded)
	}
}
