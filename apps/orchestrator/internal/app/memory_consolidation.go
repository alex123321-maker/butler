package app

import (
	"os"

	apiservice "github.com/butler/butler/apps/orchestrator/internal/api"
	"github.com/butler/butler/internal/budget"
	"github.com/butler/butler/internal/memory/consolidation"
	"github.com/butler/butler/internal/memory/jobs"
	"github.com/butler/butler/internal/providerfactory"
)

func configureConsolidationHandler(handlers map[string]jobs.ModelHandler, p providerfactory.Result, b *budget.Controller) {
	if consolidation.ReleaseQualified && os.Getenv("BUTLER_MEMORY_MODEL_JOBS_ENABLED") == "true" && os.Getenv("BUTLER_MEMORY_CONSOLIDATION_CONSUMER_ENABLED") == "true" && summaryProfileReady(p, b) {
		handlers[consolidation.Version] = consolidation.Handler{}
	}
}

func consolidationConsumerConfigured(w *jobs.ModelWorker) bool {
	if w == nil {
		return false
	}
	_, ok := w.Handlers[consolidation.Version]
	return ok
}

func configureConsolidationOwnerAPI(server *apiservice.MemoryCatalogServer, configured bool) {
	if server.Store == nil || server.ModelJobs == nil {
		return
	}
	server.Consolidation = &consolidation.Store{Catalog: server.Store, Jobs: server.ModelJobs}
	server.ConsolidationReady = configured
}
