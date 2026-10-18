package experience

import (
	"context"
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/butler/butler/internal/actionauth"
	toolbrokerv1 "github.com/butler/butler/internal/gen/toolbroker/v1"
	"github.com/butler/butler/internal/memory/catalog"
	"github.com/butler/butler/internal/memory/readcontrol"
	"github.com/butler/butler/internal/memory/reader"
	"github.com/butler/butler/internal/ownership"
	"github.com/butler/butler/internal/skills"
	"github.com/jackc/pgx/v5"
)

// These arbitrary DATA projections exercise the shared ancestry boundary; they
// are not a claim that a summary/consolidation model produced useful content.
func TestExperienceAllDerivedDataRetainsApplicabilityIntegration(t *testing.T) {
	for _, mode := range []string{"healthy", "environment", "unknown_environment", "negative", "contract_revision"} {
		t.Run(mode, func(t *testing.T) {
			f := newLessonFixture(t)
			lesson := f.publish(t)
			f.accept(t, lesson)
			refs := []catalog.SourceRef{lesson}
			for _, kind := range []string{"summary", "consolidation", "reviewed_skill_data"} {
				tx, err := f.pool.Begin(f.ctx)
				if err != nil {
					t.Fatal(err)
				}
				src, err := f.s.Sources.CreateDerivedTx(f.ctx, tx, []catalog.SourceRef{refs[len(refs)-1]}, "memory_extract", kind, "Observed derived presentation", json.RawMessage(`{}`))
				if err != nil {
					tx.Rollback(f.ctx)
					t.Fatal(err)
				}
				if err = tx.Commit(f.ctx); err != nil {
					t.Fatal(err)
				}
				refs = append(refs, ref(src))
			}
			current := f.s
			p, err := current.Prepare(f.ctx, "run-1")
			if err != nil {
				t.Fatal(err)
			}
			switch mode {
			case "environment":
				current, _ = New(f.s.Sources, strings.Repeat("e", 64), f.s.Tools)
				p, _ = current.Prepare(f.ctx, "run-1")
			case "unknown_environment":
				current = nil
				p = Prepared{}
			case "negative":
				if err = f.s.Feedback(f.ctx, lesson.ID, "run-1", "negative"); err != nil {
					t.Fatal(err)
				}
			case "contract_revision":
				c := readcontrol.Contract(readcontrol.Open)
				c.ContractRevision += "-changed"
				current, _ = New(f.s.Sources, f.s.EnvironmentHash, toolsFixture{c})
				p, _ = current.Prepare(f.ctx, "run-1")
			}
			for _, r := range refs {
				if allowed := f.eligible(t, r, current, p); allowed != (mode == "healthy") {
					t.Fatal("ancestor applicability lost", mode, r.ID, allowed)
				}
			}
		})
	}
}

func (f *lessonFixture) consumer(t *testing.T) (context.Context, actionauth.ContextSnapshot) {
	t.Helper()
	if _, err := f.pool.Exec(f.ctx, `INSERT INTO runs(run_id,session_key,input_event_id,status,current_state,autonomy_mode,model_provider,started_at,updated_at,task_id,task_revision)VALUES('consumer','experience','consumer','running','model_running','mode_1','fixture',clock_timestamp(),clock_timestamp(),'task-1',1);INSERT INTO session_ownership(session_key,run_id,owner_id,lease_id,fence_epoch,expires_at)VALUES('experience','consumer','owner','lease',1,clock_timestamp()+interval '1 hour')`); err != nil {
		t.Fatal(err)
	}
	return ownership.WithToken(context.Background(), nil, ownership.Token{SessionKey: "experience", RunID: "consumer", LeaseID: "lease", FenceEpoch: 1}), actionauth.ContextSnapshot{RunID: "consumer", RunState: "model_running", OwnerID: "owner", SessionKey: "experience", TaskID: "task-1", PinnedTaskRevision: 1, TaskRevision: 1, ConstraintsRevision: 1, TaskStatus: "active", LeaseID: "lease", LeaseEpoch: 1, LeaseExpiresAt: time.Now().Add(time.Hour), ExperienceEnvironmentHash: f.s.EnvironmentHash}
}

func TestExperienceTrustedSubsetPrecedesSearchRankingAndOpenIntegration(t *testing.T) {
	f := newLessonFixture(t)
	lesson := f.publish(t)
	f.accept(t, lesson)
	live, _ := f.consumer(t)
	p, err := f.s.Prepare(live, "consumer")
	if err != nil {
		t.Fatal(err)
	}
	r := reader.Reader{Sources: f.s.Sources, SourceSubset: []catalog.SourceRef{lesson}, SourceEligibility: func(ctx context.Context, tx pgx.Tx, src catalog.Source) (bool, error) {
		return f.s.EligibleTx(ctx, tx, src, p, true)
	}}
	tx, err := f.pool.Begin(live)
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Rollback(live)
	result, err := r.ReadTx(live, tx, readcontrol.Search, readcontrol.Args{Query: "observed", Limit: 1, MaxChars: 512})
	if err != nil || len(result.Items) != 1 || result.Items[0].Source.ID != lesson.ID {
		t.Fatal("trusted subset not applied before ranked limit", err, result.Status)
	}
	denied, err := r.ReadTx(live, tx, readcontrol.Open, readcontrol.Args{SourceID: f.parent.ID, MaxChars: 512})
	if err != nil || len(denied.Items) != 0 || denied.Status != "denied" {
		t.Fatal("ancestor body escaped treatment", err)
	}
	if err = tx.Commit(live); err != nil {
		t.Fatal(err)
	}
}

type lessonSkillVerifier struct {
	f      *lessonFixture
	proofs []skills.SourceProof
}

func (v lessonSkillVerifier) VerifySkillSources(_ context.Context, owner string, refs []skills.EvidenceRef) ([]skills.SourceProof, error) {
	out := []skills.SourceProof{}
	for _, r := range refs {
		found := false
		for _, p := range v.proofs {
			if p.EvidenceRef == r && p.OwnerID == owner {
				out = append(out, p)
				found = true
				break
			}
		}
		if !found {
			return nil, skills.ErrEvidence
		}
	}
	return out, nil
}
func (v lessonSkillVerifier) VerifyLessonLineage(ctx context.Context, owner string, q skills.LessonRequest) ([]catalog.SourceRef, error) {
	required, err := v.f.s.Ancestors(ctx, q.Roots)
	if err != nil {
		return nil, err
	}
	if len(required) > 0 && len(q.Lessons) == 0 {
		return nil, skills.ErrEvidence
	}
	return v.f.s.VerifyPromotion(ctx, owner, q.Lessons, q.Proofs, q.Dependencies)
}

type lessonSkillEnvironment struct {
	snapshot actionauth.ContextSnapshot
	contract *toolbrokerv1.ToolContract
}

func (e *lessonSkillEnvironment) GetActionContext(context.Context, string) (actionauth.ContextSnapshot, error) {
	return e.snapshot, nil
}
func (e *lessonSkillEnvironment) ContractsForRun(context.Context, string) ([]*toolbrokerv1.ToolContract, error) {
	return []*toolbrokerv1.ToolContract{e.contract}, nil
}
func (e *lessonSkillEnvironment) GetSkillContract(context.Context, string) (*toolbrokerv1.ToolContract, error) {
	return e.contract, nil
}
func (e *lessonSkillEnvironment) GetSkillContractTx(context.Context, pgx.Tx, string) (*toolbrokerv1.ToolContract, error) {
	return e.contract, nil
}

func TestExperienceReviewedSkillRetainsAncestorPinsIntegration(t *testing.T) {
	for _, mode := range []string{"healthy", "environment", "unknown_environment", "negative", "contract_revision"} {
		t.Run(mode, func(t *testing.T) {
			f := newLessonFixture(t)
			first, p1 := f.verifiedCase(t, 2)
			second, p2 := f.verifiedCase(t, 3)
			live, snapshot := f.consumer(t)
			environment := &lessonSkillEnvironment{snapshot: snapshot, contract: readcontrol.Contract(readcontrol.Open)}
			verifier := lessonSkillVerifier{f: f, proofs: []skills.SourceProof{p1, p2}}
			store := skills.NewStore(f.pool, "owner", verifier, environment, environment)
			store.SetSources(f.s.Sources)
			m := skills.Manifest{Format: skills.Format, ID: "controlled-experience", Version: "1.0.0", Source: skills.Source{URI: "https://fixture.invalid/controlled", Revision: "independent-cases"}, Description: "Controlled component predicate, not native quality", When: []string{"current exact environment"}, Inputs: []string{"current source"}, NotApplicable: []string{"changed environment"}, Dependencies: f.observation.Environment.Tools, Steps: []skills.Step{{ID: "read", Tool: readcontrol.Open, Intent: "Observe current source", SuccessCheck: "Current read receipt", OnError: "stop", Retry: "read_again"}}, Tests: []skills.Scenario{{Name: "success", Kind: "verified_success", Expected: "Independent confirmed original"}, {Name: "stop", Kind: "failure_stop", Expected: "Stop on unknown"}, {Name: "change", Kind: "interface_change", Expected: "Reject changed interface"}}, Sources: []skills.EvidenceRef{p1.EvidenceRef, p2.EvidenceRef}, LessonSources: []catalog.SourceRef{first, second}}
			raw, _ := json.Marshal(m)
			pack, err := skills.Parse(raw)
			if err != nil {
				t.Fatal(err)
			}
			record, err := store.Stage(f.ctx, pack)
			if err != nil {
				t.Fatal("actual skill candidate", err)
			}
			if report, err := store.Test(f.ctx, pack.Digest); err != nil || !report.Passed {
				t.Fatal("actual skill validation", err)
			}
			if _, err = store.Activate(f.ctx, pack.Digest, record.Generation); err != nil {
				t.Fatal("actual owner CAS activation", err)
			}
			if index, err := store.Index(live, "consumer", "", 12); err != nil || len(index) != 1 {
				t.Fatal("current skill description suppressed", err)
			}
			if _, err = store.Load(live, "consumer", pack.Digest, 16384); err != nil {
				t.Fatal("current reviewed skill load", err)
			}
			var pins int
			if f.pool.QueryRow(f.ctx, `SELECT count(*) FROM memory_experience_pins WHERE run_id='consumer'`).Scan(&pins) != nil || pins != 2 {
				t.Fatal("skill lost lesson ancestors", pins)
			}
			if err = f.s.CheckPinned(live, "consumer"); err != nil {
				t.Fatal("healthy final ancestry guard", err)
			}
			switch mode {
			case "environment":
				environment.snapshot.ExperienceEnvironmentHash = strings.Repeat("e", 64)
			case "unknown_environment":
				environment.snapshot.ExperienceEnvironmentHash = ""
			case "negative":
				if err = f.s.Feedback(f.ctx, first.ID, "run-1", "misapplied"); err != nil {
					t.Fatal(err)
				}
			case "contract_revision":
				environment.contract = readcontrol.Contract(readcontrol.Open)
				environment.contract.ContractRevision += "-changed"
			}
			if mode == "contract_revision" {
				f.s.Tools = toolsFixture{environment.contract}
			}
			index, err := store.Index(live, "consumer", "", 12)
			if err != nil {
				t.Fatal(err)
			}
			_, loadErr := store.Load(live, "consumer", pack.Digest, 16384)
			if mode == "healthy" {
				if len(index) != 1 || loadErr != nil {
					t.Fatal("healthy replay denied", loadErr)
				}
			} else if len(index) != 0 || loadErr == nil {
				t.Fatal("derived skill bypassed current lesson condition", mode, loadErr)
			}
			current := f.s
			if mode == "environment" {
				current, _ = New(f.s.Sources, environment.snapshot.ExperienceEnvironmentHash, f.s.Tools)
			} else if mode == "unknown_environment" {
				current = nil
			}
			if (current.CheckPinned(live, "consumer") == nil) != (mode == "healthy") {
				t.Fatal("current final release missed derived skill lesson condition", mode)
			}
		})
	}
}
