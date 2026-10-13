package main

import "reflect"

const expenseExceptionSHA = "31b930ac4e39206d65424ac951f65cc4656b71166995650122cbdda3e63b6ab4"
const expenseExceptionReply = "Разрешить UNKNOWN только для закрытого сбоя №537; новый прогон должен иметь полный измеренный расход."
const priorProtocolSHA = "9ce27e6d9a15f598fadc037e5de377869672937fef84849fc662610ec6379de3"

func verifyV6Prerequisite(p object) {
	keys(p, "version baseline_attempts maximum_new_attempts maximum_total_attempts prior_history_sha256 retained_quality_prerequisite_sha256 retained_quality_final_audit_sha256 failed_v5_audit_sha256 failed_v5_correlation_sha256 human_expense_exception_sha256 historical_known_input_tokens_subtotal historical_known_output_tokens_subtotal historical_known_total_tokens_subtotal historical_unknown_total_attempts historical_known_usage_pairs historical_unknown_usage_pairs provider_stage_observation all_new_expense_must_be_measured maximum_generation_tokens requested_model retry_policy maximum_model_retries_per_input model_retry_backoff_ms capture_kernel_operation_id prior_protocol_sha256 human_safe_timeout_waiver_sha256 allow_unmeasured_safe_timeout_retry all_successful_expense_must_be_measured")
	require(p["version"] == "butler-chain-high-reviewed-v6-prerequisite-v2" && p["prior_protocol_sha256"] == priorProtocolSHA && p["human_safe_timeout_waiver_sha256"] == safeTimeoutWaiverSHA && p["allow_unmeasured_safe_timeout_retry"] == true && p["all_new_expense_must_be_measured"] == false && p["all_successful_expense_must_be_measured"] == true, "v2_exact_narrow_human_waiver_only")
	prior := obj(clone(p))
	for _, key := range []string{"prior_protocol_sha256", "human_safe_timeout_waiver_sha256", "allow_unmeasured_safe_timeout_retry", "all_successful_expense_must_be_measured"} {
		delete(prior, key)
	}
	prior["version"], prior["all_new_expense_must_be_measured"] = "butler-chain-high-reviewed-v6-prerequisite-v1", true
	verifyPriorV6Protocol(prior)
}

func verifyPriorV6Protocol(p object) {
	keys(p, "version baseline_attempts maximum_new_attempts maximum_total_attempts prior_history_sha256 retained_quality_prerequisite_sha256 retained_quality_final_audit_sha256 failed_v5_audit_sha256 failed_v5_correlation_sha256 human_expense_exception_sha256 historical_known_input_tokens_subtotal historical_known_output_tokens_subtotal historical_known_total_tokens_subtotal historical_unknown_total_attempts historical_known_usage_pairs historical_unknown_usage_pairs provider_stage_observation all_new_expense_must_be_measured maximum_generation_tokens requested_model retry_policy maximum_model_retries_per_input model_retry_backoff_ms capture_kernel_operation_id")
	require(p["version"] == "butler-chain-high-reviewed-v6-prerequisite-v1" && integer(p["baseline_attempts"]) == 537 && integer(p["maximum_new_attempts"]) == 78 && integer(p["maximum_total_attempts"]) == 615 && p["prior_history_sha256"] == priorSHA, "v6_actual_history_and_remaining_cap")
	require(p["retained_quality_prerequisite_sha256"] == "9480184b2f5586d8f9826b8db328c52a845ee812bd48afbf1432ca7f30bfd5d3" && p["retained_quality_final_audit_sha256"] == qualitySHA && p["failed_v5_audit_sha256"] == "592d27ee34178a9b336eb0f9cce4784fac1ba214b22f05e1ad0b247a48f97c04" && p["failed_v5_correlation_sha256"] == "fb8a430c327c4abec5984cad6fec6e79aa57d36866500dd588979722f282a424" && p["human_expense_exception_sha256"] == expenseExceptionSHA, "v6_retained_quality_failure_and_human_scope")
	require(integer(p["historical_known_input_tokens_subtotal"]) == 294598 && integer(p["historical_known_output_tokens_subtotal"]) == 11827 && integer(p["historical_known_total_tokens_subtotal"]) == 572459 && integer(p["historical_unknown_total_attempts"]) == 47 && integer(p["historical_known_usage_pairs"]) == 510 && integer(p["historical_unknown_usage_pairs"]) == 27, "v6_known_subtotals_preserve_all_unknowns")
	require(p["provider_stage_observation"] == true && p["all_new_expense_must_be_measured"] == true && integer(p["maximum_generation_tokens"]) == 32768 && p["requested_model"] == "gemini-3.8-flash-high", "v6_other_fixed_criteria")
	require(p["retry_policy"] == "one_nominal_timeout_per_accepted_input_v1" && integer(p["maximum_model_retries_per_input"]) == 1 && integer(p["model_retry_backoff_ms"]) == 2000 && p["capture_kernel_operation_id"] == true, "v6_prospective_retry_policy")
}

// The actual trusted human reply changes one expense gate only. Its exact raw
// identity is separately required at the operator boundary. This is no signature
// or authority to a tool/model, and grants no exception to a later request.
func verifyExpenseException(value any) {
	waiver := obj(value)
	keys(waiver, "version source reply_to user_reply allowed_closed_ordinal allowed_unknown_fields retain_original_null_usage retain_closed_chain_no_go new_after_537_all_attempts_require_complete_measured_usage future_unknown_expense_exceptions_authorized other_quality_release_criteria_changed historical_266_cause prior_total_unknown_dimensions prior_known_total_tokens_subtotal remaining_operator_protocol_attempt_cap absolute_operator_protocol_history_ceiling independent_native_admission_still_required issue33_ready limitations")
	require(waiver["version"] == "butler-chain-v6-human-expense-exception-v1" && waiver["source"] == "explicit_user_reply_in_current_task" && waiver["reply_to"] == "request_user_input_async:call_48746905f5834b09be7b567fd2531f0d" && waiver["user_reply"] == expenseExceptionReply, "exact_trusted_human_expense_reply")
	require(integer(waiver["allowed_closed_ordinal"]) == 537 && reflect.DeepEqual(arr(waiver["allowed_unknown_fields"]), []any{"input_tokens", "output_tokens", "total_tokens"}) && waiver["retain_original_null_usage"] == true && waiver["retain_closed_chain_no_go"] == true && waiver["new_after_537_all_attempts_require_complete_measured_usage"] == true && waiver["future_unknown_expense_exceptions_authorized"] == false && waiver["other_quality_release_criteria_changed"] == false, "single_closed_expense_exception_only")
	require(waiver["historical_266_cause"] == "UNKNOWN" && integer(waiver["prior_total_unknown_dimensions"]) == 47 && integer(waiver["prior_known_total_tokens_subtotal"]) == 572459 && integer(waiver["remaining_operator_protocol_attempt_cap"]) == 78 && integer(waiver["absolute_operator_protocol_history_ceiling"]) == 615 && waiver["independent_native_admission_still_required"] == true && waiver["issue33_ready"] == false, "expense_exception_preserves_history_and_other_gates")
	require(reflect.DeepEqual(arr(waiver["limitations"]), []any{"This records the trusted human reply; it is not an external signature or administrative isolation proof.", "The exception does not recover actual expense, change the failed V5 outcome or claim complete historical expense.", "Existing pre-task unknowns remain unknown; no new attempt after537 receives an unknown-expense exception."}), "expense_provenance_limits")
}
