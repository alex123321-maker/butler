package api

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/butler/butler/internal/owneridentity"
	"github.com/butler/butler/internal/workspaceexec"
)

type ownerWorkspaceFixture struct {
	calls   int
	actor   owneridentity.Actor
	request workspaceexec.DiscardRequest
	err     error
}

func (f *ownerWorkspaceFixture) Get(ctx context.Context, id string) (workspaceexec.RecoveryStatus, error) {
	f.calls++
	f.actor, _ = owneridentity.FromContext(ctx)
	return workspaceexec.RecoveryStatus{WorkspaceID: id, TaskID: "task-owner", ResourceVersion: 7, WorkspaceGeneration: 0, TaskRevision: 2, ConstraintsRevision: 2, Phase: "quarantined", CanDiscard: true, OriginalOutcome: "unknown"}, f.err
}
func (f *ownerWorkspaceFixture) ListForTask(ctx context.Context, id string) ([]workspaceexec.RecoveryStatus, error) {
	status, err := f.Get(ctx, "workspace-owner")
	status.TaskID = id
	return []workspaceexec.RecoveryStatus{status}, err
}
func (f *ownerWorkspaceFixture) RequestDiscard(ctx context.Context, req workspaceexec.DiscardRequest) (workspaceexec.RecoveryStatus, error) {
	status, err := f.Get(ctx, req.WorkspaceID)
	f.request = req
	status.Phase = "requested"
	status.CanDiscard = false
	return status, err
}

func TestOwnerWorkspaceRecoveryHTTPRejectsCallerAuthorityAndPreservesUnknown(t *testing.T) {
	fixture := &ownerWorkspaceFixture{}
	server := NewWorkspaceRecoveryServer(fixture, NewOwnerControlAuth("fixture-owner-token-000000000000000", "owner"))
	handler := server.Handle("/api/workspaces/")
	body := `{"workspace_id":"workspace-owner","resource_version":7,"workspace_generation":0,"task_revision":2,"constraints_revision":2}`
	send := func(raw, token, delivery string) *httptest.ResponseRecorder {
		req := httptest.NewRequest(http.MethodPost, "/api/workspaces/workspace-owner/discard", strings.NewReader(raw))
		if token != "" {
			req.Header.Set("Authorization", "Bearer "+token)
		}
		req.Header.Set("X-Butler-Delivery-ID", delivery)
		res := httptest.NewRecorder()
		handler.ServeHTTP(res, req)
		return res
	}
	if res := send(body, "", "one"); res.Code != 401 || fixture.calls != 0 {
		t.Fatal("unauthenticated discard reached controller")
	}
	for _, raw := range []string{strings.Replace(body, `"resource_version":7`, `"resource_version":7,"resource_version":9`, 1), strings.Replace(body, "constraints_revision", "con\u017ftraints_revision", 1), strings.Replace(body, `"workspace_generation":0`, `"workspace_generation":null`, 1), body + ` {}`, strings.Replace(body, "workspace-owner", "other-workspace", 1), strings.TrimSuffix(body, "}") + `,"root_path":"/private/root"}`, strings.TrimSuffix(body, "}") + `,"owner_id":"owner"}`, strings.Replace(body, `"task_revision":2,`, "", 1)} {
		if res := send(raw, "fixture-owner-token-000000000000000", "one"); res.Code != 400 || fixture.calls != 0 {
			t.Fatalf("ambiguous discard body accepted: %s", raw)
		}
	}
	if res := send(body, "fixture-owner-token-000000000000000", ""); res.Code != 400 || fixture.calls != 0 {
		t.Fatal("discard allocated a replacement delivery")
	}
	response := send(body, "fixture-owner-token-000000000000000", "stable-owner-intent")
	if response.Code != 200 || fixture.actor.OwnerID != "owner" || fixture.actor.DeliveryID != "stable-owner-intent" || fixture.request.WorkspaceGeneration != 0 {
		t.Fatal("trusted actor or explicit zero generation lost")
	}
	var status workspaceexec.RecoveryStatus
	if json.Unmarshal(response.Body.Bytes(), &status) != nil || status.OriginalOutcome != "unknown" || status.Phase != "requested" || status.CanDiscard {
		t.Fatal("discard request became proof of effect or erasure")
	}
	fixture.err = workspaceexec.ErrRecoveryConflict
	if res := send(body, "fixture-owner-token-000000000000000", "new-intent"); res.Code != 409 || strings.Contains(res.Body.String(), fixture.err.Error()) {
		t.Fatal("private backend error escaped conflict")
	}
}

func TestOwnerWorkspaceRecoveryDiscoveryRequiresOwnerAndUsesBoundedTaskProjection(t *testing.T) {
	fixture := &ownerWorkspaceFixture{}
	server := NewWorkspaceRecoveryServer(fixture, NewOwnerControlAuth("fixture-owner-token-000000000000000", "owner"))
	handler := server.HandleTaskList("task-owner")
	req := httptest.NewRequest(http.MethodGet, "/api/v3/tasks/task-owner/workspace-recovery", nil)
	denied := httptest.NewRecorder()
	handler.ServeHTTP(denied, req)
	if denied.Code != 401 || fixture.calls != 0 {
		t.Fatal("opaque task ID inferred ownership")
	}
	req.Header.Set("Authorization", "Bearer fixture-owner-token-000000000000000")
	allowed := httptest.NewRecorder()
	handler.ServeHTTP(allowed, req)
	if allowed.Code != 200 || fixture.actor.OwnerID != "owner" || !strings.Contains(allowed.Body.String(), `"limit":50`) || !strings.Contains(allowed.Body.String(), `"workspace_id":"workspace-owner"`) || !strings.Contains(allowed.Body.String(), `"original_outcome":"unknown"`) {
		t.Fatal("safe unknown Create discovery lost")
	}
	if allowed.Header().Get("Cache-Control") != "no-store" {
		t.Fatal("private recovery snapshot cached")
	}
}
