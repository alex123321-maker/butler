package main

import "reflect"

const v9PrerequisiteSHA = "4f753010b86b57ff2ea4cc8ff698d56d23787ab1ec20b572cb35ad329b1802c3"
const v9HumanSHA = "83ca7491e4fc4a3673097f62f9590117e42387daadc656935318b5bd6caf4ebe"
const v9PriorSHA = "7e397ba703ad12b37daacfd5406a57a93ed58237ca6ed6823932c788032fa8b2"
const v9HumanSemantic = `{
  "version": "butler-chain-v9-human-policy-amendment-v1",
  "source": "direct_user_messages_in_current_task",
  "user_messages": [
    "Хорошо тогда чутьчуть ослабь требования и если прям критичных проблем не останется тогда можно закрывать пр",
    "Ты можешь не делать сотни проверок перед проверкой на модели HIGH. У меня подписка и денег не списывается, потратить больше чем дано нельзя, смело тестируй модель",
    "Так может убрать/увеличить таймаут, если модель думает, то пускай, какбудто лучше чтобы агент выполнил задачу чем сказал что не сможет. Или ты считаешь что слишком долго на такие задачи он тратит времени?"
  ],
  "interpretation": "Continue the bounded native High chain from factual544. Advertise collection only after an actual current tested child return, retain the independent Broker authorization checks and original parent completion gates, and serialize parent and child provider entries in the isolated observer without holding database transactions during waiting or inference. Preserve every closed failure, unknown measurement, original proposal and safety guard.",
  "window_id": "issue33-high-release-chain-v9-20261013-baseline544",
  "baseline_attempts": 544,
  "maximum_new_attempts": 71,
  "maximum_total_attempts": 615,
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
  "maximum_concurrent_native_entries": 1
}`
const v9PrerequisiteSemantic = `{
  "version": "butler-chain-high-reviewed-v9-prerequisite-v1",
  "window_id": "issue33-high-release-chain-v9-20261013-baseline544",
  "baseline_attempts": 544,
  "maximum_new_attempts": 71,
  "maximum_total_attempts": 615,
  "prior_history_artifact_sha256": "bf6ac2a9e0115a3a98036605b310c9cbd6637db99dd1c7f53870a75bf1900142",
  "prior_history_sha256": "7e397ba703ad12b37daacfd5406a57a93ed58237ca6ed6823932c788032fa8b2",
  "retained_quality_prerequisite_sha256": "9480184b2f5586d8f9826b8db328c52a845ee812bd48afbf1432ca7f30bfd5d3",
  "retained_quality_final_audit_sha256": "f8248d19ec2a0c222b39d46fbc46c0d0cf4ccc3c1361c89965b597eedbb21cab",
  "prior_v6_prerequisite_sha256": "bef93bde0afa1251c67eb60f01dbbe6f8142b4f32b5a8076c0f34d5d9b402077",
  "prior_v6_timeout_waiver_sha256": "44991353526966ab57cfd0155c3f65af00061b87308eed37c2b735f207e75180",
  "historical_known_input_tokens_subtotal": 348648,
  "historical_known_output_tokens_subtotal": 13138,
  "historical_known_total_tokens_subtotal": 647378,
  "historical_unknown_total_attempts": 48,
  "historical_known_usage_pairs": 516,
  "historical_unknown_usage_pairs": 28,
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
  "human_policy_amendment_sha256": "83ca7491e4fc4a3673097f62f9590117e42387daadc656935318b5bd6caf4ebe",
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
  "maximum_concurrent_native_entries": 1
}`

func v9AuditPolicy() auditPolicy {
	p := v8AuditPolicy()
	p.Baseline, p.Maximum, p.DayModelTurns = 544, 71, 71
	p.KnownTotal, p.UnknownTotal = 647378, 48
	p.WindowID = "issue33-high-release-chain-v9-20261013-baseline544"
	p.PriorSHA, p.PrerequisiteSHA, p.HumanSHA = v9PriorSHA, v9PrerequisiteSHA, v9HumanSHA
	p.PassStatus = "CHAIN_V9_WITNESS_AUDIT_PASS"
	p.SerializedEntries = true
	p.WindowPolicy["provider_entries_serialized"] = true
	p.VerifyHuman = func(in inputs) {
		verifyExpenseException(in.ExpenseException)
		require(reflect.DeepEqual(obj(in.SafeTimeoutWaiver), frozenObject(v9HumanSemantic)), "v9_exact_human_amendment")
	}
	p.VerifyPrerequisite = func(o object) {
		require(reflect.DeepEqual(o, frozenObject(v9PrerequisiteSemantic)), "v9_exact_prerequisite")
	}
	return p
}
