package main

import "reflect"

const v11PrerequisiteSHA = "9f57fb1c613ffcd86f5f2b19a9cdd2b17c92f2c726723551fd9fbd44b8957888"
const v11HumanSHA = "31a2381c9f23f1ceeb9687f87bcb705c484433617491b8ec30d1173988bbd5f8"
const v11HistoricalExceptionSHA = "e2ebb0c52c406f21c1120b78c90aa9e3181d83c0875411a07738fc084286297d"
const v11PriorSHA = "fb4b085728544f0709dbc58dc7d5a3ba678412cc6a0c96514363a6fab2c27dee"
const v11HumanSemantic = `{
  "version": "butler-chain-v11-human-policy-amendment-v1",
  "source": "direct_user_messages_in_current_task",
  "user_messages": [
    "Хорошо тогда чутьчуть ослабь требования и если прям критичных проблем не останется тогда можно закрывать пр",
    "Ты можешь не делать сотни проверок перед проверкой на модели HIGH. У меня подписка и денег не списывается, потратить больше чем дано нельзя, смело тестируй модель",
    "Так может убрать/увеличить таймаут, если модель думает, то пускай, какбудто лучше чтобы агент выполнил задачу чем сказал что не сможет. Или ты считаешь что слишком долго на такие задачи он тратит времени?"
  ],
  "interpretation": "Continue the finite full High chain from factual557 with fixture-only, value-free observations of the unchanged source, lease and task authority checks. Preserve the original 2000ms source and 750ms watchdog bounds and every release/Stop/dedupe/original-result guard. Retain closed557 expense as historical UNKNOWN only under the separately pinned narrow interpretation of the later user relaxation. Fresh completed attempts still require fully measured usage and selected version; source uncertainty never grants model retry or actions. New557+71 ceiling628 is prospective and does not reopen earlier615 or627 windows.",
  "window_id": "issue33-high-release-chain-v11-20261013-baseline557",
  "baseline_attempts": 557,
  "maximum_new_attempts": 71,
  "maximum_total_attempts": 628,
  "maximum_generation_tokens": 32768,
  "maximum_attempt_ms": 300000,
  "maximum_case_ms": 2400000,
  "maximum_global_ms": 7200000,
  "maximum_child_model_turns": 4,
  "maximum_child_external_calls": 4,
  "maximum_child_active_ms": 1320000,
  "maximum_model_retries_per_input": 1,
  "model_retry_backoff_ms": 2000,
  "safe_timeout_stages": [
    "http_round_trip",
    "response_stream"
  ],
  "keep_closed_537_and_539_unknown": true,
  "all_completed_entries_require_measured_usage_and_selected_version": true,
  "failed_unknown_requires_proved_safe_same_run_recovery": true,
  "critical_safety_or_original_result_criteria_changed": false,
  "quality_or_comparison_criteria_changed": false,
  "release_chain_latency_policy_changed": true,
  "independent_acceptance_required_before_ready_merge_closure": true,
  "native_window_reopened": false,
  "issue33_ready": false,
  "limitations": [
    "This records the direct messages and the assistant's bounded operational choices; it is not a cryptographic human signature.",
    "The new deadline does not establish that the model was thinking during an unanswered HTTP request.",
    "Subscription-no-overage pricing does not supply missing token measurements or provider-side billing and runtime records."
  ],
  "proposal_retention_required": true,
  "maximum_private_proposal_bytes": 262144,
  "original_model_arguments_and_authorization_fingerprints_preserved": true,
  "provider_entries_serialized": true,
  "maximum_concurrent_native_entries": 1,
  "maximum_extension_harness_lifetime_ms": 2400000,
  "extension_harness_lifetime_source": "remaining_case_deadline",
  "extension_harness_lifetime_test_only": true,
  "legacy_extension_harness_lifetime_ms": 180000,
  "old_sealed_absolute_ceiling": 627,
  "old_closed_attempts_or_unknowns_refunded": false,
  "new_scope_allocation_is_prospective": true,
  "historical557_expense_exception_sha256": "e2ebb0c52c406f21c1120b78c90aa9e3181d83c0875411a07738fc084286297d",
  "source_check_observer_enabled": true,
  "source_check_failure_observation_schema": "butler-source-check-observation-v1",
  "source_check_observation_max_failed_checks": 64,
  "source_check_observation_contains_query_or_argument_values": false,
  "source_check_timeout_ms": 2000,
  "authority_watchdog_check_timeout_ms": 750,
  "source_check_or_watchdog_bounds_changed": false,
  "production_source_check_or_authority_behavior_changed": false
}`

const v11PrerequisiteSemantic = `{
  "version": "butler-chain-high-reviewed-v11-prerequisite-v1",
  "window_id": "issue33-high-release-chain-v11-20261013-baseline557",
  "baseline_attempts": 557,
  "maximum_new_attempts": 71,
  "maximum_total_attempts": 628,
  "prior_history_artifact_sha256": "fb85f5ce35a5217990d83af664060908574c0ac0682fa8d111e20ac0c2deef0f",
  "prior_history_sha256": "fb4b085728544f0709dbc58dc7d5a3ba678412cc6a0c96514363a6fab2c27dee",
  "retained_quality_prerequisite_sha256": "9480184b2f5586d8f9826b8db328c52a845ee812bd48afbf1432ca7f30bfd5d3",
  "retained_quality_final_audit_sha256": "f8248d19ec2a0c222b39d46fbc46c0d0cf4ccc3c1361c89965b597eedbb21cab",
  "prior_v6_prerequisite_sha256": "bef93bde0afa1251c67eb60f01dbbe6f8142b4f32b5a8076c0f34d5d9b402077",
  "prior_v6_timeout_waiver_sha256": "44991353526966ab57cfd0155c3f65af00061b87308eed37c2b735f207e75180",
  "historical_known_input_tokens_subtotal": 475637,
  "historical_known_output_tokens_subtotal": 15159,
  "historical_known_total_tokens_subtotal": 793703,
  "historical_unknown_total_attempts": 49,
  "historical_known_usage_pairs": 528,
  "historical_unknown_usage_pairs": 29,
  "provider_stage_observation": true,
  "capture_kernel_operation_id": true,
  "requested_model": "gemini-3.8-flash-high",
  "observed_successful_model_version": "gemini-3.8-flash-n",
  "maximum_generation_tokens": 32768,
  "maximum_attempt_ms": 300000,
  "maximum_case_ms": 2400000,
  "maximum_global_ms": 7200000,
  "allowed_provider_operation_stages": [
    "http_round_trip",
    "response_stream"
  ],
  "maximum_model_retries_per_input": 1,
  "model_retry_backoff_ms": 2000,
  "all_completed_entries_require_measured_usage_and_selected_version": true,
  "failed_usage_shape": "valid_all_three_known_or_all_three_null",
  "failed_model_version_shape": "selected_version_or_null",
  "all_other_quality_source_authority_stop_effect_delivery_gates_retained": true,
  "issue33_ready": false,
  "human_policy_amendment_sha256": "31a2381c9f23f1ceeb9687f87bcb705c484433617491b8ec30d1173988bbd5f8",
  "proposal_retention_required": true,
  "maximum_private_proposal_bytes": 262144,
  "preserve_original_proposal_bytes_and_authorization_fingerprint": true,
  "actual544_bridge_sha256": "ca18f5d5f3a78ac9d61ada16472399ecc3024801b8b077795fabbe3db9584650",
  "failed_v8_audit_sha256": "5573b9f25e13a4f3aefacacf142aeda7846b68962f8cd26df4498417097c580f",
  "failed_v8_correlation_sha256": "2e1eed8eadb5874299693bd28ea9857fbd6d96ce9a2e1865009f2368403761d3",
  "prior_v8_prerequisite_sha256": "effa65a6e39fcecce2510c76554685d047ee5e8c7ed1b9e6d27e633133d402ec",
  "prior_v8_authority_sha256": "3560bb090a6b2c8b8bc78a987b2331eaf2eb55550b5e62572fbaf0bb7646973d",
  "retained_closed_v8_proposal_diagnostic_sha256": "8d5b296bfacd45da1d578896435378e7151787ee4847f26d6953267da3af87d8",
  "provider_entries_serialized": true,
  "maximum_concurrent_native_entries": 1,
  "actual556_bridge_sha256": "70041e9a3275a2099d39eda98d4535420eea7bfdae1bb3eb34bd58662f51ae3b",
  "failed_v9_audit_sha256": "731cf102a25d42a091441ea045cedb5964c66ebb305890c564cc3b5b9713ad83",
  "failed_v9_correlation_sha256": "b96457cbb16bfbc89cb3fb32c62a3d9622939efb9fbe574eb97253c4f89f9b6c",
  "prior_v9_prerequisite_sha256": "4f753010b86b57ff2ea4cc8ff698d56d23787ab1ec20b572cb35ad329b1802c3",
  "prior_v9_authority_sha256": "83ca7491e4fc4a3673097f62f9590117e42387daadc656935318b5bd6caf4ebe",
  "retained_closed_v9_proposal_diagnostic_sha256": "8b5c8683e1aa23e49c15809e5d12432df3402ad01365a2e5a1567ed6f55350fb",
  "maximum_extension_harness_lifetime_ms": 2400000,
  "extension_harness_lifetime_source": "remaining_case_deadline",
  "extension_harness_lifetime_test_only": true,
  "legacy_extension_harness_lifetime_ms": 180000,
  "old_sealed_absolute_ceiling": 627,
  "old_closed_attempts_or_unknowns_refunded": false,
  "new_scope_allocation_is_prospective": true,
  "actual557_bridge_sha256": "78d320c595483d666b25cbaee55be764b53b0004bdd6806ecb4924e4f01130ef",
  "failed_v10_audit_sha256": "28b4dbc9cdfbd8fb8e838b2f14bb698b7142e587f0a44ec2e534a716dce62534",
  "failed_v10_correlation_sha256": "f7c20a7246eb2fc5a653f39f4bfe7f06498e152be3d50c4b523357eff163af8d",
  "prior_v10_prerequisite_sha256": "193e3b8c21b00ffdda62ea0dddb7ecf17357f912d7c78e0d349873e92ee63b86",
  "prior_v10_authority_sha256": "417dfd7bf7e9f0fba535d18a50552696f8efd08a9bf19b2f4a54d4cfb922da1d",
  "historical557_expense_exception_sha256": "e2ebb0c52c406f21c1120b78c90aa9e3181d83c0875411a07738fc084286297d",
  "source_check_observer_enabled": true,
  "source_check_failure_observation_schema": "butler-source-check-observation-v1",
  "source_check_observation_max_failed_checks": 64,
  "source_check_observation_contains_query_or_argument_values": false,
  "source_check_timeout_ms": 2000,
  "authority_watchdog_check_timeout_ms": 750,
  "source_check_or_watchdog_bounds_changed": false,
  "production_source_check_or_authority_behavior_changed": false
}`

const v11HistoricalExceptionSemantic = `{
  "version": "butler-chain-v11-historical557-expense-exception-v1",
  "source": "direct_later_user_messages_and_explicit_bounded_assistant_interpretation",
  "user_messages": [
    "Хорошо тогда чутьчуть ослабь требования и если прям критичных проблем не останется тогда можно закрывать пр",
    "Ты можешь не делать сотни проверок перед проверкой на модели HIGH. У меня подписка и денег не списывается, потратить больше чем дано нельзя, смело тестируй модель",
    "Так может убрать/увеличить таймаут, если модель думает, то пускай, какбудто лучше чтобы агент выполнил задачу чем сказал что не сможет. Или ты считаешь что слишком долго на такие задачи он тратит времени?"
  ],
  "interpretation": "The later instruction to modestly relax requirements conditional on no critical problems permits retaining the unavailable expense of the sealed source-check cancellation557 as historical UNKNOWN. This does not establish a nominal provider timeout, permit model retry after source uncertainty, alter any failed report or exempt a fresh completed attempt from measured expense.",
  "allowed_historical_ordinals": [
    557
  ],
  "historical_window_id": "issue33-high-release-chain-v10-20261013-baseline556",
  "exact_failure_audit_sha256": "28b4dbc9cdfbd8fb8e838b2f14bb698b7142e587f0a44ec2e534a716dce62534",
  "exact_failure_correlation_sha256": "f7c20a7246eb2fc5a653f39f4bfe7f06498e152be3d50c4b523357eff163af8d",
  "exact_history_artifact_sha256": "fb85f5ce35a5217990d83af664060908574c0ac0682fa8d111e20ac0c2deef0f",
  "core_failure_reason": "memory_source_check_timeout",
  "underlying_source_check_cause": "UNKNOWN",
  "input_tokens": null,
  "output_tokens": null,
  "total_tokens": null,
  "model_version": null,
  "historical_failure_remains_no_go": true,
  "original_measurements_or_states_changed": false,
  "refunds_or_zero_cost_claims_allowed": false,
  "applies_to_future_attempts": false,
  "nominal_provider_timeout_established": false,
  "source_uncertain_model_retry_or_effects_allowed": false,
  "independent_current_source_and_all_five_original_result_gates_retained": true,
  "literal_new_human557_signature_claimed": false,
  "issue33_ready": false
}`

func v11AuditPolicy() auditPolicy {
	p := v10AuditPolicy()
	p.Baseline, p.Maximum, p.DayModelTurns = 557, 71, 71
	p.UnknownTotal = 49
	p.WindowID = "issue33-high-release-chain-v11-20261013-baseline557"
	p.PriorSHA, p.PrerequisiteSHA, p.HumanSHA = v11PriorSHA, v11PrerequisiteSHA, v11HumanSHA
	p.PassStatus = "CHAIN_V11_WITNESS_AUDIT_PASS"
	p.RequireSourceObservation = true
	p.VerifyHuman = func(in inputs) {
		verifyExpenseException(in.ExpenseException)
		require(reflect.DeepEqual(obj(in.SafeTimeoutWaiver), frozenObject(v11HumanSemantic)), "v11_exact_human_amendment")
	}
	p.VerifyPrerequisite = func(o object) {
		require(reflect.DeepEqual(o, frozenObject(v11PrerequisiteSemantic)), "v11_exact_prerequisite")
	}
	return p
}
func verifyV11HistoricalException(value any) {
	require(reflect.DeepEqual(obj(value), frozenObject(v11HistoricalExceptionSemantic)), "v11_exact_historical557_exception")
}
