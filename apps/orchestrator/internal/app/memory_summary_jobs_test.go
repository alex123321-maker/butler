package app

import (
	"github.com/butler/butler/internal/budget"
	"github.com/butler/butler/internal/memory/catalog"
	"github.com/butler/butler/internal/memory/compaction"
	"github.com/butler/butler/internal/providerfactory"
	"github.com/butler/butler/internal/transport"
	"github.com/butler/butler/internal/transport/antigravity"
	"testing"
)

func TestSummaryConsumerRequiresBothExplicitGates(t *testing.T) {
	c := catalog.New(nil, nil)
	b := budget.NewController(nil, budget.DefaultPolicy())
	b.Policy.OutputTokens = 32768
	for _, tc := range []struct {
		worker, summary string
		want            bool
	}{{"false", "false", false}, {"true", "false", false}, {"false", "true", false}, {"true", "true", compaction.ReleaseQualified}} {
		t.Run(tc.worker+"/"+tc.summary, func(t *testing.T) {
			t.Setenv("BUTLER_MEMORY_MODEL_JOBS_ENABLED", tc.worker)
			t.Setenv("BUTLER_MEMORY_SUMMARY_CONSUMER_ENABLED", tc.summary)
			_, w := configureModelJobs(c, b, providerfactory.Result{Provider: summaryLimitProvider{output: 32768}, ModelName: antigravity.FlashHighModel}, 18000)
			if w == nil {
				if tc.want {
					t.Fatal("consumer absent")
				}
				return
			}
			h, ok := w.Handlers[compaction.HandlerName]
			if ok != tc.want {
				t.Fatal("consumer gate", ok)
			}
			if tc.want && h.(compaction.Consumer).ContextBudget != 18000 {
				t.Fatal("configured context budget lost")
			}
		})
	}
}

type summaryLimitProvider struct {
	transport.ModelProvider
	output int64
}

func (summaryLimitProvider) Name() string                      { return antigravity.Name }
func (p summaryLimitProvider) InferenceLimits() (int64, int64) { return 1048576, p.output }

func TestSummaryProfileRequiresQualifiedNativeCeilingAndBudget(t *testing.T) {
	for _, tc := range []struct {
		output, budget int64
		model          string
		want           bool
	}{
		{8192, 32768, antigravity.FlashHighModel, false},
		{32768, 8192, antigravity.FlashHighModel, false},
		{32768, 32768, antigravity.DefaultModel, false},
		{32768, 32768, antigravity.FlashHighModel, true},
	} {
		b := budget.NewController(nil, budget.DefaultPolicy())
		b.Policy.OutputTokens = tc.budget
		p := providerfactory.Result{Provider: summaryLimitProvider{output: tc.output}, ModelName: tc.model}
		if got := summaryProfileReady(p, b); got != tc.want {
			t.Fatal("summary native profile gate", tc, got)
		}
		if err := validateExpandedNativeOutputBudget(p, b.Policy); (tc.output > 8192 && tc.budget < tc.output) != (err != nil) {
			t.Fatal("expanded foreground ceiling under-reserved", tc, err)
		}
	}
}
