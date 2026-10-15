package app

import (
	"github.com/butler/butler/internal/memory/candidates"
	"github.com/butler/butler/internal/memory/jobs"
	"testing"
)

func TestCandidateProductionReleaseCannotBeEnabledBeforeAcceptance(t *testing.T) {
	for _, flag := range []string{"", "true"} {
		t.Setenv("BUTLER_MEMORY_MODEL_CANDIDATES_ENABLED", flag)
		handlers := map[string]jobs.ModelHandler{}
		configureCandidateHandler(handlers)
		if !memoryCandidateReleaseQualified && handlers[candidates.Version] != nil {
			t.Fatal("unqualified consumer was enabled")
		}
	}
}
