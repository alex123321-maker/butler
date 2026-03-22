package client

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestBrowserBridgeClientDispatchAction(t *testing.T) {
	t.Parallel()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v1/actions/dispatch" {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"action":{"single_tab_session_id":"single-tab-5","session_status":"ACTIVE","result_json":"{\"ok\":true}","current_url":"https://example.com","current_title":"Example"}}`))
	}))
	defer server.Close()

	client := NewBrowserBridge(server.URL, 5*time.Second, nil)
	result, err := client.DispatchAction(context.Background(), DispatchActionParams{
		SingleTabSessionID: "single-tab-5",
		BoundTabRef:        "22",
		ActionType:         "navigate",
	})
	if err != nil {
		t.Fatalf("DispatchAction returned error: %v", err)
	}
	if result.Action["session_status"] != "ACTIVE" {
		t.Fatalf("unexpected dispatch action payload: %+v", result.Action)
	}
}
