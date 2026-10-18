package app

import (
	"encoding/json"
	"testing"

	"github.com/butler/butler/internal/memory/catalog"
	"github.com/butler/butler/internal/memory/readcontrol"
)

func TestTemporalBrokerReadIncludesMetadataAndPinsBeforeObservationIntegration(t *testing.T) {
	f := newNamedToolControlDispatchFixture(t, nil, false, false, readcontrol.Open)
	var a readcontrol.Args
	if err := json.Unmarshal([]byte(f.call.ArgsJson), &a); err != nil {
		t.Fatal(err)
	}
	c, err := f.f.sources.Propose(f.f.owner, catalog.ClaimRequest{SourceID: a.SourceID, SourceRevision: a.Revision, Key: "temporal.reviewed", Value: json.RawMessage(`true`), Summary: "owner reviewed fixture fact"})
	if err != nil {
		t.Fatal(err)
	}
	c, err = f.f.sources.Decide(f.f.owner, catalog.Decision{ClaimID: c.ID})
	if err != nil {
		t.Fatal(err)
	}
	if _, err = f.f.sources.SetTemporal(f.f.owner, catalog.TemporalUpdate{ClaimID: c.ID, ExpectedHeadRevision: 1, Validity: catalog.ValidityUnknown, Class: catalog.FreshnessTechnicalFact}); err != nil {
		t.Fatal(err)
	}
	prepareMemoryReadCheckpoint(t, f)
	service, err := NewMemoryReads(f.f.pool, f.f.sources)
	if err != nil {
		t.Fatal(err)
	}
	service.temporal = true
	h, err := readcontrol.NewHandler("guarded-request-private-process-fixture-token", service)
	if err != nil {
		t.Fatal(err)
	}
	process := controlBrokerProcess(t, f, h, readcontrol.Path, "BUTLER_MEMORY_READ_HARNESS="+readcontrol.Open)
	result, err := process.ExecuteAuthorizedToolCall(f.f.ctx, f.call, f.grant.ID)
	if err != nil || result.GetStatus() != "completed" {
		t.Fatal(err, result)
	}
	read, err := readcontrol.DecodeResult([]byte(result.GetResultJson()))
	if err != nil || len(read.Items) != 1 || len(read.Items[0].Temporal) != 1 {
		t.Fatal("typed temporal data missing or invalid", read, err)
	}
	m := read.Items[0].Temporal[0]
	if m.ClaimID != c.ID || m.Validity != "unknown" || m.VerifiedAt != nil || m.Freshness.Status != "unverified" || m.Freshness.CurrentUsable || !m.Freshness.RequiresRevalidation {
		t.Fatal("source freshness invented fact confidence", m)
	}
	if _, err = f.f.sources.SetTemporal(f.f.owner, catalog.TemporalUpdate{ClaimID: c.ID, ExpectedRevision: 1, ExpectedHeadRevision: 1, Validity: catalog.ValidityUnknown, Class: catalog.FreshnessTechnicalFact, OwnerVerifiedNow: true}); err != nil {
		t.Fatal(err)
	}
	if err = f.f.sources.CheckRunSources(f.owned, f.call.RunId); err == nil {
		t.Fatal("already opened result ignored metadata-only owner update")
	}
}
