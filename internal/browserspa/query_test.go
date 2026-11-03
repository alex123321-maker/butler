package browserspa

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestQueryProfileGrammar(t *testing.T) {
	for _, query := range []string{
		`{ catalog { title price } }`,
		`query Catalog($filter: String!, $limit: Int = 10) { rows: catalog(filter: $filter, limit: $limit) { title } }`,
		`# mutation is a comment
query Q($ids: [ID!]!) { rows(ids:$ids, input:{sort:ASC, nested:[true,null,-1,1.5e2,"mutation"]}) { id } } # tail`,
	} {
		raw, _ := json.Marshal(map[string]any{"query": query, "variables": map[string]any{"filter": "books"}})
		if err := QueryBody(raw); err != nil {
			t.Errorf("valid supported query rejected: %s", query)
		}
	}
	for _, query := range []string{
		`mutation { buy }`, `subscription { feed }`, `query { a } mutation { buy }`,
		`query A { a } query B { b }`, `query { ...Hidden } fragment Hidden on X { buy }`,
		`query { a @include(if:true) }`, `query {}`, `query { a() }`, `query () { a }`,
		`query Q($a:Int=$b) { a }`, `query { a(x:0x10) }`, `query { a(x:01) }`,
		`query { a(x:-true) }`, `query { a(x:"unterminated) }`, `query { a(x: [1) }`,
		`query { a(x: {v: 1) }`, `query { a`, `query { a } trailing`,
		`query { a(x: """block""") }`, `query { ä }`,
		`query {` + strings.Repeat("a{", 14) + "b" + strings.Repeat("}", 15),
		`query {` + strings.Repeat("a ", 257) + `}`,
	} {
		raw, _ := json.Marshal(map[string]any{"query": query})
		if QueryBody(raw) == nil {
			t.Errorf("unsafe/unsupported document accepted: %s", query)
		}
	}
}

func TestQueryProfileEnvelope(t *testing.T) {
	for _, raw := range []string{
		`{"query":"{ a }","query":"mutation { buy }"}`,
		`{"query":"{ a }","extensions":{"persistedQuery":{}}}`,
		`[{"query":"{ a }"}]`, `{"query":"{ a }","variables":[]}`,
		`{"query":"query Q { a }","operationName":"Other"}`,
		`{"query":"{ a }","operationName":"Other"}`,
		`{"query":"{ a }","variables":{"x":1,"x":2}}`,
		`{"query":"{ a }"} {"query":"mutation { buy }"}`,
		`{"query":"{ a }","unknown":true}`, `null`, `{}`, "{\xff}",
	} {
		if QueryBody([]byte(raw)) == nil {
			t.Errorf("invalid envelope accepted: %s", raw)
		}
	}
	for _, raw := range []string{
		`{"query":"{ a }"}`, `{"query":"query Q { a }","operationName":"Q","variables":null}`,
	} {
		if err := QueryBody([]byte(raw)); err != nil {
			t.Error(err)
		}
	}
}

func FuzzQueryProfileTerminates(f *testing.F) {
	f.Add([]byte(`{"query":"{ a }"}`))
	f.Add([]byte(`{"query":"mutation { buy }"}`))
	f.Fuzz(func(t *testing.T, data []byte) { _ = QueryBody(data) })
}
