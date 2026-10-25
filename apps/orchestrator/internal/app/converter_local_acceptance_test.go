package app

import (
	"regexp"
	"strings"
	"testing"
	"time"

	"github.com/butler/butler/internal/generatedtools"
)

// Keep local failure diagnostics in the existing fixed safe vocabulary.
type converterLocalDiagnosticWriter struct{ t *testing.T }

func (w converterLocalDiagnosticWriter) Write(b []byte) (int, error) {
	s := string(b)
	if strings.Contains(s, `msg="execution authority observation stopped"`) {
		phase := regexp.MustCompile(`phase=(lease|task_authority)\b`).FindString(s)
		class := regexp.MustCompile(`classification=(changed|unavailable|deadline|stale)\b`).FindString(s)
		elapsed := regexp.MustCompile(`elapsed_ms=[0-9]+\b`).FindString(s)
		w.t.Log("local authority diagnostic", phase, class, elapsed)
	}
	return len(b), nil
}

func TestConverterDeveloperBoundedLocalContainerE2E(t *testing.T) {
	t.Setenv("BUTLER_CONVERTER_LOCAL_DIAGNOSTICS", "1")
	testConverterDeveloperWorkersObserved(t, true, &converterWorkerFaultHooks{factory: func(t *testing.T, p generatedtools.Profile, path string) *developerFixture {
		f := newDeveloperWorkerFixtureWithLifetime(t, p, path, true, 3*time.Minute)
		// Fixture wall-time only: production ownership, source, model and tool
		// proof bounds and finite action budgets remain unchanged.
		f.workerWaitTimeout = 90 * time.Second
		return f
	}})
}
