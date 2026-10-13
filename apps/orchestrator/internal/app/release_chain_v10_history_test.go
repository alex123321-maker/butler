package app

import (
	"encoding/json"
	"reflect"
	"strconv"
	"testing"
	"time"
)

// This reader preserves the closed V9 observation. It grants no prospective
// admission and does not replace the original window's 615 ceiling.
const releaseChainV10HistoryArtifactSHA = "8509f520a5b013900c5365f06868656448afd74a3f24d2f88971f013024055f5"
const releaseChainV10HistoryFold = "a8ff280f6905081c83c48f8c0dc327576792e0de9ddd44b8e952cc9d0e0ec00d"
const releaseChainV10HistoryBridgeSHA = "70041e9a3275a2099d39eda98d4535420eea7bfdae1bb3eb34bd58662f51ae3b"
const releaseChainV10HistoryFailedCode = "1937bdc8f152469d918ea83092a8d75a614f07c5"
const releaseChainV10HistoryFailedTree = "fcfa5d238138cb1b49e2fc6462ec7c9694b5ecb0328619a25247548cd0a3c852"
const releaseChainV10HistoryFailedApp = "6a208f2d1d0a041b1c6fcdc40b390380daebc4a24865f02cd2afca48f07b9309"

func releaseChainV10HistoryFailedArtifacts() map[string]string {
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

type releaseChainV10HistoryObjects struct {
	History          nativeHistoricalEvidence
	Bridge           map[string]any
	Evidence         map[string]map[string]any
	Product, Initial []map[string]any
}

func readReleaseChainV10HistoryObjects() (releaseChainV10HistoryObjects, error) {
	v := releaseChainV10HistoryObjects{Evidence: map[string]map[string]any{}}
	raw, err := readReleaseChainV5Artifact(highHistoryPath("release-chain-v9-history556-20261013.json"), releaseChainV10HistoryArtifactSHA)
	if err != nil || json.Unmarshal(raw, &v.History) != nil {
		return v, errNativeFixtureDenied
	}
	raw, err = readReleaseChainV5Artifact(highHistoryPath("release-chain-v9-carry556-independent-20261013.json"), releaseChainV10HistoryBridgeSHA)
	if err != nil || json.Unmarshal(raw, &v.Bridge) != nil {
		return v, errNativeFixtureDenied
	}
	for name, pin := range releaseChainV10HistoryFailedArtifacts() {
		raw, err = readReleaseChainV5Artifact(highHistoryPath("release-chain-v9-"+name+"-20261013.json"), pin)
		if err != nil {
			return v, errNativeFixtureDenied
		}
		switch name {
		case "pg-product":
			err = json.Unmarshal(raw, &v.Product)
		case "pg-initial":
			err = json.Unmarshal(raw, &v.Initial)
		default:
			var m map[string]any
			err = json.Unmarshal(raw, &m)
			v.Evidence[name] = m
		}
		if err != nil {
			return v, errNativeFixtureDenied
		}
	}
	return v, nil
}

func validateReleaseChainV10History(prior nativeHistoricalEvidence, v releaseChainV10HistoryObjects) error {
	h, b, all := v.History, v.Bridge, v.Evidence
	pins := releaseChainV10HistoryFailedArtifacts()
	if h.NativeAttempts != 556 || len(h.Usage) != 556 || h.Status != prior.Status || h.KnownUsagePairs != 528 || h.UnknownUsagePairs != 28 || h.BaselineSHA256 != releaseChainV10HistoryFold || len(prior.Usage) != 544 || !reflect.DeepEqual(h.Usage[:544], prior.Usage) || len(h.Artifacts) != len(prior.Artifacts)+1 || !reflect.DeepEqual(h.Artifacts[:len(prior.Artifacts)], prior.Artifacts) || h.Artifacts[len(prior.Artifacts)] != (nativePriorArtifact{ReportSHA256: pins["final-audit"], ReleaseRef: releaseChainV10HistoryFailedCode, NewAttempts: 12}) {
		return errNativeFixtureDenied
	}
	var input, output int64
	unknown := 0
	for _, u := range h.Usage {
		if (u.ActualInputTokens == nil) != (u.ActualOutputTokens == nil) {
			return errNativeFixtureDenied
		}
		if u.ActualInputTokens == nil {
			unknown++
		} else {
			input += *u.ActualInputTokens
			output += *u.ActualOutputTokens
		}
	}
	if input != 475637 || output != 15159 || unknown != 28 {
		return errNativeFixtureDenied
	}
	vector := []string{prior.BaselineSHA256, pins["final-audit"], pins["high"], pins["pg-ledger"], pins["pg-product"], pins["pg-initial"], pins["runtime-state"], pins["failure-correlation"], releaseChainV10HistoryFailedCode, releaseChainV10HistoryFailedTree, releaseChainV10HistoryFailedApp}
	var actual []string
	for _, x := range releaseChainV6Array(b, "fold_vector") {
		s, ok := x.(string)
		if !ok {
			return errNativeFixtureDenied
		}
		actual = append(actual, s)
	}
	if !reflect.DeepEqual(vector, actual) || shadowV4Hash(vector) != releaseChainV10HistoryFold || !releaseChainV6Exact(b, map[string]any{"status": "INDEPENDENT_SEALED556_CARRY_BRIDGE", "prior_history544_sha256": releaseChainV9HistoryArtifactSHA, "prior_fold_sha256": releaseChainV9HistoryFold, "fold_sha256": releaseChainV10HistoryFold, "remaining_original_absolute615_attempts": float64(59), "prospective_model_admission": false, "old_originals_or_measurements_changed": false, "all_new_expense_measured": true, "issue33_ready": false}) {
		return errNativeFixtureDenied
	}
	c, scope, sql, audit, diagnostic := all["failure-correlation"], all["high"], all["pg-ledger"], all["final-audit"], all["proposal-diagnostics"]
	if !releaseChainV6Exact(c, map[string]any{"status": "INDEPENDENT_ACTUAL_V9_CHAIN_NO_GO_CORRELATED", "code_commit": releaseChainV10HistoryFailedCode, "source_sha256": releaseChainV10HistoryFailedTree, "native_scope_sealed": true, "native_request_count": float64(12), "kernel_model_reservation_count": float64(12), "extra_unobserved_kernel_model_quotes": float64(0), "actual_original_result": nil, "actual_notice_count": float64(0), "actual_channel_requests": float64(0), "actual_model_retry": nil, "accepted_model_retry_failures": float64(0), "budget_all_eight_dimensions_recomputed": true, "budget_scope_count": float64(8), "budget_reservation_count": float64(28), "all_owning_client_request_intervals_serialized": true, "issue33_ready": false}) || !releaseChainV6Exact(scope, map[string]any{"status": "failed", "baseline_attempts": float64(544), "maximum_new_attempts": float64(71), "maximum_total_attempts": float64(615), "human_messages": float64(0), "issue33_ready": false}) || !releaseChainV6Exact(releaseChainV6Object(audit, "chain_audit"), map[string]any{"status": "NO_GO", "carry_forward_attempts": float64(556), "accepted_model_retry_failures": float64(0), "issue33_ready": false}) || len(releaseChainV6Object(scope, "scenarios")) != 0 || len(releaseChainV6Array(c, "verified_scenarios")) != 0 {
		return errNativeFixtureDenied
	}
	carry := releaseChainV6Object(c, "carry_forward")
	if !reflect.DeepEqual(carry, b["carry_forward"]) || !releaseChainV6Exact(carry, map[string]any{"attempts": float64(556), "known_input_tokens_subtotal": float64(475637), "known_output_tokens_subtotal": float64(15159), "known_total_tokens_subtotal": float64(793703), "known_usage_pairs": float64(528), "unknown_usage_pairs": float64(28), "unknown_total_attempts": float64(48), "old_unknown_266_cause": "UNKNOWN"}) || !releaseChainV6Exact(releaseChainV6Object(c, "failure_classification"), map[string]any{"exact_ordinal556_failure_subcause": "UNKNOWN_FROM_CURRENT_SAFE_EXPORTS", "specific_prior541_cause": "UNKNOWN_FROM_CURRENT_SAFE_EXPORTS", "specific_prior544_child_cause": "UNKNOWN_FROM_CURRENT_SAFE_EXPORTS", "ordinal556_effect_state": "effect_unknown", "ordinal556_dispatch_outcome": "unknown", "ordinal556_required_shape_valid": true, "ordinal556_target_equals_fixed_project_fixture": true, "global_deadline_expired": false, "model_deadline_expired": false}) {
		return errNativeFixtureDenied
	}
	paid := releaseChainV6Object(scope, "scope")
	window := releaseChainV6Object(paid, "window")
	if !reflect.DeepEqual(window, sql["window"]) || !releaseChainV6Exact(paid, map[string]any{"total_attempts": float64(556), "new_attempts": float64(12), "failed_provider_entries": float64(0), "unknown_full_token_cost": float64(0), "unknown_token_cost_entries": float64(0), "unknown_model_version_entries": float64(0), "accepted_model_retry_failures": float64(0), "accepted_unmeasured_timeout_entries": float64(0)}) || !releaseChainV6Exact(window, map[string]any{"native_baseline": float64(544), "attempts": float64(556), "maximum_new_attempts": float64(71), "prior_history_hash": releaseChainV9HistoryFold, "prerequisite_hash": releaseChainV9ReviewedPrerequisiteSHA, "provider_stage_observation": true, "kernel_operation_binding": true}) {
		return errNativeFixtureDenied
	}
	policy := releaseChainV6Object(window, "protocol_policy")
	if len(policy) != 8 || !releaseChainV6Exact(policy, map[string]any{"maximum_generation_tokens": float64(32768), "safe_timeout_stages": "http_or_stream", "maximum_attempt_ms": float64(300000), "maximum_case_ms": float64(2400000), "maximum_global_ms": float64(7200000), "proposal_retention_required": true, "maximum_private_proposal_bytes": float64(262144), "provider_entries_serialized": true}) {
		return errNativeFixtureDenied
	}
	rows, sqlRows := releaseChainV6Array(paid, "requests"), releaseChainV6Array(sql, "requests")
	inputs := []int64{7160, 10092, 11848, 4063, 13283, 14888, 15507, 7743, 9185, 10097, 11039, 12084}
	outputs := []int64{155, 404, 92, 155, 165, 77, 518, 54, 77, 218, 81, 25}
	totals := []int64{9035, 11739, 12245, 12946, 14071, 15203, 16883, 8368, 9793, 10613, 11267, 14162}
	elapsed := []int64{15629, 13049, 5898, 55795, 7766, 5194, 11395, 6017, 5153, 4891, 4663, 15197}
	if len(rows) != 12 || len(sqlRows) != 12 {
		return errNativeFixtureDenied
	}
	var finished time.Time
	operations := map[string]bool{}
	runKinds := map[string]bool{}
	for n, item := range rows {
		r, ok := item.(map[string]any)
		pg, pgOK := sqlRows[n].(map[string]any)
		if !ok || !pgOK {
			return errNativeFixtureDenied
		}
		canon, err := releaseChainV6CanonicalRow(pg)
		if err != nil || !reflect.DeepEqual(canon, r) {
			return errNativeFixtureDenied
		}
		kind := "submit"
		if n == 0 || n == 3 || n == 7 {
			kind = "start"
		}
		terminal := float64(0)
		var reason any
		if n == 2 || n == 6 {
			terminal = 1
			reason = "STOP"
		}
		if !releaseChainV6Exact(r, map[string]any{"ordinal": float64(545 + n), "kind": kind, "is_parent": n < 3 || n > 6, "scenario": "normal_sent", "outcome": "completed", "provider_operation_stage": "response_stream", "model_version": "gemini-3.8-flash-n", "actual_input_tokens": float64(inputs[n]), "actual_output_tokens": float64(outputs[n]), "actual_total_tokens": float64(totals[n]), "elapsed_ms": float64(elapsed[n]), "failure_stage": nil, "diagnostic_code": nil, "finish_reason": reason, "completed_count": terminal, "final_count": terminal, "terminal_count": terminal, "stop_final_count": terminal, "cancelled_count": float64(0), "timed_out_count": float64(0), "transport_error_count": float64(0), "max_tokens_final_count": float64(0), "usage_observation_uncertain": false, "version_observation_uncertain": false, "finish_observation_uncertain": false}) {
			return errNativeFixtureDenied
		}
		admittedText, admittedOK := r["admitted_at_utc"].(string)
		finishedText, finishedOK := r["finished_at_utc"].(string)
		if !admittedOK || !finishedOK {
			return errNativeFixtureDenied
		}
		admitted, e1 := time.Parse(time.RFC3339Nano, admittedText)
		end, e2 := time.Parse(time.RFC3339Nano, finishedText)
		if e1 != nil || e2 != nil || admitted.Before(finished) || end.Before(admitted) || end.Sub(admitted) > 300*time.Second {
			return errNativeFixtureDenied
		}
		finished = end
		op, ok := r["kernel_operation_id"].(string)
		if !ok || op == "" || operations[op] {
			return errNativeFixtureDenied
		}
		operations[op] = true
		run, ok := r["run_id"].(string)
		if !ok {
			return errNativeFixtureDenied
		}
		runKinds[run] = true
		u := h.Usage[544+n]
		if u.RunID != run || u.ReservationID != "release-chain-v9-native-"+strconv.Itoa(545+n) || u.ActualInputTokens == nil || u.ActualOutputTokens == nil || *u.ActualInputTokens != inputs[n] || *u.ActualOutputTokens != outputs[n] {
			return errNativeFixtureDenied
		}
	}
	if len(runKinds) != 3 || len(v.Product) != 1 || len(v.Initial) != 1 {
		return errNativeFixtureDenied
	}
	for _, w := range []map[string]any{v.Product[0], v.Initial[0]} {
		if w["scenario"] != "normal_sent" || shadowV4Hash(w["payload_json"]) != w["payload_sha256"] {
			return errNativeFixtureDenied
		}
	}
	product := releaseChainV6Object(v.Product[0], "payload_json")
	db := releaseChainV6Object(product, "postgres")
	initial := releaseChainV6Object(v.Initial[0], "payload_json")
	if !releaseChainV6Exact(product, map[string]any{"test_failed": true, "original_result": nil, "patch_artifact_file_sha256": nil, "all_channel_requests": float64(0), "recipient_acceptances": float64(0)}) || !releaseChainV6Exact(initial, map[string]any{"preexisting_case_native_requests": float64(0), "preexisting_root_model_reservations": float64(0), "existing_target_catalogs": float64(0), "existing_target_versions": float64(0), "setup_execution": "owner_services_and_real_approved_broker_reads_without_model"}) || len(releaseChainV6Array(db, "actions")) != 10 || len(releaseChainV6Array(db, "model_budget_attempts")) != 12 || len(releaseChainV6Array(db, "budget_reservations")) != 28 || len(releaseChainV6Array(db, "budget_scopes")) != 8 || len(releaseChainV6Array(db, "notices")) != 0 || len(releaseChainV6Array(db, "converter_usages")) != 0 {
		return errNativeFixtureDenied
	}
	known, uncertain := 0, 0
	for _, item := range releaseChainV6Array(db, "actions") {
		r, ok := item.(map[string]any)
		if !ok {
			return errNativeFixtureDenied
		}
		switch r["state"] {
		case "effect_confirmed":
			known++
		case "effect_unknown":
			uncertain++
			if r["tool_name"] != "extension.project.inspect" || r["dispatch_outcome"] != "unknown" {
				return errNativeFixtureDenied
			}
		default:
			return errNativeFixtureDenied
		}
	}
	if known != 9 || uncertain != 1 || !releaseChainV6Exact(diagnostic, map[string]any{"status": "INDEPENDENT_TEN_ORIGINAL_PROPOSALS_CORRELATED_CHAIN_NO_GO", "all_ten_core_action_fingerprints_match": true, "confirmed_actions": float64(9), "unknown_actions": float64(1), "verified_scenarios": float64(0), "specific_prior541_cause": "UNKNOWN", "specific_prior544_child_cause": "UNKNOWN", "all_model_argument_values_remain_private": true, "whole_private_directory_read_or_mounted": false, "issue33_ready": false}) {
		return errNativeFixtureDenied
	}
	proofs := releaseChainV6Array(diagnostic, "proposals")
	ordinals := []int{545, 546, 548, 549, 550, 552, 553, 554, 555, 556}
	if len(proofs) != 10 {
		return errNativeFixtureDenied
	}
	for n, item := range proofs {
		r, ok := item.(map[string]any)
		state := "effect_confirmed"
		if n == 9 {
			state = "effect_unknown"
		}
		if !ok || !releaseChainV6Exact(r, map[string]any{"ordinal": float64(ordinals[n]), "original_bytes_and_base64_match": true, "original_authorization_hash_matches_metadata": true, "actual_core_authorized_action_match": true, "actual_action_state": state, "model_argument_values_emitted": false}) {
			return errNativeFixtureDenied
		}
	}
	if !releaseChainV6Exact(releaseChainV6Object(diagnostic, "ordinal556_inspect_shape"), map[string]any{"required_fields_exact": true, "url_field_type_string": true, "url_equals_fixed_project_fixture_target": true, "body_absent": true, "authorization_absent": true, "current_action_authorization_hash_matches": true, "action_state": "effect_unknown", "dispatch_outcome": "unknown", "exact_failure_subcause": "UNKNOWN_FROM_CURRENT_SAFE_EXPORTS"}) {
		return errNativeFixtureDenied
	}
	return nil
}

func readReleaseChainV10ActualHistory() (nativeHistoricalEvidence, error) {
	prior, err := readReleaseChainV9ActualHistory()
	if err != nil {
		return nativeHistoricalEvidence{}, errNativeFixtureDenied
	}
	v, err := readReleaseChainV10HistoryObjects()
	if err != nil || validateReleaseChainV10History(prior, v) != nil {
		return nativeHistoricalEvidence{}, errNativeFixtureDenied
	}
	return v.History, nil
}

func TestReleaseChainV10ActualHistoryPreserves556(t *testing.T) {
	h, err := readReleaseChainV10ActualHistory()
	if err != nil || h.NativeAttempts != 556 || h.Usage[536].ActualInputTokens != nil || h.Usage[538].ActualInputTokens != nil {
		t.Fatal("actual closed556 or historical unknown denied", err)
	}
}

func TestReleaseChainV10HistoryRejectsTampering(t *testing.T) {
	prior, err := readReleaseChainV9ActualHistory()
	if err != nil {
		t.Fatal(err)
	}
	original, err := readReleaseChainV10HistoryObjects()
	if err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"old_usage_changed", "new_cost_changed", "terminal_pattern_changed", "parallel_intervals", "inspect_unknown_promoted", "old_no_go_promoted", "proposal_binding_lost", "old_ceiling_refunded", "invented_failure_cause"} {
		t.Run(name, func(t *testing.T) {
			raw, _ := json.Marshal(original)
			var v releaseChainV10HistoryObjects
			if json.Unmarshal(raw, &v) != nil {
				t.Fatal("clone failed")
			}
			s := releaseChainV6Object(v.Evidence["high"], "scope")
			rows := releaseChainV6Array(s, "requests")
			r := rows[0].(map[string]any)
			switch name {
			case "old_usage_changed":
				x := int64(1)
				v.History.Usage[536].ActualInputTokens = &x
				v.History.Usage[536].ActualOutputTokens = &x
			case "new_cost_changed":
				r["actual_total_tokens"] = float64(9036)
			case "terminal_pattern_changed":
				rows[2].(map[string]any)["terminal_count"] = float64(0)
			case "parallel_intervals":
				rows[3].(map[string]any)["admitted_at_utc"] = r["admitted_at_utc"]
				// Change both projections so equality is intact and the serial
				// interval predicate must reject the overlapping child call.
				releaseChainV6Array(v.Evidence["pg-ledger"], "requests")[3].(map[string]any)["admitted_at"] = r["admitted_at_utc"]
			case "inspect_unknown_promoted":
				releaseChainV6Object(v.Evidence["proposal-diagnostics"], "ordinal556_inspect_shape")["action_state"] = "effect_confirmed"
			case "old_no_go_promoted":
				releaseChainV6Object(v.Evidence["final-audit"], "chain_audit")["status"] = "PASS"
			case "proposal_binding_lost":
				releaseChainV6Array(v.Evidence["proposal-diagnostics"], "proposals")[9].(map[string]any)["actual_core_authorized_action_match"] = false
			case "old_ceiling_refunded":
				v.Bridge["remaining_original_absolute615_attempts"] = float64(71)
			case "invented_failure_cause":
				releaseChainV6Object(v.Evidence["failure-correlation"], "failure_classification")["exact_ordinal556_failure_subcause"] = "runtime_timeout_proved"
			}
			if validateReleaseChainV10History(prior, v) == nil {
				t.Fatal("tampered closed history accepted", name)
			}
		})
	}
}
