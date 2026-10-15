package experience

import (
	"context"
	"encoding/json"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/butler/butler/internal/actionauth"
	"github.com/butler/butler/internal/budget"
	toolbrokerv1 "github.com/butler/butler/internal/gen/toolbroker/v1"
	"github.com/butler/butler/internal/memory/catalog"
	"github.com/butler/butler/internal/memory/jobs"
	"github.com/butler/butler/internal/memory/memorytest"
	"github.com/butler/butler/internal/memory/readcontrol"
	"github.com/butler/butler/internal/memory/transcript"
	"github.com/butler/butler/internal/owneridentity"
	"github.com/butler/butler/internal/ownership"
	"github.com/butler/butler/internal/skills"
	"github.com/butler/butler/internal/transport"
	"github.com/jackc/pgx/v5/pgxpool"
)

type toolsFixture struct{ contract *toolbrokerv1.ToolContract }

func (f toolsFixture) ListToolsForRun(context.Context, string) ([]*toolbrokerv1.ToolContract, error) {
	return []*toolbrokerv1.ToolContract{f.contract}, nil
}

type noteProvider struct {
	transport.ModelProvider
	output string
	before func()
	calls  int
}

func (*noteProvider) Name() string                    { return "antigravity" }
func (*noteProvider) InferenceLimits() (int64, int64) { return 32768, 4096 }
func (*noteProvider) CancelRun(context.Context, transport.CancelRunRequest) (*transport.TransportEvent, error) {
	return nil, nil
}
func (p *noteProvider) StartRun(ctx context.Context, r transport.StartRunRequest) (transport.EventStream, error) {
	if err := transport.CheckInferenceAuthority(ctx); err != nil {
		return nil, err
	}
	if err := transport.CheckInferenceDispatch(ctx); err != nil {
		return nil, err
	}
	p.calls++
	if p.before != nil {
		p.before()
	}
	ch := make(chan transport.TransportEvent, 1)
	ch <- transport.NewAssistantFinalEvent(r.Context.RunID, p.Name(), transport.AssistantFinal{Content: p.output, Usage: &transport.Usage{InputTokens: 30, OutputTokens: 20, TotalTokens: 50}})
	close(ch)
	return ch, nil
}

type lessonFixture struct {
	s           *Store
	ctx         context.Context
	pool        *pgxpool.Pool
	parent      catalog.Source
	observation Observation
	snapshot    catalog.SourceRef
	j           *jobs.Store
	w           *jobs.ModelWorker
	p           *noteProvider
}

func newLessonFixture(t *testing.T) *lessonFixture {
	t.Helper()
	return newLessonFixtureWithPool(t, memorytest.Database(t), filepath.Join(t.TempDir(), "anchor"))
}
func newLessonFixtureWithPool(t *testing.T, pool *pgxpool.Pool, anchorPath string) *lessonFixture {
	t.Helper()
	ctx := owneridentity.WithActor(context.Background(), owneridentity.Actor{OwnerID: "owner", Channel: "web", DeliveryID: "experience-fixture"})
	manifest := catalog.NewFileManifest(anchorPath)
	if err := manifest.Initialize(); err != nil {
		t.Fatal(err)
	}
	c := catalog.New(pool, manifest)
	if _, err := pool.Exec(ctx, `INSERT INTO sessions(session_key,user_id,channel)VALUES('experience','owner','web');INSERT INTO tasks(task_id,session_key,owner_id,goal)VALUES('task-1','experience','owner','bounded task');INSERT INTO runs(run_id,session_key,input_event_id,status,current_state,autonomy_mode,model_provider,started_at,updated_at,task_id,task_revision)VALUES('run-1','experience','seed','completed','completed','mode_1','fixture',clock_timestamp(),clock_timestamp(),'task-1',1)`); err != nil {
		t.Fatal(err)
	}
	contract := readcontrol.Contract(readcontrol.Open)
	s, err := New(c, strings.Repeat("b", 64), toolsFixture{contract})
	if err != nil {
		t.Fatal(err)
	}
	parent, err := c.Put(ctx, catalog.PutSource{ExternalKey: "experience-original", Kind: "document", ScopeType: "user", ScopeID: "owner", Body: "Actual source observation fixture, not an inference quality claim."})
	if err != nil {
		t.Fatal(err)
	}
	o := observationFixture()
	dHash, _ := actionauth.ContractHash(contract)
	o.Environment.Tools = []skills.Dependency{{Tool: contract.ToolName, ContractHash: dHash, ContractRevision: contract.ContractRevision}}
	o.Actions[0].Tool = contract.ToolName
	o.Actions[0].ContractHash = dHash
	o.Actions[0].ContractRevision = contract.ContractRevision
	if _, err = pool.Exec(ctx, `INSERT INTO transcript_heads(session_key)VALUES('experience')ON CONFLICT DO NOTHING`); err != nil {
		t.Fatal(err)
	}
	captureFixtureTools(t, pool, ctx, &o)
	snapshot, err := s.Seal(ctx, o, []catalog.SourceRef{ref(parent)})
	if err != nil {
		t.Fatal(err)
	}
	b := budget.NewController(pool, budget.DefaultPolicy())
	b.SetAuthorityResolver(actionauth.ResolveBudgetAuthority)
	b.Policy.ModelMillis = 10000
	b.Policy.Models["antigravity:experience-fixture"] = budget.ModelPrice{Basis: "subscription_no_overage"}
	j := jobs.New(pool, c, b, nil)
	if err = j.SetModelControl(ctx, "enabled"); err != nil {
		t.Fatal(err)
	}
	raw, _ := json.Marshal(notesFixture())
	p := &noteProvider{output: string(raw)}
	w := &jobs.ModelWorker{Jobs: j, Provider: p, Model: "experience-fixture", Handlers: map[string]jobs.ModelHandler{Version: Handler{}}}
	return &lessonFixture{s, ctx, pool, parent, o, snapshot, j, w, p}
}

// Actual Transcript Store rows back this explicit controlled typed fixture.
// Only the separate app/Broker test qualifies construction from actual effects.
func captureFixtureTools(t *testing.T, pool *pgxpool.Pool, ctx context.Context, o *Observation) {
	t.Helper()
	store := transcript.NewStore(pool)
	if _, err := store.AppendToolCall(ctx, transcript.ToolCall{ToolCallID: o.Actions[0].ToolCallID, RunID: o.RunID, ToolName: o.Actions[0].Tool, Status: "effect_unknown", RuntimeTarget: "controlled-fixture", ResultJSON: `{"observed":"bounded fixture result"}`}); err != nil {
		t.Fatal(err)
	}
	snapshot, err := store.CaptureRunTools(ctx, o.RunID, 2*MaxActions)
	if err != nil {
		t.Fatal(err)
	}
	o.SessionKey = snapshot.SessionKey
	o.TranscriptRevision = snapshot.SourceRevision
	o.ToolPins = snapshot.Pins
}
func (f *lessonFixture) publish(t *testing.T) catalog.SourceRef {
	t.Helper()
	raw, _ := json.Marshal(notesFixture())
	f.p.output = string(raw)
	q := jobs.ModelPolicyRequest{ID: "experience-policy", Handler: Version, Trigger: "source_changed", IntervalSeconds: 60, MaxJobs: 1, Privacy: "cloud_allowed", Job: jobs.Request{TaskID: "task-1", OriginalRunID: "run-1", Sources: []catalog.SourceRef{f.snapshot}, Deadline: time.Now().Add(time.Hour), Allowance: budget.Units{ModelTurns: 1, Tokens: 100000, ProviderInputBytes: 8388608, ActiveMillis: 10000}}}
	if _, err := f.j.CreateModelPolicy(f.ctx, q); err != nil {
		t.Fatal(err)
	}
	if err := f.j.PollModelPolicies(f.ctx, func(string) bool { return true }); err != nil {
		t.Fatal(err)
	}
	if worked, err := f.w.ProcessOne(f.ctx); !worked || err != nil {
		t.Fatal("owned worker", worked, err)
	}
	var id string
	if err := f.pool.QueryRow(f.ctx, `SELECT source_id FROM memory_experience_lessons WHERE run_id=$1`, f.observation.RunID).Scan(&id); err != nil {
		t.Fatal(err)
	}
	src, err := f.s.Sources.Get(f.ctx, id)
	if err != nil {
		t.Fatal(err)
	}
	return ref(src)
}
func (f *lessonFixture) accept(t *testing.T, r catalog.SourceRef) {
	t.Helper()
	var claim string
	if f.pool.QueryRow(f.ctx, `SELECT claim_id FROM memory_experience_lessons WHERE source_id=$1`, r.ID).Scan(&claim) != nil {
		t.Fatal("lesson projection missing")
	}
	if _, err := f.s.Sources.Decide(f.ctx, catalog.Decision{ClaimID: claim}); err != nil {
		t.Fatal(err)
	}
}
func (f *lessonFixture) eligible(t *testing.T, r catalog.SourceRef, s *Store, p Prepared) bool {
	t.Helper()
	tx, err := f.pool.Begin(f.ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Rollback(f.ctx)
	ss, err := f.s.Sources.AccessTx(f.ctx, tx, []catalog.SourceRef{r}, "memory_read")
	if err != nil {
		return false
	}
	allowed, err := s.EligibleTx(f.ctx, tx, ss[0], p, false)
	if err != nil {
		t.Fatal(err)
	}
	return allowed
}

func TestExperienceWorkerCandidateAcceptanceEnvironmentIntegration(t *testing.T) {
	f := newLessonFixture(t)
	r := f.publish(t)
	p, err := f.s.Prepare(f.ctx, "run-1")
	if err != nil {
		t.Fatal(err)
	}
	if f.eligible(t, r, f.s, p) {
		t.Fatal("candidate became accepted experience")
	}
	f.accept(t, r)
	if !f.eligible(t, r, f.s, p) {
		t.Fatal("current accepted lesson suppressed")
	}
	if f.eligible(t, r, nil, Prepared{}) {
		t.Fatal("disabled runtime reused lesson")
	}
	for _, changed := range []toolsFixture{{readcontrol.Contract(readcontrol.Search)}, {&toolbrokerv1.ToolContract{ToolName: readcontrol.Open, Status: "active", ContractRevision: "unknown"}}} {
		s, _ := New(f.s.Sources, f.s.EnvironmentHash, changed)
		p, _ := s.Prepare(f.ctx, "run-1")
		if f.eligible(t, r, s, p) {
			t.Fatal("unknown/changed tool matched")
		}
	}
	s, _ := New(f.s.Sources, strings.Repeat("d", 64), f.s.Tools)
	other, _ := s.Prepare(f.ctx, "run-1")
	if f.eligible(t, r, s, other) {
		t.Fatal("changed environment reused lesson")
	}
	if _, err = f.w.ProcessOne(f.ctx); err != nil {
		t.Fatal(err)
	}
	if f.p.calls != 1 {
		t.Fatal("completed job repeated inference")
	}
	var attempts, episodes int
	if f.pool.QueryRow(f.ctx, `SELECT (SELECT count(*) FROM memory_model_attempts),(SELECT count(*) FROM memory_episodes)`).Scan(&attempts, &episodes) != nil || attempts != 1 || episodes != 1 {
		t.Fatal("duplicate publication")
	}
}

func TestExperienceLaterOwnerVerificationPreservesUnknownCaseIntegration(t *testing.T) {
	f := newLessonFixture(t)
	prior, err := f.s.Seal(f.ctx, f.observation, []catalog.SourceRef{ref(f.parent)})
	if err != nil || prior != f.snapshot {
		t.Fatal("exact observation retry changed identity", err)
	}
	verified := f.observation
	verified.OwnerVerdict = "verified"
	verified.OwnerResultID = "later-owner-result"
	verified.VerifiedCriteria = []string{"owner subsequently inspected actual outcome"}
	current, err := f.s.Seal(f.ctx, verified, []catalog.SourceRef{ref(f.parent)})
	if err != nil || current.ID == prior.ID {
		t.Fatal("later observation overwrote unknown history", err)
	}
	old, err := f.s.Sources.Get(f.ctx, prior.ID)
	if err != nil {
		t.Fatal(err)
	}
	var observation Observation
	if json.Unmarshal([]byte(old.Body), &observation) != nil || observation.OwnerVerdict != "unknown" {
		t.Fatal("historical unknown upgraded")
	}
	var count int
	if f.pool.QueryRow(f.ctx, `SELECT count(*) FROM memory_experience_observations`).Scan(&count) != nil || count != 2 {
		t.Fatal("immutable verification versions lost")
	}
}

func TestExperienceFeedbackAndCurrentPinsIntegration(t *testing.T) {
	f := newLessonFixture(t)
	r := f.publish(t)
	f.accept(t, r)
	if _, err := f.pool.Exec(f.ctx, `INSERT INTO runs(run_id,session_key,input_event_id,status,current_state,autonomy_mode,model_provider,started_at,updated_at,task_id,task_revision)VALUES('consumer','experience','consumer','running','model_running','mode_1','fixture',clock_timestamp(),clock_timestamp(),'task-1',1);INSERT INTO session_ownership(session_key,run_id,owner_id,lease_id,fence_epoch,expires_at)VALUES('experience','consumer','owner','lease',1,clock_timestamp()+interval '1 hour')`); err != nil {
		t.Fatal(err)
	}
	live := ownership.WithToken(context.Background(), nil, ownership.Token{SessionKey: "experience", RunID: "consumer", LeaseID: "lease", FenceEpoch: 1})
	p, _ := f.s.Prepare(live, "consumer")
	tx, _ := f.pool.Begin(live)
	ss, err := f.s.Sources.AccessTx(live, tx, []catalog.SourceRef{r}, "memory_read")
	if err != nil {
		t.Fatal(err)
	}
	if allowed, err := f.s.EligibleTx(live, tx, ss[0], p, true); err != nil || !allowed {
		t.Fatal(allowed, err)
	}
	if err = tx.Commit(live); err != nil {
		t.Fatal(err)
	}
	if err = f.s.CheckPinned(live, "consumer"); err != nil {
		t.Fatal(err)
	}
	if err = f.s.Feedback(f.ctx, r.ID, "run-1", "unknown"); err != nil {
		t.Fatal(err)
	}
	if !f.eligible(t, r, f.s, p) {
		t.Fatal("unknown fabricated negative/success")
	}
	if err = f.s.Feedback(f.ctx, r.ID, "run-1", "useful"); err == nil {
		t.Fatal("unknown overwritten")
	}
	if err = f.s.Feedback(f.ctx, r.ID, "consumer", "misapplied"); err != nil {
		t.Fatal(err)
	}
	if f.eligible(t, r, f.s, p) {
		t.Fatal("negative feedback ignored")
	}
	if f.s.CheckPinned(live, "consumer") == nil {
		t.Fatal("late negative feedback released pinned lesson")
	}
	var count int
	if f.pool.QueryRow(f.ctx, `SELECT count(*) FROM memory_experience_feedback`).Scan(&count) != nil || count != 2 {
		t.Fatal("feedback history lost")
	}
}

func TestExperienceLateCorrectionAndDeletionBlockPublicationIntegration(t *testing.T) {
	for _, mode := range []string{"correction", "deletion", "task_stop", "tool_update", "tool_delete"} {
		t.Run(mode, func(t *testing.T) {
			f := newLessonFixture(t)
			f.p.before = func() {
				switch mode {
				case "correction":
					_, err := f.s.Sources.Correct(f.ctx, catalog.Correction{SourceID: f.parent.ID, ExpectedRevision: 1, Body: "Corrected source"})
					if err != nil {
						t.Error(err)
					}
				case "deletion":
					_, err := f.s.Sources.Delete(f.ctx, f.parent.ID, "delete", 1)
					if err != nil {
						t.Error(err)
					}
				case "task_stop":
					if _, err := f.pool.Exec(f.ctx, `UPDATE tasks SET status='paused',revision=revision+1 WHERE task_id='task-1'`); err != nil {
						t.Error(err)
					}
				case "tool_update":
					if _, err := f.pool.Exec(f.ctx, `UPDATE tool_calls SET result='{"corrected":"tool observation withdrawn"}'::jsonb WHERE tool_call_id=$1`, f.observation.ToolPins[0].ID); err != nil {
						t.Error(err)
					}
				case "tool_delete":
					if _, err := f.pool.Exec(f.ctx, `DELETE FROM tool_calls WHERE tool_call_id=$1`, f.observation.ToolPins[0].ID); err != nil {
						t.Error(err)
					}
				}
			}
			q := jobs.ModelPolicyRequest{ID: "experience-late-policy", Handler: Version, Trigger: "source_changed", IntervalSeconds: 60, MaxJobs: 1, Privacy: "cloud_allowed", Job: jobs.Request{TaskID: "task-1", OriginalRunID: "run-1", Sources: []catalog.SourceRef{f.snapshot}, Deadline: time.Now().Add(time.Hour), Allowance: budget.Units{ModelTurns: 1, Tokens: 100000, ProviderInputBytes: 8388608, ActiveMillis: 10000}}}
			if _, err := f.j.CreateModelPolicy(f.ctx, q); err != nil {
				t.Fatal(err)
			}
			if err := f.j.PollModelPolicies(f.ctx, func(string) bool { return true }); err != nil {
				t.Fatal(err)
			}
			_, _ = f.w.ProcessOne(f.ctx)
			var lessons, attempts, reservations int
			if err := f.pool.QueryRow(f.ctx, `SELECT (SELECT count(*) FROM memory_experience_lessons),(SELECT count(*) FROM memory_model_attempts),(SELECT count(*) FROM execution_budget_reservations)`).Scan(&lessons, &attempts, &reservations); err != nil || lessons != 0 || attempts != 1 || reservations != 1 {
				t.Fatal("late publication/paid history lost", lessons, attempts, reservations, err)
			}
		})
	}
}
