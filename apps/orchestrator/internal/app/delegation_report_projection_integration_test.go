package app

import (
	"testing"

	"github.com/butler/butler/internal/delegation"
	"github.com/butler/butler/internal/memory/catalog"
)

func TestDelegationGeneratedReportFromActualRecordsIntegration(t *testing.T) {
	for _, unknown := range []bool{false, true} {
		name := "confirmed-observation"
		if unknown {
			name = "unknown-partial-observation"
		}
		t.Run(name, func(t *testing.T) {
			f := newDelegationGuardFixture(t)
			c, child := f.admit(t, "projected-contract")
			result, expected, _ := f.complete(t, c, child, unknown, unknown)
			// Kernel maintenance has no parent/child worker token or owner actor.
			// End the real child lease before projecting its already durable final.
			if _, err := f.pool.Exec(f.ctx, `UPDATE session_ownership SET expires_at=clock_timestamp()-INTERVAL '1 second' WHERE session_key=$1`, child.SessionKey); err != nil {
				t.Fatal(err)
			}
			collected, err := f.manager.CollectGenerated(f.ctx, f.parent.TaskID, child.TaskID, result.ResultID)
			if err != nil || collected.Contract.State != "result_ready" || collected.Report == nil {
				t.Fatal(collected, err)
			}
			source, err := f.sources.Get(f.owner, collected.Report.ID)
			if err != nil || source.ScopeType != "session" || source.ScopeID != child.SessionKey {
				t.Fatal(source, err)
			}
			r, err := delegation.ParseReport([]byte(source.Body))
			if err != nil || len(r.Accomplished) != 0 || !equalDelegationActions(r.Actions, expected.Actions) || r.Budget.Used != expected.Budget.Used || !equalStrings(r.Budget.ReservationIDs, expected.Budget.ReservationIDs) {
				t.Fatal("projection differed from owning journal/budget or promoted success", r, err)
			}
			reasons := map[string]bool{}
			for _, v := range r.Uncertainty {
				reasons[v.Kind] = true
			}
			if !reasons["verification_missing"] || unknown && (!reasons["effect_unknown"] || !reasons["execution_failed"]) {
				t.Fatal("lost current limitations", r)
			}
			var before, after int
			if err = f.pool.QueryRow(f.ctx, `SELECT count(*) FROM memory_sources`).Scan(&before); err != nil {
				t.Fatal(err)
			}
			replayed, err := f.manager.CollectGenerated(f.ctx, f.parent.TaskID, child.TaskID, result.ResultID)
			if err != nil || replayed.Report == nil || *replayed.Report != *collected.Report {
				t.Fatal("collection replay changed report identity", replayed, err)
			}
			if err = f.pool.QueryRow(f.ctx, `SELECT count(*) FROM memory_sources`).Scan(&after); err != nil || after != before {
				t.Fatal("replay published a second source", before, after, err)
			}
			current, err := f.store.Get(f.ctx, child.TaskID)
			if err != nil || current.Status != "active" {
				t.Fatal("observations completed semantic task", current, err)
			}
			// Normal report comparison still refuses a forged budget after the
			// loaders were separated from parsing the caller's report.
			r.Budget.Used.ModelTurns++
			final, err := f.sources.Get(f.owner, "message:final:"+result.RunID)
			if err != nil {
				t.Fatal(err)
			}
			changedRef := f.report(t, "forged-projected-budget", child, r, append([]catalog.SourceRef{{ID: final.ID, Revision: final.Revision, AccessRevision: final.AccessRevision, Hash: final.Hash}}, refsForDelegation(c)...))
			tx, err := f.pool.Begin(f.ctx)
			if err != nil {
				t.Fatal(err)
			}
			err = f.guard.CheckReportTx(f.ctx, tx, f.parent, child, result, changedRef)
			tx.Rollback(f.ctx)
			if err == nil {
				t.Fatal("forged actual reserved units accepted")
			}
		})
	}
}

func TestDelegationGeneratedReportDeletionRollsBackIntegration(t *testing.T) {
	f := newDelegationGuardFixture(t)
	c, child := f.admit(t, "deleted-projection-contract")
	result, _, _ := f.complete(t, c, child, false, false)
	if _, err := f.sources.Delete(f.owner, c.ContractSource.ID, "delete", c.ContractSource.Revision); err != nil {
		t.Fatal(err)
	}
	var before, after int
	if err := f.pool.QueryRow(f.ctx, `SELECT count(*) FROM memory_sources`).Scan(&before); err != nil {
		t.Fatal(err)
	}
	if _, err := f.manager.CollectGenerated(f.ctx, f.parent.TaskID, child.TaskID, result.ResultID); err == nil {
		t.Fatal("deleted contract produced a report")
	}
	if err := f.pool.QueryRow(f.ctx, `SELECT count(*) FROM memory_sources`).Scan(&after); err != nil || before != after {
		t.Fatal("failed collection retained a new derived body", before, after, err)
	}
}
