package api

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	approvals "github.com/butler/butler/apps/orchestrator/internal/approval"
)

type fakeApprovalsStore struct {
	items      []approvals.Record
	itemByID   map[string]approvals.Record
	lastStatus string
	lastRunID  string
	err        error
}

func (f *fakeApprovalsStore) ListApprovals(_ context.Context, status, runID, sessionKey string, limit, offset int) ([]approvals.Record, error) {
	f.lastStatus = status
	f.lastRunID = runID
	if f.err != nil {
		return nil, f.err
	}
	return f.items, nil
}

func (f *fakeApprovalsStore) GetApprovalByID(_ context.Context, approvalID string) (approvals.Record, error) {
	if f.err != nil {
		return approvals.Record{}, f.err
	}
	item, ok := f.itemByID[approvalID]
	if !ok {
		return approvals.Record{}, approvals.ErrApprovalNotFound
	}
	return item, nil
}

type fakeApprovalsResolver struct {
	record  approvals.Record
	changed bool
	err     error
}

func (f *fakeApprovalsResolver) ResolveByToolCall(_ context.Context, params approvals.ResolveByToolCallParams) (approvals.Record, bool, error) {
	if f.err != nil {
		return approvals.Record{}, false, f.err
	}
	f.record.ToolCallID = params.ToolCallID
	if params.Approved {
		f.record.Status = approvals.StatusApproved
	} else {
		f.record.Status = approvals.StatusRejected
	}
	return f.record, f.changed, nil
}

func TestApprovalsListAndGet(t *testing.T) {
	t.Parallel()

	now := time.Date(2026, 3, 19, 15, 0, 0, 0, time.UTC)
	store := &fakeApprovalsStore{
		items:    []approvals.Record{{ApprovalID: "a1", RunID: "run-1", SessionKey: "telegram:chat:1", ToolCallID: "tool-1", Status: approvals.StatusPending, RequestedVia: approvals.RequestedViaTelegram, ToolName: "http.request", RequestedAt: now, UpdatedAt: now}},
		itemByID: map[string]approvals.Record{"a1": {ApprovalID: "a1", RunID: "run-1", SessionKey: "telegram:chat:1", ToolCallID: "tool-1", Status: approvals.StatusPending, RequestedVia: approvals.RequestedViaTelegram, ToolName: "http.request", RequestedAt: now, UpdatedAt: now}},
	}
	server := NewApprovalsServer(store, &fakeApprovalsResolver{})

	listReq := httptest.NewRequest(http.MethodGet, "/api/v2/approvals?status=pending&run_id=run-1", nil)
	listRes := httptest.NewRecorder()
	server.HandleListApprovals().ServeHTTP(listRes, listReq)
	if listRes.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", listRes.Code)
	}
	if store.lastStatus != "pending" || store.lastRunID != "run-1" {
		t.Fatalf("expected status/run filters propagated, got %q/%q", store.lastStatus, store.lastRunID)
	}

	getReq := httptest.NewRequest(http.MethodGet, "/api/v2/approvals/a1", nil)
	getRes := httptest.NewRecorder()
	server.HandleGetApproval("/api/v2/approvals/").ServeHTTP(getRes, getReq)
	if getRes.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", getRes.Code)
	}
}

func TestApprovalsApproveRejectConflictHandling(t *testing.T) {
	t.Parallel()

	now := time.Date(2026, 3, 19, 16, 0, 0, 0, time.UTC)
	base := approvals.Record{ApprovalID: "a2", RunID: "run-2", SessionKey: "telegram:chat:2", ToolCallID: "tool-2", Status: approvals.StatusPending, RequestedVia: approvals.RequestedViaTelegram, ToolName: "http.request", RequestedAt: now, UpdatedAt: now}
	store := &fakeApprovalsStore{itemByID: map[string]approvals.Record{"a2": base}}
	resolver := &fakeApprovalsResolver{record: base, changed: true}
	server := NewApprovalsServer(store, resolver)

	approveReq := httptest.NewRequest(http.MethodPost, "/api/v2/approvals/a2/approve", nil)
	approveRes := httptest.NewRecorder()
	server.HandleApprove("/api/v2/approvals/").ServeHTTP(approveRes, approveReq)
	if approveRes.Code != http.StatusOK {
		t.Fatalf("expected 200 approve, got %d", approveRes.Code)
	}

	resolver.changed = false
	rejectReq := httptest.NewRequest(http.MethodPost, "/api/v2/approvals/a2/reject", nil)
	rejectRes := httptest.NewRecorder()
	server.HandleReject("/api/v2/approvals/").ServeHTTP(rejectRes, rejectReq)
	if rejectRes.Code != http.StatusConflict {
		t.Fatalf("expected 409 for idempotent/conflict reject, got %d", rejectRes.Code)
	}

	var payload map[string]any
	if err := json.Unmarshal(rejectRes.Body.Bytes(), &payload); err != nil {
		t.Fatalf("decode conflict payload: %v", err)
	}
	if payload["changed"] != false {
		t.Fatalf("expected changed=false in conflict payload, got %v", payload["changed"])
	}
}
