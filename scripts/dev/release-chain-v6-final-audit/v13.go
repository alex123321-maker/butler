package main

import "reflect"

const v13PrerequisiteSHA = "c1aa03290cc85616c4f2156acfde3d116037e7cafd0a4f89b5ea2b9d545c2f4e"
const v13HumanSHA = "e62000001cd3685560587b1e74c6e0583118e8d0b143cc386f860ac4965d6934"
const v13BootstrapSHA = "dca503946b4a1bfc5a09215b128b5d4d1ea5f3d71333d4a09edcff5271ef7326"
const v13HumanSemantic = `{
  "version": "butler-chain-v13-human-policy-amendment-v1",
  "source": "direct_user_messages_in_current_task",
  "user_messages": [
    "Хорошо тогда чутьчуть ослабь требования и если прям критичных проблем не останется тогда можно закрывать пр",
    "Ты можешь не делать сотни проверок перед проверкой на модели HIGH. У меня подписка и денег не списывается, потратить больше чем дано нельзя, смело тестируй модель",
    "Так может убрать/увеличить таймаут, если модель думает, то пускай, какбудто лучше чтобы агент выполнил задачу чем сказал что не сможет. Или ты считаешь что слишком долго на такие задачи он тратит времени?",
    "А может уберём ограничения на количество вызовов? Есть ли какие то подводные камни в этом, кроме потенциально большего расхода токенов? Я просто думаю что это и развяжет руки агенту и он сможет качественнее выполнять работу"
  ],
  "interpretation": "Continue a fresh finite High chain from factual575 after independently proved zero-native V12 bootstrap denial. Use the new fixed120 fixture constructor and matching SQL ceiling instead of passing120 to legacy80; preserve the legacy80 path and every immutable counter/deadline/ownership/source rule. The same prospective575+120=695 allocation is a new declared scope, not reopening or borrowing an old window. Owned GET inspection/check reads follow current Core budgets and every actual receipt; mandatory mutations remain once. Historical575 expense/UNKNOWN and all five original-result/Stop/delivery/independent acceptance gates remain unchanged.",
  "window_id": "issue33-high-release-chain-v13-20261013-baseline575",
  "baseline_attempts": 575,
  "maximum_new_attempts": 120,
  "maximum_total_attempts": 695,
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
  "previous_v12_scope_remains_sealed": true
}`
const v13PrerequisiteSemantic = `{
  "version": "butler-chain-high-reviewed-v13-prerequisite-v1",
  "window_id": "issue33-high-release-chain-v13-20261013-baseline575",
  "baseline_attempts": 575,
  "maximum_new_attempts": 120,
  "maximum_total_attempts": 695,
  "prior_history_artifact_sha256": "4f62e26456070b298ee411f2d7681fbf6dda0d6662cf869a4cbcba8674303220",
  "prior_history_sha256": "d24720adf3522dc19ada8b53a7d54d4485fe7774c0cd029d3c717c9f1468db05",
  "retained_quality_prerequisite_sha256": "9480184b2f5586d8f9826b8db328c52a845ee812bd48afbf1432ca7f30bfd5d3",
  "retained_quality_final_audit_sha256": "f8248d19ec2a0c222b39d46fbc46c0d0cf4ccc3c1361c89965b597eedbb21cab",
  "prior_v6_prerequisite_sha256": "bef93bde0afa1251c67eb60f01dbbe6f8142b4f32b5a8076c0f34d5d9b402077",
  "prior_v6_timeout_waiver_sha256": "44991353526966ab57cfd0155c3f65af00061b87308eed37c2b735f207e75180",
  "historical_known_input_tokens_subtotal": 689116,
  "historical_known_output_tokens_subtotal": 18790,
  "historical_known_total_tokens_subtotal": 1039921,
  "historical_unknown_total_attempts": 49,
  "historical_known_usage_pairs": 546,
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
  "human_policy_amendment_sha256": "e62000001cd3685560587b1e74c6e0583118e8d0b143cc386f860ac4965d6934",
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
  "previous_v12_scope_remains_sealed": true
}`
const v13BootstrapSemantic = `{
  "version": "butler-release-chain-v12-zero-native-bootstrap-review-v1",
  "reviewed_at_utc": "2026-10-13T18:43:08.212403+00:00",
  "status": "NO_GO_ZERO_NATIVE_BOOTSTRAP",
  "issue33_ready": false,
  "reviewer_role": "/root/v5_final_independent_audit",
  "actual_proof": "/srv/storage/butler/agents/scenario-release-chain-v12-high-20261013/reports/release-chain-process-20261013T183424Z-2945984",
  "measured_source_commit": "cc63e7b4bacba8d3c46af7842c96d99838626390",
  "source_tree_sha256": "d5d7d24a7143905f55479ef51e297cfd94e5bf423e137abf08f675c59fa28b8b",
  "source_manifest_sha256": "bf5c17e1c643e776d83fc73463c5bf50f8f9d24dee35c5b96d927d3fd77a1c4a",
  "source_inventory_files": 2340,
  "app_binary_sha256": "24a7bbe71e557384a940da5fb5ff7171d4b6361711bd03dc827a6d77964e9594",
  "runtime": {
    "image_id": "sha256:5cf287a799e6b94384bad13d16b14904c531f51ba65792237e122ce42b392f61",
    "postgres_image_id": "sha256:ac08538c6f8b9904c33c8224c5e5706dbe760aca29db1d096972b4052c22a75d",
    "converter_image_id": "sha256:ce64758a109eb420d874a118f87920e625e12d3634e03b4a5573fd9f6e5d3507",
    "started_at": "2026-10-13T18:34:28.599826054Z",
    "finished_at": "2026-10-13T18:35:02.962229479Z",
    "exit_code": 1,
    "oom_killed": false,
    "memory_limit_bytes": 1610612736
  },
  "evidence": {
    "tests.jsonl": {
      "sha256": "7902ceb83a9587c2fa68148cd644246c2037e17099e5da967963bcbf8d8917cc",
      "bytes": 15279
    },
    "runtime-state.json": {
      "sha256": "aac9eb7b1e992e5147eac7df7edbecd0400cf99f1e76b87ceb50c2a7612675b3",
      "bytes": 434
    },
    "scope.json": {
      "sha256": "6c8d9c955e99c32bde31ded6e889d6e5f7d5161ac9ad62611d6947a60a214adb",
      "bytes": 295
    },
    "summary.json": {
      "sha256": "e1de86393b082375dfc95f50c71d648b13ec6f1a76fac6d48cdb586ebd93d2f8",
      "bytes": 154
    },
    "source-manifest.sha256": {
      "sha256": "bf5c17e1c643e776d83fc73463c5bf50f8f9d24dee35c5b96d927d3fd77a1c4a",
      "bytes": 267764
    },
    "source-commit.txt": {
      "sha256": "7232999d7e4f7018518c67a670c0edfa24c5efb7f81f45cf482f323c67a3b0d6",
      "bytes": 41
    },
    "pg-initial-witness.json": {
      "sha256": "e3b0c44298fc1c149afbf4c8996fb92427ae41e4649b934ca495991b7852b855",
      "bytes": 0
    },
    "pg-ledger-witness.json": {
      "sha256": "e3b0c44298fc1c149afbf4c8996fb92427ae41e4649b934ca495991b7852b855",
      "bytes": 0
    },
    "pg-product-witness.json": {
      "sha256": "e3b0c44298fc1c149afbf4c8996fb92427ae41e4649b934ca495991b7852b855",
      "bytes": 0
    },
    "binaries.sha256": {
      "sha256": "467d7f17815fa9e2b703f7e7ab55a85a0129fa3bcc4b7dd7ead92ad46d9cc500",
      "bytes": 1447
    },
    "native-evidence.sha256": {
      "sha256": "62b78148f5a4874b123a5e6094d37189410926f38b792c790a66f45e1adcfa35",
      "bytes": 1046
    },
    "events.sha256": {
      "sha256": "0d51da7ffa7705710d2c572c330fc344e963112ff32636c03dc47d06ffbdd301",
      "bytes": 201
    },
    "runtime-profile.json": {
      "sha256": "1ab435172fa063abaecc742f78ba36048a0202a9f0e3546afd9e6fe09e889a12",
      "bytes": 181
    },
    "hardware.json": {
      "sha256": "c035ff8ccecb9a5f3d3cf1a789d9b70a80fbfbb08c2f9fa29ec3caed35d81071",
      "bytes": 736
    }
  },
  "failed_first_case": "normal_sent",
  "other_four_cases_run": false,
  "bootstrap_cause": "fixture_window_constructor_maximum120_exceeds_legacy80_before_DDL",
  "observed_failure": "declared release chain provider unavailable",
  "secondary_cleanup_observation": "safe private proposal metadata unavailable; no native proposal sink had been constructed",
  "derived_native_provider_requests": 0,
  "native_provider_constructor_reachable": false,
  "core_executor_constructed": false,
  "kernel_model_quotes_reachable": false,
  "provider_auth_import_reachable": false,
  "actual_native_calls_field_original": null,
  "original_field_overwritten": false,
  "historical_attempts_unchanged": 575,
  "new_usage_entries": 0,
  "historical_known_input_subtotal": 689116,
  "historical_known_output_subtotal": 18790,
  "historical_known_total_subtotal": 1039921,
  "historical_unknown_total_entries": 49,
  "historical_unknown_io_pairs": 29,
  "verified_scenarios": [],
  "original_window_remains_sealed": true,
  "boundary_basis": [
    "Exact frozen NewWindow passes120 to a constructor that unconditionally rejects maximum>80 before CREATE TABLE or any invocation of the native provider constructor.",
    "The factory propagates that error before provider auth import/initialization; Core executor/worker and its model quotation are created only after the factory successfully returns.",
    "Recorded Fatal469 agrees with that factory denial. Empty PG exports and null scope are corroboration only; absence alone is not treated as proof of zero."
  ],
  "limits": [
    "No live owning PostgreSQL query or private runtime directory inspection was performed by reviewer.",
    "The original summary actual_native_calls=NULL and all empty exports remain unchanged; zero is separately derived from retained source ordering plus actual failure, not a fabricated original measurement.",
    "No claim of zero setup actions, preflight TLS activity, or credential-file preparation is made.",
    "The admission review f507 correctly checked source/runtime/pins/40 targeted tests but missed the constructor120 versus legacy80 incompatibility. It also retained stale descriptive V11/557 fields; this report does not overwrite that artifact or claim its narrative was current."
  ]
}`

func v13AuditPolicy() auditPolicy {
	p := v12AuditPolicy()
	p.WindowID = "issue33-high-release-chain-v13-20261013-baseline575"
	p.PrerequisiteSHA, p.HumanSHA = v13PrerequisiteSHA, v13HumanSHA
	p.PassStatus = "CHAIN_V13_WITNESS_AUDIT_PASS"
	p.VerifyHuman = func(in inputs) {
		verifyExpenseException(in.ExpenseException)
		require(reflect.DeepEqual(obj(in.SafeTimeoutWaiver), frozenObject(v13HumanSemantic)), "v13_exact_human_amendment")
	}
	p.VerifyPrerequisite = func(o object) {
		require(reflect.DeepEqual(o, frozenObject(v13PrerequisiteSemantic)), "v13_exact_prerequisite")
	}
	return p
}

// The original NULL field stays NULL. Separate zero is a sealed independent
// source-order/failure proof, never inferred from the absence of PG exports.
func verifyV13Bootstrap(value any) {
	require(reflect.DeepEqual(obj(value), frozenObject(v13BootstrapSemantic)), "v13_exact_zero_native_bootstrap")
}
func verifyV13BootstrapSource(root string) {
	verifyV13Bootstrap(pinnedObject(root, "release-chain-v12-bootstrap-independent-20261013.json", v13BootstrapSHA))
}
