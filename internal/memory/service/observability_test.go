package service

import (
	"context"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/butler/butler/internal/memory/catalog"
	"github.com/butler/butler/internal/metrics"
)

type observedReader struct {
	result SearchResult
	err    error
}

func (r observedReader) Search(context.Context, []Scope, string, int) ([]SemanticHit, error) {
	return r.result.Hits, r.err
}
func (r observedReader) SearchWithPolicy(context.Context, []Scope, string, int, QueryPolicy) (SearchResult, error) {
	return r.result, r.err
}
func scrape(t *testing.T, m *metrics.Registry) string {
	t.Helper()
	w := httptest.NewRecorder()
	m.Handler().ServeHTTP(w, httptest.NewRequest("GET", "/metrics", nil))
	return w.Body.String()
}
func TestOrdinaryBundleMetricsAndPrivateCardinality(t *testing.T) {
	m := metrics.New()
	r := observedReader{result: SearchResult{Hits: []SemanticHit{{Title: "PRIVATE_CONTENT", Summary: "PRIVATE_CONTENT", SourceRef: "PRIVATE_SOURCE"}}, Diagnostics: []string{"semantic_index_unavailable", "PRIVATE_DIAGNOSTIC"}}}
	s := New(Config{Metrics: m, Semantic: r, ContextBudgetTokens: 256})
	_, err := s.BuildBundle(context.Background(), BundleRequest{SessionKey: "PRIVATE_SESSION", UserID: "PRIVATE_OWNER", UserMessage: "PRIVATE_QUERY", QueryPolicy: &QueryPolicy{Local: true}})
	if err != nil {
		t.Fatal(err)
	}
	out := scrape(t, m)
	for _, want := range []string{`butler_memory_operations_total{outcome="success",stage="document_search"} 1`, `butler_memory_operations_total{outcome="success",stage="bundle"} 1`, `reason="semantic_index_unavailable"`, `reason="context_budget"`, `reason="unknown"`, `butler_memory_operation_duration_seconds_count{stage="context"} 1`} {
		if !strings.Contains(out, want) {
			t.Fatalf("missing %s in metrics", want)
		}
	}
	if strings.Contains(out, "PRIVATE_") {
		t.Fatal("private labels leaked")
	}
}
func TestFailedAndCancelledBundleCompletesOnce(t *testing.T) {
	for _, tc := range []struct {
		err     error
		outcome string
	}{{catalog.ErrDenied, "denied"}, {catalog.ErrStale, "denied"}, {context.Canceled, "cancelled"}} {
		m := metrics.New()
		s := New(Config{Metrics: m, ScopeResolver: func(context.Context) ([]Scope, error) { return nil, tc.err }})
		_, _ = s.BuildBundle(context.Background(), BundleRequest{})
		out := scrape(t, m)
		if !strings.Contains(out, `butler_memory_operations_total{outcome="`+tc.outcome+`",stage="bundle"} 1`) || !strings.Contains(out, `butler_memory_operation_duration_seconds_count{stage="search"} 1`) {
			t.Fatal(out)
		}
	}
}
