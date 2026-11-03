package browserspa

import (
	"encoding/json"
	"testing"
)

func TestSubscriptionGrammarKeepsMutationsAndMultipleRootsDenied(t *testing.T) {
	for _, q := range []string{`subscription Stock{stock}`, `subscription S($id:ID!){alias:stock(id:$id){amount}}`} {
		raw, _ := json.Marshal(map[string]any{"query": q})
		if SubscriptionBody(raw) != nil {
			t.Fatal("supported subscription denied")
		}
		if QueryBody(raw) == nil {
			t.Fatal("subscription admitted as POST query")
		}
	}
	for _, q := range []string{`{stock}`, `query{stock}`, `mutation{buy}`, `subscription{a b}`, `subscription{__typename}`, `subscription{x:__schema{name}}`, `subscription{...F} fragment F on X{stock}`, `subscription{stock} mutation{buy}`} {
		raw, _ := json.Marshal(map[string]any{"query": q})
		if SubscriptionBody(raw) == nil {
			t.Fatal("unsupported subscription accepted", q)
		}
	}
}

func TestSocketReadStateMachineRejectsReplayMutationAndAuthPayload(t *testing.T) {
	s := &SocketPolicy{}
	sub := []byte(`{"id":"one","type":"subscribe","payload":{"query":"subscription Stock{stock}"}}`)
	if s.Client(sub) == nil || s.Server([]byte(`{"type":"connection_ack"}`)) == nil {
		t.Fatal("read granted before initialization")
	}
	if s.Client([]byte(`{"type":"connection_init","payload":{"Authorization":"secret"}}`)) == nil {
		t.Fatal("authentication payload accepted")
	}
	if s.Client([]byte(`{"type":"connection_init"}`)) != nil || s.Server([]byte(`{"type":"connection_ack"}`)) != nil {
		t.Fatal("initialization positive control failed")
	}
	if s.Client([]byte(`{"type":"connection_init"}`)) == nil {
		t.Fatal("duplicate init accepted")
	}
	if s.Client([]byte(`{"id":"mut","type":"subscribe","payload":{"query":"mutation{buy}"}}`)) == nil {
		t.Fatal("mutation forwarded")
	}
	if s.Client(sub) != nil {
		t.Fatal("read subscription denied")
	}
	if s.Client(sub) == nil {
		t.Fatal("duplicate subscription ID replayed")
	}
	if s.Server([]byte(`{"id":"other","type":"next","payload":{"data":{}}}`)) == nil {
		t.Fatal("foreign subscription result accepted")
	}
	if s.Server([]byte(`{"id":"one","type":"next","payload":{"data":{"stock":1}}}`)) != nil {
		t.Fatal("read result denied")
	}
	if s.Client([]byte(`{"id":"one","type":"complete"}`)) != nil || s.Client(sub) == nil {
		t.Fatal("completion reopened old ID")
	}
	if s.Server([]byte(`{"id":"one","type":"next","payload":{"data":{}}}`)) == nil {
		t.Fatal("completed result accepted")
	}
	for _, id := range []string{"two", "three", "four"} {
		raw, _ := json.Marshal(map[string]any{"id": id, "type": "subscribe", "payload": map[string]string{"query": "{stock}"}})
		if s.Client(raw) != nil {
			t.Fatal("bounded query positive control failed")
		}
	}
	if s.Client([]byte(`{"id":"five","type":"subscribe","payload":{"query":"{stock}"}}`)) == nil {
		t.Fatal("subscription cap exceeded")
	}
	if s.Client([]byte(`{"type":"buy","payload":{}}`)) == nil || s.Client([]byte(`{"type":"ping","type":"subscribe"}`)) == nil {
		t.Fatal("arbitrary/duplicate-key frame accepted")
	}
	if s.Server([]byte(`{"type":"ping","payload":{"t":42}}`)) != nil || s.Client([]byte(`{"type":"pong","payload":{"t":42}}`)) != nil {
		t.Fatal("correlated ping reply denied")
	}
	if s.Client([]byte(`{"type":"pong","payload":{"t":99}}`)) == nil {
		t.Fatal("unrequested arbitrary control payload forwarded")
	}
}
