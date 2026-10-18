package app

import (
	"testing"

	"github.com/butler/butler/internal/config"
	"github.com/butler/butler/internal/memory/candidates"
	"github.com/butler/butler/internal/memory/compaction"
	"github.com/butler/butler/internal/memory/jobs"
)

func TestMemoryDoctorUsesActualHandlerAndMeasuredProfile(t *testing.T) {
	t.Setenv("BUTLER_MEMORY_MODEL_JOBS_ENABLED", "true")
	t.Setenv("BUTLER_MEMORY_MODEL_CANDIDATES_ENABLED", "true")
	t.Setenv("BUTLER_MEMORY_SUMMARY_CONSUMER_ENABLED", "true")
	p, b := candidateQualifiedProfile()
	w := &jobs.ModelWorker{Handlers: map[string]jobs.ModelHandler{candidates.Version: candidates.Handler{}}}
	states := memoryFeatureConfigurations(config.OrchestratorConfig{MemoryReadEnabled: true, MemoryAdaptiveRetrievalEnabled: true}, p, b, w, false, true, false, false)
	for _, s := range states {
		switch s.Name {
		case "candidate_jobs":
			if !s.Wired || !s.Requested || !s.ProfileReady || s.Qualified != memoryCandidateReleaseQualified {
				t.Fatal(s)
			}
		case "task_compaction":
			if s.Wired || !s.Requested || !s.ProfileReady || s.Qualified != compaction.ReleaseQualified {
				t.Fatal(s)
			}
		case "adaptive_retrieval":
			if s.Wired || !s.Requested || s.Qualified != adaptiveRetrievalQualified {
				t.Fatal(s)
			}
		case "source_reads":
			if !s.Wired || !s.Requested {
				t.Fatal(s)
			}
		}
	}
	b.Policy.ModelMillis = 299999
	for _, s := range memoryFeatureConfigurations(config.OrchestratorConfig{}, p, b, w, false, false, false, false) {
		if (s.Name == "candidate_jobs" || s.Name == "task_compaction" || s.Name == "temporal_facts") && s.ProfileReady {
			t.Fatal("unmeasured timeout appeared supported", s)
		}
	}
}
