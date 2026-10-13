package main

import (
	"path/filepath"
	"reflect"
)

const v7PriorSHA = "a8a0d81d750e08c7ba2c4ddd70d312a3d3776657b950a2e36a583041d0a24261"
const v7PrerequisiteSHA = "fc4f5cfd6fa7209f212cb47c767c4f80c60d304fb3c1513371c27008850636a6"
const v7HumanSHA = "efef2aa0f09ec0cf2fcfce7e55375920117f482a215f6fd6894abc6afd1f759e"
const v7HumanSemantic = `{
  "version": "butler-chain-v7-human-policy-amendment-v1",
  "source": "direct_user_messages_in_current_task",
  "user_messages": [
    "Хорошо тогда чутьчуть ослабь требования и если прям критичных проблем не останется тогда можно закрывать пр",
    "Ты можешь не делать сотни проверок перед проверкой на модели HIGH. У меня подписка и денег не списывается, потратить больше чем дано нельзя, смело тестируй модель",
    "Так может убрать/увеличить таймаут, если модель думает, то пускай, какбудто лучше чтобы агент выполнил задачу чем сказал что не сможет. Или ты считаешь что слишком долго на такие задачи он тратит времени?"
  ],
  "interpretation": "Allow a new bounded native High chain with longer deadlines and one safe HTTP or response-stream timeout retry; retain every closed failure and unknown measurement. Reuse independently verified unchanged checks and run narrow checks of changed guards before model execution.",
  "window_id": "issue33-high-release-chain-v7b-20261013-baseline539",
  "baseline_attempts": 539,
  "maximum_new_attempts": 76,
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
  "safe_timeout_stages": ["http_round_trip", "response_stream"],
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
  ]
}`
const v7PrerequisiteSemantic = `{
  "version": "butler-chain-high-reviewed-v7-prerequisite-v1",
  "window_id": "issue33-high-release-chain-v7b-20261013-baseline539",
  "baseline_attempts": 539,
  "maximum_new_attempts": 76,
  "maximum_total_attempts": 615,
  "prior_history_artifact_sha256": "f5424aa422a1f018c7bf4b7e13684e9ea86d6a68f316c8f82e31e00443228f1d",
  "prior_history_sha256": "a8a0d81d750e08c7ba2c4ddd70d312a3d3776657b950a2e36a583041d0a24261",
  "actual539_bridge_sha256": "a7f0dc24d6d30db0713a2f03bfc82d20ec03ab0fd0bed6bb2dbf05b061cc0c6d",
  "failed_v6_audit_sha256": "339e54f39e51c43972c54b87d2d335f4eefa8340ac6d51cfad05b8391772a0c2",
  "failed_v6_correlation_sha256": "5d809525016869a2199339c3f8188594f643009448bb640caef37552692cfb2e",
  "retained_quality_prerequisite_sha256": "9480184b2f5586d8f9826b8db328c52a845ee812bd48afbf1432ca7f30bfd5d3",
  "retained_quality_final_audit_sha256": "f8248d19ec2a0c222b39d46fbc46c0d0cf4ccc3c1361c89965b597eedbb21cab",
  "prior_v6_prerequisite_sha256": "bef93bde0afa1251c67eb60f01dbbe6f8142b4f32b5a8076c0f34d5d9b402077",
  "prior_v6_timeout_waiver_sha256": "44991353526966ab57cfd0155c3f65af00061b87308eed37c2b735f207e75180",
  "historical_known_input_tokens_subtotal": 301714,
  "historical_known_output_tokens_subtotal": 11982,
  "historical_known_total_tokens_subtotal": 580620,
  "historical_unknown_total_attempts": 48,
  "historical_known_usage_pairs": 511,
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
  "human_policy_amendment_sha256": "efef2aa0f09ec0cf2fcfce7e55375920117f482a215f6fd6894abc6afd1f759e"
}`

func frozenObject(raw string) object {
	v, e := decode([]byte(raw))
	require(e == nil, "frozen_policy_decode")
	return obj(v)
}
func verifyV7Human(in inputs) {
	verifyExpenseException(in.ExpenseException)
	require(reflect.DeepEqual(obj(in.SafeTimeoutWaiver), frozenObject(v7HumanSemantic)), "v7_exact_human_amendment")
}
func verifyV7Prerequisite(p object) {
	require(reflect.DeepEqual(p, frozenObject(v7PrerequisiteSemantic)), "v7_exact_prerequisite")
}

func retainedFailedV6Pins() map[string]string {
	return map[string]string{
		"final-audit":         "339e54f39e51c43972c54b87d2d335f4eefa8340ac6d51cfad05b8391772a0c2",
		"high":                "98530d41063be35a7d417825ac6f65768802f714ff2bc9a68bbff52ffc51d207",
		"pg-ledger":           "6f3539a915923d52f281cc330ebd29a702f8ebc0c763f490f8b263263af9c89c",
		"pg-product":          "c751cfd523f20e8eec07ef892cecc08811680b4edb577866243844fb3af3624f",
		"pg-initial":          "a2c4b54309fd430c1d5ef6371021e6c353949cad182d762533776f80575f930f",
		"runtime-state":       "e02a84ecdfc5df40808581cb6b85984898087676e35d9017bb839e95ed9971f7",
		"failure-correlation": "5d809525016869a2199339c3f8188594f643009448bb640caef37552692cfb2e",
	}
}
func pinnedObject(root, leaf, pin string) any {
	b, e := readPinnedPublic(filepath.Join(root, "deploy", "acceptance", leaf), pin)
	require(e == nil, "v7_retained_public_bytes")
	v, e := decode(b)
	require(e == nil, "v7_retained_public_json")
	return v
}
func verifyRetainedFailedV6Source(root string) {
	bootstrap := obj(pinnedObject(root, "release-chain-v7-bootstrap-failure-20261013.json", "8d143d3819b580d6f056896e00f46970ab3f39d674cb4f25ce5c2cb4cc1d076e"))
	require(bootstrap["status"] == "SEALED_BOOTSTRAP_NO_GO_ZERO_NATIVE" && integer(bootstrap["native_request_admissions"]) == 0 && integer(bootstrap["historical_attempt_baseline_unchanged"]) == 539 && bootstrap["provider_factory_reached"] == false && bootstrap["old_summary_actual_native_calls"] == nil, "v7_closed_zero_native_bootstrap")
	pinnedObject(root, "release-chain-v7-high-prerequisite-bootstrap1-20261013.json", "9a689c4a535c36524868d00a11f00a2a259d868b95d70fe2d64f0a852476e860")
	pinnedObject(root, "release-chain-v7-user-policy-amendment-bootstrap1-20261013.json", "169430ff19321b029196763a8aeae388a9d1d13c585ef8d07e59bbf55cdbf6e8")
	objects := map[string]any{}
	for kind, pin := range retainedFailedV6Pins() {
		objects[kind] = pinnedObject(root, "release-chain-v6-"+kind+"-20261013.json", pin)
	}
	verifyRetainedFailedV6(objects)
	verifyV6Prerequisite(obj(pinnedObject(root, "release-chain-v6-high-prerequisite-20261013.json", prerequisiteSHA)))
	verifySafeTimeoutWaiver(pinnedObject(root, "release-chain-v6-user-safe-timeout-waiver-20261013.json", safeTimeoutWaiverSHA))
	bridge := obj(pinnedObject(root, "release-chain-v7-carry539-independent-20261013.json", "a7f0dc24d6d30db0713a2f03bfc82d20ec03ab0fd0bed6bb2dbf05b061cc0c6d"))
	require(bridge["status"] == "INDEPENDENT_SEALED539_CARRY_BRIDGE" && bridge["fold_sha256"] == v7PriorSHA && bridge["old_originals_or_measurements_changed"] == false && bridge["issue33_ready"] == false, "v7_closed_carry_bridge")
	require(hashValue(arr(bridge["fold_vector"])) == v7PriorSHA, "v7_closed_fold_vector")
	old := obj(pinnedObject(root, "release-chain-v6-prior-history537-20261013.json", "71e640379d3904570afa4057be2580ce5713a2883275673bd9d1e356c163a2e7"))
	history := obj(pinnedObject(root, "release-chain-v7-prior-history539-20261013.json", "f5424aa422a1f018c7bf4b7e13684e9ea86d6a68f316c8f82e31e00443228f1d"))
	require(integer(history["native_attempts"]) == 539 && integer(history["known_usage_pairs"]) == 511 && integer(history["unknown_usage_pairs"]) == 28, "v7_history_cardinality")
	usage, prior := arr(history["usage"]), arr(old["usage"])
	require(len(usage) == 539 && len(prior) == 537 && reflect.DeepEqual(usage[:537], prior), "v7_old_measurements_immutable")
	require(len(arr(history["artifacts"])) == len(arr(old["artifacts"]))+1 && reflect.DeepEqual(arr(history["artifacts"])[:len(arr(old["artifacts"]))], arr(old["artifacts"])), "v7_old_artifacts_immutable")
	knownI, knownO, unknown := int64(0), int64(0), 0
	for _, v := range usage {
		u := obj(v)
		if u["actual_input_tokens"] == nil || u["actual_output_tokens"] == nil {
			require(u["actual_input_tokens"] == nil && u["actual_output_tokens"] == nil, "historical_partial_usage")
			unknown++
			continue
		}
		knownI += integer(u["actual_input_tokens"])
		knownO += integer(u["actual_output_tokens"])
	}
	require(knownI == 301714 && knownO == 11982 && unknown == 28, "v7_known_io_subtotals")
	for j, n := range arr(obj(objects["pg-ledger"])["requests"]) {
		u := obj(usage[537+j])
		r := obj(n)
		require(u["run_id"] == r["run_id"] && u["actual_input_tokens"] == r["input_tokens"] && u["actual_output_tokens"] == r["output_tokens"], "v7_paid_entries_exact_carry")
	}
}
func verifyRetainedFailedV6(objects map[string]any) {
	pins := retainedFailedV6Pins()
	vector := []string{priorSHA, pins["final-audit"], pins["high"], pins["pg-ledger"], pins["pg-product"], pins["pg-initial"], pins["runtime-state"], pins["failure-correlation"], "829d77582691b7e80a4fcbd6279b8db3c1046669", "dbb3737b32824e9324ff82b34975e5a4b9c60ba5a676b965022b948e741ad017", "b3601b408ffac951022d543906e92cc61abfeebbb9514dca0b6001ad8edf3a9f"}
	require(hashValue(vector) == v7PriorSHA, "v7_history_fold")
	oldAudit := obj(obj(objects["final-audit"])["chain_audit"])
	require(oldAudit["status"] == "NO_GO" && oldAudit["issue33_ready"] == false && integer(oldAudit["carry_forward_attempts"]) == 539 && integer(oldAudit["accepted_model_retry_failures"]) == 0, "v7_old_independent_no_go_preserved")
	scope, ledger := obj(obj(objects["high"])["scope"]), obj(objects["pg-ledger"])
	require(integer(scope["total_attempts"]) == 539 && integer(ledger["total_attempts"]) == 539 && reflect.DeepEqual(scope["window"], ledger["window"]), "v7_old_scope_pg_window_equal")
	native, sql := arr(scope["requests"]), arr(ledger["requests"])
	require(len(native) == 2 && len(sql) == 2, "v7_old_native_inventory")
	for j := range native {
		verifyRequestWithPolicy(obj(native[j]), obj(sql[j]), auditPolicy{HTTPTimeouts: true})
	}
	failed := obj(native[1])
	require(integer(failed["ordinal"]) == 539 && failed["actual_total_tokens"] == nil && failed["model_version"] == nil && failed["provider_operation_stage"] == "http_round_trip", "v7_closed539_unknown_phase_retained")
	require(integer(scope["accepted_model_retry_failures"]) == 1, "old_failed_collector_subset_bug_preserved")
	for _, kind := range []string{"pg-product", "pg-initial"} {
		ws := arr(objects[kind])
		require(len(ws) == 1, "v7_old_one_case")
		w := obj(ws[0])
		require(w["scenario"] == "normal_sent" && w["payload_sha256"] == hashValue(w["payload_json"]), "v7_old_payload_identity")
	}
	product := obj(obj(arr(objects["pg-product"])[0])["payload_json"])
	require(product["test_failed"] == true && product["original_result"] == nil && integer(product["all_channel_requests"]) == 0, "v7_old_no_result_no_notice")
	require(integer(obj(objects["runtime-state"])["exit_code"]) == 1 && obj(objects["runtime-state"])["oom_killed"] == false, "v7_old_runtime_failure")
	corr := obj(objects["failure-correlation"])
	carry := obj(corr["carry_forward"])
	require(corr["issue33_ready"] == false && integer(carry["attempts"]) == 539 && integer(carry["known_total_tokens_subtotal"]) == 580620 && integer(carry["unknown_total_attempts"]) == 48 && carry["old_unknown_266_cause"] == "UNKNOWN", "v7_historical_unknowns_honest")
}
