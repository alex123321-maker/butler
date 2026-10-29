package app

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/butler/butler/internal/actionauth"
	commonv1 "github.com/butler/butler/internal/gen/common/v1"
	"github.com/butler/butler/internal/memory/catalog"
	"github.com/butler/butler/internal/memory/experience"
	"github.com/butler/butler/internal/memory/factcontrol"
	"github.com/butler/butler/internal/memory/jobs"
	"github.com/butler/butler/internal/memory/readcontrol"
	"github.com/butler/butler/internal/memory/transcript"
	"github.com/butler/butler/internal/skills"
)

// The producer is an explicit unverified typed projection fixture; generation
// from real Broker effects is covered by the separate owning-chain test. This
// test proves actual Broker lesson reads and current release guards, not quality.
func TestExperienceActualBrokerReadAndNegativeReleaseIntegration(t *testing.T) {
	f := newNamedToolControlDispatchFixture(t, nil, false, false, readcontrol.Open)
	store, err := experience.New(f.f.sources, strings.Repeat("b", 64), experienceToolsFixture{readcontrol.Contract(readcontrol.Open)})
	if err != nil {
		t.Fatal(err)
	}
	f.reader.lessons = store
	result := createUnverifiedReadLesson(t, f, store)
	// An ordinary Episodic projection can be a descendant of a typed lesson.
	// Its adapter must retain ancestor applicability rather than classify only
	// the root as an ordinary episode and bypass the experience gate.
	tx, err := f.f.pool.Begin(f.f.owner)
	if err != nil {
		t.Fatal(err)
	}
	derived, err := f.f.sources.CreateDerivedTx(f.f.owner, tx, []catalog.SourceRef{{ID: result.ID, Revision: result.Revision, AccessRevision: result.AccessRevision, Hash: result.Hash}}, "memory_extract", "Derived lesson presentation", "Observed dependent episodic DATA", json.RawMessage(`{}`))
	if err != nil {
		tx.Rollback(f.f.owner)
		t.Fatal(err)
	}
	derivedClaim, err := f.f.sources.ProposeTx(f.f.owner, tx, catalog.ClaimRequest{MemoryType: "episodic", SourceID: derived.ID, SourceRevision: derived.Revision, Key: "episode.experience.derived_fixture", Value: json.RawMessage(`{"observed":"dependent DATA"}`), Summary: "Derived experience presentation", Confidence: 0, DedupKey: "derived-episode-fixture"})
	if err != nil {
		tx.Rollback(f.f.owner)
		t.Fatal(err)
	}
	if err = tx.Commit(f.f.owner); err != nil {
		t.Fatal(err)
	}
	if _, err = f.f.sources.Decide(f.f.owner, catalog.Decision{ClaimID: derivedClaim.ID}); err != nil {
		t.Fatal(err)
	}
	var episode int64
	if f.f.pool.QueryRow(f.f.ctx, `SELECT episode_id FROM memory_claims WHERE claim_id=$1`, derivedClaim.ID).Scan(&episode) != nil {
		t.Fatal("derived episode missing")
	}
	if allowed, err := experienceObjectAllowed(f.owned, f.f.sources, store, episode); err != nil || !allowed {
		t.Fatal("healthy derived episode suppressed", err)
	}
	changed, _ := experience.New(f.f.sources, strings.Repeat("e", 64), experienceToolsFixture{readcontrol.Contract(readcontrol.Open)})
	for _, current := range []*experience.Store{nil, changed} {
		if allowed, err := experienceObjectAllowed(f.owned, f.f.sources, current, episode); err != nil || allowed {
			t.Fatal("derived episode lost environment applicability", err)
		}
	}
	if _, err = f.f.pool.Exec(f.f.ctx, `UPDATE runs SET autonomy_mode='mode_0' WHERE run_id=$1`, f.call.RunId); err != nil {
		t.Fatal(err)
	}
	args, _ := json.Marshal(readcontrol.Args{SourceID: result.ID, Revision: result.Revision, AccessRevision: result.AccessRevision, Hash: result.Hash, MaxChars: 4096})
	f.call.ArgsJson = string(args)
	f.call.ToolCallId = "experience-read"
	f.call.AutonomyMode = commonv1.AutonomyMode_AUTONOMY_MODE_0
	f.reader.lessons = store
	prepared, err := f.auth.Prepare(f.f.ctx, f.call, readcontrol.Contract(readcontrol.Open))
	if err != nil || prepared.RequiresApproval {
		t.Fatal(err)
	}
	grant, err := f.auth.Authorize(f.f.ctx, f.call, readcontrol.Contract(readcontrol.Open), prepared.Binding.ActionID, "", "")
	if err != nil {
		t.Fatal(err)
	}
	f.prepared, f.grant = prepared, grant
	prepareMemoryReadCheckpoint(t, f)
	reads, err := NewMemoryReads(f.f.pool, f.f.sources, AdaptiveReadOptions{Experience: store})
	if err != nil {
		t.Fatal(err)
	}
	h, err := readcontrol.NewHandler("guarded-request-private-process-fixture-token", reads)
	if err != nil {
		t.Fatal(err)
	}
	broker := controlBrokerProcess(t, f, h, readcontrol.Path, "BUTLER_MEMORY_READ_HARNESS="+readcontrol.Open)
	read, err := broker.ExecuteAuthorizedToolCall(f.f.ctx, f.call, grant.ID)
	if err != nil || read.GetStatus() != "completed" {
		t.Fatal("actual lesson read failed", err, read)
	}
	data, err := readcontrol.DecodeResult([]byte(read.ResultJson))
	if err != nil || len(data.Items) != 1 || !strings.Contains(data.Items[0].Text, "task_unverified") {
		t.Fatal("unverified outcome lost", err)
	}
	if err = store.CheckPinned(f.owned, f.call.RunId); err != nil {
		t.Fatal(err)
	}
	// The separate accepted-facts path must apply the same ancestor and exact
	// exposure restrictions. This is a real accepted episodic projection from
	// the lesson, not a manually supplied temporal result.
	factRead := func(current *experience.Store, subset []catalog.SourceRef) readcontrol.Result {
		t.Helper()
		service := &MemoryReads{sources: f.f.sources, experience: current, sourceSubset: subset}
		var environment experience.Prepared
		if current != nil {
			var err error
			environment, err = current.Prepare(f.owned, f.call.RunId)
			if err != nil {
				t.Fatal(err)
			}
		}
		tx, err := f.f.pool.Begin(f.owned)
		if err != nil {
			t.Fatal(err)
		}
		defer tx.Rollback(f.owned)
		out, err := service.readTemporalFactsTx(f.owned, tx, factcontrol.Args{Sources: []readcontrol.SourceRef{{ID: derived.ID, Revision: derived.Revision, AccessRevision: derived.AccessRevision, Hash: derived.Hash}}, Limit: 4}, environment)
		if err != nil {
			t.Fatal(err)
		}
		return out
	}
	if out := factRead(store, nil); out.Status != "ok" || len(out.Items) != 1 {
		t.Fatal("applicable accepted derivative not readable", out)
	}
	for _, current := range []*experience.Store{nil, changed} {
		if out := factRead(current, nil); out.Status != "denied" || len(out.Items) != 0 {
			t.Fatal("accepted facts lost ancestor environment restriction", out)
		}
	}
	if out := factRead(store, []catalog.SourceRef{}); out.Status != "denied" || len(out.Items) != 0 {
		t.Fatal("accepted facts bypassed empty exposure cohort", out)
	}
	// Exercise the actual separate facts dispatch as well: the owning boundary
	// must forward its freshly prepared environment into the temporal reader.
	f.reader.memoryTemporalReady = true
	f.call.ToolName = factcontrol.Tool
	f.call.ToolCallId = "experience-temporal-read"
	factArgs, _ := json.Marshal(factcontrol.Args{Sources: []readcontrol.SourceRef{{ID: derived.ID, Revision: derived.Revision, AccessRevision: derived.AccessRevision, Hash: derived.Hash}}, Limit: 4})
	f.call.ArgsJson = string(factArgs)
	f.prepared, err = f.auth.Prepare(f.f.ctx, f.call, factcontrol.Contract())
	if err != nil || f.prepared.RequiresApproval {
		t.Fatal("typed derivative facts admission rejected", err)
	}
	f.grant, err = f.auth.Authorize(f.f.ctx, f.call, factcontrol.Contract(), f.prepared.Binding.ActionID, "", "")
	if err != nil {
		t.Fatal(err)
	}
	prepareMemoryReadCheckpoint(t, f)
	reads.temporal = true
	factHandler, err := factcontrol.NewHandler("guarded-request-private-process-fixture-token", reads)
	if err != nil {
		t.Fatal(err)
	}
	factBroker := controlBrokerProcess(t, f, factHandler, factcontrol.Path, "BUTLER_MEMORY_FACT_HARNESS=1")
	factResult, err := factBroker.ExecuteAuthorizedToolCall(f.f.ctx, f.call, f.grant.ID)
	if err != nil || factResult.GetStatus() != "completed" {
		t.Fatal("actual derived temporal facts dispatch failed", err)
	}
	facts, err := readcontrol.DecodeResult([]byte(factResult.GetResultJson()))
	if err != nil || facts.Status != "ok" || len(facts.Items) != 1 {
		t.Fatal("actual facts boundary lost current environment", err, facts.Status)
	}
	if err = store.Feedback(f.f.owner, result.ID, f.call.RunId, "misapplied"); err != nil {
		t.Fatal(err)
	}
	if allowed, err := experienceObjectAllowed(f.owned, f.f.sources, store, episode); err != nil || allowed {
		t.Fatal("derived episode ignored negative ancestor feedback", err)
	}
	if out := factRead(store, nil); out.Status != "denied" || len(out.Items) != 0 {
		t.Fatal("accepted facts ignored negative ancestor feedback", out)
	}
	if store.CheckPinned(f.owned, f.call.RunId) == nil {
		t.Fatal("current negative feedback failed release guard")
	}
	if _, err = f.reader.GetActionContext(f.f.ctx, f.call.RunId); err == nil {
		t.Fatal("next Broker action ignored negative pinned lesson")
	}
	var status, goal string
	if f.f.pool.QueryRow(f.f.ctx, `SELECT status,goal FROM tasks WHERE task_id=$1`, f.f.parent.TaskID).Scan(&status, &goal) != nil || status != "active" || goal != f.f.parent.Goal {
		t.Fatal("lesson changed task authority")
	}
}

// Shared explicit unverified projection; never fabricates a successful task.
func createUnverifiedReadLesson(t *testing.T, f *dispatchAdmissionFixture, store *experience.Store) catalog.Source {
	t.Helper()
	var err error
	d := skills.Dependency{Tool: f.call.ToolName, ContractHash: f.prepared.Binding.ContractHash, ContractRevision: f.prepared.Binding.ContractRevision}
	o := experience.Observation{Format: experience.SnapshotFormat, OwnerID: f.f.parent.OwnerID, TaskID: f.f.parent.TaskID, TaskRevision: f.f.parent.Revision, ConstraintsRevision: f.f.parent.ConstraintsRevision, RunID: f.call.RunId, ResultID: "unverified-projection-fixture", Goal: f.f.parent.Goal, Constraints: []string{}, Environment: experience.Environment{Fingerprint: store.EnvironmentHash, Tools: []skills.Dependency{d}}, AgentReport: "No independently verified outcome in this fixture", OwnerVerdict: "unknown", VerifiedCriteria: []string{}, Actions: []experience.Action{{ID: f.prepared.EffectID, Tool: d.Tool, ContractHash: d.ContractHash, ContractRevision: d.ContractRevision, ArgumentsHash: f.prepared.Binding.ArgsHash, Effect: actionauth.StateAllowed, Evidence: []string{}}}, Failures: []experience.FailedCall{}, Unknowns: []string{"action_not_executed"}}
	o.Actions[0].ToolCallID = f.call.ToolCallId
	transcripts := transcript.NewStore(f.f.pool)
	if _, err = transcripts.AppendToolCall(f.owned, transcript.ToolCall{ToolCallID: f.call.ToolCallId, RunID: f.call.RunId, ToolName: f.call.ToolName, Status: "prepared", RuntimeTarget: "explicit-unexecuted-fixture"}); err != nil {
		t.Fatal(err)
	}
	captured, err := transcripts.CaptureRunTools(f.owned, f.call.RunId, 2*experience.MaxActions)
	if err != nil {
		t.Fatal(err)
	}
	o.SessionKey = captured.SessionKey
	o.TranscriptRevision = captured.SourceRevision
	o.ToolPins = captured.Pins
	p := f.args.Sources[0]
	snapshot, err := store.Seal(f.f.owner, o, []catalog.SourceRef{{ID: p.ID, Revision: p.Revision, AccessRevision: p.AccessRevision, Hash: p.Hash}})
	if err != nil {
		t.Fatal(err)
	}
	input, err := f.f.sources.Get(f.f.owner, snapshot.ID)
	if err != nil {
		t.Fatal(err)
	}
	note, _ := json.Marshal(experience.Notes{Summary: "Unverified observed attempt; revalidation required", When: []string{"Current original environment"}, NotApplicable: []string{"No outcome was verified"}, Steps: []experience.Step{{ActionID: o.Actions[0].ID, Comment: "Observe before acting; this action has not executed"}}})
	handler := experience.Handler{}
	body, err := handler.Validate([]catalog.Source{input}, string(note))
	if err != nil {
		t.Fatal(err)
	}
	tx, err := f.f.pool.Begin(f.f.owner)
	if err != nil {
		t.Fatal(err)
	}
	result, err := f.f.sources.CreateDerivedTx(f.f.owner, tx, []catalog.SourceRef{snapshot}, "memory_extract", "Typed lesson fixture", body, json.RawMessage(`{}`))
	if err != nil {
		t.Fatal(err)
	}
	if err = handler.PublishTx(f.f.owner, tx, f.f.sources, jobs.Job{OwnerID: o.OwnerID, TaskID: o.TaskID, OriginalRunID: o.RunID, Purpose: experience.Version}, jobs.ModelAttempt{}, []catalog.Source{input}, result, body); err != nil {
		t.Fatal(err)
	}
	if err = tx.Commit(f.f.owner); err != nil {
		t.Fatal(err)
	}
	var claim string
	if f.f.pool.QueryRow(f.f.ctx, `SELECT claim_id FROM memory_experience_lessons WHERE source_id=$1`, result.ID).Scan(&claim) != nil {
		t.Fatal("candidate missing")
	}
	if _, err = f.f.sources.Decide(f.f.owner, catalog.Decision{ClaimID: claim}); err != nil {
		t.Fatal(err)
	}
	return result
}

func TestExperienceIncompatibleSearchAndOpenAreDataResultsThroughBrokerIntegration(t *testing.T) {
	for _, tool := range []string{readcontrol.Search, readcontrol.Open} {
		t.Run(tool, func(t *testing.T) {
			f := newNamedToolControlDispatchFixture(t, nil, false, false, tool)
			store, err := experience.New(f.f.sources, strings.Repeat("b", 64), experienceToolsFixture{readcontrol.Contract(tool)})
			if err != nil {
				t.Fatal(err)
			}
			lesson := createUnverifiedReadLesson(t, f, store)
			ordinary, err := f.f.sources.Put(f.f.owner, catalog.PutSource{ExternalKey: "compatible-data", Kind: "document", ScopeType: "user", ScopeID: f.f.parent.OwnerID, Title: "Fixture current DATA", Body: "Fixture current ordinary DATA"})
			if err != nil {
				t.Fatal(err)
			}
			args := map[string]any{"query": "fixture", "limit": 1}
			if tool == readcontrol.Open {
				args = map[string]any{"source_id": lesson.ID, "source_revision": lesson.Revision, "access_revision": lesson.AccessRevision, "content_hash": lesson.Hash, "max_chars": 1000}
			}
			raw, _ := json.Marshal(args)
			f.call.ArgsJson, f.call.ToolCallId = string(raw), "filtered-current-read"
			f.call.AutonomyMode = commonv1.AutonomyMode_AUTONOMY_MODE_0
			if _, err = f.f.pool.Exec(f.f.ctx, `UPDATE runs SET autonomy_mode='mode_0' WHERE run_id=$1`, f.call.RunId); err != nil {
				t.Fatal(err)
			}
			f.prepared, err = f.auth.Prepare(f.f.ctx, f.call, readcontrol.Contract(tool))
			if err != nil {
				t.Fatal(err)
			}
			f.grant, err = f.auth.Authorize(f.f.ctx, f.call, readcontrol.Contract(tool), f.prepared.Binding.ActionID, "", "")
			if err != nil {
				t.Fatal(err)
			}
			prepareMemoryReadCheckpoint(t, f)
			// No experience consumer: the lesson is inapplicable. Source scope and
			// actual dispatch authority still apply to the ordinary permitted DATA.
			reads, err := NewMemoryReads(f.f.pool, f.f.sources, AdaptiveReadOptions{sourceSubset: []catalog.SourceRef{{ID: lesson.ID, Revision: lesson.Revision, AccessRevision: lesson.AccessRevision, Hash: lesson.Hash}, {ID: ordinary.ID, Revision: ordinary.Revision, AccessRevision: ordinary.AccessRevision, Hash: ordinary.Hash}}})
			if err != nil {
				t.Fatal(err)
			}
			h, err := readcontrol.NewHandler("guarded-request-private-process-fixture-token", reads)
			if err != nil {
				t.Fatal(err)
			}
			broker := controlBrokerProcess(t, f, h, readcontrol.Path, "BUTLER_MEMORY_READ_HARNESS="+tool)
			result, err := broker.ExecuteAuthorizedToolCall(f.f.ctx, f.call, f.grant.ID)
			if err != nil || result.GetStatus() != "completed" {
				t.Fatal("inapplicable source became an uncertain dispatched read", err, result)
			}
			data, err := readcontrol.DecodeResult([]byte(result.ResultJson))
			if err != nil {
				t.Fatal(err)
			}
			if tool == readcontrol.Search && (data.Status != "ok" || len(data.Items) != 1 || data.Items[0].Source.ID != ordinary.ID) {
				t.Fatal("ordinary DATA was lost behind an incompatible lesson", data.Status)
			}
			if tool == readcontrol.Open && (data.Status != "denied" || len(data.Items) != 0) {
				t.Fatal("incompatible body released", data.Status)
			}
			var pins, unknown int
			if err = f.f.pool.QueryRow(f.f.ctx, `SELECT (SELECT count(*) FROM memory_run_sources WHERE run_id=$1 AND source_id=$2),(SELECT count(*) FROM action_effects WHERE initial_binding->>'run_id'=$1 AND state='effect_unknown')`, f.call.RunId, lesson.ID).Scan(&pins, &unknown); err != nil || pins != 0 || unknown != 0 {
				t.Fatal("inapplicable read left pins or UNKNOWN", err, pins, unknown)
			}
		})
	}
}
