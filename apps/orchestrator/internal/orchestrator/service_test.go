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
	provider := &mockProvider{events: []transport.TransportEvent{
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

func TestExecuteFailsRunOnTransportError(t *testing.T) {
	t.Parallel()

	runs := newMemoryRunManager()
	leases := &memoryLeaseManager{}
	service := NewService(&memorySessionRepo{}, leases, runs, &memoryTranscriptStore{}, &mockProvider{events: []transport.TransportEvent{
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

type memorySessionRepo struct{}

func (m *memorySessionRepo) CreateSession(_ context.Context, params session.CreateSessionParams) (session.SessionRecord, bool, error) {
	now := time.Now().UTC()
	return session.SessionRecord{SessionKey: params.SessionKey, UserID: params.UserID, Channel: params.Channel, MetadataJSON: params.MetadataJSON, CreatedAt: now, UpdatedAt: now}, true, nil
}

type memoryLeaseManager struct {
	releaseCount int
	leaseID      string
}

func (m *memoryLeaseManager) AcquireLease(_ context.Context, params session.AcquireLeaseParams) (session.LeaseRecord, error) {
	if m.leaseID == "" {
		m.leaseID = "lease-1"
	}
	now := time.Now().UTC()
	return session.LeaseRecord{LeaseID: m.leaseID, SessionKey: params.SessionKey, RunID: params.RunID, OwnerID: params.OwnerID, AcquiredAt: now, ExpiresAt: now.Add(params.TTL)}, nil
}

func (m *memoryLeaseManager) RenewLease(context.Context, string, time.Duration) (session.LeaseRecord, error) {
	return session.LeaseRecord{}, nil
}

func (m *memoryLeaseManager) ReleaseLease(context.Context, string) (bool, error) {
	m.releaseCount++
	return true, nil
}

type memoryRunManager struct {
	records map[string]*sessionv1.RunRecord
	history map[string][]commonv1.RunState
	nextID  int
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

func (m *memoryRunManager) statesFor(runID string) []commonv1.RunState {
	return append([]commonv1.RunState(nil), m.history[runID]...)
}

type memoryTranscriptStore struct {
	messages []transcript.Message
}

func (m *memoryTranscriptStore) AppendMessage(_ context.Context, message transcript.Message) (transcript.Message, error) {
	m.messages = append(m.messages, message)
	return message, nil
}

type mockProvider struct {
	events []transport.TransportEvent
	err    error
}

func (m *mockProvider) Name() string { return "openai" }

func (m *mockProvider) Capabilities(context.Context, transport.TransportRunContext) (transport.CapabilitySnapshot, error) {
	return transport.CapabilitySnapshot{SupportsStreaming: true}, nil
}

func (m *mockProvider) StartRun(_ context.Context, req transport.StartRunRequest) (transport.EventStream, error) {
	if m.err != nil {
		return nil, m.err
	}
	ch := make(chan transport.TransportEvent, len(m.events))
	for _, event := range m.events {
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

func (m *mockProvider) SubmitToolResult(context.Context, transport.SubmitToolResultRequest) (transport.EventStream, error) {
	return nil, nil
}

func (m *mockProvider) CancelRun(context.Context, transport.CancelRunRequest) (*transport.TransportEvent, error) {
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
