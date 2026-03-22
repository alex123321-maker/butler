package api

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	approvals "github.com/butler/butler/apps/orchestrator/internal/approval"
	"github.com/butler/butler/apps/orchestrator/internal/observability"
	"github.com/butler/butler/apps/orchestrator/internal/run"
)

func TestStreamServer_InitialSnapshotAndFallback(t *testing.T) {
	t.Parallel()

	hub := observability.NewHub()
	taskServer := NewTaskViewServer(&fakeTaskReader{}, nil, nil, nil, nil, nil, nil)
	overviewServer := NewOverviewServer(&stagedTaskReader{byStatus: map[string][]run.TaskRow{}, errByKey: map[string]error{}}, nil)
	approvalsServer := NewApprovalsServer(&fakeApprovalsStore{itemByID: map[string]approvals.Record{}}, &fakeApprovalsResolver{}, nil)
	systemServer := NewSystemServer(nil, &fakeSystemTaskReader{byStatus: map[string][]run.TaskRow{}}, &fakeSystemApprovalsRepo{}, "openai", true, false, "dual", true, 90)
	activityServer := NewActivityServer(&fakeActivityStore{})

	server := NewStreamServer(hub, taskServer, overviewServer, approvalsServer, systemServer, activityServer)
	request := httptest.NewRequest(http.MethodGet, "/api/v2/stream?topics=overview,tasks,approvals,system,activity&type=overview.updated", nil)
	recorder := httptest.NewRecorder()

	done := make(chan struct{})
	go func() {
		server.HandleStream().ServeHTTP(recorder, request)
		close(done)
	}()

	time.Sleep(120 * time.Millisecond)
	_ = request.Context().Err()

	body := recorder.Body.String()
	if !strings.Contains(body, "event: overview.updated") {
		t.Fatalf("expected overview.updated snapshot event, got body: %s", body)
	}
	if !strings.Contains(body, "manual_refresh_or_polling") {
		t.Fatalf("expected fallback hint in stream body, got: %s", body)
	}
}
