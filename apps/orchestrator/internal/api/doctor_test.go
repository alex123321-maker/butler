package api

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

type mockDoctorChecker struct {
	status     string
	reportJSON json.RawMessage
	err        error
}

func (m *mockDoctorChecker) RunCheck(_ context.Context) (string, json.RawMessage, error) {
	return m.status, m.reportJSON, m.err
}

type mockMemoryReporter struct {
	report map[string]any
	err    error
}

func (m *mockMemoryReporter) Report(context.Context) (map[string]any, error) {
	return m.report, m.err
}

func TestDoctorHandleRunCheck_MethodNotAllowed(t *testing.T) {
	srv := NewDoctorServer(nil, &mockDoctorChecker{}, nil, nil)
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/doctor/check", nil)
	srv.HandleRunCheck().ServeHTTP(rec, req)
	if rec.Code != http.StatusMethodNotAllowed {
		t.Fatalf("expected 405, got %d", rec.Code)
	}
}

func TestDoctorHandleRunCheck_CheckerError(t *testing.T) {
	checker := &mockDoctorChecker{err: errTestFail}
	srv := NewDoctorServer(nil, checker, nil, nil)
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/v1/doctor/check", nil)
	srv.HandleRunCheck().ServeHTTP(rec, req)
	if rec.Code != http.StatusInternalServerError {
		t.Fatalf("expected 500, got %d", rec.Code)
	}
}

func TestDoctorHandleRunCheck_Success_NoPool(t *testing.T) {
	// When pool is nil, the store fails but we still return the report
	report := json.RawMessage(`{"status":"healthy","checks":[]}`)
	checker := &mockDoctorChecker{status: "healthy", reportJSON: report}
	srv := NewDoctorServer(nil, checker, &mockMemoryReporter{report: map[string]any{"queue": map[string]any{"healthy": true, "depth": float64(0)}}}, nil)
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/v1/doctor/check", nil)
	srv.HandleRunCheck().ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec.Code)
	}

	var resp map[string]any
	if err := json.NewDecoder(rec.Body).Decode(&resp); err != nil {
		t.Fatalf("decode body: %v", err)
	}
	if resp["status"] != "healthy" {
		t.Errorf("expected status healthy, got %v", resp["status"])
	}
	if resp["stored"] != false {
		t.Errorf("expected stored=false, got %v", resp["stored"])
	}
	reportBody := resp["report"].(map[string]any)
	if _, ok := reportBody["memory"]; !ok {
		t.Fatalf("expected memory doctor data, got %+v", reportBody)
	}
}

func TestDoctorHandleListReports_MethodNotAllowed(t *testing.T) {
	srv := NewDoctorServer(nil, &mockDoctorChecker{}, nil, nil)
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/v1/doctor/reports", nil)
	srv.HandleListReports().ServeHTTP(rec, req)
	if rec.Code != http.StatusMethodNotAllowed {
		t.Fatalf("expected 405, got %d", rec.Code)
	}
}

func TestDoctorHandleListReports_NoPool(t *testing.T) {
	srv := NewDoctorServer(nil, &mockDoctorChecker{}, nil, nil)
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/doctor/reports", nil)
	srv.HandleListReports().ServeHTTP(rec, req)
	// With nil pool, we return empty list
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec.Code)
	}
	var resp map[string]any
	if err := json.NewDecoder(rec.Body).Decode(&resp); err != nil {
		t.Fatalf("decode: %v", err)
	}
	reports, ok := resp["reports"].([]any)
	if !ok {
		t.Fatal("expected reports array")
	}
	if len(reports) != 0 {
		t.Errorf("expected empty reports, got %d", len(reports))
	}
}

func TestToolBrokerDoctorChecker_Success(t *testing.T) {
	resultJSON := `{"status":"healthy","checks":[{"name":"config","status":"healthy"}]}`
	checker := NewToolBrokerDoctorChecker(func(_ context.Context, toolName, _ string) (string, error) {
		if toolName != "doctor.check_system" {
			t.Errorf("unexpected tool name: %s", toolName)
		}
		return resultJSON, nil
	})

	status, report, err := checker.RunCheck(context.Background())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if status != "healthy" {
		t.Errorf("expected healthy, got %s", status)
	}
	if string(report) != resultJSON {
		t.Errorf("report mismatch: %s", string(report))
	}
}

func TestToolBrokerDoctorChecker_Error(t *testing.T) {
	checker := NewToolBrokerDoctorChecker(func(_ context.Context, _, _ string) (string, error) {
		return "", errTestFail
	})

	status, _, err := checker.RunCheck(context.Background())
	if err == nil {
		t.Fatal("expected error")
	}
	if status != "unhealthy" {
		t.Errorf("expected unhealthy on error, got %s", status)
	}
}

func TestToolBrokerDoctorChecker_InvalidJSON(t *testing.T) {
	checker := NewToolBrokerDoctorChecker(func(_ context.Context, _, _ string) (string, error) {
		return "not json", nil
	})

	status, _, err := checker.RunCheck(context.Background())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if status != "unknown" {
		t.Errorf("expected unknown for invalid JSON, got %s", status)
	}
}

var errTestFail = json.Unmarshal([]byte("invalid"), nil)

func init() {
	errTestFail = http.ErrAbortHandler // use a known error for testing
}
