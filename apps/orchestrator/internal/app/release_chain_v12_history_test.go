package app

import (
	"encoding/json"
	"fmt"
	"reflect"
	"testing"
)

const releaseChainV12HistoryArtifactSHA = "4f62e26456070b298ee411f2d7681fbf6dda0d6662cf869a4cbcba8674303220"
const releaseChainV12HistoryBridgeSHA = "043ef974962836ab2c69691dccdc1d1add15c379632599b1889057cd55d3fc00"
const releaseChainV12HistoryFold = "d24720adf3522dc19ada8b53a7d54d4485fe7774c0cd029d3c717c9f1468db05"
const releaseChainV12HistoryProposalSHA = "be70e93a61d10873bab71a934a54ce763ee1d25f52b8c951adb6a70585ee8c2e"

func releaseChainV12HistoryFailedArtifacts() map[string]string {
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

type releaseChainV12HistoryObjects struct {
	History           nativeHistoricalEvidence
	Bridge, Proposals map[string]any
	Evidence          map[string]map[string]any
	Product, Initial  []map[string]any
}

func readReleaseChainV12HistoryObjects() (releaseChainV12HistoryObjects, error) {
	v := releaseChainV12HistoryObjects{Evidence: map[string]map[string]any{}}
	for _, item := range []struct {
		name, pin string
		target    any
	}{
		{"history575", releaseChainV12HistoryArtifactSHA, &v.History},
		{"carry575-independent", releaseChainV12HistoryBridgeSHA, &v.Bridge},
		{"proposal-diagnostics", releaseChainV12HistoryProposalSHA, &v.Proposals},
	} {
		raw, err := readReleaseChainV5Artifact(highHistoryPath("release-chain-v11-"+item.name+"-20261013.json"), item.pin)
		if err != nil || json.Unmarshal(raw, item.target) != nil {
			return v, errNativeFixtureDenied
		}
	}
	for name, pin := range releaseChainV12HistoryFailedArtifacts() {
		raw, err := readReleaseChainV5Artifact(highHistoryPath("release-chain-v11-"+name+"-20261013.json"), pin)
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

// Historical ingestion only: the fixed-count failure stays NO_GO and the two
// legacy read receipt fields stay NULL. This creates no prospective admission,
// new expense exception, repeated-read policy or native window.
func validateReleaseChainV12History(prior nativeHistoricalEvidence, v releaseChainV12HistoryObjects) error {
	h, b, all := v.History, v.Bridge, v.Evidence
	pins := releaseChainV12HistoryFailedArtifacts()
	const code = "323da2db21b37e2efd39639eaff98f838a33f0c6"
	const tree = "f7158f46360ed52794188063a1c7860c90d1cbb4ffc9d1c8b65f4fdbe9cc5849"
	const app = "e1e20c8d64b868f7a1752a10edee3f8f8dc10a4ceeea787a51fc5b48735fb25d"
	if len(prior.Usage) != 557 || h.NativeAttempts != 575 || len(h.Usage) != 575 || h.Status != prior.Status || h.KnownUsagePairs != 546 || h.UnknownUsagePairs != 29 || h.BaselineSHA256 != releaseChainV12HistoryFold || !reflect.DeepEqual(h.Usage[:557], prior.Usage) || len(h.Artifacts) != len(prior.Artifacts)+1 || !reflect.DeepEqual(h.Artifacts[:len(prior.Artifacts)], prior.Artifacts) || h.Artifacts[len(prior.Artifacts)] != (nativePriorArtifact{ReportSHA256: pins["final-audit"], ReleaseRef: code, NewAttempts: 18}) {
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
	if input != 689116 || output != 18790 || unknown != 29 {
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
	if !reflect.DeepEqual(vector, actual) || shadowV4Hash(vector) != releaseChainV12HistoryFold || !releaseChainV6Exact(b, map[string]any{"status": "INDEPENDENT_SEALED575_CARRY_BRIDGE", "prior_history557_sha256": releaseChainV11HistoryArtifactSHA, "prior_fold_sha256": releaseChainV11HistoryFold, "fold_sha256": releaseChainV12HistoryFold, "all_new_expense_measured": true, "new_unknown_expense_entries": float64(0), "remaining_original_absolute628_attempts": float64(53), "prior_absolute615_and627_history_changed": false, "prospective_model_admission": false, "old_originals_or_measurements_changed": false, "repeated_same_artifact_check_read_actual_receipts_unknown": true, "issue33_ready": false}) {
		return errNativeFixtureDenied
	}
	c, scope, pg, audit := all["failure-correlation"], all["high"], all["pg-ledger"], all["final-audit"]
	if !releaseChainV6Exact(c, map[string]any{"status": "INDEPENDENT_ACTUAL_V11_CHAIN_NO_GO_CORRELATED", "code_commit": code, "source_sha256": tree, "native_scope_sealed": true, "native_request_count": float64(18), "kernel_model_reservation_count": float64(18), "actual_notice_count": float64(0), "actual_channel_requests": float64(0), "actual_model_retry": nil, "accepted_model_retry_failures": float64(0), "original_private_tool_proposals_read": float64(14), "all_fourteen_actual_core_argument_fingerprints_match": true, "legacy_two_check_read_actual_response_fields_retained": false, "all_new_expense_measured": true, "budget_all_eight_dimensions_recomputed": true, "budget_scope_count": float64(8), "budget_reservation_count": float64(38), "issue33_ready": false}) || !releaseChainV6Exact(releaseChainV6Object(audit, "chain_audit"), map[string]any{"status": "NO_GO", "carry_forward_attempts": float64(575), "all_new_expense_measured": true, "accepted_model_retry_failures": float64(0), "issue33_ready": false}) {
		return errNativeFixtureDenied
	}
	if !releaseChainV6Exact(scope, map[string]any{"status": "failed", "baseline_attempts": float64(557), "maximum_new_attempts": float64(71), "maximum_total_attempts": float64(628), "human_messages": float64(0), "issue33_ready": false}) || len(releaseChainV6Object(scope, "scenarios")) != 0 || len(releaseChainV6Array(c, "verified_scenarios")) != 0 {
		return errNativeFixtureDenied
	}
	carry := releaseChainV6Object(c, "carry_forward")
	if !reflect.DeepEqual(carry, b["carry_forward"]) || !releaseChainV6Exact(carry, map[string]any{"attempts": float64(575), "known_input_tokens_subtotal": float64(689116), "known_output_tokens_subtotal": float64(18790), "known_total_tokens_subtotal": float64(1039921), "unknown_total_attempts": float64(49), "unknown_usage_pairs": float64(29), "known_usage_pairs": float64(546), "old_unknown_266_cause": "UNKNOWN"}) || !releaseChainV6Exact(releaseChainV6Object(c, "failure_classification"), map[string]any{"fixed_scripted_journal_expected_actions": float64(13), "actual_confirmed_actions": float64(14), "additional_confirmed_operation": "extension.project.check", "additional_operation_method": "GET", "additional_operation_manifest_effect": "read", "same_immutable_prepared_artifact_request_target": true, "actual_per_read_result_checks_passed": nil, "actual_per_read_result_applied": nil, "per_read_actual_receipt_fields_unknown_not_inferred_from_request": true, "source_check_or_provider_timeout_observed": false, "observer_cancellations_after_completed_runs_not_source_failures": true, "specific_prior541_cause": "UNKNOWN", "specific_prior544_child_cause": "UNKNOWN", "specific_prior556_inspect_cause": "UNKNOWN", "specific_prior557_source_cause": "UNKNOWN"}) {
		return errNativeFixtureDenied
	}
	paid := releaseChainV6Object(scope, "scope")
	window := releaseChainV6Object(paid, "window")
	if !reflect.DeepEqual(window, pg["window"]) || !releaseChainV6Exact(window, map[string]any{"native_baseline": float64(557), "attempts": float64(575), "maximum_new_attempts": float64(71), "prior_history_hash": releaseChainV11HistoryFold, "prerequisite_hash": releaseChainV11ReviewedPrerequisiteSHA}) || !releaseChainV6Exact(paid, map[string]any{"total_attempts": float64(575), "new_attempts": float64(18), "failed_provider_entries": float64(0), "unknown_full_token_cost": float64(0), "accepted_model_retry_failures": float64(0), "accepted_unmeasured_timeout_entries": float64(0)}) {
		return errNativeFixtureDenied
	}
	rows, sql := releaseChainV6Array(paid, "requests"), releaseChainV6Array(pg, "requests")
	if len(rows) != 18 || len(sql) != 18 {
		return errNativeFixtureDenied
	}
	// Exact measured input/output/total/latency and the four separate provider runs.
	expected := [][4]int64{{7182, 155, 8880, 14382}, {9951, 404, 11637, 12143}, {11746, 89, 12176, 6126}, {4069, 155, 7989, 24324}, {8326, 165, 9163, 6726}, {9984, 77, 10405, 6378}, {10709, 373, 11738, 8403}, {7765, 53, 8643, 8452}, {9464, 77, 10142, 6770}, {10446, 218, 11029, 7059}, {11441, 81, 11815, 4607}, {12636, 25, 15294, 18408}, {15587, 132, 16537, 9448}, {16847, 88, 17501, 7096}, {17853, 746, 20612, 60374}, {8885, 154, 18723, 59765}, {19158, 88, 21078, 13769}, {21430, 551, 22856, 25481}}
	var newInput, newOutput, newTotal int64
	byOperation := map[string]map[string]any{}
	runs := map[string]bool{}
	for i, item := range rows {
		r, ok := item.(map[string]any)
		s, sqlOK := sql[i].(map[string]any)
		if !ok || !sqlOK {
			return errNativeFixtureDenied
		}
		canon, err := releaseChainV6CanonicalRow(s)
		if err != nil || !reflect.DeepEqual(canon, r) {
			return errNativeFixtureDenied
		}
		ordinal := 558 + i
		kind := "submit"
		if i == 0 || i == 3 || i == 7 || i == 15 {
			kind = "start"
		}
		complete := float64(0)
		var finish any
		if i == 2 || i == 6 || i == 14 || i == 17 {
			complete = 1
			finish = "STOP"
		}
		want := map[string]any{"ordinal": float64(ordinal), "kind": kind, "is_parent": !(i >= 3 && i <= 6), "scenario": "normal_sent", "outcome": "completed", "provider_operation_stage": "response_stream", "model_version": "gemini-3.8-flash-n", "failure_stage": nil, "diagnostic_code": nil, "finish_reason": finish, "actual_input_tokens": float64(expected[i][0]), "actual_output_tokens": float64(expected[i][1]), "actual_total_tokens": float64(expected[i][2]), "elapsed_ms": float64(expected[i][3]), "completed_count": complete, "final_count": complete, "terminal_count": complete, "stop_final_count": complete, "cancelled_count": float64(0), "timed_out_count": float64(0), "transport_error_count": float64(0), "max_tokens_final_count": float64(0), "usage_observation_uncertain": false, "version_observation_uncertain": false, "finish_observation_uncertain": false}
		if !releaseChainV6Exact(r, want) {
			return errNativeFixtureDenied
		}
		u := h.Usage[557+i]
		if u.RunID != r["run_id"] || u.ReservationID != fmt.Sprintf("release-chain-v11-native-%d", ordinal) || u.ActualInputTokens == nil || u.ActualOutputTokens == nil || *u.ActualInputTokens != expected[i][0] || *u.ActualOutputTokens != expected[i][1] {
			return errNativeFixtureDenied
		}
		op, opOK := r["kernel_operation_id"].(string)
		run, runOK := r["run_id"].(string)
		if !opOK || !runOK || op == "" || byOperation[op] != nil {
			return errNativeFixtureDenied
		}
		byOperation[op] = r
		runs[run] = true
		newInput += expected[i][0]
		newOutput += expected[i][1]
		newTotal += expected[i][2]
	}
	if len(runs) != 4 || newInput != 213479 || newOutput != 3631 || newTotal != 246218 {
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
	result := releaseChainV6Object(fin, "original_result")
	if !releaseChainV6Exact(fin, map[string]any{"test_failed": true, "all_channel_requests": float64(0), "recipient_acceptances": float64(0)}) || !releaseChainV6Exact(initial, map[string]any{"preexisting_case_native_requests": float64(0), "preexisting_root_model_reservations": float64(0), "existing_target_versions": float64(0), "existing_target_catalogs": float64(0)}) || !releaseChainV6Exact(result, map[string]any{"patch_count": float64(1), "prepare_receipt_count": float64(1), "check_receipt_count": float64(2), "check_tool_call_id": nil, "check_receipt_artifact_id": nil, "checks_passed": nil, "applied": nil}) {
		return errNativeFixtureDenied
	}
	counts := map[string]int{"runs": 4, "tasks": 2, "accepted_inputs": 4, "actions": 14, "dispatches": 14, "authorizations": 14, "final_results": 4, "developer_admissions": 1, "converter_returns": 1, "converter_versions": 1, "converter_catalog": 1, "converter_validations": 2, "converter_trials": 1, "converter_usages": 1, "notices": 0, "model_budget_attempts": 18, "budget_scopes": 8, "budget_reservations": 38}
	for table, count := range counts {
		if len(releaseChainV6Array(db, table)) != count {
			return errNativeFixtureDenied
		}
	}
	for _, table := range []string{"runs", "accepted_inputs"} {
		for _, item := range releaseChainV6Array(db, table) {
			r, ok := item.(map[string]any)
			if !ok || r["status"] != "completed" {
				return errNativeFixtureDenied
			}
			if table == "accepted_inputs" && !releaseChainV6Exact(r, map[string]any{"attempts": float64(1), "error_code": "", "model_retry": nil}) {
				return errNativeFixtureDenied
			}
		}
	}
	for _, item := range releaseChainV6Array(db, "tasks") {
		r, ok := item.(map[string]any)
		if !ok || r["status"] != "active" {
			return errNativeFixtureDenied
		}
	}
	seenOperations := map[string]bool{}
	for _, item := range releaseChainV6Array(db, "model_budget_attempts") {
		op, ok := item.(map[string]any)
		if !ok {
			return errNativeFixtureDenied
		}
		id, _ := op["operation_id"].(string)
		r := byOperation[id]
		if r == nil || seenOperations[id] || !releaseChainV6Exact(op, map[string]any{"run_id": r["run_id"], "actual_input_tokens": r["actual_input_tokens"], "actual_output_tokens": r["actual_output_tokens"]}) || op["observed_at"] == nil {
			return errNativeFixtureDenied
		}
		seenOperations[id] = true
	}
	tools := map[string]int{}
	for _, item := range releaseChainV6Array(db, "actions") {
		r, ok := item.(map[string]any)
		if !ok || !releaseChainV6Exact(r, map[string]any{"state": "effect_confirmed", "dispatch_outcome": "dispatch_completed"}) {
			return errNativeFixtureDenied
		}
		name, _ := r["tool_name"].(string)
		tools[name]++
	}
	if tools["extension.project.check"] != 2 || tools["extension.project.prepare-patch"] != 1 || tools["converter.developer-inventory.convert"] != 1 {
		return errNativeFixtureDenied
	}
	if !releaseChainV6Exact(v.Proposals, map[string]any{"status": "INDEPENDENT_FOURTEEN_ORIGINAL_PROPOSALS_CORRELATED_CHAIN_NO_GO", "confirmed_actions": float64(14), "all_fourteen_core_action_fingerprints_match": true, "actual_both_read_receipt_booleans_retained": false, "same_immutable_patch_request_target": true, "legacy_original_result_nulls_preserved": true, "duplicate_writes_or_mutation_replays_observed": false, "verified_scenarios": float64(0), "issue33_ready": false}) || len(releaseChainV6Array(v.Proposals, "proposals")) != 14 || len(releaseChainV6Array(v.Proposals, "two_project_check_read_requests")) != 2 {
		return errNativeFixtureDenied
	}
	for _, item := range releaseChainV6Array(v.Proposals, "proposals") {
		r, ok := item.(map[string]any)
		if !ok || !releaseChainV6Exact(r, map[string]any{"original_bytes_and_base64_match": true, "original_authorization_hash_matches_metadata": true, "actual_core_authorized_action_match": true, "actual_action_state": "effect_confirmed", "model_argument_values_emitted": false}) {
			return errNativeFixtureDenied
		}
	}
	ob := releaseChainV6Object(fin, "source_check_observation")
	if !releaseChainV6Exact(ob, map[string]any{"version": "butler-source-check-observation-v1", "dropped_failed_checks": float64(0)}) || len(releaseChainV6Array(ob, "failed_checks")) != 2 {
		return errNativeFixtureDenied
	}
	for _, item := range releaseChainV6Array(ob, "failed_checks") {
		r, ok := item.(map[string]any)
		if !ok || !releaseChainV6Exact(r, map[string]any{"role": "task_authority_watchdog", "failure_category": "cancelled", "maximum_check_ms": float64(750), "kernel_operation_id": nil, "acquisition_elapsed_ms": float64(0)}) || (r["elapsed_ms"] != float64(0) && r["elapsed_ms"] != float64(2)) {
			return errNativeFixtureDenied
		}
	}
	return nil
}

func readReleaseChainV12ActualHistory() (nativeHistoricalEvidence, error) {
	prior, err := readReleaseChainV11ActualHistory()
	if err != nil {
		return nativeHistoricalEvidence{}, errNativeFixtureDenied
	}
	v, err := readReleaseChainV12HistoryObjects()
	if err != nil || validateReleaseChainV12History(prior, v) != nil {
		return nativeHistoricalEvidence{}, errNativeFixtureDenied
	}
	return v.History, nil
}

func TestReleaseChainV12ActualHistoryPreserves575(t *testing.T) {
	h, err := readReleaseChainV12ActualHistory()
	if err != nil || h.NativeAttempts != 575 || h.KnownUsagePairs != 546 || h.UnknownUsagePairs != 29 || h.Usage[556].ActualInputTokens != nil || h.Usage[557].ActualInputTokens == nil {
		t.Fatal("closed575 or historical557 changed", err)
	}
}

func TestReleaseChainV12HistoryRejectsTampering(t *testing.T) {
	prior, err := readReleaseChainV11ActualHistory()
	if err != nil {
		t.Fatal(err)
	}
	original, err := readReleaseChainV12HistoryObjects()
	if err != nil {
		t.Fatal(err)
	}
	for _, kind := range []string{"old_usage_changed", "new_measured_cost_changed", "legacy_read_receipt_filled", "retry_invented", "no_go_promoted", "old_ceiling_refunded", "cleanup_called_source_timeout", "original_proposal_match_removed", "kernel_operation_duplicated"} {
		t.Run(kind, func(t *testing.T) {
			raw, _ := json.Marshal(original)
			var v releaseChainV12HistoryObjects
			if json.Unmarshal(raw, &v) != nil {
				t.Fatal("clone failed")
			}
			switch kind {
			case "old_usage_changed":
				x := int64(1)
				v.History.Usage[536].ActualInputTokens = &x
				v.History.Usage[536].ActualOutputTokens = &x
			case "new_measured_cost_changed":
				x := int64(7183)
				v.History.Usage[557].ActualInputTokens = &x
			case "legacy_read_receipt_filled":
				releaseChainV6Object(releaseChainV6Object(v.Product[0], "payload_json"), "original_result")["checks_passed"] = true
			case "retry_invented":
				v.Evidence["failure-correlation"]["accepted_model_retry_failures"] = float64(1)
			case "no_go_promoted":
				releaseChainV6Object(v.Evidence["final-audit"], "chain_audit")["status"] = "PASS"
			case "old_ceiling_refunded":
				v.Bridge["remaining_original_absolute628_attempts"] = float64(71)
			case "cleanup_called_source_timeout":
				releaseChainV6Object(v.Evidence["failure-correlation"], "failure_classification")["source_check_or_provider_timeout_observed"] = true
			case "original_proposal_match_removed":
				v.Proposals["all_fourteen_core_action_fingerprints_match"] = false
			case "kernel_operation_duplicated":
				db := releaseChainV6Object(releaseChainV6Object(v.Product[0], "payload_json"), "postgres")
				ops := releaseChainV6Array(db, "model_budget_attempts")
				ops[1] = ops[0]
			}
			// A projection writer can persist internally consistent hashes. Exercise
			// the semantic NULL and unique-operation boundaries beyond checksum rejection.
			v.Product[0]["payload_sha256"] = shadowV4Hash(v.Product[0]["payload_json"])
			if validateReleaseChainV12History(prior, v) == nil {
				t.Fatal("tampered historical575 accepted", kind)
			}
		})
	}
}
