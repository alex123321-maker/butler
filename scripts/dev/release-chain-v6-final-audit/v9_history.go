package main

import "reflect"

// These are byte pins of independently retained closed evidence. They grant no
// model admission, replace no old measurement, and require no old private mount.
func verifyRetainedFailedV8Source(root string) {
	verifyRetainedFailedV7bSource(root)
	pins := map[string]string{
		"final-audit":          "5573b9f25e13a4f3aefacacf142aeda7846b68962f8cd26df4498417097c580f",
		"high":                 "d319637dcd6a74e80c7cdb18749e9c35501bd35c590daa7d72b934543a04f9e8",
		"pg-ledger":            "504884085e50a77f45c8057c062b195deb29a756f5d868d6dbb9ed235b07c333",
		"pg-product":           "534411ccfcefe90b40ea22a890335602c909f66ff64b85197231ae6d09e504b9",
		"pg-initial":           "6e1b70738d86f7f5eed465e9bf5b46ff4bdf862f036fdb06cc90470e736c545a",
		"runtime-state":        "f705bd247f2d3b25f855694186a8ce9ca214e360784f527fbeb1facf730f94c7",
		"failure-correlation":  "2e1eed8eadb5874299693bd28ea9857fbd6d96ce9a2e1865009f2368403761d3",
		"proposal-diagnostics": "8d5b296bfacd45da1d578896435378e7151787ee4847f26d6953267da3af87d8",
	}
	all := map[string]any{}
	for name, pin := range pins {
		all[name] = pinnedObject(root, "release-chain-v8-"+name+"-20261013.json", pin)
	}
	h := obj(pinnedObject(root, "release-chain-v8-history544-20261013.json", "bf6ac2a9e0115a3a98036605b310c9cbd6637db99dd1c7f53870a75bf1900142"))
	b := obj(pinnedObject(root, "release-chain-v8-carry544-independent-20261013.json", "ca18f5d5f3a78ac9d61ada16472399ecc3024801b8b077795fabbe3db9584650"))
	prior := obj(pinnedObject(root, "release-chain-v7b-history541-20261013.json", "ebbc1a11df409a22c58e2b602fb60f5614c4ee7d71e0c0b9e13f9c055113b079"))
	usage, old := arr(h["usage"]), arr(prior["usage"])
	require(integer(h["native_attempts"]) == 544 && integer(h["known_usage_pairs"]) == 516 && integer(h["unknown_usage_pairs"]) == 28 && h["baseline_sha256"] == v9PriorSHA && h["status"] == prior["status"], "v9_actual544_history")
	require(len(usage) == 544 && len(old) == 541 && reflect.DeepEqual(usage[:541], old), "v9_old_measurements_immutable")
	oldArtifacts, artifacts := arr(prior["artifacts"]), arr(h["artifacts"])
	require(len(artifacts) == len(oldArtifacts)+1 && reflect.DeepEqual(artifacts[:len(oldArtifacts)], oldArtifacts), "v9_old_artifacts_immutable")
	require(reflect.DeepEqual(obj(artifacts[len(oldArtifacts)]), frozenObject(`{"report_sha256":"5573b9f25e13a4f3aefacacf142aeda7846b68962f8cd26df4498417097c580f","release_ref":"5057fbf1a90e01952ea939c2c1fd7af13c3db6a6","new_attempts":3}`)), "v9_actual_closed_artifact")
	vector := []any{v8PriorSHA, pins["final-audit"], pins["high"], pins["pg-ledger"], pins["pg-product"], pins["pg-initial"], pins["runtime-state"], pins["failure-correlation"], "5057fbf1a90e01952ea939c2c1fd7af13c3db6a6", "5ca494878ceaaa942e2176fb41bbbcb0899761b9c7abf83da358be00a62184ec", "031d960c4b26f2a055c974ec668e78132ed339fd04cb3b550af7fc891b758e97"}
	require(b["status"] == "INDEPENDENT_SEALED544_CARRY_BRIDGE" && b["fold_sha256"] == v9PriorSHA && reflect.DeepEqual(arr(b["fold_vector"]), vector) && hashValue(vector) == v9PriorSHA && b["old_originals_or_measurements_changed"] == false && b["prospective_model_admission"] == false && integer(b["remaining_absolute615_attempts"]) == 71, "v9_closed_fold")
	c := obj(all["failure-correlation"])
	require(c["status"] == "INDEPENDENT_ACTUAL_V8_CHAIN_NO_GO_CORRELATED" && c["native_scope_sealed"] == true && c["issue33_ready"] == false && integer(c["native_request_count"]) == 3 && c["actual_original_result"] == nil && integer(c["actual_notice_count"]) == 0, "v9_closed_no_go")
	failure := obj(c["failure_classification"])
	require(failure["specific_prior541_cause"] == "UNKNOWN_FROM_CURRENT_SAFE_EXPORTS" && failure["child_pre_native_failure_reason"] == "UNKNOWN_FROM_CURRENT_SAFE_EXPORTS" && failure["ordinal544_original_child_reference_equals_actual_admitted_child"] == true && failure["ordinal544_child_candidate_or_final_ready"] == false, "v9_previous_unknowns_preserved")
	require(obj(obj(all["final-audit"])["chain_audit"])["status"] == "NO_GO", "v9_previous_failure_not_promoted")
	var in, out int64
	unknown := 0
	for _, v := range usage {
		u := obj(v)
		if u["actual_input_tokens"] == nil {
			require(u["actual_output_tokens"] == nil, "v9_historical_pair")
			unknown++
		} else {
			require(u["actual_output_tokens"] != nil, "v9_historical_pair")
			in += integer(u["actual_input_tokens"])
			out += integer(u["actual_output_tokens"])
		}
	}
	carry := obj(c["carry_forward"])
	require(in == 348648 && out == 13138 && unknown == 28 && integer(carry["known_total_tokens_subtotal"]) == 647378 && integer(carry["unknown_total_attempts"]) == 48 && carry["old_unknown_266_cause"] == "UNKNOWN" && reflect.DeepEqual(carry, obj(b["carry_forward"])), "v9_historical_cost_honest")
	d := obj(all["proposal-diagnostics"])
	require(d["status"] == "INDEPENDENT_THREE_ORIGINAL_PROPOSALS_CORRELATED_CHAIN_NO_GO" && d["issue33_ready"] == false && d["whole_private_directory_read_or_mounted"] == false && d["all_model_argument_values_remain_private"] == true && d["ordinal544_requested_child_equals_actual_admitted_child"] == true && d["ordinal544_required_fields_exact"] == true && d["ordinal544_action_intent_absent"] == true && d["specific_prior541_cause"] == "UNKNOWN", "v9_closed_proposal_boundary")
	proposals := arr(d["proposals"])
	require(len(proposals) == 3, "v9_closed_proposal_inventory")
	for i, v := range proposals {
		p := obj(v)
		require(integer(p["ordinal"]) == int64(542+i) && p["original_authorization_hash_matches_metadata"] == true && p["original_bytes_and_base64_match"] == true && p["model_argument_values_emitted"] == false && p["actual_core_authorized_action_match"] == (i < 2), "v9_closed_original_proposals")
	}
}
