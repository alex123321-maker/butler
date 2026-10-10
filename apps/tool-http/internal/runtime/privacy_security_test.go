package runtime

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"sync/atomic"
	"testing"

	runtimev1 "github.com/butler/butler/internal/gen/runtime/v1"
	toolbrokerv1 "github.com/butler/butler/internal/gen/toolbroker/v1"
	"github.com/butler/butler/internal/security/egress"
	"github.com/butler/butler/internal/security/egress/egresstest"
)

func TestReflectedCredentialCookieAndEncodingsNeverLeaveRuntime(t *testing.T) {
	secret, cookie := "synthetic private / credential+value", "synthetic-cookie-value"
	recipient := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer "+secret {
			t.Error("credential was not injected privately")
		}
		w.Header().Set("Set-Cookie", "sid="+cookie+"; HttpOnly")
		w.Header().Set("X-Echo", secret)
		w.Header().Set("Content-Type", "text/plain")
		_, _ = io.WriteString(w, strings.Join([]string{secret, cookie, url.QueryEscape(secret), base64.StdEncoding.EncodeToString([]byte(secret)), base64.RawURLEncoding.EncodeToString([]byte(cookie))}, " "))
	}))
	defer recipient.Close()
	var logs bytes.Buffer
	server := NewServer(egresstest.Client(recipient), slog.New(slog.NewJSONHandler(&logs, nil)))
	args := map[string]any{"method": "GET", "url": egresstest.Origin, "auth": map[string]any{"type": "credential_ref", "alias": "private", "field": "token"}}
	encoded, _ := json.Marshal(args)
	response, err := server.Execute(context.Background(), &runtimev1.ExecuteRequest{ToolCall: &toolbrokerv1.ToolCall{ToolName: "http.request", ArgsJson: string(encoded)}, Contract: &toolbrokerv1.ToolContract{ToolName: "http.request", AllowedDomains: []string{"external.test"}}, ResolvedCredentials: []*runtimev1.ResolvedCredential{{Alias: "private", Field: "token", Value: secret}}})
	if err != nil || response.GetResult().GetStatus() != "completed" {
		t.Fatal("authorized fixture request failed")
	}
	output := response.GetResult().GetResultJson() + logs.String()
	for _, private := range []string{secret, cookie, url.QueryEscape(secret), base64.StdEncoding.EncodeToString([]byte(secret)), base64.RawURLEncoding.EncodeToString([]byte(cookie)), "Set-Cookie", "X-Echo"} {
		if strings.Contains(output, private) {
			t.Fatalf("private response material leaked: variant %d", len(private))
		}
	}
	if !strings.Contains(output, "[REDACTED]") {
		t.Fatal("reflected values were not removed")
	}
}

func TestPrivateDestinationProtocolHeadersAndOversizedReleaseHaveNoEffects(t *testing.T) {
	var effects atomic.Int32
	recipient := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { effects.Add(1); w.WriteHeader(200) }))
	defer recipient.Close()
	server := NewServer(egresstest.Client(recipient), nil)
	for _, args := range []map[string]any{
		{"url": "http://127.0.0.1/private?password=synthetic-private"},
		{"url": egresstest.Origin, "headers": map[string]string{"Authorization": "Bearer synthetic-private"}},
		{"url": egresstest.Origin, "headers": map[string]string{"Host": "127.0.0.1"}},
		{"url": egresstest.Origin, "method": "POST", "body": strings.Repeat("x", int(egress.MaxRequestBytes))},
	} {
		encoded, _ := json.Marshal(args)
		response, err := server.Execute(context.Background(), &runtimev1.ExecuteRequest{ToolCall: &toolbrokerv1.ToolCall{ToolName: "http.request", ArgsJson: string(encoded)}, Contract: &toolbrokerv1.ToolContract{ToolName: "http.request"}})
		if err != nil || response.GetResult().GetStatus() != "failed" {
			t.Fatal("unsafe request accepted")
		}
		if strings.Contains(response.GetResult().GetError().GetMessage(), "synthetic-private") {
			t.Fatal("private request data appeared in error")
		}
	}
	if effects.Load() != 0 {
		t.Fatal("denied request had recipient effects")
	}
}
