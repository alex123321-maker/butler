package orchestrator

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/butler/butler/apps/orchestrator/internal/session"
	commonv1 "github.com/butler/butler/internal/gen/common/v1"
	runv1 "github.com/butler/butler/internal/gen/run/v1"
	sessionv1 "github.com/butler/butler/internal/gen/session/v1"
	toolbrokerv1 "github.com/butler/butler/internal/gen/toolbroker/v1"
	"github.com/butler/butler/internal/ingress"
	"github.com/butler/butler/internal/memory/embeddings"
	memoryservice "github.com/butler/butler/internal/memory/service"
	"github.com/butler/butler/internal/memory/transcript"
	"github.com/butler/butler/internal/transport"
)

func TestExecuteRunsHappyPath(t *testing.T) {
	t.Parallel()

	sessions := &memorySessionRepo{}
	leases := &memoryLeaseManager{}
	runs := newMemoryRunManager()
	transcripts := &memoryTranscriptStore{}
	delivery := &recordingDeliverySink{}
	provider := &mockProvider{startEvents: []transport.TransportEvent{
		transport.NewRunStartedEvent("", "openai", transport.CapabilitySnapshot{SupportsStreaming: true}, &transport.ProviderSessionRef{ProviderName: "openai", ResponseRef: "resp_123"}),
		transport.NewAssistantDeltaEvent("", "openai", transport.AssistantDelta{Content: "Hel", SequenceNo: 1}),
		transport.NewAssistantFinalEvent("", "openai", transport.AssistantFinal{Content: "Hello world", FinishReason: "completed"}),
		transport.NewTerminalEvent("", transport.EventTypeRunCompleted, "openai"),
	}}

	service := NewService(sessions, leases, runs, transcripts, provider, Config{
		ProviderName: "openai",
		ModelName:    "gpt-5-mini",
		Delivery:     delivery,
	}, nil)

	result, err := service.Execute(context.Background(), ingress.InputEvent{
		EventID:        "event-1",
		EventType:      runv1.InputEventType_INPUT_EVENT_TYPE_USER_MESSAGE,
		SessionKey:     "telegram:chat:1",
		Source:         "telegram",
		PayloadJSON:    `{"text":"hello"}`,
		CreatedAt:      time.Now().UTC(),
		IdempotencyKey: "event-1",
	})
	if err != nil {
		t.Fatalf("Execute returned error: %v", err)
	}
	if result.AssistantResponse != "Hello world" {
		t.Fatalf("expected final response, got %q", result.AssistantResponse)
	}
	if got := delivery.contents(); len(got) != 2 || got[0] != "Hel" || got[1] != "Hello world" {
		t.Fatalf("expected ordered delivery events, got %v", got)
	}
	if len(transcripts.messages) != 2 {
		t.Fatalf("expected user and assistant transcript messages, got %d", len(transcripts.messages))
	}
	if runs.records[result.RunID].GetProviderSessionRef() == "" {
		t.Fatal("expected provider session ref to be persisted")
	}
	if transcripts.messages[0].Role != "user" || transcripts.messages[1].Role != "assistant" {
		t.Fatalf("unexpected transcript roles: %+v", transcripts.messages)
	}
	states := runs.statesFor(result.RunID)
	wantStates := []commonv1.RunState{
		commonv1.RunState_RUN_STATE_CREATED,
		commonv1.RunState_RUN_STATE_QUEUED,
		commonv1.RunState_RUN_STATE_ACQUIRED,
		commonv1.RunState_RUN_STATE_PREPARING,
		commonv1.RunState_RUN_STATE_MODEL_RUNNING,
		commonv1.RunState_RUN_STATE_FINALIZING,
		commonv1.RunState_RUN_STATE_COMPLETED,
	}
	if len(states) != len(wantStates) {
		t.Fatalf("unexpected state count: got %v want %v", states, wantStates)
	}
	for i := range states {
		if states[i] != wantStates[i] {
			t.Fatalf("unexpected state at %d: got %v want %v", i, states[i], wantStates[i])
		}
	}
	if leases.releaseCount != 1 {
		t.Fatalf("expected lease release on success")
	}
}

func TestExecuteRunsSequentialToolLoop(t *testing.T) {
	t.Parallel()

	sessions := &memorySessionRepo{}
	leases := &memoryLeaseManager{}
	runs := newMemoryRunManager()
	transcripts := &memoryTranscriptStore{}
	provider := &mockProvider{
		startEvents: []transport.TransportEvent{
			transport.NewRunStartedEvent("", "openai", transport.CapabilitySnapshot{SupportsStreaming: true}, &transport.ProviderSessionRef{ProviderName: "openai", ResponseRef: "resp_123"}),
			transport.NewToolCallRequestedEvent("", "openai", transport.ToolCallRequest{ToolCallRef: "call_1", ToolName: "http.request", ArgsJSON: `{"url":"https://example.com"}`, SequenceNo: 1}),
		},
		submitEvents: []transport.TransportEvent{
			transport.NewAssistantFinalEvent("", "openai", transport.AssistantFinal{Content: "done after tool", FinishReason: "completed"}),
			transport.NewTerminalEvent("", transport.EventTypeRunCompleted, "openai"),
		},
	}
	tools := &mockToolExecutor{result: &toolbrokerv1.ToolResult{Status: "completed", ResultJson: `{"status_code":200}`}}

	service := NewService(sessions, leases, runs, transcripts, provider, Config{ProviderName: "openai", ModelName: "gpt-5-mini", Tools: tools}, nil)
	result, err := service.Execute(context.Background(), ingress.InputEvent{EventID: "event-1", EventType: runv1.InputEventType_INPUT_EVENT_TYPE_USER_MESSAGE, SessionKey: "telegram:chat:1", Source: "telegram", PayloadJSON: `{"text":"fetch"}`, CreatedAt: time.Now().UTC(), IdempotencyKey: "event-1"})
	if err != nil {
		t.Fatalf("Execute returned error: %v", err)
	}
	if result.AssistantResponse != "done after tool" {
		t.Fatalf("expected resumed assistant response, got %q", result.AssistantResponse)
	}
	if len(tools.calls) != 1 || tools.calls[0].GetToolName() != "http.request" {
		t.Fatalf("expected one tool call, got %+v", tools.calls)
	}
	if len(provider.submittedResults) != 1 || provider.submittedResults[0].ToolCallRef != "call_1" {
		t.Fatalf("expected one submitted tool result, got %+v", provider.submittedResults)
	}
	if len(transcripts.toolCalls) != 1 || transcripts.toolCalls[0].ToolName != "http.request" {
		t.Fatalf("expected tool transcript entry, got %+v", transcripts.toolCalls)
	}
	states := runs.statesFor(result.RunID)
	wantStates := []commonv1.RunState{commonv1.RunState_RUN_STATE_CREATED, commonv1.RunState_RUN_STATE_QUEUED, commonv1.RunState_RUN_STATE_ACQUIRED, commonv1.RunState_RUN_STATE_PREPARING, commonv1.RunState_RUN_STATE_MODEL_RUNNING, commonv1.RunState_RUN_STATE_TOOL_PENDING, commonv1.RunState_RUN_STATE_TOOL_RUNNING, commonv1.RunState_RUN_STATE_AWAITING_MODEL_RESUME, commonv1.RunState_RUN_STATE_MODEL_RUNNING, commonv1.RunState_RUN_STATE_FINALIZING, commonv1.RunState_RUN_STATE_COMPLETED}
	if len(states) != len(wantStates) {
		t.Fatalf("unexpected state count: got %v want %v", states, wantStates)
	}
	for i := range states {
		if states[i] != wantStates[i] {
			t.Fatalf("unexpected state at %d: got %v want %v", i, states[i], wantStates[i])
		}
	}
}

func TestExecuteToolWithApprovalApproved(t *testing.T) {
	t.Parallel()

	sessions := &memorySessionRepo{}
	leases := &memoryLeaseManager{}
	runs := newMemoryRunManager()
	transcripts := &memoryTranscriptStore{}
	delivery := &recordingDeliverySink{}
	gate := NewApprovalGate()
	provider := &mockProvider{
		startEvents: []transport.TransportEvent{
			transport.NewRunStartedEvent("", "openai", transport.CapabilitySnapshot{SupportsStreaming: true}, &transport.ProviderSessionRef{ProviderName: "openai", ResponseRef: "resp_123"}),
			transport.NewToolCallRequestedEvent("", "openai", transport.ToolCallRequest{ToolCallRef: "call_1", ToolName: "http.request", ArgsJSON: `{"url":"https://example.com"}`, SequenceNo: 1}),
		},
		submitEvents: []transport.TransportEvent{
			transport.NewAssistantFinalEvent("", "openai", transport.AssistantFinal{Content: "done after approved tool", FinishReason: "completed"}),
			transport.NewTerminalEvent("", transport.EventTypeRunCompleted, "openai"),
		},
	}
	tools := &mockToolExecutor{result: &toolbrokerv1.ToolResult{Status: "completed", ResultJson: `{"status_code":200}`}}
	checker := &mockApprovalChecker{toolsRequiringApproval: map[string]bool{"http.request": true}}

	service := NewService(sessions, leases, runs, transcripts, provider, Config{
		ProviderName:    "openai",
		ModelName:       "gpt-5-mini",
		Tools:           tools,
		ApprovalChecker: checker,
		ApprovalGate:    gate,
		Delivery:        delivery,
	}, nil)

	// Approve asynchronously when approval request arrives.
	go func() {
		for {
			if len(delivery.approvalRequests) > 0 {
				gate.Resolve(delivery.approvalRequests[0].ToolCallID, true)
				return
			}
			time.Sleep(5 * time.Millisecond)
		}
	}()

	result, err := service.Execute(context.Background(), ingress.InputEvent{EventID: "event-1", EventType: runv1.InputEventType_INPUT_EVENT_TYPE_USER_MESSAGE, SessionKey: "telegram:chat:1", Source: "telegram", PayloadJSON: `{"text":"fetch"}`, CreatedAt: time.Now().UTC(), IdempotencyKey: "event-1"})
	if err != nil {
		t.Fatalf("Execute returned error: %v", err)
	}
	if result.AssistantResponse != "done after approved tool" {
		t.Fatalf("expected resumed assistant response, got %q", result.AssistantResponse)
	}
	if len(delivery.approvalRequests) != 1 {
		t.Fatalf("expected one approval request, got %d", len(delivery.approvalRequests))
	}
	if delivery.approvalRequests[0].ToolName != "http.request" {
		t.Fatalf("expected approval for http.request, got %q", delivery.approvalRequests[0].ToolName)
	}
	if len(tools.calls) != 1 {
		t.Fatalf("expected tool to be executed after approval, got %d calls", len(tools.calls))
	}
	states := runs.statesFor(result.RunID)
	// Should include awaiting_approval in the state sequence.
	foundApproval := false
	for _, s := range states {
		if s == commonv1.RunState_RUN_STATE_AWAITING_APPROVAL {
			foundApproval = true
			break
		}
	}
	if !foundApproval {
		t.Fatalf("expected awaiting_approval state in transitions, got %v", states)
	}
}

func TestExecuteToolWithApprovalRejected(t *testing.T) {
	t.Parallel()

	sessions := &memorySessionRepo{}
	leases := &memoryLeaseManager{}
	runs := newMemoryRunManager()
	transcripts := &memoryTranscriptStore{}
	delivery := &recordingDeliverySink{}
	gate := NewApprovalGate()
	provider := &mockProvider{
		startEvents: []transport.TransportEvent{
			transport.NewRunStartedEvent("", "openai", transport.CapabilitySnapshot{SupportsStreaming: true}, &transport.ProviderSessionRef{ProviderName: "openai", ResponseRef: "resp_123"}),
			transport.NewToolCallRequestedEvent("", "openai", transport.ToolCallRequest{ToolCallRef: "call_1", ToolName: "http.request", ArgsJSON: `{"url":"https://danger.com"}`, SequenceNo: 1}),
		},
		submitEvents: []transport.TransportEvent{
			transport.NewAssistantFinalEvent("", "openai", transport.AssistantFinal{Content: "tool was rejected", FinishReason: "completed"}),
			transport.NewTerminalEvent("", transport.EventTypeRunCompleted, "openai"),
		},
	}
	tools := &mockToolExecutor{result: &toolbrokerv1.ToolResult{Status: "completed", ResultJson: `{}`}}
	checker := &mockApprovalChecker{toolsRequiringApproval: map[string]bool{"http.request": true}}

	service := NewService(sessions, leases, runs, transcripts, provider, Config{
		ProviderName:    "openai",
		ModelName:       "gpt-5-mini",
		Tools:           tools,
		ApprovalChecker: checker,
		ApprovalGate:    gate,
		Delivery:        delivery,
	}, nil)

	// Reject asynchronously when approval request arrives.
	go func() {
		for {
			if len(delivery.approvalRequests) > 0 {
				gate.Resolve(delivery.approvalRequests[0].ToolCallID, false)
				return
			}
			time.Sleep(5 * time.Millisecond)
		}
	}()

	result, err := service.Execute(context.Background(), ingress.InputEvent{EventID: "event-1", EventType: runv1.InputEventType_INPUT_EVENT_TYPE_USER_MESSAGE, SessionKey: "telegram:chat:1", Source: "telegram", PayloadJSON: `{"text":"do something dangerous"}`, CreatedAt: time.Now().UTC(), IdempotencyKey: "event-1"})
	if err != nil {
		t.Fatalf("Execute returned error: %v", err)
	}
	if result.AssistantResponse != "tool was rejected" {
		t.Fatalf("expected rejected tool response, got %q", result.AssistantResponse)
	}
	// Tool should not have been called.
	if len(tools.calls) != 0 {
		t.Fatalf("expected tool to NOT be executed after rejection, got %d calls", len(tools.calls))
	}
	// Provider should still get a result (the rejection).
	if len(provider.submittedResults) != 1 {
		t.Fatalf("expected one submitted result (rejection), got %d", len(provider.submittedResults))
	}
}

func TestExecuteIncludesMemoryAwareContext(t *testing.T) {
	t.Parallel()

	provider := &mockProvider{startEvents: []transport.TransportEvent{
		transport.NewAssistantFinalEvent("", "openai", transport.AssistantFinal{Content: "memory aware", FinishReason: "completed"}),
		transport.NewTerminalEvent("", transport.EventTypeRunCompleted, "openai"),
	}}
	episodeStore := &stubEpisodeStore{entries: []MemoryEpisode{stubEpisode{summary: "Fixed Redis outage before", distance: 0.01}}}
	workingStore := &stubWorkingStore{snapshots: map[string]WorkingMemorySnapshot{
		"telegram:chat:1": {SessionKey: "telegram:chat:1", Goal: "Stabilize Redis", EntitiesJSON: `{"service":"redis"}`, PendingStepsJSON: `["check health"]`, Status: "active"},
	}}
	service := NewService(&memorySessionRepo{}, &memoryLeaseManager{}, newMemoryRunManager(), &memoryTranscriptStore{}, provider, Config{
		ProviderName: "openai",
		ModelName:    "gpt-5-mini",
		ProfileStore: stubProfileStore{entries: []MemoryProfileEntry{stubProfileEntry{key: "language", summary: "User prefers Russian"}}},
		EpisodeStore: episodeStore,
		WorkingStore: workingStore,
	}, nil)

	result, err := service.Execute(context.Background(), ingress.InputEvent{EventID: "event-1", EventType: runv1.InputEventType_INPUT_EVENT_TYPE_USER_MESSAGE, SessionKey: "telegram:chat:1", Source: "telegram", PayloadJSON: `{"text":"check redis"}`, CreatedAt: time.Now().UTC(), IdempotencyKey: "event-1"})
	if err != nil {
		t.Fatalf("Execute returned error: %v", err)
	}
	if result.AssistantResponse != "memory aware" {
		t.Fatalf("unexpected assistant response %q", result.AssistantResponse)
	}
	if len(provider.startRequests) != 1 || len(provider.startRequests[0].InputItems) < 2 {
		t.Fatalf("expected memory prompt plus user input, got %+v", provider.startRequests)
	}
	if provider.startRequests[0].InputItems[0].Role != "system" {
		t.Fatalf("expected first input item to be system memory prompt, got %+v", provider.startRequests[0].InputItems)
	}
	if !strings.Contains(provider.startRequests[0].InputItems[0].Content, "Working memory:") {
		t.Fatalf("expected working memory in prompt, got %q", provider.startRequests[0].InputItems[0].Content)
	}
	if episodeStore.calls != 0 {
		t.Fatalf("expected episodic retrieval to be skipped without embeddings, got %d calls", episodeStore.calls)
	}
}

func TestExecuteUsesConfiguredMemoryBundleService(t *testing.T) {
	t.Parallel()

	provider := &mockProvider{startEvents: []transport.TransportEvent{
		transport.NewAssistantFinalEvent("", "openai", transport.AssistantFinal{Content: "memory aware", FinishReason: "completed"}),
		transport.NewTerminalEvent("", transport.EventTypeRunCompleted, "openai"),
	}}
	bundles := &stubMemoryBundleService{bundle: memoryservice.Bundle{
		Items: map[string]any{
			"session_summary": "Remember the deployment checklist.",
			"working": map[string]any{
				"goal":            "Ship release",
				"active_entities": map[string]any{"service": "web"},
				"pending_steps":   []any{"run tests"},
				"working_status":  "active",
			},
		},
		Prompt: "Session summary:\nRemember the deployment checklist.\n\nWorking memory:\n- Goal: Ship release",
		Working: memoryservice.WorkingContext{
			Goal:           "Ship release",
			ActiveEntities: map[string]any{"service": "web"},
			PendingSteps:   []any{"run tests"},
			Scratch:        map[string]any{"source": "memory-service"},
			Status:         "active",
		},
	}}
	service := NewService(&memorySessionRepo{}, &memoryLeaseManager{}, newMemoryRunManager(), &memoryTranscriptStore{}, provider, Config{
		ProviderName:  "openai",
		ModelName:     "gpt-5-mini",
		MemoryBundles: bundles,
	}, nil)

	result, err := service.Execute(context.Background(), ingress.InputEvent{EventID: "event-bundle-1", EventType: runv1.InputEventType_INPUT_EVENT_TYPE_USER_MESSAGE, SessionKey: "telegram:chat:bundle", Source: "telegram", PayloadJSON: `{"text":"continue"}`, CreatedAt: time.Now().UTC(), IdempotencyKey: "event-bundle-1"})
	if err != nil {
		t.Fatalf("Execute returned error: %v", err)
	}
	if result.AssistantResponse != "memory aware" {
		t.Fatalf("unexpected assistant response %q", result.AssistantResponse)
	}
	if bundles.calls != 1 {
		t.Fatalf("expected configured memory bundle service to be called once, got %d", bundles.calls)
	}
	if bundles.lastRequest.SessionKey != "telegram:chat:bundle" || bundles.lastRequest.UserID != "telegram:chat:bundle" || bundles.lastRequest.UserMessage != "continue" || !bundles.lastRequest.IncludeQuery {
		t.Fatalf("unexpected memory bundle request %+v", bundles.lastRequest)
	}
	if len(provider.startRequests) != 1 || provider.startRequests[0].InputItems[0].Role != "system" {
		t.Fatalf("expected memory prompt to be injected, got %+v", provider.startRequests)
	}
	if !strings.Contains(provider.startRequests[0].InputItems[0].Content, "deployment checklist") {
		t.Fatalf("expected memory prompt from bundle service, got %q", provider.startRequests[0].InputItems[0].Content)
	}
	metadata := runsMetadata(t, service, result.RunID)
	workingBundle, ok := metadata["memory_bundle"].(map[string]any)["working"].(map[string]any)
	if !ok || workingBundle["goal"] != "Ship release" {
		t.Fatalf("expected bundle service output in run metadata, got %+v", metadata)
	}
	workingStore := service.config.WorkingStore
	if workingStore != nil {
		t.Fatal("expected no direct working store for configured bundle service test")
	}
}

func TestExecuteLoadsAndClearsWorkingMemoryOnCompletion(t *testing.T) {
	t.Parallel()

	provider := &mockProvider{startEvents: []transport.TransportEvent{
		transport.NewAssistantFinalEvent("", "openai", transport.AssistantFinal{Content: "done", FinishReason: "completed"}),
		transport.NewTerminalEvent("", transport.EventTypeRunCompleted, "openai"),
	}}
	workingStore := &stubWorkingStore{snapshots: map[string]WorkingMemorySnapshot{
		"telegram:chat:working": {SessionKey: "telegram:chat:working", Goal: "Existing goal", EntitiesJSON: `{"service":"postgres"}`, PendingStepsJSON: `["inspect logs"]`, Status: "active"},
	}}
	transientStore := &stubTransientWorkingStore{}
	service := NewService(&memorySessionRepo{}, &memoryLeaseManager{}, newMemoryRunManager(), &memoryTranscriptStore{}, provider, Config{
		ProviderName:   "openai",
		ModelName:      "gpt-5-mini",
		WorkingStore:   workingStore,
		TransientStore: transientStore,
	}, nil)

	result, err := service.Execute(context.Background(), ingress.InputEvent{EventID: "event-working-1", EventType: runv1.InputEventType_INPUT_EVENT_TYPE_USER_MESSAGE, SessionKey: "telegram:chat:working", Source: "telegram", PayloadJSON: `{"text":"continue task"}`, CreatedAt: time.Now().UTC(), IdempotencyKey: "event-working-1"})
	if err != nil {
		t.Fatalf("Execute returned error: %v", err)
	}
	if result.AssistantResponse != "done" {
		t.Fatalf("unexpected response %q", result.AssistantResponse)
	}
	if len(workingStore.getCalls) == 0 {
		t.Fatal("expected working memory snapshot to be loaded during prepare")
	}
	if len(provider.startRequests) != 1 {
		t.Fatalf("expected single provider start request, got %d", len(provider.startRequests))
	}
	metadata := runsMetadata(t, service, result.RunID)
	workingBundle, ok := metadata["memory_bundle"].(map[string]any)["working"].(map[string]any)
	if !ok {
		t.Fatalf("expected structured working memory bundle, got %+v", metadata)
	}
	if workingBundle["goal"] != "Existing goal" {
		t.Fatalf("unexpected working goal %+v", workingBundle)
	}
	if len(workingStore.clearCalls) != 1 || workingStore.clearCalls[0] != "telegram:chat:working" {
		t.Fatalf("expected working memory clear on completion, got %+v", workingStore.clearCalls)
	}
	if len(transientStore.clearCalls) != 1 {
		t.Fatalf("expected transient working state clear on completion, got %+v", transientStore.clearCalls)
	}
	if _, exists := workingStore.snapshots["telegram:chat:working"]; exists {
		t.Fatal("expected completed working memory snapshot to be removed")
	}
}

func TestExecuteRetainsWorkingMemoryOnFailure(t *testing.T) {
	t.Parallel()

	runs := newMemoryRunManager()
	workingStore := &stubWorkingStore{snapshots: map[string]WorkingMemorySnapshot{
		"telegram:chat:failure": {SessionKey: "telegram:chat:failure", Goal: "Recover service", EntitiesJSON: `{"service":"api"}`, PendingStepsJSON: `["restart"]`, Status: "active"},
	}}
	transientStore := &stubTransientWorkingStore{}
	service := NewService(&memorySessionRepo{}, &memoryLeaseManager{}, runs, &memoryTranscriptStore{}, &mockProvider{startEvents: []transport.TransportEvent{
		transport.NewTransportErrorEvent("", "openai", errors.New("provider down")),
	}}, Config{ProviderName: "openai", ModelName: "gpt-5-mini", WorkingStore: workingStore, TransientStore: transientStore}, nil)

	_, err := service.Execute(context.Background(), ingress.InputEvent{EventID: "event-working-fail", EventType: runv1.InputEventType_INPUT_EVENT_TYPE_USER_MESSAGE, SessionKey: "telegram:chat:failure", Source: "telegram", PayloadJSON: `{"text":"continue"}`, CreatedAt: time.Now().UTC(), IdempotencyKey: "event-working-fail"})
	if err == nil {
		t.Fatal("expected Execute error")
	}
	if len(workingStore.clearCalls) != 0 {
		t.Fatalf("expected failed runs to retain working memory, got clear calls %+v", workingStore.clearCalls)
	}
	snapshot := workingStore.snapshots["telegram:chat:failure"]
	if snapshot.Status != "retain" {
		t.Fatalf("expected retained working memory status, got %+v", snapshot)
	}
	if !strings.Contains(snapshot.ScratchJSON, "provider down") {
		t.Fatalf("expected failure note in scratch payload, got %q", snapshot.ScratchJSON)
	}
	transient := transientStore.states["telegram:chat:failure:run-1"]
	if transient.Status != "retain" {
		t.Fatalf("expected retained transient state, got %+v", transient)
	}
}

func TestExecuteSanitizesWorkingMemoryToolPayloads(t *testing.T) {
	t.Parallel()

	provider := &mockProvider{
		startEvents: []transport.TransportEvent{
			transport.NewRunStartedEvent("", "openai", transport.CapabilitySnapshot{SupportsStreaming: true}, &transport.ProviderSessionRef{ProviderName: "openai", ResponseRef: "resp_123"}),
			transport.NewToolCallRequestedEvent("", "openai", transport.ToolCallRequest{ToolCallRef: "call_1", ToolName: "http.request", ArgsJSON: `{"authorization":"Bearer abc","password":"secret"}`, SequenceNo: 1}),
		},
		submitEvents: []transport.TransportEvent{
			transport.NewAssistantFinalEvent("", "openai", transport.AssistantFinal{Content: "done after tool", FinishReason: "completed"}),
			transport.NewTerminalEvent("", transport.EventTypeRunCompleted, "openai"),
		},
	}
	tools := &mockToolExecutor{result: &toolbrokerv1.ToolResult{Status: "completed", ResultJson: `{"access_token":"abc","cookie":"session=abc123"}`}}
	workingStore := &stubWorkingStore{snapshots: map[string]WorkingMemorySnapshot{}}
	service := NewService(&memorySessionRepo{}, &memoryLeaseManager{}, newMemoryRunManager(), &memoryTranscriptStore{}, provider, Config{ProviderName: "openai", ModelName: "gpt-5-mini", Tools: tools, WorkingStore: workingStore}, nil)

	_, err := service.Execute(context.Background(), ingress.InputEvent{EventID: "event-sanitize-1", EventType: runv1.InputEventType_INPUT_EVENT_TYPE_USER_MESSAGE, SessionKey: "telegram:chat:sanitize", Source: "telegram", PayloadJSON: `{"text":"fetch"}`, CreatedAt: time.Now().UTC(), IdempotencyKey: "event-sanitize-1"})
	if err != nil {
		t.Fatalf("Execute returned error: %v", err)
	}
	joined := ""
	for _, save := range workingStore.saveCalls {
		joined += save.ScratchJSON + save.Goal + save.EntitiesJSON + save.PendingStepsJSON
	}
	for _, secret := range []string{"Bearer abc", "session=abc123", "secret"} {
		if strings.Contains(joined, secret) {
			t.Fatalf("expected secret %q to be redacted in working memory saves %q", secret, joined)
		}
	}
	if !strings.Contains(joined, "[REDACTED") {
		t.Fatalf("expected redaction markers in %q", joined)
	}
}

func TestExecuteStoresTransientWorkingStateDuringToolLifecycle(t *testing.T) {
	t.Parallel()

	provider := &mockProvider{
		startEvents: []transport.TransportEvent{
			transport.NewRunStartedEvent("", "openai", transport.CapabilitySnapshot{SupportsStreaming: true}, &transport.ProviderSessionRef{ProviderName: "openai", ResponseRef: "resp_123"}),
			transport.NewToolCallRequestedEvent("", "openai", transport.ToolCallRequest{ToolCallRef: "call_1", ToolName: "http.request", ArgsJSON: `{"url":"https://example.com"}`, SequenceNo: 1}),
		},
		submitEvents: []transport.TransportEvent{
			transport.NewAssistantFinalEvent("", "openai", transport.AssistantFinal{Content: "done after tool", FinishReason: "completed"}),
			transport.NewTerminalEvent("", transport.EventTypeRunCompleted, "openai"),
		},
	}
	tools := &mockToolExecutor{result: &toolbrokerv1.ToolResult{Status: "completed", ResultJson: `{"status_code":200}`}}
	transientStore := &stubTransientWorkingStore{}
	service := NewService(&memorySessionRepo{}, &memoryLeaseManager{}, newMemoryRunManager(), &memoryTranscriptStore{}, provider, Config{ProviderName: "openai", ModelName: "gpt-5-mini", Tools: tools, TransientStore: transientStore}, nil)

	_, err := service.Execute(context.Background(), ingress.InputEvent{EventID: "event-transient-1", EventType: runv1.InputEventType_INPUT_EVENT_TYPE_USER_MESSAGE, SessionKey: "telegram:chat:transient", Source: "telegram", PayloadJSON: `{"text":"fetch"}`, CreatedAt: time.Now().UTC(), IdempotencyKey: "event-transient-1"})
	if err != nil {
		t.Fatalf("Execute returned error: %v", err)
	}
	if len(transientStore.saveCalls) < 3 {
		t.Fatalf("expected transient checkpoints to be saved, got %d", len(transientStore.saveCalls))
	}
	if transientStore.saveCalls[0].Status != "preparing" {
		t.Fatalf("expected first transient checkpoint to be preparing, got %+v", transientStore.saveCalls[0])
	}
}

func TestExecuteUsesEmbeddingProviderForEpisodicRetrieval(t *testing.T) {
	t.Parallel()
	provider := &mockProvider{startEvents: []transport.TransportEvent{
		transport.NewAssistantFinalEvent("", "openai", transport.AssistantFinal{Content: "memory aware", FinishReason: "completed"}),
		transport.NewTerminalEvent("", transport.EventTypeRunCompleted, "openai"),
	}}
	episodeStore := &stubEpisodeStore{entries: []MemoryEpisode{stubEpisode{summary: "Recovered Postgres connection before", distance: 0.01}}}
	service := NewService(&memorySessionRepo{}, &memoryLeaseManager{}, newMemoryRunManager(), &memoryTranscriptStore{}, provider, Config{
		ProviderName: "openai",
		ModelName:    "gpt-5-mini",
		EpisodeStore: episodeStore,
		Embeddings:   stubEmbeddingProvider{embedding: testEmbeddingVector(0.2)},
	}, nil)
	result, err := service.Execute(context.Background(), ingress.InputEvent{EventID: "event-2", EventType: runv1.InputEventType_INPUT_EVENT_TYPE_USER_MESSAGE, SessionKey: "telegram:chat:2", Source: "telegram", PayloadJSON: `{"text":"check postgres"}`, CreatedAt: time.Now().UTC(), IdempotencyKey: "event-2"})
	if err != nil {
		t.Fatalf("Execute returned error: %v", err)
	}
	if result.AssistantResponse != "memory aware" {
		t.Fatalf("unexpected assistant response %q", result.AssistantResponse)
	}
	if episodeStore.calls == 0 {
		t.Fatal("expected episodic retrieval to call the store")
	}
	if len(provider.startRequests) != 1 || provider.startRequests[0].InputItems[0].Role != "system" {
		t.Fatalf("expected memory prompt plus user input, got %+v", provider.startRequests)
	}
	if !strings.Contains(provider.startRequests[0].InputItems[0].Content, "Recovered Postgres connection before") {
		t.Fatalf("expected episodic memory prompt, got %q", provider.startRequests[0].InputItems[0].Content)
	}
	if episodeStore.lastLimit != 3 {
		t.Fatalf("expected default episodic limit 3, got %d", episodeStore.lastLimit)
	}
}

func TestExecuteIncludesSessionSummaryInContext(t *testing.T) {
	t.Parallel()

	provider := &mockProvider{startEvents: []transport.TransportEvent{
		transport.NewAssistantFinalEvent("", "openai", transport.AssistantFinal{Content: "aware of summary", FinishReason: "completed"}),
		transport.NewTerminalEvent("", transport.EventTypeRunCompleted, "openai"),
	}}
	summaryReader := &stubSummaryReader{summaries: map[string]string{
		"telegram:chat:1": "Current goal: deploy new service. Recent events: fixed Redis connection. Open tasks: update Docker config.",
	}}
	service := NewService(&memorySessionRepo{}, &memoryLeaseManager{}, newMemoryRunManager(), &memoryTranscriptStore{}, provider, Config{
		ProviderName:  "openai",
		ModelName:     "gpt-5-mini",
		SummaryReader: summaryReader,
	}, nil)

	result, err := service.Execute(context.Background(), ingress.InputEvent{
		EventID:        "event-1",
		EventType:      runv1.InputEventType_INPUT_EVENT_TYPE_USER_MESSAGE,
		SessionKey:     "telegram:chat:1",
		Source:         "telegram",
		PayloadJSON:    `{"text":"what are we doing?"}`,
		CreatedAt:      time.Now().UTC(),
		IdempotencyKey: "event-1",
	})
	if err != nil {
		t.Fatalf("Execute returned error: %v", err)
	}
	if result.AssistantResponse != "aware of summary" {
		t.Fatalf("unexpected response %q", result.AssistantResponse)
	}
	if len(provider.startRequests) != 1 {
		t.Fatalf("expected 1 start request, got %d", len(provider.startRequests))
	}
	items := provider.startRequests[0].InputItems
	if len(items) < 2 {
		t.Fatalf("expected at least 2 input items (system + user), got %d", len(items))
	}
	if items[0].Role != "system" {
		t.Fatalf("expected first input item to be system memory prompt, got %q", items[0].Role)
	}
	if !strings.Contains(items[0].Content, "Session summary:") {
		t.Fatalf("expected session summary section in memory prompt, got %q", items[0].Content)
	}
	if !strings.Contains(items[0].Content, "deploy new service") {
		t.Fatalf("expected summary content in memory prompt, got %q", items[0].Content)
	}
}

func TestExecuteSessionSummaryWithProfileAndEpisodes(t *testing.T) {
	t.Parallel()

	provider := &mockProvider{startEvents: []transport.TransportEvent{
		transport.NewAssistantFinalEvent("", "openai", transport.AssistantFinal{Content: "full context", FinishReason: "completed"}),
		transport.NewTerminalEvent("", transport.EventTypeRunCompleted, "openai"),
	}}
	summaryReader := &stubSummaryReader{summaries: map[string]string{
		"telegram:chat:1": "Working on deployment pipeline.",
	}}
	service := NewService(&memorySessionRepo{}, &memoryLeaseManager{}, newMemoryRunManager(), &memoryTranscriptStore{}, provider, Config{
		ProviderName:  "openai",
		ModelName:     "gpt-5-mini",
		SummaryReader: summaryReader,
		ProfileStore:  stubProfileStore{entries: []MemoryProfileEntry{stubProfileEntry{key: "name", summary: "User is Alice"}}},
	}, nil)

	result, err := service.Execute(context.Background(), ingress.InputEvent{
		EventID:        "event-1",
		EventType:      runv1.InputEventType_INPUT_EVENT_TYPE_USER_MESSAGE,
		SessionKey:     "telegram:chat:1",
		Source:         "telegram",
		PayloadJSON:    `{"text":"continue"}`,
		CreatedAt:      time.Now().UTC(),
		IdempotencyKey: "event-1",
	})
	if err != nil {
		t.Fatalf("Execute returned error: %v", err)
	}
	if result.AssistantResponse != "full context" {
		t.Fatalf("unexpected response %q", result.AssistantResponse)
	}
	prompt := provider.startRequests[0].InputItems[0].Content
	// Session summary should come first in the prompt.
	summaryIdx := strings.Index(prompt, "Session summary:")
	profileIdx := strings.Index(prompt, "Profile memory:")
	if summaryIdx < 0 {
		t.Fatalf("expected session summary in prompt, got %q", prompt)
	}
	if profileIdx < 0 {
		t.Fatalf("expected profile memory in prompt, got %q", prompt)
	}
	if summaryIdx > profileIdx {
		t.Fatalf("expected session summary before profile memory in prompt")
	}
}

func TestExecuteSkipsSummaryWhenEmpty(t *testing.T) {
	t.Parallel()

	provider := &mockProvider{startEvents: []transport.TransportEvent{
		transport.NewAssistantFinalEvent("", "openai", transport.AssistantFinal{Content: "no summary", FinishReason: "completed"}),
		transport.NewTerminalEvent("", transport.EventTypeRunCompleted, "openai"),
	}}
	summaryReader := &stubSummaryReader{summaries: map[string]string{}}
	service := NewService(&memorySessionRepo{}, &memoryLeaseManager{}, newMemoryRunManager(), &memoryTranscriptStore{}, provider, Config{
		ProviderName:  "openai",
		ModelName:     "gpt-5-mini",
		SummaryReader: summaryReader,
	}, nil)

	result, err := service.Execute(context.Background(), ingress.InputEvent{
		EventID:        "event-1",
		EventType:      runv1.InputEventType_INPUT_EVENT_TYPE_USER_MESSAGE,
		SessionKey:     "telegram:chat:1",
		Source:         "telegram",
		PayloadJSON:    `{"text":"hello"}`,
		CreatedAt:      time.Now().UTC(),
		IdempotencyKey: "event-1",
	})
	if err != nil {
		t.Fatalf("Execute returned error: %v", err)
	}
	if result.AssistantResponse != "no summary" {
		t.Fatalf("unexpected response %q", result.AssistantResponse)
	}
	// No memory context should be injected when summary is empty and no other memory.
	items := provider.startRequests[0].InputItems
	if len(items) != 1 {
		t.Fatalf("expected only user input item when no memory context, got %d items", len(items))
	}
	if items[0].Role != "user" {
		t.Fatalf("expected only user role, got %q", items[0].Role)
	}
}

func TestExecuteSummaryReaderErrorDoesNotBlockRun(t *testing.T) {
	t.Parallel()

	provider := &mockProvider{startEvents: []transport.TransportEvent{
		transport.NewAssistantFinalEvent("", "openai", transport.AssistantFinal{Content: "still works", FinishReason: "completed"}),
		transport.NewTerminalEvent("", transport.EventTypeRunCompleted, "openai"),
	}}
	summaryReader := &stubSummaryReader{err: errors.New("database error")}
	service := NewService(&memorySessionRepo{}, &memoryLeaseManager{}, newMemoryRunManager(), &memoryTranscriptStore{}, provider, Config{
		ProviderName:  "openai",
		ModelName:     "gpt-5-mini",
		SummaryReader: summaryReader,
	}, nil)

	result, err := service.Execute(context.Background(), ingress.InputEvent{
		EventID:        "event-1",
		EventType:      runv1.InputEventType_INPUT_EVENT_TYPE_USER_MESSAGE,
		SessionKey:     "telegram:chat:1",
		Source:         "telegram",
		PayloadJSON:    `{"text":"hello"}`,
		CreatedAt:      time.Now().UTC(),
		IdempotencyKey: "event-1",
	})
	if err != nil {
		t.Fatalf("Execute should not fail when summary reader errors: %v", err)
	}
	if result.AssistantResponse != "still works" {
		t.Fatalf("unexpected response %q", result.AssistantResponse)
	}
}

func TestFormatMemoryPromptWithSummary(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name           string
		profile        []map[string]any
		episodes       []map[string]any
		summary        string
		working        memoryservice.WorkingContext
		wantContains   []string
		wantNotContain []string
	}{
		{
			name:    "summary with working memory",
			summary: "Deploying app.",
			working: memoryservice.WorkingContext{Goal: "Ship release", ActiveEntities: map[string]any{"service": "web"}, PendingSteps: []any{"run tests"}, Status: "active"},
			wantContains: []string{
				"Session summary:",
				"Working memory:",
				"Goal: Ship release",
				"Active entities: {\"service\":\"web\"}",
				"Pending steps: [\"run tests\"]",
			},
		},
		{
			name:         "summary only",
			summary:      "Current goal: fix bugs.",
			wantContains: []string{"Session summary:", "Current goal: fix bugs."},
		},
		{
			name:    "summary with profile",
			profile: []map[string]any{{"key": "name", "summary": "Alice"}},
			summary: "Deploying app.",
			wantContains: []string{
				"Session summary:",
				"Deploying app.",
				"Profile memory:",
				"- name: Alice",
			},
		},
		{
			name:           "no summary",
			profile:        []map[string]any{{"key": "lang", "summary": "Go"}},
			wantContains:   []string{"Profile memory:"},
			wantNotContain: []string{"Session summary:"},
		},
		{
			name:           "empty summary string",
			summary:        "   ",
			wantNotContain: []string{"Session summary:"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := memoryservice.FormatPrompt(tt.working, tt.profile, tt.episodes, tt.summary)
			for _, want := range tt.wantContains {
				if !strings.Contains(result, want) {
					t.Errorf("expected prompt to contain %q, got %q", want, result)
				}
			}
			for _, dontWant := range tt.wantNotContain {
				if strings.Contains(result, dontWant) {
					t.Errorf("expected prompt to NOT contain %q, got %q", dontWant, result)
				}
			}
		})
	}
}

func runsMetadata(t *testing.T, service *Service, runID string) map[string]any {
	t.Helper()
	runManager, ok := service.runs.(*memoryRunManager)
	if !ok {
		t.Fatal("expected in-memory run manager")
	}
	metadata := map[string]any{}
	if err := json.Unmarshal([]byte(runManager.records[runID].GetMetadataJson()), &metadata); err != nil {
		t.Fatalf("decode run metadata: %v", err)
	}
	return metadata
}

func TestExecuteFailsRunOnTransportError(t *testing.T) {
	t.Parallel()

	runs := newMemoryRunManager()
	leases := &memoryLeaseManager{}
	service := NewService(&memorySessionRepo{}, leases, runs, &memoryTranscriptStore{}, &mockProvider{startEvents: []transport.TransportEvent{
		transport.NewTransportErrorEvent("", "openai", errors.New("provider down")),
	}}, Config{ProviderName: "openai", ModelName: "gpt-5-mini"}, nil)

	_, err := service.Execute(context.Background(), ingress.InputEvent{
		EventID:        "event-1",
		EventType:      runv1.InputEventType_INPUT_EVENT_TYPE_USER_MESSAGE,
		SessionKey:     "telegram:chat:1",
		Source:         "telegram",
		PayloadJSON:    `{"text":"hello"}`,
		CreatedAt:      time.Now().UTC(),
		IdempotencyKey: "event-1",
	})
	if err == nil {
		t.Fatal("expected Execute error")
	}
	states := runs.statesFor("run-1")
	if states[len(states)-1] != commonv1.RunState_RUN_STATE_FAILED {
		t.Fatalf("expected failed terminal state, got %v", states)
	}
	if leases.releaseCount != 1 {
		t.Fatalf("expected lease release on failure")
	}
}

func TestExecuteFailsRunWhenLeaseAcquireFails(t *testing.T) {
	t.Parallel()

	runs := newMemoryRunManager()
	service := NewService(&memorySessionRepo{}, &memoryLeaseManager{acquireErr: session.ErrLeaseConflict}, runs, &memoryTranscriptStore{}, &mockProvider{}, Config{ProviderName: "openai", ModelName: "gpt-5-mini"}, nil)

	_, err := service.Execute(context.Background(), ingress.InputEvent{
		EventID:        "event-1",
		EventType:      runv1.InputEventType_INPUT_EVENT_TYPE_USER_MESSAGE,
		SessionKey:     "telegram:chat:1",
		Source:         "telegram",
		PayloadJSON:    `{"text":"hello"}`,
		CreatedAt:      time.Now().UTC(),
		IdempotencyKey: "event-1",
	})
	if err == nil {
		t.Fatal("expected Execute error")
	}
	states := runs.statesFor("run-1")
	if states[len(states)-1] != commonv1.RunState_RUN_STATE_FAILED {
		t.Fatalf("expected failed terminal state after lease acquire failure, got %v", states)
	}
}

func TestExecuteFailsRunWhenQueueTransitionFails(t *testing.T) {
	t.Parallel()

	runs := newMemoryRunManager()
	runs.transitionErrs = map[commonv1.RunState]error{commonv1.RunState_RUN_STATE_QUEUED: errors.New("queue write failed")}
	service := NewService(&memorySessionRepo{}, &memoryLeaseManager{}, runs, &memoryTranscriptStore{}, &mockProvider{}, Config{ProviderName: "openai", ModelName: "gpt-5-mini"}, nil)

	_, err := service.Execute(context.Background(), ingress.InputEvent{
		EventID:        "event-1",
		EventType:      runv1.InputEventType_INPUT_EVENT_TYPE_USER_MESSAGE,
		SessionKey:     "telegram:chat:1",
		Source:         "telegram",
		PayloadJSON:    `{"text":"hello"}`,
		CreatedAt:      time.Now().UTC(),
		IdempotencyKey: "event-1",
	})
	if err == nil {
		t.Fatal("expected Execute error")
	}
	states := runs.statesFor("run-1")
	if states[len(states)-1] != commonv1.RunState_RUN_STATE_FAILED {
		t.Fatalf("expected failed terminal state after queue transition failure, got %v", states)
	}
}

func TestExecuteFailsRunFromFinalizing(t *testing.T) {
	t.Parallel()

	runs := newMemoryRunManager()
	service := NewService(&memorySessionRepo{}, &memoryLeaseManager{}, runs, &memoryTranscriptStore{failAssistant: errors.New("persist assistant failed")}, &mockProvider{startEvents: []transport.TransportEvent{
		transport.NewAssistantFinalEvent("", "openai", transport.AssistantFinal{Content: "Hello world", FinishReason: "completed"}),
		transport.NewTerminalEvent("", transport.EventTypeRunCompleted, "openai"),
	}}, Config{ProviderName: "openai", ModelName: "gpt-5-mini"}, nil)

	_, err := service.Execute(context.Background(), ingress.InputEvent{
		EventID:        "event-1",
		EventType:      runv1.InputEventType_INPUT_EVENT_TYPE_USER_MESSAGE,
		SessionKey:     "telegram:chat:1",
		Source:         "telegram",
		PayloadJSON:    `{"text":"hello"}`,
		CreatedAt:      time.Now().UTC(),
		IdempotencyKey: "event-1",
	})
	if err == nil {
		t.Fatal("expected Execute error")
	}
	states := runs.statesFor("run-1")
	if states[len(states)-1] != commonv1.RunState_RUN_STATE_FAILED {
		t.Fatalf("expected finalizing failure to end in failed, got %v", states)
	}
}

func TestExecuteFailsRunWhenModelRunningTransitionFails(t *testing.T) {
	t.Parallel()

	runs := newMemoryRunManager()
	runs.transitionErrs = map[commonv1.RunState]error{commonv1.RunState_RUN_STATE_MODEL_RUNNING: errors.New("transition to model running failed")}
	service := NewService(&memorySessionRepo{}, &memoryLeaseManager{}, runs, &memoryTranscriptStore{}, &mockProvider{}, Config{ProviderName: "openai", ModelName: "gpt-5-mini"}, nil)

	_, err := service.Execute(context.Background(), ingress.InputEvent{
		EventID:        "event-1",
		EventType:      runv1.InputEventType_INPUT_EVENT_TYPE_USER_MESSAGE,
		SessionKey:     "telegram:chat:1",
		Source:         "telegram",
		PayloadJSON:    `{"text":"hello"}`,
		CreatedAt:      time.Now().UTC(),
		IdempotencyKey: "event-1",
	})
	if err == nil {
		t.Fatal("expected Execute error")
	}
	states := runs.statesFor("run-1")
	if states[len(states)-1] != commonv1.RunState_RUN_STATE_FAILED {
		t.Fatalf("expected failed terminal state after model_running transition failure, got %v", states)
	}
}

func TestExecuteRenewsLeaseDuringLongRun(t *testing.T) {
	t.Parallel()

	leases := &memoryLeaseManager{}
	runs := newMemoryRunManager()
	provider := &delayedProvider{delay: 1200 * time.Millisecond}
	service := NewService(&memorySessionRepo{}, leases, runs, &memoryTranscriptStore{}, provider, Config{ProviderName: "openai", ModelName: "gpt-5-mini", LeaseTTL: 1}, nil)

	result, err := service.Execute(context.Background(), ingress.InputEvent{
		EventID:        "event-1",
		EventType:      runv1.InputEventType_INPUT_EVENT_TYPE_USER_MESSAGE,
		SessionKey:     "telegram:chat:1",
		Source:         "telegram",
		PayloadJSON:    `{"text":"hello"}`,
		CreatedAt:      time.Now().UTC(),
		IdempotencyKey: "event-1",
	})
	if err != nil {
		t.Fatalf("Execute returned error: %v", err)
	}
	if result.AssistantResponse == "" {
		t.Fatal("expected final assistant response")
	}
	if leases.renewCount == 0 {
		t.Fatal("expected lease renewals during long-running model execution")
	}
}

type memorySessionRepo struct{}

func (m *memorySessionRepo) CreateSession(_ context.Context, params session.CreateSessionParams) (session.SessionRecord, bool, error) {
	now := time.Now().UTC()
	return session.SessionRecord{SessionKey: params.SessionKey, UserID: params.UserID, Channel: params.Channel, MetadataJSON: params.MetadataJSON, CreatedAt: now, UpdatedAt: now}, true, nil
}

type memoryLeaseManager struct {
	releaseCount int
	leaseID      string
	renewCount   int
	acquireErr   error
	renewErr     error
}

func (m *memoryLeaseManager) AcquireLease(_ context.Context, params session.AcquireLeaseParams) (session.LeaseRecord, error) {
	if m.acquireErr != nil {
		return session.LeaseRecord{}, m.acquireErr
	}
	if m.leaseID == "" {
		m.leaseID = "lease-1"
	}
	now := time.Now().UTC()
	return session.LeaseRecord{LeaseID: m.leaseID, SessionKey: params.SessionKey, RunID: params.RunID, OwnerID: params.OwnerID, AcquiredAt: now, ExpiresAt: now.Add(params.TTL)}, nil
}

func (m *memoryLeaseManager) RenewLease(context.Context, string, time.Duration) (session.LeaseRecord, error) {
	m.renewCount++
	if m.renewErr != nil {
		return session.LeaseRecord{}, m.renewErr
	}
	return session.LeaseRecord{}, nil
}

func (m *memoryLeaseManager) ReleaseLease(context.Context, string) (bool, error) {
	m.releaseCount++
	return true, nil
}

type memoryRunManager struct {
	records        map[string]*sessionv1.RunRecord
	history        map[string][]commonv1.RunState
	transitionErrs map[commonv1.RunState]error
	nextID         int
}

func newMemoryRunManager() *memoryRunManager {
	return &memoryRunManager{records: make(map[string]*sessionv1.RunRecord), history: make(map[string][]commonv1.RunState)}
}

func (m *memoryRunManager) CreateRun(_ context.Context, req *sessionv1.CreateRunRequest) (*sessionv1.RunRecord, error) {
	m.nextID++
	runID := "run-" + string(rune('0'+m.nextID))
	record := &sessionv1.RunRecord{RunId: runID, SessionKey: req.GetSessionKey(), CurrentState: commonv1.RunState_RUN_STATE_CREATED, Status: "created", ModelProvider: req.GetModelProvider(), MetadataJson: req.GetMetadataJson()}
	m.records[runID] = record
	m.history[runID] = append(m.history[runID], commonv1.RunState_RUN_STATE_CREATED)
	return record, nil
}

func (m *memoryRunManager) TransitionRun(_ context.Context, req *sessionv1.UpdateRunStateRequest) (*sessionv1.RunRecord, error) {
	if err := m.transitionErrs[req.GetToState()]; err != nil {
		return nil, err
	}
	record := m.records[req.GetRunId()]
	record.CurrentState = req.GetToState()
	record.Status = req.GetToState().String()
	record.LeaseId = req.GetLeaseId()
	record.ErrorMessage = req.GetErrorMessage()
	m.history[req.GetRunId()] = append(m.history[req.GetRunId()], req.GetToState())
	return record, nil
}

func (m *memoryRunManager) GetRun(_ context.Context, runID string) (*sessionv1.RunRecord, error) {
	return m.records[runID], nil
}

func (m *memoryRunManager) PersistProviderSessionRef(_ context.Context, runID, providerSessionRef string) (*sessionv1.RunRecord, error) {
	record := m.records[runID]
	record.ProviderSessionRef = providerSessionRef
	return record, nil
}

func (m *memoryRunManager) statesFor(runID string) []commonv1.RunState {
	return append([]commonv1.RunState(nil), m.history[runID]...)
}

type memoryTranscriptStore struct {
	messages      []transcript.Message
	toolCalls     []transcript.ToolCall
	failAssistant error
}

func (m *memoryTranscriptStore) AppendMessage(_ context.Context, message transcript.Message) (transcript.Message, error) {
	if message.Role == "assistant" && m.failAssistant != nil {
		return transcript.Message{}, m.failAssistant
	}
	m.messages = append(m.messages, message)
	return message, nil
}

func (m *memoryTranscriptStore) AppendToolCall(_ context.Context, call transcript.ToolCall) (transcript.ToolCall, error) {
	m.toolCalls = append(m.toolCalls, call)
	return call, nil
}

type mockProvider struct {
	startEvents      []transport.TransportEvent
	startRequests    []transport.StartRunRequest
	submitEvents     []transport.TransportEvent
	submittedResults []transport.SubmitToolResultRequest
	err              error
}

func (m *mockProvider) Name() string { return "openai" }

func (m *mockProvider) Capabilities(context.Context, transport.TransportRunContext) (transport.CapabilitySnapshot, error) {
	return transport.CapabilitySnapshot{SupportsStreaming: true}, nil
}

func (m *mockProvider) StartRun(_ context.Context, req transport.StartRunRequest) (transport.EventStream, error) {
	if m.err != nil {
		return nil, m.err
	}
	m.startRequests = append(m.startRequests, req)
	ch := make(chan transport.TransportEvent, len(m.startEvents))
	for _, event := range m.startEvents {
		if event.RunID == "" {
			event.RunID = req.Context.RunID
		}
		ch <- event
	}
	close(ch)
	return ch, nil
}

type stubProfileStore struct{ entries []MemoryProfileEntry }

func (s stubProfileStore) GetByScope(context.Context, string, string) ([]MemoryProfileEntry, error) {
	return s.entries, nil
}

type stubEpisodeStore struct {
	entries    []MemoryEpisode
	calls      int
	lastLimit  int
	lastVector []float32
}

func (s *stubEpisodeStore) Search(_ context.Context, _ string, _ string, embedding []float32, limit int) ([]MemoryEpisode, error) {
	s.calls++
	s.lastLimit = limit
	s.lastVector = append([]float32(nil), embedding...)
	return s.entries, nil
}

type stubProfileEntry struct {
	key     string
	summary string
}

func (e stubProfileEntry) ProfileKey() string     { return e.key }
func (e stubProfileEntry) ProfileSummary() string { return e.summary }

type stubEpisode struct {
	summary  string
	distance float64
}

func (e stubEpisode) EpisodeSummary() string   { return e.summary }
func (e stubEpisode) EpisodeDistance() float64 { return e.distance }

type stubEmbeddingProvider struct{ embedding []float32 }

func (s stubEmbeddingProvider) EmbedQuery(context.Context, string) ([]float32, error) {
	return append([]float32(nil), s.embedding...), nil
}

type stubSummaryReader struct {
	summaries map[string]string
	err       error
}

func (s *stubSummaryReader) GetSummary(_ context.Context, sessionKey string) (string, error) {
	if s.err != nil {
		return "", s.err
	}
	return s.summaries[sessionKey], nil
}

type stubMemoryBundleService struct {
	bundle      memoryservice.Bundle
	err         error
	calls       int
	lastRequest memoryservice.BundleRequest
}

func (s *stubMemoryBundleService) BuildBundle(_ context.Context, req memoryservice.BundleRequest) (memoryservice.Bundle, error) {
	s.calls++
	s.lastRequest = req
	if s.err != nil {
		return memoryservice.Bundle{}, s.err
	}
	return s.bundle, nil
}

type stubWorkingStore struct {
	snapshots  map[string]WorkingMemorySnapshot
	getCalls   []string
	saveCalls  []WorkingMemorySnapshot
	clearCalls []string
	errGet     error
	errSave    error
	errClear   error
}

func (s *stubWorkingStore) Get(_ context.Context, sessionKey string) (WorkingMemorySnapshot, error) {
	s.getCalls = append(s.getCalls, sessionKey)
	if s.errGet != nil {
		return WorkingMemorySnapshot{}, s.errGet
	}
	snapshot, ok := s.snapshots[sessionKey]
	if !ok {
		return WorkingMemorySnapshot{}, ErrWorkingMemoryNotFound
	}
	return snapshot, nil
}

func (s *stubWorkingStore) Save(_ context.Context, snapshot WorkingMemorySnapshot) (WorkingMemorySnapshot, error) {
	s.saveCalls = append(s.saveCalls, snapshot)
	if s.errSave != nil {
		return WorkingMemorySnapshot{}, s.errSave
	}
	if s.snapshots == nil {
		s.snapshots = map[string]WorkingMemorySnapshot{}
	}
	if snapshot.MemoryType == "" {
		snapshot.MemoryType = "working"
	}
	if snapshot.ProvenanceJSON == "" {
		snapshot.ProvenanceJSON = `{"source_type":"` + snapshot.SourceType + `","source_id":"` + snapshot.SourceID + `"}`
	}
	s.snapshots[snapshot.SessionKey] = snapshot
	return snapshot, nil
}

func (s *stubWorkingStore) Clear(_ context.Context, sessionKey string) error {
	s.clearCalls = append(s.clearCalls, sessionKey)
	if s.errClear != nil {
		return s.errClear
	}
	delete(s.snapshots, sessionKey)
	return nil
}

type stubTransientWorkingStore struct {
	states     map[string]TransientWorkingState
	saveCalls  []TransientWorkingState
	clearCalls []string
	errGet     error
	errSave    error
	errClear   error
}

func (s *stubTransientWorkingStore) Get(_ context.Context, sessionKey, runID string) (TransientWorkingState, error) {
	if s.errGet != nil {
		return TransientWorkingState{}, s.errGet
	}
	state, ok := s.states[sessionKey+":"+runID]
	if !ok {
		return TransientWorkingState{}, ErrTransientWorkingStateNotFound
	}
	return state, nil
}

func (s *stubTransientWorkingStore) Save(_ context.Context, state TransientWorkingState, _ time.Duration) (TransientWorkingState, error) {
	s.saveCalls = append(s.saveCalls, state)
	if s.errSave != nil {
		return TransientWorkingState{}, s.errSave
	}
	if s.states == nil {
		s.states = map[string]TransientWorkingState{}
	}
	s.states[state.SessionKey+":"+state.RunID] = state
	return state, nil
}

func (s *stubTransientWorkingStore) Clear(_ context.Context, sessionKey, runID string) error {
	s.clearCalls = append(s.clearCalls, sessionKey+":"+runID)
	if s.errClear != nil {
		return s.errClear
	}
	delete(s.states, sessionKey+":"+runID)
	return nil
}

func testEmbeddingVector(value float32) []float32 {
	vector := make([]float32, embeddings.VectorDimensions)
	for i := range vector {
		vector[i] = value
	}
	return vector
}

func (m *mockProvider) ContinueRun(context.Context, transport.ContinueRunRequest) (transport.EventStream, error) {
	return nil, nil
}

func (m *mockProvider) SubmitToolResult(_ context.Context, req transport.SubmitToolResultRequest) (transport.EventStream, error) {
	m.submittedResults = append(m.submittedResults, req)
	ch := make(chan transport.TransportEvent, len(m.submitEvents))
	for _, event := range m.submitEvents {
		if event.RunID == "" {
			event.RunID = req.RunID
		}
		ch <- event
	}
	close(ch)
	return ch, nil
}

func (m *mockProvider) CancelRun(context.Context, transport.CancelRunRequest) (*transport.TransportEvent, error) {
	return nil, nil
}

type delayedProvider struct {
	delay time.Duration
}

func (p *delayedProvider) Name() string { return "openai" }

func (p *delayedProvider) Capabilities(context.Context, transport.TransportRunContext) (transport.CapabilitySnapshot, error) {
	return transport.CapabilitySnapshot{SupportsStreaming: true}, nil
}

func (p *delayedProvider) StartRun(_ context.Context, req transport.StartRunRequest) (transport.EventStream, error) {
	ch := make(chan transport.TransportEvent, 3)
	go func() {
		defer close(ch)
		ch <- transport.NewRunStartedEvent(req.Context.RunID, "openai", transport.CapabilitySnapshot{SupportsStreaming: true}, &transport.ProviderSessionRef{ProviderName: "openai", ResponseRef: "resp_123"})
		time.Sleep(p.delay)
		ch <- transport.NewAssistantFinalEvent(req.Context.RunID, "openai", transport.AssistantFinal{Content: "Hello after renew", FinishReason: "completed"})
		ch <- transport.NewTerminalEvent(req.Context.RunID, transport.EventTypeRunCompleted, "openai")
	}()
	return ch, nil
}

func (p *delayedProvider) ContinueRun(context.Context, transport.ContinueRunRequest) (transport.EventStream, error) {
	return nil, nil
}

func (p *delayedProvider) SubmitToolResult(context.Context, transport.SubmitToolResultRequest) (transport.EventStream, error) {
	return nil, nil
}

type mockToolExecutor struct {
	calls  []*toolbrokerv1.ToolCall
	result *toolbrokerv1.ToolResult
	err    error
}

func (m *mockToolExecutor) ExecuteToolCall(_ context.Context, call *toolbrokerv1.ToolCall) (*toolbrokerv1.ToolResult, error) {
	m.calls = append(m.calls, call)
	if m.err != nil {
		return nil, m.err
	}
	if m.result == nil {
		return &toolbrokerv1.ToolResult{ToolCallId: call.GetToolCallId(), RunId: call.GetRunId(), ToolName: call.GetToolName(), Status: "completed", ResultJson: `{}`}, nil
	}
	result := *m.result
	if result.ToolCallId == "" {
		result.ToolCallId = call.GetToolCallId()
	}
	if result.RunId == "" {
		result.RunId = call.GetRunId()
	}
	if result.ToolName == "" {
		result.ToolName = call.GetToolName()
	}
	return &result, nil
}

func (p *delayedProvider) CancelRun(context.Context, transport.CancelRunRequest) (*transport.TransportEvent, error) {
	return nil, nil
}

type mockApprovalChecker struct {
	toolsRequiringApproval map[string]bool
}

func (m *mockApprovalChecker) RequiresApproval(_ context.Context, toolName string) (bool, error) {
	return m.toolsRequiringApproval[toolName], nil
}

type recordingDeliverySink struct {
	events           []DeliveryEvent
	approvalRequests []ApprovalRequest
}

func (r *recordingDeliverySink) DeliverAssistantDelta(_ context.Context, event DeliveryEvent) error {
	r.events = append(r.events, event)
	return nil
}

func (r *recordingDeliverySink) DeliverAssistantFinal(_ context.Context, event DeliveryEvent) error {
	r.events = append(r.events, event)
	return nil
}

func (r *recordingDeliverySink) DeliverApprovalRequest(_ context.Context, req ApprovalRequest) error {
	r.approvalRequests = append(r.approvalRequests, req)
	return nil
}

func (r *recordingDeliverySink) contents() []string {
	result := make([]string, 0, len(r.events))
	for _, event := range r.events {
		result = append(result, event.Content)
	}
	return result
}
