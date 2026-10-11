package candidatecontrol

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

const proposal = `{"source_id":"source","source_revision":1,"fact_key":"language","value_json":"\"ru\"","summary":"candidate"}`

func TestMemoryCandidatePrivateShape(t *testing.T) {
	for _, raw := range []string{
		`null`, proposal + ` {}`, strings.Replace(proposal, `"source_revision":1`, `"source_revision":1.5`, 1),
		strings.Replace(proposal, `"source_revision":1`, `"source_revision":null`, 1),
		strings.Replace(proposal, `"source_id":"source"`, `"source_id":"source","source_id":"other"`, 1),
		strings.Replace(proposal, `"source_id"`, `"Source_ID"`, 1),
		strings.Replace(proposal, `"summary":"candidate"`, `"summary":null`, 1),
		strings.Replace(proposal, `"summary":"candidate"`, `"summary":"candidate","owner_id":"forged"`, 1),
		strings.Replace(proposal, `"summary":"candidate"`, `"summary":"candidate","dedup_key":"forged"`, 1),
		strings.Replace(proposal, `"summary":"candidate"`, `"summary":"candidate","confidence":1`, 1),
	} {
		if _, err := DecodeArgs(raw); err == nil {
			t.Fatal("ambiguous/authority-bearing candidate accepted", raw)
		}
	}
	if _, err := DecodeArgs(proposal); err != nil {
		t.Fatal(err)
	}
	if _, err := decodeReceipt([]byte(`{"claim_id":"claim_` + strings.Repeat("a", 40) + `","status":"confirmed"}`)); err == nil {
		t.Fatal("receipt claimed owner acceptance")
	}
}

type candidateFixture struct{ calls int }

func (f *candidateFixture) ProposeDispatched(context.Context, Envelope) (Receipt, error) {
	f.calls++
	return Receipt{"claim_" + strings.Repeat("a", 40), "candidate"}, nil
}
func TestMemoryCandidatePrivateHTTP(t *testing.T) {
	f := &candidateFixture{}
	token := strings.Repeat("private-fixture-", 3)
	h, err := NewHandler(token, f)
	if err != nil {
		t.Fatal(err)
	}
	s := httptest.NewServer(h)
	defer s.Close()
	e := Envelope{RunID: "run", ToolCallID: "call", EffectID: "effect", ArgsJSON: proposal, ContractHash: strings.Repeat("b", 64)}
	client, err := NewClient(s.URL+Path, token)
	if err != nil {
		t.Fatal(err)
	}
	if receipt, err := client.ProposeDispatched(context.Background(), e); err != nil || receipt.Status != "candidate" || f.calls != 1 {
		t.Fatal(receipt, err)
	}
	raw, _ := json.Marshal(e)
	for _, suffix := range []string{"?query=1", "/other"} {
		r := httptest.NewRequest(http.MethodPost, Path+suffix, strings.NewReader(string(raw)))
		r.Header.Set("Authorization", "Bearer "+token)
		w := httptest.NewRecorder()
		h.ServeHTTP(w, r)
		if w.Code == http.StatusOK || f.calls != 1 {
			t.Fatal("alternate endpoint invoked candidate control")
		}
	}
	r := httptest.NewRequest(http.MethodPost, Path, strings.NewReader(string(raw)))
	r.Header.Add("Authorization", "Bearer "+token)
	r.Header.Add("Authorization", "Bearer "+token)
	w := httptest.NewRecorder()
	h.ServeHTTP(w, r)
	if w.Code == http.StatusOK || f.calls != 1 {
		t.Fatal("ambiguous authentication reached publication")
	}
	for _, endpoint := range []string{"http://192.168.1.2" + Path, s.URL + Path + "?", s.URL + "/other", s.URL + Path + "#secret"} {
		if _, err := NewClient(endpoint, token); err == nil {
			t.Fatal("unsafe endpoint accepted", endpoint)
		}
	}
	redirect := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, s.URL+Path, http.StatusTemporaryRedirect)
	}))
	defer redirect.Close()
	c, err := NewClient(redirect.URL+Path, token)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := c.ProposeDispatched(context.Background(), e); err == nil || f.calls != 1 {
		t.Fatal("redirect followed with private authentication")
	}
}
