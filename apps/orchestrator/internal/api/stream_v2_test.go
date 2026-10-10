package api

import (
	"bufio"
	"context"
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
	httpServer := httptest.NewServer(server.HandleStream())
	defer httpServer.Close()
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, httpServer.URL+"/api/v2/stream?topics=overview,tasks,approvals,system,activity&type=overview.updated", nil)
	if err != nil {
		t.Fatal(err)
	}
	response, err := httpServer.Client().Do(request)
	if err != nil {
		t.Fatal(err)
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		t.Fatalf("unexpected stream status: %d", response.StatusCode)
	}
	var received strings.Builder
	scanner := bufio.NewScanner(response.Body)
	for scanner.Scan() {
		line := scanner.Text()
		received.WriteString(line + "\n")
		if strings.Contains(line, "manual_refresh_or_polling") {
			break
		}
	}
	if err := scanner.Err(); err != nil {
		t.Fatalf("read initial stream snapshots: %v", err)
	}
	cancel()
	body := received.String()
	if !strings.Contains(body, "event: overview.updated") {
		t.Fatalf("expected overview.updated snapshot event, got body: %s", body)
	}
	if !strings.Contains(body, "manual_refresh_or_polling") {
		t.Fatalf("expected fallback hint in stream body, got: %s", body)
	}
}
