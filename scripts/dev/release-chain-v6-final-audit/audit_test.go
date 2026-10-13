package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func record(fields string, values object) object {
	r := object{}
	for _, k := range strings.Fields(fields) {
		r[k] = nil
	}
	for k, v := range values {
		r[k] = v
	}
	return r
}
func units(turns int64) object {
	u := record(unitKeys, object{})
	for k := range u {
		if turns == 0 {
			u[k] = int64(0)
		} else {
			u[k] = int64(100000000)
		}
	}
	u["model_turns"] = turns
	u["tokens"] = turns * 1081344
	u["active_millis"] = turns * 120000
	return u
}
func normalizedInputs(x inputs) inputs {
	return inputs{clone(x.Scope), clone(x.Ledger), clone(x.Initial), clone(x.Product), clone(x.Runtime), clone(x.Quality), clone(x.Prerequisite), clone(x.ExpenseException)}
}
func actualPublic(t *testing.T, name, pin string) any {
	t.Helper()
	root, e := os.Getwd()
	if e != nil {
		t.Fatal(e)
	}
	for {
		if _, e := os.Stat(filepath.Join(root, "go.mod")); e == nil {
			break
		}
		parent := filepath.Dir(root)
		if parent == root {
			t.Fatal("repository root unavailable")
		}
		root = parent
	}
	raw, e := os.ReadFile(filepath.Join(root, "deploy", "acceptance", name))
	if e != nil || digest(raw) != pin {
		t.Fatal("pinned safe prerequisite unavailable")
	}
	v, e := decode(raw)
	if e != nil {
		t.Fatal(e)
	}
	return v
}
func syntheticWitness(t *testing.T) inputs {
	t.Helper()
	base := time.Date(2026, 10, 13, 6, 0, 0, 0, time.UTC)
	ts := func(t time.Time) string { return t.Format(time.RFC3339Nano) }
	native, sql, initial, products := []any{}, []any{}, []any{}, []any{}
	results := object{}
	ordinal := 537
	for caseIndex, name := range scenarios {
		start := base.Add(time.Duration(caseIndex*5+1) * time.Second)
		root, child := "root-"+name, "child-"+name
		end := start.Add(time.Second)
		promoted := object{"task_id": root, "status": "active", "revision": 1, "constraints_revision": 1, "result_delivery": "verified_task"}
		accepted := record("input_id source status task_id run_id accepted_at expires_at", object{"input_id": "input-admit-" + name, "source": "initiative", "status": "accepted", "task_id": root, "accepted_at": ts(start.Add(-time.Second / 2)), "expires_at": ts(base.Add(time.Hour))})
		doc := object{"source_id": "doc-" + name, "revision": 1, "access_revision": 1, "content_hash": digest([]byte("name,count\nprivate-original-document,2\n"))}
		need := object{"source_id": "need-" + name, "revision": 1, "access_revision": 1, "content_hash": digest([]byte("synthetic-need"))}
		spec := object{"source_id": "spec-" + name, "revision": 1, "access_revision": 1, "content_hash": digest([]byte("synthetic-spec"))}
		refs := object{"document": doc, "need": need, "spec": spec}
		setupRuns, setupActions := []any{}, []any{}
		for j := 0; j < 3; j++ {
			setupRuns = append(setupRuns, record("run_id task_id task_revision current_state model_provider started_at finished_at", object{"run_id": "setup-" + name + string(rune('a'+j)), "current_state": "completed", "model_provider": "fixture", "started_at": ts(start.Add(-time.Second)), "finished_at": ts(start.Add(-time.Second / 2))}))
		}
		for j := 0; j < 4; j++ {
			setupActions = append(setupActions, record("action_id run_id tool_call_id action_hash effect_id requires_approval created_at tool_name effect_state authorization_id authorization_state", object{"action_id": "setup-action-" + name + string(rune('a'+j)), "run_id": obj(setupRuns[j%3])["run_id"], "action_hash": digest([]byte("setup-action")), "requires_approval": true, "effect_state": "effect_confirmed", "authorization_state": "dispatch_completed"}))
		}
		ini := record("version scenario configured_native_high ordinary_core_entry setup_execution adapter_development model_authored_ir human_recipient native_batch_support native_multi_function_contract generic_wrapper_batch_handoff_evidence core_batch_capability target_converter_id target_need_sha256 original_refs existing_target_versions existing_target_catalogs preexisting_root_model_reservations preexisting_case_native_requests postgres", object{"version": "butler-release-chain-v5-initial-witness-v1", "scenario": name, "configured_native_high": true, "ordinary_core_entry": "after_owner_bound_initiative_promoted_input", "setup_execution": "owner_services_and_real_approved_broker_reads_without_model", "adapter_development": "production_deterministic_finite_ir_builder_selected_by_native_child", "model_authored_ir": false, "human_recipient": false, "native_batch_support": false, "native_multi_function_contract": "capability_mismatch_before_tool_release", "generic_wrapper_batch_handoff_evidence": "Go_spy_fixture_only", "core_batch_capability": false, "target_converter_id": "developer-inventory", "target_need_sha256": digest([]byte("need-object")), "original_refs": refs, "existing_target_versions": 0, "existing_target_catalogs": 0, "preexisting_root_model_reservations": 0, "preexisting_case_native_requests": 0, "postgres": object{"promoted_task": promoted, "accepted_input": accepted, "setup_runs": setupRuns, "setup_actions": setupActions, "setup_approved_count": 4, "initial_converter_need_count": 0}})
		pg := object{"root_task_id": root, "live_tree_leases": 0, "run_outbox_rows": 0, "digest_outbox_rows": 0, "technical_sent_rows": 0}
		for k := range productFields {
			pg[k] = []any{}
		}
		add := func(table string, value object) {
			pg[table] = append(pg[table].([]any), record(productFields[table], value))
		}
		rootStatus, childStatus := "completed", "paused"
		if name == "owner_stop" {
			rootStatus, childStatus = "cancelled", "cancelled"
		}
		add("tasks", object{"task_id": root, "parent_task_id": nil, "status": rootStatus, "revision": 2, "constraints_revision": 2, "result_delivery": "verified_task", "completion_result_id": "owner-result-" + name, "created_at": ts(start.Add(-time.Second)), "finished_at": ts(end)})
		add("tasks", object{"task_id": child, "parent_task_id": root, "status": childStatus, "revision": 2, "constraints_revision": 2, "result_delivery": "verified_task", "created_at": ts(start), "finished_at": nil})
		for _, ref := range []object{doc, need, spec} {
			add("catalog_sources", object{"source_id": ref["source_id"], "revision": 1, "access_revision": 1, "status": "active", "content_hash": ref["content_hash"]})
		}
		scopeChild := units(4)
		scopeChild["external_calls"] = 4
		for _, s := range []object{{"scope_id": "task-" + name, "kind": "task", "subject_id": root, "limits": units(24), "reserved": units(0)}, {"scope_id": "day-" + name, "kind": "owner_day", "subject_id": "owner-day", "limits": units(80), "reserved": units(0)}, {"scope_id": "child-" + name, "kind": "delegation", "subject_id": child, "limits": scopeChild, "reserved": units(0)}} {
			s["deadline"] = ts(base.Add(time.Hour))
			add("budget_scopes", s)
		}
		phases := []struct {
			kind, task string
			tools      []string
		}{{"admit", root, []string{"converters.discover", "converter.developer", ""}}, {"child", child, []string{"converters.discover", "converters.develop", "converters.test", ""}}, {"collect", root, []string{"converters.collect", "converters.test", "converters.trial", "converters.activate", ""}}, {"convert", root, []string{"converter.developer-inventory.convert", "extension.project.inspect", "extension.project.prepare-patch", "extension.project.check", ""}}}
		if name == "owner_stop" {
			phases = phases[:1]
			phases[0].tools = phases[0].tools[:2]
		}
		var childRun, convertRun string
		var patchCall, checkCall string
		caseCalls := 0
		for _, phase := range phases {
			run := phase.kind + "-run-" + name
			inputID := "input-" + phase.kind + "-" + name
			if phase.kind == "child" {
				childRun = run
			}
			if phase.kind == "convert" {
				convertRun = run
			}
			add("runs", object{"run_id": run, "task_id": phase.task, "task_revision": 1, "status": "completed", "current_state": "completed", "model_provider": "antigravity", "started_at": ts(start), "finished_at": ts(end)})
			add("accepted_inputs", object{"input_id": inputID, "event_id": "event-" + run, "session_sequence": 1, "source": "fixture-promoted", "task_id": phase.task, "run_id": run, "status": "completed", "checkpoint": "done", "attempts": 1, "accepted_at": ts(start), "updated_at": ts(end), "expires_at": ts(base.Add(time.Hour)), "error_code": "", "not_before": ts(end)})
			for step, tool := range phase.tools {
				ordinal++
				at := start.Add(time.Duration(caseCalls) * 20 * time.Millisecond)
				finish := at.Add(10 * time.Millisecond)
				caseCalls++
				isFinal := tool == ""
				fc, tc := 0, 0
				var reason any
				if isFinal {
					fc, tc, reason = 1, 1, "STOP"
				}
				r := object{"ordinal": ordinal, "scenario": name, "kind": "submit", "root_task_id": root, "run_id": run, "is_parent": phase.task == root, "input_sha256": digest([]byte(run + tool)), "admitted_at_utc": ts(at), "finished_at_utc": ts(finish), "elapsed_ms": 10, "outcome": "completed", "failure_stage": nil, "diagnostic_code": nil, "actual_input_tokens": 10, "actual_output_tokens": 2, "actual_total_tokens": 15, "model_version": "gemini-3.8-flash-n", "final_count": fc, "terminal_count": tc, "completed_count": tc, "cancelled_count": 0, "timed_out_count": 0, "transport_error_count": 0, "finish_reason": reason, "usage_observation_uncertain": false, "version_observation_uncertain": false, "stop_final_count": fc, "max_tokens_final_count": 0, "finish_observation_uncertain": false, "provider_operation_stage": "response_stream"}
				op, res := "operation-"+name+string(rune(ordinal)), "reservation-"+name+string(rune(ordinal))
				r["kernel_operation_id"] = op
				if step == 0 {
					r["kind"] = "start"
				}
				native = append(native, r)
				pr := obj(clone(r))
				for from, to := range map[string]string{"input_sha256": "input_hash", "admitted_at_utc": "admitted_at", "finished_at_utc": "finished_at", "actual_input_tokens": "input_tokens", "actual_output_tokens": "output_tokens", "actual_total_tokens": "total_tokens"} {
					pr[to] = pr[from]
					delete(pr, from)
				}
				sql = append(sql, pr)
				charge := units(1)
				for k := range charge {
					charge[k] = int64(0)
				}
				charge["model_turns"], charge["tokens"] = int64(1), int64(1081344)
				charge["active_millis"] = 120000
				ids := []any{"task-" + name, "day-" + name}
				if phase.task == child {
					ids = append(ids, "child-"+name)
				}
				add("budget_reservations", object{"reservation_id": res, "operation_id": op, "fingerprint": digest([]byte(op)), "created_at": ts(at.Add(-time.Millisecond)), "charge": charge, "quoted_cost_known": true, "scope_ids": ids})
				add("model_budget_attempts", object{"operation_id": op, "run_id": run, "reservation_id": res, "provider": "antigravity", "model": "gemini-3.8-flash-high", "input_allowance": 1048576, "output_allowance": 32768, "actual_input_tokens": 10, "actual_output_tokens": 2, "created_at": ts(at.Add(-time.Millisecond)), "observed_at": ts(finish)})
				if isFinal {
					add("final_results", object{"result_id": "final-" + run, "run_id": run, "task_id": phase.task, "task_revision": 1, "constraints_revision": 1, "content_hash": digest([]byte("synthetic-final")), "answer_state": "final", "verdict": "unverified", "privacy_redacted": false})
					add("task_results", object{"result_id": "final-" + run, "task_id": phase.task, "task_revision": 1, "run_id": run, "verdict": "unverified", "verifier": "model_run", "created_at": ts(finish)})
					continue
				}
				action, effect, auth, call := "action-"+op, "effect-"+op, "auth-"+op, "call-"+op
				if tool == "extension.project.prepare-patch" {
					patchCall = call
				}
				if tool == "extension.project.check" {
					checkCall = call
				}
				add("actions", object{"action_id": action, "run_id": run, "tool_call_id": call, "tool_name": tool, "action_hash": digest([]byte(action)), "effect_id": effect, "fingerprint": digest([]byte(effect)), "state": "effect_confirmed", "dispatch_outcome": "dispatch_completed", "created_at": ts(finish.Add(time.Millisecond))})
				add("authorizations", object{"authorization_id": auth, "action_id": action, "action_hash": digest([]byte(action)), "mandate_id": "mandate-" + name, "mandate_revision": 1, "state": "dispatch_completed", "expires_at": ts(base.Add(time.Hour)), "created_at": ts(finish.Add(time.Millisecond)), "dispatched_at": ts(finish.Add(2 * time.Millisecond)), "finished_at": ts(finish.Add(3 * time.Millisecond))})
				add("dispatches", object{"authorization_id": auth, "action_id": effect, "execution_action_id": action, "outcome": "dispatch_completed", "dispatched_at": ts(finish.Add(2 * time.Millisecond)), "finished_at": ts(finish.Add(3 * time.Millisecond))})
			}
		}
		if name == "owner_stop" {
			childRun = "child-run-" + name
			add("runs", object{"run_id": childRun, "task_id": child, "task_revision": 1, "status": "cancelled", "current_state": "cancelled", "model_provider": "antigravity", "started_at": ts(start), "finished_at": ts(end)})
		}
		fault := object{"triggered": name == "restart_sent" || name == "owner_stop", "old_child_lease_sha256": nil, "old_child_run_id": "", "stopped_child_source_denied": false, "stopped_child_broker_prepare_denied": false}
		fin := object{"version": "butler-release-chain-v5-product-witness-v1", "scenario": name, "test_failed": false, "postgres": pg, "native_batch_support": false, "native_multi_function_contract": "capability_mismatch_before_tool_release", "generic_wrapper_batch_handoff_evidence": "Go_spy_fixture_only", "core_batch_capability": false, "original_document": doc, "original_need": need, "original_spec": spec, "patch_artifact_file_sha256": nil, "original_result": nil, "recipient_requests": 0, "recipient_acceptances": 0, "all_channel_requests": 0, "fault": fault}
		if name == "owner_stop" {
			fault["old_child_lease_sha256"] = digest([]byte("private-lease-not-exported"))
			fault["old_child_run_id"] = childRun
			fault["stopped_child_source_denied"] = true
			fault["stopped_child_broker_prepare_denied"] = true
		} else {
			dig, needHash, profile := digest([]byte("synthetic-adapter")), ini["target_need_sha256"], digest([]byte("synthetic-profile"))
			outputHash := digest([]byte(`[{"label":"private-original-document","quantity":2}]`))
			outputRef := object{"source_id": "output-" + name, "revision": 1, "access_revision": 1, "content_hash": outputHash}
			add("catalog_sources", object{"source_id": outputRef["source_id"], "revision": 1, "access_revision": 1, "status": "active", "content_hash": outputHash})
			add("goals", object{"goal_id": "goal-" + name, "kind": "maintenance", "status": "active", "revision": 1, "expires_at": ts(base.Add(time.Hour))})
			add("mandates", object{"mandate_id": "mandate-" + name, "revision": 1, "status": "active", "scope_kind": "task", "goal_id": "goal-" + name, "goal_revision": 1, "expires_at": ts(base.Add(time.Hour))})
			add("task_authority", object{"task_id": root, "goal_id": "goal-" + name, "goal_revision": 1, "mandate_id": "mandate-" + name, "mandate_revision": 1})
			add("converter_versions", object{"digest": dig, "converter_id": "developer-inventory", "version": "1.0.0", "need_id": "need-id", "need_hash": needHash, "source_ref_json": need, "profile_pin": profile, "created_by_run": childRun})
			add("converter_states", object{"digest": dig, "status": "active"})
			add("converter_catalog", object{"converter_id": "developer-inventory", "generation": 1, "active_digest": dig})
			add("converter_returns", object{"child_task_id": child, "parent_task_id": root, "result_id": "final-" + childRun, "child_run_id": childRun, "digest": dig, "projection_hash": digest([]byte("return"))})
			add("developer_admissions", object{"task_id": child, "parent_task_id": root, "parent_revision": 1, "parent_constraints_revision": 1})
			add("delegations", object{"task_id": child, "root_task_id": root, "parent_task_id": root, "parent_revision": 1, "parent_constraints_revision": 1, "depth": 1, "state": "result_ready", "deadline": ts(base.Add(time.Hour))})
			for _, run := range []string{childRun, "collect-run-" + name} {
				add("converter_validations", object{"validation_id": "validation-" + run, "digest": dig, "run_id": run, "package_hash": dig, "need_hash": needHash, "profile_hash": profile, "checker": "butler.delimited-json.checks.v1", "passed": "true"})
			}
			add("converter_trials", object{"action_id": "trial-action", "digest": dig, "run_id": "collect-run-" + name, "task_id": root, "task_revision": 1, "status": "passed", "input_ref_json": doc, "output_ref_json": outputRef, "output_hash": outputHash})
			add("converter_usages", object{"action_id": "convert-action", "digest": dig, "run_id": convertRun, "task_id": root, "task_revision": 1, "constraints_revision": 1, "generation": 1, "output_ref_json": outputRef, "output_hash": outputHash})
			for _, purpose := range []string{"candidate_return", "activated_catalog"} {
				input := "input-collect-" + name
				if purpose == "activated_catalog" {
					input = "input-convert-" + name
				}
				add("continuations", object{"continuation_id": purpose + name, "child_task_id": child, "parent_task_id": root, "parent_revision": 1, "constraints_revision": 1, "purpose": purpose, "intent_hash": digest([]byte(purpose)), "state": "queued", "input_id": input, "ready_hash": digest([]byte("ready"))})
			}
			add("task_results", object{"result_id": "owner-result-" + name, "task_id": root, "task_revision": 1, "run_id": convertRun, "verdict": "verified", "verifier": "owner", "created_at": ts(end)})
			attempts, state, ack, errorCode := 1, "sent", "101", ""
			if name == "explicit_absence" {
				attempts = 2
			}
			if name == "lost_ack" {
				state, ack, errorCode = "delivery_unknown", "", "sending_claim_expired"
			}
			add("notices", object{"notification_id": "verified-task:" + root, "task_id": root, "result_id": "owner-result-" + name, "task_revision": 2, "constraints_revision": 2, "run_id": convertRun, "channel": "telegram", "status": state, "attempt_count": attempts, "channel_ack": ack, "error_code": errorCode, "created_at": ts(end)})
			add("accepted_inputs", object{"input_id": "owner-control-" + name, "event_id": "owner-control-" + name, "session_sequence": 4, "source": "web", "task_id": root, "run_id": nil, "status": "completed", "checkpoint": "owner_control_applied", "attempts": 0, "accepted_at": ts(end), "updated_at": ts(end), "expires_at": ts(base.Add(time.Hour))})
			add("initiatives", object{"initiative_id": "initiative-" + name, "task_id": root, "status": "completed", "reason": "verified_task_result"})
			artifact := digest([]byte("patch-" + name))
			fin["patch_artifact_file_sha256"] = digest([]byte("stored-patch-" + name))
			fin["original_result"] = object{"patch_count": 1, "artifact_id": artifact, "project_revision": digest([]byte("extension-project-fixture\x00Butler reviewed fixture\nready=false\n")), "changes": []any{object{"path": "README.md", "before_sha256": digest([]byte("Butler reviewed fixture\nready=false\n")), "after_sha256": digest([]byte("Butler reviewed fixture\nready=true\n"))}}, "patch_tool_call_id": patchCall, "check_tool_call_id": checkCall, "prepare_receipt_count": 1, "check_receipt_count": 1, "prepare_receipt_artifact_id": artifact, "check_receipt_artifact_id": artifact, "checks_passed": true, "applied": false}
			fin["recipient_requests"], fin["recipient_acceptances"], fin["all_channel_requests"] = attempts, 1, attempts
		}
		for _, scope := range rows(pg, "budget_scopes") {
			used := obj(scope["reserved"])
			for _, res := range rows(pg, "budget_reservations") {
				for _, id := range arr(res["scope_ids"]) {
					if id == scope["scope_id"] {
						for _, dim := range strings.Fields(unitKeys) {
							used[dim] = integer(clone(used[dim])) + integer(clone(obj(res["charge"])[dim]))
						}
					}
				}
			}
		}
		results[name] = true
		ini = obj(clone(ini))
		fin = obj(clone(fin))
		initial = append(initial, object{"scenario": name, "payload_sha256": hashValue(ini), "payload_json": ini, "created_at": ts(start.Add(-time.Millisecond))})
		products = append(products, object{"scenario": name, "payload_sha256": hashValue(fin), "payload_json": fin, "created_at": ts(end.Add(time.Millisecond))})
	}
	window := object{"native_baseline": 537, "attempts": ordinal, "prior_history_hash": priorSHA, "prerequisite_hash": prerequisiteSHA, "expires_at": ts(base.Add(time.Hour)), "maximum_new_attempts": 78, "provider_stage_observation": true, "kernel_operation_binding": true}
	scope := object{"status": "passed", "window_id": "issue33-high-release-chain-v6-20261013-baseline537", "baseline_attempts": 537, "maximum_total_attempts": 615, "maximum_new_attempts": 78, "scenarios": results, "scope": object{"total_attempts": ordinal, "new_attempts": ordinal - 537, "unknown_full_token_cost": 0, "failed_provider_entries": 0, "accepted_model_retry_failures": 0, "overlong_provider_entries": 0, "requests": native, "window": window}, "planning": "actual_native_High", "human_messages": 0, "issue33_ready": false}
	ledger := object{"baseline_attempts": 537, "total_attempts": ordinal, "new_attempts": ordinal - 537, "requests": sql, "window": window}
	runtime := object{"started_at": ts(base), "finished_at": ts(base.Add(30 * time.Second)), "exit_code": 0, "oom_killed": false, "memory_limit_bytes": 1536 << 20, "image_id": "sha256:5cf287a799e6b94384bad13d16b14904c531f51ba65792237e122ce42b392f61", "postgres_image_id": "sha256:" + strings.Repeat("b", 64), "converter_image_id": "sha256:" + strings.Repeat("c", 64)}
	return normalizedInputs(inputs{scope, ledger, initial, products, runtime, actualPublic(t, "cloud-shadow-v5-final-audit-20261013.json", qualitySHA), candidatePrerequisite(t), actualPublic(t, "release-chain-v6-user-expense-exception-20261013.json", expenseExceptionSHA)})
}

func TestSyntheticChainWitnessPassNeverGrantsIssueAcceptance(t *testing.T) {
	r, e := audit(syntheticWitness(t))
	if e != nil || r.Status != "CHAIN_V6_WITNESS_AUDIT_PASS" || r.Cost.Attempts != 70 || r.Cost.Total != 1050 || r.UnknownHistoricalTotal != 47 || r.Ready || r.AuditorCalls != 0 {
		t.Fatal("synthetic verifier contract", e, r.Status, r.Reasons)
	}
}
func tamperProduct(in *inputs, index int, fn func(object)) {
	list := arr(in.Product)
	w := obj(list[index])
	p := obj(w["payload_json"])
	fn(p)
	w["payload_sha256"] = hashValue(p)
}
func TestChainWitnessRejectsCoherentSemanticTampering(t *testing.T) {
	baseline := syntheticWitness(t)
	if r, e := audit(baseline); e != nil || r.Status != "CHAIN_V6_WITNESS_AUDIT_PASS" {
		t.Fatal("tamper matrix baseline is not valid", e)
	}
	cases := map[string]func(*inputs){
		"scope_pg_cost_diff":   func(x *inputs) { obj(arr(obj(x.Ledger)["requests"])[0])["total_tokens"] = jsonNumber(16) },
		"unknown_native_spend": func(x *inputs) { obj(arr(obj(obj(x.Scope)["scope"])["requests"])[0])["actual_total_tokens"] = nil },
		"ordinal_replay":       func(x *inputs) { obj(arr(obj(obj(x.Scope)["scope"])["requests"])[1])["ordinal"] = jsonNumber(538) },
		"fake_prior": func(x *inputs) {
			obj(obj(obj(x.Scope)["scope"])["window"])["prior_history_hash"] = strings.Repeat("0", 64)
		},
		"window_deadline_extended":     func(x *inputs) { obj(obj(obj(x.Scope)["scope"])["window"])["expires_at"] = "2026-10-14T06:00:00Z" },
		"collector_scenario_omitted":   func(x *inputs) { delete(obj(obj(x.Scope)["scenarios"]), "lost_ack") },
		"actual_native_image_replaced": func(x *inputs) { obj(x.Runtime)["image_id"] = "sha256:" + strings.Repeat("e", 64) },
		"missing_initial":              func(x *inputs) { x.Initial = arr(x.Initial)[1:] },
		"initial_after_inference":      func(x *inputs) { obj(arr(x.Initial)[0])["created_at"] = "2026-10-13T06:00:02Z" },
		"preexisting_adapter": func(x *inputs) {
			w := obj(arr(x.Initial)[0])
			p := obj(w["payload_json"])
			p["existing_target_versions"] = jsonNumber(1)
			w["payload_sha256"] = hashValue(p)
		},
		"model_setup_false_claim": func(x *inputs) {
			w := obj(arr(x.Initial)[0])
			p := obj(w["payload_json"])
			p["model_authored_ir"] = true
			w["payload_sha256"] = hashValue(p)
		},
		"lost_ack_resend": func(x *inputs) { tamperProduct(x, 3, func(p object) { p["recipient_requests"] = jsonNumber(2) }) },
		"lost_ack_confirmed_falsely": func(x *inputs) {
			tamperProduct(x, 3, func(p object) { rows(obj(p["postgres"]), "notices")[0]["status"] = "sent" })
		},
		"extra_notice": func(x *inputs) {
			tamperProduct(x, 0, func(p object) {
				pg := obj(p["postgres"])
				pg["notices"] = append(arr(pg["notices"]), clone(arr(pg["notices"])[0]))
			})
		},
		"model_final_verified": func(x *inputs) {
			tamperProduct(x, 0, func(p object) { rows(obj(p["postgres"]), "final_results")[0]["verdict"] = "verified" })
		},
		"technical_delivery": func(x *inputs) {
			tamperProduct(x, 0, func(p object) { obj(p["postgres"])["technical_sent_rows"] = jsonNumber(1) })
		},
		"adapter_replaced": func(x *inputs) {
			tamperProduct(x, 0, func(p object) {
				rows(obj(p["postgres"]), "converter_catalog")[0]["active_digest"] = strings.Repeat("0", 64)
			})
		},
		"parent_check_missing": func(x *inputs) {
			tamperProduct(x, 0, func(p object) {
				pg := obj(p["postgres"])
				pg["converter_validations"] = arr(pg["converter_validations"])[:1]
			})
		},
		"original_output_wrong": func(x *inputs) {
			tamperProduct(x, 0, func(p object) {
				rows(obj(p["postgres"]), "converter_usages")[0]["output_hash"] = strings.Repeat("0", 64)
			})
		},
		"patch_wrong_bytes": func(x *inputs) {
			tamperProduct(x, 0, func(p object) {
				obj(arr(obj(p["original_result"])["changes"])[0])["after_sha256"] = strings.Repeat("0", 64)
			})
		},
		"patch_applied": func(x *inputs) { tamperProduct(x, 0, func(p object) { obj(p["original_result"])["applied"] = true }) },
		"check_receipt_ambiguous": func(x *inputs) {
			tamperProduct(x, 0, func(p object) { obj(p["original_result"])["check_receipt_count"] = jsonNumber(2) })
		},
		"duplicate_effect": func(x *inputs) {
			tamperProduct(x, 0, func(p object) { a := rows(obj(p["postgres"]), "actions"); a[1]["effect_id"] = a[0]["effect_id"] })
		},
		"no_broker_authorization": func(x *inputs) {
			tamperProduct(x, 0, func(p object) { pg := obj(p["postgres"]); pg["authorizations"] = arr(pg["authorizations"])[1:] })
		},
		"unknown_effect": func(x *inputs) {
			tamperProduct(x, 0, func(p object) { rows(obj(p["postgres"]), "actions")[0]["state"] = "effect_unknown" })
		},
		"effect_before_native_cost": func(x *inputs) {
			tamperProduct(x, 0, func(p object) { rows(obj(p["postgres"]), "actions")[0]["created_at"] = "2026-10-13T06:00:01Z" })
		},
		"kernel_actual_cost_changed": func(x *inputs) {
			tamperProduct(x, 0, func(p object) {
				rows(obj(p["postgres"]), "model_budget_attempts")[0]["actual_input_tokens"] = jsonNumber(11)
			})
		},
		"root_budget_expanded": func(x *inputs) {
			tamperProduct(x, 0, func(p object) {
				obj(rows(obj(p["postgres"]), "budget_scopes")[0]["limits"])["model_turns"] = jsonNumber(25)
			})
		},
		"unmatched_kernel_measurement": func(x *inputs) {
			tamperProduct(x, 0, func(p object) {
				pg := obj(p["postgres"])
				extra := obj(clone(arr(pg["model_budget_attempts"])[0]))
				extra["operation_id"] = "unmatched"
				pg["model_budget_attempts"] = append(arr(pg["model_budget_attempts"]), extra)
			})
		},
		"stopped_child_source_allowed": func(x *inputs) {
			tamperProduct(x, 4, func(p object) { obj(p["fault"])["stopped_child_source_denied"] = false })
		},
		"stopped_child_effect": func(x *inputs) {
			tamperProduct(x, 4, func(p object) { pg := obj(p["postgres"]); rows(pg, "actions")[0]["run_id"] = "child-run-owner_stop" })
		},
		"stopped_tree_notice": func(x *inputs) { tamperProduct(x, 4, func(p object) { p["recipient_acceptances"] = jsonNumber(1) }) },
		"live_stopped_lease": func(x *inputs) {
			tamperProduct(x, 4, func(p object) { obj(p["postgres"])["live_tree_leases"] = jsonNumber(1) })
		},
		"stopped_patch": func(x *inputs) {
			tamperProduct(x, 4, func(p object) { p["patch_artifact_file_sha256"] = strings.Repeat("a", 64) })
		},
		"private_payload_extra": func(x *inputs) {
			tamperProduct(x, 0, func(p object) { p["provider_token"] = "synthetic-private-must-never-emit" })
		},
	}
	for name, alter := range cases {
		t.Run(name, func(t *testing.T) {
			in := syntheticWitness(t)
			alter(&in)
			r, e := audit(in)
			if e == nil || r.Status != "NO_GO" || r.Ready || r.AuditorCalls != 0 {
				t.Fatal("coherent malicious witness accepted")
			}
			raw, _ := json.Marshal(r)
			if strings.Contains(string(raw), "synthetic-private-must-never-emit") {
				t.Fatal("private text in checker output")
			}
		})
	}
}
func TestChainSafeJSONDeniesDuplicateAndUnboundedFrames(t *testing.T) {
	for _, raw := range []string{`{"status":"passed","status":"failed"}`, `{"x":{"usage":1,"usage":2}}`, `{} {}`, `{"n":NaN}`, strings.Repeat("[", 42) + "0" + strings.Repeat("]", 42)} {
		if _, e := decode([]byte(raw)); e == nil {
			t.Fatal("ambiguous framing accepted")
		}
	}
	if _, e := decode([]byte{'{', '"', 'x', '"', ':', '"', 255, '"', '}'}); e == nil {
		t.Fatal("invalid UTF8 metadata repaired")
	}
}

func TestChainJournalCoherentMutationAndFailedSpendPreservation(t *testing.T) {
	for _, name := range []string{"repeated_ordinal", "extended_expiry", "wrong_version", "provisional_usage", "failed_known_cost", "scope_ref_removed", "reserved_counter_reset", "model_task_result_promoted"} {
		t.Run(name, func(t *testing.T) {
			x := syntheticWitness(t)
			a := obj(arr(obj(obj(x.Scope)["scope"])["requests"])[0])
			b := obj(arr(obj(x.Ledger)["requests"])[0])
			switch name {
			case "repeated_ordinal":
				a["ordinal"], b["ordinal"] = jsonNumber(537), jsonNumber(537)
			case "extended_expiry":
				obj(obj(obj(x.Scope)["scope"])["window"])["expires_at"] = "2026-10-13T08:00:00Z"
				obj(obj(x.Ledger)["window"])["expires_at"] = "2026-10-13T08:00:00Z"
			case "wrong_version":
				a["model_version"], b["model_version"] = "gemini-3.8-flash-other", "gemini-3.8-flash-other"
			case "provisional_usage":
				a["usage_observation_uncertain"], b["usage_observation_uncertain"] = true, true
			case "failed_known_cost":
				a["outcome"], b["outcome"] = "transport_error", "transport_error"
			case "scope_ref_removed":
				tamperProduct(&x, 0, func(p object) { rows(obj(p["postgres"]), "budget_reservations")[0]["scope_ids"] = []any{} })
			case "reserved_counter_reset":
				tamperProduct(&x, 0, func(p object) {
					obj(rows(obj(p["postgres"]), "budget_scopes")[0]["reserved"])["model_turns"] = jsonNumber(0)
				})
			case "model_task_result_promoted":
				tamperProduct(&x, 0, func(p object) {
					row := rows(obj(p["postgres"]), "task_results")[0]
					row["verdict"], row["verifier"] = "verified", "owner"
				})
			}
			r, e := audit(x)
			if e == nil || r.Status != "NO_GO" || r.Ready {
				t.Fatal("coherent tampered evidence accepted")
			}
			if r.Cost.Attempts != 70 || r.Cost.Input != 700 || r.Cost.Output != 140 || r.Cost.Total != 1050 || r.Cost.Unknown != 0 {
				t.Fatal("failed complete measurement refunded", r.Cost)
			}
		})
	}
}

func TestOwningCostPreservedBeyondCapAndUnknownDimensions(t *testing.T) {
	for _, name := range []string{"over_cap", "unknown_dimension"} {
		t.Run(name, func(t *testing.T) {
			x := syntheticWitness(t)
			ledger := obj(x.Ledger)
			if name == "over_cap" {
				for len(arr(ledger["requests"])) < 79 {
					ledger["requests"] = append(arr(ledger["requests"]), clone(arr(ledger["requests"])[0]))
				}
			} else {
				a := obj(arr(ledger["requests"])[0])
				a["input_tokens"] = nil
				a["total_tokens"] = nil
			}
			r, e := audit(x)
			if e == nil || r.Status != "NO_GO" || r.Ready {
				t.Fatal("invalid owning journal accepted")
			}
			if name == "over_cap" && (r.Cost.Attempts != 79 || r.Cost.Total != 1185 || r.Total != 616) {
				t.Fatal("over-cap entries refunded", r.Cost, r.Total)
			}
			if name == "unknown_dimension" && (r.Cost.Attempts != 70 || r.Cost.Input != 690 || r.Cost.Output != 140 || r.Cost.Total != 1035 || r.Cost.Unknown != 1 || r.Cost.UnknownInput != 1 || r.Cost.UnknownOutput != 0) {
				t.Fatal("unknown PG cost invented", r.Cost)
			}
		})
	}
}

func TestOperatorRuntimeImageAndProfileBindings(t *testing.T) {
	pgImage, runtimeImage, converter := "sha256:"+strings.Repeat("b", 64), "sha256:"+strings.Repeat("c", 64), strings.Repeat("d", 64)
	hardware := obj(clone(object{"go_image": "sha256:5cf287a799e6b94384bad13d16b14904c531f51ba65792237e122ce42b392f61", "postgres_image": pgImage, "runtime_base_image": runtimeImage, "go": "1.27.2", "kernel": "synthetic-kernel", "logical_cpus": 8, "host_ram_kib": 8388608, "cpu_manifest_sha256": strings.Repeat("a", 64), "build_memory_bytes": 2 << 30, "fixture_memory_bytes": 1536 << 20, "converter_memory_bytes": 128 << 20, "build_cpus": 2, "fixture_cpus": 2, "postgres_cpus": 1, "converter_cpus": 1, "provider": "actual_native_High", "actual_native_calls": nil, "channel": "loopback_http_simulator", "weights": "not_observed"}))
	runtime := obj(syntheticWitness(t).Runtime)
	profile := obj(clone(object{"dependency": object{"id": "butler-converter-ir", "version": "1.0.0", "sha256": converter}, "checker": "butler.delimited-json.checks.v1"}))
	profileHash := operatorBindings(hardware, runtime, profile, pgImage, runtimeImage, converter)
	want := digest([]byte(`{"dependency":{"id":"butler-converter-ir","version":"1.0.0","sha256":"` + converter + `"},"checker":"butler.delimited-json.checks.v1"}`))
	if profileHash != want {
		t.Fatal("production profile byte-order changed")
	}
	for _, name := range []string{"native_image", "postgres_image", "converter_image", "prepared_pg_image", "declared_memory", "hardware_extra", "profile_dependency", "profile_checker"} {
		t.Run(name, func(t *testing.T) {
			h, r, p := obj(clone(hardware)), obj(clone(runtime)), obj(clone(profile))
			switch name {
			case "native_image":
				r["image_id"] = "sha256:" + strings.Repeat("e", 64)
			case "postgres_image":
				r["postgres_image_id"] = "sha256:" + strings.Repeat("e", 64)
			case "converter_image":
				r["converter_image_id"] = "sha256:" + strings.Repeat("e", 64)
			case "prepared_pg_image":
				h["postgres_image"] = "sha256:" + strings.Repeat("e", 64)
			case "declared_memory":
				h["fixture_memory_bytes"] = jsonNumber(2 << 30)
			case "hardware_extra":
				h["private_secret"] = "synthetic-must-not-emit"
			case "profile_dependency":
				obj(p["dependency"])["sha256"] = strings.Repeat("e", 64)
			case "profile_checker":
				p["checker"] = "foreign-checker"
			}
			denied := false
			func() {
				defer func() { denied = recover() != nil }()
				operatorBindings(h, r, p, pgImage, runtimeImage, converter)
			}()
			if !denied {
				t.Fatal("operator runtime tamper accepted")
			}
		})
	}
}

func TestFullSourceManifestBytesAndInventory(t *testing.T) {
	root := t.TempDir()
	if e := os.Mkdir(filepath.Join(root, "nested"), 0700); e != nil {
		t.Fatal(e)
	}
	if e := os.WriteFile(filepath.Join(root, "a.go"), []byte("source-a"), 0600); e != nil {
		t.Fatal(e)
	}
	if e := os.WriteFile(filepath.Join(root, "nested", "b.json"), []byte("source-b"), 0600); e != nil {
		t.Fatal(e)
	}
	one := digest([]byte("source-a")) + "  ./a.go\n"
	two := digest([]byte("source-b")) + "  ./nested/b.json\n"
	if e := sourceManifest(root, []byte(one+two)); e != nil {
		t.Fatal("exact complete snapshot manifest denied", e)
	}
	for name, raw := range map[string]string{"missing": one, "duplicate": one + one + two, "wrong_bytes": strings.Repeat("0", 64) + one[64:] + two, "traversal": digest([]byte("source-a")) + "  ./../a.go\n" + two, "unowned": one + two + digest([]byte("missing")) + "  ./missing.go\n", "missing_lf": strings.TrimSuffix(one+two, "\n")} {
		t.Run(name, func(t *testing.T) {
			if e := sourceManifest(root, []byte(raw)); e == nil {
				t.Fatal("source manifest tamper accepted")
			}
		})
	}
}
func TestNamedSafeFilePinsBoundsAndMode(t *testing.T) {
	path := filepath.Join(t.TempDir(), "safe.json")
	raw := []byte(`{"safe":1}`)
	if e := os.WriteFile(path, raw, 0600); e != nil {
		t.Fatal(e)
	}
	stat, e := os.Stat(path)
	if e != nil {
		t.Fatal(e)
	}
	got, e := readExact(path, digest(raw), 1024)
	if stat.Mode().Perm() == 0600 {
		if e != nil || string(got) != string(raw) {
			t.Fatal("POSIX private exact input denied", e)
		}
	} else if e == nil {
		t.Fatal("unsupported filesystem mode granted private read")
	}
	if _, e := readExact(path, strings.Repeat("0", 64), 1024); e == nil {
		t.Fatal("wrong private pin accepted")
	}
	if _, e := readExact(path, digest(raw), 4); e == nil {
		t.Fatal("named input exceeds declared bound")
	}
	if _, e := readExact(path, "", 1024); e == nil {
		t.Fatal("missing independent pin accepted")
	}
}
