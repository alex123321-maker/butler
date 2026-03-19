package api

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	activity "github.com/butler/butler/apps/orchestrator/internal/activity"
)

type fakeActivityStore struct {
	items []activity.Record
	err   error
}

func (f *fakeActivityStore) ListActivities(_ context.Context, params activity.ListParams) ([]activity.Record, error) {
	if f.err != nil {
		return nil, f.err
	}
	return f.items, nil
}

func TestActivityServerList(t *testing.T) {
	t.Parallel()

	now := time.Date(2026, 3, 19, 17, 0, 0, 0, time.UTC)
	server := NewActivityServer(&fakeActivityStore{items: []activity.Record{{ActivityID: 1, RunID: "run-1", SessionKey: "telegram:chat:1", ActivityType: activity.TypeTaskReceived, Title: "Task received", Summary: "Task context prepared", DetailsJSON: `{"source":"telegram"}`, ActorType: "system", Severity: activity.SeverityInfo, CreatedAt: now}}})

	req := httptest.NewRequest(http.MethodGet, "/api/v2/activity?severity=info&run_id=run-1", nil)
	res := httptest.NewRecorder()
	server.HandleListActivity().ServeHTTP(res, req)
	if res.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", res.Code)
	}
	var payload map[string]any
	if err := json.Unmarshal(res.Body.Bytes(), &payload); err != nil {
		t.Fatalf("decode payload failed: %v", err)
	}
	if len(payload["activity"].([]any)) != 1 {
		t.Fatalf("expected one activity item, got %d", len(payload["activity"].([]any)))
	}
}
