package task_test

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/butler/butler/apps/orchestrator/internal/api"
)

func TestHTTPControlRequiresProofAndCurrentRevisionIntegration(t *testing.T) {
	f := newControlsFixture(t)
	server := api.NewTaskControlServer(f.tasks)
	server.SetControls(f.controls)
	token := "fixture-owner-token-at-least-32-bytes"
	server.SetOwnerControlAuth(api.NewOwnerControlAuth(token, f.owner))
	handler := server.HandleItem("/api/v3/tasks/")
	body := `{"kind":"amend","expected_revision":1,"instruction":"prepare only","execution_policy":"prepare_only"}`
	send := func(bearer, delivery string) *httptest.ResponseRecorder {
		request := httptest.NewRequest(http.MethodPost, "/api/v3/tasks/"+f.root.TaskID+"/control", strings.NewReader(body))
		request.Header.Set("X-Butler-Actor", f.owner)
		request.Header.Set("X-Butler-Delivery-ID", delivery)
		if bearer != "" {
			request.Header.Set("Authorization", "Bearer "+bearer)
		}
		response := httptest.NewRecorder()
		handler.ServeHTTP(response, request)
		return response
	}
	if response := send("", "header-spoof"); response.Code != 401 {
		t.Fatal("actor header supplied authority", response.Code)
	}
	if response := send("wrong-owner-token", "wrong"); response.Code != 401 {
		t.Fatal("wrong bearer accepted", response.Code)
	}
	if response := send(token, "first"); response.Code != 200 {
		t.Fatal(response.Body.String())
	}
	if response := send(token, "first"); response.Code != 200 {
		t.Fatal("redelivery rejected", response.Body.String())
	}
	if response := send(token, "stale"); response.Code != 409 {
		t.Fatal("stale UI version accepted", response.Body.String())
	}
}
