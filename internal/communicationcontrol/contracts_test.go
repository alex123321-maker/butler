package communicationcontrol

import (
	"context"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestCommunicationStrictIntentAndBlockingQuestions(t *testing.T) {
	good := `{"schema_version":1,"question":"Choose","input":{"type":"single_choice","options":[{"id":"a","label":"A"}]}}`
	if _, e := DecodeArgs("communication.widget", good); e != nil {
		t.Fatal(e)
	}
	for _, raw := range []string{
		strings.Replace(good, `"question"`, `"wait_for_response":false,"question"`, 1),
		strings.Replace(good, `"question"`, `"wait_for_response":null,"question"`, 1),
		strings.Replace(good, `"question"`, `"owner_id":"victim","question"`, 1),
		strings.Replace(good, `"question"`, `"chat_id":"other","question"`, 1),
		strings.Replace(good, `"schema_version":1`, `"schema_version":1,"schema_version":1`, 1),
		strings.Replace(good, `"id":"a"`, `"id":"a","id":"b"`, 1),
		strings.Replace(good, `"type":"single_choice"`, `"type":"single_choice","min_selections":1`, 1),
		good + ` {}`, `null`,
	} {
		if _, e := DecodeArgs("communication.widget", raw); e == nil {
			t.Fatalf("accepted ambiguous/unauthorized intent %s", raw)
		}
	}
	for _, raw := range []string{`{"schema_version":1,"purpose":"result","text":"ok","images":[]}`, `{"schema_version":1,"purpose":"result","text":"ok","text":"different"}`, `{"schema_version":1,"purpose":"question","text":"ok"}`} {
		if _, e := DecodeArgs("communication.message", raw); e == nil {
			t.Fatal("wrong method fields accepted", raw)
		}
	}
	for _, c := range Contracts() {
		for _, name := range []string{`"owner_id"`, `"chat_id"`, `"token"`, `"lease_id"`, `"grant"`} {
			if strings.Contains(c.InputSchemaJson, name) {
				t.Fatal("authority in schema", c.ToolName, name)
			}
		}
	}
}
func TestCommunicationTypedAnswers(t *testing.T) {
	q := QuestionInput{Type: "multi_choice", Options: []Option{{ID: "a"}, {ID: "b"}}, Min: 1, Max: 2}
	if ValidateAnswer(q, Answer{OptionIDs: []string{"a", "b"}}) != nil {
		t.Fatal("valid answer rejected")
	}
	for _, a := range []Answer{{}, {OptionIDs: []string{"a", "a"}}, {OptionIDs: []string{"c"}}, {Text: "yes"}, {OptionIDs: []string{"a"}, Text: "a"}} {
		if ValidateAnswer(q, a) == nil {
			t.Fatal("invalid answer", a)
		}
	}
	if ValidateAnswer(QuestionInput{Type: "text", MinLength: 2, MaxLength: 3}, Answer{Text: "абв"}) != nil {
		t.Fatal("UTF-8 answer length")
	}
}

type receiptService struct{ calls int }

func (s *receiptService) ExecuteDispatched(context.Context, Envelope) (Receipt, error) {
	s.calls++
	return Receipt{Status: "accepted", PublicationRef: "publication:fixture", Delivery: []Delivery{}}, nil
}
func TestCommunicationPrivateHTTPBoundary(t *testing.T) {
	const token = "communication-private-fixture-token-123456"
	svc := &receiptService{}
	h, e := NewHandler(token, svc)
	if e != nil {
		t.Fatal(e)
	}
	server := httptest.NewServer(h)
	defer server.Close()
	req := httptest.NewRequest("POST", Path, strings.NewReader(`{}`))
	w := httptest.NewRecorder()
	h.ServeHTTP(w, req)
	if w.Code != 403 || svc.calls != 0 {
		t.Fatal("missing private auth accepted")
	}
	c, e := NewClient(server.URL+Path, token)
	if e != nil {
		t.Fatal(e)
	}
	env := Envelope{RunID: "run", ToolCallID: "call", EffectID: "effect", ContractHash: strings.Repeat("a", 64), Tool: "communication.message", ArgsJSON: `{"schema_version":1,"purpose":"result","text":"ok"}`}
	if _, e = c.ExecuteDispatched(context.Background(), env); e != nil || svc.calls != 1 {
		t.Fatal("private roundtrip", e)
	}
	for _, url := range []string{server.URL + "/other", server.URL + Path + "?", server.URL + Path + "#part", "http://name:password@localhost" + Path} {
		if _, e = NewClient(url, token); e == nil {
			t.Fatal("unsafe endpoint accepted", url)
		}
	}
	for _, endpoint := range []string{"http://other:8080" + Path, "http://192.168.1.2" + Path, "http://orchestrator:9090" + Path, "http://orchestrator.evil:8080" + Path} {
		if _, e = NewClient(endpoint, token); e == nil {
			t.Fatal("private bearer token permitted over remote plain HTTP", endpoint)
		}
	}
	for _, endpoint := range []string{"http://orchestrator:8080" + Path, "https://private.example" + Path} {
		if _, e = NewClient(endpoint, token); e != nil {
			t.Fatal("allowed private/TLS endpoint rejected", endpoint, e)
		}
	}
}
