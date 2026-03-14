package health

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestServiceCheckAggregatesHealthyResults(t *testing.T) {
	svc := New(
		"orchestrator",
		FuncChecker{CheckName: "postgres", Fn: func(context.Context) error { return nil }},
		FuncChecker{CheckName: "redis", Fn: func(context.Context) error { return nil }},
	)
	svc.now = fixedClock()

	report := svc.Check(context.Background())
	if report.Status != StatusHealthy {
		t.Fatalf("expected healthy status, got %q", report.Status)
	}
	if report.Service != "orchestrator" {
		t.Fatalf("expected service name, got %q", report.Service)
	}
	if len(report.Checks) != 2 {
		t.Fatalf("expected 2 checks, got %d", len(report.Checks))
	}
	if report.Checks[0].Name != "postgres" || report.Checks[1].Name != "redis" {
		t.Fatalf("expected sorted check names, got %#v", report.Checks)
	}
}

func TestServiceCheckMarksUnhealthyResult(t *testing.T) {
	svc := New(
		"orchestrator",
		FuncChecker{CheckName: "postgres", Fn: func(context.Context) error { return errors.New("connection refused") }},
	)
	svc.now = fixedClock()

	report := svc.Check(context.Background())
	if report.Status != StatusUnhealthy {
		t.Fatalf("expected unhealthy status, got %q", report.Status)
	}
	if report.Checks[0].Message != "connection refused" {
		t.Fatalf("expected error message, got %q", report.Checks[0].Message)
	}
}

func TestHandlerReturnsJSONReport(t *testing.T) {
	svc := New(
		"tool-broker",
		FuncChecker{CheckName: "redis", Fn: func(context.Context) error { return nil }},
	)
	svc.now = fixedClock()

	req := httptest.NewRequest(http.MethodGet, "/health", nil)
	rr := httptest.NewRecorder()
	svc.Handler().ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200 response, got %d", rr.Code)
	}

	var report Report
	if err := json.Unmarshal(rr.Body.Bytes(), &report); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}
	if report.Status != StatusHealthy {
		t.Fatalf("expected healthy report, got %q", report.Status)
	}
	if report.Service != "tool-broker" {
		t.Fatalf("expected tool-broker service, got %q", report.Service)
	}
}

func TestHandlerReturns503ForUnhealthyReport(t *testing.T) {
	svc := New(
		"tool-broker",
		FuncChecker{CheckName: "postgres", Fn: func(context.Context) error { return errors.New("timeout") }},
	)
	svc.now = fixedClock()

	req := httptest.NewRequest(http.MethodGet, "/health", nil)
	rr := httptest.NewRecorder()
	svc.Handler().ServeHTTP(rr, req)

	if rr.Code != http.StatusServiceUnavailable {
		t.Fatalf("expected 503 response, got %d", rr.Code)
	}
}

func fixedClock() func() time.Time {
	values := []time.Time{
		time.Date(2026, time.March, 15, 10, 0, 0, 0, time.UTC),
		time.Date(2026, time.March, 15, 10, 0, 0, 0, time.UTC),
		time.Date(2026, time.March, 15, 10, 0, 1, 0, time.UTC),
		time.Date(2026, time.March, 15, 10, 0, 1, 0, time.UTC),
		time.Date(2026, time.March, 15, 10, 0, 2, 0, time.UTC),
		time.Date(2026, time.March, 15, 10, 0, 2, 0, time.UTC),
	}
	index := 0
	return func() time.Time {
		if index >= len(values) {
			return values[len(values)-1]
		}
		value := values[index]
		index++
		return value
	}
}
