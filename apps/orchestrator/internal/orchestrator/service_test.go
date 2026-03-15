package orchestrator

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/butler/butler/apps/orchestrator/internal/session"
	commonv1 "github.com/butler/butler/internal/gen/common/v1"
	runv1 "github.com/butler/butler/internal/gen/run/v1"
	sessionv1 "github.com/butler/butler/internal/gen/session/v1"
	toolbrokerv1 "github.com/butler/butler/internal/gen/toolbroker/v1"
	"github.com/butler/butler/internal/ingress"
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

type recordingDeliverySink struct {
	events []DeliveryEvent
}

func (r *recordingDeliverySink) DeliverAssistantDelta(_ context.Context, event DeliveryEvent) error {
	r.events = append(r.events, event)
	return nil
}

func (r *recordingDeliverySink) DeliverAssistantFinal(_ context.Context, event DeliveryEvent) error {
	r.events = append(r.events, event)
	return nil
}

func (r *recordingDeliverySink) contents() []string {
	result := make([]string, 0, len(r.events))
	for _, event := range r.events {
		result = append(result, event.Content)
	}
	return result
}
