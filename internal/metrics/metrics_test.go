package metrics

import (
	"io"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestIncrCounterAppearsInMetricsOutput(t *testing.T) {
	registry := New()

	if err := registry.IncrCounter(MetricRequestsTotal, map[string]string{
		"service":   "orchestrator",
		"operation": "create_run",
		"status":    "ok",
	}); err != nil {
		t.Fatalf("IncrCounter returned error: %v", err)
	}

	req := httptest.NewRequest("GET", "/metrics", nil)
	rr := httptest.NewRecorder()
	registry.Handler().ServeHTTP(rr, req)

	body, err := io.ReadAll(rr.Result().Body)
	if err != nil {
		t.Fatalf("failed to read metrics body: %v", err)
	}

	output := string(body)
	if !strings.Contains(output, `butler_requests_total{operation="create_run",service="orchestrator",status="ok"} 1`) {
		t.Fatalf("expected counter output, got %q", output)
	}
	if rr.Code != 200 {
		t.Fatalf("expected 200 status, got %d", rr.Code)
	}
}

func TestObserveHistogramAndSetGauge(t *testing.T) {
	registry := New()

	if err := registry.ObserveHistogram(MetricRequestDurationSeconds, 0.42, map[string]string{
		"service":   "tool-broker",
		"operation": "execute_tool",
	}); err != nil {
		t.Fatalf("ObserveHistogram returned error: %v", err)
	}
	if err := registry.SetGauge("butler_active_runs", 3, map[string]string{
		"service": "orchestrator",
	}); err != nil {
		t.Fatalf("SetGauge returned error: %v", err)
	}

	req := httptest.NewRequest("GET", "/metrics", nil)
	rr := httptest.NewRecorder()
	registry.Handler().ServeHTTP(rr, req)
	output := rr.Body.String()

	if !strings.Contains(output, `butler_active_runs{service="orchestrator"} 3`) {
		t.Fatalf("expected gauge output, got %q", output)
	}
	if !strings.Contains(output, `butler_request_duration_seconds_bucket{operation="execute_tool",service="tool-broker",le="0.5"}`) {
		t.Fatalf("expected histogram bucket output, got %q", output)
	}
}

func TestRejectsMismatchedLabelsForExistingMetric(t *testing.T) {
	registry := New()

	if err := registry.IncrCounter(MetricRequestsTotal, map[string]string{
		"service":   "orchestrator",
		"operation": "create_run",
		"status":    "ok",
	}); err != nil {
		t.Fatalf("IncrCounter returned error: %v", err)
	}

	err := registry.IncrCounter(MetricRequestsTotal, map[string]string{
		"service": "orchestrator",
	})
	if err == nil {
		t.Fatal("expected mismatched label error")
	}
}
