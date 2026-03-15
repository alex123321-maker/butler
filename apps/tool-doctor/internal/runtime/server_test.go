package runtime

import (
	"context"
	"encoding/json"
	"errors"
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

type stubInspector struct {
	report SystemReport
}

func (s stubInspector) CheckSystem(context.Context) SystemReport {
	return s.report
}
