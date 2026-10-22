package app

import (
	"bufio"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/butler/butler/internal/metrics"
)

func TestCommunicationHTTPStreamingFlushIntegration(t *testing.T) {
	server := httptest.NewServer(instrumentHTTP("orchestrator", metrics.New(), corsMiddleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		flusher, ok := w.(http.Flusher)
		if !ok {
			t.Error("instrumentation removed SSE flush")
			w.WriteHeader(500)
			return
		}
		w.Header().Set("Content-Type", "text/event-stream")
		_, _ = w.Write([]byte("event: refresh\ndata: {}\n\n"))
		flusher.Flush()
		<-r.Context().Done()
	}))))
	defer server.Close()
	server.Client().Timeout = 3 * time.Second
	response, err := server.Client().Get(server.URL)
	if err != nil {
		t.Fatal(err)
	}
	defer response.Body.Close()
	if response.StatusCode != 200 {
		t.Fatal(response.StatusCode)
	}
	line, err := bufio.NewReader(response.Body).ReadString('\n')
	if err != nil || line != "event: refresh\n" {
		t.Fatal("live stream not flushed", line, err)
	}
}

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

func TestCORSMiddlewareHandlesSettingsPreflight(t *testing.T) {
	handler := corsMiddleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	req := httptest.NewRequest(http.MethodOptions, "/api/v1/settings/BUTLER_TELEGRAM_BOT_TOKEN", nil)
	req.Header.Set("Access-Control-Request-Method", http.MethodPut)
	RR := httptest.NewRecorder()
	handler.ServeHTTP(RR, req)

	if RR.Code != http.StatusNoContent {
		t.Fatalf("expected status 204, got %d", RR.Code)
	}
	if got := RR.Header().Get("Access-Control-Allow-Methods"); got != "GET, POST, PUT, DELETE, OPTIONS" {
		t.Fatalf("unexpected allowed methods header: %q", got)
	}
	if got := RR.Header().Get("Access-Control-Allow-Headers"); got != "Content-Type, Authorization, X-Butler-Delivery-ID, Last-Event-ID" {
		t.Fatalf("unexpected allowed headers: %q", got)
	}
	if got := RR.Header().Get("Access-Control-Allow-Origin"); got != "*" {
		t.Fatalf("unexpected allowed origin: %q", got)
	}
}

func TestNormalizeSingleTabTransportMode(t *testing.T) {
	t.Parallel()

	if got := normalizeSingleTabTransportMode("native_only"); got != "native_only" {
		t.Fatalf("expected native_only, got %q", got)
	}
	if got := normalizeSingleTabTransportMode("remote_preferred"); got != "remote_preferred" {
		t.Fatalf("expected remote_preferred, got %q", got)
	}
	if got := normalizeSingleTabTransportMode("DUAL"); got != "dual" {
		t.Fatalf("expected dual, got %q", got)
	}
	if got := normalizeSingleTabTransportMode(""); got != "dual" {
		t.Fatalf("expected dual default, got %q", got)
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
