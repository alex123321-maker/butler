package app

import (
	"os"

	"github.com/butler/butler/internal/budget"
	"github.com/butler/butler/internal/memory/candidates"
	"github.com/butler/butler/internal/memory/jobs"
	"github.com/butler/butler/internal/providerfactory"
	"github.com/butler/butler/internal/transport/antigravity"
)

// Qualification is a reviewed code-owned decision, separate from operator flags
// and the measured profile check. Test/shadow harnesses instantiate the same
// handler explicitly while that release decision is pending.
const memoryCandidateReleaseQualified = false

func configureCandidateHandler(handlers map[string]jobs.ModelHandler, p providerfactory.Result, b *budget.Controller) {
	if memoryCandidateReleaseQualified && os.Getenv("BUTLER_MEMORY_MODEL_JOBS_ENABLED") == "true" && os.Getenv("BUTLER_MEMORY_MODEL_CANDIDATES_ENABLED") == "true" && candidateProfileReady(p, b) {
		handlers[candidates.Version] = candidates.Handler{}
	}
}

// Qualification belongs to the measured native profile, not every provider or
// ceiling that shares the same handler. The worker still clamps actual model
// calls to 300s and all current policy/claim/authority deadlines apply.
func candidateProfileReady(p providerfactory.Result, b *budget.Controller) bool {
	if p.Provider == nil || p.Provider.Name() != antigravity.Name || p.ModelName != antigravity.FlashHighModel || b == nil {
		return false
	}
	limits, ok := p.Provider.(interface{ InferenceLimits() (int64, int64) })
	if !ok {
		return false
	}
	_, output := limits.InferenceLimits()
	return output == 32768 && b.Policy.OutputTokens >= output && b.Policy.ModelMillis >= 300000
}
