package app

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/butler/butler/apps/orchestrator/internal/api"
	"github.com/butler/butler/apps/orchestrator/internal/completion"
	"github.com/butler/butler/apps/orchestrator/internal/task"
	"github.com/butler/butler/internal/actionauth"
	"github.com/butler/butler/internal/budget"
	toolbrokerv1 "github.com/butler/butler/internal/gen/toolbroker/v1"
	"github.com/butler/butler/internal/memory/catalog"
	"github.com/butler/butler/internal/memory/experience"
	"github.com/butler/butler/internal/memory/jobs"
	"github.com/butler/butler/internal/memory/readcontrol"
	"github.com/butler/butler/internal/memory/transcript"
	"github.com/butler/butler/internal/skills"
	"github.com/butler/butler/internal/transport"
)

type experienceToolsFixture struct{ contract *toolbrokerv1.ToolContract }

func (f experienceToolsFixture) ListToolsForRun(context.Context, string) ([]*toolbrokerv1.ToolContract, error) {
	return []*toolbrokerv1.ToolContract{f.contract}, nil
}

type experienceNoteProvider struct {
	transport.ModelProvider
	note  experience.Notes
	calls int
}

func (*experienceNoteProvider) Name() string                    { return "antigravity" }
func (*experienceNoteProvider) InferenceLimits() (int64, int64) { return 32768, 4096 }
func (*experienceNoteProvider) CancelRun(context.Context, transport.CancelRunRequest) (*transport.TransportEvent, error) {
	return nil, nil
}
func (p *experienceNoteProvider) StartRun(ctx context.Context, r transport.StartRunRequest) (transport.EventStream, error) {
	if err := transport.CheckInferenceAuthority(ctx); err != nil {
		return nil, err
	}
	if err := transport.CheckInferenceDispatch(ctx); err != nil {
		return nil, err
	}
	p.calls++
	b, _ := json.Marshal(p.note)
	ch := make(chan transport.TransportEvent, 1)
	ch <- transport.NewAssistantFinalEvent(r.Context.RunID, p.Name(), transport.AssistantFinal{Content: string(b), Usage: &transport.Usage{InputTokens: 40, OutputTokens: 30, TotalTokens: 70}})
	close(ch)
	return ch, nil
}

// This is controlled inference, not native note/answer quality. The source task
// action goes through the actual compiled production Broker, real dispatch and
// effect journal. The normal owned transcript/completion/owner verifier then
// supply the independent typed observation consumed by the real model worker.
func TestExperienceActualBrokerTaskOwnerPolicyAndEpisodeIntegration(t *testing.T) {
	f := newNamedToolControlDispatchFixture(t, nil, false, false, readcontrol.Open)
	prepareMemoryReadCheckpoint(t, f)
	reads, err := NewMemoryReads(f.f.pool, f.f.sources)
	if err != nil {
		t.Fatal(err)
	}
	h, err := readcontrol.NewHandler("guarded-request-private-process-fixture-token", reads)
	if err != nil {
		t.Fatal(err)
	}
	broker := controlBrokerProcess(t, f, h, readcontrol.Path, "BUTLER_MEMORY_READ_HARNESS="+readcontrol.Open)
	result, err := broker.ExecuteAuthorizedToolCall(f.f.ctx, f.call, f.grant.ID)
	if err != nil || result.GetStatus() != "completed" || result.GetEffectState() != string(actionauth.StateConfirmed) {
		t.Fatal("actual source action failed", err, result)
	}
	history := transcript.NewStore(f.f.pool)
	at := time.Now().UTC()
	if _, err = history.AppendToolCall(f.owned, transcript.ToolCall{ToolCallID: f.call.ToolCallId, RunID: f.call.RunId, ToolName: f.call.ToolName, ArgsJSON: f.call.ArgsJson, Status: "completed", RuntimeTarget: readcontrol.Contract(readcontrol.Open).RuntimeTarget, StartedAt: at, FinishedAt: &at, ResultJSON: result.ResultJson}); err != nil {
		t.Fatal(err)
	}
	if _, err = f.f.pool.Exec(f.f.ctx, `UPDATE runs SET status='finalizing',current_state='finalizing' WHERE run_id=$1`, f.call.RunId); err != nil {
		t.Fatal(err)
	}
	results := completion.New(f.f.pool)
	results.Evidence = effectEvidence{journal: f.journal}
	results.DataGuard = f.f.sources.CheckRunSourcesTx
	final, err := results.Save(f.owned, completion.Request{RunID: f.call.RunId, SessionKey: f.f.parent.SessionKey, TaskID: f.f.parent.TaskID, TaskRevision: f.f.parent.Revision, ConstraintsRevision: f.f.parent.ConstraintsRevision, Content: "The exact source was read; original content was not changed."})
	if err != nil {
		t.Fatal(err)
	}
	ownerResult, err := f.f.store.SaveVerifiedResult(f.f.owner, task.Result{TaskID: f.f.parent.TaskID, TaskRevision: f.f.parent.Revision, RunID: f.call.RunId, Summary: "Owner independently observed the original read", EvidenceRefs: []string{"original source observed"}, VerifiedCriteria: append([]string{}, f.f.parent.CompletionCriteria...), Verifier: "owner", Verdict: "verified"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err = f.f.leases.ReleaseLease(f.f.ctx, f.grant.LeaseID); err != nil {
		t.Fatal(err)
	}
	lessons, err := experience.New(f.f.sources, strings.Repeat("b", 64), experienceToolsFixture{readcontrol.Contract(readcontrol.Open)})
	if err != nil {
		t.Fatal(err)
	}
	evidence := &skillEvidence{tasks: f.f.store, journal: f.journal, transcript: history, budgets: budget.NewLedger(f.f.pool), results: results, sources: f.f.sources, lessons: lessons}
	o, parents, err := evidence.taskObservation(f.f.owner, f.f.parent.OwnerID, f.call.RunId, lessons.EnvironmentHash)
	if err != nil || o.OwnerVerdict != "verified" || o.OwnerResultID != ownerResult.ResultID || o.ResultID != final.ResultID || len(o.Actions) != 1 || o.Actions[0].Effect != actionauth.StateConfirmed || len(parents) == 0 {
		t.Fatal("typed observation lost actual source/effect/owner separation", err)
	}
	f.f.budgets.Policy.Models["antigravity:experience-fixture"] = budget.ModelPrice{Basis: "subscription_no_overage"}
	f.f.budgets.Policy.ModelMillis = 10000
	j := jobs.New(f.f.pool, f.f.sources, f.f.budgets, nil)
	provider := &experienceNoteProvider{note: experience.Notes{Summary: "Observe the original exact DATA before reporting", When: []string{"Same exact environment and tool contract"}, NotApplicable: []string{"Changed version or missing current source grant"}, Steps: []experience.Step{{ActionID: o.Actions[0].ID, Comment: "Actual source read was effect-confirmed; no mutation was performed"}}}}
	w := &jobs.ModelWorker{Jobs: j, Provider: provider, Model: "experience-fixture", Handlers: map[string]jobs.ModelHandler{experience.Version: experience.Handler{}}}
	server := httptest.NewServer((&api.MemoryCatalogServer{Store: f.f.sources, Auth: api.NewOwnerControlAuth(strings.Repeat("e", 40), f.f.parent.OwnerID), ModelJobs: j, ModelWorkerConfigured: true, ExperiencePolicies: &experiencePolicies{lessons, j, evidence}, Experience: lessons}).Handle())
	defer server.Close()
	serial := 0
	post := func(path string, v, out any) {
		t.Helper()
		b, _ := json.Marshal(v)
		request, _ := http.NewRequest(http.MethodPost, server.URL+path, bytes.NewReader(b))
		request.Header.Set("Authorization", "Bearer "+strings.Repeat("e", 40))
		serial++
		request.Header.Set("X-Butler-Delivery-ID", fmt.Sprint("experience-", serial))
		response, err := server.Client().Do(request)
		if err != nil {
			t.Fatal(err)
		}
		defer response.Body.Close()
		body, _ := io.ReadAll(response.Body)
		if response.StatusCode < 200 || response.StatusCode >= 300 {
			t.Fatalf("%s: %d %s", path, response.StatusCode, body)
		}
		if out != nil && json.Unmarshal(body, out) != nil {
			t.Fatal("invalid owner receipt")
		}
	}
	q := experience.PolicyRequest{RunID: f.call.RunId, Policy: jobs.ModelPolicyRequest{ID: "actual-task-experience", Privacy: "cloud_allowed", IntervalSeconds: 60, MaxJobs: 1, Job: jobs.Request{Deadline: time.Now().Add(time.Hour), Allowance: budget.Units{ModelTurns: 1, Tokens: 100000, ActiveMillis: 10000, ProviderInputBytes: 8388608}}}}
	post("/api/v4/memory/model-experience-policies", q, nil)
	post("/api/v4/memory/model-worker-control", map[string]string{"state": "enabled"}, nil)
	if err = j.PollModelPolicies(f.f.ctx, func(string) bool { return true }); err != nil {
		t.Fatal(err)
	}
	if worked, err := w.ProcessOne(f.f.ctx); !worked || err != nil {
		view, _ := j.ModelView(f.f.owner)
		t.Fatal("owned lesson publication", worked, err, view)
	}
	var claim, source string
	if err = f.f.pool.QueryRow(f.f.ctx, `SELECT claim_id,source_id FROM memory_experience_lessons`).Scan(&claim, &source); err != nil {
		t.Fatal(err)
	}
	var before int
	_ = f.f.pool.QueryRow(f.f.ctx, `SELECT count(*) FROM memory_episodes`).Scan(&before)
	if before != 0 {
		t.Fatal("worker accepted its own note")
	}
	post("/api/v4/memory/decisions", catalog.Decision{ClaimID: claim}, nil)
	var content string
	var count int
	if err = f.f.pool.QueryRow(f.f.ctx, `SELECT content FROM memory_episodes`).Scan(&content); err != nil {
		t.Fatal(err)
	}
	var lesson experience.Lesson
	if json.Unmarshal([]byte(content), &lesson) != nil || lesson.Verification != "owner_verified_with_observed_actions" || lesson.InstructionAuthority || lesson.Observation.OwnerResultID != ownerResult.ResultID {
		t.Fatal("episode verification/provenance mismatch")
	}
	if err = f.f.pool.QueryRow(f.f.ctx, `SELECT count(*) FROM memory_source_dependencies WHERE child_source_id=$1`, source).Scan(&count); err != nil || count < 2 {
		t.Fatal("complete original graph lost", count, err)
	}
	if provider.calls != 1 {
		t.Fatal("unexpected repeated model attempt")
	}
	proofs, err := evidence.VerifySkillSources(f.f.owner, o.OwnerID, []skills.EvidenceRef{{TaskID: o.TaskID, TaskRevision: o.TaskRevision, ResultID: o.OwnerResultID}})
	if err != nil || len(proofs) != 1 {
		t.Fatal("actual source proof for lesson lineage", err)
	}
	lessonSource, err := f.f.sources.Get(f.f.owner, source)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = evidence.VerifyLessonLineage(f.f.owner, o.OwnerID, skills.LessonRequest{Roots: []catalog.SourceRef{{ID: lessonSource.ID, Revision: lessonSource.Revision, AccessRevision: lessonSource.AccessRevision, Hash: lessonSource.Hash}}, Proofs: proofs, Dependencies: o.Environment.Tools}); err == nil {
		t.Fatal("omitted lesson_sources downgraded actual experience lineage")
	}
	post("/api/v4/memory/model-experience-feedback", map[string]string{"source_id": source, "run_id": f.call.RunId, "verdict": "unknown"}, nil)
	var metadata string
	if err = f.f.pool.QueryRow(f.f.ctx, `SELECT to_jsonb(l)::text FROM memory_experience_lessons l`).Scan(&metadata); err != nil || strings.Contains(metadata, provider.note.Summary) || strings.Contains(metadata, o.AgentReport) {
		t.Fatal("unparented lesson prose in metadata")
	}
}

func TestExperienceProductionGateCannotBeBypassed(t *testing.T) {
	t.Setenv("BUTLER_MEMORY_EXPERIENCE_ENABLED", "true")
	if _, err := configureExperience(catalog.New(nil, nil), experienceToolsFixture{}); err == nil {
		t.Fatal("flag bypassed unqualified gate")
	}
}
