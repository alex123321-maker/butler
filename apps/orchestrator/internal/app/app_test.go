package app

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/butler/butler/internal/metrics"
)

func TestInstrumentHTTPRecordsMetrics(t *testing.T) {
	registry := metrics.New()
	handler := instrumentHTTP("orchestrator", registry, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusAccepted)
	}))

	req := httptest.NewRequest(http.MethodGet, "/health", nil)
	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)

	if rr.Code != http.StatusAccepted {
		t.Fatalf("expected status 202, got %d", rr.Code)
	}

	metricsReq := httptest.NewRequest(http.MethodGet, "/metrics", nil)
	metricsRR := httptest.NewRecorder()
	registry.Handler().ServeHTTP(metricsRR, metricsReq)
	output := metricsRR.Body.String()

	if !contains(output, `butler_requests_total{operation="GET /health",service="orchestrator",status="ok"} 1`) {
		t.Fatalf("expected request metric, got %q", output)
	}
	if !contains(output, `butler_request_duration_seconds_bucket{operation="GET /health",service="orchestrator",le="0.005"}`) {
		t.Fatalf("expected duration metric, got %q", output)
	}
}

func TestInstrumentHTTPRecordsErrors(t *testing.T) {
	registry := metrics.New()
	handler := instrumentHTTP("orchestrator", registry, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusBadGateway)
	}))

	handler.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest(http.MethodGet, "/api/v1/events", nil))

	metricsRR := httptest.NewRecorder()
	registry.Handler().ServeHTTP(metricsRR, httptest.NewRequest(http.MethodGet, "/metrics", nil))
	output := metricsRR.Body.String()

	if !contains(output, `butler_errors_total{error_class="http_error",operation="GET /api/v1/events",service="orchestrator"} 1`) {
		t.Fatalf("expected error metric, got %q", output)
	}
	if !contains(output, `butler_requests_total{operation="GET /api/v1/events",service="orchestrator",status="error"} 1`) {
		t.Fatalf("expected request error metric, got %q", output)
	}
}

func contains(haystack, needle string) bool {
	return len(haystack) >= len(needle) && (haystack == needle || len(haystack) > len(needle) && (stringIndex(haystack, needle) >= 0))
}

func stringIndex(haystack, needle string) int {
	for i := 0; i+len(needle) <= len(haystack); i++ {
		if haystack[i:i+len(needle)] == needle {
			return i
		}
	}
	return -1
}
