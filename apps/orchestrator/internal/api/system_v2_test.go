package api

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	approvals "github.com/butler/butler/apps/orchestrator/internal/approval"
	"github.com/butler/butler/apps/orchestrator/internal/run"
)

type fakeSystemTaskReader struct {
	byStatus map[string][]run.TaskRow
	err      error
}

func (f *fakeSystemTaskReader) ListTasks(_ context.Context, params run.TaskListParams) ([]run.TaskRow, error) {
	if f.err != nil {
		return nil, f.err
	}
	return f.byStatus[params.Status], nil
}

type fakeSystemApprovalsRepo struct {
	items []approvals.Record
	err   error
}

func (f *fakeSystemApprovalsRepo) CreateApproval(ctx context.Context, params approvals.CreateParams) (approvals.Record, error) {
	return approvals.Record{}, nil
}
func (f *fakeSystemApprovalsRepo) GetApprovalByToolCallID(ctx context.Context, toolCallID string) (approvals.Record, error) {
	return approvals.Record{}, approvals.ErrApprovalNotFound
}
func (f *fakeSystemApprovalsRepo) GetApprovalByID(ctx context.Context, approvalID string) (approvals.Record, error) {
	return approvals.Record{}, approvals.ErrApprovalNotFound
}
func (f *fakeSystemApprovalsRepo) ListApprovals(ctx context.Context, status, runID, sessionKey string, limit, offset int) ([]approvals.Record, error) {
	if f.err != nil {
		return nil, f.err
	}
	return f.items, nil
}
func (f *fakeSystemApprovalsRepo) ResolveApproval(ctx context.Context, params approvals.ResolveParams) (approvals.Record, error) {
	return approvals.Record{}, nil
}
func (f *fakeSystemApprovalsRepo) InsertEvent(ctx context.Context, event approvals.Event) error {
	return nil
}

func TestSystemServer_DegradedAndHealthyStates(t *testing.T) {
	t.Parallel()

	now := time.Now().UTC()
	server := NewSystemServer(
		nil,
		&fakeSystemTaskReader{byStatus: map[string][]run.TaskRow{"failed": {{RunID: "run-1", ErrorSummary: "error", UpdatedAt: now}}}},
		&fakeSystemApprovalsRepo{items: []approvals.Record{{ApprovalID: "a1", Status: approvals.StatusPending}}},
		"openai",
		true,
		true,
	)

	request := httptest.NewRequest(http.MethodGet, "/api/v2/system", nil)
	recorder := httptest.NewRecorder()
	server.HandleGetSystem().ServeHTTP(recorder, request)
	if recorder.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", recorder.Code)
	}
	var payload map[string]any
	if err := json.Unmarshal(recorder.Body.Bytes(), &payload); err != nil {
		t.Fatalf("decode response failed: %v", err)
	}
	health := payload["health"].(map[string]any)
	if health["status"] != "degraded" {
		t.Fatalf("expected degraded health, got %v", health["status"])
	}
	if payload["pending_approvals"].(float64) != 1 {
		t.Fatalf("expected pending approvals=1, got %v", payload["pending_approvals"])
	}
}
