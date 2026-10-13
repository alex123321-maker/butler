package app

import (
	"encoding/json"
	"testing"

	"github.com/butler/butler/internal/actionauth"
	"github.com/butler/butler/internal/extensions/projectfixture"
	"github.com/butler/butler/internal/generatedtools"
	"github.com/butler/butler/internal/security/egress/egresstest"
	"github.com/butler/butler/internal/transport/antigravity"
)

func TestReleaseChainV15TaskProofLegacyOptIn(t *testing.T) {
	if (releaseChainHighEntry{}).TaskScopedOriginalProof || (releaseChainModelConfig{}).TaskScopedOriginalProof || releaseChainV15TaskScopedProof(nil) {
		t.Fatal("legacy phase verifier changed")
	}
}

// The journal setup is synthetic, explicitly not a native/Broker dispatch
// claim. The PG joins and receipt parser use real stored rows and actual HTTP
// GET responses; faults retain that same fixture and break one proof boundary.
func TestReleaseChainV15TaskScopedOriginalProofIntegration(t *testing.T) {
	for _, kind := range []string{"previous_phase_qualified_calls", "stale_run_revision", "foreign_owner_binding", "unconfirmed_dispatch", "unknown_effect", "unfinished_phase", "stale_current_task"} {
		t.Run(kind, func(t *testing.T) {
			f := newDeveloperFixture(t)
			get, artifact, directory, prepare := v12RealRecipient(t)
			f.projectDirectory = directory
			f.modelConfig = &releaseChainModelConfig{Provider: antigravity.Name, Model: antigravity.FlashHighModel, AllowRepeatedPatchChecks: true, TaskScopedOriginalProof: true}
			if _, err := f.f.pool.Exec(f.f.ctx, `UPDATE runs SET status='completed',current_state='completed',finished_at=clock_timestamp() WHERE run_id=$1`, f.parentRun); err != nil {
				t.Fatal(err)
			}
			// A separate run of the same root legitimately reuses a call ID. No
			// global tool-call uniqueness rule is introduced by owner verification.
			checkRun := "later-original-phase"
			if _, err := f.f.pool.Exec(f.f.ctx, `INSERT INTO runs(run_id,session_key,input_event_id,status,current_state,autonomy_mode,model_provider,started_at,updated_at,finished_at,task_id,task_revision) VALUES($1,$2,'synthetic-original-phase','completed','completed','mode_2','fixture',clock_timestamp(),clock_timestamp(),clock_timestamp(),$3,$4)`, checkRun, f.f.parent.SessionKey, f.f.parent.TaskID, f.f.parent.Revision); err != nil {
				t.Fatal(err)
			}
			store := func(run, call, tool string, args []byte, result string) {
				t.Helper()
				id := generatedtools.HashBytes([]byte(run + "\x00" + call))
				binding := actionauth.ActionBinding{ActionID: "proof-action-" + id, ActionHash: id, RunID: run, ToolCallID: call, TaskID: f.f.parent.TaskID, TaskRevision: f.f.parent.Revision, ConstraintsRevision: f.f.parent.ConstraintsRevision, OwnerID: f.f.parent.OwnerID, ToolName: tool, ContractHash: id, ArgsHash: id, Resource: "tool:" + tool, Effect: actionauth.EffectRead}
				if tool == "extension.project.prepare-patch" {
					binding.Effect = actionauth.EffectPrepare
				}
				if kind == "foreign_owner_binding" && tool == "extension.project.check" {
					binding.OwnerID = "foreign"
				}
				raw, _ := json.Marshal(binding)
				effect, auth := "proof-effect-"+id, "proof-auth-"+id
				state, outcome := "effect_confirmed", "dispatch_completed"
				if kind == "unknown_effect" && tool == "extension.project.check" {
					state = "effect_unknown"
				}
				if _, err := f.f.pool.Exec(f.f.ctx, `INSERT INTO tool_calls(tool_call_id,run_id,tool_name,args,status,runtime_target,started_at,finished_at,result) VALUES($1,$2,$3,$4,'completed','synthetic-loopback-project',clock_timestamp(),clock_timestamp(),$5)`, call, run, tool, args, []byte(result)); err != nil {
					t.Fatal(err)
				}
				if _, err := f.f.pool.Exec(f.f.ctx, `INSERT INTO action_effects(action_id,fingerprint,initial_binding,policy,state,dispatch_outcome) VALUES($1,$2,$3,'{}'::jsonb,$4,$5)`, effect, id, raw, state, outcome); err != nil {
					t.Fatal(err)
				}
				if _, err := f.f.pool.Exec(f.f.ctx, `INSERT INTO action_intents(action_id,run_id,tool_call_id,action_hash,binding,requires_approval,expires_at,effect_id) VALUES($1,$2,$3,$4,$5,false,clock_timestamp()+interval '1 minute',$6)`, binding.ActionID, run, call, id, raw, effect); err != nil {
					t.Fatal(err)
				}
				if _, err := f.f.pool.Exec(f.f.ctx, `INSERT INTO action_authorizations(authorization_id,action_id,action_hash,grant_data,expires_at,state,dispatched_at,finished_at) VALUES($1,$2,$3,'{}'::jsonb,clock_timestamp()+interval '1 minute','dispatch_completed',clock_timestamp(),clock_timestamp())`, auth, binding.ActionID, id); err != nil {
					t.Fatal(err)
				}
				if kind == "unconfirmed_dispatch" && tool == "extension.project.check" {
					outcome = "unknown"
				}
				if _, err := f.f.pool.Exec(f.f.ctx, `INSERT INTO action_effect_dispatches(authorization_id,action_id,execution_action_id,outcome,dispatched_at,finished_at) VALUES($1,$2,$3,$4,clock_timestamp(),clock_timestamp())`, auth, effect, binding.ActionID, outcome); err != nil {
					t.Fatal(err)
				}
			}
			proposal := projectfixture.Proposal{ProjectRevision: projectfixture.Revision(), Changes: []projectfixture.Change{{Path: "README.md", Before: projectfixture.Original, After: projectfixture.Prepared}}}
			prepareArgs, _ := json.Marshal(map[string]any{"url": egresstest.Origin + "/project/patches", "body": proposal})
			store(f.parentRun, "shared-id", "extension.project.prepare-patch", prepareArgs, prepare)
			inspectArgs, _ := json.Marshal(map[string]string{"url": egresstest.Origin + "/project"})
			store(f.parentRun, "inspect", "extension.project.inspect", inspectArgs, get("/project"))
			path := "/project/patches/" + artifact + "/checks"
			checkArgs, _ := json.Marshal(map[string]string{"url": egresstest.Origin + path})
			store(checkRun, "shared-id", "extension.project.check", checkArgs, get(path))
			current := f.f.parent
			switch kind {
			case "stale_run_revision":
				if _, err := f.f.pool.Exec(f.f.ctx, `UPDATE runs SET task_revision=task_revision+1 WHERE run_id=$1`, checkRun); err != nil {
					t.Fatal(err)
				}
			case "unfinished_phase":
				if _, err := f.f.pool.Exec(f.f.ctx, `UPDATE runs SET status='model_running',current_state='model_running' WHERE run_id=$1`, checkRun); err != nil {
					t.Fatal(err)
				}
			case "stale_current_task":
				current.ConstraintsRevision++
			}
			a, pr, pc, cr, cc, err := releaseChainV15OriginalProof(f.f.ctx, f, current)
			if kind == "previous_phase_qualified_calls" {
				if err != nil || a != artifact || pr != f.parentRun || cr != checkRun || pc != "shared-id" || cc != "shared-id" {
					t.Fatal("confirmed previous phase/current task proof rejected", err)
				}
			} else if err == nil {
				t.Fatal("foreign, stale, unknown or incomplete proof accepted")
			}
		})
	}
}
