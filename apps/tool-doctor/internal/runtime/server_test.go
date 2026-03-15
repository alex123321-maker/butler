package runtime

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/butler/butler/internal/config"
	runtimev1 "github.com/butler/butler/internal/gen/runtime/v1"
	toolbrokerv1 "github.com/butler/butler/internal/gen/toolbroker/v1"
	"github.com/butler/butler/internal/health"
)

func TestCheckerCheckSystem(t *testing.T) {
	t.Parallel()
	checker := NewChecker(config.ToolDoctorConfig{}, config.Snapshot{})
	checker.now = func() time.Time { return time.Date(2026, 3, 15, 12, 0, 0, 0, time.UTC) }
	checker.postgresCheck = func(context.Context) error { return nil }
	checker.redisCheck = func(context.Context) error { return errors.New("redis unavailable") }
	checker.providerCheck = func(context.Context) error { return nil }
	checker.cfg.Postgres.URL = "postgres://doctor@localhost/butler"
	checker.cfg.Redis.URL = "redis://localhost:6379/0"
	report := checker.CheckSystem(context.Background())
	if report.Status != health.StatusUnhealthy {
		t.Fatalf("expected unhealthy report, got %s", report.Status)
	}
	if len(report.Checks) != 3 {
		t.Fatalf("expected 3 checks, got %d", len(report.Checks))
	}
}

func TestCheckerCheckContainer(t *testing.T) {
	t.Parallel()
	checker := NewChecker(config.ToolDoctorConfig{ContainerTargets: []config.DoctorContainerTarget{{Name: "orchestrator", URL: "http://orchestrator:8080/health"}}}, config.Snapshot{})
	checker.containerCheck = func(_ context.Context, target config.DoctorContainerTarget) error {
		if target.Name != "orchestrator" {
			t.Fatalf("unexpected target %+v", target)
		}
		return nil
	}
	report := checker.CheckContainer(context.Background())
	if report.Status != health.StatusHealthy {
		t.Fatalf("expected healthy report, got %+v", report)
	}
}

func TestCheckerCheckProviderUsesHTTPProbe(t *testing.T) {
	t.Parallel()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if got := r.Header.Get("Authorization"); got != "Bearer sk-test" {
			t.Fatalf("unexpected authorization header %q", got)
		}
		if r.URL.Path != "/models" {
			t.Fatalf("unexpected path %q", r.URL.Path)
		}
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"data":[]}`))
	}))
	defer server.Close()
	checker := NewChecker(config.ToolDoctorConfig{OpenAIAPIKey: "sk-test", OpenAIBaseURL: server.URL}, config.Snapshot{})
	report := checker.CheckProvider(context.Background())
	if report.Status != health.StatusHealthy {
		t.Fatalf("expected healthy provider report, got %+v", report)
	}
}

func TestServerExecute(t *testing.T) {
	t.Parallel()
	server := NewServer(stubInspector{report: SystemReport{Status: health.StatusHealthy, Checks: []health.CheckResult{{Name: "config", Status: health.StatusHealthy}}}}, nil)
	resp, err := server.Execute(context.Background(), &runtimev1.ExecuteRequest{ToolCall: &toolbrokerv1.ToolCall{ToolCallId: "tool-1", RunId: "run-1", ToolName: "doctor.check_system"}})
	if err != nil {
		t.Fatalf("Execute returned error: %v", err)
	}
	if resp.GetResult().GetStatus() != "completed" {
		t.Fatalf("expected completed result, got %+v", resp.GetResult())
	}
	var payload SystemReport
	if err := json.Unmarshal([]byte(resp.GetResult().GetResultJson()), &payload); err != nil {
		t.Fatalf("decode result json: %v", err)
	}
	if payload.Status != health.StatusHealthy {
		t.Fatalf("unexpected payload status %s", payload.Status)
	}
}

func TestServerExecuteProviderCheck(t *testing.T) {
	t.Parallel()
	server := NewServer(stubInspector{provider: SystemReport{Status: health.StatusHealthy, Checks: []health.CheckResult{{Name: "provider.openai", Status: health.StatusHealthy}}}}, nil)
	resp, err := server.Execute(context.Background(), &runtimev1.ExecuteRequest{ToolCall: &toolbrokerv1.ToolCall{ToolCallId: "tool-2", RunId: "run-1", ToolName: "doctor.check_provider"}})
	if err != nil {
		t.Fatalf("Execute returned error: %v", err)
	}
	if resp.GetResult().GetStatus() != "completed" {
		t.Fatalf("expected completed result, got %+v", resp.GetResult())
	}
}

type stubInspector struct {
	report    SystemReport
	database  SystemReport
	container SystemReport
	provider  SystemReport
}

func (s stubInspector) CheckSystem(context.Context) SystemReport {
	return s.report
}

func (s stubInspector) CheckDatabase(context.Context) SystemReport {
	return s.database
}

func (s stubInspector) CheckContainer(context.Context) SystemReport {
	return s.container
}

func (s stubInspector) CheckProvider(context.Context) SystemReport {
	return s.provider
}
