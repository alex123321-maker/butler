package runtime

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	runtimev1 "github.com/butler/butler/internal/gen/runtime/v1"
	toolbrokerv1 "github.com/butler/butler/internal/gen/toolbroker/v1"
	"github.com/butler/butler/internal/websearch"
)

type searchTransport func(*http.Request) (*http.Response, error)

func (f searchTransport) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }

func TestSearchSeparatesDiscoveryFromVerifiedSources(t *testing.T) {
	s := NewServer(nil, nil)
	s.searchClient = &http.Client{Transport: searchTransport(func(r *http.Request) (*http.Response, error) {
		if r.Method != "GET" || r.URL.Host != "lite.duckduckgo.com" || r.URL.Query().Get("q") != "новый сезон & время" || r.Header.Get("Authorization") != "" || r.Header.Get("Cookie") != "" || r.Header.Get("User-Agent") == "" {
			t.Fatalf("wrong query request %+v", r)
		}
		w := httptest.NewRecorder()
		w.Header().Set("Content-Type", "text/html")
		w.WriteString(`<table><tr><td><a class="result-link" href="//duckduckgo.com/l/?uddg=https%3A%2F%2Fexample.com%2Fnews">Official &amp; current</a></td></tr><tr><td class="result-snippet">A schedule <b>update</b></td></tr></table><a class="result-link" href="http://127.0.0.1/private">private</a>`)
		return w.Result(), nil
	})}
	r, err := s.Execute(context.Background(), &runtimev1.ExecuteRequest{ToolCall: &toolbrokerv1.ToolCall{ToolName: "web.search", ArgsJson: `{"query":"новый сезон & время","limit":5}`}})
	if err != nil || r.Result.Status != "completed" || !strings.Contains(r.Result.ResultJson, `https://example.com/news`) || !strings.Contains(r.Result.ResultJson, `A schedule update`) || strings.Contains(r.Result.ResultJson, `127.0.0.1`) {
		t.Fatalf("%+v %v", r, err)
	}
}
func TestSearchChallengeAndUnknownMarkupCannotBecomeEmptySuccess(t *testing.T) {
	for _, body := range []string{`<form id="anomaly-form">challenge</form>`, `<html>changed markup</html>`} {
		if _, err := parseSearch(body, 5); err == nil {
			t.Fatal("unrecognized page accepted")
		}
	}
	items, err := parseSearch(`<div class="no-results">No results</div>`, 5)
	if err != nil || items == nil || len(items) != 0 {
		t.Fatal(items, err)
	}
	for _, q := range []string{"", "secret\nmore", strings.Repeat("я", 513)} {
		if _, err := websearch.URL(q); err == nil {
			t.Fatal("invalid query")
		}
	}
}
