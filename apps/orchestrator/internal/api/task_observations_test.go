package api

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/butler/butler/apps/orchestrator/internal/task"
	"github.com/butler/butler/internal/owneridentity"
)

type overviewFixture struct {
	taskController
	called bool
}

func (f *overviewFixture) OwnerOverview(ctx context.Context, id string) (task.OwnerOverview, error) {
	f.called = true
	a, ok := owneridentity.FromContext(ctx)
	if !ok || a.OwnerID != "owner" {
		return task.OwnerOverview{}, task.ErrUnauthorized
	}
	return task.OwnerOverview{TaskID: id, Revision: 2}, nil
}

func TestOwnerTaskObservationsAuthenticateBeforeRead(t *testing.T) {
	fixture := &overviewFixture{}
	s := NewTaskControlServer(fixture)
	s.SetOwnerControlAuth(NewOwnerControlAuth("fixture-owner-control-token-000000000", "owner"))
	for _, bearer := range []string{"", "forged"} {
		r := httptest.NewRequest(http.MethodGet, "/api/v3/tasks/task/observations?owner_id=owner", nil)
		r.Header.Set("Authorization", "Bearer "+bearer)
		r.Header.Set("X-Butler-Owner-ID", "owner")
		w := httptest.NewRecorder()
		s.HandleItem("/api/v3/tasks/").ServeHTTP(w, r)
		if w.Code != 401 || fixture.called {
			t.Fatal("unverified actor reached task projection")
		}
	}
	r := httptest.NewRequest(http.MethodGet, "/api/v3/tasks/task/observations?owner_id=foreign", nil)
	r.Header.Set("Authorization", "Bearer fixture-owner-control-token-000000000")
	w := httptest.NewRecorder()
	s.HandleItem("/api/v3/tasks/").ServeHTTP(w, r)
	if w.Code != 200 || !fixture.called {
		t.Fatal("authenticated owner projection unavailable")
	}
}

func TestTaskRiskDoesNotFollowRunFailureOrActivity(t *testing.T) {
	for _, state := range []string{"failed", "timed_out", "tool_running", "awaiting_approval", "completed", ""} {
		if deriveRiskLevel(state) != "unknown" {
			t.Fatal("technical state was used as action risk")
		}
	}
}
