package api

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	artifacts "github.com/butler/butler/apps/orchestrator/internal/artifacts"
	deliveryevents "github.com/butler/butler/apps/orchestrator/internal/deliveryevents"
	"github.com/butler/butler/apps/orchestrator/internal/run"
	"github.com/butler/butler/apps/orchestrator/internal/session"
	"github.com/butler/butler/internal/memory/transcript"
)

func TestContract_ListTasks_ContainsRequiredFields(t *testing.T) {
	t.Parallel()

	now := time.Date(2026, 3, 19, 10, 0, 0, 0, time.UTC)
	server := NewTaskViewServer(&fakeTaskReader{items: []run.TaskRow{
		{
			TaskID:         "run-contract-1",
			RunID:          "run-contract-1",
			SessionKey:     "telegram:chat:1",
			SourceChannel:  "telegram",
			SessionChannel: "telegram",
			RunState:       "awaiting_approval",
			StartedAt:      now,
			UpdatedAt:      now,
			ModelProvider:  "openai",
			AutonomyMode:   "mode_1",
			RiskLevel:      "medium",
		},
	}}, nil, nil, nil, nil, nil, nil)

	request := httptest.NewRequest(http.MethodGet, "/api/v2/tasks", nil)
	recorder := httptest.NewRecorder()

	server.HandleListTasks().ServeHTTP(recorder, request)

	if recorder.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", recorder.Code, recorder.Body.String())
	}

	var payload map[string]any
	if err := json.Unmarshal(recorder.Body.Bytes(), &payload); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	tasks := requireSliceField(t, payload, "tasks")
	if len(tasks) != 1 {
		t.Fatalf("expected one task in response, got %d", len(tasks))
	}

	task := requireMapField(t, tasks[0], "tasks[0]")
	for _, field := range []string{
		"task_id",
		"run_id",
		"session_key",
		"source_channel",
		"status",
		"run_state",
		"needs_user_action",
		"user_action_channel",
		"waiting_reason",
		"started_at",
		"updated_at",
		"finished_at",
		"outcome_summary",
		"error_summary",
		"risk_level",
		"provider",
		"autonomy_mode",
	} {
		requireKey(t, task, field)
	}

	if _, ok := task["needs_user_action"].(bool); !ok {
		t.Fatalf("expected bool needs_user_action, got %T", task["needs_user_action"])
	}
	if _, ok := task["waiting_reason"].(string); !ok {
		t.Fatalf("expected string waiting_reason, got %T", task["waiting_reason"])
	}
}

func TestContract_GetTaskDetail_ContainsTelegramDeliveryStateContract(t *testing.T) {
	t.Parallel()

	now := time.Date(2026, 3, 19, 10, 0, 0, 0, time.UTC)
	server := NewTaskViewServer(
		&fakeTaskReader{},
		&fakeRunReader{record: run.Record{
			RunID:         "run-contract-detail",
			SessionKey:    "telegram:chat:42",
			CurrentState:  "awaiting_approval",
			Status:        "awaiting_approval",
			ModelProvider: "openai",
			AutonomyMode:  "mode_1",
			StartedAt:     now,
			UpdatedAt:     now,
		}},
		&fakeSessionReader{record: session.SessionRecord{SessionKey: "telegram:chat:42", Channel: "telegram"}},
		&fakeTranscriptStore{tx: transcript.Transcript{Messages: []transcript.Message{{Role: "user", Content: "ship release"}, {Role: "assistant", Content: "waiting for approval"}}}},
		&fakeTransitionStore{},
		&fakeTaskArtifactsStore{items: []artifacts.Record{{ArtifactID: "art-1", ArtifactType: artifacts.TypeAssistantFinal, Title: "final", CreatedAt: now}}},
		&fakeTaskDeliveryStore{rec: &deliveryevents.Record{RunID: "run-contract-detail", SessionKey: "telegram:chat:42", Channel: deliveryevents.ChannelTelegram, DeliveryType: deliveryevents.TypeApprovalRequest, State: deliveryevents.StateWaiting, CreatedAt: now}},
	)

	request := httptest.NewRequest(http.MethodGet, "/api/v2/tasks/run-contract-detail", nil)
	recorder := httptest.NewRecorder()

	server.HandleGetTaskDetail("/api/v2/tasks/").ServeHTTP(recorder, request)

	if recorder.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", recorder.Code, recorder.Body.String())
	}

	var payload map[string]any
	if err := json.Unmarshal(recorder.Body.Bytes(), &payload); err != nil {
		t.Fatalf("decode response: %v", err)
	}

	waitingState := requireMapField(t, requireKey(t, payload, "waiting_state"), "waiting_state")
	if _, ok := waitingState["needs_user_action"].(bool); !ok {
		t.Fatalf("expected bool waiting_state.needs_user_action, got %T", waitingState["needs_user_action"])
	}
	if _, ok := waitingState["waiting_reason"].(string); !ok {
		t.Fatalf("expected string waiting_state.waiting_reason, got %T", waitingState["waiting_reason"])
	}

	deliveryState := requireMapField(t, requireKey(t, waitingState, "telegram_delivery_state"), "waiting_state.telegram_delivery_state")
	for _, field := range []string{"channel", "state", "delivery_type", "error_message"} {
		requireKey(t, deliveryState, field)
	}
	if deliveryState["state"] != deliveryevents.StateWaiting {
		t.Fatalf("expected telegram delivery state %q, got %v", deliveryevents.StateWaiting, deliveryState["state"])
	}
}

func TestContract_GetOverview_ContainsUXCriticalFields(t *testing.T) {
	t.Parallel()

	now := time.Date(2026, 3, 19, 12, 0, 0, 0, time.UTC)
	reader := &stagedTaskReader{
		byStatus: map[string][]run.TaskRow{
			"in_progress": {},
			"waiting_for_reply_in_telegram": {
				{TaskID: "run-wait-telegram", RunID: "run-wait-telegram", SessionKey: "telegram:chat:2", SourceChannel: "telegram", SessionChannel: "telegram", RunState: "awaiting_approval", UpdatedAt: now.Add(-time.Minute), ModelProvider: "openai", RiskLevel: "medium"},
			},
			"waiting_for_approval": {
				{TaskID: "run-wait-web", RunID: "run-wait-web", SessionKey: "web:user:3", SourceChannel: "web", SessionChannel: "web", RunState: "awaiting_approval", UpdatedAt: now.Add(-2 * time.Minute), ModelProvider: "openai", RiskLevel: "medium"},
			},
			"failed":                {},
			"completed":             {},
			"completed_with_issues": {},
			"cancelled":             {},
		},
		errByKey: map[string]error{},
	}

	server := NewOverviewServer(reader, nil)
	request := httptest.NewRequest(http.MethodGet, "/api/v2/overview", nil)
	recorder := httptest.NewRecorder()

	server.HandleGetOverview().ServeHTTP(recorder, request)

	if recorder.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", recorder.Code, recorder.Body.String())
	}

	var payload map[string]any
	if err := json.Unmarshal(recorder.Body.Bytes(), &payload); err != nil {
		t.Fatalf("decode response: %v", err)
	}

	attentionItems := requireSliceField(t, payload, "attention_items")
	if len(attentionItems) < 1 {
		t.Fatal("expected non-empty attention_items")
	}
	item := requireMapField(t, attentionItems[0], "attention_items[0]")
	for _, field := range []string{"needs_user_action", "waiting_reason", "status"} {
		requireKey(t, item, field)
	}
	if _, ok := item["needs_user_action"].(bool); !ok {
		t.Fatalf("expected bool attention_items[0].needs_user_action, got %T", item["needs_user_action"])
	}
	if _, ok := item["waiting_reason"].(string); !ok {
		t.Fatalf("expected string attention_items[0].waiting_reason, got %T", item["waiting_reason"])
	}
}

func requireKey(t *testing.T, payload map[string]any, field string) any {
	t.Helper()
	value, ok := payload[field]
	if !ok {
		t.Fatalf("missing required field %q", field)
	}
	return value
}

func requireMapField(t *testing.T, value any, path string) map[string]any {
	t.Helper()
	item, ok := value.(map[string]any)
	if !ok {
		t.Fatalf("expected %s to be object, got %T", path, value)
	}
	return item
}

func requireSliceField(t *testing.T, payload map[string]any, field string) []any {
	t.Helper()
	value := requireKey(t, payload, field)
	items, ok := value.([]any)
	if !ok {
		t.Fatalf("expected %s to be array, got %T", field, value)
	}
	return items
}
