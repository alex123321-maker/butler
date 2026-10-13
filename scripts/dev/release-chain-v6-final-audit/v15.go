package main

import "reflect"

const v15PrerequisiteSHA = "85affdbb6ad88bb64012b580317ac03e051064a74eef86fd6e390697d4922166"
const v15HumanSHA = "1ef90ad0872a249633173e41a8ede60fb588c98ccf4bd9ee33b5bb6169cfb65c"
const v15PriorSHA = "46dccfc5040075940c810baae10b6340517fe5edfee57a9537b62d7906cd6cea"
const v15HumanSemantic = `{
  "version": "butler-chain-v15-human-policy-amendment-v1",
  "source": "direct_user_messages_in_current_task",
  "user_messages": [
    "Хорошо тогда чутьчуть ослабь требования и если прям критичных проблем не останется тогда можно закрывать пр",
    "Ты можешь не делать сотни проверок перед проверкой на модели HIGH. У меня подписка и денег не списывается, потратить больше чем дано нельзя, смело тестируй модель",
    "Так может убрать/увеличить таймаут, если модель думает, то пускай, какбудто лучше чтобы агент выполнил задачу чем сказал что не сможет. Или ты считаешь что слишком долго на такие задачи он тратит времени?",
    "А может уберём ограничения на количество вызовов? Есть ли какие то подводные камни в этом, кроме потенциально большего расхода токенов? Я просто думаю что это и развяжет руки агенту и он сможет качественнее выполнять работу",
    "Сразу запускай модель, после правки"
  ],
  "interpretation": "Immediately build and run a fresh High chain from factual611 after the future-only verifier fix. The fixture verifies the immutable project patch/check across qualified completed runs of the same current root task, owner, session, task and constraints revisions with confirmed dispatches; the owner result remains bound to the actual original conversion final. Legacy run-only verification remains unchanged. Production current activated-context repair is unchanged from measured9c. All five original-result/source/authority/Stop/delivery/dedup and independent final acceptance gates remain. Retain V14 NO_GO, its all-known18cost and old UNKNOWNs without retry, refund or acceptance rewrite.",
  "window_id": "issue33-high-release-chain-v15-20261013-baseline611",
  "baseline_attempts": 611,
  "maximum_new_attempts": 120,
  "maximum_total_attempts": 731,
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
  "old_sealed_absolute_ceiling": 628,
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
  "production_source_check_or_authority_behavior_changed": false,
  "allow_repeated_patch_checks": true,
  "patch_check_read_count_limit_source": "owning_core_budget",
  "repeated_project_inspection_allowed": true,
  "patch_check_read_method": "GET",
  "patch_check_effect": "read",
  "patch_check_receipt_inventory_required": true,
  "patch_check_receipts_must_agree": true,
  "mandatory_mutation_action_counts_changed": false,
  "release_chain_exact_read_count_criterion_changed": true,
  "original_write_or_notice_repeat_allowed": false,
  "original_legacy_read_receipt_unknowns_reconstructed": false,
  "zero_native_v12_bootstrap_sha256": "dca503946b4a1bfc5a09215b128b5d4d1ea5f3d71333d4a09edcff5271ef7326",
  "new_fixture_window_declared_limit": 120,
  "legacy_fixture_window_maximum": 80,
  "zero_bootstrap_adds_native_ordinal": false,
  "bootstrap_original_null_fields_rewritten": false,
  "previous_v12_scope_remains_sealed": true,
  "previous_v13_scope_remains_sealed": true,
  "independent_pre_native_admission_required": false,
  "offline_test_suite_before_native_required": false,
  "confirmed_activation_context_test_only": false,
  "metadata_fixed120_reader_test_only": true,
  "historical593_fold_sha256": "f7bb9011255ec3bfe0286f29cd1fd958cd719cf6db7d0a2dc6fef2b24534a265",
  "previous_v14_scope_remains_sealed": true,
  "task_scoped_original_proof": true,
  "original_proof_qualified_run_call_ids_required": true,
  "original_proof_current_task_constraints_and_confirmed_dispatch_required": true,
  "historical611_fold_sha256": "46dccfc5040075940c810baae10b6340517fe5edfee57a9537b62d7906cd6cea"
}`
const v15PrerequisiteSemantic = `{
  "version": "butler-chain-high-reviewed-v15-prerequisite-v1",
  "window_id": "issue33-high-release-chain-v15-20261013-baseline611",
  "baseline_attempts": 611,
  "maximum_new_attempts": 120,
  "maximum_total_attempts": 731,
  "prior_history_artifact_sha256": "4f62e26456070b298ee411f2d7681fbf6dda0d6662cf869a4cbcba8674303220",
  "prior_history_sha256": "46dccfc5040075940c810baae10b6340517fe5edfee57a9537b62d7906cd6cea",
  "retained_quality_prerequisite_sha256": "9480184b2f5586d8f9826b8db328c52a845ee812bd48afbf1432ca7f30bfd5d3",
  "retained_quality_final_audit_sha256": "f8248d19ec2a0c222b39d46fbc46c0d0cf4ccc3c1361c89965b597eedbb21cab",
  "prior_v6_prerequisite_sha256": "bef93bde0afa1251c67eb60f01dbbe6f8142b4f32b5a8076c0f34d5d9b402077",
  "prior_v6_timeout_waiver_sha256": "44991353526966ab57cfd0155c3f65af00061b87308eed37c2b735f207e75180",
  "historical_known_input_tokens_subtotal": 1103351,
  "historical_known_output_tokens_subtotal": 25577,
  "historical_known_total_tokens_subtotal": 1512064,
  "historical_unknown_total_attempts": 49,
  "historical_known_usage_pairs": 582,
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
  "human_policy_amendment_sha256": "1ef90ad0872a249633173e41a8ede60fb588c98ccf4bd9ee33b5bb6169cfb65c",
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
  "old_sealed_absolute_ceiling": 628,
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
  "production_source_check_or_authority_behavior_changed": false,
  "actual575_bridge_sha256": "043ef974962836ab2c69691dccdc1d1add15c379632599b1889057cd55d3fc00",
  "failed_v11_audit_sha256": "85a32e433345e53db47b0f314feb55c5d23baadff0e18be0756084e77f9f54ae",
  "failed_v11_correlation_sha256": "638cebaedc3b4b4ceed5745c70053e5c1aaa0fe4d245681c97ce990781ec14be",
  "prior_v11_prerequisite_sha256": "9f57fb1c613ffcd86f5f2b19a9cdd2b17c92f2c726723551fd9fbd44b8957888",
  "prior_v11_authority_sha256": "31a2381c9f23f1ceeb9687f87bcb705c484433617491b8ec30d1173988bbd5f8",
  "retained_closed_v11_proposal_diagnostic_sha256": "be70e93a61d10873bab71a934a54ce763ee1d25f52b8c951adb6a70585ee8c2e",
  "allow_repeated_patch_checks": true,
  "patch_check_read_count_limit_source": "owning_core_budget",
  "repeated_project_inspection_allowed": true,
  "patch_check_read_method": "GET",
  "patch_check_effect": "read",
  "patch_check_receipt_inventory_required": true,
  "patch_check_receipts_must_agree": true,
  "mandatory_mutation_action_counts_changed": false,
  "release_chain_exact_read_count_criterion_changed": true,
  "original_write_or_notice_repeat_allowed": false,
  "original_legacy_read_receipt_unknowns_reconstructed": false,
  "prior_v12_prerequisite_sha256": "4d9af878b09f812c0f8fa69198f03c377b7485b89faf953f180bd34fa69430ca",
  "prior_v12_authority_sha256": "77f0355763d3e020b3e43215c32bbdf490d665422eaafb061f4376622720447d",
  "zero_native_v12_bootstrap_sha256": "dca503946b4a1bfc5a09215b128b5d4d1ea5f3d71333d4a09edcff5271ef7326",
  "new_fixture_window_declared_limit": 120,
  "legacy_fixture_window_maximum": 80,
  "zero_bootstrap_adds_native_ordinal": false,
  "bootstrap_original_null_fields_rewritten": false,
  "previous_v12_scope_remains_sealed": true,
  "actual593_history_sha256": "b93399340f64d881c99180b3ea6b62885341ba25f6de70edb64a34a0b51789f0",
  "actual593_bridge_sha256": "61a4bbda07cfa28e4d92caf2557d5447a96ed47abf54fbc71c13c849634e1398",
  "failed_v13_supplemental_report_sha256": "ab3da7aa5cb254401ca12eb0ed26e69819bf0d0d0da84e94d51c8fd31dee9b93",
  "failed_v13_correlation_sha256": "e23955bcf779f1c062529c0531ca6984db4119dd529009f05a2ade4a7528aca7",
  "original_v13_proposal_metadata_null_preserved": true,
  "original_v13_go_audit_stdout_empty_preserved": true,
  "native_immediately_after_build": true,
  "independent_pre_native_admission_required": false,
  "offline_test_suite_before_native_required": false,
  "legacy_fixture_metadata_maximum": 80,
  "future_fixture_metadata_declared_limit": 120,
  "actual611_history_sha256": "bcb0716067930c1cd65cbbce869ae5bcda9056ce0e6bb0b034c3c845644fef28",
  "actual611_bridge_sha256": "267a715de1c479773222368006eaaf21d5f852c6909271a44828846befe50e65",
  "failed_v14_supplemental_report_sha256": "aeb9dce14ef0310b96de9cda1eeded0889d8ab7b2bec95b512be1d843df8009a",
  "failed_v14_correlation_sha256": "b52ac5c96bf62674fa0c81a5201fccdc419b76eff6bed011150483865de04e23",
  "task_scoped_original_proof": true,
  "original_proof_qualified_run_call_ids_required": true,
  "original_proof_current_task_constraints_and_confirmed_dispatch_required": true
}`

func v15AuditPolicy() auditPolicy {
	p := v14AuditPolicy()
	p.Baseline, p.KnownTotal = 611, 1512064
	p.WindowID = "issue33-high-release-chain-v15-20261013-baseline611"
	p.PriorSHA, p.PrerequisiteSHA, p.HumanSHA = v15PriorSHA, v15PrerequisiteSHA, v15HumanSHA
	p.PassStatus = "CHAIN_V15_WITNESS_AUDIT_PASS"
	p.RequireTaskScopedOriginalProof = true
	p.VerifyHuman = func(in inputs) {
		verifyExpenseException(in.ExpenseException)
		require(reflect.DeepEqual(obj(in.SafeTimeoutWaiver), frozenObject(v15HumanSemantic)), "v15_exact_human_amendment")
	}
	p.VerifyPrerequisite = func(o object) {
		require(reflect.DeepEqual(o, frozenObject(v15PrerequisiteSemantic)), "v15_exact_prerequisite")
	}
	return p
}

// The existing witness exports qualified run/call, actual receipt hashes,
// current task/run revision and the owner result's conversion-final run. The
// private full action-binding constraints/owner checks and evidence strings
// remain enforced by the source-pinned fixture SQL; no new witness is invented.
func verifyV15TaskScopedOriginalProof(name string, ini, fin object) {
	if name == "owner_stop" {
		return
	} // existing Stop/result/notice prohibitions remain
	db := obj(fin["postgres"])
	root := text(db["root_task_id"])
	task := byID(rows(db, "tasks"), "task_id")[root]
	// Completion legitimately advances task revisions. Actions and the owner
	// result remain bound to the original active promoted epoch, as the fixture
	// checked before the owner transition; never compare them to the later epoch.
	promoted := obj(obj(ini["postgres"])["promoted_task"])
	require(task != nil && promoted["task_id"] == root, "v15_current_root_task")
	runs := byID(rows(db, "runs"), "run_id")
	o := obj(fin["original_result"])
	var conversionRun string
	for _, a := range rows(db, "actions") {
		r := runs[text(a["run_id"])]
		if a["tool_name"] == "converter.developer-inventory.convert" {
			require(conversionRun == "" && r != nil && r["task_id"] == root, "v15_one_original_conversion_run")
			conversionRun = text(r["run_id"])
		}
		if a["tool_name"] != "extension.project.prepare-patch" {
			continue
		}
		require(r != nil && r["task_id"] == root && r["task_revision"] == promoted["revision"] && r["current_state"] == "completed" && a["tool_call_id"] == o["patch_tool_call_id"], "v15_current_prepare_phase")
	}
	require(conversionRun != "", "v15_original_conversion_missing")
	for _, list := range []string{"check_receipts", "inspect_receipts"} {
		for _, receipt := range rows(o, list) {
			r := runs[text(receipt["run_id"])]
			require(r != nil && r["task_id"] == root && r["task_revision"] == promoted["revision"] && r["current_state"] == "completed", "v15_current_qualified_read_phase")
		}
	}
	verified := 0
	for _, result := range rows(db, "task_results") {
		if result["task_id"] == root && result["verdict"] == "verified" {
			verified++
			require(result["run_id"] == conversionRun && result["task_revision"] == promoted["revision"] && result["verifier"] == "owner", "v15_owner_result_actual_conversion_final")
		}
	}
	require(verified == 1, "v15_one_owner_conversion_verification")
}
