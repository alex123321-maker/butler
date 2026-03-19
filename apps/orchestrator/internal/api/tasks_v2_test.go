package api

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	artifacts "github.com/butler/butler/apps/orchestrator/internal/artifacts"
	deliveryevents "github.com/butler/butler/apps/orchestrator/internal/deliveryevents"
	"github.com/butler/butler/apps/orchestrator/internal/run"
	"github.com/butler/butler/apps/orchestrator/internal/session"
	"github.com/butler/butler/internal/memory/transcript"
)

type fakeTaskReader struct {
	items      []run.TaskRow
	err        error
	lastParams run.TaskListParams
}

func (f *fakeTaskReader) ListTasks(_ context.Context, params run.TaskListParams) ([]run.TaskRow, error) {
	f.lastParams = params
	if f.err != nil {
		return nil, f.err
	}
	return f.items, nil
}

type fakeRunReader struct {
	record run.Record
	err    error
}

func (f *fakeRunReader) GetRun(_ context.Context, _ string) (run.Record, error) {
	if f.err != nil {
		return run.Record{}, f.err
	}
	return f.record, nil
}

func (f *fakeRunReader) ListRunsBySessionKey(_ context.Context, _ string) ([]run.Record, error) {
	return nil, nil
}

type fakeSessionReader struct {
	record session.SessionRecord
	err    error
}

func (f *fakeSessionReader) ListSessions(_ context.Context, _, _ int) ([]session.SessionRecord, error) {
	return nil, nil
}

func (f *fakeSessionReader) GetSessionByKey(_ context.Context, _ string) (session.SessionRecord, error) {
	if f.err != nil {
		return session.SessionRecord{}, f.err
	}
	return f.record, nil
}

type fakeTranscriptStore struct {
	tx  transcript.Transcript
	err error
}

func (f *fakeTranscriptStore) GetRunTranscript(_ context.Context, _ string) (transcript.Transcript, error) {
	if f.err != nil {
		return transcript.Transcript{}, f.err
	}
	return f.tx, nil
}

type fakeTransitionStore struct {
	items []run.StateTransition
	err   error
}

func (f *fakeTransitionStore) ListTransitions(_ context.Context, _ string) ([]run.StateTransition, error) {
	if f.err != nil {
		return nil, f.err
	}
	return f.items, nil
}

type fakeTaskArtifactsStore struct {
	items []artifacts.Record
	err   error
}

type fakeTaskDeliveryStore struct {
	rec *deliveryevents.Record
	err error
}

func (f *fakeTaskDeliveryStore) LatestByRun(_ context.Context, _ string) (*deliveryevents.Record, error) {
	if f.err != nil {
		return nil, f.err
	}
	return f.rec, nil
}

func (f *fakeTaskArtifactsStore) ListArtifactsByRun(_ context.Context, _ string, _ int) ([]artifacts.Record, error) {
	if f.err != nil {
		return nil, f.err
	}
	return f.items, nil
}

func TestHandleListTasks_ParsesFiltersAndReturnsPayload(t *testing.T) {
	t.Parallel()

	now := time.Date(2026, 3, 19, 10, 0, 0, 0, time.UTC)
	reader := &fakeTaskReader{items: []run.TaskRow{
		{
			TaskID:         "run-1",
			RunID:          "run-1",
			SessionKey:     "telegram:chat:1",
			SourceChannel:  "telegram",
			SessionChannel: "telegram",
			RunState:       "awaiting_approval",
			StartedAt:      now,
			UpdatedAt:      now,
			ModelProvider:  "openai",
			AutonomyMode:   "mode_1",
			OutcomeSummary: "need your approval",
			RiskLevel:      "medium",
		},
	}}

	server := NewTaskViewServer(reader, nil, nil, nil, nil, nil, nil)
	request := httptest.NewRequest(http.MethodGet, "/api/v2/tasks?status=waiting_for_reply_in_telegram&needs_user_action=true&waiting_reason=approval_required&source_channel=telegram&provider=openai&from=2026-03-19T00:00:00Z&to=2026-03-20T00:00:00Z&query=approval&limit=10&offset=5&sort=updated_at_desc", nil)
	recorder := httptest.NewRecorder()

	server.HandleListTasks().ServeHTTP(recorder, request)

	if recorder.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", recorder.Code, recorder.Body.String())
	}

	if reader.lastParams.Status != "waiting_for_reply_in_telegram" {
		t.Fatalf("expected status filter, got %q", reader.lastParams.Status)
	}
	if reader.lastParams.NeedsUserAction == nil || !*reader.lastParams.NeedsUserAction {
		t.Fatal("expected needs_user_action=true")
	}
	if reader.lastParams.WaitingReason != "approval_required" {
		t.Fatalf("expected waiting_reason, got %q", reader.lastParams.WaitingReason)
	}
	if reader.lastParams.SourceChannel != "telegram" {
		t.Fatalf("expected source channel, got %q", reader.lastParams.SourceChannel)
	}
	if reader.lastParams.Provider != "openai" {
		t.Fatalf("expected provider, got %q", reader.lastParams.Provider)
	}
	if reader.lastParams.Limit != 10 || reader.lastParams.Offset != 5 {
		t.Fatalf("expected limit/offset 10/5, got %d/%d", reader.lastParams.Limit, reader.lastParams.Offset)
	}
	if reader.lastParams.Sort != run.TaskSortUpdatedAtDesc {
		t.Fatalf("expected updated_at_desc sort, got %q", reader.lastParams.Sort)
	}

	var payload map[string]any
	if err := json.Unmarshal(recorder.Body.Bytes(), &payload); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	tasks := payload["tasks"].([]any)
	if len(tasks) != 1 {
		t.Fatalf("expected 1 task, got %d", len(tasks))
	}
	item := tasks[0].(map[string]any)
	if item["status"] != "waiting_for_reply_in_telegram" {
		t.Fatalf("expected mapped waiting status, got %v", item["status"])
	}
	if item["user_action_channel"] != "telegram" {
		t.Fatalf("expected telegram action channel, got %v", item["user_action_channel"])
	}
	if item["waiting_reason"] != "approval_required" {
		t.Fatalf("expected approval_required waiting reason, got %v", item["waiting_reason"])
	}
}

func TestHandleListTasks_ValidatesQueryTypes(t *testing.T) {
	t.Parallel()

	server := NewTaskViewServer(&fakeTaskReader{}, nil, nil, nil, nil, nil, nil)
	request := httptest.NewRequest(http.MethodGet, "/api/v2/tasks?limit=bad", nil)
	recorder := httptest.NewRecorder()

	server.HandleListTasks().ServeHTTP(recorder, request)

	if recorder.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", recorder.Code)
	}
}

func TestTaskPresentationMapping_TimedOutAndWebApproval(t *testing.T) {
	t.Parallel()

	status, needsAction, actionChannel, waitingReason := mapTaskPresentation("timed_out", "telegram")
	if status != "completed_with_issues" || needsAction || actionChannel != "none" || waitingReason != "" {
		t.Fatalf("unexpected timed_out mapping: %s %v %s %s", status, needsAction, actionChannel, waitingReason)
	}

	status, needsAction, actionChannel, waitingReason = mapTaskPresentation("awaiting_approval", "web")
	if status != "waiting_for_approval" || !needsAction || actionChannel != "web" || waitingReason != "approval_required" {
		t.Fatalf("unexpected awaiting_approval(web) mapping: %s %v %s %s", status, needsAction, actionChannel, waitingReason)
	}
}

func TestHandleGetTaskDetail_WaitingForTelegramAndDebugRefs(t *testing.T) {
	t.Parallel()

	now := time.Date(2026, 3, 19, 10, 0, 0, 0, time.UTC)
	server := NewTaskViewServer(
		&fakeTaskReader{},
		&fakeRunReader{record: run.Record{
			RunID:         "run-42",
			SessionKey:    "telegram:chat:42",
			CurrentState:  "awaiting_approval",
			Status:        "awaiting_approval",
			ModelProvider: "openai",
			AutonomyMode:  "mode_1",
			StartedAt:     now,
			UpdatedAt:     now,
		}},
		&fakeSessionReader{record: session.SessionRecord{SessionKey: "telegram:chat:42", Channel: "telegram"}},
		&fakeTranscriptStore{tx: transcript.Transcript{Messages: []transcript.Message{
			{Role: "user", Content: "please deploy latest build"},
			{Role: "assistant", Content: "Need approval before deploy."},
		}}},
		&fakeTransitionStore{items: []run.StateTransition{{FromState: "tool_pending", ToState: "awaiting_approval", TriggeredBy: "orchestrator", TransitionedAt: now}}},
		&fakeTaskArtifactsStore{items: []artifacts.Record{{ArtifactID: "art-1", ArtifactType: artifacts.TypeAssistantFinal, Title: "Assistant final response", Summary: "Need approval before deploy.", ContentFormat: "text", SourceType: "message", SourceRef: "run-42", CreatedAt: now}}},
		&fakeTaskDeliveryStore{rec: &deliveryevents.Record{RunID: "run-42", SessionKey: "telegram:chat:42", Channel: deliveryevents.ChannelTelegram, DeliveryType: deliveryevents.TypeApprovalRequest, State: deliveryevents.StateWaiting, CreatedAt: now}},
	)

	request := httptest.NewRequest(http.MethodGet, "/api/v2/tasks/run-42", nil)
	recorder := httptest.NewRecorder()

	server.HandleGetTaskDetail("/api/v2/tasks/").ServeHTTP(recorder, request)

	if recorder.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", recorder.Code, recorder.Body.String())
	}

	var payload map[string]any
	if err := json.Unmarshal(recorder.Body.Bytes(), &payload); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	waiting := payload["waiting_state"].(map[string]any)
	if waiting["user_action_channel"] != "telegram" {
		t.Fatalf("expected telegram waiting channel, got %v", waiting["user_action_channel"])
	}
	if waiting["telegram_delivery_state"].(map[string]any)["state"] != deliveryevents.StateWaiting {
		t.Fatalf("expected delivery waiting state, got %v", waiting["telegram_delivery_state"])
	}
	if !strings.Contains(waiting["note"].(string), "Telegram") {
		t.Fatalf("expected telegram waiting note, got %v", waiting["note"])
	}
	debugRefs := payload["debug_refs"].(map[string]any)
	if debugRefs["run"] != "/api/v1/runs/run-42" {
		t.Fatalf("unexpected debug run ref: %v", debugRefs["run"])
	}
	timeline := payload["timeline_preview"].([]any)
	if len(timeline) != 1 {
		t.Fatalf("expected one transition in timeline preview, got %d", len(timeline))
	}
	if artifactsList := payload["artifacts"].([]any); len(artifactsList) != 1 {
		t.Fatalf("expected 1 artifact in task detail, got %d", len(artifactsList))
	}
}

func TestHandleGetTaskDetail_NotFound(t *testing.T) {
	t.Parallel()

	server := NewTaskViewServer(
		&fakeTaskReader{},
		&fakeRunReader{err: run.ErrRunNotFound},
		nil,
		nil,
		nil,
		nil,
		nil,
	)

	request := httptest.NewRequest(http.MethodGet, "/api/v2/tasks/run-missing", nil)
	recorder := httptest.NewRecorder()

	server.HandleGetTaskDetail("/api/v2/tasks/").ServeHTTP(recorder, request)

	if recorder.Code != http.StatusNotFound {
		t.Fatalf("expected 404, got %d", recorder.Code)
	}
}
