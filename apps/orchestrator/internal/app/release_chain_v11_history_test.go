package app

import (
	"encoding/json"
	"reflect"
	"testing"
)

const releaseChainV11HistoryArtifactSHA = "fb85f5ce35a5217990d83af664060908574c0ac0682fa8d111e20ac0c2deef0f"
const releaseChainV11HistoryBridgeSHA = "78d320c595483d666b25cbaee55be764b53b0004bdd6806ecb4924e4f01130ef"
const releaseChainV11HistoryFold = "fb4b085728544f0709dbc58dc7d5a3ba678412cc6a0c96514363a6fab2c27dee"

func releaseChainV11HistoryFailedArtifacts() map[string]string {
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

type releaseChainV11HistoryObjects struct {
	History          nativeHistoricalEvidence
	Bridge           map[string]any
	Evidence         map[string]map[string]any
	Product, Initial []map[string]any
}

func readReleaseChainV11HistoryObjects() (releaseChainV11HistoryObjects, error) {
	v := releaseChainV11HistoryObjects{Evidence: map[string]map[string]any{}}
	raw, err := readReleaseChainV5Artifact(highHistoryPath("release-chain-v10-history557-20261013.json"), releaseChainV11HistoryArtifactSHA)
	if err != nil || json.Unmarshal(raw, &v.History) != nil {
		return v, errNativeFixtureDenied
	}
	raw, err = readReleaseChainV5Artifact(highHistoryPath("release-chain-v10-carry557-independent-20261013.json"), releaseChainV11HistoryBridgeSHA)
	if err != nil || json.Unmarshal(raw, &v.Bridge) != nil {
		return v, errNativeFixtureDenied
	}
	for name, pin := range releaseChainV11HistoryFailedArtifacts() {
		raw, err = readReleaseChainV5Artifact(highHistoryPath("release-chain-v10-"+name+"-20261013.json"), pin)
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

// The historical cancellation remains outside the nominal provider timeout
// recovery policy. This function creates neither a waiver nor a new window.
func validateReleaseChainV11History(prior nativeHistoricalEvidence, v releaseChainV11HistoryObjects) error {
	h, b, all := v.History, v.Bridge, v.Evidence
	pins := releaseChainV11HistoryFailedArtifacts()
	const code = "0f9b61e23fd56fefddd1c42cd3aa9efa14087d83"
	const tree = "2096a0a5f7a80ad8a912dc0df1f7b2452053d68b27b55c6abdd783af6ce96879"
	const app = "4deec48d416f372c9606b3e6e1d3fc168a8e479427d5be3e7660bf7fc6934694"
	if len(prior.Usage) != 556 || h.NativeAttempts != 557 || len(h.Usage) != 557 || h.Status != prior.Status || h.KnownUsagePairs != 528 || h.UnknownUsagePairs != 29 || h.BaselineSHA256 != releaseChainV11HistoryFold || !reflect.DeepEqual(h.Usage[:556], prior.Usage) || len(h.Artifacts) != len(prior.Artifacts)+1 || !reflect.DeepEqual(h.Artifacts[:len(prior.Artifacts)], prior.Artifacts) || h.Artifacts[len(prior.Artifacts)] != (nativePriorArtifact{ReportSHA256: pins["final-audit"], ReleaseRef: code, NewAttempts: 1}) {
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
	if input != 475637 || output != 15159 || unknown != 29 {
		return errNativeFixtureDenied
	}
	vector := []string{prior.BaselineSHA256, pins["final-audit"], pins["high"], pins["pg-ledger"], pins["pg-product"], pins["pg-initial"], pins["runtime-state"], pins["failure-correlation"], code, tree, app}
	actual := []string{}
	for _, item := range releaseChainV6Array(b, "fold_vector") {
		s, ok := item.(string)
		if !ok {
			return errNativeFixtureDenied
		}
		actual = append(actual, s)
	}
	if !reflect.DeepEqual(vector, actual) || shadowV4Hash(vector) != releaseChainV11HistoryFold || !releaseChainV6Exact(b, map[string]any{"status": "INDEPENDENT_SEALED557_CARRY_BRIDGE", "prior_history556_sha256": releaseChainV10HistoryArtifactSHA, "prior_fold_sha256": releaseChainV10HistoryFold, "fold_sha256": releaseChainV11HistoryFold, "all_new_expense_measured": false, "new_unknown_cancelled_source_check_entry": float64(557), "new_unknown_is_nominal_safe_timeout_exception": false, "remaining_original_absolute627_attempts": float64(70), "prospective_model_admission": false, "old_originals_or_measurements_changed": false, "issue33_ready": false}) {
		return errNativeFixtureDenied
	}
	c, scope, pg, audit := all["failure-correlation"], all["high"], all["pg-ledger"], all["final-audit"]
	if !releaseChainV6Exact(c, map[string]any{"status": "INDEPENDENT_ACTUAL_V10_CHAIN_NO_GO_CORRELATED", "code_commit": code, "source_sha256": tree, "native_scope_sealed": true, "native_request_count": float64(1), "kernel_model_reservation_count": float64(1), "actual_original_result": nil, "actual_notice_count": float64(0), "actual_channel_requests": float64(0), "actual_model_retry": nil, "accepted_model_retry_failures": float64(0), "original_private_tool_proposals_read": float64(0), "all_new_expense_measured": false, "known_zero_subtotal_is_not_zero_actual_expense": true, "budget_all_eight_dimensions_recomputed": true, "budget_reservation_count": float64(6), "budget_scope_count": float64(7), "issue33_ready": false}) || !releaseChainV6Exact(releaseChainV6Object(audit, "chain_audit"), map[string]any{"status": "NO_GO", "carry_forward_attempts": float64(557), "all_new_expense_measured": false, "accepted_model_retry_failures": float64(0), "issue33_ready": false}) {
		return errNativeFixtureDenied
	}
	if !releaseChainV6Exact(scope, map[string]any{"status": "failed", "baseline_attempts": float64(556), "maximum_new_attempts": float64(71), "maximum_total_attempts": float64(627), "human_messages": float64(0), "issue33_ready": false}) || !reflect.DeepEqual(scope["scenarios"], map[string]any{"normal_sent": false}) || len(releaseChainV6Array(scope, "proposal_observations")) != 0 || len(releaseChainV6Array(c, "verified_scenarios")) != 0 {
		return errNativeFixtureDenied
	}
	carry := releaseChainV6Object(c, "carry_forward")
	if !reflect.DeepEqual(carry, b["carry_forward"]) || !releaseChainV6Exact(carry, map[string]any{"attempts": float64(557), "known_input_tokens_subtotal": float64(475637), "known_output_tokens_subtotal": float64(15159), "known_total_tokens_subtotal": float64(793703), "unknown_total_attempts": float64(49), "unknown_usage_pairs": float64(29), "known_usage_pairs": float64(528), "old_unknown_266_cause": "UNKNOWN"}) || !releaseChainV6Exact(releaseChainV6Object(c, "failure_classification"), map[string]any{"core_stop_reason": "memory_source_check_timeout", "core_source_check_cancellation_not_nominal_model_timeout": true, "current_source_check_underlying_timeout_cause": "UNKNOWN_FROM_CURRENT_SAFE_EXPORTS", "safe_model_retry_authorized_or_observed": false, "new_unknown_expense_estimated_or_refunded": false, "all_original_native_usage_and_version": nil, "specific_prior541_cause": "UNKNOWN_FROM_CURRENT_SAFE_EXPORTS", "specific_prior544_child_cause": "UNKNOWN_FROM_CURRENT_SAFE_EXPORTS", "specific_prior556_inspect_cause": "UNKNOWN_FROM_CURRENT_SAFE_EXPORTS"}) {
		return errNativeFixtureDenied
	}
	paid := releaseChainV6Object(scope, "scope")
	window := releaseChainV6Object(paid, "window")
	if !reflect.DeepEqual(window, pg["window"]) || !releaseChainV6Exact(window, map[string]any{"native_baseline": float64(556), "attempts": float64(557), "maximum_new_attempts": float64(71), "prior_history_hash": releaseChainV10HistoryFold, "prerequisite_hash": releaseChainV10ReviewedPrerequisiteSHA}) || !releaseChainV6Exact(paid, map[string]any{"total_attempts": float64(557), "new_attempts": float64(1), "failed_provider_entries": float64(1), "unknown_full_token_cost": float64(1), "unknown_token_cost_entries": float64(1), "unknown_model_version_entries": float64(1), "accepted_model_retry_failures": float64(0), "accepted_unmeasured_timeout_entries": float64(0)}) {
		return errNativeFixtureDenied
	}
	rows, sql := releaseChainV6Array(paid, "requests"), releaseChainV6Array(pg, "requests")
	if len(rows) != 1 || len(sql) != 1 {
		return errNativeFixtureDenied
	}
	r, ok := rows[0].(map[string]any)
	s, sqlOK := sql[0].(map[string]any)
	if !ok || !sqlOK {
		return errNativeFixtureDenied
	}
	canon, err := releaseChainV6CanonicalRow(s)
	if err != nil || !reflect.DeepEqual(canon, r) {
		return errNativeFixtureDenied
	}
	if !releaseChainV6Exact(r, map[string]any{"ordinal": float64(557), "kind": "start", "is_parent": true, "scenario": "normal_sent", "outcome": "cancelled", "provider_operation_stage": "http_round_trip", "failure_stage": "stream", "diagnostic_code": "transport_connection_error", "elapsed_ms": float64(13506), "actual_input_tokens": nil, "actual_output_tokens": nil, "actual_total_tokens": nil, "model_version": nil, "finish_reason": nil, "completed_count": float64(0), "final_count": float64(0), "terminal_count": float64(0), "stop_final_count": float64(0), "cancelled_count": float64(0), "timed_out_count": float64(0), "transport_error_count": float64(0), "max_tokens_final_count": float64(0), "usage_observation_uncertain": false, "version_observation_uncertain": false, "finish_observation_uncertain": false}) {
		return errNativeFixtureDenied
	}
	u := h.Usage[556]
	if u.RunID != r["run_id"] || u.ReservationID != "release-chain-v10-native-557" || u.ActualInputTokens != nil || u.ActualOutputTokens != nil {
		return errNativeFixtureDenied
	}
	if len(v.Product) != 1 || len(v.Initial) != 1 {
		return errNativeFixtureDenied
	}
	for _, w := range []map[string]any{v.Product[0], v.Initial[0]} {
		if w["scenario"] != "normal_sent" || shadowV4Hash(w["payload_json"]) != w["payload_sha256"] {
			return errNativeFixtureDenied
		}
	}
	fin := releaseChainV6Object(v.Product[0], "payload_json")
	db := releaseChainV6Object(fin, "postgres")
	initial := releaseChainV6Object(v.Initial[0], "payload_json")
	if !releaseChainV6Exact(fin, map[string]any{"original_result": nil, "patch_artifact_file_sha256": nil, "all_channel_requests": float64(0), "recipient_acceptances": float64(0)}) || !releaseChainV6Exact(initial, map[string]any{"preexisting_case_native_requests": float64(0), "preexisting_root_model_reservations": float64(0), "existing_target_versions": float64(0), "existing_target_catalogs": float64(0)}) || db["live_tree_leases"] != float64(0) {
		return errNativeFixtureDenied
	}
	for _, table := range []string{"actions", "dispatches", "delegations", "developer_admissions", "converter_returns", "converter_versions", "converter_catalog", "converter_validations", "converter_trials", "converter_usages", "notices", "final_results"} {
		if len(releaseChainV6Array(db, table)) != 0 {
			return errNativeFixtureDenied
		}
	}
	inputs, ops := releaseChainV6Array(db, "accepted_inputs"), releaseChainV6Array(db, "model_budget_attempts")
	if len(inputs) != 1 || len(ops) != 1 {
		return errNativeFixtureDenied
	}
	in, inOK := inputs[0].(map[string]any)
	op, opOK := ops[0].(map[string]any)
	if !inOK || !opOK || !releaseChainV6Exact(in, map[string]any{"status": "blocked", "error_code": "execution_stopped", "attempts": float64(1), "model_retry": nil}) || !releaseChainV6Exact(op, map[string]any{"operation_id": r["kernel_operation_id"], "run_id": r["run_id"], "actual_input_tokens": nil, "actual_output_tokens": nil, "observed_at": nil}) {
		return errNativeFixtureDenied
	}
	return nil
}

func readReleaseChainV11ActualHistory() (nativeHistoricalEvidence, error) {
	prior, err := readReleaseChainV10ActualHistory()
	if err != nil {
		return nativeHistoricalEvidence{}, errNativeFixtureDenied
	}
	v, err := readReleaseChainV11HistoryObjects()
	if err != nil || validateReleaseChainV11History(prior, v) != nil {
		return nativeHistoricalEvidence{}, errNativeFixtureDenied
	}
	return v.History, nil
}

func TestReleaseChainV11ActualHistoryPreserves557(t *testing.T) {
	h, err := readReleaseChainV11ActualHistory()
	if err != nil || h.NativeAttempts != 557 || h.Usage[556].ActualInputTokens != nil || h.Usage[556].ActualOutputTokens != nil || h.Usage[536].ActualInputTokens != nil || h.Usage[538].ActualInputTokens != nil {
		t.Fatal("closed557 unknown or earlier unknown altered", err)
	}
}

func TestReleaseChainV11HistoryRejectsTampering(t *testing.T) {
	prior, err := readReleaseChainV10ActualHistory()
	if err != nil {
		t.Fatal(err)
	}
	original, err := readReleaseChainV11HistoryObjects()
	if err != nil {
		t.Fatal(err)
	}
	for _, kind := range []string{"old_usage_changed", "new_null_filled", "source_cancel_called_nominal_retry", "no_go_promoted", "old_ceiling_refunded", "tool_effect_invented", "underlying_cause_invented"} {
		t.Run(kind, func(t *testing.T) {
			raw, _ := json.Marshal(original)
			var v releaseChainV11HistoryObjects
			if json.Unmarshal(raw, &v) != nil {
				t.Fatal("clone failed")
			}
			switch kind {
			case "old_usage_changed":
				x := int64(1)
				v.History.Usage[536].ActualInputTokens = &x
				v.History.Usage[536].ActualOutputTokens = &x
			case "new_null_filled":
				x := int64(0)
				v.History.Usage[556].ActualInputTokens = &x
				v.History.Usage[556].ActualOutputTokens = &x
			case "source_cancel_called_nominal_retry":
				releaseChainV6Object(v.Evidence["failure-correlation"], "failure_classification")["core_source_check_cancellation_not_nominal_model_timeout"] = false
			case "no_go_promoted":
				releaseChainV6Object(v.Evidence["final-audit"], "chain_audit")["status"] = "PASS"
			case "old_ceiling_refunded":
				v.Bridge["remaining_original_absolute627_attempts"] = float64(71)
			case "tool_effect_invented":
				releaseChainV6Object(releaseChainV6Object(v.Product[0], "payload_json"), "postgres")["actions"] = []any{map[string]any{"state": "effect_confirmed"}}
			case "underlying_cause_invented":
				releaseChainV6Object(v.Evidence["failure-correlation"], "failure_classification")["current_source_check_underlying_timeout_cause"] = "provider_slow"
			}
			if validateReleaseChainV11History(prior, v) == nil {
				t.Fatal("tampered closed557 accepted", kind)
			}
		})
	}
}
