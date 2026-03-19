package api

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	deliveryevents "github.com/butler/butler/apps/orchestrator/internal/deliveryevents"
	"github.com/butler/butler/apps/orchestrator/internal/run"
)

type stagedTaskReader struct {
	byStatus map[string][]run.TaskRow
	errByKey map[string]error
}

type fakeOverviewDeliveryStore struct {
	byRun map[string]*deliveryevents.Record
}

func (f *fakeOverviewDeliveryStore) LatestByRun(_ context.Context, runID string) (*deliveryevents.Record, error) {
	if f.byRun == nil {
		return nil, nil
	}
	return f.byRun[runID], nil
}

func (s *stagedTaskReader) ListTasks(_ context.Context, params run.TaskListParams) ([]run.TaskRow, error) {
	key := params.Status
	if key == "" {
		key = "all"
	}
	if err, ok := s.errByKey[key]; ok {
		return nil, err
	}
	return s.byStatus[key], nil
}

func TestHandleGetOverview_AggregatesSectionsAndCounts(t *testing.T) {
	t.Parallel()

	now := time.Date(2026, 3, 19, 12, 0, 0, 0, time.UTC)
	reader := &stagedTaskReader{
		byStatus: map[string][]run.TaskRow{
			"in_progress": {
				{TaskID: "run-active", RunID: "run-active", SessionKey: "telegram:chat:1", SourceChannel: "telegram", SessionChannel: "telegram", RunState: "model_running", UpdatedAt: now, ModelProvider: "openai", RiskLevel: "low"},
			},
			"waiting_for_reply_in_telegram": {
				{TaskID: "run-wait-telegram", RunID: "run-wait-telegram", SessionKey: "telegram:chat:2", SourceChannel: "telegram", SessionChannel: "telegram", RunState: "awaiting_approval", UpdatedAt: now.Add(-time.Minute), ModelProvider: "openai", RiskLevel: "medium"},
			},
			"waiting_for_approval": {
				{TaskID: "run-wait-web", RunID: "run-wait-web", SessionKey: "web:user:3", SourceChannel: "web", SessionChannel: "web", RunState: "awaiting_approval", UpdatedAt: now.Add(-2 * time.Minute), ModelProvider: "openai", RiskLevel: "medium"},
			},
			"failed": {
				{TaskID: "run-failed", RunID: "run-failed", SessionKey: "telegram:chat:4", SourceChannel: "telegram", SessionChannel: "telegram", RunState: "failed", UpdatedAt: now.Add(-3 * time.Minute), ErrorSummary: "tool error", ModelProvider: "openai", RiskLevel: "high"},
			},
			"completed": {
				{TaskID: "run-completed", RunID: "run-completed", SessionKey: "telegram:chat:5", SourceChannel: "telegram", SessionChannel: "telegram", RunState: "completed", UpdatedAt: now.Add(-4 * time.Minute), OutcomeSummary: "done", ModelProvider: "openai", RiskLevel: "low"},
			},
			"completed_with_issues": {},
			"cancelled":             {},
		},
		errByKey: map[string]error{},
	}

	server := NewOverviewServer(reader, &fakeOverviewDeliveryStore{byRun: map[string]*deliveryevents.Record{"run-wait-web": {RunID: "run-wait-web", Channel: deliveryevents.ChannelTelegram, State: deliveryevents.StateWaiting}}})
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
	if len(payload["attention_items"].([]any)) == 0 {
		t.Fatal("expected non-empty attention_items")
	}
	if len(payload["active_tasks"].([]any)) != 1 {
		t.Fatalf("expected one active task, got %d", len(payload["active_tasks"].([]any)))
	}
	counts := payload["counts"].(map[string]any)
	if int(counts["approvals_pending_count"].(float64)) != 2 {
		t.Fatalf("expected approvals_pending_count=2, got %v", counts["approvals_pending_count"])
	}
	if int(counts["tasks_waiting_for_telegram_reply_count"].(float64)) != 2 {
		t.Fatalf("expected waiting telegram count=2, got %v", counts["tasks_waiting_for_telegram_reply_count"])
	}
	if int(counts["failed_tasks_count"].(float64)) < 1 {
		t.Fatalf("expected failed tasks count >=1, got %v", counts["failed_tasks_count"])
	}
	if len(payload["partial_errors"].([]any)) != 0 {
		t.Fatalf("expected no partial errors, got %v", payload["partial_errors"])
	}
}

func TestHandleGetOverview_PartialDegradation(t *testing.T) {
	t.Parallel()

	reader := &stagedTaskReader{
		byStatus: map[string][]run.TaskRow{
			"in_progress": {},
		},
		errByKey: map[string]error{
			"failed": errors.New("db down"),
		},
	}

	server := NewOverviewServer(reader, nil)
	request := httptest.NewRequest(http.MethodGet, "/api/v2/overview", nil)
	recorder := httptest.NewRecorder()

	server.HandleGetOverview().ServeHTTP(recorder, request)

	if recorder.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", recorder.Code)
	}
	var payload map[string]any
	if err := json.Unmarshal(recorder.Body.Bytes(), &payload); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	partial := payload["partial_errors"].([]any)
	if len(partial) == 0 {
		t.Fatal("expected partial errors when one source fails")
	}
	summary := payload["system_summary"].(map[string]any)
	if summary["state"] != "degraded" {
		t.Fatalf("expected degraded state, got %v", summary["state"])
	}
}
