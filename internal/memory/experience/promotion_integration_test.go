package experience

import (
	"encoding/json"
	"fmt"
	"testing"
	"time"

	"github.com/butler/butler/internal/actionauth"
	"github.com/butler/butler/internal/budget"
	"github.com/butler/butler/internal/memory/catalog"
	"github.com/butler/butler/internal/memory/jobs"
	"github.com/butler/butler/internal/skills"
)

// Typed proof fixture for the promotion predicate. The owning app/Broker test
// separately proves observation construction from real journal/results. These
// records are not a native task-success or planning-quality observation.
func (f *lessonFixture) verifiedCase(t *testing.T, n int) (catalog.SourceRef, skills.SourceProof) {
	t.Helper()
	task, run, ownerResult := fmt.Sprint("task-", n), fmt.Sprint("run-", n), fmt.Sprint("owner-", n)
	if _, err := f.pool.Exec(f.ctx, `INSERT INTO tasks(task_id,session_key,owner_id,goal)VALUES($1,'experience','owner','Independent observed case')`, task); err != nil {
		t.Fatal(err)
	}
	if _, err := f.pool.Exec(f.ctx, `INSERT INTO runs(run_id,session_key,input_event_id,status,current_state,autonomy_mode,model_provider,started_at,updated_at,task_id,task_revision)VALUES($2,'experience',$2,'completed','completed','mode_1','fixture',clock_timestamp(),clock_timestamp(),$1,1)`, task, run); err != nil {
		t.Fatal(err)
	}
	parent, err := f.s.Sources.Put(f.ctx, catalog.PutSource{ExternalKey: fmt.Sprint("independent-case-", n), Kind: "document", ScopeType: "user", ScopeID: "owner", Body: fmt.Sprint("Observed independent original case ", n)})
	if err != nil {
		t.Fatal(err)
	}
	o := f.observation
	o.TaskID = task
	o.RunID = run
	o.ResultID = fmt.Sprint("final-", n)
	o.OwnerResultID = ownerResult
	o.OwnerVerdict = "verified"
	o.VerifiedCriteria = []string{"recipient state matched"}
	o.Unknowns = []string{}
	o.Failures = []FailedCall{}
	o.Actions = append([]Action{}, o.Actions...)
	o.Actions[0].ID = fmt.Sprint("action-", n)
	o.Actions[0].ToolCallID = fmt.Sprint("observed-call-", n)
	o.Actions[0].Effect = actionauth.StateConfirmed
	o.Actions[0].Failure = ""
	o.Actions[0].Evidence = []string{fmt.Sprint("effect-evidence-", n)}
	captureFixtureTools(t, f.pool, f.ctx, &o)
	snapshot, err := f.s.Seal(f.ctx, o, []catalog.SourceRef{ref(parent)})
	if err != nil {
		t.Fatal(err)
	}
	note := notesFixture()
	note.Steps[0].ActionID = o.Actions[0].ID
	raw, _ := json.Marshal(note)
	f.p.output = string(raw)
	q := jobs.ModelPolicyRequest{ID: fmt.Sprint("independent-policy-", n), Handler: Version, Trigger: "source_changed", Privacy: "cloud_allowed", IntervalSeconds: 60, MaxJobs: 1, Job: jobs.Request{TaskID: task, OriginalRunID: run, Sources: []catalog.SourceRef{snapshot}, Deadline: time.Now().Add(time.Hour), Allowance: budget.Units{ModelTurns: 1, Tokens: 100000, ProviderInputBytes: 8388608, ActiveMillis: 10000}}}
	if _, err = f.j.CreateModelPolicy(f.ctx, q); err != nil {
		t.Fatal(err)
	}
	if err = f.j.PollModelPolicies(f.ctx, func(string) bool { return true }); err != nil {
		t.Fatal(err)
	}
	if worked, err := f.w.ProcessOne(f.ctx); !worked || err != nil {
		t.Fatal(worked, err)
	}
	var id string
	if f.pool.QueryRow(f.ctx, `SELECT source_id FROM memory_experience_lessons WHERE run_id=$1`, run).Scan(&id) != nil {
		t.Fatal("lesson missing")
	}
	source, err := f.s.Sources.Get(f.ctx, id)
	if err != nil {
		t.Fatal(err)
	}
	r := ref(source)
	f.accept(t, r)
	proof := skills.SourceProof{EvidenceRef: skills.EvidenceRef{TaskID: task, TaskRevision: 1, ResultID: ownerResult}, OwnerID: "owner", Verifier: "owner", Verdict: "verified", EvidenceRefs: []string{"owner-observation"}, ConfirmedActions: []string{o.Actions[0].ID}, ConfirmedContracts: o.Environment.Tools, ContentParents: []catalog.SourceRef{ref(parent)}}
	return r, proof
}

func TestExperiencePromotionRequiresIndependentConfirmedCasesIntegration(t *testing.T) {
	f := newLessonFixture(t)
	first, p1 := f.verifiedCase(t, 2)
	second, p2 := f.verifiedCase(t, 3)
	deps := f.observation.Environment.Tools
	if parents, err := f.s.VerifyPromotion(f.ctx, "owner", []catalog.SourceRef{first, second}, []skills.SourceProof{p1, p2}, deps); err != nil || len(parents) != 2 {
		t.Fatal("independent current cases rejected", err)
	}
	for _, proofs := range [][]skills.SourceProof{{p1}, {p1, p1}, {p2, p2}} {
		if _, err := f.s.VerifyPromotion(f.ctx, "owner", []catalog.SourceRef{first, second}, proofs, deps); err == nil {
			t.Fatal("repeated proof counted as independent")
		}
	}
	bad := p2
	bad.ConfirmedActions = []string{"unrelated-action"}
	if _, err := f.s.VerifyPromotion(f.ctx, "owner", []catalog.SourceRef{first, second}, []skills.SourceProof{p1, bad}, deps); err == nil {
		t.Fatal("unrelated effect established recipe")
	}
	unknown := f.publish(t)
	f.accept(t, unknown)
	if _, err := f.s.VerifyPromotion(f.ctx, "owner", []catalog.SourceRef{first, unknown}, []skills.SourceProof{p1, p2}, deps); err == nil {
		t.Fatal("unknown case became best practice")
	}
	if err := f.s.Feedback(f.ctx, second.ID, p2.TaskID, "negative"); err == nil {
		t.Fatal("invented feedback run accepted")
	}
	if err := f.s.Feedback(f.ctx, second.ID, "run-3", "negative"); err != nil {
		t.Fatal(err)
	}
	if _, err := f.s.VerifyPromotion(f.ctx, "owner", []catalog.SourceRef{first, second}, []skills.SourceProof{p1, p2}, deps); err == nil {
		t.Fatal("negative experience promoted")
	}
	if _, err := f.s.Sources.Delete(f.ctx, f.parent.ID, "delete", f.parent.Revision); err != nil {
		t.Fatal(err)
	}
	if _, err := f.s.Sources.Get(f.ctx, unknown.ID); err == nil {
		t.Fatal("deleted ancestor left unknown lesson readable")
	}
}
