package app

import (
	"testing"

	"github.com/butler/butler/internal/memory/consolidation"
	"github.com/butler/butler/internal/memory/jobs"
)

func TestConsolidationReleaseGateCannotBeEnabledByOperatorFlag(t *testing.T) {
	t.Setenv("BUTLER_MEMORY_MODEL_JOBS_ENABLED", "true")
	t.Setenv("BUTLER_MEMORY_CONSOLIDATION_CONSUMER_ENABLED", "true")
	h := map[string]jobs.ModelHandler{}
	p, b := candidateQualifiedProfile()
	configureConsolidationHandler(h, p, b)
	if (h[consolidation.Version] != nil) != consolidation.ReleaseQualified || consolidationConsumerConfigured(&jobs.ModelWorker{Handlers: h}) != consolidation.ReleaseQualified {
		t.Fatal("qualified consumer and actual registry disagree")
	}
	if consolidationConsumerConfigured(nil) {
		t.Fatal("missing worker configured")
	}
}
