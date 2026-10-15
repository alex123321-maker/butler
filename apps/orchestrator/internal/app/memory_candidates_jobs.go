package app

import (
	"os"

	"github.com/butler/butler/internal/memory/candidates"
	"github.com/butler/butler/internal/memory/jobs"
)

// This release has not yet passed independent Russian semantic quality and the
// composed lifecycle attack gate. An operator flag cannot promote an unqualified
// consumer. Test/shadow harnesses instantiate the same handler explicitly.
const memoryCandidateReleaseQualified = false

func configureCandidateHandler(handlers map[string]jobs.ModelHandler) {
	if memoryCandidateReleaseQualified && os.Getenv("BUTLER_MEMORY_MODEL_CANDIDATES_ENABLED") == "true" {
		handlers[candidates.Version] = candidates.Handler{}
	}
}
