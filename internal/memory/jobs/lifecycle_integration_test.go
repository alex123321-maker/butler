package jobs

import (
	"errors"
	"testing"

	"github.com/butler/butler/internal/memory/catalog"
	"github.com/butler/butler/internal/memory/memorytest"
)

func TestLifecycleModelCorpusRepeatForgetIntegration(t *testing.T) {
	for _, attack := range memorytest.LifecycleAttacks(t) {
		t.Run(attack.ID, func(t *testing.T) {
			w, owner, src := modelFixture(t)
			var e error
			src, e = w.Jobs.catalog.Correct(owner, catalog.Correction{SourceID: src.ID, ExpectedRevision: src.Revision, Title: attack.ID, Body: attack.Body})
			if e != nil {
				t.Fatal(e)
			}
			provider := w.Provider.(*modelFixtureProvider)
			provider.source = src
			j := schedule(t, w, owner, policyRequest(src))
			if e = w.execute(owner, j, QuoteProbe{}); e != nil {
				t.Fatal(e)
			}
			var result, op string
			if e = w.Jobs.pool.QueryRow(owner, `SELECT result_source_id,operation_id FROM memory_model_attempts WHERE job_id=$1 AND state='published'`, j.ID).Scan(&result, &op); e != nil {
				t.Fatal(e)
			}
			read, e := w.Jobs.catalog.Get(owner, result)
			if e != nil || read.Kind != "model_candidate" || read.Origin.IsInstruction() || read.ScopeID != "owner" {
				t.Fatal("job output gained authority", e)
			}
			// A lost publication ACK returns the already committed result, while
			// executing an old job claim cannot repeat inference or publication.
			got, e := w.Jobs.publishModel(owner, j, ModelAttempt{OperationID: op}, "retry ACK")
			if e != nil || got != result {
				t.Fatal("publication ACK not idempotent", got, e)
			}
			if e = w.execute(owner, j, QuoteProbe{}); e == nil {
				t.Fatal("completed job executed again")
			}
			if provider.calls.Load() != 1 {
				t.Fatal("repeat job performed inference")
			}
			var authority int
			if e = w.Jobs.pool.QueryRow(owner, `SELECT (SELECT count(*) FROM memory_claims)+(SELECT count(*) FROM action_authorizations)+(SELECT count(*) FROM action_mandates)+(SELECT count(*) FROM memory_profile)`).Scan(&authority); e != nil || authority != 0 {
				t.Fatal("output changed facts or authority", authority, e)
			}
			if _, e = w.Jobs.catalog.Delete(owner, src.ID, "delete", src.Revision); e != nil {
				t.Fatal(e)
			}
			restarted := New(w.Jobs.pool, w.Jobs.catalog, w.Jobs.budgets, w.Jobs.target)
			if _, e = restarted.catalog.Get(owner, result); !errors.Is(e, catalog.ErrDenied) {
				t.Fatal("forgotten job result survived worker restart", e)
			}
		})
	}
}
