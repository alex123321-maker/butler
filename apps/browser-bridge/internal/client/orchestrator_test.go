package client

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/butler/butler/apps/browser-bridge/internal/protocol"
)

func TestClientCreateBindRequestAndGetActiveSession(t *testing.T) {
	t.Parallel()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodPost && r.URL.Path == "/api/v2/single-tab/bind-requests":
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"approval":{"approval_id":"approval-1","approval_type":"browser_tab_selection"}}`))
		case r.Method == http.MethodGet && r.URL.Path == "/api/v2/single-tab/session":
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"single_tab_session":{"single_tab_session_id":"single-tab-1","status":"ACTIVE"}}`))
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	client := New(server.URL, 5*time.Second, nil)
	approval, err := client.CreateBindRequest(context.Background(), protocol.BindRequestParams{
		RunID:      "run-1",
		SessionKey: "telegram:chat:1",
		TabCandidates: []protocol.BindTabCandidate{{
			InternalTabRef: "browser-a:1",
			Title:          "Inbox",
			CurrentURL:     "https://mail.example.com",
		}},
	})
	if err != nil {
		t.Fatalf("CreateBindRequest returned error: %v", err)
	}
	if approval.Approval["approval_id"] != "approval-1" {
		t.Fatalf("unexpected approval payload: %+v", approval.Approval)
	}

	session, err := client.GetActiveSession(context.Background(), "telegram:chat:1")
	if err != nil {
		t.Fatalf("GetActiveSession returned error: %v", err)
	}
	if session.SingleTabSession["single_tab_session_id"] != "single-tab-1" {
		t.Fatalf("unexpected session payload: %+v", session.SingleTabSession)
	}
}
