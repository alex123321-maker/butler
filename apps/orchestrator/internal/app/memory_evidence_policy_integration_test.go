package app

import (
	"encoding/json"
	"errors"
	commonv1 "github.com/butler/butler/internal/gen/common/v1"
	"github.com/butler/butler/internal/memory/catalog"
	"github.com/butler/butler/internal/memory/experience"
	"github.com/butler/butler/internal/memory/readcontrol"
	"github.com/butler/butler/internal/memory/transcript"
	"strings"
	"testing"
	"time"
)

func TestExperienceEvidenceSelectionAndDurableStopThroughBrokerIntegration(t *testing.T) {
	f := newNamedToolControlDispatchFixture(t, nil, false, false, readcontrol.Open)
	store, err := experience.New(f.f.sources, strings.Repeat("b", 64), experienceToolsFixture{readcontrol.Contract(readcontrol.Open)})
	if err != nil {
		t.Fatal(err)
	}
	f.reader.lessons = store
	lesson := createUnverifiedReadLesson(t, f, store)
	f.call.ToolName = readcontrol.Search
	f.call.ToolCallId = "evidence-discovery"
	f.call.ArgsJson = `{"query":"fixture","limit":1}`
	f.call.AutonomyMode = commonv1.AutonomyMode_AUTONOMY_MODE_0
	if _, err = f.f.pool.Exec(f.f.ctx, `UPDATE runs SET autonomy_mode='mode_0' WHERE run_id=$1`, f.call.RunId); err != nil {
		t.Fatal(err)
	}
	f.prepared, err = f.auth.Prepare(f.f.ctx, f.call, readcontrol.Contract(readcontrol.Search))
	if err != nil {
		t.Fatal(err)
	}
	f.grant, err = f.auth.Authorize(f.f.ctx, f.call, readcontrol.Contract(readcontrol.Search), f.prepared.Binding.ActionID, "", "")
	if err != nil {
		t.Fatal(err)
	}
	prepareMemoryReadCheckpoint(t, f)
	newReads := func() *MemoryReads {
		r, err := NewMemoryReads(f.f.pool, f.f.sources, AdaptiveReadOptions{Experience: store, sourceSubset: []catalog.SourceRef{{ID: lesson.ID, Revision: lesson.Revision, AccessRevision: lesson.AccessRevision, Hash: lesson.Hash}}})
		if err != nil {
			t.Fatal(err)
		}
		return r
	}
	reads := newReads()
	h, err := readcontrol.NewHandler("guarded-request-private-process-fixture-token", reads)
	if err != nil {
		t.Fatal(err)
	}
	broker := controlBrokerProcess(t, f, h, readcontrol.Path, "BUTLER_MEMORY_READ_HARNESS="+readcontrol.Search)
	result, err := broker.ExecuteAuthorizedToolCall(f.f.ctx, f.call, f.grant.ID)
	if err != nil || result.GetStatus() != "completed" {
		t.Fatal("actual Broker discovery", err, result)
	}
	data, err := readcontrol.DecodeResult([]byte(result.ResultJson))
	if err != nil || len(data.Items) != 1 {
		t.Fatal("discovery DATA", err)
	}
	var locator struct {
		Value experience.EvidenceLocator `json:"experience_evidence_v1"`
	}
	if json.Unmarshal(data.Items[0].Locator, &locator) != nil || len(locator.Value.Ranges) != 6 || locator.Value.HistoricalTaskVerified {
		t.Fatal("actual Broker lost bounded evidence locator", string(data.Items[0].Locator))
	}
	// Broker effects are durable before Core appends its transcript receipt.
	now := time.Now().UTC()
	history := transcript.NewStore(f.f.pool)
	if _, err = history.AppendToolCall(f.owned, transcript.ToolCall{ToolCallID: f.call.ToolCallId, RunID: f.call.RunId, ToolName: f.call.ToolName, ArgsJSON: f.call.ArgsJson, Status: "completed", ResultJSON: result.ResultJson, ErrorJSON: `{}`, StartedAt: now, FinishedAt: &now}); err != nil {
		t.Fatal(err)
	}
	if err = newReads().CheckProposal(f.owned, f.call.RunId, readcontrol.Search, `{"query":"refine"}`); err != nil {
		t.Fatal("confirmed first discovery incorrectly stopped", err)
	}
	for _, id := range []string{"failed-search-1", "failed-search-2"} {
		now = now.Add(time.Millisecond)
		if _, err = history.AppendToolCall(f.owned, transcript.ToolCall{ToolCallID: id, RunID: f.call.RunId, ToolName: readcontrol.Search, ArgsJSON: `{"query":"retry"}`, Status: "failed", ResultJSON: `{}`, ErrorJSON: `{}`, StartedAt: now, FinishedAt: &now}); err != nil {
			t.Fatal(err)
		}
	}
	if err = newReads().CheckProposal(f.owned, f.call.RunId, readcontrol.Search, `{"query":"different words"}`); !errors.Is(err, experience.ErrSearchBound) {
		t.Fatal("restart/refining reset failed discovery allowance", err)
	}
	if err = newReads().CheckProposal(f.owned, f.call.RunId, "extension.project.inspect", `{}`); err != nil {
		t.Fatal("read exhaustion blocked task action", err)
	}
	if memoryEvidencePolicy(nil) != nil || memoryEvidencePolicy(&MemoryReads{}) != nil {
		t.Fatal("disabled typed nil enabled policy")
	}
	if err = newReads().CheckProposal(f.owned, "another-run", readcontrol.Search, `{"query":"fixture"}`); err == nil {
		t.Fatal("foreign run reused current read history")
	}
}
