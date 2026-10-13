package main

import (
	"reflect"
	"strings"
)

var productFields = map[string]string{
	"tasks":                 "task_id parent_task_id status revision constraints_revision result_delivery completion_result_id created_at finished_at",
	"runs":                  "run_id task_id task_revision status current_state model_provider started_at finished_at error_type",
	"accepted_inputs":       "input_id event_id session_sequence source task_id run_id status checkpoint attempts accepted_at updated_at expires_at error_code not_before model_retry",
	"model_budget_attempts": "operation_id run_id reservation_id provider model input_allowance output_allowance actual_input_tokens actual_output_tokens created_at observed_at",
	"budget_reservations":   "reservation_id operation_id fingerprint created_at charge quoted_cost_known scope_ids",
	"budget_scopes":         "scope_id kind subject_id limits reserved deadline created_at updated_at",
	"delegations":           "task_id root_task_id parent_task_id parent_revision parent_constraints_revision contract_revision contract_source_id depth state input_id result_id deadline",
	"developer_admissions":  "task_id parent_task_id parent_revision parent_constraints_revision execution_action_id effect_id authorization_id action_hash args_hash contract_hash snapshot_hash",
	"converter_versions":    "digest converter_id version need_id need_hash source_ref_json profile_pin created_by_run",
	"converter_needs":       "need_id converter_id need_hash source_ref_json task_id task_revision constraints_revision run_id compatible",
	"converter_states":      "digest status", "converter_catalog": "converter_id generation active_digest",
	"converter_validations": "validation_id digest run_id action_id package_hash need_hash profile_hash checker passed",
	"converter_trials":      "action_id digest run_id task_id task_revision status input_ref_json output_ref_json output_hash",
	"converter_usages":      "action_id digest run_id task_id task_revision constraints_revision generation output_ref_json output_hash",
	"converter_events":      "event_id converter_id digest generation kind run_id action_id",
	"converter_returns":     "child_task_id parent_task_id result_id child_run_id digest projection_hash",
	"continuations":         "continuation_id child_task_id parent_task_id parent_revision constraints_revision purpose intent_hash state input_id ready_hash",
	"actions":               "action_id run_id tool_call_id tool_name action_hash effect_id fingerprint state dispatch_outcome created_at",
	"authorizations":        "authorization_id action_id action_hash mandate_id mandate_revision state expires_at created_at dispatched_at finished_at",
	"dispatches":            "authorization_id action_id execution_action_id outcome dispatched_at finished_at",
	"final_results":         "result_id run_id task_id task_revision constraints_revision artifact_id message_id content_hash answer_state verdict privacy_redacted",
	"task_results":          "result_id task_id task_revision run_id verdict verifier created_at",
	"catalog_sources":       "source_id revision access_revision status content_hash",
	"goals":                 "goal_id kind status revision expires_at", "mandates": "mandate_id revision status scope_kind goal_id goal_revision expires_at revoked_at",
	"task_authority": "task_id goal_id goal_revision mandate_id mandate_revision observation_ref",
	"notices":        "notification_id task_id result_id task_revision constraints_revision run_id channel status attempt_count channel_ack error_code created_at",
	"initiatives":    "initiative_id task_id status reason",
}

const unitKeys = "model_turns tokens cost_micros external_calls children active_millis egress_bytes provider_input_bytes"

func verifyProduct(name string, ini, fin object, native []object) {
	verifyProductWithPolicy(name, ini, fin, native, v6AuditPolicy())
}
func verifyProductWithPolicy(name string, ini, fin object, native []object, policy auditPolicy) {
	keys(ini, "version scenario configured_native_high ordinary_core_entry setup_execution adapter_development model_authored_ir human_recipient native_batch_support native_multi_function_contract generic_wrapper_batch_handoff_evidence core_batch_capability target_converter_id target_need_sha256 original_refs existing_target_versions existing_target_catalogs preexisting_root_model_reservations preexisting_case_native_requests postgres")
	keys(fin, "version scenario test_failed postgres native_batch_support native_multi_function_contract generic_wrapper_batch_handoff_evidence core_batch_capability original_document original_need original_spec patch_artifact_file_sha256 original_result recipient_requests recipient_acceptances all_channel_requests fault")
	require(ini["version"] == "butler-release-chain-v5-initial-witness-v1" && fin["version"] == "butler-release-chain-v5-product-witness-v1" && ini["scenario"] == name && fin["scenario"] == name && fin["test_failed"] == false, "product_identity")
	require(ini["configured_native_high"] == true && ini["ordinary_core_entry"] == "after_owner_bound_initiative_promoted_input" && ini["setup_execution"] == "owner_services_and_real_approved_broker_reads_without_model" && ini["adapter_development"] == "production_deterministic_finite_ir_builder_selected_by_native_child" && ini["model_authored_ir"] == false && ini["human_recipient"] == false, "authorship_boundary")
	for _, v := range []object{ini, fin} {
		require(v["native_batch_support"] == false && v["core_batch_capability"] == false && v["native_multi_function_contract"] == "capability_mismatch_before_tool_release" && v["generic_wrapper_batch_handoff_evidence"] == "Go_spy_fixture_only", "batch_boundary")
	}
	for _, field := range []string{"existing_target_versions", "existing_target_catalogs", "preexisting_root_model_reservations", "preexisting_case_native_requests"} {
		require(integer(ini[field]) == 0, "adapter_not_initially_absent")
	}
	setup := obj(ini["postgres"])
	keys(setup, "promoted_task accepted_input setup_runs setup_actions setup_approved_count initial_converter_need_count")
	initialTask := obj(setup["promoted_task"])
	keys(initialTask, "task_id status revision constraints_revision result_delivery")
	root := text(initialTask["task_id"])
	require(root != "" && initialTask["status"] == "active" && initialTask["result_delivery"] == "verified_task", "promoted_task")
	require(len(rows(setup, "setup_runs")) == 3 && len(rows(setup, "setup_actions")) == 4 && integer(setup["setup_approved_count"]) == 4, "cheap_observation_chain")
	setupRuns := byID(rows(setup, "setup_runs"), "run_id")
	for _, a := range rows(setup, "setup_runs") {
		keys(a, "run_id task_id task_revision current_state model_provider started_at finished_at")
		require(a["model_provider"] == "fixture" && a["current_state"] == "completed" && !moment(a["finished_at"]).After(moment(native[0]["admitted_at_utc"])), "model_free_setup")
	}
	for _, a := range rows(setup, "setup_actions") {
		keys(a, "action_id run_id tool_call_id action_hash effect_id requires_approval created_at tool_name effect_state authorization_id authorization_state")
		require(setupRuns[text(a["run_id"])] != nil && a["requires_approval"] == true && a["effect_state"] == "effect_confirmed" && a["authorization_state"] == "dispatch_completed", "approved_setup_read")
		sha(a["action_hash"])
	}
	accepted := obj(setup["accepted_input"])
	keys(accepted, "input_id source status task_id run_id accepted_at expires_at")
	require(accepted["task_id"] == root && text(accepted["input_id"]) != "", "original_promoted_input")
	pg := obj(fin["postgres"])
	inventory := "root_task_id live_tree_leases run_outbox_rows digest_outbox_rows technical_sent_rows"
	for k := range productFields {
		inventory += " " + k
	}
	keys(pg, inventory)
	require(pg["root_task_id"] == root && integer(pg["live_tree_leases"])+integer(pg["run_outbox_rows"])+integer(pg["digest_outbox_rows"])+integer(pg["technical_sent_rows"]) == 0, "quiet_completed_tree")
	for table, fields := range productFields {
		if policy.RequireProposals && table == "actions" {
			fields += " args_hash"
		}
		for _, row := range rows(pg, table) {
			keys(row, fields)
		}
	}
	tasks := byID(rows(pg, "tasks"), "task_id")
	require(len(tasks) == 2 && tasks[root] != nil, "root_child_identity")
	var child string
	for id, t := range tasks {
		require(t["result_delivery"] == "verified_task", "delivery_inheritance")
		if id != root {
			child = id
			require(t["parent_task_id"] == root, "child_lineage")
		}
	}
	runRows := rows(pg, "runs")
	runs := byID(runRows, "run_id")
	sources := byID(rows(pg, "catalog_sources"), "source_id")
	refs := obj(ini["original_refs"])
	keys(refs, "document need spec")
	for _, pair := range [][2]string{{"document", "original_document"}, {"need", "original_need"}, {"spec", "original_spec"}} {
		ref := obj(refs[pair[0]])
		keys(ref, "source_id revision access_revision content_hash")
		require(reflect.DeepEqual(ref, fin[pair[1]]), "original_reference_changed")
		s := sources[text(ref["source_id"])]
		require(s != nil && s["status"] == "active" && s["revision"] == ref["revision"] && s["access_revision"] == ref["access_revision"] && s["content_hash"] == ref["content_hash"], "original_source_not_current")
	}
	require(obj(refs["document"])["content_hash"] == digest([]byte("name,count\nprivate-original-document,2\n")), "original_document_bytes")
	inputRows := byID(rows(pg, "accepted_inputs"), "input_id")
	require(inputRows[text(accepted["input_id"])] != nil, "original_input_discarded")
	for _, a := range native {
		run := runs[text(a["run_id"])]
		require(run != nil && a["root_task_id"] == root && (run["task_id"] == root || run["task_id"] == child) && a["is_parent"] == (run["task_id"] == root) && run["model_provider"] == "antigravity", "native_core_run_lineage")
	}
	verifyBudgetWithPolicy(pg, native, name, root, child, policy)
	actions := rows(pg, "actions")
	verifyActions(pg, native, runs)
	fault := obj(fin["fault"])
	keys(fault, "triggered old_child_lease_sha256 old_child_run_id stopped_child_source_denied stopped_child_broker_prepare_denied")
	if name == "owner_stop" {
		require(tasks[root]["status"] == "cancelled" && tasks[child]["status"] == "cancelled" && fault["triggered"] == true && fault["stopped_child_source_denied"] == true && fault["stopped_child_broker_prepare_denied"] == true && runs[text(fault["old_child_run_id"])] != nil && runs[text(fault["old_child_run_id"])]["task_id"] == child, "stopped_tree_proof")
		sha(fault["old_child_lease_sha256"])
		require(len(actions) == 2 && len(rows(pg, "final_results")) == 0 && len(rows(pg, "task_results")) == 0 && len(rows(pg, "notices")) == 0 && fin["original_result"] == nil && fin["patch_artifact_file_sha256"] == nil && integer(fin["recipient_requests"])+integer(fin["recipient_acceptances"])+integer(fin["all_channel_requests"]) == 0, "stop_no_success_or_notice")
		for _, a := range actions {
			require(runs[text(a["run_id"])]["task_id"] == root, "stopped_descendant_effect")
		}
		for _, a := range native {
			require(a["is_parent"] == true, "stopped_descendant_inference")
		}
		for _, table := range []string{"converter_versions", "converter_catalog", "converter_trials", "converter_usages", "converter_returns", "continuations"} {
			require(len(rows(pg, table)) == 0, "stopped_adapter_effect")
		}
		return
	}
	require(tasks[root]["status"] == "completed" && tasks[child]["status"] == "paused" && len(runRows) == 4 && len(inputRows) == 5 && integer(tasks[root]["revision"]) == integer(initialTask["revision"])+1 && integer(tasks[root]["constraints_revision"]) == integer(initialTask["constraints_revision"])+1, "same_promoted_task_complete")
	for _, a := range runRows {
		require(a["status"] == "completed" && a["current_state"] == "completed", "ordinary_run_not_complete")
	}
	ordinary, control := 0, 0
	for _, a := range inputRows {
		require(a["status"] == "completed", "completed_input_required")
		if a["run_id"] == nil || a["run_id"] == "" {
			require(a["task_id"] == root && a["checkpoint"] == "owner_control_applied", "owner_completion_control_input")
			control++
			continue
		}
		require(runs[text(a["run_id"])] != nil, "ordinary_input_run_missing")
		if a["model_retry"] == nil {
			require(a["attempts"] == jsonNumber(1), "ordinary_input_replayed")
		} else {
			require(a["attempts"] == jsonNumber(2), "retry_input_claim_count")
		}
		ordinary++
	}
	require(ordinary == 4 && control == 1, "ordinary_and_owner_control_inputs")
	require(name != "restart_sent" || fault["triggered"] == true, "actual_restart_not_triggered")
	verifyAuthority(pg, root, native)
	verifyAdapter(pg, ini, root, child, refs, sources)
	verifyOriginalResult(fin, actions)
	finals := rows(pg, "final_results")
	require(len(finals) == 4, "native_final_inventory")
	finalByRun := byID(finals, "run_id")
	for _, f := range finals {
		require(f["verdict"] == "unverified" && f["answer_state"] == "final" && f["privacy_redacted"] == false, "model_final_promoted")
		sha(f["content_hash"])
		run := runs[text(f["run_id"])]
		require(run != nil && run["task_id"] == f["task_id"] && f["task_revision"] == run["task_revision"], "final_current_task_binding")
		if f["task_id"] == root {
			require(f["task_revision"] == initialTask["revision"] && f["constraints_revision"] == initialTask["constraints_revision"], "final_current_root_revision")
		} else {
			require(integer(f["constraints_revision"])+1 == integer(tasks[child]["constraints_revision"]) && integer(f["task_revision"])+1 == integer(tasks[child]["revision"]), "final_current_child_revision")
		}
	}
	for _, n := range native {
		if integer(n["final_count"]) == 1 {
			require(finalByRun[text(n["run_id"])] != nil, "native_final_not_persisted")
		}
	}
	results := rows(pg, "task_results")
	require(len(results) == 5, "ordinary_and_owner_result_inventory")
	byID(results, "result_id")
	var result object
	models := 0
	for _, value := range results {
		if value["verdict"] == "verified" && value["verifier"] == "owner" {
			require(result == nil, "one_owner_verified_result")
			result = value
			continue
		}
		require(value["verdict"] == "unverified" && value["verifier"] == "model_run", "task_model_result_not_verified")
		final := finalByRun[text(value["run_id"])]
		require(final != nil && value["result_id"] == final["result_id"] && value["task_id"] == final["task_id"] && value["task_revision"] == final["task_revision"], "model_task_result_binding")
		models++
	}
	require(result != nil && models == 4, "one_owner_verified_result")
	require(result["task_id"] == root && result["result_id"] == tasks[root]["completion_result_id"] && result["verdict"] == "verified" && result["verifier"] == "owner" && result["task_revision"] == initialTask["revision"] && runs[text(result["run_id"])] != nil, "owner_original_result")
	notices := rows(pg, "notices")
	require(len(notices) == 1, "one_semantic_notice")
	notice := notices[0]
	require(notice["notification_id"] == "verified-task:"+root && notice["task_id"] == root && notice["result_id"] == result["result_id"] && notice["run_id"] == result["run_id"] && notice["task_revision"] == tasks[root]["revision"] && notice["constraints_revision"] == tasks[root]["constraints_revision"], "stable_notice_binding")
	count := int64(1)
	if name == "explicit_absence" {
		count = 2
	}
	require(integer(notice["attempt_count"]) == count && integer(fin["recipient_requests"]) == count && integer(fin["recipient_acceptances"]) == 1 && integer(fin["all_channel_requests"]) == count, "no_delivery_replay")
	if name == "lost_ack" {
		require(notice["status"] == "delivery_unknown" && notice["channel_ack"] == "" && notice["error_code"] == "sending_claim_expired", "lost_ack_stays_unknown")
	} else {
		require(notice["status"] == "sent" && notice["channel_ack"] == "101", "one_confirmed_delivery")
	}
	initiatives := rows(pg, "initiatives")
	require(len(initiatives) == 1 && initiatives[0]["task_id"] == root && initiatives[0]["status"] == "completed" && initiatives[0]["reason"] == "verified_task_result", "initiative_original_problem_closed")
}
func jsonNumber(n int64) any { return clone(n) }
func verifyBudget(pg object, native []object, name, root, child string) {
	verifyBudgetWithPolicy(pg, native, name, root, child, v6AuditPolicy())
}
func verifyBudgetWithPolicy(pg object, native []object, name, root, child string, policy auditPolicy) {
	reservations := byID(rows(pg, "budget_reservations"), "reservation_id")
	allScopes := byID(rows(pg, "budget_scopes"), "scope_id")
	sums := map[string]map[string]int64{}
	for _, res := range reservations {
		require(res["quoted_cost_known"] == true, "conservative_quote_unknown")
		sha(res["fingerprint"])
		keys(obj(res["charge"]), unitKeys)
		ids := arr(res["scope_ids"])
		require(len(ids) > 0 && len(ids) <= 16, "reservation_scope_inventory")
		seen := map[string]bool{}
		for _, value := range ids {
			id := text(value)
			require(allScopes[id] != nil && !seen[id], "reservation_scope_identity")
			seen[id] = true
			if sums[id] == nil {
				sums[id] = map[string]int64{}
			}
			for _, dim := range strings.Fields(unitKeys) {
				sums[id][dim] += integer(obj(res["charge"])[dim])
				require(sums[id][dim] <= 1<<48, "quote_sum_overflow")
			}
		}
	}
	scoped := rows(pg, "budget_scopes")
	foundTask, foundDay, foundChild := false, false, false
	for _, scope := range scoped {
		limit, used := obj(scope["limits"]), obj(scope["reserved"])
		keys(limit, unitKeys)
		keys(used, unitKeys)
		for _, dim := range strings.Fields(unitKeys) {
			require(integer(used[dim]) <= integer(limit[dim]), "normal_budget_exceeded")
			require(integer(used[dim]) == sums[text(scope["scope_id"])][dim], "quoted_scope_sum_mismatch")
		}
		if scope["kind"] == "task" && scope["subject_id"] == root {
			foundTask = true
			require(integer(limit["model_turns"]) == 24 && integer(limit["tokens"]) == 24*1081344 && integer(limit["active_millis"]) == 24*policy.Attempt.Milliseconds(), "root_budget_changed")
		}
		if scope["kind"] == "owner_day" && integer(limit["model_turns"]) == policy.DayModelTurns {
			foundDay = true
			if policy.WindowPolicy != nil {
				require(integer(limit["tokens"]) == policy.DayModelTurns*1081344 && integer(limit["active_millis"]) == policy.DayModelTurns*policy.Attempt.Milliseconds(), "v7_day_budget_changed")
			}
		}
		if scope["kind"] == "delegation" && scope["subject_id"] == child {
			foundChild = true
			require(integer(limit["model_turns"]) == 4 && integer(limit["tokens"]) == 4*1081344 && integer(limit["external_calls"]) == 4, "child_budget_changed")
			if policy.WindowPolicy != nil {
				require(integer(limit["active_millis"]) == policy.ChildActiveMillis, "v7_child_active_budget_changed")
			}
		}
	}
	require(foundTask && foundDay && foundChild, "ordinary_budget_scopes_missing")
	byRun := map[string][]object{}
	all := rows(pg, "model_budget_attempts")
	byOperation := byID(all, "operation_id")
	for _, a := range all {
		res := reservations[text(a["reservation_id"])]
		require(res != nil && res["operation_id"] == a["operation_id"] && a["provider"] == "antigravity" && a["model"] == "gemini-3.8-flash-high" && integer(a["input_allowance"]) == 1048576 && integer(a["output_allowance"]) == 32768, "normal_kernel_admission")
		charge := obj(res["charge"])
		require(integer(charge["model_turns"]) == 1 && integer(charge["tokens"]) == 1081344 && integer(charge["active_millis"]) == policy.Attempt.Milliseconds(), "quote_is_not_measured_cost")
		byRun[text(a["run_id"])] = append(byRun[text(a["run_id"])], a)
	}
	for _, a := range byRun {
		sortedByTime(a, "created_at")
	}
	consumed := map[string]bool{}
	for _, n := range native {
		match := byOperation[text(n["kernel_operation_id"])]
		require(match != nil && !consumed[text(match["operation_id"])] && match["run_id"] == n["run_id"] && !moment(match["created_at"]).After(moment(n["admitted_at_utc"])) && match["actual_input_tokens"] == n["actual_input_tokens"] && match["actual_output_tokens"] == n["actual_output_tokens"], "native_measurement_not_kernel_correlated")
		if n["outcome"] != "completed" && n["actual_input_tokens"] == nil && n["actual_output_tokens"] == nil && n["actual_total_tokens"] == nil {
			require(match["observed_at"] == nil, "unmeasured_timeout_kernel_nulls_preserved")
		} else {
			require(match["observed_at"] != nil, "known_native_usage_requires_kernel_observation")
		}
		consumed[text(match["operation_id"])] = true
	}
	extras := 0
	for _, a := range all {
		if !consumed[text(a["operation_id"])] {
			extras++
			require(name == "owner_stop" && a["actual_input_tokens"] == nil && a["actual_output_tokens"] == nil && a["observed_at"] == nil, "unmatched_kernel_measurement")
		}
	}
	require(name == "owner_stop" && extras <= 2 || name != "owner_stop" && extras == 0, "stop_pre_provider_quote_boundary")
	runs := byID(rows(pg, "runs"), "run_id")
	childCalls := 0
	for _, a := range all {
		run := runs[text(a["run_id"])]
		require(run != nil, "kernel_run_lineage")
		if run["task_id"] == child {
			childCalls++
		}
	}
	require(childCalls <= 4, "native_child_turn_budget")
}
func verifyActions(pg object, native []object, runs map[string]object) {
	actions := rows(pg, "actions")
	byID(actions, "action_id")
	byID(actions, "effect_id")
	byID(actions, "fingerprint")
	auth := byID(rows(pg, "authorizations"), "action_id")
	dispatch := byID(rows(pg, "dispatches"), "execution_action_id")
	require(len(auth) == len(actions) && len(dispatch) == len(actions), "action_authorization_dispatch_inventory")
	used := map[int64]bool{}
	for _, a := range actions {
		sha(a["action_hash"])
		sha(a["fingerprint"])
		id := text(a["action_id"])
		grant, d := auth[id], dispatch[id]
		require(grant != nil && d != nil && a["state"] == "effect_confirmed" && a["dispatch_outcome"] == "dispatch_completed" && grant["state"] == "dispatch_completed" && d["outcome"] == "dispatch_completed" && d["authorization_id"] == grant["authorization_id"] && d["action_id"] == a["effect_id"] && grant["action_hash"] == a["action_hash"], "broker_current_action_not_confirmed")
		require(runs[text(a["run_id"])] != nil && !moment(grant["expires_at"]).Before(moment(grant["finished_at"])) && !moment(d["finished_at"]).Before(moment(d["dispatched_at"])), "current_action_deadline")
		var match object
		at := moment(a["created_at"])
		for _, n := range native {
			if n["outcome"] == "completed" && n["run_id"] == a["run_id"] && integer(n["final_count"]) == 0 && !used[integer(n["ordinal"])] && !moment(n["finished_at_utc"]).After(at) {
				if match == nil || moment(n["finished_at_utc"]).After(moment(match["finished_at_utc"])) {
					match = n
				}
			}
		}
		require(match != nil, "tool_effect_before_durable_native_measurement")
		used[integer(match["ordinal"])] = true
	}
	for _, n := range native {
		if n["outcome"] != "completed" {
			require(!used[integer(n["ordinal"])], "failed_retry_cannot_dispatch")
		} else {
			require(integer(n["final_count"]) == 1 || used[integer(n["ordinal"])], "native_tool_packet_without_owned_action")
		}
	}
}
func verifyAuthority(pg object, root string, native []object) {
	goals := byID(rows(pg, "goals"), "goal_id")
	mandates := byID(rows(pg, "mandates"), "mandate_id")
	bindings := byID(rows(pg, "task_authority"), "task_id")
	b := bindings[root]
	require(b != nil, "owner_authority_binding_missing")
	g, m := goals[text(b["goal_id"])], mandates[text(b["mandate_id"])]
	require(g != nil && m != nil && g["status"] == "active" && m["status"] == "active" && m["revoked_at"] == nil && g["revision"] == b["goal_revision"] && m["revision"] == b["mandate_revision"] && m["goal_id"] == b["goal_id"] && m["goal_revision"] == b["goal_revision"], "current_goal_mandate")
	last := moment(native[len(native)-1]["finished_at_utc"])
	require(moment(g["expires_at"]).After(last) && moment(m["expires_at"]).After(last), "owner_authority_expired")
}
func verifyAdapter(pg, ini object, root, child string, refs object, sources map[string]object) {
	versions, catalog, states := rows(pg, "converter_versions"), rows(pg, "converter_catalog"), rows(pg, "converter_states")
	require(len(versions) == 1 && len(catalog) == 1 && len(states) == 1, "one_missing_adapter_developed")
	v, c := versions[0], catalog[0]
	dig := sha(v["digest"])
	need := sha(ini["target_need_sha256"])
	require(v["converter_id"] == ini["target_converter_id"] && v["need_hash"] == need && c["converter_id"] == v["converter_id"] && c["active_digest"] == dig && integer(c["generation"]) > 0 && states[0]["digest"] == dig && states[0]["status"] == "active", "original_adapter_activation")
	ret := rows(pg, "converter_returns")
	adm := rows(pg, "developer_admissions")
	del := rows(pg, "delegations")
	require(len(ret) == 1 && len(adm) == 1 && len(del) == 1 && ret[0]["digest"] == dig && ret[0]["child_task_id"] == child && ret[0]["parent_task_id"] == root && adm[0]["task_id"] == child && adm[0]["parent_task_id"] == root && del[0]["task_id"] == child && del[0]["root_task_id"] == root && del[0]["parent_task_id"] == root && integer(del[0]["depth"]) == 1, "native_delegation_return")
	runs := byID(rows(pg, "runs"), "run_id")
	require(runs[text(v["created_by_run"])] != nil && runs[text(v["created_by_run"])]["task_id"] == child, "adapter_not_native_child_developed")
	validations := rows(pg, "converter_validations")
	require(len(validations) >= 2, "independent_parent_checks_missing")
	parent, childCheck := false, false
	for _, x := range validations {
		require(x["digest"] == dig && x["package_hash"] == dig && x["need_hash"] == need && x["profile_hash"] == v["profile_pin"] && x["checker"] == "butler.delimited-json.checks.v1" && x["passed"] == "true" && runs[text(x["run_id"])] != nil, "adapter_check_original_contract")
		if runs[text(x["run_id"])]["task_id"] == root {
			parent = true
		}
		if runs[text(x["run_id"])]["task_id"] == child {
			childCheck = true
		}
	}
	require(parent && childCheck, "separate_child_parent_checks")
	trials, uses := rows(pg, "converter_trials"), rows(pg, "converter_usages")
	require(len(trials) == 1 && len(uses) == 1, "one_trial_and_original_use")
	expected := digest([]byte(`[{"label":"private-original-document","quantity":2}]`))
	for _, x := range []object{trials[0], uses[0]} {
		require(x["digest"] == dig && x["task_id"] == root && x["output_hash"] == expected, "exact_original_conversion")
		ref := obj(x["output_ref_json"])
		s := sources[text(ref["source_id"])]
		require(s != nil && s["content_hash"] == expected && s["status"] == "active", "converted_original_source")
	}
	require(trials[0]["status"] == "passed" && reflect.DeepEqual(trials[0]["input_ref_json"], refs["document"]) && uses[0]["generation"] == c["generation"], "original_trial_and_current_catalog_use")
	continuations := rows(pg, "continuations")
	require(len(continuations) == 2, "two_ordinary_parent_continuations")
	purposes := map[string]bool{}
	inputs := byID(rows(pg, "accepted_inputs"), "input_id")
	for _, x := range continuations {
		require(x["parent_task_id"] == root && x["child_task_id"] == child && inputs[text(x["input_id"])] != nil && !purposes[text(x["purpose"])], "continuation_current_lineage")
		purposes[text(x["purpose"])] = true
		sha(x["intent_hash"])
		sha(x["ready_hash"])
	}
	require(purposes["candidate_return"] && purposes["activated_catalog"], "original_parent_continuation_purposes")
}
func verifyOriginalResult(fin object, actions []object) {
	sha(fin["patch_artifact_file_sha256"])
	o := obj(fin["original_result"])
	keys(o, "patch_count artifact_id project_revision changes patch_tool_call_id check_tool_call_id prepare_receipt_count check_receipt_count prepare_receipt_artifact_id check_receipt_artifact_id checks_passed applied")
	require(integer(o["patch_count"]) == 1 && integer(o["prepare_receipt_count"]) == 1 && integer(o["check_receipt_count"]) == 1 && o["checks_passed"] == true && o["applied"] == false && o["artifact_id"] == o["prepare_receipt_artifact_id"] && o["artifact_id"] == o["check_receipt_artifact_id"], "actual_original_patch_receipt")
	sha(o["artifact_id"])
	require(o["project_revision"] == digest([]byte("extension-project-fixture\x00Butler reviewed fixture\nready=false\n")), "original_project_revision")
	changes := arr(o["changes"])
	require(len(changes) == 1, "one_original_patch")
	c := obj(changes[0])
	keys(c, "path before_sha256 after_sha256")
	require(c["path"] == "README.md" && c["before_sha256"] == digest([]byte("Butler reviewed fixture\nready=false\n")) && c["after_sha256"] == digest([]byte("Butler reviewed fixture\nready=true\n")), "exact_prepared_original_bytes")
	foundPrepare, foundCheck := 0, 0
	for _, a := range actions {
		if a["tool_call_id"] == o["patch_tool_call_id"] && a["tool_name"] == "extension.project.prepare-patch" {
			foundPrepare++
		}
		if a["tool_call_id"] == o["check_tool_call_id"] && a["tool_name"] == "extension.project.check" {
			foundCheck++
		}
	}
	require(foundPrepare == 1 && foundCheck == 1, "original_patch_actual_broker_actions")
}
