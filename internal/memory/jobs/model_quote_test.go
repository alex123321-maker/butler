package jobs

import (
	"testing"

	"github.com/butler/butler/internal/budget"
)

func TestModelQuoteFiniteCallCeiling(t *testing.T) {
	for _, tc := range []struct{ configured, want int64 }{
		{1000, 1000}, {120000, 120000}, {300000, 300000}, {86400000, 300000},
	} {
		p := budget.DefaultPolicy()
		p.ModelMillis = tc.configured
		p.Models["antigravity:fixture"] = budget.ModelPrice{Basis: "subscription_no_overage"}
		w := ModelWorker{Jobs: &Store{budgets: budget.NewController(nil, p)}, Provider: &modelFixtureProvider{}, Model: "fixture"}
		q, err := w.quote()
		if err != nil || q.ActiveMillis != tc.want || q.ModelTurns != 1 || q.Tokens != 192 {
			t.Fatalf("configured %d: quote=%+v err=%v", tc.configured, q, err)
		}
	}
}
