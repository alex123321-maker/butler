package initiative

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/butler/butler/apps/orchestrator/internal/task"
	"github.com/butler/butler/internal/actionauth"
	"github.com/butler/butler/internal/extensions"
	"github.com/butler/butler/internal/extensions/projectfixture"
	toolbrokerv1 "github.com/butler/butler/internal/gen/toolbroker/v1"
	"github.com/jackc/pgx/v5"
)

func setupBrokerObserver(t *testing.T) (*fixture, extensions.Package) {
	t.Helper()
	f := setup(t)
	f.store.RunDataGuardTx = func(context.Context, pgx.Tx, string) error { return nil } // fixture has no source lifecycle records
	p, err := projectfixture.Manifest(hash("fixture-runtime-artifact"), "reviewed-fixture-v1")
	if err != nil {
		t.Fatal(err)
	}
	s := extensions.NewStore(f.pool, f.source.OwnerID, map[string]extensions.Profile{"reference-http": {Target: "fixture-runtime:9090", Dependency: p.Manifest.Dependencies[0]}}, nil)
	if _, err = s.Stage(f.actor("extension-stage"), p); err != nil {
		t.Fatal(err)
	}
	if err = s.Review(f.actor("extension-review"), p.Manifest.ID, p.Digest); err != nil {
		t.Fatal(err)
	}
	f.source.ObserverKind, f.source.ProjectRef, f.source.ProjectRevision = "broker_project", projectfixture.ProjectRef, projectfixture.Revision()
	f.source.CheckManifestHash, f.source.CheckToolName, f.source.CheckResource = p.Digest, "extension.project.inspect", "https://example.invalid/project"
	if _, err = f.store.SaveSource(f.actor("broker-source"), f.source, 1); err != nil {
		t.Fatal(err)
	}
	f.source.Revision = 2
	return f, p
}

// A durable fixture journal is used for parser/current-version adversarial tests.
// The separate app E2E exercises the actual Broker/runtime/approval processes.
func fixtureBrokerFact(t *testing.T, f *fixture, p extensions.Package, attempt, callID, origin string, passed bool) brokerFact {
	t.Helper()
	var record task.Record
	var err error
	if attempt == "" {
		record, err = f.promoter.Tasks.Create(f.ctx, task.CreateRequest{SessionKey: f.source.SessionKey, OwnerID: f.source.OwnerID, Goal: "Inspect fixed project", Source: map[string]any{"type": origin, "project_ref": f.source.ProjectRef, "project_revision": f.source.ProjectRevision}, CompletionCriteria: []string{"project check observed"}})
		if err != nil {
			t.Fatal(err)
		}
		attempt = "attempt-" + callID
		_, err = f.pool.Exec(f.ctx, `INSERT INTO runs(run_id,session_key,input_event_id,status,autonomy_mode,current_state,model_provider,started_at,updated_at,finished_at,task_id,task_revision) VALUES($1,$2,$1,'completed','mode_1','completed','fixture',clock_timestamp(),clock_timestamp(),clock_timestamp(),$3,$4)`, attempt, record.SessionKey, record.TaskID, record.Revision)
		if err != nil {
			t.Fatal(err)
		}
	} else {
		var taskID string
		if err = f.pool.QueryRow(f.ctx, `SELECT task_id FROM runs WHERE run_id=$1`, attempt).Scan(&taskID); err != nil {
			t.Fatal(err)
		}
		record, err = f.promoter.Tasks.Get(f.ctx, taskID)
		if err != nil {
			t.Fatal(err)
		}
	}
	at := time.Now().UTC()
	args, _ := json.Marshal(map[string]string{"url": f.source.CheckResource})
	contract := &toolbrokerv1.ToolContract{ToolName: f.source.CheckToolName, ToolClass: "extension", Status: "enabled", ContractRevision: p.Digest + ":1", RuntimeTarget: "fixture-runtime:9090", InputSchemaJson: string(p.Manifest.Operations[0].InputSchema), OutputSchemaJson: string(p.Manifest.Operations[0].OutputSchema), Extension: &toolbrokerv1.ExtensionContract{ExtensionId: p.Manifest.ID, ManifestHash: p.Digest, Generation: 1, Method: "GET", Effect: "read"}}
	b, _, err := actionauth.BuildBinding("action-"+callID, &toolbrokerv1.ToolCall{RunId: attempt, ToolCallId: callID, ToolName: f.source.CheckToolName, ArgsJson: string(args)}, contract, actionauth.ContextSnapshot{RunID: attempt, OwnerID: f.source.OwnerID, TaskID: record.TaskID, TaskRevision: record.Revision, ConstraintsRevision: record.ConstraintsRevision})
	if err != nil {
		t.Fatal(err)
	}
	b = actionauth.BindExtensionAccess(b, 1, at.Add(time.Hour))
	encoded, _ := json.Marshal(b)
	errorKind := "ready_flag_false"
	if passed {
		errorKind = ""
	}
	body, _ := json.Marshal(map[string]any{"observation_format": "project_check_v1", "project_ref": f.source.ProjectRef, "project_revision": f.source.ProjectRevision, "checks_passed": passed, "check_id": hash([]any{"ready_flag", passed}), "error_kind": errorKind, "produced_by_butler": origin != "initiative", "untrusted_text": "MODEL_INSTRUCTION_DO_NOT_EXPOSE"})
	result, _ := json.Marshal(map[string]any{"status_code": 200, "body": string(body)})
	g := actionauth.Grant{ID: "grant-" + callID, Binding: b, State: "dispatch_completed", ExpiresAt: at.Add(time.Hour)}
	grant, _ := json.Marshal(g)
	_, err = f.pool.Exec(f.ctx, `INSERT INTO action_effects(action_id,fingerprint,initial_binding,policy,state) VALUES($1,$2,$3::jsonb,'{"retry_safety":"read_only","verification_method":"read_result"}','effect_confirmed');`, b.ActionID, b.ActionHash, encoded)
	if err != nil {
		t.Fatal(err)
	}
	_, err = f.pool.Exec(f.ctx, `INSERT INTO action_intents(action_id,run_id,tool_call_id,action_hash,binding,requires_approval,expires_at,effect_id) VALUES($1,$2,$3,$4,$5::jsonb,true,$6,$1)`, b.ActionID, attempt, callID, b.ActionHash, encoded, g.ExpiresAt)
	if err != nil {
		t.Fatal(err)
	}
	_, err = f.pool.Exec(f.ctx, `INSERT INTO action_authorizations(authorization_id,action_id,action_hash,grant_data,expires_at,state,dispatched_at,finished_at) VALUES($1,$2,$3,$4::jsonb,$5,'dispatch_completed',$6,$6)`, g.ID, b.ActionID, b.ActionHash, grant, g.ExpiresAt, at)
	if err != nil {
		t.Fatal(err)
	}
	_, err = f.pool.Exec(f.ctx, `INSERT INTO action_effect_dispatches(authorization_id,action_id,execution_action_id,outcome,dispatched_at,finished_at) VALUES($1,$2,$2,'dispatch_completed',$3,$3)`, g.ID, b.ActionID, at)
	if err != nil {
		t.Fatal(err)
	}
	_, err = f.pool.Exec(f.ctx, `INSERT INTO tool_calls(tool_call_id,run_id,tool_name,args,status,runtime_target,started_at,finished_at,result) VALUES($1,$2,$3,$4::jsonb,'completed','fixture-runtime:9090',$5,$5,$6::jsonb)`, callID, attempt, b.ToolName, args, at, result)
	if err != nil {
		t.Fatal(err)
	}
	return brokerFact{RunID: attempt, CallID: callID, TaskID: record.TaskID, TaskRevision: record.Revision, ConstraintsRevision: record.ConstraintsRevision, Own: origin == "initiative", StartedAt: at, At: at, Args: args, Binding: encoded, Result: result}
}

func observeBrokerFact(t *testing.T, f *fixture, observer ProjectObserver, id string) (Candidate, *Event) {
	t.Helper()
	e, err := observer.Check(f.ctx, f.source, ClaimedTick{})
	if err != nil || e == nil {
		t.Fatal("missing bounded broker fact", err)
	}
	e.ID, e.SourceID = id, f.source.ID
	c, _, err := f.store.Observe(f.ctx, f.source.OwnerID, *e, f.source.Revision, time.Now().UTC())
	if err != nil {
		t.Fatal(err)
	}
	return c, e
}

func TestBrokerProjectObserverCountsDistinctAttemptsAndPinsEvidenceIntegration(t *testing.T) {
	f, p := setupBrokerObserver(t)
	o := ProjectObserver{Store: f.store, RunDataGuard: func(context.Context, string) error { return nil }}
	first := fixtureBrokerFact(t, f, p, "", "check-1", "owner_goal_task", false)
	_, e := observeBrokerFact(t, f, o, "observed-1")
	encoded, _ := json.Marshal(e)
	if strings.Contains(string(encoded), "MODEL_INSTRUCTION") || e.ProducedByButler || e.ObservedToolCallID != first.CallID {
		t.Fatal("untrusted result/origin became control data")
	}
	fixtureBrokerFact(t, f, p, first.RunID, "check-repeat", "owner_goal_task", false)
	if e, err := o.Check(f.ctx, f.source, ClaimedTick{}); err != nil || e != nil {
		t.Fatal("one run counted twice", err)
	}
	fixtureBrokerFact(t, f, p, "", "check-2", "owner_goal_task", false)
	observeBrokerFact(t, f, o, "observed-2")
	fixtureBrokerFact(t, f, p, "", "check-3", "owner_goal_task", false)
	c, _ := observeBrokerFact(t, f, o, "observed-3")
	if c.Status != "ready" || c.Occurrences != 3 {
		t.Fatal("three verified independent checks did not qualify", c)
	}
	if _, err := f.promoter.Promote(f.ctx, c.ID, f.source.OwnerID); err != nil {
		t.Fatal(err)
	}
	fixtureBrokerFact(t, f, p, "", "check-own", "initiative", false)
	c, e = observeBrokerFact(t, f, o, "observed-own")
	if !e.ProducedByButler || c.Reason != "own_change" {
		t.Fatal("own check produced a loop")
	}
}

func TestBrokerProjectObserverRejectsStaleUnknownAndChangedConditionsIntegration(t *testing.T) {
	for _, mode := range []string{"task_revision", "unconfirmed", "binding_hash", "wrong_resource", "release_denied"} {
		t.Run(mode, func(t *testing.T) {
			f, p := setupBrokerObserver(t)
			fact := fixtureBrokerFact(t, f, p, "", "check-boundary", "owner_goal_task", false)
			o := ProjectObserver{Store: f.store, RunDataGuard: func(context.Context, string) error { return nil }}
			var err error
			switch mode {
			case "task_revision":
				_, err = f.pool.Exec(f.ctx, `UPDATE tasks SET revision=revision+1 WHERE task_id=$1`, fact.TaskID)
			case "unconfirmed":
				_, err = f.pool.Exec(f.ctx, `UPDATE action_effects SET state='effect_unknown'`)
			case "binding_hash":
				_, err = f.pool.Exec(f.ctx, `UPDATE action_intents SET binding=jsonb_set(binding,'{args_hash}','"ffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffff"')`)
			case "wrong_resource":
				f.source.CheckResource = "https://example.invalid/other-project"
			case "release_denied":
				o.RunDataGuard = func(_ context.Context, _ string) error { return ErrConflict }
			}
			if err != nil {
				t.Fatal(err)
			}
			if e, err := o.Check(f.ctx, f.source, ClaimedTick{}); err != nil || e != nil {
				t.Fatal("invalid historical fact observed", mode, err)
			}
		})
	}
	for _, healthy := range []bool{false, true} {
		t.Run(fmt.Sprintf("new-condition-healthy-%t", healthy), func(t *testing.T) {
			f, p := setupBrokerObserver(t)
			o := ProjectObserver{Store: f.store, RunDataGuard: func(context.Context, string) error { return nil }}
			var candidate Candidate
			for n := 0; n < 3; n++ {
				fixtureBrokerFact(t, f, p, "", fmt.Sprintf("condition-%d", n), "owner_goal_task", false)
				candidate, _ = observeBrokerFact(t, f, o, fmt.Sprintf("condition-event-%d", n))
			}
			fact := fixtureBrokerFact(t, f, p, "", "condition-new", "owner_goal_task", healthy)
			if !healthy {
				var envelope map[string]any
				json.Unmarshal(fact.Result, &envelope)
				var body map[string]any
				json.Unmarshal([]byte(envelope["body"].(string)), &body)
				body["check_id"] = hash("different-check")
				raw, _ := json.Marshal(body)
				envelope["body"] = string(raw)
				result, _ := json.Marshal(envelope)
				if _, err := f.pool.Exec(f.ctx, `UPDATE tool_calls SET result=$2::jsonb WHERE tool_call_id=$1`, fact.CallID, result); err != nil {
					t.Fatal(err)
				}
			}
			updated, err := f.promoter.Promote(f.ctx, candidate.ID, f.source.OwnerID)
			if err != nil || updated.Reason != "condition_obsolete" || updated.TaskID != "" {
				t.Fatal("obsolete condition admitted", updated, err)
			}
		})
	}
}

func TestBrokerProjectBodyMustBeUniqueAndSourceBoundIntegration(t *testing.T) {
	f, p := setupBrokerObserver(t)
	fact := fixtureBrokerFact(t, f, p, "", "check-parser", "owner_goal_task", false)
	for _, mutation := range []string{"duplicate", "missing_boolean", "wrong_revision", "model_origin"} {
		copyFact := fact
		var envelope map[string]any
		json.Unmarshal(fact.Result, &envelope)
		var body map[string]any
		json.Unmarshal([]byte(envelope["body"].(string)), &body)
		switch mutation {
		case "duplicate":
			envelope["body"] = `{"checks_passed":false,"checks_passed":true}`
		case "missing_boolean":
			delete(body, "checks_passed")
		case "wrong_revision":
			body["project_revision"] = hash("another revision")
		case "model_origin":
			body["produced_by_butler"] = true
		}
		if mutation != "duplicate" {
			raw, _ := json.Marshal(body)
			envelope["body"] = string(raw)
		}
		copyFact.Result, _ = json.Marshal(envelope)
		e := copyFact.event(f.source, p)
		if mutation == "model_origin" {
			if e == nil || e.ProducedByButler {
				t.Fatal("body granted origin")
			}
		} else if e != nil {
			t.Fatal("invalid check accepted", mutation)
		}
	}
}

func TestBrokerProjectPromotionRequiresLiveSourceReleaseIntegration(t *testing.T) {
	f, p := setupBrokerObserver(t)
	for n := 0; n < 3; n++ {
		fixtureBrokerFact(t, f, p, "", fmt.Sprintf("release-%d", n), "owner_goal_task", false)
	}
	if _, err := (ProjectObserver{Store: f.store}).Check(f.ctx, f.source, ClaimedTick{}); err != ErrInvalid {
		t.Fatal("missing live release guard allowed observer", err)
	}
	o := ProjectObserver{Store: f.store, RunDataGuard: func(context.Context, string) error { return nil }}
	var candidate Candidate
	for n := 0; n < 3; n++ {
		candidate, _ = observeBrokerFact(t, f, o, fmt.Sprintf("release-event-%d", n))
	}
	checks := 0
	f.store.RunDataGuardTx = func(context.Context, pgx.Tx, string) error {
		checks++
		if checks > 1 {
			return ErrConflict
		}
		return nil
	}
	updated, err := f.promoter.Promote(f.ctx, candidate.ID, f.source.OwnerID)
	if err != nil || checks != 2 || updated.Reason != "condition_obsolete" || updated.TaskID != "" {
		t.Fatal("late source release changed but admitted work", updated, checks, err)
	}
}
