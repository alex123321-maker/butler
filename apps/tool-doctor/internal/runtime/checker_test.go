package runtime

import (
	"context"
	"testing"

	"github.com/butler/butler/internal/config"
	"github.com/butler/butler/internal/health"
)

func TestCheckSystemIncludesConfigSourceMetadata(t *testing.T) {
	checker := NewChecker(config.ToolDoctorConfig{}, config.Snapshot{})
	checker.snapshot = snapshotStub{keys: []config.ConfigKeyInfo{
		{Key: "BUTLER_LOG_LEVEL", Source: "db", ValidationStatus: config.ValidationStatusValid},
		{Key: "BUTLER_OPENAI_API_KEY", Source: "env", ValidationStatus: config.ValidationStatusValid, IsSecret: true},
	}}
	checker.postgresCheck = func(context.Context) error { return nil }
	checker.redisCheck = func(context.Context) error { return nil }

	report := checker.CheckSystem(context.Background())
	if len(report.Config) != 2 {
		t.Fatalf("expected config entries in report, got %+v", report.Config)
	}
	if report.Config[0].Source != "db" || report.Config[1].Source != "env" {
		t.Fatalf("expected source metadata in report, got %+v", report.Config)
	}
	if report.Status != health.StatusDegraded {
		t.Fatalf("expected degraded status because external checks are not configured, got %q", report.Status)
	}
}

type snapshotStub struct{ keys []config.ConfigKeyInfo }

func (s snapshotStub) ListKeys() []config.ConfigKeyInfo {
	return append([]config.ConfigKeyInfo(nil), s.keys...)
}
