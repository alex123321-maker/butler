package app

import (
	"context"
	"github.com/jackc/pgx/v5/pgxpool"
	"os"
	"testing"
	"time"
)

const releaseChainV10WindowID = "issue33-high-release-chain-v10-20261013-baseline556"
const releaseChainV10ReviewedPrerequisiteSHA = "193e3b8c21b00ffdda62ea0dddb7ecf17357f912d7c78e0d349873e92ee63b86"
const releaseChainV10AuthoritySHA = "417dfd7bf7e9f0fba535d18a50552696f8efd08a9bf19b2f4a54d4cfb922da1d"
const releaseChainV10AuthoritySemanticJSON = `{"version":"butler-chain-v10-human-policy-amendment-v1","source":"direct_user_messages_in_current_task","user_messages":["Хорошо тогда чутьчуть ослабь требования и если прям критичных проблем не останется тогда можно закрывать пр","Ты можешь не делать сотни проверок перед проверкой на модели HIGH. У меня подписка и денег не списывается, потратить больше чем дано нельзя, смело тестируй модель","Так может убрать/увеличить таймаут, если модель думает, то пускай, какбудто лучше чтобы агент выполнил задачу чем сказал что не сможет. Или ты считаешь что слишком долго на такие задачи он тратит времени?"],"interpretation":"Continue the finite full High chain from factual556. Align only the test extension subprocess lifetime with the existing case deadline; preserve the three-minute legacy default, runtime authentication and egress rules. Keep every spent prior attempt, closed UNKNOWN and critical original-result guard. The fresh 71-request allocation follows the direct instruction to test High proactively; its prospective ceiling627 does not reopen the earlier615 windows or refund their twelve new requests.","window_id":"issue33-high-release-chain-v10-20261013-baseline556","baseline_attempts":556,"maximum_new_attempts":71,"maximum_total_attempts":627,"maximum_generation_tokens":32768,"maximum_attempt_ms":300000,"maximum_case_ms":2400000,"maximum_global_ms":7200000,"maximum_child_model_turns":4,"maximum_child_external_calls":4,"maximum_child_active_ms":1320000,"maximum_model_retries_per_input":1,"model_retry_backoff_ms":2000,"safe_timeout_stages":["http_round_trip","response_stream"],"keep_closed_537_and_539_unknown":true,"all_completed_entries_require_measured_usage_and_selected_version":true,"failed_unknown_requires_proved_safe_same_run_recovery":true,"critical_safety_or_original_result_criteria_changed":false,"quality_or_comparison_criteria_changed":false,"release_chain_latency_policy_changed":true,"independent_acceptance_required_before_ready_merge_closure":true,"native_window_reopened":false,"issue33_ready":false,"limitations":["This records the direct messages and the assistant's bounded operational choices; it is not a cryptographic human signature.","The new deadline does not establish that the model was thinking during an unanswered HTTP request.","Subscription-no-overage pricing does not supply missing token measurements or provider-side billing and runtime records."],"proposal_retention_required":true,"maximum_private_proposal_bytes":262144,"original_model_arguments_and_authorization_fingerprints_preserved":true,"provider_entries_serialized":true,"maximum_concurrent_native_entries":1,"maximum_extension_harness_lifetime_ms":2400000,"extension_harness_lifetime_source":"remaining_case_deadline","extension_harness_lifetime_test_only":true,"legacy_extension_harness_lifetime_ms":180000,"old_sealed_absolute_ceiling":615,"old_closed_attempts_or_unknowns_refunded":false,"new_scope_allocation_is_prospective":true}`
const releaseChainV10PrerequisiteSemanticJSON = `{"version":"butler-chain-high-reviewed-v10-prerequisite-v1","window_id":"issue33-high-release-chain-v10-20261013-baseline556","baseline_attempts":556,"maximum_new_attempts":71,"maximum_total_attempts":627,"prior_history_artifact_sha256":"8509f520a5b013900c5365f06868656448afd74a3f24d2f88971f013024055f5","prior_history_sha256":"a8ff280f6905081c83c48f8c0dc327576792e0de9ddd44b8e952cc9d0e0ec00d","retained_quality_prerequisite_sha256":"9480184b2f5586d8f9826b8db328c52a845ee812bd48afbf1432ca7f30bfd5d3","retained_quality_final_audit_sha256":"f8248d19ec2a0c222b39d46fbc46c0d0cf4ccc3c1361c89965b597eedbb21cab","prior_v6_prerequisite_sha256":"bef93bde0afa1251c67eb60f01dbbe6f8142b4f32b5a8076c0f34d5d9b402077","prior_v6_timeout_waiver_sha256":"44991353526966ab57cfd0155c3f65af00061b87308eed37c2b735f207e75180","historical_known_input_tokens_subtotal":475637,"historical_known_output_tokens_subtotal":15159,"historical_known_total_tokens_subtotal":793703,"historical_unknown_total_attempts":48,"historical_known_usage_pairs":528,"historical_unknown_usage_pairs":28,"provider_stage_observation":true,"capture_kernel_operation_id":true,"requested_model":"gemini-3.8-flash-high","observed_successful_model_version":"gemini-3.8-flash-n","maximum_generation_tokens":32768,"maximum_attempt_ms":300000,"maximum_case_ms":2400000,"maximum_global_ms":7200000,"allowed_provider_operation_stages":["http_round_trip","response_stream"],"maximum_model_retries_per_input":1,"model_retry_backoff_ms":2000,"all_completed_entries_require_measured_usage_and_selected_version":true,"failed_usage_shape":"valid_all_three_known_or_all_three_null","failed_model_version_shape":"selected_version_or_null","all_other_quality_source_authority_stop_effect_delivery_gates_retained":true,"issue33_ready":false,"human_policy_amendment_sha256":"417dfd7bf7e9f0fba535d18a50552696f8efd08a9bf19b2f4a54d4cfb922da1d","proposal_retention_required":true,"maximum_private_proposal_bytes":262144,"preserve_original_proposal_bytes_and_authorization_fingerprint":true,"actual544_bridge_sha256":"ca18f5d5f3a78ac9d61ada16472399ecc3024801b8b077795fabbe3db9584650","failed_v8_audit_sha256":"5573b9f25e13a4f3aefacacf142aeda7846b68962f8cd26df4498417097c580f","failed_v8_correlation_sha256":"2e1eed8eadb5874299693bd28ea9857fbd6d96ce9a2e1865009f2368403761d3","prior_v8_prerequisite_sha256":"effa65a6e39fcecce2510c76554685d047ee5e8c7ed1b9e6d27e633133d402ec","prior_v8_authority_sha256":"3560bb090a6b2c8b8bc78a987b2331eaf2eb55550b5e62572fbaf0bb7646973d","retained_closed_v8_proposal_diagnostic_sha256":"8d5b296bfacd45da1d578896435378e7151787ee4847f26d6953267da3af87d8","provider_entries_serialized":true,"maximum_concurrent_native_entries":1,"actual556_bridge_sha256":"70041e9a3275a2099d39eda98d4535420eea7bfdae1bb3eb34bd58662f51ae3b","failed_v9_audit_sha256":"731cf102a25d42a091441ea045cedb5964c66ebb305890c564cc3b5b9713ad83","failed_v9_correlation_sha256":"b96457cbb16bfbc89cb3fb32c62a3d9622939efb9fbe574eb97253c4f89f9b6c","prior_v9_prerequisite_sha256":"4f753010b86b57ff2ea4cc8ff698d56d23787ab1ec20b572cb35ad329b1802c3","prior_v9_authority_sha256":"83ca7491e4fc4a3673097f62f9590117e42387daadc656935318b5bd6caf4ebe","retained_closed_v9_proposal_diagnostic_sha256":"8b5c8683e1aa23e49c15809e5d12432df3402ad01365a2e5a1567ed6f55350fb","maximum_extension_harness_lifetime_ms":2400000,"extension_harness_lifetime_source":"remaining_case_deadline","extension_harness_lifetime_test_only":true,"legacy_extension_harness_lifetime_ms":180000,"old_sealed_absolute_ceiling":615,"old_closed_attempts_or_unknowns_refunded":false,"new_scope_allocation_is_prospective":true}`

func readReleaseChainV10Prerequisite() (releaseChainV5Prerequisite, error) {
	var result releaseChainV5Prerequisite
	raw, err := readReleaseChainV5Artifact(highHistoryPath("release-chain-v10-high-prerequisite-20261013.json"), releaseChainV10ReviewedPrerequisiteSHA)
	if err != nil || !releaseChainV7ExactSemantic(raw, releaseChainV10PrerequisiteSemanticJSON) {
		return result, errNativeFixtureDenied
	}
	raw, err = readReleaseChainV5Artifact(highHistoryPath("release-chain-v10-user-policy-amendment-20261013.json"), releaseChainV10AuthoritySHA)
	if err != nil || !releaseChainV7ExactSemantic(raw, releaseChainV10AuthoritySemanticJSON) {
		return result, errNativeFixtureDenied
	}
	h, err := readReleaseChainV10ActualHistory()
	if err != nil {
		return result, errNativeFixtureDenied
	}
	result, err = readReleaseChainV5Prerequisite()
	if err != nil {
		return result, errNativeFixtureDenied
	}
	result.Baseline, result.ChainHistorySHA = h.NativeAttempts, h.BaselineSHA256
	return result, nil
}
func newReleaseChainV10Window(ctx context.Context, pool *pgxpool.Pool) (*releaseChainV5Window, error) {
	p, err := readReleaseChainV10Prerequisite()
	if err != nil {
		return nil, errNativeFixtureDenied
	}
	deadline, err := time.Parse(time.RFC3339, os.Getenv("BUTLER_SCENARIO_CHAIN_V10_HIGH_DEADLINE"))
	if err != nil {
		return nil, errNativeFixtureDenied
	}
	return createReleaseChainHighWindowWithPolicy(ctx, pool, releaseChainV10ReviewedPrerequisiteSHA, p.ChainHistorySHA, 556, "fixture_chain_v10_high", deadline, 71, true, releaseChainV9Policy(), true)
}
func TestScenarioReleaseChainActualV10HighE2E(t *testing.T) {
	if os.Getenv("BUTLER_SCENARIO_CHAIN_V10_HIGH_ACCEPTANCE") == "1" {
		t.Fatal("V10 native scope is sealed failed; no new entry from current source")
	}
	t.Skip("V10 native scope is sealed failed; no new entry from current source")
}
func TestReleaseChainV10OperationalGate(t *testing.T) {
	if os.Getenv("BUTLER_SCENARIO_CHAIN_V10_HIGH_GATE") != "1" {
		t.Skip("explicit pinned new scope required")
	}
	if _, err := readReleaseChainV10Prerequisite(); err != nil {
		t.Fatal("actual556/history/prerequisite frozen gate denied")
	}
}
