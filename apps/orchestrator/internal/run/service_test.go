package run

import (
	"context"
	"testing"
	"time"

	commonv1 "github.com/butler/butler/internal/gen/common/v1"
	runv1 "github.com/butler/butler/internal/gen/run/v1"
	sessionv1 "github.com/butler/butler/internal/gen/session/v1"
)

func TestCreateRunPersistsCreatedState(t *testing.T) {
	repo := &memoryRepository{}
	service := NewService(repo, nil)

	resp, err := service.CreateRun(context.Background(), &sessionv1.CreateRunRequest{
		SessionKey:    "telegram:chat:42",
		InputEvent:    &runv1.InputEvent{EventId: "event-1", EventType: runv1.InputEventType_INPUT_EVENT_TYPE_USER_MESSAGE},
		AutonomyMode:  commonv1.AutonomyMode_AUTONOMY_MODE_1,
		ModelProvider: "openai",
		MetadataJson:  `{"origin":"test"}`,
	})
	if err != nil {
		t.Fatalf("CreateRun returned error: %v", err)
	}
	if resp.GetCurrentState() != commonv1.RunState_RUN_STATE_CREATED {
		t.Fatalf("expected created state, got %v", resp.GetCurrentState())
	}
	if repo.created.MetadataJSON != `{"origin":"test"}` {
		t.Fatalf("unexpected metadata %q", repo.created.MetadataJSON)
	}
}

func TestFindRunByIdempotencyKeyReturnsExistingRun(t *testing.T) {
	createdAt := time.Date(2026, time.March, 15, 13, 0, 0, 0, time.UTC)
	repo := &memoryRepository{records: map[string]Record{
		"run-1": {RunID: "run-1", SessionKey: "telegram:chat:42", InputEventID: "event-1", IdempotencyKey: "dup-1", Status: "created", AutonomyMode: "mode_1", CurrentState: "created", ModelProvider: "openai", StartedAt: createdAt, UpdatedAt: createdAt},
	}}
	service := NewService(repo, nil)

	run, err := service.FindRunByIdempotencyKey(context.Background(), "telegram:chat:42", "dup-1")
	if err != nil {
		t.Fatalf("FindRunByIdempotencyKey returned error: %v", err)
	}
	if run.GetRunId() != "run-1" {
		t.Fatalf("unexpected run id %q", run.GetRunId())
	}
}

func TestTransitionRunAllowsHappyPath(t *testing.T) {
	createdAt := time.Date(2026, time.March, 15, 13, 0, 0, 0, time.UTC)
	repo := &memoryRepository{records: map[string]Record{
		"run-1": {
			RunID:         "run-1",
			SessionKey:    "telegram:chat:42",
			InputEventID:  "event-1",
			Status:        "created",
			AutonomyMode:  "mode_1",
			CurrentState:  "created",
			ModelProvider: "openai",
			StartedAt:     createdAt,
			UpdatedAt:     createdAt,
		},
	}}
	service := NewService(repo, nil)

	resp, err := service.TransitionRun(context.Background(), &sessionv1.UpdateRunStateRequest{
		RunId:     "run-1",
		FromState: commonv1.RunState_RUN_STATE_CREATED,
		ToState:   commonv1.RunState_RUN_STATE_QUEUED,
	})
	if err != nil {
		t.Fatalf("TransitionRun returned error: %v", err)
	}
	if resp.GetCurrentState() != commonv1.RunState_RUN_STATE_QUEUED {
		t.Fatalf("expected queued state, got %v", resp.GetCurrentState())
	}
	if repo.records["run-1"].Status != "queued" {
		t.Fatalf("expected queued status, got %q", repo.records["run-1"].Status)
	}
}

func TestTransitionRunRejectsInvalidTransition(t *testing.T) {
	service := NewService(&memoryRepository{}, nil)

	_, err := service.TransitionRun(context.Background(), &sessionv1.UpdateRunStateRequest{
		RunId:     "run-1",
		FromState: commonv1.RunState_RUN_STATE_CREATED,
		ToState:   commonv1.RunState_RUN_STATE_PREPARING,
	})
	if err == nil {
		t.Fatal("expected invalid transition error")
	}
}

func TestTransitionRunTerminalStateSetsFinishedAt(t *testing.T) {
	createdAt := time.Date(2026, time.March, 15, 13, 0, 0, 0, time.UTC)
	repo := &memoryRepository{records: map[string]Record{
		"run-1": {
			RunID:         "run-1",
			SessionKey:    "telegram:chat:42",
			InputEventID:  "event-1",
			Status:        "finalizing",
			AutonomyMode:  "mode_1",
			CurrentState:  "finalizing",
			ModelProvider: "openai",
			StartedAt:     createdAt,
			UpdatedAt:     createdAt,
		},
	}}
	service := NewService(repo, nil)

	resp, err := service.TransitionRun(context.Background(), &sessionv1.UpdateRunStateRequest{
		RunId:     "run-1",
		FromState: commonv1.RunState_RUN_STATE_FINALIZING,
		ToState:   commonv1.RunState_RUN_STATE_COMPLETED,
	})
	if err != nil {
		t.Fatalf("TransitionRun returned error: %v", err)
	}
	if resp.GetFinishedAt() == "" {
		t.Fatal("expected finished_at for terminal transition")
	}
}

type memoryRepository struct {
	created Record
	records map[string]Record
}

func (m *memoryRepository) CreateRun(_ context.Context, record Record) (Record, error) {
	m.created = record
	if m.records == nil {
		m.records = make(map[string]Record)
	}
	m.records[record.RunID] = record
	return record, nil
}

func (m *memoryRepository) GetRun(_ context.Context, runID string) (Record, error) {
	record, ok := m.records[runID]
	if !ok {
		return Record{}, ErrRunNotFound
	}
	return record, nil
}

func (m *memoryRepository) FindRunByIdempotencyKey(_ context.Context, sessionKey, idempotencyKey string) (Record, error) {
	for _, record := range m.records {
		if record.SessionKey == sessionKey && record.IdempotencyKey == idempotencyKey {
			return record, nil
		}
	}
	return Record{}, ErrRunNotFound
}

func (m *memoryRepository) UpdateRun(_ context.Context, params UpdateParams) (Record, error) {
	record, ok := m.records[params.RunID]
	if !ok || record.CurrentState != params.CurrentState {
		return Record{}, ErrRunNotFound
	}
	record.CurrentState = params.NextState
	record.Status = params.Status
	record.LeaseID = params.LeaseID
	record.ErrorType = params.ErrorType
	record.ErrorMessage = params.ErrorMessage
	record.FinishedAt = params.FinishedAt
	record.UpdatedAt = params.UpdatedAt
	m.records[params.RunID] = record
	return record, nil
}
