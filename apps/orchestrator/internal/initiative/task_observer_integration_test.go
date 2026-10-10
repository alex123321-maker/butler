package initiative

import (
	"encoding/json"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/butler/butler/apps/orchestrator/internal/task"
)

func projectAttempt(t *testing.T, f *fixture, id, origin, revision string, taskRevision int64) {
	t.Helper()
	r, err := f.promoter.Tasks.Create(f.ctx, task.CreateRequest{SessionKey: f.source.SessionKey, OwnerID: f.source.OwnerID, Goal: "Run fixture checks", Source: map[string]any{"type": origin, "project_ref": f.source.ProjectRef, "project_revision": revision}, CompletionCriteria: []string{"fixture checks pass"}})
	if err != nil {
		t.Fatal(err)
	}
	if _, err = f.pool.Exec(f.ctx, `INSERT INTO runs(run_id,session_key,input_event_id,status,autonomy_mode,current_state,model_provider,started_at,updated_at,finished_at,task_id,task_revision,error_message) VALUES($1,$2,$1,'failed','mode_1','failed','test',clock_timestamp(),clock_timestamp(),clock_timestamp(),$3,$4,'UNTRUSTED_RAW_ERROR_DO_NOT_PUBLISH')`, id, r.SessionKey, r.TaskID, taskRevision); err != nil {
		t.Fatal(err)
	}
}

func TestTaskObserverUsesDistinctCurrentOwnerProjectAttemptsIntegration(t *testing.T) {
	f := setup(t)
	f.source.ObserverKind, f.source.ProjectRevision = "task_project", hash("fixture-revision")
	if _, err := f.store.SaveSource(f.actor("observer-config"), f.source, 1); err != nil {
		t.Fatal(err)
	}
	f.source.Revision = 2
	o := TaskObserver{Store: f.store}
	projectAttempt(t, f, "stale-task-revision", "owner_goal_task", f.source.ProjectRevision, 2)
	projectAttempt(t, f, "other-project-revision", "owner_goal_task", hash("other-revision"), 1)
	if e, err := o.Check(f.ctx, f.source, ClaimedTick{}); err != nil || e != nil {
		t.Fatal("stale attempt observed", err)
	}
	for i := 0; i < 3; i++ {
		projectAttempt(t, f, fmt.Sprintf("fixture-failure-%d", i), "owner_goal_task", f.source.ProjectRevision, 1)
	}
	var candidate Candidate
	for i := 0; i < 3; i++ {
		e, err := o.Check(f.ctx, f.source, ClaimedTick{})
		if err != nil || e == nil {
			t.Fatal("missing independent failure", err)
		}
		e.ID = fmt.Sprintf("observed-%d", i)
		e.SourceID = f.source.ID
		encoded, _ := json.Marshal(e)
		if strings.Contains(string(encoded), "UNTRUSTED_RAW_ERROR") || e.ObservedAttemptID == "" || e.ResourceRevision != f.source.ProjectRevision {
			t.Fatal("unbound observation")
		}
		candidate, _, err = f.store.Observe(f.ctx, f.source.OwnerID, *e, 2, time.Now().UTC())
		if err != nil {
			t.Fatal(err)
		}
	}
	if candidate.Occurrences != 3 || candidate.Status != "ready" {
		t.Fatal("distinct project failures did not create candidate", candidate)
	}
	if e, err := o.Check(f.ctx, f.source, ClaimedTick{}); err != nil || e != nil {
		t.Fatal("failure delivery counted again", err)
	}
	projectAttempt(t, f, "own-attempt", "initiative", f.source.ProjectRevision, 1)
	e, err := o.Check(f.ctx, f.source, ClaimedTick{})
	if err != nil || e == nil || !e.ProducedByButler {
		t.Fatal("own origin lost", err)
	}
	e.ID = "own-delivery"
	e.SourceID = f.source.ID
	own, _, err := f.store.Observe(f.ctx, f.source.OwnerID, *e, 2, time.Now().UTC())
	if err != nil || own.Reason != "own_change" {
		t.Fatal("own failure created a loop", err)
	}
	wrong := f.source
	wrong.OwnerID = "another-owner"
	if e, err = o.Check(f.ctx, wrong, ClaimedTick{}); err != nil || e != nil {
		t.Fatal("foreign owner observed", err)
	}
}

func TestVerifiedProjectResultClearsUnconsumedFailuresIntegration(t *testing.T) {
	f := setup(t)
	f.source.ObserverKind, f.source.ProjectRevision = "task_project", hash("fixture-revision")
	projectAttempt(t, f, "old-project-failure", "owner_goal_task", f.source.ProjectRevision, 1)
	r, err := f.promoter.Tasks.Create(f.ctx, task.CreateRequest{SessionKey: f.source.SessionKey, OwnerID: f.source.OwnerID, Goal: "Prepared correction", Source: map[string]any{"type": "initiative", "project_ref": f.source.ProjectRef, "project_revision": f.source.ProjectRevision}, CompletionCriteria: []string{"checks pass"}})
	if err != nil {
		t.Fatal(err)
	}
	if _, err = f.pool.Exec(f.ctx, `INSERT INTO task_results(result_id,task_id,task_revision,summary,evidence_refs_json,verified_criteria_json,verdict,verifier) VALUES('verified-project-fixture',$1,1,'checked fixture','["checks:fixture"]','["checks pass"]','verified','owner')`, r.TaskID); err != nil {
		t.Fatal(err)
	}
	if _, err = f.pool.Exec(f.ctx, `UPDATE tasks SET status='completed',completion_result_id='verified-project-fixture',finished_at=clock_timestamp() WHERE task_id=$1`, r.TaskID); err != nil {
		t.Fatal(err)
	}
	e, err := (TaskObserver{Store: f.store}).Check(f.ctx, f.source, ClaimedTick{})
	if err != nil || e == nil || e.Kind != "project_healthy" {
		t.Fatal("verified result did not clear old failure", err)
	}
}
