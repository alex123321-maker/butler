package api

import (
	"context"
	"encoding/json"
	"errors"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestDoctorToolEnvelopeCannotHideErrors(t *testing.T) {
	for _, tc := range []struct{ raw, status, reason string }{
		{`{"status":"completed","result":{"status":"healthy"},"error":{"message":"PRIVATE_SECRET"}}`, "unhealthy", "tool_failed"},
		{`{"status":"failed","result":{"status":"healthy"}}`, "unhealthy", "tool_failed"},
		{`{"status":"denied","result":{"status":"healthy"}}`, "degraded", "authorization_denied"},
		{`{"status":"completed"}`, "unknown", "missing_result"},
		{`null`, "unknown", "tool_status"},
		{`{"status":"completed","result":null}`, "unknown", "missing_result"},
	} {
		t.Run(tc.reason+tc.raw, func(t *testing.T) {
			c := NewToolBrokerDoctorChecker(func(context.Context, string, string) (string, error) { return tc.raw, nil })
			s, b, e := c.RunCheck(context.Background())
			if e != nil || s != tc.status || !strings.Contains(string(b), tc.reason) || strings.Contains(string(b), "PRIVATE_SECRET") {
				t.Fatal(s, string(b), e)
			}
		})
	}
}
func TestDoctorMemoryReporterFailurePreservesToolReport(t *testing.T) {
	s := NewDoctorServer(nil, &mockDoctorChecker{status: "healthy", reportJSON: json.RawMessage(`{"status":"healthy","checks":[{"name":"postgres","status":"healthy"}]}`)}, &mockMemoryReporter{err: errors.New("PRIVATE_SECRET")}, nil, nil)
	s.SetOwnerControlAuth(NewOwnerControlAuth(strings.Repeat("t", 32), "owner"))
	w := httptest.NewRecorder()
	r := httptest.NewRequest("POST", "/api/v1/doctor/check", nil)
	r.Header.Set("Authorization", "Bearer "+strings.Repeat("t", 32))
	s.HandleRunCheck().ServeHTTP(w, r)
	b := w.Body.String()
	if w.Code != 200 || !strings.Contains(b, "reporter_failed") || !strings.Contains(b, "postgres") || strings.Contains(b, "PRIVATE_SECRET") || !strings.Contains(b, `"status":"unknown"`) {
		t.Fatal(b)
	}
}
func TestDoctorOwnerProofRequiredBeforeExecution(t *testing.T) {
	s := NewDoctorServer(nil, nil, nil, nil, nil)
	for _, path := range []string{"check", "reports"} {
		w := httptest.NewRecorder()
		r := httptest.NewRequest("POST", "/api/v1/doctor/"+path, nil)
		h := s.HandleRunCheck()
		if path == "reports" {
			r.Method = "GET"
			h = s.HandleListReports()
		}
		h.ServeHTTP(w, r)
		if w.Code != 401 {
			t.Fatal(w.Code)
		}
	}
}

func TestDoctorOldObservationCannotBecomeCurrentHealthy(t *testing.T) {
	c := NewToolBrokerDoctorChecker(func(context.Context, string, string) (string, error) {
		return `{"status":"completed","result":{"status":"healthy","checked_at":"2020-01-01T00:00:00Z","checks":[{"name":"postgres","status":"healthy"}]}}`, nil
	})
	s, raw, err := c.RunCheck(context.Background())
	if err != nil || s != "unknown" || !strings.Contains(string(raw), "observation_stale_or_missing") {
		t.Fatal(s, string(raw), err)
	}
}
