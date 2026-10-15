package app

import (
	"github.com/butler/butler/internal/budget"
	"github.com/butler/butler/internal/memory/catalog"
	"github.com/butler/butler/internal/memory/compaction"
	"github.com/butler/butler/internal/providerfactory"
	"testing"
)

func TestSummaryConsumerRequiresBothExplicitGates(t *testing.T) {
	c := catalog.New(nil, nil)
	b := budget.NewController(nil, budget.DefaultPolicy())
	for _, tc := range []struct {
		worker, summary string
		want            bool
	}{{"false", "false", false}, {"true", "false", false}, {"false", "true", false}, {"true", "true", true}} {
		t.Run(tc.worker+"/"+tc.summary, func(t *testing.T) {
			t.Setenv("BUTLER_MEMORY_MODEL_JOBS_ENABLED", tc.worker)
			t.Setenv("BUTLER_MEMORY_SUMMARY_CONSUMER_ENABLED", tc.summary)
			_, w := configureModelJobs(c, b, providerfactory.Result{}, 18000)
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
