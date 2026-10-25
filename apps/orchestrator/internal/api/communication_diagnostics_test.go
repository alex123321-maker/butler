package api

import (
	"context"
	"encoding/json"
	"errors"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/butler/butler/apps/orchestrator/internal/communication"
)

type diagnosticReporter struct {
	calls int
	err   error
}

func (r *diagnosticReporter) CommunicationReport(context.Context) (communication.DiagnosticReport, error) {
	r.calls++
	return communication.DiagnosticReport{Status: "degraded", FullV1Qualified: false}, r.err
}

func TestCommunicationDiagnosticOwnerAPI(t *testing.T) {
	token := strings.Repeat("t", 32)
	auth := NewOwnerControlAuth(token, "owner")
	reporter := &diagnosticReporter{}
	h := CommunicationDiagnostics(auth, reporter)
	for _, tc := range []struct {
		method, path, token string
		code                int
	}{
		{"GET", "/api/v3/communications/diagnostics", "", 401},
		{"POST", "/api/v3/communications/diagnostics", token, 405},
		{"GET", "/api/v3/communications/diagnostics?owner_id=foreign", token, 400},
		{"GET", "/api/v3/communications/diagnostics", token, 200},
	} {
		r := httptest.NewRequest(tc.method, tc.path, nil)
		if tc.token != "" {
			r.Header.Set("Authorization", "Bearer "+tc.token)
		}
		w := httptest.NewRecorder()
		h.ServeHTTP(w, r)
		if w.Code != tc.code || w.Header().Get("Cache-Control") != "no-store" {
			t.Fatal(w.Code, w.Body.String())
		}
	}
	if reporter.calls != 1 {
		t.Fatal("reporter called before owner/fixed read validation", reporter.calls)
	}
	reporter.err = errors.New("PRIVATE_SECRET_SQL")
	r := httptest.NewRequest("GET", "/api/v3/communications/diagnostics", nil)
	r.Header.Set("Authorization", "Bearer "+token)
	w := httptest.NewRecorder()
	h.ServeHTTP(w, r)
	if w.Code != 503 || strings.Contains(w.Body.String(), "PRIVATE_SECRET") {
		t.Fatal(w.Code, w.Body.String())
	}
}

func TestCommunicationDiagnosticDoctorCannotHideDeliveryFailure(t *testing.T) {
	s := NewDoctorServer(nil, &mockDoctorChecker{status: "healthy", reportJSON: json.RawMessage(`{"status":"healthy"}`)}, &mockMemoryReporter{report: map[string]any{"status": "healthy"}}, nil, nil)
	token := strings.Repeat("t", 32)
	s.SetOwnerControlAuth(NewOwnerControlAuth(token, "owner"))
	rpt := &diagnosticReporter{}
	s.SetCommunicationReporter(rpt)
	for _, failed := range []bool{false, true} {
		if failed {
			rpt.err = errors.New("PRIVATE_BODY")
		}
		r := httptest.NewRequest("POST", "/api/v1/doctor/check", nil)
		r.Header.Set("Authorization", "Bearer "+token)
		w := httptest.NewRecorder()
		s.HandleRunCheck().ServeHTTP(w, r)
		if w.Code != 200 || strings.Contains(w.Body.String(), "PRIVATE_BODY") || !strings.Contains(w.Body.String(), `"communication"`) {
			t.Fatal(w.Code, w.Body.String())
		}
		if !failed && !strings.Contains(w.Body.String(), `"status":"degraded"`) {
			t.Fatal("healthy components hid channel failure", w.Body.String())
		}
		if failed && !strings.Contains(w.Body.String(), `"status":"unknown"`) {
			t.Fatal("failed observation became healthy", w.Body.String())
		}
	}
}
