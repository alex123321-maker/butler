package runtime

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/butler/butler/internal/extensions"
	runtimev1 "github.com/butler/butler/internal/gen/runtime/v1"
	toolbrokerv1 "github.com/butler/butler/internal/gen/toolbroker/v1"
	"github.com/butler/butler/internal/security/egress/egresstest"
	"google.golang.org/protobuf/proto"
)

func runtimeFixture(t *testing.T, handler http.Handler) (*Server, *runtimev1.ExecuteRequest) {
	t.Helper()
	remote := httptest.NewServer(handler)
	t.Cleanup(remote.Close)
	profiles := map[string]extensions.Profile{"reviewed-http": {Target: "extension:9090", Dependency: extensions.Dependency{ID: "runtime", Version: "1", Source: "https://example.invalid/runtime", SHA256: strings.Repeat("a", 64)}}}
	pin := ""
	for key := range extensions.ProfilePins(profiles) {
		pin = key
	}
	e := &toolbrokerv1.ExtensionContract{ExtensionId: "notes", ManifestHash: strings.Repeat("b", 64), Version: "1.0.0", Source: "https://example.invalid/package", Generation: 2, ProfilePin: pin, Method: "GET", Effect: "read", Resources: []*toolbrokerv1.ExtensionResource{{Origin: egresstest.Origin, PathPrefix: "/notes"}}, TimeoutMs: 3000, EgressBytes: 1024 * 1024, OutputBytes: 1024}
	call := &toolbrokerv1.ToolCall{ToolName: "extension.notes.read", RunId: "run", ToolCallId: "call", ArgsJson: `{"url":"` + egresstest.Origin + `/notes/one"}`}
	req := &runtimev1.ExecuteRequest{ToolCall: call, Contract: &toolbrokerv1.ToolContract{ToolName: call.ToolName, Extension: e}, Context: &runtimev1.ExecutionContext{RunId: call.RunId, ToolCallId: call.ToolCallId, ToolName: call.ToolName, ActionId: "action", ActionFingerprint: "fingerprint", TimeoutMs: 3000}}
	return NewServerWithEgress(profiles, egresstest.Policy(remote)), req
}
func TestReviewedRuntimeRejectsDangerousProposalsBeforeNetwork(t *testing.T) {
	var contacted atomic.Int32
	s, req := runtimeFixture(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { contacted.Add(1); w.Write([]byte(`{"note":"safe"}`)) }))
	for _, test := range []struct {
		name   string
		mutate func(*runtimev1.ExecuteRequest)
	}{
		{"private target", func(r *runtimev1.ExecuteRequest) { r.ToolCall.ArgsJson = `{"url":"http://127.0.0.1/notes/one"}` }},
		{"escaped prefix", func(r *runtimev1.ExecuteRequest) {
			r.ToolCall.ArgsJson = `{"url":"` + egresstest.Origin + `/notes-other"}`
		}},
		{"encoded traversal", func(r *runtimev1.ExecuteRequest) {
			r.ToolCall.ArgsJson = `{"url":"` + egresstest.Origin + `/notes/%2e%2e/admin"}`
		}},
		{"body on read", func(r *runtimev1.ExecuteRequest) {
			r.ToolCall.ArgsJson = `{"url":"` + egresstest.Origin + `/notes/one","body":{"command":"rm"}}`
		}},
		{"shell command", func(r *runtimev1.ExecuteRequest) {
			r.ToolCall.ArgsJson = `{"url":"` + egresstest.Origin + `/notes/one","command":"sh"}`
		}},
		{"duplicate interpretation", func(r *runtimev1.ExecuteRequest) {
			r.ToolCall.ArgsJson = `{"url":"` + egresstest.Origin + `/notes/one","url":"http://127.0.0.1/private"}`
		}},
		{"unreviewed profile", func(r *runtimev1.ExecuteRequest) { r.Contract.Extension.ProfilePin = strings.Repeat("c", 64) }},
		{"write disguised as read", func(r *runtimev1.ExecuteRequest) { r.Contract.Extension.Method = "POST" }},
		{"raw credential", func(r *runtimev1.ExecuteRequest) {
			r.ToolCall.ArgsJson = `{"url":"` + egresstest.Origin + `/notes/one","authorization":"raw-secret"}`
		}},
		{"unsolicited secret", func(r *runtimev1.ExecuteRequest) {
			r.ResolvedCredentials = []*runtimev1.ResolvedCredential{{Value: "raw-secret"}}
		}},
		{"caller idempotency", func(r *runtimev1.ExecuteRequest) { r.Context.IdempotencyKey = "caller-key" }},
		{"missing grant", func(r *runtimev1.ExecuteRequest) { r.Context.ActionId = "" }},
		{"byte allowance", func(r *runtimev1.ExecuteRequest) { r.Contract.Extension.EgressBytes = 1 }},
	} {
		t.Run(test.name, func(t *testing.T) {
			changed := proto.Clone(req).(*runtimev1.ExecuteRequest)
			test.mutate(changed)
			response, err := s.Execute(context.Background(), changed)
			if err != nil || response.GetResult().GetStatus() != "failed" || contacted.Load() != 0 {
				t.Fatal("dangerous proposal reached recipient")
			}
		})
	}
	response, err := s.Execute(context.Background(), req)
	if err != nil || response.GetResult().GetStatus() != "completed" || contacted.Load() != 1 {
		t.Fatal("bounded read unavailable")
	}
}
func TestReviewedRuntimeStopsRedirectsAndResponseOverrun(t *testing.T) {
	var leaked atomic.Int32
	destination := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { leaked.Add(1) }))
	defer destination.Close()
	s, req := runtimeFixture(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, destination.URL, http.StatusTemporaryRedirect)
	}))
	response, err := s.Execute(context.Background(), req)
	if err != nil || response.GetResult().GetStatus() != "failed" || leaked.Load() != 0 {
		t.Fatal("extension followed redirect")
	}
	s, req = runtimeFixture(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(`{"large":"` + strings.Repeat("x", 2048) + `"}`))
	}))
	response, err = s.Execute(context.Background(), req)
	if err != nil || response.GetResult().GetStatus() != "failed" || strings.Contains(response.GetResult().GetResultJson(), "large") {
		t.Fatal("response overrun released partial output")
	}
}
func TestReviewedRuntimeUsesDeferredSecretsAndSystemRecipientIdentity(t *testing.T) {
	s, req := runtimeFixture(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer fixture-private-token" || r.Header.Get("Idempotency-Key") != "system-key" || r.Header.Get("Butler-Action-Fingerprint") != "fingerprint" {
			t.Error("private metadata not enforced")
		}
		json.NewEncoder(w).Encode(map[string]any{"echo": "fixture-private-token", "butler_effect_receipt": map[string]string{"idempotency_key": r.Header.Get("Idempotency-Key"), "action_fingerprint": r.Header.Get("Butler-Action-Fingerprint"), "effect_state": "effect_confirmed", "artifact_id": "artifact-one"}})
	}))
	req.Contract.Extension.Method, req.Contract.Extension.Effect = "POST", "commit"
	req.Contract.Extension.Credentials = []*toolbrokerv1.ExtensionCredential{{Input: "authorization", Field: "token", Required: true}}
	receipt := &toolbrokerv1.EffectContract{RetrySafety: "external_idempotency", VerificationMethod: "external_receipt_v1", IdempotencyHeader: "Idempotency-Key", ScopeOrigin: egresstest.Origin, VerificationUrl: egresstest.Origin + "/notes/effects/{idempotency_key}"}
	req.Contract.Extension.Receipt = receipt
	req.Contract.EffectContract = proto.Clone(receipt).(*toolbrokerv1.EffectContract)
	req.Context.IdempotencyKey = "system-key"
	req.ToolCall.ArgsJson = `{"url":"` + egresstest.Origin + `/notes/one","body":{"text":"bounded"},"authorization":{"type":"credential_ref","alias":"fixture","field":"token"}}`
	req.ToolCall.CredentialRefs = []*toolbrokerv1.CredentialRef{{Type: "credential_ref", Alias: "fixture", Field: "token"}}
	req.ResolvedCredentials = []*runtimev1.ResolvedCredential{{Alias: "fixture", Field: "token", Value: "fixture-private-token"}}
	response, err := s.Execute(context.Background(), req)
	if err != nil || response.GetResult().GetStatus() != "completed" || strings.Contains(response.GetResult().GetResultJson(), "fixture-private-token") || !strings.Contains(response.GetResult().GetResultJson(), "butler_effect_receipt") {
		t.Fatal("secret or recipient metadata boundary failed")
	}
	req.Contract.EffectContract.ScopeOrigin = "https://other.invalid"
	response, _ = s.Execute(context.Background(), req)
	if response.GetResult().GetStatus() != "failed" {
		t.Fatal("unreviewed changed receipt accepted")
	}
}
func TestReadinessHasNoWriteOrCredentialChannel(t *testing.T) {
	s, req := runtimeFixture(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != "GET" || r.ContentLength > 0 || r.Header.Get("Authorization") != "" {
			t.Error("probe acquired ordinary execution capabilities")
		}
		w.Write([]byte(`{"ready":true}`))
	}))
	probe := &runtimev1.ProbeExtensionRequest{ProbeId: "probe", Url: egresstest.Origin + "/notes/health", Extension: req.Contract.Extension}
	response, err := s.ProbeExtension(context.Background(), probe)
	if err != nil || !response.Ready || len(response.ResponseDigest) != 64 {
		t.Fatal("fixed readiness unavailable")
	}
	probe.Extension.Method = "POST"
	if _, err = s.ProbeExtension(context.Background(), probe); err == nil {
		t.Fatal("mutating probe accepted")
	}
}
