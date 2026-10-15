package app

import (
	"os"

	"github.com/butler/butler/internal/budget"
	"github.com/butler/butler/internal/config"
	"github.com/butler/butler/internal/memory/candidates"
	"github.com/butler/butler/internal/memory/compaction"
	memorydoctor "github.com/butler/butler/internal/memory/doctor"
	"github.com/butler/butler/internal/memory/jobs"
	"github.com/butler/butler/internal/providerfactory"
)

// Capture actual startup composition, independently of flags. Durable controls
// and owner policies are queried afresh by Doctor; no cached permission is used.
func memoryFeatureConfigurations(cfg config.OrchestratorConfig, p providerfactory.Result, b *budget.Controller, w *jobs.ModelWorker, proposals, reads, adaptive bool) []memorydoctor.FeatureConfiguration {
	jobsRequested := os.Getenv("BUTLER_MEMORY_MODEL_JOBS_ENABLED") == "true"
	return []memorydoctor.FeatureConfiguration{
		{Name: "source_reads", Requested: cfg.MemoryReadEnabled, Wired: reads, Qualified: true, ProfileReady: true},
		{Name: "candidate_proposals", Requested: cfg.MemoryCandidateEnabled, Wired: proposals, Qualified: true, ProfileReady: true},
		{Name: "adaptive_retrieval", Requested: cfg.MemoryAdaptiveRetrievalEnabled, Wired: adaptive, Qualified: adaptiveRetrievalQualified, ProfileReady: true},
		{Name: "candidate_jobs", Requested: jobsRequested && os.Getenv("BUTLER_MEMORY_MODEL_CANDIDATES_ENABLED") == "true", Wired: modelHandlerConfigured(w, candidates.Version), Qualified: memoryCandidateReleaseQualified, ProfileReady: candidateProfileReady(p, b), Handler: candidates.Version},
		{Name: "task_compaction", Requested: jobsRequested && os.Getenv("BUTLER_MEMORY_SUMMARY_CONSUMER_ENABLED") == "true", Wired: modelHandlerConfigured(w, compaction.HandlerName), Qualified: compaction.ReleaseQualified, ProfileReady: summaryProfileReady(p, b), Handler: compaction.HandlerName},
	}
}

func modelHandlerConfigured(w *jobs.ModelWorker, name string) bool {
	return w != nil && w.Handlers[name] != nil
}
