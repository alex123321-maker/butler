package main

import (
	"fmt"
	"reflect"
	"time"
)

const v12HistorySHA = "4f62e26456070b298ee411f2d7681fbf6dda0d6662cf869a4cbcba8674303220"
const v12BridgeSHA = "043ef974962836ab2c69691dccdc1d1add15c379632599b1889057cd55d3fc00"
const v12OldProposalSHA = "be70e93a61d10873bab71a934a54ce763ee1d25f52b8c951adb6a70585ee8c2e"

func retainedFailedV11Pins() map[string]string {
	return map[string]string{
		"final-audit":         "85a32e433345e53db47b0f314feb55c5d23baadff0e18be0756084e77f9f54ae",
		"high":                "ae890d32296fec8536fbaa9c6c0b64156aa49727c0b13beab10708991f107261",
		"pg-ledger":           "54e17978b1fdf383f987a4f6de61e28a3d14aa5029ffac574e4b116ea7f5b4fd",
		"pg-product":          "b4e09779586ae04c2c97e732667f34d481bf65cee17887632c3cbbe77735bd9f",
		"pg-initial":          "0d834cdc4c0190a54cea2fd5ae1a4d87e969f48e9c3428ea6986c0d84415a297",
		"runtime-state":       "a436b9b26c8117f374c3c2e0cad6e2cfa7e17ab86fc7ab7ee942e28ecb7c5641",
		"failure-correlation": "638cebaedc3b4b4ceed5745c70053e5c1aaa0fe4d245681c97ce990781ec14be",
	}
}
func readRetainedFailedV11(root string) map[string]any {
	all := map[string]any{}
	for name, pin := range retainedFailedV11Pins() {
		all[name] = pinnedObject(root, "release-chain-v11-"+name+"-20261013.json", pin)
	}
	all["history"] = pinnedObject(root, "release-chain-v11-history575-20261013.json", v12HistorySHA)
	all["bridge"] = pinnedObject(root, "release-chain-v11-carry575-independent-20261013.json", v12BridgeSHA)
	all["proposals"] = pinnedObject(root, "release-chain-v11-proposal-diagnostics-20261013.json", v12OldProposalSHA)
	all["prior"] = pinnedObject(root, "release-chain-v10-history557-20261013.json", v11HistorySHA)
	return all
}
func verifyRetainedFailedV11Source(root string) {
	verifyRetainedFailedV10Source(root)
	verifyRetainedFailedV11(readRetainedFailedV11(root))
}
func verifyRetainedFailedV11(all map[string]any) {
	h, prior, b := obj(all["history"]), obj(all["prior"]), obj(all["bridge"])
	usage, old := arr(h["usage"]), arr(prior["usage"])
	require(len(old) == 557 && len(usage) == 575 && integer(h["native_attempts"]) == 575 && h["status"] == prior["status"] && h["baseline_sha256"] == v12PriorSHA && reflect.DeepEqual(usage[:557], old), "v12_old_measurements_immutable")
	artifacts, oldArtifacts := arr(h["artifacts"]), arr(prior["artifacts"])
	require(len(artifacts) == len(oldArtifacts)+1 && reflect.DeepEqual(artifacts[:len(oldArtifacts)], oldArtifacts) && reflect.DeepEqual(obj(artifacts[len(oldArtifacts)]), frozenObject(`{"report_sha256":"85a32e433345e53db47b0f314feb55c5d23baadff0e18be0756084e77f9f54ae","release_ref":"323da2db21b37e2efd39639eaff98f838a33f0c6","new_attempts":18}`)), "v12_old_artifacts_immutable")
	pins := retainedFailedV11Pins()
	vector := []any{v11PriorSHA, pins["final-audit"], pins["high"], pins["pg-ledger"], pins["pg-product"], pins["pg-initial"], pins["runtime-state"], pins["failure-correlation"], "323da2db21b37e2efd39639eaff98f838a33f0c6", "f7158f46360ed52794188063a1c7860c90d1cbb4ffc9d1c8b65f4fdbe9cc5849", "e1e20c8d64b868f7a1752a10edee3f8f8dc10a4ceeea787a51fc5b48735fb25d"}
	require(b["fold_sha256"] == v12PriorSHA && hashValue(vector) == v12PriorSHA && reflect.DeepEqual(arr(b["fold_vector"]), vector) && integer(b["remaining_original_absolute628_attempts"]) == 53 && b["prospective_model_admission"] == false && b["old_originals_or_measurements_changed"] == false, "v12_closed628_no_refund")
	c, sc, ledger := obj(all["failure-correlation"]), obj(all["high"]), obj(all["pg-ledger"])
	carry := obj(c["carry_forward"])
	require(reflect.DeepEqual(carry, obj(b["carry_forward"])) && integer(carry["attempts"]) == 575 && integer(carry["known_total_tokens_subtotal"]) == 1039921 && integer(carry["unknown_total_attempts"]) == 49 && carry["old_unknown_266_cause"] == "UNKNOWN", "v12_historical_expense_honest")
	var input, output int64
	unknown := 0
	for _, v := range usage {
		u := obj(v)
		if u["actual_input_tokens"] == nil {
			require(u["actual_output_tokens"] == nil, "v12_old_cost_pair")
			unknown++
		} else {
			require(u["actual_output_tokens"] != nil, "v12_old_cost_pair")
			input += integer(u["actual_input_tokens"])
			output += integer(u["actual_output_tokens"])
		}
	}
	require(input == 689116 && output == 18790 && unknown == 29 && integer(h["known_usage_pairs"]) == 546 && integer(h["unknown_usage_pairs"]) == 29, "v12_historical_pairs_recomputed")
	require(c["status"] == "INDEPENDENT_ACTUAL_V11_CHAIN_NO_GO_CORRELATED" && c["native_scope_sealed"] == true && integer(c["native_request_count"]) == 18 && integer(c["accepted_model_retry_failures"]) == 0 && c["actual_model_retry"] == nil && integer(c["actual_notice_count"]) == 0 && c["issue33_ready"] == false, "v12_old_no_go_not_promoted")
	oldAudit := obj(obj(all["final-audit"])["chain_audit"])
	require(oldAudit["status"] == "NO_GO" && integer(oldAudit["carry_forward_attempts"]) == 575 && oldAudit["all_new_expense_measured"] == true && integer(oldAudit["accepted_model_retry_failures"]) == 0 && sc["status"] == "failed" && integer(sc["maximum_total_attempts"]) == 628 && len(obj(sc["scenarios"])) == 0, "v12_old_no_go_immutable")
	classification := obj(c["failure_classification"])
	require(integer(classification["fixed_scripted_journal_expected_actions"]) == 13 && integer(classification["actual_confirmed_actions"]) == 14 && classification["actual_per_read_result_checks_passed"] == nil && classification["actual_per_read_result_applied"] == nil && classification["per_read_actual_receipt_fields_unknown_not_inferred_from_request"] == true && classification["observer_cancellations_after_completed_runs_not_source_failures"] == true, "v12_old_read_receipt_nulls")
	for _, name := range []string{"specific_prior541_cause", "specific_prior544_child_cause", "specific_prior556_inspect_cause", "specific_prior557_source_cause"} {
		require(classification[name] == "UNKNOWN", "v12_old_cause_unknown")
	}
	paid := obj(sc["scope"])
	window := obj(paid["window"])
	require(reflect.DeepEqual(window, obj(ledger["window"])) && integer(window["native_baseline"]) == 557 && integer(window["attempts"]) == 575 && window["prior_history_hash"] == v11PriorSHA && window["prerequisite_hash"] == v11PrerequisiteSHA, "v12_old_window_immutable")
	nativeRows, pgRows := arr(paid["requests"]), arr(ledger["requests"])
	require(len(nativeRows) == 18 && len(pgRows) == 18, "v12_old_native_inventory")
	var newInput, newOutput, newTotal int64
	for i, v := range nativeRows {
		n := obj(v)
		pg := obj(clone(pgRows[i]))
		for from, to := range map[string]string{"input_hash": "input_sha256", "admitted_at": "admitted_at_utc", "finished_at": "finished_at_utc", "input_tokens": "actual_input_tokens", "output_tokens": "actual_output_tokens", "total_tokens": "actual_total_tokens"} {
			pg[to] = pg[from]
			if from == "admitted_at" || from == "finished_at" {
				pg[to] = moment(pg[from]).UTC().Format(time.RFC3339Nano)
			}
			delete(pg, from)
		}
		require(reflect.DeepEqual(pg, n) && integer(n["ordinal"]) == int64(558+i) && n["outcome"] == "completed" && n["provider_operation_stage"] == "response_stream" && n["model_version"] == "gemini-3.8-flash-n" && n["failure_stage"] == nil && n["diagnostic_code"] == nil && n["usage_observation_uncertain"] == false, "v12_old_native_exact_measurements")
		u := obj(usage[557+i])
		require(u["run_id"] == n["run_id"] && u["reservation_id"] == fmt.Sprintf("release-chain-v11-native-%d", 558+i) && u["actual_input_tokens"] == n["actual_input_tokens"] && u["actual_output_tokens"] == n["actual_output_tokens"], "v12_old_pair_native_binding")
		newInput += integer(n["actual_input_tokens"])
		newOutput += integer(n["actual_output_tokens"])
		newTotal += integer(n["actual_total_tokens"])
	}
	require(newInput == 213479 && newOutput == 3631 && newTotal == 246218, "v12_retained_all18_expense")
	product := witnesses(all["pg-product"])
	require(len(product) == 1 && product["normal_sent"] != nil, "v12_old_product_inventory")
	fin := obj(product["normal_sent"]["payload_json"])
	db := obj(fin["postgres"])
	original := obj(fin["original_result"])
	require(fin["test_failed"] == true && integer(fin["all_channel_requests"]) == 0 && integer(original["check_receipt_count"]) == 2 && original["check_tool_call_id"] == nil && original["check_receipt_artifact_id"] == nil && original["checks_passed"] == nil && original["applied"] == nil && len(rows(db, "actions")) == 14 && len(rows(db, "notices")) == 0, "v12_old_original_no_invented_success")
	var root, child string
	for _, t := range rows(db, "tasks") {
		require(t["status"] == "active", "v12_old_unverified_task")
		if t["parent_task_id"] == nil {
			root = text(t["task_id"])
		} else {
			child = text(t["task_id"])
		}
	}
	require(root != "" && child != "", "v12_old_tasks")
	verifyBudgetWithPolicy(db, rows(paid, "requests"), "normal_sent", root, child, v11AuditPolicy())
	verifyV11SourceObservation(fin["source_check_observation"], db)
	diag := obj(all["proposals"])
	require(integer(diag["confirmed_actions"]) == 14 && diag["all_fourteen_core_action_fingerprints_match"] == true && diag["actual_both_read_receipt_booleans_retained"] == false && diag["legacy_original_result_nulls_preserved"] == true && diag["duplicate_writes_or_mutation_replays_observed"] == false && len(arr(diag["proposals"])) == 14, "v12_old_original_proposals")
}
