package main

import (
	"fmt"
	"reflect"
	"time"
)

const v16HistorySHA = "d728ed54a57409cf0d86d656bb0953c934688acd6ee796b3ea018a120fcc5030"
const v16BridgeSHA = "629949179247a74b90750491d4bca850270bf70bb78c2868f6aa2c66e947188b"
const v16PriorSHA = "aa561f243f5d36c3c676ef442f42a83a4514faf8abd927e31da0c2a78ccaa975"

func retainedFailedV15Pins() map[string]string {
	return map[string]string{
		"final-audit":         "dede808835fbb71b004b226221a6083b6518e745e20a59ebf670947c9e7eec10",
		"high":                "998ae4eb78eb090ac6e4aa743fa4a7b128c3549f085cfc749cf3cd2fcabbad29",
		"pg-ledger":           "eb7c25e4b19ebd81e75e3c2203f1f21543066040589a39175ac98a07acf2fd21",
		"pg-product":          "9d4249cd4f0534ec8d554ee4cdc4e6f154c996dd62a8788c9ab5732e4715918c",
		"pg-initial":          "f77c8cb70b1a068daaa29ed1e40f9de52550e306a316762fb3eae83acfaa4621",
		"runtime-state":       "7fe2a4ab33abfe05952a9b553682533cb7d33ddb54246fe032d418be654b01d0",
		"failure-correlation": "38e697893d0e1cade7e1668e53818abd9f49bd3865532720b0a66dcec9cb21c5",
	}
}

func readRetainedFailedV15(root string) map[string]any {
	all := map[string]any{}
	for name, pin := range retainedFailedV15Pins() {
		all[name] = pinnedObject(root, "release-chain-v15-"+name+"-20261013.json", pin)
	}
	all["history"] = pinnedObject(root, "release-chain-v15-history616-20261013.json", v16HistorySHA)
	all["bridge"] = pinnedObject(root, "release-chain-v15-carry616-independent-20261013.json", v16BridgeSHA)
	all["prior"] = pinnedObject(root, "release-chain-v14-history611-20261013.json", v15HistorySHA)
	return all
}

func verifyRetainedFailedV15Source(root string) {
	verifyRetainedFailedV14Source(root)
	verifyRetainedFailedV15(readRetainedFailedV15(root))
}

func verifyRetainedFailedV15(all map[string]any) {
	h, prior, bridge := obj(all["history"]), obj(all["prior"]), obj(all["bridge"])
	usage, old := arr(h["usage"]), arr(prior["usage"])
	require(len(old) == 611 && len(usage) == 616 && integer(h["native_attempts"]) == 616 && h["status"] == prior["status"] && h["baseline_sha256"] == v16PriorSHA && reflect.DeepEqual(usage[:611], old), "v16_old_measurements_immutable")
	artifacts, oldArtifacts := arr(h["artifacts"]), arr(prior["artifacts"])
	require(len(artifacts) == len(oldArtifacts)+1 && reflect.DeepEqual(artifacts[:len(oldArtifacts)], oldArtifacts) && reflect.DeepEqual(obj(artifacts[len(oldArtifacts)]), frozenObject(`{"report_sha256":"dede808835fbb71b004b226221a6083b6518e745e20a59ebf670947c9e7eec10","release_ref":"5662b7398f8345872aedff2cec253787a3946abe","new_attempts":5}`)), "v16_old_artifacts_immutable")
	pins := retainedFailedV15Pins()
	vector := []any{v15PriorSHA, pins["final-audit"], pins["high"], pins["pg-ledger"], pins["pg-product"], pins["pg-initial"], pins["runtime-state"], pins["failure-correlation"], "5662b7398f8345872aedff2cec253787a3946abe", "276e6b17133aa8a68ca81a1eae5d2bcd3cff330371e69886e2fa9cc03f598cc1", "d8c9cf67c817072105f6e495921a8a4c3f8cb2bd27d807155febc92ba6a99e39"}
	require(bridge["fold_sha256"] == v16PriorSHA && hashValue(vector) == v16PriorSHA && reflect.DeepEqual(arr(bridge["fold_vector"]), vector) && integer(bridge["remaining_original_absolute731_attempts"]) == 115 && bridge["prospective_model_admission"] == false && bridge["old_originals_or_measurements_changed"] == false && bridge["original_metadata_preserved"] == true, "v16_closed731_no_refund")
	c, sc, ledger := obj(all["failure-correlation"]), obj(all["high"]), obj(all["pg-ledger"])
	carry := obj(c["carry_forward"])
	require(reflect.DeepEqual(carry, obj(bridge["carry_forward"])) && integer(carry["attempts"]) == 616 && integer(carry["known_total_tokens_subtotal"]) == 1578335 && integer(carry["unknown_total_attempts"]) == 50 && carry["old_unknown_266_cause"] == "UNKNOWN", "v16_historical_expense_honest")
	var input, output int64
	unknown := 0
	for _, value := range usage {
		u := obj(value)
		if u["actual_input_tokens"] == nil {
			require(u["actual_output_tokens"] == nil, "v16_old_cost_pair")
			unknown++
		} else {
			require(u["actual_output_tokens"] != nil, "v16_old_cost_pair")
			input += integer(u["actual_input_tokens"])
			output += integer(u["actual_output_tokens"])
		}
	}
	require(input == 1135037 && output == 26407 && unknown == 30 && integer(h["known_usage_pairs"]) == 586 && integer(h["unknown_usage_pairs"]) == 30, "v16_historical_pairs_recomputed")
	audit := obj(all["final-audit"])
	require(audit["version"] == "butler-independent-v15-failure-supplemental-v1" && audit["canonical_go_json_available"] == false && audit["canonical_go_not_available_reason"] == "V15-specific final auditor was not frozen at this completed run; no wrong-protocol or fabricated JSON is used", "v16_supplemental_not_canonical_go")
	oldAudit := obj(audit["chain_audit"])
	require(oldAudit["status"] == "NO_GO" && integer(oldAudit["carry_forward_attempts"]) == 616 && oldAudit["all_new_expense_measured"] == false && integer(oldAudit["accepted_model_retry_failures"]) == 0 && sc["status"] == "failed" && len(arr(sc["proposal_observations"])) == 3 && integer(sc["maximum_total_attempts"]) == 731 && reflect.DeepEqual(obj(sc["scenarios"]), object{"normal_sent": false}), "v16_old_no_go_inventory_preserved")
	require(c["status"] == "INDEPENDENT_ACTUAL_V15_CHAIN_NO_GO_CORRELATED" && c["native_scope_sealed"] == true && integer(c["native_request_count"]) == 5 && integer(c["accepted_model_retry_failures"]) == 0 && integer(c["actual_notice_count"]) == 0 && integer(c["actual_original_conversion_usage"]) == 0 && c["canonical_go_json_available"] == false && c["issue33_ready"] == false && reflect.DeepEqual(c["original_proposal_observations"], sc["proposal_observations"]) && c["original_proposal_observations_rewritten"] == false && c["all_three_actual_core_argument_fingerprints_match"] == true && len(arr(c["original_private_packets"])) == 3, "v16_old_no_go_not_promoted")
	f := obj(c["failure_classification"])
	require(integer(f["failed_ordinal"]) == 616 && f["failed_kind"] == "submit" && f["failed_role"] == "child" && f["diagnostic_code"] == "provider_input_bytes_exceeded" && f["provider_operation_stage"] == "request_serialization" && f["exact_serialized_input_bytes"] == nil && f["nominal_timeout_observed"] == false && f["safe_model_retry_observed"] == false, "v16_616_not_nominal_timeout_or_measured_size")
	for _, name := range []string{"specific_prior541_cause", "specific_prior544_child_cause", "specific_prior556_inspect_cause", "specific_prior557_source_cause"} {
		require(f[name] == "UNKNOWN", "v16_old_cause_unknown")
	}
	paid := obj(sc["scope"])
	window := obj(paid["window"])
	require(reflect.DeepEqual(window, obj(ledger["window"])) && integer(window["native_baseline"]) == 611 && integer(window["attempts"]) == 616 && window["prior_history_hash"] == v15PriorSHA && window["prerequisite_hash"] == v15PrerequisiteSHA, "v16_old_window_immutable")
	nativeRows, pgRows := arr(paid["requests"]), arr(ledger["requests"])
	require(len(nativeRows) == 5 && len(pgRows) == 5, "v16_old_native_inventory")
	var newInput, newOutput, newTotal int64
	for i, value := range nativeRows {
		n := obj(value)
		require(integer(n["ordinal"]) == int64(612+i), "v16_old_ordinal_inventory")
		u := obj(usage[611+i])
		require(u["run_id"] == n["run_id"] && u["reservation_id"] == fmt.Sprintf("release-chain-v15-native-%d", 612+i) && u["actual_input_tokens"] == n["actual_input_tokens"] && u["actual_output_tokens"] == n["actual_output_tokens"], "v16_old_pair_native_binding")
		if i < 4 {
			verifyRequestWithPolicy(n, obj(pgRows[i]), v15AuditPolicy())
			require(n["outcome"] == "completed" && n["provider_operation_stage"] == "response_stream" && n["model_version"] == "gemini-3.8-flash-n", "v16_old_completed_known")
			newInput += integer(n["actual_input_tokens"])
			newOutput += integer(n["actual_output_tokens"])
			newTotal += integer(n["actual_total_tokens"])
			if i == 3 {
				require(integer(n["elapsed_ms"]) == 207712 && integer(n["actual_input_tokens"]) == 4074 && integer(n["actual_output_tokens"]) == 165 && integer(n["actual_total_tokens"]) == 35693, "v16_615_completed_not_timeout")
			}
			continue
		}
		// Deliberately do not feed this historical serialization failure to the
		// nominal-timeout admission classifier or change NULL observations to zero.
		pg := obj(clone(pgRows[i]))
		for from, to := range map[string]string{"input_hash": "input_sha256", "admitted_at": "admitted_at_utc", "finished_at": "finished_at_utc", "input_tokens": "actual_input_tokens", "output_tokens": "actual_output_tokens", "total_tokens": "actual_total_tokens"} {
			pg[to] = pg[from]
			if from == "admitted_at" || from == "finished_at" {
				pg[to] = moment(pg[from]).UTC().Format(time.RFC3339Nano)
			}
			delete(pg, from)
		}
		require(reflect.DeepEqual(pg, n) && n["outcome"] == "transport_error" && n["provider_operation_stage"] == "request_serialization" && n["diagnostic_code"] == "provider_input_bytes_exceeded" && n["failure_stage"] == "stream" && n["actual_input_tokens"] == nil && n["actual_output_tokens"] == nil && n["actual_total_tokens"] == nil && n["model_version"] == nil && integer(n["final_count"]) == 0 && integer(n["transport_error_count"]) == 1, "v16_616_original_null_no_retry")
	}
	require(newInput == 31686 && newOutput == 830 && newTotal == 66271, "v16_retained_four_known_expense_and_one_unknown")
	product := witnesses(all["pg-product"])
	require(len(product) == 1 && product["normal_sent"] != nil, "v16_old_product_inventory")
	fin := obj(product["normal_sent"]["payload_json"])
	db := obj(fin["postgres"])
	require(fin["test_failed"] == true && integer(fin["all_channel_requests"]) == 0 && fin["original_result"] == nil && len(rows(db, "notices")) == 0 && len(rows(db, "converter_usages")) == 0 && len(rows(db, "actions")) == 3, "v16_old_no_original_success")
	var root, child string
	for _, t := range rows(db, "tasks") {
		if t["parent_task_id"] == nil {
			root = text(t["task_id"])
			require(t["status"] == "active", "v16_old_root_unverified")
		} else {
			child = text(t["task_id"])
			require(t["status"] == "blocked", "v16_old_child_block_retained")
		}
	}
	require(root != "" && child != "", "v16_old_tasks")
	verifyBudgetWithPolicy(db, rows(paid, "requests"), "normal_sent", root, child, v15AuditPolicy())
	verifyV11SourceObservation(fin["source_check_observation"], db)
	for _, input := range rows(db, "accepted_inputs") {
		require(input["model_retry"] == nil, "v16_616_no_retry_invented")
	}
}
