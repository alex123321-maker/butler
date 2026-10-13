package main

import (
	"path/filepath"
	"reflect"
)

const safeTimeoutWaiverSHA = "44991353526966ab57cfd0155c3f65af00061b87308eed37c2b735f207e75180"
const safeTimeoutWaiverReply = "Хорошо тогда чутьчуть ослабь требования и если прям критичных проблем не останется тогда можно закрывать пр"

// This is the human's bounded prospective interpretation, not provider, model
// or tool authority. Missing bytes remain unknown in every retained accounting.
func verifySafeTimeoutWaiver(value any) {
	w := obj(value)
	keys(w, "version source user_reply window_id baseline_attempts prior_protocol_sha256 retained_537_exception_sha256 expense_exception_policy allowed_failed_outcomes required_diagnostic_code required_provider_operation_stage failed_usage_shape failed_model_version_shape retain_original_null_measurements retain_historical_no_go maximum_model_retries_per_input model_retry_backoff_ms require_exact_kernel_operation_and_spent_reservations require_started_retry_and_fresh_completed_start require_fixed_scenario_result_or_owner_stop_guarantees all_completed_entries_require_measured_usage_and_selected_version other_new_unknown_measurements_allowed other_safety_or_quality_criteria_changed maximum_new_attempts maximum_total_attempts maximum_attempt_ms maximum_case_ms maximum_global_ms independent_native_admission_still_required issue33_ready limitations")
	require(w["version"] == "butler-chain-v6-human-safe-timeout-waiver-v1" && w["source"] == "explicit_user_reply_in_current_task" && w["user_reply"] == safeTimeoutWaiverReply, "exact_prospective_human_timeout_reply")
	require(w["window_id"] == "issue33-high-release-chain-v6-20261013-baseline537" && integer(w["baseline_attempts"]) == 537 && w["prior_protocol_sha256"] == priorProtocolSHA && w["retained_537_exception_sha256"] == expenseExceptionSHA, "timeout_waiver_original_protocol_and_history")
	require(w["expense_exception_policy"] == "one_proved_nominal_response_stream_timeout_retry_v1" && reflect.DeepEqual(arr(w["allowed_failed_outcomes"]), []any{"timed_out", "transport_error"}) && w["required_diagnostic_code"] == "provider_timeout" && w["required_provider_operation_stage"] == "response_stream" && w["failed_usage_shape"] == "valid_all_three_known_or_all_three_null" && w["failed_model_version_shape"] == "selected_version_or_null", "only_proved_stream_timeout_nullable_contract")
	for _, key := range []string{"retain_original_null_measurements", "retain_historical_no_go", "require_exact_kernel_operation_and_spent_reservations", "require_started_retry_and_fresh_completed_start", "require_fixed_scenario_result_or_owner_stop_guarantees", "all_completed_entries_require_measured_usage_and_selected_version", "independent_native_admission_still_required"} {
		require(w[key] == true, "timeout_waiver_safety_guarantees_unchanged")
	}
	require(w["other_new_unknown_measurements_allowed"] == false && w["other_safety_or_quality_criteria_changed"] == false && w["issue33_ready"] == false, "timeout_waiver_no_generic_unknown_or_release")
	require(integer(w["maximum_model_retries_per_input"]) == 1 && integer(w["model_retry_backoff_ms"]) == 2000 && integer(w["maximum_new_attempts"]) == 78 && integer(w["maximum_total_attempts"]) == 615 && integer(w["maximum_attempt_ms"]) == 120000 && integer(w["maximum_case_ms"]) == 1200000 && integer(w["maximum_global_ms"]) == 3600000, "timeout_waiver_resource_limits_unchanged")
	require(reflect.DeepEqual(arr(w["limitations"]), []any{"This records the trusted human reply and its narrow prospective interpretation; it is not an external signature or administrative isolation proof.", "Missing failed-request expense or version remains unknown, separately counted and never inferred from a retry or successful result.", "The sealed V5 NO_GO, original 537 exception and every safety, result, quality and resource criterion remain unchanged."}), "timeout_waiver_provenance_limits")
}

func verifyPriorV6ProtocolSource(root string) {
	raw, err := readPinnedPublic(filepath.Join(root, "deploy", "acceptance", "release-chain-v6-high-prerequisite-v1-20261013.json"), priorProtocolSHA)
	require(err == nil, "original_v6_protocol_bytes_preserved")
	parsed, err := decode(raw)
	require(err == nil, "original_v6_protocol_decode")
	verifyPriorV6Protocol(obj(parsed))
}
