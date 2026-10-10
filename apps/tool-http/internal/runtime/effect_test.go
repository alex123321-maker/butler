package runtime

import (
	"context"
	"github.com/butler/butler/internal/security/egress/egresstest"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"

	runtimev1 "github.com/butler/butler/internal/gen/runtime/v1"
	toolbrokerv1 "github.com/butler/butler/internal/gen/toolbroker/v1"
)

func TestSystemEffectKeyIsScopedAndNeverRedirected(t *testing.T) {
	var received atomic.Int32
	other := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { received.Add(1) }))
	defer other.Close()
	origin := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Idempotency-Key") != "system-key" || r.Header.Get("Butler-Action-Fingerprint") != "system-fingerprint" {
			t.Error("system effect identity missing")
		}
		http.Redirect(w, r, other.URL, http.StatusTemporaryRedirect)
	}))
	defer origin.Close()
	server := NewServer(egresstest.Client(origin), nil)
	request := &runtimev1.ExecuteRequest{Context: &runtimev1.ExecutionContext{ActionId: "effect-id", IdempotencyKey: "system-key", ActionFingerprint: "system-fingerprint"}, ToolCall: &toolbrokerv1.ToolCall{ToolName: "http.request", ArgsJson: `{"method":"POST","url":"` + egresstest.Origin + `"}`}, Contract: &toolbrokerv1.ToolContract{ToolName: "http.request", EffectContract: &toolbrokerv1.EffectContract{RetrySafety: "external_idempotency", VerificationMethod: "external_receipt_v1", IdempotencyHeader: "Idempotency-Key", ScopeOrigin: egresstest.Origin}}}
	response, err := server.Execute(context.Background(), request)
	if err != nil || response.GetResult().GetStatus() != "completed" || received.Load() != 0 {
		t.Fatal("scoped mutation followed a redirect")
	}
	for _, args := range []string{`{"method":"POST","url":"` + egresstest.Origin + `","headers":{"Idempotency-Key":"caller-key"}}`, `{"method":"POST","url":"` + other.URL + `"}`} {
		request.ToolCall.ArgsJson = args
		response, err = server.Execute(context.Background(), request)
		if err != nil || response.GetResult().GetStatus() != "failed" || received.Load() != 0 {
			t.Fatal("caller changed the key or recipient")
		}
	}
}

func TestGenericContractDoesNotAcquireRecipientGuarantee(t *testing.T) {
	var received atomic.Int32
	site := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { received.Add(1) }))
	defer site.Close()
	server := NewServer(site.Client(), nil)
	req := &runtimev1.ExecuteRequest{Context: &runtimev1.ExecutionContext{ActionId: "effect-id", IdempotencyKey: "injected-key", ActionFingerprint: "fingerprint"}, ToolCall: &toolbrokerv1.ToolCall{ToolName: "http.request", ArgsJson: `{"method":"POST","url":"` + site.URL + `"}`}, Contract: &toolbrokerv1.ToolContract{ToolName: "http.request"}}
	response, err := server.Execute(context.Background(), req)
	if err != nil || response.GetResult().GetStatus() != "failed" || received.Load() != 0 {
		t.Fatal("generic service received a fictitious idempotency guarantee")
	}
}
