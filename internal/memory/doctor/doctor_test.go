package doctor

import (
	"context"
	"testing"
)

func TestReporterWithoutDeps(t *testing.T) {
	t.Parallel()
	report, err := NewReporter(nil, nil).Report(context.Background())
	if err != nil {
		t.Fatalf("Report returned error: %v", err)
	}
	if report["queue"].(map[string]any)["healthy"] != false {
		t.Fatalf("expected unhealthy queue report, got %+v", report)
	}
}
