package main

import (
	"reflect"
	"strconv"
	"time"
)

const v10HistorySHA = "8509f520a5b013900c5365f06868656448afd74a3f24d2f88971f013024055f5"
const v10BridgeSHA = "70041e9a3275a2099d39eda98d4535420eea7bfdae1bb3eb34bd58662f51ae3b"

func retainedFailedV9Pins() map[string]string {
	return map[string]string{
		"final-audit":          "731cf102a25d42a091441ea045cedb5964c66ebb305890c564cc3b5b9713ad83",
		"high":                 "332ed92d91d264988375eebf884c408781f19bd8eb3f18649dea48b8bc67afe1",
		"pg-ledger":            "66fa92d8254fca914608afdfc03c2541772dd73d504cb60b820b783e64fd8998",
		"pg-product":           "c7515abd5c1b33a72e38dcb1bab48b866820b732b78e214decd438913023f642",
		"pg-initial":           "f809e9f8c76dd54d2a35535bcfb249687726d6dd4835c5a82a55d3eba0a3f188",
		"runtime-state":        "9f227ea935aceb2810bd296cfad72fb5787b70ba09d19d9419a4c83a4b695dde",
		"failure-correlation":  "b96457cbb16bfbc89cb3fb32c62a3d9622939efb9fbe574eb97253c4f89f9b6c",
		"proposal-diagnostics": "8b5c8683e1aa23e49c15809e5d12432df3402ad01365a2e5a1567ed6f55350fb",
	}
}

func readRetainedFailedV9(root string) map[string]any {
	all := map[string]any{}
	for name, pin := range retainedFailedV9Pins() {
		// pinnedObject also accepts the actual product/initial JSON arrays.
		all[name] = pinnedObject(root, "release-chain-v9-"+name+"-20261013.json", pin)
	}
	all["history"] = pinnedObject(root, "release-chain-v9-history556-20261013.json", v10HistorySHA)
	all["bridge"] = pinnedObject(root, "release-chain-v9-carry556-independent-20261013.json", v10BridgeSHA)
	all["prior"] = pinnedObject(root, "release-chain-v8-history544-20261013.json", "bf6ac2a9e0115a3a98036605b310c9cbd6637db99dd1c7f53870a75bf1900142")
	return all
}

func verifyRetainedFailedV9Source(root string) {
	verifyRetainedFailedV8Source(root)
	verifyRetainedFailedV9(readRetainedFailedV9(root))
}

// Exact byte pins bind the complete old exports; these recomputations expose the
// no-refund, expense, UNKNOWN and original-action boundaries independently.
func verifyRetainedFailedV9(all map[string]any) {
	h, prior, b := obj(all["history"]), obj(all["prior"]), obj(all["bridge"])
	usage, old := arr(h["usage"]), arr(prior["usage"])
	require(integer(h["native_attempts"]) == 556 && len(usage) == 556 && len(old) == 544 && h["status"] == prior["status"] && integer(h["known_usage_pairs"]) == 528 && integer(h["unknown_usage_pairs"]) == 28 && h["baseline_sha256"] == v10PriorSHA, "v10_actual556_history")
	require(reflect.DeepEqual(usage[:544], old), "v10_old_measurements_immutable")
	artifacts, oldArtifacts := arr(h["artifacts"]), arr(prior["artifacts"])
	require(len(artifacts) == len(oldArtifacts)+1 && reflect.DeepEqual(artifacts[:len(oldArtifacts)], oldArtifacts), "v10_old_artifacts_immutable")
	pins := retainedFailedV9Pins()
	const measuredCode = "1937bdc8f152469d918ea83092a8d75a614f07c5"
	const measuredTree = "fcfa5d238138cb1b49e2fc6462ec7c9694b5ecb0328619a25247548cd0a3c852"
	const measuredApp = "6a208f2d1d0a041b1c6fcdc40b390380daebc4a24865f02cd2afca48f07b9309"
	require(reflect.DeepEqual(obj(artifacts[len(oldArtifacts)]), frozenObject(`{"report_sha256":"731cf102a25d42a091441ea045cedb5964c66ebb305890c564cc3b5b9713ad83","release_ref":"1937bdc8f152469d918ea83092a8d75a614f07c5","new_attempts":12}`)), "v10_closed_artifact_no_refund")
	vector := []any{v9PriorSHA, pins["final-audit"], pins["high"], pins["pg-ledger"], pins["pg-product"], pins["pg-initial"], pins["runtime-state"], pins["failure-correlation"], measuredCode, measuredTree, measuredApp}
	require(b["status"] == "INDEPENDENT_SEALED556_CARRY_BRIDGE" && b["fold_sha256"] == v10PriorSHA && hashValue(vector) == v10PriorSHA && reflect.DeepEqual(arr(b["fold_vector"]), vector) && b["prior_fold_sha256"] == v9PriorSHA && integer(b["remaining_original_absolute615_attempts"]) == 59 && b["prospective_model_admission"] == false && b["old_originals_or_measurements_changed"] == false && b["all_new_expense_measured"] == true, "v10_closed615_fold_no_refund")
	c, scope, pg := obj(all["failure-correlation"]), obj(all["high"]), obj(all["pg-ledger"])
	require(c["status"] == "INDEPENDENT_ACTUAL_V9_CHAIN_NO_GO_CORRELATED" && c["code_commit"] == measuredCode && c["source_sha256"] == measuredTree && c["native_scope_sealed"] == true && integer(c["native_request_count"]) == 12 && integer(c["extra_unobserved_kernel_model_quotes"]) == 0 && c["actual_original_result"] == nil && integer(c["actual_notice_count"]) == 0 && c["issue33_ready"] == false && len(arr(c["verified_scenarios"])) == 0, "v10_closed_v9_no_go")
	require(obj(obj(all["final-audit"])["chain_audit"])["status"] == "NO_GO" && scope["status"] == "failed" && integer(scope["baseline_attempts"]) == 544 && integer(scope["maximum_total_attempts"]) == 615 && integer(scope["maximum_new_attempts"]) == 71 && len(obj(scope["scenarios"])) == 0, "v10_previous_failure_not_promoted")
	f := obj(c["failure_classification"])
	require(f["exact_ordinal556_failure_subcause"] == "UNKNOWN_FROM_CURRENT_SAFE_EXPORTS" && f["specific_prior541_cause"] == "UNKNOWN_FROM_CURRENT_SAFE_EXPORTS" && f["specific_prior544_child_cause"] == "UNKNOWN_FROM_CURRENT_SAFE_EXPORTS" && f["ordinal556_effect_state"] == "effect_unknown" && f["ordinal556_dispatch_outcome"] == "unknown", "v10_old_specific_causes_unknown")
	var input, output int64
	unknown := 0
	for _, value := range usage {
		u := obj(value)
		if u["actual_input_tokens"] == nil {
			require(u["actual_output_tokens"] == nil, "v10_old_cost_pair")
			unknown++
		} else {
			require(u["actual_output_tokens"] != nil, "v10_old_cost_pair")
			input += integer(u["actual_input_tokens"])
			output += integer(u["actual_output_tokens"])
		}
	}
	carry := obj(c["carry_forward"])
	require(input == 475637 && output == 15159 && unknown == 28 && integer(carry["known_total_tokens_subtotal"]) == 793703 && integer(carry["unknown_total_attempts"]) == 48 && carry["old_unknown_266_cause"] == "UNKNOWN" && reflect.DeepEqual(carry, obj(b["carry_forward"])), "v10_historical_cost_honest")
	paid := obj(scope["scope"])
	window := obj(paid["window"])
	require(reflect.DeepEqual(window, obj(pg["window"])) && integer(window["native_baseline"]) == 544 && integer(window["attempts"]) == 556 && window["prior_history_hash"] == v9PriorSHA && window["prerequisite_hash"] == v9PrerequisiteSHA && reflect.DeepEqual(obj(window["protocol_policy"]), v9AuditPolicy().WindowPolicy), "v10_closed_window_immutable")
	native, sql := arr(paid["requests"]), arr(pg["requests"])
	require(len(native) == 12 && len(sql) == 12, "v10_closed_request_inventory")
	var finished time.Time
	var newInput, newOutput, total int64
	for i, value := range native {
		r := obj(value)
		verifyRequestWithPolicy(r, obj(sql[i]), v9AuditPolicy())
		require(integer(r["ordinal"]) == int64(545+i) && r["outcome"] == "completed" && !moment(r["admitted_at_utc"]).Before(finished), "v10_closed_requests_serialized")
		finished = moment(r["finished_at_utc"])
		u := obj(usage[544+i])
		require(u["run_id"] == r["run_id"] && u["reservation_id"] == "release-chain-v9-native-"+strconv.Itoa(545+i) && u["actual_input_tokens"] == r["actual_input_tokens"] && u["actual_output_tokens"] == r["actual_output_tokens"], "v10_paid_history_join")
		newInput += integer(r["actual_input_tokens"])
		newOutput += integer(r["actual_output_tokens"])
		total += integer(r["actual_total_tokens"])
	}
	require(newInput == 126989 && newOutput == 2021 && total == 146325, "v10_all_twelve_paid_costs_retained")
	product, initial := witnesses(all["pg-product"]), witnesses(all["pg-initial"])
	require(len(product) == 1 && len(initial) == 1 && product["normal_sent"] != nil && initial["normal_sent"] != nil, "v10_closed_witness_inventory")
	fin := obj(product["normal_sent"]["payload_json"])
	require(fin["test_failed"] == true && fin["original_result"] == nil && integer(fin["all_channel_requests"]) == 0, "v10_no_invented_original_result")
	d := obj(all["proposal-diagnostics"])
	require(d["status"] == "INDEPENDENT_TEN_ORIGINAL_PROPOSALS_CORRELATED_CHAIN_NO_GO" && d["all_ten_core_action_fingerprints_match"] == true && integer(d["confirmed_actions"]) == 9 && integer(d["unknown_actions"]) == 1 && d["all_model_argument_values_remain_private"] == true && d["whole_private_directory_read_or_mounted"] == false, "v10_closed_proposal_boundary")
	ordinals := []int64{545, 546, 548, 549, 550, 552, 553, 554, 555, 556}
	proposals := arr(d["proposals"])
	require(len(proposals) == len(ordinals), "v10_closed_proposal_inventory")
	for i, value := range proposals {
		p := obj(value)
		state := "effect_confirmed"
		if i == 9 {
			state = "effect_unknown"
		}
		require(integer(p["ordinal"]) == ordinals[i] && p["original_bytes_and_base64_match"] == true && p["original_authorization_hash_matches_metadata"] == true && p["actual_core_authorized_action_match"] == true && p["actual_action_state"] == state && p["model_argument_values_emitted"] == false, "v10_closed_original_proposals")
	}
	shape := obj(d["ordinal556_inspect_shape"])
	require(shape["required_fields_exact"] == true && shape["url_equals_fixed_project_fixture_target"] == true && shape["authorization_absent"] == true && shape["body_absent"] == true && shape["exact_failure_subcause"] == "UNKNOWN_FROM_CURRENT_SAFE_EXPORTS" && shape["action_state"] == "effect_unknown", "v10_556_unknown_preserved")
}
