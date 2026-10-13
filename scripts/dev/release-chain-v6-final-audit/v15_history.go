package main

import (
	"fmt"
	"reflect"
)

const v15HistorySHA = "bcb0716067930c1cd65cbbce869ae5bcda9056ce0e6bb0b034c3c845644fef28"
const v15BridgeSHA = "267a715de1c479773222368006eaaf21d5f852c6909271a44828846befe50e65"

func retainedFailedV14Pins() map[string]string {
	return map[string]string{
		"final-audit":         "aeb9dce14ef0310b96de9cda1eeded0889d8ab7b2bec95b512be1d843df8009a",
		"high":                "3cd185f57451d6eda8fb6ebcc9e15848a287dbd11e047efee95468b8af2ae511",
		"pg-ledger":           "077c5de67b8d3d0d7a922f4e625f52ae3fcc3f10a8e8d4fed176369038f70b82",
		"pg-product":          "34dd19c4e08f9e2d3b574e998c1b3a6e7ef3d3e08a23243de39529ab09f1c400",
		"pg-initial":          "bc2f5e4a34866b66961c15a5ba34892be9fe87de4ca71bc3e1d0ab50992c4728",
		"runtime-state":       "4acfc702315bb4a216690edae6291d7a4e1c3aa071cdcce7115e7182fd3a39fa",
		"failure-correlation": "b52ac5c96bf62674fa0c81a5201fccdc419b76eff6bed011150483865de04e23",
	}
}

func readRetainedFailedV14(root string) map[string]any {
	all := map[string]any{}
	for name, pin := range retainedFailedV14Pins() {
		all[name] = pinnedObject(root, "release-chain-v14-"+name+"-20261013.json", pin)
	}
	all["history"] = pinnedObject(root, "release-chain-v14-history611-20261013.json", v15HistorySHA)
	all["bridge"] = pinnedObject(root, "release-chain-v14-carry611-independent-20261013.json", v15BridgeSHA)
	all["prior"] = pinnedObject(root, "release-chain-v13-history593-20261013.json", v14HistorySHA)
	return all
}

func verifyRetainedFailedV14Source(root string) {
	verifyRetainedFailedV13Source(root)
	verifyV13BootstrapSource(root)
	verifyRetainedFailedV14(readRetainedFailedV14(root))
}

// V14 had no frozen mode-specific checker at completion. Its independent
// supplemental record stays distinct from future canonical Go output.
func verifyRetainedFailedV14(all map[string]any) {
	h, prior, b := obj(all["history"]), obj(all["prior"]), obj(all["bridge"])
	usage, old := arr(h["usage"]), arr(prior["usage"])
	require(len(old) == 593 && len(usage) == 611 && integer(h["native_attempts"]) == 611 && h["status"] == prior["status"] && h["baseline_sha256"] == v15PriorSHA && reflect.DeepEqual(usage[:593], old), "v15_old_measurements_immutable")
	artifacts, oldArtifacts := arr(h["artifacts"]), arr(prior["artifacts"])
	require(len(artifacts) == len(oldArtifacts)+1 && reflect.DeepEqual(artifacts[:len(oldArtifacts)], oldArtifacts) && reflect.DeepEqual(obj(artifacts[len(oldArtifacts)]), frozenObject(`{"report_sha256":"aeb9dce14ef0310b96de9cda1eeded0889d8ab7b2bec95b512be1d843df8009a","release_ref":"9c9c826e4df62f97704473e17bd86562c187cf9a","new_attempts":18}`)), "v15_old_artifacts_immutable")
	pins := retainedFailedV14Pins()
	vector := []any{v14PriorSHA, pins["final-audit"], pins["high"], pins["pg-ledger"], pins["pg-product"], pins["pg-initial"], pins["runtime-state"], pins["failure-correlation"], "9c9c826e4df62f97704473e17bd86562c187cf9a", "2088977d4631e9a13167dede98ad74990114f4addfbfb51d89b0e7e4a13783e3", "aadbd7333a196c4162404236989fb49a2df24a3335368d6efeeb8771d17dc361"}
	require(b["fold_sha256"] == v15PriorSHA && hashValue(vector) == v15PriorSHA && reflect.DeepEqual(arr(b["fold_vector"]), vector) && integer(b["remaining_original_absolute713_attempts"]) == 102 && b["prospective_model_admission"] == false && b["old_originals_or_measurements_changed"] == false && b["original_metadata_preserved"] == true && b["all_fourteen_qualified_original_packets_match"] == true, "v15_closed713_no_refund")
	c, sc, ledger := obj(all["failure-correlation"]), obj(all["high"]), obj(all["pg-ledger"])
	carry := obj(c["carry_forward"])
	require(reflect.DeepEqual(carry, obj(b["carry_forward"])) && integer(carry["attempts"]) == 611 && integer(carry["known_total_tokens_subtotal"]) == 1512064 && integer(carry["unknown_total_attempts"]) == 49 && carry["old_unknown_266_cause"] == "UNKNOWN", "v15_historical_expense_honest")
	var input, output int64
	unknown := 0
	for _, v := range usage {
		u := obj(v)
		if u["actual_input_tokens"] == nil {
			require(u["actual_output_tokens"] == nil, "v15_old_cost_pair")
			unknown++
		} else {
			require(u["actual_output_tokens"] != nil, "v15_old_cost_pair")
			input += integer(u["actual_input_tokens"])
			output += integer(u["actual_output_tokens"])
		}
	}
	require(input == 1103351 && output == 25577 && unknown == 29 && integer(h["known_usage_pairs"]) == 582 && integer(h["unknown_usage_pairs"]) == 29, "v15_historical_pairs_recomputed")
	audit := obj(all["final-audit"])
	require(audit["version"] == "butler-independent-v14-failure-supplemental-v1" && audit["canonical_go_json_available"] == false && audit["canonical_go_not_available_reason"] == "V14-specific final auditor was not frozen at this completed run; no wrong-protocol or fabricated JSON is used", "v15_supplemental_not_canonical_go")
	oldAudit := obj(audit["chain_audit"])
	require(oldAudit["status"] == "NO_GO" && integer(oldAudit["carry_forward_attempts"]) == 611 && oldAudit["all_new_expense_measured"] == true && integer(oldAudit["accepted_model_retry_failures"]) == 0 && sc["status"] == "failed" && len(arr(sc["proposal_observations"])) == 14 && integer(sc["maximum_total_attempts"]) == 713 && len(obj(sc["scenarios"])) == 0, "v15_old_no_go_inventory_preserved")
	require(c["status"] == "INDEPENDENT_ACTUAL_V14_CHAIN_NO_GO_CORRELATED" && c["native_scope_sealed"] == true && integer(c["native_request_count"]) == 18 && integer(c["accepted_model_retry_failures"]) == 0 && integer(c["actual_notice_count"]) == 0 && integer(c["actual_original_conversion_usage"]) == 1 && c["canonical_go_json_available"] == false && c["issue33_ready"] == false && reflect.DeepEqual(c["original_proposal_observations"], sc["proposal_observations"]) && c["original_proposal_observations_rewritten"] == false && c["all_fourteen_actual_core_argument_fingerprints_match"] == true && len(arr(c["original_private_packets"])) == 14, "v15_old_no_go_not_promoted")
	f := obj(c["failure_classification"])
	require(f["exact_fatal"] == "original readiness correction missing" && f["original_conversion_done_but_owner_verification_missing"] == true && f["two_actual_check_calls_in_distinct_owned_root_runs"] == true && f["two_actual_checks_same_artifact_true_unapplied"] == true && f["both_owning_pg_result_hashes_equal"] == true && f["source_or_provider_timeout_observed"] == false, "v15_old_readiness_not_rewritten")
	for _, name := range []string{"specific_prior541_cause", "specific_prior544_child_cause", "specific_prior556_inspect_cause", "specific_prior557_source_cause"} {
		require(f[name] == "UNKNOWN", "v15_old_cause_unknown")
	}
	paid := obj(sc["scope"])
	window := obj(paid["window"])
	require(reflect.DeepEqual(window, obj(ledger["window"])) && integer(window["native_baseline"]) == 593 && integer(window["attempts"]) == 611 && window["prior_history_hash"] == v14PriorSHA && window["prerequisite_hash"] == v14PrerequisiteSHA, "v15_old_window_immutable")
	nativeRows, pgRows := arr(paid["requests"]), arr(ledger["requests"])
	require(len(nativeRows) == 18 && len(pgRows) == 18, "v15_old_native_inventory")
	var newInput, newOutput, newTotal int64
	for i, v := range nativeRows {
		n := obj(v)
		verifyRequestWithPolicy(n, obj(pgRows[i]), v14AuditPolicy())
		require(integer(n["ordinal"]) == int64(594+i) && n["outcome"] == "completed" && n["provider_operation_stage"] == "response_stream" && n["model_version"] == "gemini-3.8-flash-n", "v15_old_native_exact_measurements")
		u := obj(usage[593+i])
		require(u["run_id"] == n["run_id"] && u["reservation_id"] == fmt.Sprintf("release-chain-v14-native-%d", 594+i) && u["actual_input_tokens"] == n["actual_input_tokens"] && u["actual_output_tokens"] == n["actual_output_tokens"], "v15_old_pair_native_binding")
		newInput += integer(n["actual_input_tokens"])
		newOutput += integer(n["actual_output_tokens"])
		newTotal += integer(n["actual_total_tokens"])
	}
	require(newInput == 193079 && newOutput == 3556 && newTotal == 215393, "v15_retained_all18_expense")
	// The exact exported leaf is an array containing one qualified witness.
	product := witnesses(all["pg-product"])
	require(len(product) == 1 && product["normal_sent"] != nil, "v15_old_product_inventory")
	fin := obj(product["normal_sent"]["payload_json"])
	db := obj(fin["postgres"])
	require(fin["test_failed"] == true && integer(fin["all_channel_requests"]) == 0 && len(rows(db, "notices")) == 0 && len(rows(db, "converter_usages")) == 1 && len(rows(db, "actions")) == 14, "v15_old_original_no_invented_success")
	var root, child string
	for _, t := range rows(db, "tasks") {
		if t["parent_task_id"] == nil {
			root = text(t["task_id"])
			require(t["status"] == "active", "v15_old_unverified_task_preserved")
		} else {
			child = text(t["task_id"])
		}
	}
	require(root != "" && child != "", "v15_old_tasks")
	verifyBudgetWithPolicy(db, rows(paid, "requests"), "normal_sent", root, child, v14AuditPolicy())
	verifyV11SourceObservation(fin["source_check_observation"], db)
	verifyV12OriginalResult(fin, rows(db, "actions"), byID(rows(db, "runs"), "run_id"), root, child, "developer-inventory")
	for _, result := range rows(db, "task_results") {
		require(result["verdict"] != "verified" && result["verifier"] != "owner", "v15_old_owner_verification_not_invented")
	}
}
