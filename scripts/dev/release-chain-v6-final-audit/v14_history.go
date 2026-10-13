package main

import (
	"fmt"
	"reflect"
)

const v14HistorySHA = "b93399340f64d881c99180b3ea6b62885341ba25f6de70edb64a34a0b51789f0"
const v14BridgeSHA = "61a4bbda07cfa28e4d92caf2557d5447a96ed47abf54fbc71c13c849634e1398"

func retainedFailedV13Pins() map[string]string {
	return map[string]string{
		"final-audit":         "ab3da7aa5cb254401ca12eb0ed26e69819bf0d0d0da84e94d51c8fd31dee9b93",
		"high":                "97be7689270c2fbefe642287ba0f2bc9770a2dd27931549f7dde1bbfc0b8a214",
		"pg-ledger":           "48fcc81af4be04014a03a829a6a574468ed73b4cb4d6a908ec1a70999fd99f62",
		"pg-product":          "36a84b49ac9fb9753cb628f597f690f7bcd41c9f401dc086d33217451f561544",
		"pg-initial":          "c0816cddc6ae1df53d94cb1bb0ab766a50d96e2a7bd445d1bfc2383e0f02162d",
		"runtime-state":       "6d6934f7c56c2c2551e9a8a50be7910c09bf6e6e97db93838dbb172328484c93",
		"failure-correlation": "e23955bcf779f1c062529c0531ca6984db4119dd529009f05a2ade4a7528aca7",
	}
}

func readRetainedFailedV13(root string) map[string]any {
	all := map[string]any{}
	for name, pin := range retainedFailedV13Pins() {
		all[name] = pinnedObject(root, "release-chain-v13-"+name+"-20261013.json", pin)
	}
	all["history"] = pinnedObject(root, "release-chain-v13-history593-20261013.json", v14HistorySHA)
	all["bridge"] = pinnedObject(root, "release-chain-v13-carry593-independent-20261013.json", v14BridgeSHA)
	all["prior"] = pinnedObject(root, "release-chain-v11-history575-20261013.json", v12HistorySHA)
	return all
}

func verifyRetainedFailedV13Source(root string) {
	verifyRetainedFailedV11Source(root)
	verifyV13BootstrapSource(root)
	verifyRetainedFailedV13(readRetainedFailedV13(root))
}

// The prior checker emitted no JSON. Its separate independent supplemental
// record is preserved as such; missing private inventory is never refilled.
func verifyRetainedFailedV13(all map[string]any) {
	h, prior, b := obj(all["history"]), obj(all["prior"]), obj(all["bridge"])
	usage, old := arr(h["usage"]), arr(prior["usage"])
	require(len(old) == 575 && len(usage) == 593 && integer(h["native_attempts"]) == 593 && h["status"] == prior["status"] && h["baseline_sha256"] == v14PriorSHA && reflect.DeepEqual(usage[:575], old), "v14_old_measurements_immutable")
	artifacts, oldArtifacts := arr(h["artifacts"]), arr(prior["artifacts"])
	require(len(artifacts) == len(oldArtifacts)+1 && reflect.DeepEqual(artifacts[:len(oldArtifacts)], oldArtifacts) && reflect.DeepEqual(obj(artifacts[len(oldArtifacts)]), frozenObject(`{"report_sha256":"ab3da7aa5cb254401ca12eb0ed26e69819bf0d0d0da84e94d51c8fd31dee9b93","release_ref":"09cf34c05ab1fc1840dc73f1c9e1ecb2766b0faf","new_attempts":18}`)), "v14_old_artifacts_immutable")
	pins := retainedFailedV13Pins()
	vector := []any{v12PriorSHA, pins["final-audit"], pins["high"], pins["pg-ledger"], pins["pg-product"], pins["pg-initial"], pins["runtime-state"], pins["failure-correlation"], "09cf34c05ab1fc1840dc73f1c9e1ecb2766b0faf", "e203276bb12e30c16c975952f7e70c92295adba80e76c85dc08d9ca43ade6e70", "c290b25cacdeaf35fb7b89787fe18630689a29c13acbf37ed24bd6b50e005d26"}
	require(b["fold_sha256"] == v14PriorSHA && hashValue(vector) == v14PriorSHA && reflect.DeepEqual(arr(b["fold_vector"]), vector) && integer(b["remaining_original_absolute695_attempts"]) == 102 && b["prospective_model_admission"] == false && b["old_originals_or_measurements_changed"] == false && b["original_scope_proposal_observations"] == nil, "v14_closed695_no_refund")
	c, sc, ledger := obj(all["failure-correlation"]), obj(all["high"]), obj(all["pg-ledger"])
	carry := obj(c["carry_forward"])
	require(reflect.DeepEqual(carry, obj(b["carry_forward"])) && integer(carry["attempts"]) == 593 && integer(carry["known_total_tokens_subtotal"]) == 1296671 && integer(carry["unknown_total_attempts"]) == 49 && carry["old_unknown_266_cause"] == "UNKNOWN", "v14_historical_expense_honest")
	var input, output int64
	unknown := 0
	for _, v := range usage {
		u := obj(v)
		if u["actual_input_tokens"] == nil {
			require(u["actual_output_tokens"] == nil, "v14_old_cost_pair")
			unknown++
		} else {
			require(u["actual_output_tokens"] != nil, "v14_old_cost_pair")
			input += integer(u["actual_input_tokens"])
			output += integer(u["actual_output_tokens"])
		}
	}
	require(input == 910272 && output == 22021 && unknown == 29 && integer(h["known_usage_pairs"]) == 564 && integer(h["unknown_usage_pairs"]) == 29, "v14_historical_pairs_recomputed")
	audit := obj(all["final-audit"])
	require(audit["version"] == "butler-independent-v13-failure-supplemental-v1" && audit["canonical_go_json_available"] == false && integer(audit["canonical_go_exit"]) == 1 && integer(audit["canonical_go_stdout_bytes"]) == 0 && audit["canonical_go_stdout_sha256"] == digest(nil) && audit["canonical_go_stderr"] == "typed_operator_contract_denied", "v14_supplemental_not_canonical_go")
	oldAudit := obj(audit["chain_audit"])
	require(oldAudit["status"] == "NO_GO" && integer(oldAudit["carry_forward_attempts"]) == 593 && oldAudit["all_new_expense_measured"] == true && integer(oldAudit["accepted_model_retry_failures"]) == 0 && sc["status"] == "failed" && sc["proposal_observations"] == nil && integer(sc["maximum_total_attempts"]) == 695 && len(obj(sc["scenarios"])) == 0, "v14_old_no_go_null_inventory")
	require(c["status"] == "INDEPENDENT_ACTUAL_V13_CHAIN_NO_GO_CORRELATED" && c["native_scope_sealed"] == true && integer(c["native_request_count"]) == 18 && integer(c["accepted_model_retry_failures"]) == 0 && integer(c["actual_notice_count"]) == 0 && integer(c["actual_original_conversion_usage"]) == 0 && c["canonical_go_json_available"] == false && c["issue33_ready"] == false && c["original_proposal_observations"] == nil && c["original_proposal_observations_rewritten"] == false, "v14_old_no_go_not_promoted")
	f := obj(c["failure_classification"])
	require(f["denied_actual_tool"] == "converters.activate" && f["denied_digest_is_already_active"] == true && f["denied_generation_matches_current_catalog"] == false && f["denied_action_or_effect_created"] == false && integer(f["source_pinned_metadata_reader_maximum"]) == 80 && f["metadata_reader_denies_before_glob"] == true && f["model_reason_for_rechecking_or_reactivation"] == "UNKNOWN", "v14_old_denial_not_rewritten")
	for _, name := range []string{"specific_prior541_cause", "specific_prior544_child_cause", "specific_prior556_inspect_cause", "specific_prior557_source_cause"} {
		require(f[name] == "UNKNOWN", "v14_old_cause_unknown")
	}
	paid := obj(sc["scope"])
	window := obj(paid["window"])
	require(reflect.DeepEqual(window, obj(ledger["window"])) && integer(window["native_baseline"]) == 575 && integer(window["attempts"]) == 593 && window["prior_history_hash"] == v12PriorSHA && window["prerequisite_hash"] == v13PrerequisiteSHA, "v14_old_window_immutable")
	nativeRows, pgRows := arr(paid["requests"]), arr(ledger["requests"])
	require(len(nativeRows) == 18 && len(pgRows) == 18, "v14_old_native_inventory")
	var newInput, newOutput, newTotal int64
	for i, v := range nativeRows {
		n := obj(v)
		verifyRequestWithPolicy(n, obj(pgRows[i]), v13AuditPolicy())
		require(integer(n["ordinal"]) == int64(576+i) && n["outcome"] == "completed" && n["provider_operation_stage"] == "response_stream" && n["model_version"] == "gemini-3.8-flash-n", "v14_old_native_exact_measurements")
		u := obj(usage[575+i])
		require(u["run_id"] == n["run_id"] && u["reservation_id"] == fmt.Sprintf("release-chain-v13-native-%d", 576+i) && u["actual_input_tokens"] == n["actual_input_tokens"] && u["actual_output_tokens"] == n["actual_output_tokens"], "v14_old_pair_native_binding")
		newInput += integer(n["actual_input_tokens"])
		newOutput += integer(n["actual_output_tokens"])
		newTotal += integer(n["actual_total_tokens"])
	}
	require(newInput == 221156 && newOutput == 3231 && newTotal == 256750, "v14_retained_all18_expense")
	// The exact exported leaf is an array containing one qualified witness.
	product := witnesses(all["pg-product"])
	require(len(product) == 1 && product["normal_sent"] != nil, "v14_old_product_inventory")
	fin := obj(product["normal_sent"]["payload_json"])
	db := obj(fin["postgres"])
	require(fin["test_failed"] == true && integer(fin["all_channel_requests"]) == 0 && len(rows(db, "notices")) == 0 && len(rows(db, "converter_usages")) == 0 && len(rows(db, "actions")) == 14, "v14_old_original_no_invented_success")
	var root, child string
	for _, t := range rows(db, "tasks") {
		if t["parent_task_id"] == nil {
			root = text(t["task_id"])
			require(t["status"] == "blocked", "v14_old_block_preserved")
		} else {
			child = text(t["task_id"])
		}
	}
	require(root != "" && child != "", "v14_old_tasks")
	verifyBudgetWithPolicy(db, rows(paid, "requests"), "normal_sent", root, child, v13AuditPolicy())
	verifyV11SourceObservation(fin["source_check_observation"], db)
}
