package browserspa

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestPrivateCapabilitiesFailClosed(t *testing.T) {
	token := strings.Repeat("c", 40)
	for _, mode := range []string{"valid", "no_auth", "wrong_profile", "redirect", "oversized", "incomplete"} {
		t.Run(mode, func(t *testing.T) {
			h := CapabilitiesHandler(token)
			s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.URL.Path != CapabilityPath {
					t.Error("wrong capability path")
					w.WriteHeader(404)
					return
				}
				switch mode {
				case "wrong_profile":
					_, _ = w.Write([]byte(`{"profile":"other","worker_tls_gate":true,"live_authority":true,"terminal_context":true}`))
				case "redirect":
					http.Redirect(w, r, "http://127.0.0.1/forbidden", 302)
				case "oversized":
					_, _ = w.Write([]byte(strings.Repeat(" ", 1025)))
				case "incomplete":
					_, _ = w.Write([]byte(`{"profile":"graphql-read-stream-v1"}`))
				default:
					h.ServeHTTP(w, r)
				}
			}))
			defer s.Close()
			candidate := token
			if mode == "no_auth" {
				candidate = strings.Repeat("x", 40)
			}
			if (ProbeCapabilities(context.Background(), s.URL+"/control", candidate) == nil) != (mode == "valid") {
				t.Fatal("invalid capability decision")
			}
		})
	}
	for _, code := range []string{"spa_websocket_denied", "spa_response_denied", "spa_runtime_lost", "spa_finish_unknown"} {
		if !ValidDiagnostic(code) {
			t.Fatal("known safe diagnostic lost")
		}
	}
	if Diagnostic("https://secret.example/password") != "spa_query_failed" || ValidDiagnostic("spa_secret") {
		t.Fatal("unbounded diagnostic")
	}
}
