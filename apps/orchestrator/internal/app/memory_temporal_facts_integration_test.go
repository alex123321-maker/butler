package app

import (
	"encoding/json"
	"fmt"
	"testing"
	"time"

	commonv1 "github.com/butler/butler/internal/gen/common/v1"
	"github.com/butler/butler/internal/memory/adaptive"
	"github.com/butler/butler/internal/memory/catalog"
	"github.com/butler/butler/internal/memory/factcontrol"
	"github.com/butler/butler/internal/memory/readcontrol"
)

func TestTemporalFactsActualBrokerHistoricalKnownAndCurrentBoundIntegration(t *testing.T) {
	f := newNamedToolControlDispatchFixture(t, nil, false, false, factcontrol.Tool)
	create := func(key, body, value, active string) (catalog.Source, catalog.Claim) {
		t.Helper()
		src, err := f.f.sources.Put(f.f.owner, catalog.PutSource{ExternalKey: key, Kind: "owner_statement", ScopeType: "user", ScopeID: f.f.parent.OwnerID, Body: body})
		if err != nil {
			t.Fatal(err)
		}
		c, err := f.f.sources.Propose(f.f.owner, catalog.ClaimRequest{SourceID: src.ID, SourceRevision: 1, Key: "server.current", Value: json.RawMessage(value), Summary: body})
		if err != nil {
			t.Fatal(err)
		}
		c, err = f.f.sources.Decide(f.f.owner, catalog.Decision{ClaimID: c.ID, ExpectedActiveID: active})
		if err != nil {
			t.Fatal(err)
		}
		return src, c
	}
	feb := time.Date(2026, 2, 1, 0, 0, 0, 0, time.UTC)
	march := feb.AddDate(0, 1, 0)
	midFeb := feb.Add(14 * 24 * time.Hour)
	a, ca := create("server-a", "В феврале сервер A.", `"A"`, "")
	if _, err := f.f.sources.SetTemporal(f.f.owner, catalog.TemporalUpdate{ClaimID: ca.ID, ExpectedHeadRevision: 1, Validity: catalog.ValidityInterval, ValidFrom: &feb, ValidTo: &march, Class: catalog.FreshnessTechnicalFact}); err != nil {
		t.Fatal(err)
	}
	var knownA time.Time
	if err := f.f.pool.QueryRow(f.f.ctx, `SELECT clock_timestamp()`).Scan(&knownA); err != nil {
		t.Fatal(err)
	}
	b, cb := create("server-b", "С марта сервер B.", `"B"`, ca.ID)
	if _, err := f.f.sources.SetTemporal(f.f.owner, catalog.TemporalUpdate{ClaimID: cb.ID, ExpectedHeadRevision: 2, Validity: catalog.ValidityInterval, ValidFrom: &march, Class: catalog.FreshnessTechnicalFact, OwnerVerifiedNow: true}); err != nil {
		t.Fatal(err)
	}
	store, err := adaptive.NewStore(f.f.pool, f.f.sources)
	if err != nil {
		t.Fatal(err)
	}
	if err = store.Init(f.owned, adaptive.Classify("Вспомни нашу историю сервера")); err != nil {
		t.Fatal(err)
	}
	reads, err := NewMemoryReads(f.f.pool, f.f.sources, AdaptiveReadOptions{Store: store})
	if err != nil {
		t.Fatal(err)
	}
	reads.temporal = true
	h, err := factcontrol.NewHandler("guarded-request-private-process-fixture-token", reads)
	if err != nil {
		t.Fatal(err)
	}
	process := controlBrokerProcess(t, f, h, factcontrol.Path, "BUTLER_MEMORY_FACT_HARNESS=1")
	if _, err = f.f.pool.Exec(f.f.ctx, `UPDATE runs SET autonomy_mode='mode_0' WHERE run_id=$1`, f.call.RunId); err != nil {
		t.Fatal(err)
	}
	f.call.AutonomyMode = commonv1.AutonomyMode_AUTONOMY_MODE_0
	refs := []readcontrol.SourceRef{{ID: a.ID, Revision: a.Revision, AccessRevision: a.AccessRevision, Hash: a.Hash}, {ID: b.ID, Revision: b.Revision, AccessRevision: b.AccessRevision, Hash: b.Hash}}
	cases := []struct{ valid, known, want string }{{midFeb.Format(time.RFC3339Nano), "", ca.ID}, {"", "", cb.ID}, {"", knownA.Format(time.RFC3339Nano), ca.ID}, {midFeb.Format(time.RFC3339Nano), knownA.Format(time.RFC3339Nano), ca.ID}}
	for index, tc := range cases {
		f.call.ToolCallId = fmt.Sprintf("temporal-facts-%d", index)
		raw, _ := json.Marshal(factcontrol.Args{Sources: refs, ValidAt: tc.valid, KnownAt: tc.known, Limit: 4})
		f.call.ArgsJson = string(raw)
		f.prepared, err = f.auth.Prepare(f.f.ctx, f.call, factcontrol.Contract())
		if err != nil || f.prepared.RequiresApproval {
			t.Fatal("bounded DATA facts requires approval in mode0", err)
		}
		f.grant, err = f.auth.Authorize(f.f.ctx, f.call, factcontrol.Contract(), f.prepared.Binding.ActionID, "", "")
		if err != nil {
			t.Fatal(err)
		}
		prepareMemoryReadCheckpoint(t, f)
		result, err := process.ExecuteAuthorizedToolCall(f.f.ctx, f.call, f.grant.ID)
		if err != nil || result.GetStatus() != "completed" {
			t.Fatal("actual temporal Broker dispatch", index, err, result)
		}
		out, err := readcontrol.DecodeResult([]byte(result.GetResultJson()))
		if err != nil || out.Status != "ok" || len(out.Items) != 1 || len(out.Items[0].Temporal) != 1 || out.Items[0].Temporal[0].ClaimID != tc.want || out.Items[0].ReadRef != "" {
			t.Fatal("wrong dated fact or fabricated source-open citation", index, out, err)
		}
		if (tc.valid != "" || tc.known != "") && !out.Items[0].Temporal[0].Historical {
			t.Fatal("historical lookup presented as current")
		}
		observation := out.Items[0].Temporal[0]
		basis := "unknown"
		if tc.want == cb.ID {
			basis = "owner_statement"
			if observation.Freshness.Status != "fresh" || !observation.Freshness.CurrentUsable || observation.Freshness.RequiresRevalidation {
				t.Fatal("fresh accepted verification lost behind source currency marker", observation)
			}
		}
		if observation.VerificationBasis != basis || out.Items[0].Freshness != "current_revision_unverified_fact" {
			t.Fatal("accepted verification basis conflated with source revision currency", observation)
		}
	}
	var calls, opened, reservations int
	if err = f.f.pool.QueryRow(f.f.ctx, `SELECT (SELECT calls FROM memory_retrieval_plans WHERE run_id=$1),(SELECT count(*) FROM memory_retrieval_opened),(SELECT count(*) FROM execution_budget_reservations)`, f.call.RunId).Scan(&calls, &opened, &reservations); err != nil || calls != 4 || opened != 0 || reservations != 4 {
		t.Fatal("temporal lookup bypassed common bound or minted citations", calls, opened, reservations, err)
	}
	if err = store.CheckProposal(f.owned, f.call.RunId, factcontrol.Tool, f.call.ArgsJson); err != adaptive.ErrBound {
		t.Fatal("facts bypassed adaptive finite loop", err)
	}
	if _, err = f.f.sources.SetTemporal(f.f.owner, catalog.TemporalUpdate{ClaimID: ca.ID, ExpectedRevision: 1, ExpectedHeadRevision: 2, Validity: catalog.ValidityInterval, ValidFrom: &feb, ValidTo: &march, Class: catalog.FreshnessTechnicalFact, OwnerVerifiedNow: true}); err != nil {
		t.Fatal(err)
	}
	if err = f.f.sources.CheckRunSources(f.owned, f.call.RunId); err == nil {
		t.Fatal("historical fact survived changed owner temporal interpretation")
	}
}
