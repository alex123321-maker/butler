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

type fakeSingleTabCoordinator struct {
	createResult singletab.CreateBindRequestResult
	createErr    error
	session      singletab.Record
	sessionErr   error
	lastUpdate   *singletab.UpdateSessionStateParams
	lastCreate   *singletab.CreateBindRequestParams
}

func (f *fakeSingleTabCoordinator) CreateBindRequest(_ context.Context, params singletab.CreateBindRequestParams) (singletab.CreateBindRequestResult, error) {
	copyParams := params
	f.lastCreate = &copyParams
	if f.createErr != nil {
		return singletab.CreateBindRequestResult{}, f.createErr
	}
	if f.createResult.Approval.RunID == "" {
		f.createResult.Approval.RunID = params.RunID
		f.createResult.Approval.SessionKey = params.SessionKey
	}
	return f.createResult, nil
}

func (f *fakeSingleTabCoordinator) GetActiveSession(_ context.Context, sessionKey string) (singletab.Record, error) {
	if f.sessionErr != nil {
		return singletab.Record{}, f.sessionErr
	}
	record := f.session
	record.SessionKey = sessionKey
	return record, nil
}

func (f *fakeSingleTabCoordinator) GetSession(_ context.Context, sessionID string) (singletab.Record, error) {
	if f.sessionErr != nil {
		return singletab.Record{}, f.sessionErr
	}
	record := f.session
	record.SingleTabSessionID = sessionID
	return record, nil
}

func (f *fakeSingleTabCoordinator) ReleaseSession(_ context.Context, params singletab.ReleaseSessionParams) (singletab.Record, bool, error) {
	if f.sessionErr != nil {
		return singletab.Record{}, false, f.sessionErr
	}
	record := f.session
	record.SingleTabSessionID = params.SingleTabSessionID
	record.Status = singletab.StatusRevokedByUser
	releasedAt := params.ReleasedAt
	if releasedAt.IsZero() {
		now := time.Date(2026, 3, 22, 19, 0, 0, 0, time.UTC)
		releasedAt = now
	}
	record.ReleasedAt = &releasedAt
	return record, true, nil
}

func (f *fakeSingleTabCoordinator) UpdateSessionState(_ context.Context, params singletab.UpdateSessionStateParams) (singletab.Record, error) {
	if f.sessionErr != nil {
		return singletab.Record{}, f.sessionErr
	}
	copyParams := params
	f.lastUpdate = &copyParams
	record := f.session
	record.SingleTabSessionID = params.SingleTabSessionID
	if params.Status != "" {
		record.Status = params.Status
	}
	if params.CurrentURL != "" {
		record.CurrentURL = params.CurrentURL
	}
	if params.CurrentTitle != "" {
		record.CurrentTitle = params.CurrentTitle
	}
	if params.BrowserInstanceID != "" {
		record.BrowserInstanceID = params.BrowserInstanceID
	}
	if !params.LastSeenAt.IsZero() {
		record.LastSeenAt = &params.LastSeenAt
	}
	f.session = record
	return record, nil
}

func TestSingleTabCreateBindRequest(t *testing.T) {
	t.Parallel()

	now := time.Date(2026, 3, 22, 18, 0, 0, 0, time.UTC)
	server := NewSingleTabServer(&fakeSingleTabCoordinator{
		createResult: singletab.CreateBindRequestResult{
			Approval: approvals.Record{
				ApprovalID:   "approval-bind-5",
				RunID:        "run-5",
				SessionKey:   "telegram:chat:5",
				ToolCallID:   "single-tab-bind-5",
				ApprovalType: approvals.ApprovalTypeBrowserTabSelection,
				Status:       approvals.StatusPending,
				RequestedVia: approvals.RequestedViaBoth,
				ToolName:     "single_tab.bind",
				RequestedAt:  now,
				UpdatedAt:    now,
			},
			Candidates: []approvals.TabCandidate{{
				ApprovalID:     "approval-bind-5",
				CandidateToken: "tabtok-5",
				Title:          "Docs",
				Domain:         "docs.example.com",
				CurrentURL:     "https://docs.example.com",
				DisplayLabel:   "Docs - docs.example.com",
				Status:         "available",
				CreatedAt:      now,
			}},
		},
	})

	body := bytes.NewBufferString(`{"run_id":"run-5","session_key":"telegram:chat:5","request_source":"browser-bridge","tab_candidates":[{"internal_tab_ref":"browser-a:5","title":"Docs","domain":"docs.example.com","current_url":"https://docs.example.com"}]}`)
	req := httptest.NewRequest(http.MethodPost, "/api/v2/single-tab/bind-requests", body)
	res := httptest.NewRecorder()
	server.HandleCreateBindRequest().ServeHTTP(res, req)
	if res.Code != http.StatusCreated {
		t.Fatalf("expected 201, got %d", res.Code)
	}

	var payload map[string]any
	if err := json.Unmarshal(res.Body.Bytes(), &payload); err != nil {
		t.Fatalf("decode payload: %v", err)
	}
	approval := payload["approval"].(map[string]any)
	if approval["approval_type"] != approvals.ApprovalTypeBrowserTabSelection {
		t.Fatalf("expected browser tab selection approval type, got %v", approval["approval_type"])
	}
}

func TestSingleTabCreateBindRequestViaExtensionDiscoveryRelay(t *testing.T) {
	t.Parallel()

	now := time.Date(2026, 3, 22, 18, 5, 0, 0, time.UTC)
	coordinator := &fakeSingleTabCoordinator{
		createResult: singletab.CreateBindRequestResult{
			Approval: approvals.Record{
				ApprovalID:   "approval-bind-relay-5",
				RunID:        "run-5",
				SessionKey:   "telegram:chat:5",
				ToolCallID:   "single-tab-bind-5",
				ApprovalType: approvals.ApprovalTypeBrowserTabSelection,
				Status:       approvals.StatusPending,
				RequestedVia: approvals.RequestedViaBoth,
				ToolName:     "single_tab.bind",
				RequestedAt:  now,
				UpdatedAt:    now,
			},
			Candidates: []approvals.TabCandidate{{
				ApprovalID:     "approval-bind-relay-5",
				CandidateToken: "tabtok-relay-5",
				Title:          "Docs",
				Domain:         "docs.example.com",
				CurrentURL:     "https://docs.example.com",
				DisplayLabel:   "Docs - docs.example.com",
				Status:         "available",
				CreatedAt:      now,
			}},
		},
	}
	server := NewSingleTabServer(coordinator)

	createDone := make(chan *httptest.ResponseRecorder, 1)
	go func() {
		body := bytes.NewBufferString(`{"run_id":"run-5","session_key":"telegram:chat:5","tool_call_id":"single-tab-bind-5","discover_tabs_via_extension":true}`)
		req := httptest.NewRequest(http.MethodPost, "/api/v2/single-tab/bind-requests", body)
		res := httptest.NewRecorder()
		server.HandleCreateBindRequest().ServeHTTP(res, req)
		createDone <- res
	}()

	pollReq := httptest.NewRequest(http.MethodGet, "/api/v2/extension/single-tab/bind-requests/next?browser_instance_id=browser-bind-1&timeout_ms=5000", nil)
	pollRes := httptest.NewRecorder()
	server.HandleExtensionPollNextBindRequest().ServeHTTP(pollRes, pollReq)
	if pollRes.Code != http.StatusOK {
		t.Fatalf("expected 200 poll bind request, got %d", pollRes.Code)
	}

	var pollPayload map[string]any
	if err := json.Unmarshal(pollRes.Body.Bytes(), &pollPayload); err != nil {
		t.Fatalf("decode poll bind payload: %v", err)
	}
	dispatch := pollPayload["dispatch"].(map[string]any)
	dispatchID := dispatch["dispatch_id"].(string)

	resolveBody := bytes.NewBufferString(`{"ok":true,"browser_hint":"Chrome","tab_candidates":[{"internal_tab_ref":"52","title":"Docs","domain":"docs.example.com","current_url":"https://docs.example.com","display_label":"Docs - docs.example.com"}]}`)
	resolveReq := httptest.NewRequest(http.MethodPost, "/api/v2/extension/single-tab/bind-requests/"+dispatchID+"/result?browser_instance_id=browser-bind-1", resolveBody)
	resolveRes := httptest.NewRecorder()
	server.HandleExtensionResolveBindRequest("/api/v2/extension/single-tab/bind-requests/").ServeHTTP(resolveRes, resolveReq)
	if resolveRes.Code != http.StatusOK {
		t.Fatalf("expected 200 resolve bind request, got %d", resolveRes.Code)
	}

	select {
	case createRes := <-createDone:
		if createRes.Code != http.StatusCreated {
			t.Fatalf("expected 201 create bind request, got %d", createRes.Code)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for create bind request completion")
	}
	if coordinator.lastCreate == nil {
		t.Fatal("expected create bind request params to be captured")
	}
	if got := len(coordinator.lastCreate.Candidates); got != 1 {
		t.Fatalf("expected one discovered candidate, got %d", got)
	}
	if got := coordinator.lastCreate.Candidates[0].InternalTabRef; got != "52" {
		t.Fatalf("unexpected discovered internal tab ref %q", got)
	}
}

func TestSingleTabGetActiveSession(t *testing.T) {
	t.Parallel()

	now := time.Date(2026, 3, 22, 18, 30, 0, 0, time.UTC)
	server := NewSingleTabServer(&fakeSingleTabCoordinator{
		session: singletab.Record{
			SingleTabSessionID: "single-tab-8",
			ApprovalID:         "approval-bind-8",
			Status:             singletab.StatusActive,
			BoundTabRef:        "browser-a:8",
			CurrentURL:         "https://mail.example.com/inbox",
			CurrentTitle:       "Inbox",
			SelectedVia:        "telegram",
			SelectedBy:         "telegram_user:8",
			CreatedAt:          now,
			UpdatedAt:          now,
		},
	})

	req := httptest.NewRequest(http.MethodGet, "/api/v2/single-tab/session?session_key=telegram:chat:8", nil)
	res := httptest.NewRecorder()
	server.HandleGetActiveSession().ServeHTTP(res, req)
	if res.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", res.Code)
	}
}

func TestSingleTabGetSessionAndRelease(t *testing.T) {
	t.Parallel()

	now := time.Date(2026, 3, 22, 18, 45, 0, 0, time.UTC)
	server := NewSingleTabServer(&fakeSingleTabCoordinator{
		session: singletab.Record{
			SingleTabSessionID: "single-tab-9",
			ApprovalID:         "approval-bind-9",
			Status:             singletab.StatusActive,
			BoundTabRef:        "browser-a:9",
			CurrentURL:         "https://example.com",
			CurrentTitle:       "Example",
			SelectedVia:        "web",
			SelectedBy:         "alice",
			CreatedAt:          now,
			UpdatedAt:          now,
		},
	})

	getReq := httptest.NewRequest(http.MethodGet, "/api/v2/single-tab/session/single-tab-9", nil)
	getRes := httptest.NewRecorder()
	server.HandleGetSession("/api/v2/single-tab/session/").ServeHTTP(getRes, getReq)
	if getRes.Code != http.StatusOK {
		t.Fatalf("expected 200 get session, got %d", getRes.Code)
	}

	releaseReq := httptest.NewRequest(http.MethodPost, "/api/v2/single-tab/session/single-tab-9/release", nil)
	releaseReq.Header.Set("X-Butler-Actor", "alice")
	releaseRes := httptest.NewRecorder()
	server.HandleReleaseSession("/api/v2/single-tab/session/").ServeHTTP(releaseRes, releaseReq)
	if releaseRes.Code != http.StatusOK {
		t.Fatalf("expected 200 release session, got %d", releaseRes.Code)
	}
}

func TestSingleTabUpdateSessionState(t *testing.T) {
	t.Parallel()

	now := time.Date(2026, 3, 22, 18, 55, 0, 0, time.UTC)
	server := NewSingleTabServer(&fakeSingleTabCoordinator{
		session: singletab.Record{
			SingleTabSessionID: "single-tab-11",
			ApprovalID:         "approval-bind-11",
			Status:             singletab.StatusActive,
			BoundTabRef:        "browser-a:11",
			CurrentURL:         "https://old.example.com",
			CurrentTitle:       "Old",
			CreatedAt:          now,
			UpdatedAt:          now,
		},
	})

	body := bytes.NewBufferString(`{"status":"ACTIVE","current_url":"https://new.example.com","current_title":"New"}`)
	req := httptest.NewRequest(http.MethodPost, "/api/v2/single-tab/session/single-tab-11/state", body)
	res := httptest.NewRecorder()
	server.HandleUpdateSessionState("/api/v2/single-tab/session/").ServeHTTP(res, req)
	if res.Code != http.StatusOK {
		t.Fatalf("expected 200 update state, got %d", res.Code)
	}
}

func TestSingleTabRelayDispatchFlow(t *testing.T) {
	t.Parallel()

	now := time.Date(2026, 3, 22, 19, 10, 0, 0, time.UTC)
	server := NewSingleTabServer(&fakeSingleTabCoordinator{
		session: singletab.Record{
			SingleTabSessionID: "single-tab-relay-1",
			SessionKey:         "telegram:chat:relay-1",
			ApprovalID:         "approval-bind-relay-1",
			Status:             singletab.StatusActive,
			BoundTabRef:        "31",
			CurrentURL:         "https://example.com",
			CurrentTitle:       "Example",
			CreatedAt:          now,
			UpdatedAt:          now,
		},
	})

	dispatchBody := bytes.NewBufferString(`{"single_tab_session_id":"single-tab-relay-1","bound_tab_ref":"31","action_type":"status"}`)
	dispatchReq := httptest.NewRequest(http.MethodPost, "/api/v2/single-tab/actions/dispatch", dispatchBody)
	dispatchDone := make(chan *httptest.ResponseRecorder, 1)
	go func() {
		res := httptest.NewRecorder()
		server.HandleRelayDispatchAction().ServeHTTP(res, dispatchReq)
		dispatchDone <- res
	}()

	pollReq := httptest.NewRequest(http.MethodGet, "/api/v2/extension/single-tab/actions/next?session_key=telegram:chat:relay-1&timeout_ms=5000&browser_instance_id=browser-relay-1", nil)
	pollRes := httptest.NewRecorder()
	server.HandleExtensionPollNextAction().ServeHTTP(pollRes, pollReq)
	if pollRes.Code != http.StatusOK {
		t.Fatalf("expected 200 poll next action, got %d", pollRes.Code)
	}

	var pollPayload map[string]any
	if err := json.Unmarshal(pollRes.Body.Bytes(), &pollPayload); err != nil {
		t.Fatalf("decode poll payload: %v", err)
	}
	dispatch := pollPayload["dispatch"].(map[string]any)
	dispatchID := dispatch["dispatch_id"].(string)

	resolveBody := bytes.NewBufferString(`{"ok":true,"result":{"single_tab_session_id":"single-tab-relay-1","session_status":"ACTIVE","result_json":"{\"ok\":true}"}}`)
	resolveReq := httptest.NewRequest(http.MethodPost, "/api/v2/extension/single-tab/actions/"+dispatchID+"/result?browser_instance_id=browser-relay-1", resolveBody)
	resolveRes := httptest.NewRecorder()
	server.HandleExtensionResolveAction("/api/v2/extension/single-tab/actions/").ServeHTTP(resolveRes, resolveReq)
	if resolveRes.Code != http.StatusOK {
		t.Fatalf("expected 200 resolve action, got %d", resolveRes.Code)
	}

	select {
	case dispatchRes := <-dispatchDone:
		if dispatchRes.Code != http.StatusOK {
			t.Fatalf("expected 200 relay dispatch, got %d", dispatchRes.Code)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for relay dispatch completion")
	}
}

func TestSingleTabRelayDispatchMarksHostDisconnectedWhenHeartbeatStale(t *testing.T) {
	t.Parallel()

	staleSeenAt := time.Now().UTC().Add(-10 * time.Minute)
	coordinator := &fakeSingleTabCoordinator{
		session: singletab.Record{
			SingleTabSessionID: "single-tab-relay-stale-1",
			SessionKey:         "telegram:chat:relay-stale-1",
			Status:             singletab.StatusActive,
			BoundTabRef:        "41",
			BrowserInstanceID:  "browser-stale-1",
			CurrentURL:         "https://example.com/stale",
			CurrentTitle:       "Stale",
			LastSeenAt:         &staleSeenAt,
		},
	}
	server := NewSingleTabServer(coordinator)
	server.SetRelayHeartbeatTTL(45 * time.Second)

	dispatchBody := bytes.NewBufferString(`{"single_tab_session_id":"single-tab-relay-stale-1","bound_tab_ref":"41","action_type":"status"}`)
	dispatchReq := httptest.NewRequest(http.MethodPost, "/api/v2/single-tab/actions/dispatch", dispatchBody)
	dispatchRes := httptest.NewRecorder()
	server.HandleRelayDispatchAction().ServeHTTP(dispatchRes, dispatchReq)
	if dispatchRes.Code != http.StatusServiceUnavailable {
		t.Fatalf("expected 503 for stale heartbeat, got %d", dispatchRes.Code)
	}
	if coordinator.lastUpdate == nil {
		t.Fatal("expected session update on stale heartbeat")
	}
	if coordinator.lastUpdate.Status != singletab.StatusHostDisconnected {
		t.Fatalf("expected status HOST_DISCONNECTED, got %s", coordinator.lastUpdate.Status)
	}
	if coordinator.lastUpdate.StatusReason != "extension heartbeat timed out" {
		t.Fatalf("unexpected status reason: %s", coordinator.lastUpdate.StatusReason)
	}
}
