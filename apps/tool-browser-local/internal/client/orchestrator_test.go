package client

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestClientGetAndReleaseSession(t *testing.T) {
	t.Parallel()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/v2/single-tab/session/single-tab-1":
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"single_tab_session":{"single_tab_session_id":"single-tab-1","status":"ACTIVE"}}`))
		case "/api/v2/single-tab/session/single-tab-1/state":
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"single_tab_session":{"single_tab_session_id":"single-tab-1","status":"ACTIVE","current_url":"https://example.com/next"}}`))
		case "/api/v2/single-tab/session/single-tab-1/release":
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"single_tab_session":{"single_tab_session_id":"single-tab-1","status":"REVOKED_BY_USER"},"changed":true}`))
		case "/api/v2/artifacts/browser-captures":
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"artifact":{"artifact_id":"artifact-capture-1","artifact_type":"browser_capture"}}`))
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	client := New(server.URL, 5*time.Second, nil)
	session, err := client.GetSession(context.Background(), "single-tab-1")
	if err != nil {
		t.Fatalf("GetSession returned error: %v", err)
	}
	if session.SingleTabSession["status"] != "ACTIVE" {
		t.Fatalf("unexpected session payload: %+v", session.SingleTabSession)
	}

	updated, err := client.UpdateSessionState(context.Background(), "single-tab-1", UpdateSessionStateParams{
		Status:     "ACTIVE",
		CurrentURL: "https://example.com/next",
	})
	if err != nil {
		t.Fatalf("UpdateSessionState returned error: %v", err)
	}
	if updated.SingleTabSession["current_url"] != "https://example.com/next" {
		t.Fatalf("unexpected updated session payload: %+v", updated.SingleTabSession)
	}

	artifact, err := client.CreateBrowserCaptureArtifact(context.Background(), CreateBrowserCaptureArtifactParams{
		RunID:              "run-1",
		SessionKey:         "telegram:chat:1",
		SingleTabSessionID: "single-tab-1",
		ImageDataURL:       "data:image/png;base64,abc",
	})
	if err != nil {
		t.Fatalf("CreateBrowserCaptureArtifact returned error: %v", err)
	}
	if artifact.Artifact["artifact_id"] != "artifact-capture-1" {
		t.Fatalf("unexpected artifact payload: %+v", artifact.Artifact)
	}

	released, changed, err := client.ReleaseSession(context.Background(), "single-tab-1")
	if err != nil {
		t.Fatalf("ReleaseSession returned error: %v", err)
	}
	if !changed {
		t.Fatal("expected changed=true")
	}
	if released.SingleTabSession["status"] != "REVOKED_BY_USER" {
		t.Fatalf("unexpected released session payload: %+v", released.SingleTabSession)
	}
}
