package main

import "reflect"

const v11HistorySHA = "fb85f5ce35a5217990d83af664060908574c0ac0682fa8d111e20ac0c2deef0f"
const v11BridgeSHA = "78d320c595483d666b25cbaee55be764b53b0004bdd6806ecb4924e4f01130ef"

func retainedFailedV10Pins() map[string]string {
	return map[string]string{
		"final-audit":         "28b4dbc9cdfbd8fb8e838b2f14bb698b7142e587f0a44ec2e534a716dce62534",
		"high":                "9161adb7a986911c2c9a714e1c7308e4f4c4b3eeb673f6a6533e73d6489261c8",
		"pg-ledger":           "9329ec125b80f3ca57ac04367f21cd6e2e3c5cab0fa908e42958140b290eaba4",
		"pg-product":          "c0f7d077267c00d22e26b8893579aa20203f10ed8ab590f0f3b048ea1dee5e18",
		"pg-initial":          "e60e6b1ba8a690bbd9c1b72ef5e70612a416005fa1a66e250b508a1fc02c4b1e",
		"runtime-state":       "24692d039c421a6f820208355c98c45812ad6df8c724d1c2a30b872d236ae657",
		"failure-correlation": "f7c20a7246eb2fc5a653f39f4bfe7f06498e152be3d50c4b523357eff163af8d",
	}
}

func readRetainedFailedV10(root string) map[string]any {
	all := map[string]any{}
	for name, pin := range retainedFailedV10Pins() {
		all[name] = pinnedObject(root, "release-chain-v10-"+name+"-20261013.json", pin)
	}
	all["history"] = pinnedObject(root, "release-chain-v10-history557-20261013.json", v11HistorySHA)
	all["bridge"] = pinnedObject(root, "release-chain-v10-carry557-independent-20261013.json", v11BridgeSHA)
	all["prior"] = pinnedObject(root, "release-chain-v9-history556-20261013.json", v10HistorySHA)
	all["historical-exception"] = pinnedObject(root, "release-chain-v11-user-expense-exception-20261013.json", v11HistoricalExceptionSHA)
	return all
}

func verifyRetainedFailedV10Source(root string) {
	verifyRetainedFailedV9Source(root)
	verifyRetainedFailedV10(readRetainedFailedV10(root))
}

func verifyRetainedFailedV10(all map[string]any) {
	verifyV11HistoricalException(all["historical-exception"])
	h, prior, b := obj(all["history"]), obj(all["prior"]), obj(all["bridge"])
	usage, old := arr(h["usage"]), arr(prior["usage"])
	require(integer(h["native_attempts"]) == 557 && len(usage) == 557 && len(old) == 556 && h["status"] == prior["status"] && integer(h["known_usage_pairs"]) == 528 && integer(h["unknown_usage_pairs"]) == 29 && h["baseline_sha256"] == v11PriorSHA && reflect.DeepEqual(usage[:556], old), "v11_old_measurements_immutable")
	artifacts, oldArtifacts := arr(h["artifacts"]), arr(prior["artifacts"])
	require(len(artifacts) == len(oldArtifacts)+1 && reflect.DeepEqual(artifacts[:len(oldArtifacts)], oldArtifacts), "v11_old_artifacts_immutable")
	require(reflect.DeepEqual(obj(artifacts[len(oldArtifacts)]), frozenObject(`{"report_sha256":"28b4dbc9cdfbd8fb8e838b2f14bb698b7142e587f0a44ec2e534a716dce62534","release_ref":"0f9b61e23fd56fefddd1c42cd3aa9efa14087d83","new_attempts":1}`)), "v11_closed557_artifact")
	pins := retainedFailedV10Pins()
	vector := []any{v10PriorSHA, pins["final-audit"], pins["high"], pins["pg-ledger"], pins["pg-product"], pins["pg-initial"], pins["runtime-state"], pins["failure-correlation"], "0f9b61e23fd56fefddd1c42cd3aa9efa14087d83", "2096a0a5f7a80ad8a912dc0df1f7b2452053d68b27b55c6abdd783af6ce96879", "4deec48d416f372c9606b3e6e1d3fc168a8e479427d5be3e7660bf7fc6934694"}
	require(b["status"] == "INDEPENDENT_SEALED557_CARRY_BRIDGE" && b["fold_sha256"] == v11PriorSHA && hashValue(vector) == v11PriorSHA && reflect.DeepEqual(arr(b["fold_vector"]), vector) && integer(b["remaining_original_absolute627_attempts"]) == 70 && b["prospective_model_admission"] == false && b["old_originals_or_measurements_changed"] == false && b["new_unknown_is_nominal_safe_timeout_exception"] == false, "v11_closed627_no_refund_or_retry")
	c, scope, pg := obj(all["failure-correlation"]), obj(all["high"]), obj(all["pg-ledger"])
	require(c["status"] == "INDEPENDENT_ACTUAL_V10_CHAIN_NO_GO_CORRELATED" && c["native_scope_sealed"] == true && integer(c["native_request_count"]) == 1 && integer(c["accepted_model_retry_failures"]) == 0 && c["actual_model_retry"] == nil && c["actual_original_result"] == nil && integer(c["actual_notice_count"]) == 0 && c["issue33_ready"] == false, "v11_old_source_failure_not_promoted")
	require(obj(obj(all["final-audit"])["chain_audit"])["status"] == "NO_GO" && scope["status"] == "failed" && integer(scope["baseline_attempts"]) == 556 && integer(scope["maximum_total_attempts"]) == 627, "v11_previous_no_go_immutable")
	f := obj(c["failure_classification"])
	require(f["core_stop_reason"] == "memory_source_check_timeout" && f["core_source_check_cancellation_not_nominal_model_timeout"] == true && f["current_source_check_underlying_timeout_cause"] == "UNKNOWN_FROM_CURRENT_SAFE_EXPORTS" && f["safe_model_retry_authorized_or_observed"] == false && f["new_unknown_expense_estimated_or_refunded"] == false && f["specific_prior556_inspect_cause"] == "UNKNOWN_FROM_CURRENT_SAFE_EXPORTS", "v11_557_cause_unknown_no_retry")
	carry := obj(c["carry_forward"])
	require(reflect.DeepEqual(carry, obj(b["carry_forward"])) && integer(carry["known_total_tokens_subtotal"]) == 793703 && integer(carry["unknown_total_attempts"]) == 49 && integer(carry["unknown_usage_pairs"]) == 29 && carry["old_unknown_266_cause"] == "UNKNOWN", "v11_historical_unknown_expense_honest")
	var input, output int64
	unknown := 0
	for _, value := range usage {
		u := obj(value)
		if u["actual_input_tokens"] == nil {
			require(u["actual_output_tokens"] == nil, "v11_historical_cost_pair")
			unknown++
		} else {
			require(u["actual_output_tokens"] != nil, "v11_historical_cost_pair")
			input += integer(u["actual_input_tokens"])
			output += integer(u["actual_output_tokens"])
		}
	}
	require(input == 475637 && output == 15159 && unknown == 29, "v11_historical_cost_recomputed")
	paid := obj(scope["scope"])
	window := obj(paid["window"])
	require(reflect.DeepEqual(window, obj(pg["window"])) && integer(window["native_baseline"]) == 556 && integer(window["attempts"]) == 557 && window["prior_history_hash"] == v10PriorSHA && window["prerequisite_hash"] == v10PrerequisiteSHA, "v11_old_window_immutable")
	nativeRows := arr(paid["requests"])
	require(len(nativeRows) == 1 && len(arr(pg["requests"])) == 1, "v11_old_request_inventory")
	r, u := obj(nativeRows[0]), obj(usage[556])
	require(integer(r["ordinal"]) == 557 && r["outcome"] == "cancelled" && r["provider_operation_stage"] == "http_round_trip" && r["actual_input_tokens"] == nil && r["actual_output_tokens"] == nil && r["actual_total_tokens"] == nil && r["model_version"] == nil && u["actual_input_tokens"] == nil && u["actual_output_tokens"] == nil && u["run_id"] == r["run_id"] && u["reservation_id"] == "release-chain-v10-native-557", "v11_original557_null_retained")
	product := witnesses(all["pg-product"])
	require(len(product) == 1 && product["normal_sent"] != nil, "v11_old_product_inventory")
	fin := obj(product["normal_sent"]["payload_json"])
	db := obj(fin["postgres"])
	for _, table := range []string{"actions", "dispatches", "delegations", "converter_versions", "notices", "final_results"} {
		require(len(rows(db, table)) == 0, "v11_no_invented_old_effect_or_result")
	}
	require(fin["original_result"] == nil && integer(fin["all_channel_requests"]) == 0, "v11_no_invented_old_delivery")
}
