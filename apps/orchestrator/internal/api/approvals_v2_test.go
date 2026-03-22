package api

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	approvals "github.com/butler/butler/apps/orchestrator/internal/approval"
	singletab "github.com/butler/butler/apps/orchestrator/internal/singletab"
)

type fakeApprovalsStore struct {
	items      []approvals.Record
	itemByID   map[string]approvals.Record
	candidates map[string][]approvals.TabCandidate
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

func (f *fakeApprovalsStore) ListTabCandidates(_ context.Context, approvalID string) ([]approvals.TabCandidate, error) {
	if f.err != nil {
		return nil, f.err
	}
	return append([]approvals.TabCandidate(nil), f.candidates[approvalID]...), nil
}

func (f *fakeApprovalsStore) SelectTabCandidate(_ context.Context, approvalID, candidateToken string, selectedAt time.Time) (approvals.TabCandidate, error) {
	if f.err != nil {
		return approvals.TabCandidate{}, f.err
	}
	for index := range f.candidates[approvalID] {
		if f.candidates[approvalID][index].CandidateToken == candidateToken {
			f.candidates[approvalID][index].Status = "selected"
			f.candidates[approvalID][index].SelectedAt = &selectedAt
			return f.candidates[approvalID][index], nil
		}
	}
	return approvals.TabCandidate{}, approvals.ErrTabCandidateNotFound
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

type fakeApprovalsSelector struct {
	result singletab.ActivationResult
	err    error
}

func (f *fakeApprovalsSelector) ActivateFromApproval(_ context.Context, params singletab.ActivateFromApprovalParams) (singletab.ActivationResult, error) {
	if f.err != nil {
		return singletab.ActivationResult{}, f.err
	}
	f.result.Candidate.CandidateToken = params.CandidateToken
	return f.result, nil
}

func TestApprovalsListAndGet(t *testing.T) {
	t.Parallel()

	now := time.Date(2026, 3, 19, 15, 0, 0, 0, time.UTC)
	store := &fakeApprovalsStore{
		items: []approvals.Record{{
			ApprovalID:   "a1",
			RunID:        "run-1",
			SessionKey:   "telegram:chat:1",
			ToolCallID:   "tool-1",
			ApprovalType: approvals.ApprovalTypeBrowserTabSelection,
			Status:       approvals.StatusPending,
			RequestedVia: approvals.RequestedViaTelegram,
			ToolName:     "single_tab.bind",
			PayloadJSON:  `{"selection_mode":"single"}`,
			RequestedAt:  now,
			UpdatedAt:    now,
		}},
		itemByID: map[string]approvals.Record{"a1": {
			ApprovalID:   "a1",
			RunID:        "run-1",
			SessionKey:   "telegram:chat:1",
			ToolCallID:   "tool-1",
			ApprovalType: approvals.ApprovalTypeBrowserTabSelection,
			Status:       approvals.StatusPending,
			RequestedVia: approvals.RequestedViaTelegram,
			ToolName:     "single_tab.bind",
			PayloadJSON:  `{"selection_mode":"single"}`,
			RequestedAt:  now,
			UpdatedAt:    now,
		}},
		candidates: map[string][]approvals.TabCandidate{
			"a1": {{
				ApprovalID:     "a1",
				CandidateToken: "tab-1",
				Title:          "Inbox",
				Domain:         "mail.example.com",
				CurrentURL:     "https://mail.example.com/inbox",
				DisplayLabel:   "Inbox - mail.example.com",
				Status:         "available",
				CreatedAt:      now,
			}},
		},
	}
	server := NewApprovalsServer(store, &fakeApprovalsResolver{}, nil)

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
	var getPayload map[string]any
	if err := json.Unmarshal(getRes.Body.Bytes(), &getPayload); err != nil {
		t.Fatalf("decode get payload: %v", err)
	}
	approvalBody := getPayload["approval"].(map[string]any)
	if approvalBody["approval_type"] != approvals.ApprovalTypeBrowserTabSelection {
		t.Fatalf("expected approval_type %q, got %v", approvals.ApprovalTypeBrowserTabSelection, approvalBody["approval_type"])
	}
	candidates := approvalBody["tab_candidates"].([]any)
	if len(candidates) != 1 {
		t.Fatalf("expected 1 tab candidate in response, got %d", len(candidates))
	}
}

func TestApprovalsApproveRejectConflictHandling(t *testing.T) {
	t.Parallel()

	now := time.Date(2026, 3, 19, 16, 0, 0, 0, time.UTC)
	base := approvals.Record{ApprovalID: "a2", RunID: "run-2", SessionKey: "telegram:chat:2", ToolCallID: "tool-2", ApprovalType: approvals.ApprovalTypeToolCall, Status: approvals.StatusPending, RequestedVia: approvals.RequestedViaTelegram, ToolName: "http.request", RequestedAt: now, UpdatedAt: now}
	store := &fakeApprovalsStore{itemByID: map[string]approvals.Record{"a2": base}, candidates: map[string][]approvals.TabCandidate{}}
	resolver := &fakeApprovalsResolver{record: base, changed: true}
	server := NewApprovalsServer(store, resolver, nil)

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

func TestApprovalsSelectTabCreatesSingleTabSession(t *testing.T) {
	t.Parallel()

	now := time.Date(2026, 3, 22, 16, 0, 0, 0, time.UTC)
	base := approvals.Record{
		ApprovalID:   "a3",
		RunID:        "run-3",
		SessionKey:   "web:session:3",
		ToolCallID:   "tool-bind-3",
		ApprovalType: approvals.ApprovalTypeBrowserTabSelection,
		Status:       approvals.StatusPending,
		RequestedVia: approvals.RequestedViaBoth,
		ToolName:     "single_tab.bind",
		RequestedAt:  now,
		UpdatedAt:    now,
	}
	store := &fakeApprovalsStore{
		itemByID: map[string]approvals.Record{"a3": base},
		candidates: map[string][]approvals.TabCandidate{
			"a3": {{
				ApprovalID:     "a3",
				CandidateToken: "tabtok-3",
				Title:          "Inbox",
				Domain:         "mail.example.com",
				CurrentURL:     "https://mail.example.com/inbox",
				DisplayLabel:   "Inbox - mail.example.com",
				Status:         "selected",
				CreatedAt:      now,
			}},
		},
	}
	selector := &fakeApprovalsSelector{
		result: singletab.ActivationResult{
			Approval: base,
			Session: singletab.Record{
				SingleTabSessionID: "single-tab-1",
				SessionKey:         "web:session:3",
				ApprovalID:         "a3",
				Status:             singletab.StatusActive,
				BoundTabRef:        "browser-a:3",
				CurrentURL:         "https://mail.example.com/inbox",
				CurrentTitle:       "Inbox",
				SelectedVia:        "web",
				SelectedBy:         "alice",
				CreatedAt:          now,
				UpdatedAt:          now,
			},
			Changed: true,
		},
	}
	server := NewApprovalsServer(store, &fakeApprovalsResolver{}, selector)

	body := bytes.NewBufferString(`{"candidate_token":"tabtok-3","current_url":"https://mail.example.com/inbox"}`)
	req := httptest.NewRequest(http.MethodPost, "/api/v2/approvals/a3/select-tab", body)
	req.Header.Set("X-Butler-Actor", "alice")
	res := httptest.NewRecorder()
	server.HandleSelectTab("/api/v2/approvals/").ServeHTTP(res, req)
	if res.Code != http.StatusOK {
		t.Fatalf("expected 200 select-tab, got %d", res.Code)
	}

	var payload map[string]any
	if err := json.Unmarshal(res.Body.Bytes(), &payload); err != nil {
		t.Fatalf("decode select-tab payload: %v", err)
	}
	sessionBody := payload["single_tab_session"].(map[string]any)
	if sessionBody["single_tab_session_id"] != "single-tab-1" {
		t.Fatalf("expected single_tab_session_id single-tab-1, got %v", sessionBody["single_tab_session_id"])
	}
	if payload["changed"] != true {
		t.Fatalf("expected changed=true, got %v", payload["changed"])
	}
}
