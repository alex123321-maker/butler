package main

import (
	"fmt"
	"path/filepath"
	"reflect"
	"time"
)

const v19ClosedV18HistorySHA = "fc07d5fc3e9ddb001d9d73b8c6ad15e7af78961cb5ede31c8701288ab56147d0"
const v19ClosedV18BridgeSHA = "ca1366f6096a3e5fcabf26355ceb5103fe8a765a228bb55f4be4241359fea945"
const v19ClosedV18FinalSHA = "a053abc85135abb38bd20d7e396b34c82e7a363334091ccf04327344e5263c39"
const v19ClosedV18CorrelationSHA = "08c7d4876ec265cac35b02e96fed0da7079720d9b1659ff974e001ade5e9e932"
const v19ClosedV18Fold = "236b21dfb08799ebc2b0eaf463faae875173f2aa45ff392928d439a7ed9e5810"
const v19RetainedAbsenceSHA = "a829eade1b7529ed990d5436ac1e7c9c764f42d9b4210c25131723b8044d11de"
const v19ClosedV18Commit = "67133e17b58acd825b2988a281079e1253da7af8"
const v19ClosedV18Tree = "b917ec423adcfdffbdf0713785cf962b619ceeee03bf4885e2d189fd6c0d5ca3"
const v19ClosedV18App = "5126b31cad0345e9fc296a5b5f43a47ecdba0fd9caa32dd9b358cc0f90719fff"

var v19ClosedV18RawPins = map[string]string{
	"scope":   "7281eebacaad634555f22c3a07c8a72d700fa9c86cb4df686a7860f5dbb55492",
	"ledger":  "7a2418b1293788d6ffbbc1468142fd4a1886370d735166447268fe116711dce2",
	"product": "81edce9ab9b272757a6dff16ab32f0535bfd0c47fe9e449748fd29bf6adec9ef",
	"initial": "d381eb350f0a8932d0c09a2469c2328ec297782ed69a7bfbf4208c25c061bffe",
	"runtime": "5a81015f6c64782f028065e40dcbcd27378065a66a32d16da8c123b0a71927f3",
}

type v19ClosedV18Evidence struct {
	Prior, History, Bridge, Correlation, Final, Component object
	Scope, Ledger, Runtime                                object
	Initial, Product                                      []any
}

// The caller first verifies the unchanged 658-history prefix and Stop17 proof
// with verifyV18RetainedHistory. These five additional *safe* original leaves
// are individually mounted read-only, never a private runtime directory.
func verifyRetainedFailedV18Source(root string) {
	e := v19ClosedV18Evidence{
		Prior:       obj(pinnedObject(root, "release-chain-v17-history658-20261013.json", v17HistorySHA)),
		History:     obj(pinnedObject(root, "release-chain-v18-history705-20261013.json", v19ClosedV18HistorySHA)),
		Bridge:      obj(pinnedObject(root, "release-chain-v18-carry705-independent-20261013.json", v19ClosedV18BridgeSHA)),
		Correlation: obj(pinnedObject(root, "release-chain-v18-failure-correlation-20261013.json", v19ClosedV18CorrelationSHA)),
		Final:       obj(pinnedObject(root, "release-chain-v18-final-audit-20261013.json", v19ClosedV18FinalSHA)),
		Component:   obj(pinnedObject(root, "v18-explicit-absence-component-independent-20261013.json", v19RetainedAbsenceSHA)),
	}
	values := map[string]any{}
	for _, name := range []string{"scope", "ledger", "product", "initial", "runtime"} {
		raw, err := readExact(filepath.Join("/retained-v18", name+".json"), v19ClosedV18RawPins[name], 8<<20)
		require(err == nil, "v19_retained_v18_named_safe_leaf")
		values[name], err = decode(raw)
		require(err == nil, "v19_retained_v18_original_json")
	}
	e.Scope, e.Ledger, e.Runtime = obj(values["scope"]), obj(values["ledger"]), obj(values["runtime"])
	e.Initial, e.Product = arr(values["initial"]), arr(values["product"])
	verifyV19ClosedV18Evidence(e)
}

func verifyV19ClosedV18Evidence(e v19ClosedV18Evidence) {
	verifyV18ClosedHistoryObjects(e.Prior, e.History, e.Bridge, 18, 658, 705, v19ClosedV18Fold, 2649327, 2098118, 42953, 52, 32)
	pins := obj(e.Bridge["new_failed_v18_artifacts"])
	expected := object{"final-audit": v19ClosedV18FinalSHA, "high": v19ClosedV18RawPins["scope"], "pg-ledger": v19ClosedV18RawPins["ledger"], "pg-product": v19ClosedV18RawPins["product"], "pg-initial": v19ClosedV18RawPins["initial"], "runtime-state": v19ClosedV18RawPins["runtime"], "failure-correlation": v19ClosedV18CorrelationSHA}
	require(reflect.DeepEqual(pins, expected), "v19_exact_closed_v18_pins")
	vector := arr(e.Bridge["fold_vector"])
	require(vector[8] == v19ClosedV18Commit && vector[9] == v19ClosedV18Tree && vector[10] == v19ClosedV18App && e.Bridge["canonical_go_json_available"] == false && e.Bridge["originals_or_measurements_changed"] == false && integer(e.Bridge["remaining_original_absolute778_attempts"]) == 73, "v19_closed_v18_source_no_refund")
	for _, d := range []object{e.Final, e.Correlation} {
		b := obj(d["evidence_bindings"])
		require(b["code_commit"] == v19ClosedV18Commit && b["source_sha256"] == v19ClosedV18Tree && b["app_sha256"] == v19ClosedV18App && b["prerequisite_sha256"] == v18PrerequisiteSHA && b["human_policy_sha256"] == v18HumanSHA && d["canonical_go_json_available"] == false, "v19_closed_v18_provenance")
	}
	report := obj(e.Final["chain_audit"])
	require(report["status"] == "NO_GO" && report["issue33_ready"] == false && integer(report["carry_forward_attempts"]) == 705 && len(arr(report["verified_scenarios"])) == 0 && reflect.DeepEqual(arr(report["failed_scenarios"]), []any{"lost_ack", "restart_sent"}) && report["common_completion_evidence"] == nil, "v19_closed_v18_not_rehabilitated")
	for _, d := range []object{report, e.Correlation} {
		require(integer(d["accepted_model_retry_failures"]) == 0, "v19_closed_v18_no_retry")
	}
	require(e.Correlation["final_audit_sha256"] == v19ClosedV18FinalSHA && e.Correlation["original_product_rewritten"] == false && e.Correlation["original_proposal_observations_rewritten"] == false && e.Correlation["original_scope_lost_ack_omission_preserved"] == true && e.Correlation["specific_prior658_cancellation_cause"] == "UNKNOWN", "v19_closed_v18_original_boundary")
	require(e.Scope["status"] == "failed" && e.Scope["issue33_ready"] == false && integer(e.Scope["baseline_attempts"]) == 658 && integer(e.Scope["maximum_new_attempts"]) == 120 && integer(e.Scope["maximum_total_attempts"]) == 778 && reflect.DeepEqual(obj(e.Scope["scenarios"]), object{"explicit_absence": true, "restart_sent": false}) && reflect.DeepEqual(e.Correlation["original_scope_scenarios"], e.Scope["scenarios"]), "v19_closed_v18_missing_scenario_not_invented")
	sc := obj(e.Scope["scope"])
	require(integer(e.Ledger["baseline_attempts"]) == 658 && integer(e.Ledger["total_attempts"]) == 705 && integer(e.Ledger["new_attempts"]) == 47 && reflect.DeepEqual(sc["window"], e.Ledger["window"]), "v19_closed_v18_scope_pg_counter")
	w := obj(e.Ledger["window"])
	require(integer(w["attempts"]) == 705 && integer(w["native_baseline"]) == 658 && integer(w["maximum_new_attempts"]) == 120 && w["prior_history_hash"] == v18PriorSHA && w["prerequisite_hash"] == v18PrerequisiteSHA && w["kernel_operation_binding"] == true && w["provider_stage_observation"] == true && reflect.DeepEqual(w["protocol_policy"], v18AuditPolicy().WindowPolicy), "v19_closed_v18_exact_window")
	pgRequests, scopeRequests := rows(e.Ledger, "requests"), rows(sc, "requests")
	require(len(pgRequests) == 47 && len(scopeRequests) == 47 && reflect.DeepEqual(e.Correlation["requests"], e.Ledger["requests"]), "v19_closed_v18_native_inventory")
	usage := arr(e.History["usage"])
	var input, output, total int64
	for i, r := range pgRequests {
		n := scopeRequests[i]
		verifyV19RetainedPGScopeRequest(r, n)
		scenario := "explicit_absence"
		if i >= 17 {
			scenario = "lost_ack"
		}
		if i >= 32 {
			scenario = "restart_sent"
		}
		require(integer(r["ordinal"]) == int64(659+i) && r["scenario"] == scenario && r["provider_operation_stage"] == "response_stream", "v19_closed_v18_paid_order")
		if i != 0 {
			require(!moment(pgRequests[i-1]["finished_at"]).After(moment(r["admitted_at"])), "v19_closed_v18_serialized_provider")
		}
		u := obj(usage[658+i])
		require(u["run_id"] == r["run_id"] && u["reservation_id"] == fmt.Sprintf("release-chain-v18-native-%d", 659+i) && u["actual_input_tokens"] == r["input_tokens"] && u["actual_output_tokens"] == r["output_tokens"], "v19_closed_v18_usage_exact_pg")
		if i < 46 {
			verifyRequestWithPolicy(n, r, v18AuditPolicy())
			require(r["outcome"] == "completed", "v19_closed_v18_known_completed")
			input += integer(r["input_tokens"])
			output += integer(r["output_tokens"])
			total += integer(r["total_tokens"])
		} else {
			require(r["outcome"] == "cancelled" && integer(r["elapsed_ms"]) == 18911 && r["failure_stage"] == "stream" && r["diagnostic_code"] == "transport_connection_error" && r["input_tokens"] == nil && r["output_tokens"] == nil && r["total_tokens"] == nil && r["model_version"] == nil && r["finish_reason"] == nil, "v19_705_unknown_source_cancel_not_timeout")
			for _, k := range []string{"final_count", "terminal_count", "completed_count", "cancelled_count", "timed_out_count", "transport_error_count", "stop_final_count", "max_tokens_final_count"} {
				require(integer(r[k]) == 0, "v19_705_no_fabricated_terminal_event")
			}
			for _, k := range []string{"usage_observation_uncertain", "version_observation_uncertain", "finish_observation_uncertain"} {
				require(r[k] == false, "v19_705_observation_flags")
			}
		}
	}
	require(input == 496208 && output == 8642 && total == 549811, "v19_closed_v18_all_native_cost")
	cost := obj(e.Bridge["actual_native_cost"])
	require(integer(cost["attempts"]) == 47 && integer(cost["known_input_tokens"]) == input && integer(cost["known_output_tokens"]) == output && integer(cost["known_total_tokens"]) == total && integer(cost["known_usage_pairs"]) == 46 && integer(cost["unknown_input_attempts"]) == 1 && integer(cost["unknown_output_attempts"]) == 1 && integer(cost["unknown_total_attempts"]) == 1 && reflect.DeepEqual(report["all_new_measured_cost"], cost) && reflect.DeepEqual(e.Correlation["all_new_measured_cost"], cost), "v19_closed_v18_cost_correlated")
	for _, k := range []string{"failed_provider_entries", "unknown_full_token_cost", "unknown_token_cost_entries", "unknown_model_version_entries"} {
		require(integer(sc[k]) == 1, "v19_closed_v18_factual_unknown_counts")
	}
	for _, k := range []string{"accepted_model_retry_failures", "accepted_unknown_timeout_cost_entries", "accepted_unknown_timeout_version_entries", "accepted_unmeasured_timeout_entries", "overlong_provider_entries"} {
		require(integer(sc[k]) == 0, "v19_closed_v18_no_unknown_waiver")
	}
	require(integer(e.Runtime["exit_code"]) == 1 && e.Runtime["oom_killed"] == false && moment(e.Runtime["started_at"]).Before(moment(pgRequests[0]["admitted_at"])) && moment(pgRequests[46]["finished_at"]).Before(moment(e.Runtime["finished_at"])) && moment(e.Runtime["finished_at"]).Sub(moment(e.Runtime["started_at"])) <= 120*time.Minute, "v19_closed_v18_runtime")
	initial, product := v19RetainedWrappers(e.Initial), v19RetainedWrappers(e.Product)
	require(reflect.DeepEqual(e.Correlation["original_pg_product_witness"], e.Product) && reflect.DeepEqual(e.Correlation["original_proposal_observations"], e.Scope["proposal_observations"]), "v19_closed_v18_raw_projection_identity")
	for _, name := range []string{"explicit_absence", "lost_ack", "restart_sent"} {
		f := obj(product[name]["payload_json"])
		if name == "explicit_absence" {
			continue
		}
		verifyV19RetainedWatchdogFailure(name, f, pgRequests[46], obj(obj(e.Correlation["case_failure_correlations"])[name]))
	}
	verifyV19RetainedAbsence(e.Component, initial["explicit_absence"], product["explicit_absence"], scopeRequests[:17], arr(e.Scope["proposal_observations"]))
}

func verifyV19RetainedPGScopeRequest(pg, scope object) {
	rename := map[string]string{"input_hash": "input_sha256", "input_tokens": "actual_input_tokens", "output_tokens": "actual_output_tokens", "total_tokens": "actual_total_tokens", "admitted_at": "admitted_at_utc", "finished_at": "finished_at_utc"}
	n := object{}
	for k, v := range pg {
		if name := rename[k]; name != "" {
			k = name
		}
		n[k] = v
	}
	for _, k := range []string{"admitted_at_utc", "finished_at_utc"} {
		require(moment(n[k]).Equal(moment(scope[k])), "v19_closed_v18_exact_pg_time")
		n[k] = scope[k]
	}
	require(reflect.DeepEqual(n, scope), "v19_closed_v18_exact_pg_scope_row")
}

func v19RetainedWrappers(values []any) map[string]object {
	require(len(values) == 3, "v19_closed_v18_case_wrappers")
	m := map[string]object{}
	for i, value := range values {
		v := obj(value)
		name := text(v["scenario"])
		require(name == []string{"explicit_absence", "lost_ack", "restart_sent"}[i] && v["payload_sha256"] == hashValue(v["payload_json"]) && m[name] == nil, "v19_closed_v18_wrapper_hash_order")
		m[name] = v
	}
	return m
}

func verifyV19RetainedWatchdogFailure(name string, fin, last, correlation object) {
	pg := obj(fin["postgres"])
	require(fin["test_failed"] == true && fin["original_result_capture_status"] == "captured" && len(rows(pg, "notices")) == 0 && correlation["pool_connection_holder_or_lock_subcause"] == "UNKNOWN" && correlation["nominal_model_timeout"] == false && correlation["provider_quota_or_rate_limit_proved"] == false && reflect.DeepEqual(correlation["source_check_observation"], fin["source_check_observation"]), "v19_closed_v18_failed_case_not_passed")
	var blocked object
	for _, input := range rows(pg, "accepted_inputs") {
		if input["status"] == "blocked" {
			require(blocked == nil, "v19_closed_v18_single_blocked_input")
			blocked = input
		}
	}
	require(blocked != nil && blocked["source"] == "converter-return" && blocked["checkpoint"] == "model_budget_reserved" && blocked["error_code"] == "execution_stopped" && blocked["model_retry"] == nil && reflect.DeepEqual(blocked, correlation["blocked_input"]), "v19_closed_v18_source_failure_no_retry")
	obs := obj(fin["source_check_observation"])
	require(integer(obs["dropped_failed_checks"]) == 0 && len(arr(obs["failed_checks"])) == 2, "v19_closed_v18_failure_observation_inventory")
	var watchdog, source object
	for _, v := range arr(obs["failed_checks"]) {
		f := obj(v)
		require(f["run_id"] == blocked["run_id"], "v19_closed_v18_same_current_run_failure")
		switch f["role"] {
		case "task_authority_watchdog":
			require(watchdog == nil, "v19_closed_v18_watchdog_unique")
			watchdog = f
		case "source_check":
			require(source == nil, "v19_closed_v18_source_unique")
			source = f
		default:
			require(false, "v19_closed_v18_unexpected_observer_role")
		}
	}
	require(watchdog != nil && source != nil && watchdog["phase"] == "pool_acquire" && watchdog["failure_category"] == "deadline_exceeded" && integer(watchdog["maximum_check_ms"]) == 750 && source["phase"] == "query" && source["failure_category"] == "cancelled" && integer(source["maximum_check_ms"]) == 2000, "v19_closed_v18_watchdog_source_boundary")
	pool := obj(watchdog["pool_after"])
	require(integer(pool["max_conns"]) == 2 && integer(pool["acquired_conns"]) == 2 && integer(pool["idle_conns"]) == 0, "v19_closed_v18_pool_saturation_observed")
	elapsed, remaining := int64(551), int64(550)
	if name == "restart_sent" {
		elapsed, remaining = 218, 218
		require(source["kernel_operation_id"] == last["kernel_operation_id"] && source["run_id"] == last["run_id"] && !moment(source["finished_at_utc"]).After(moment(last["finished_at"])), "v19_closed_v18_705_bound_source_cancel")
	}
	require(integer(watchdog["elapsed_ms"]) == elapsed && integer(watchdog["context_time_left_ms_at_start"]) == remaining && moment(source["finished_at_utc"]).Sub(moment(watchdog["finished_at_utc"])) >= 0 && moment(source["finished_at_utc"]).Sub(moment(watchdog["finished_at_utc"])) < time.Millisecond && moment(watchdog["finished_at_utc"]).Before(moment(blocked["updated_at"])), "v19_closed_v18_actual_failure_timing")
}

func verifyV19RetainedAbsence(component, iniWrapper, finalWrapper object, native []object, metadata []any) {
	require(component["status"] == "EXPLICIT_ABSENCE_CRITICAL_COMPONENT_PASS" && component["overall_chain_status"] == "NO_GO" && component["all_release_cases_passed"] == false && component["issue33_ready"] == false && component["original_measurements_changed"] == false && integer(component["native_auth_or_model_calls_by_auditor"]) == 0, "v19_retained_absence_component_only")
	require(component["native_code_commit"] == v19ClosedV18Commit && component["current_case_payload_sha256"] == finalWrapper["payload_sha256"] && component["initial_case_payload_sha256"] == iniWrapper["payload_sha256"] && component["actual_original_csv_output_sha256"] == digest([]byte(v18GoldenOutput)), "v19_retained_absence_original_source_binding")
	for _, k := range []string{"all13_qualified_current_grants_and_dispatches_confirmed", "all17_measured_kernel_operations_correlated", "all_eight_budget_dimensions_recomputed", "child_and_independent_parent_adapter_validation", "cleanup_cancel_observation_occurs_after_completed_run", "one_confirmed_notice_after_definite_absence_two_attempts_one_acceptance", "one_prepare_one_immutable_unapplied_patch_actual_check_true", "owner_verified_same_original_task_result", "root_and_child_correct_final_states"} {
		require(component[k] == true, "v19_retained_absence_critical_proof")
	}
	require(integer(component["duplicate_external_mutations"]) == 0 && len(arr(component["all13_original_named_packets"])) == 13, "v19_retained_absence_no_duplicate_mutation")
	// Recompute the complete safe initial-to-final critical predicates, including
	// budget/kernel rows, current sources, qualified task actions, immutable
	// artifact/check receipts and actual definite-absence delivery retry. The
	// pinned separate peer component binds the 13 exact original private packet
	// digests; this helper does not import raw authentication or hidden stream.
	verifyProductWithPolicy("explicit_absence", obj(iniWrapper["payload_json"]), obj(finalWrapper["payload_json"]), native, v18AuditPolicy())
	indexed := map[int64]object{}
	for _, v := range metadata {
		m := obj(v)
		if m["scenario"] == "explicit_absence" {
			require(indexed[integer(m["ordinal"])] == nil, "v19_retained_absence_metadata_unique")
			indexed[integer(m["ordinal"])] = m
		}
	}
	require(len(indexed) == 13, "v19_retained_absence_metadata_inventory")
	byOrdinal := map[int64]object{}
	for _, r := range native {
		byOrdinal[integer(r["ordinal"])] = r
	}
	seenActions := map[string]bool{}
	for _, value := range arr(component["all13_original_named_packets"]) {
		p := obj(value)
		m := indexed[integer(p["ordinal"])]
		require(m != nil && p["private_proposal_sha256"] == m["private_proposal_sha256"] && p["original_arguments_sha256"] == m["arguments_sha256"] && p["actual_core_authorization_arguments_hash_matches"] == true && p["qualified_current_grant_and_confirmed_dispatch"] == true, "v19_retained_absence_original_packet_proof")
		r := byOrdinal[integer(p["ordinal"])]
		require(r != nil, "v19_retained_absence_packet_native_identity")
		matches := 0
		for _, action := range rows(obj(obj(finalWrapper["payload_json"])["postgres"]), "actions") {
			if action["run_id"] == r["run_id"] && action["tool_name"] == p["tool_name"] && action["args_hash"] == m["arguments_authorization_sha256"] {
				id := text(action["action_id"])
				require(!seenActions[id] && action["state"] == "effect_confirmed" && action["dispatch_outcome"] == "dispatch_completed", "v19_retained_absence_current_action_binding")
				seenActions[id] = true
				matches++
			}
		}
		require(matches == 1, "v19_retained_absence_packet_current_action")
	}
}
