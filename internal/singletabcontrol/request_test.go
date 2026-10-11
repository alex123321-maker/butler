package singletabcontrol

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

type requestFixture struct{ calls int }

func (s *requestFixture) RequestDispatched(context.Context, RequestEnvelope) (RequestReceipt, error) {
	s.calls++
	return RequestReceipt{ApprovalID: strings.Repeat("a", 48), State: "awaiting_owner_choice"}, nil
}
func TestGuardedRequestPrivateContract(t *testing.T) {
	for _, a := range []string{"{}", " { } \n"} {
		if !ValidRequestArgs(a) {
			t.Fatal("empty object refused")
		}
	}
	for _, a := range []string{"null", "[]", "{}{}", `{"run_id":"forged"}`, `{"x":0,"x":1}`} {
		if ValidRequestArgs(a) {
			t.Fatal("nonempty request args accepted", a)
		}
	}
	service := &requestFixture{}
	token := strings.Repeat("p", 32)
	handler, err := NewRequestHandler(token, service)
	if err != nil {
		t.Fatal(err)
	}
	server := httptest.NewServer(handler)
	defer server.Close()
	client, err := NewRequestClient(server.URL, token)
	if err != nil {
		t.Fatal(err)
	}
	e := RequestEnvelope{RunID: "run", ToolCallID: "call", EffectID: "effect", ArgsJSON: "{}", ContractHash: strings.Repeat("b", 64)}
	receipt, err := client.RequestDispatched(context.Background(), e)
	if err != nil || service.calls != 1 || receipt.State != "awaiting_owner_choice" {
		t.Fatal(receipt, err)
	}
	raw, _ := json.Marshal(e)
	for _, invalid := range []string{"null", string(raw) + "{}", strings.Replace(string(raw), `"run_id":"run"`, `"run_id":"run","RUN_ID":"run"`, 1), strings.Replace(string(raw), `"run_id":"run"`, `"run_id":"run","run_\u0069d":"other"`, 1), strings.Replace(string(raw), `"run_id":"run"`, `"run_id":null`, 1)} {
		req := httptest.NewRequest(http.MethodPost, "/internal/guarded-tab-request", strings.NewReader(invalid))
		req.Header.Set("Authorization", "Bearer "+token)
		out := httptest.NewRecorder()
		handler.ServeHTTP(out, req)
		if out.Code == http.StatusOK || service.calls != 1 {
			t.Fatal("ambiguous private envelope reached owning service")
		}
	}
	for _, endpoint := range []string{"http://192.0.2.1/internal/guarded-tab-request", "https://user:password@example.test/private", "https://example.test/private?next=token", "https://example.test/private#other"} {
		if _, err = NewRequestClient(endpoint, token); err == nil {
			t.Fatal("unsafe fixed endpoint accepted")
		}
	}
	redirect := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, server.URL, http.StatusTemporaryRedirect)
	}))
	defer redirect.Close()
	redirected, err := NewRequestClient(redirect.URL, token)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = redirected.RequestDispatched(context.Background(), e); err == nil || service.calls != 1 {
		t.Fatal("private token followed redirect")
	}
}
