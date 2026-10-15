package doctor

import (
	"context"
	"errors"
	"github.com/butler/butler/internal/memory/diagnostics"
	"testing"
)

func TestReporterWithoutDeps(t *testing.T) {
	t.Parallel()
	report, err := NewReporter(nil, nil).Report(context.Background())
	if err != nil {
		t.Fatalf("Report returned error: %v", err)
	}
	if report["legacy_queue"].(map[string]any)["status"] != "unknown" || report["status"] != "unknown" {
		t.Fatalf("missing observations must be unknown: %+v", report)
	}
	// Pipeline and embedding should default to disabled/unconfigured.
	if report["components"].(map[string]diagnostics.Component)["legacy_pipeline"].Status != "disabled" {
		t.Fatalf("expected pipeline_worker disabled by default, got %+v", report)
	}
	if report["components"].(map[string]diagnostics.Component)["legacy_embedding"].Configured != false {
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
	pw := report["components"].(map[string]diagnostics.Component)["legacy_pipeline"]
	if !pw.Effective || pw.Status != "unknown" {
		t.Fatalf("configuration cannot prove worker health: %+v", pw)
	}
	ep := report["components"].(map[string]diagnostics.Component)["legacy_embedding"]
	if !ep.Configured || ep.Status != "unknown" {
		t.Fatalf("configuration cannot prove inference: %+v", ep)
	}
}

type failingQueue struct{}

func (failingQueue) Depth(context.Context) (int64, error) { return 0, errors.New("PRIVATE_SECRET") }
func TestQueueFailurePreservesIndependentComponents(t *testing.T) {
	r := NewReporter(nil, nil)
	r.queue = failingQueue{}
	r.Configure(Options{SemanticConfigured: true, SemanticEffective: true})
	got, err := r.Report(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	q := got["legacy_queue"].(map[string]any)
	if _, ok := q["depth"]; ok {
		t.Fatal("failed queue became zero")
	}
	c := got["components"].(map[string]diagnostics.Component)
	if !c["semantic_retrieval"].Effective || c["legacy_pipeline"].Status != "disabled" {
		t.Fatal(c)
	}
	if got["answer_quality"].(map[string]any)["status"] != "unverified" {
		t.Fatal("invented quality")
	}
}

func TestRequestedUnavailableSemanticAndConfigurationQuality(t *testing.T) {
	r := NewReporter(nil, nil)
	first, _ := r.Report(context.Background())
	r.Configure(Options{SemanticConfigured: true})
	second, _ := r.Report(context.Background())
	c := second["components"].(map[string]diagnostics.Component)["semantic_retrieval"]
	if c.Status != "degraded" || !c.Configured || c.Effective {
		t.Fatal(c)
	}
	a, b := first["answer_quality"].(map[string]any), second["answer_quality"].(map[string]any)
	if a["configuration"] == b["configuration"] || a["status"] != "unverified" || b["status"] != "unverified" {
		t.Fatal("configuration change became accepted quality", a, b)
	}
}
