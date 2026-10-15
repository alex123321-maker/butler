package app

import (
	"testing"

	"github.com/butler/butler/internal/budget"
	"github.com/butler/butler/internal/memory/consolidation"
	"github.com/butler/butler/internal/memory/jobs"
	"github.com/butler/butler/internal/providerfactory"
)

func TestConsolidationReleaseGateCannotBeEnabledByOperatorFlag(t *testing.T) {
	if consolidation.ReleaseQualified {
		t.Skip("replace pending gate assertion with measured profile qualification when accepted")
	}
	t.Setenv("BUTLER_MEMORY_MODEL_JOBS_ENABLED", "true")
	t.Setenv("BUTLER_MEMORY_CONSOLIDATION_CONSUMER_ENABLED", "true")
	h := map[string]jobs.ModelHandler{}
	configureConsolidationHandler(h, providerfactory.Result{}, &budget.Controller{})
	if _, ok := h[consolidation.Version]; ok || consolidationConsumerConfigured(&jobs.ModelWorker{Handlers: h}) {
		t.Fatal("unqualified consumer registered")
	}
	if consolidationConsumerConfigured(nil) {
		t.Fatal("missing worker configured")
	}
}
