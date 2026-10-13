package main

import (
	"path/filepath"
	"reflect"
)

const v8PrerequisiteSHA = "effa65a6e39fcecce2510c76554685d047ee5e8c7ed1b9e6d27e633133d402ec"
const v8HumanSHA = "3560bb090a6b2c8b8bc78a987b2331eaf2eb55550b5e62572fbaf0bb7646973d"
const v8PriorSHA = "c74760a2d757f14e9f953bf2195fb557334a9910f8da62cbb3699c96a8758332"
const v8HumanSemantic = `{
  "version": "butler-chain-v8-human-policy-amendment-v1",
  "source": "direct_user_messages_in_current_task",
  "user_messages": [
    "Хорошо тогда чутьчуть ослабь требования и если прям критичных проблем не останется тогда можно закрывать пр",
    "Ты можешь не делать сотни проверок перед проверкой на модели HIGH. У меня подписка и денег не списывается, потратить больше чем дано нельзя, смело тестируй модель",
    "Так может убрать/увеличить таймаут, если модель думает, то пускай, какбудто лучше чтобы агент выполнил задачу чем сказал что не сможет. Или ты считаешь что слишком долго на такие задачи он тратит времени?"
  ],
  "interpretation": "Continue the bounded native High chain with the same longer deadlines and safe timeout recovery. Correct the confirmed schema versus decoder mismatch losslessly at the typed argument boundary, retain exact original tool proposal bytes privately before release, and keep every closed failure, unknown measurement and safety guard.",
  "window_id": "issue33-high-release-chain-v8-20261013-baseline541",
  "baseline_attempts": 541,
  "maximum_new_attempts": 74,
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
  "original_model_arguments_and_authorization_fingerprints_preserved": true
}`
const v8PrerequisiteSemantic = `{
  "version": "butler-chain-high-reviewed-v8-prerequisite-v1",
  "window_id": "issue33-high-release-chain-v8-20261013-baseline541",
  "baseline_attempts": 541,
  "maximum_new_attempts": 74,
  "maximum_total_attempts": 615,
  "prior_history_artifact_sha256": "ebbc1a11df409a22c58e2b602fb60f5614c4ee7d71e0c0b9e13f9c055113b079",
  "prior_history_sha256": "c74760a2d757f14e9f953bf2195fb557334a9910f8da62cbb3699c96a8758332",
  "retained_quality_prerequisite_sha256": "9480184b2f5586d8f9826b8db328c52a845ee812bd48afbf1432ca7f30bfd5d3",
  "retained_quality_final_audit_sha256": "f8248d19ec2a0c222b39d46fbc46c0d0cf4ccc3c1361c89965b597eedbb21cab",
  "prior_v6_prerequisite_sha256": "bef93bde0afa1251c67eb60f01dbbe6f8142b4f32b5a8076c0f34d5d9b402077",
  "prior_v6_timeout_waiver_sha256": "44991353526966ab57cfd0155c3f65af00061b87308eed37c2b735f207e75180",
  "historical_known_input_tokens_subtotal": 318461,
  "historical_known_output_tokens_subtotal": 12525,
  "historical_known_total_tokens_subtotal": 611404,
  "historical_unknown_total_attempts": 48,
  "historical_known_usage_pairs": 513,
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
  "human_policy_amendment_sha256": "3560bb090a6b2c8b8bc78a987b2331eaf2eb55550b5e62572fbaf0bb7646973d",
  "actual541_bridge_sha256": "a87d3628ae367507838a8d4fac3da937c39837e6e048ebd3ccaf7d5bc7361cef",
  "failed_v7b_audit_sha256": "a75e06331c6151a79726e94559fbe3c48f84e1af23dc0a8351d78ce3db517c28",
  "failed_v7b_correlation_sha256": "d6d2aa53d46d54ec2efd32ad97be6e5594093545002d1693c315c631366ee12d",
  "prior_v7b_prerequisite_sha256": "fc4f5cfd6fa7209f212cb47c767c4f80c60d304fb3c1513371c27008850636a6",
  "proposal_retention_required": true,
  "maximum_private_proposal_bytes": 262144,
  "preserve_original_proposal_bytes_and_authorization_fingerprint": true
}`

func v8AuditPolicy() auditPolicy {
	p := v7AuditPolicy()
	p.Baseline = 541
	p.Maximum = 74
	p.KnownTotal = 611404
	p.UnknownTotal = 48
	p.WindowID = "issue33-high-release-chain-v8-20261013-baseline541"
	p.PriorSHA = v8PriorSHA
	p.PrerequisiteSHA = v8PrerequisiteSHA
	p.HumanSHA = v8HumanSHA
	p.PassStatus = "CHAIN_V8_WITNESS_AUDIT_PASS"
	p.DayModelTurns = 74
	p.RequireProposals = true
	p.WindowPolicy = obj(clone(object{"maximum_generation_tokens": 32768, "safe_timeout_stages": "http_or_stream", "maximum_attempt_ms": 300000, "maximum_case_ms": 2400000, "maximum_global_ms": 7200000, "proposal_retention_required": true, "maximum_private_proposal_bytes": 262144}))
	p.VerifyHuman = func(in inputs) {
		verifyExpenseException(in.ExpenseException)
		require(reflect.DeepEqual(obj(in.SafeTimeoutWaiver), frozenObject(v8HumanSemantic)), "v8_exact_human_amendment")
	}
	p.VerifyPrerequisite = func(o object) {
		require(reflect.DeepEqual(o, frozenObject(v8PrerequisiteSemantic)), "v8_exact_prerequisite")
	}
	p.ReadPrivateProposal = func(m object) []byte {
		raw, e := readExact(filepath.Join("/proposals", proposalLeaf(integer(m["ordinal"]))), sha(m["private_proposal_sha256"]), 262144)
		require(e == nil, "v8_private_proposal_identity")
		return raw
	}
	return p
}
func verifyRetainedFailedV7bSource(root string) {
	pins := map[string]string{"final-audit": "a75e06331c6151a79726e94559fbe3c48f84e1af23dc0a8351d78ce3db517c28", "high": "d954dfce56bca311f3b83ea427d709fd87cb0a7c14ddf385462c80affce982d2", "pg-ledger": "6f75edc22c093bba57fbde3b978edfb93177e6ad5c761ce868e8f56f6a8e8ed3", "pg-product": "f6eef31876d685fb1f778484f25d99b8c25cfa41565e6b71db70c308f2af3884", "pg-initial": "69077eea2e57bcd8cc90a37f6f1cc56ad5921877f4c5c26afa3f58d285d9c392", "runtime-state": "605eb28f639803df22ba43a24af289c263a0278f017a08ffa3bfd70fed779871", "failure-correlation": "d6d2aa53d46d54ec2efd32ad97be6e5594093545002d1693c315c631366ee12d"}
	for k, pin := range pins {
		pinnedObject(root, "release-chain-v7b-"+k+"-20261013.json", pin)
	}
	h := obj(pinnedObject(root, "release-chain-v7b-history541-20261013.json", "ebbc1a11df409a22c58e2b602fb60f5614c4ee7d71e0c0b9e13f9c055113b079"))
	b := obj(pinnedObject(root, "release-chain-v7b-carry541-independent-20261013.json", "a87d3628ae367507838a8d4fac3da937c39837e6e048ebd3ccaf7d5bc7361cef"))
	c := obj(pinnedObject(root, "release-chain-v7b-failure-correlation-20261013.json", pins["failure-correlation"]))
	prior := obj(pinnedObject(root, "release-chain-v7-prior-history539-20261013.json", "f5424aa422a1f018c7bf4b7e13684e9ea86d6a68f316c8f82e31e00443228f1d"))
	require(integer(h["native_attempts"]) == 541 && integer(h["known_usage_pairs"]) == 513 && integer(h["unknown_usage_pairs"]) == 28 && h["baseline_sha256"] == v8PriorSHA, "v8_actual541_history")
	usage, old := arr(h["usage"]), arr(prior["usage"])
	require(len(usage) == 541 && len(old) == 539 && reflect.DeepEqual(usage[:539], old), "v8_old_measurements_immutable")
	require(len(arr(h["artifacts"])) == len(arr(prior["artifacts"]))+1 && reflect.DeepEqual(arr(h["artifacts"])[:len(arr(prior["artifacts"]))], arr(prior["artifacts"])), "v8_old_artifacts_immutable")
	require(b["status"] == "INDEPENDENT_SEALED541_CARRY_BRIDGE" && b["fold_sha256"] == v8PriorSHA && hashValue(arr(b["fold_vector"])) == v8PriorSHA && b["old_originals_or_measurements_changed"] == false && b["prospective_model_admission"] == false, "v8_closed_fold")
	require(c["native_scope_sealed"] == true && c["issue33_ready"] == false && c["actual_original_result"] == nil && integer(c["actual_notice_count"]) == 0 && obj(c["failure_classification"])["specific_rejected_argument_or_contract_cause"] == "UNKNOWN_FROM_CURRENT_SAFE_EXPORTS", "v8_old_generic_cause_unknown")
	var in, out int64
	unknown := 0
	for _, v := range usage {
		u := obj(v)
		if u["actual_input_tokens"] == nil {
			require(u["actual_output_tokens"] == nil, "v8_historical_pair")
			unknown++
		} else {
			in += integer(u["actual_input_tokens"])
			out += integer(u["actual_output_tokens"])
		}
	}
	require(in == 318461 && out == 12525 && unknown == 28, "v8_actual_known_io")
	carry := obj(c["carry_forward"])
	require(integer(carry["known_total_tokens_subtotal"]) == 611404 && integer(carry["unknown_total_attempts"]) == 48 && carry["old_unknown_266_cause"] == "UNKNOWN" && reflect.DeepEqual(carry, obj(b["carry_forward"])), "v8_historical_cost_honest")
}
