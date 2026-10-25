package app

import (
	"context"
	"regexp"
	"strings"
	"testing"
	"time"

	"github.com/butler/butler/internal/generatedtools"
	"github.com/butler/butler/internal/transport"
	"github.com/jackc/pgx/v5/pgxpool"
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

// Exercise the exact generic native entry path before auth or inference, while
// preserving ordinary Broker/runtime effects and independently checked output.
func TestConverterDeveloperGenericFactoryLocalContainerE2E(t *testing.T) {
	var f *developerFixture
	var checker *releaseChainEntryChecker
	cfg := scriptedReleaseChainModelConfiguration()
	cfg.FixtureTimeout = 3 * time.Minute
	cfg.Factory = func(ctx context.Context, pool *pgxpool.Pool, root string) (transport.ModelProvider, error) {
		args := f.args()
		args.Budget = cfg.ChildBudget
		planner := &converterWorkerProvider{need: f.needRef, spec: f.specRef, document: f.document, args: args, admissionInput: f.admissionInput, states: map[string]*converterWorkerState{}}
		checker = &releaseChainEntryChecker{ModelProvider: planner, pool: pool, sources: f.f.sources, root: root, document: f.document, need: f.needRef, spec: f.specRef}
		return checker, nil
	}
	testConverterDeveloperWorkersObserved(t, true, &converterWorkerFaultHooks{factory: func(t *testing.T, p generatedtools.Profile, path string) *developerFixture {
		f = newDeveloperWorkerFixtureWithModel(t, p, path, true, cfg.FixtureTimeout, &cfg)
		return f
	}, after: func(t *testing.T, f *developerFixture, p *converterWorkerProvider) {
		if checker == nil || checker.starts.Load() != 4 || checker.submits.Load() != 10 || checker.continues.Load() != 0 || checker.parentStarts.Load() != 3 || checker.childStarts.Load() != 1 {
			t.Fatal("generic converter entry/current source proof incomplete")
		}
		t.Log("generic converter entries=14; exact current lineage/lease/source pins; native calls=0")
	}})
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
