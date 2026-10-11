package delegation_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/butler/butler/internal/actionauth"
	"github.com/butler/butler/internal/delegation"
	toolbrokerv1 "github.com/butler/butler/internal/gen/toolbroker/v1"
)

func admissionJSON(t *testing.T) string {
	t.Helper()
	a := delegation.AdmissionArgs{ContractSource: delegation.SourceRef{ID: "contract", Revision: 1, AccessRevision: 1, Hash: strings.Repeat("a", 64)}, Sources: []delegation.SourceRef{}, Dependencies: []string{}, Budget: delegation.Limits{ModelTurns: 2, Tokens: 2048, ActiveMillis: 1000}, Deadline: time.Now().Add(time.Minute)}
	raw, err := json.Marshal(a)
	if err != nil {
		t.Fatal(err)
	}
	return string(raw)
}

func TestAdmissionBoundedShapeAndFixedContract(t *testing.T) {
	raw := admissionJSON(t)
	if _, err := delegation.DecodeAdmissionArgs(raw); err != nil {
		t.Fatal(err)
	}
	for _, invalid := range []string{
		strings.Replace(raw, `"source_refs":[]`, `"source_refs":null`, 1),
		strings.Replace(raw, `"dependencies":[]`, `"dependencies":null`, 1),
		strings.Replace(raw, `"tokens":2048`, `"tokens":2048,"tokens":999`, 1),
		strings.Replace(raw, `"tokens":2048`, `"tokens":-1`, 1),
		strings.Replace(raw, `"tokens":2048,`, ``, 1),
		strings.Replace(raw, `"source_id":"contract"`, `"source_id":"contract","owner_id":"forged"`, 1),
		strings.TrimSuffix(raw, "}") + `,"permissions":[]}`,
		strings.TrimSuffix(raw, "}") + `,"parent_task_id":"forged"}`,
		raw + `{}`, strings.Repeat(" ", 128*1024) + raw,
	} {
		if _, err := delegation.DecodeAdmissionArgs(invalid); err == nil {
			t.Fatalf("ambiguous or authority-bearing JSON accepted: %.150s", invalid)
		}
	}
	contract := delegation.AdmissionContract()
	b, _, err := actionauth.BuildBinding("intent", &toolbrokerv1.ToolCall{RunId: "run", ToolCallId: "call", ToolName: delegation.AdmissionTool, ArgsJson: raw}, contract, actionauth.ContextSnapshot{OwnerID: "owner", RunID: "run", TaskID: "parent", TaskRevision: 1, ConstraintsRevision: 1})
	if err != nil || b.Effect != actionauth.EffectPrepare || b.Resource != "tool:task.delegate" || contract.GetRuntimeTarget() != delegation.AdmissionTarget || contract.GetSupportsCredentialRefs() {
		t.Fatalf("fixed control contract changed: %+v %v", b, err)
	}
	h, _ := actionauth.ContractHash(contract)
	contract.InputSchemaJson = `{}`
	other, _ := actionauth.ContractHash(contract)
	if h == other {
		t.Fatal("contract mutation did not invalidate pinned identity")
	}
}

type admissionStub struct{ calls atomic.Int32 }

func (s *admissionStub) AdmitDispatched(context.Context, delegation.AdmissionEnvelope) (delegation.AdmissionReceipt, error) {
	s.calls.Add(1)
	return delegation.AdmissionReceipt{TaskID: "child", InputID: "input", State: "queued", ContractRevision: 1}, nil
}

func TestAdmissionPrivateHTTPBoundary(t *testing.T) {
	const token = "private-fixture-token-32bytes-long-value"
	stub := &admissionStub{}
	handler, err := delegation.NewAdmissionHandler(token, stub)
	if err != nil {
		t.Fatal(err)
	}
	server := httptest.NewServer(handler)
	defer server.Close()
	for _, body := range []string{`{}`, `{"run_id":"a","run_id":"b"}`, `{"owner_id":"forged"}`, `{} {}`, strings.Repeat("x", 256*1024+1)} {
		req, _ := http.NewRequest(http.MethodPost, server.URL, strings.NewReader(body))
		if body != `{}` {
			req.Header.Set("Authorization", "Bearer "+token)
		}
		response, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Fatal(err)
		}
		response.Body.Close()
		if response.StatusCode == 200 {
			t.Fatal("untrusted or ambiguous envelope reached service")
		}
	}
	if stub.calls.Load() != 0 {
		t.Fatal("invalid envelope called kernel admission")
	}
	client, err := delegation.NewAdmissionClient(server.URL, token)
	if err != nil {
		t.Fatal(err)
	}
	if r, err := client.AdmitDispatched(context.Background(), delegation.AdmissionEnvelope{RunID: "run"}); err != nil || r.TaskID != "child" {
		t.Fatal(r, err)
	}
	if stub.calls.Load() != 1 {
		t.Fatal("valid private request not forwarded once")
	}
	for _, endpoint := range []string{"http://user:pass@localhost/", "http://localhost/?token=x", "http://localhost/#x", "file:///tmp", "//localhost/path"} {
		if _, err := delegation.NewAdmissionClient(endpoint, token); err == nil {
			t.Fatal("unsafe operator endpoint accepted")
		}
	}
	var targetCalls atomic.Int32
	target := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { targetCalls.Add(1) }))
	defer target.Close()
	redirect := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, target.URL, http.StatusTemporaryRedirect)
	}))
	defer redirect.Close()
	client, _ = delegation.NewAdmissionClient(redirect.URL, token)
	if _, err := client.AdmitDispatched(context.Background(), delegation.AdmissionEnvelope{}); err == nil || targetCalls.Load() != 0 {
		t.Fatal("private token followed redirect")
	}
}
