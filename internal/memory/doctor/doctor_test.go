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
	// Pipeline and embedding should default to disabled/unconfigured.
	if report["pipeline_worker"].(map[string]any)["enabled"] != false {
		t.Fatalf("expected pipeline_worker disabled by default, got %+v", report)
	}
	if report["embedding_provider"].(map[string]any)["configured"] != false {
		t.Fatalf("expected embedding_provider unconfigured by default, got %+v", report)
	}
}

func TestReporterWithPipelineAndEmbeddingEnabled(t *testing.T) {
	t.Parallel()
	reporter := NewReporter(nil, nil)
	reporter.SetPipelineEnabled(true)
	reporter.SetEmbeddingConfigured(true)
	report, err := reporter.Report(context.Background())
	if err != nil {
		t.Fatalf("Report returned error: %v", err)
	}
	pw := report["pipeline_worker"].(map[string]any)
	if pw["enabled"] != true || pw["healthy"] != true {
		t.Fatalf("expected pipeline_worker enabled and healthy, got %+v", pw)
	}
	ep := report["embedding_provider"].(map[string]any)
	if ep["configured"] != true || ep["healthy"] != true {
		t.Fatalf("expected embedding_provider configured and healthy, got %+v", ep)
	}
}
