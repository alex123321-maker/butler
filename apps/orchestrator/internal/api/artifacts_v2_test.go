package api

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	activity "github.com/butler/butler/apps/orchestrator/internal/activity"
	artifacts "github.com/butler/butler/apps/orchestrator/internal/artifacts"
)

type fakeArtifactsStore struct {
	items    []artifacts.Record
	itemByID map[string]artifacts.Record
	err      error
}

type fakeTaskActivityStore struct {
	items []activity.Record
	err   error
}

func (f *fakeTaskActivityStore) ListActivities(_ context.Context, params activity.ListParams) ([]activity.Record, error) {
	if f.err != nil {
		return nil, f.err
	}
	return f.items, nil
}

func (f *fakeArtifactsStore) ListArtifacts(_ context.Context, params artifacts.ListParams) ([]artifacts.Record, error) {
	if f.err != nil {
		return nil, f.err
	}
	return f.items, nil
}

func (f *fakeArtifactsStore) ListArtifactsByRun(_ context.Context, runID string, limit int) ([]artifacts.Record, error) {
	if f.err != nil {
		return nil, f.err
	}
	return f.items, nil
}

func (f *fakeArtifactsStore) GetArtifactByID(_ context.Context, artifactID string) (artifacts.Record, error) {
	if f.err != nil {
		return artifacts.Record{}, f.err
	}
	item, ok := f.itemByID[artifactID]
	if !ok {
		return artifacts.Record{}, artifacts.ErrArtifactNotFound
	}
	return item, nil
}

func TestArtifactsEndpoints_ListGetAndTaskArtifacts(t *testing.T) {
	t.Parallel()

	now := time.Date(2026, 3, 19, 16, 30, 0, 0, time.UTC)
	record := artifacts.Record{ArtifactID: "art-1", RunID: "run-1", SessionKey: "telegram:chat:1", ArtifactType: artifacts.TypeAssistantFinal, Title: "Assistant final response", Summary: "done", ContentText: "done", ContentJSON: `{"kind":"assistant_final"}`, ContentFormat: "text", SourceType: "message", SourceRef: "run-1", CreatedAt: now, UpdatedAt: now}
	server := NewArtifactsServer(
		&fakeArtifactsStore{items: []artifacts.Record{record}, itemByID: map[string]artifacts.Record{"art-1": record}},
		&fakeTaskActivityStore{items: []activity.Record{{ActivityID: 1, RunID: "run-1", SessionKey: "telegram:chat:1", ActivityType: activity.TypeTaskReceived, Title: "Task received", Summary: "Task context prepared", DetailsJSON: `{"source":"telegram"}`, ActorType: "system", Severity: activity.SeverityInfo, CreatedAt: now}}},
	)

	listReq := httptest.NewRequest(http.MethodGet, "/api/v2/artifacts?type=assistant_final", nil)
	listRes := httptest.NewRecorder()
	server.HandleListArtifacts().ServeHTTP(listRes, listReq)
	if listRes.Code != http.StatusOK {
		t.Fatalf("expected 200 list, got %d", listRes.Code)
	}

	getReq := httptest.NewRequest(http.MethodGet, "/api/v2/artifacts/art-1", nil)
	getRes := httptest.NewRecorder()
	server.HandleGetArtifact("/api/v2/artifacts/").ServeHTTP(getRes, getReq)
	if getRes.Code != http.StatusOK {
		t.Fatalf("expected 200 get, got %d", getRes.Code)
	}

	taskReq := httptest.NewRequest(http.MethodGet, "/api/v2/tasks/run-1/artifacts", nil)
	taskRes := httptest.NewRecorder()
	server.HandleListTaskArtifacts("/api/v2/tasks/").ServeHTTP(taskRes, taskReq)
	if taskRes.Code != http.StatusOK {
		t.Fatalf("expected 200 task artifacts, got %d", taskRes.Code)
	}
	var payload map[string]any
	if err := json.Unmarshal(taskRes.Body.Bytes(), &payload); err != nil {
		t.Fatalf("decode payload failed: %v", err)
	}
	if len(payload["artifacts"].([]any)) != 1 {
		t.Fatalf("expected 1 artifact, got %d", len(payload["artifacts"].([]any)))
	}

	activityReq := httptest.NewRequest(http.MethodGet, "/api/v2/tasks/run-1/activity", nil)
	activityRes := httptest.NewRecorder()
	server.HandleListTaskActivity("/api/v2/tasks/").ServeHTTP(activityRes, activityReq)
	if activityRes.Code != http.StatusOK {
		t.Fatalf("expected 200 task activity, got %d", activityRes.Code)
	}
}
