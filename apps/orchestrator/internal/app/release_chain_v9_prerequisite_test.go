package app

import (
	"context"
	"encoding/json"
	"os"
	"reflect"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

const releaseChainV9WindowID = "issue33-high-release-chain-v9-20261013-baseline544"
const releaseChainV9ReviewedPrerequisiteSHA = "4f753010b86b57ff2ea4cc8ff698d56d23787ab1ec20b572cb35ad329b1802c3"
const releaseChainV9AuthoritySHA = "83ca7491e4fc4a3673097f62f9590117e42387daadc656935318b5bd6caf4ebe"
const releaseChainV9HistoryArtifactSHA = "bf6ac2a9e0115a3a98036605b310c9cbd6637db99dd1c7f53870a75bf1900142"
const releaseChainV9HistoryFold = "7e397ba703ad12b37daacfd5406a57a93ed58237ca6ed6823932c788032fa8b2"
const releaseChainV9BridgeSHA = "ca18f5d5f3a78ac9d61ada16472399ecc3024801b8b077795fabbe3db9584650"
const releaseChainV9FailedCode = "5057fbf1a90e01952ea939c2c1fd7af13c3db6a6"
const releaseChainV9FailedTree = "5ca494878ceaaa942e2176fb41bbbcb0899761b9c7abf83da358be00a62184ec"
const releaseChainV9FailedApp = "031d960c4b26f2a055c974ec668e78132ed339fd04cb3b550af7fc891b758e97"
const releaseChainV9AuthoritySemanticJSON = `{"version":"butler-chain-v9-human-policy-amendment-v1","source":"direct_user_messages_in_current_task","user_messages":["Хорошо тогда чутьчуть ослабь требования и если прям критичных проблем не останется тогда можно закрывать пр","Ты можешь не делать сотни проверок перед проверкой на модели HIGH. У меня подписка и денег не списывается, потратить больше чем дано нельзя, смело тестируй модель","Так может убрать/увеличить таймаут, если модель думает, то пускай, какбудто лучше чтобы агент выполнил задачу чем сказал что не сможет. Или ты считаешь что слишком долго на такие задачи он тратит времени?"],"interpretation":"Continue the bounded native High chain from factual544. Advertise collection only after an actual current tested child return, retain the independent Broker authorization checks and original parent completion gates, and serialize parent and child provider entries in the isolated observer without holding database transactions during waiting or inference. Preserve every closed failure, unknown measurement, original proposal and safety guard.","window_id":"issue33-high-release-chain-v9-20261013-baseline544","baseline_attempts":544,"maximum_new_attempts":71,"maximum_total_attempts":615,"maximum_generation_tokens":32768,"maximum_attempt_ms":300000,"maximum_case_ms":2400000,"maximum_global_ms":7200000,"maximum_child_model_turns":4,"maximum_child_external_calls":4,"maximum_child_active_ms":1320000,"maximum_model_retries_per_input":1,"model_retry_backoff_ms":2000,"safe_timeout_stages":["http_round_trip","response_stream"],"keep_closed_537_and_539_unknown":true,"all_completed_entries_require_measured_usage_and_selected_version":true,"failed_unknown_requires_proved_safe_same_run_recovery":true,"critical_safety_or_original_result_criteria_changed":false,"quality_or_comparison_criteria_changed":false,"release_chain_latency_policy_changed":true,"independent_acceptance_required_before_ready_merge_closure":true,"native_window_reopened":false,"issue33_ready":false,"limitations":["This records the direct messages and the assistant's bounded operational choices; it is not a cryptographic human signature.","The new deadline does not establish that the model was thinking during an unanswered HTTP request.","Subscription-no-overage pricing does not supply missing token measurements or provider-side billing and runtime records."],"proposal_retention_required":true,"maximum_private_proposal_bytes":262144,"original_model_arguments_and_authorization_fingerprints_preserved":true,"provider_entries_serialized":true,"maximum_concurrent_native_entries":1}`
const releaseChainV9PrerequisiteSemanticJSON = `{"version":"butler-chain-high-reviewed-v9-prerequisite-v1","window_id":"issue33-high-release-chain-v9-20261013-baseline544","baseline_attempts":544,"maximum_new_attempts":71,"maximum_total_attempts":615,"prior_history_artifact_sha256":"bf6ac2a9e0115a3a98036605b310c9cbd6637db99dd1c7f53870a75bf1900142","prior_history_sha256":"7e397ba703ad12b37daacfd5406a57a93ed58237ca6ed6823932c788032fa8b2","retained_quality_prerequisite_sha256":"9480184b2f5586d8f9826b8db328c52a845ee812bd48afbf1432ca7f30bfd5d3","retained_quality_final_audit_sha256":"f8248d19ec2a0c222b39d46fbc46c0d0cf4ccc3c1361c89965b597eedbb21cab","prior_v6_prerequisite_sha256":"bef93bde0afa1251c67eb60f01dbbe6f8142b4f32b5a8076c0f34d5d9b402077","prior_v6_timeout_waiver_sha256":"44991353526966ab57cfd0155c3f65af00061b87308eed37c2b735f207e75180","historical_known_input_tokens_subtotal":348648,"historical_known_output_tokens_subtotal":13138,"historical_known_total_tokens_subtotal":647378,"historical_unknown_total_attempts":48,"historical_known_usage_pairs":516,"historical_unknown_usage_pairs":28,"provider_stage_observation":true,"capture_kernel_operation_id":true,"requested_model":"gemini-3.8-flash-high","observed_successful_model_version":"gemini-3.8-flash-n","maximum_generation_tokens":32768,"maximum_attempt_ms":300000,"maximum_case_ms":2400000,"maximum_global_ms":7200000,"allowed_provider_operation_stages":["http_round_trip","response_stream"],"maximum_model_retries_per_input":1,"model_retry_backoff_ms":2000,"all_completed_entries_require_measured_usage_and_selected_version":true,"failed_usage_shape":"valid_all_three_known_or_all_three_null","failed_model_version_shape":"selected_version_or_null","all_other_quality_source_authority_stop_effect_delivery_gates_retained":true,"issue33_ready":false,"human_policy_amendment_sha256":"83ca7491e4fc4a3673097f62f9590117e42387daadc656935318b5bd6caf4ebe","proposal_retention_required":true,"maximum_private_proposal_bytes":262144,"preserve_original_proposal_bytes_and_authorization_fingerprint":true,"actual544_bridge_sha256":"ca18f5d5f3a78ac9d61ada16472399ecc3024801b8b077795fabbe3db9584650","failed_v8_audit_sha256":"5573b9f25e13a4f3aefacacf142aeda7846b68962f8cd26df4498417097c580f","failed_v8_correlation_sha256":"2e1eed8eadb5874299693bd28ea9857fbd6d96ce9a2e1865009f2368403761d3","prior_v8_prerequisite_sha256":"effa65a6e39fcecce2510c76554685d047ee5e8c7ed1b9e6d27e633133d402ec","prior_v8_authority_sha256":"3560bb090a6b2c8b8bc78a987b2331eaf2eb55550b5e62572fbaf0bb7646973d","retained_closed_v8_proposal_diagnostic_sha256":"8d5b296bfacd45da1d578896435378e7151787ee4847f26d6953267da3af87d8","provider_entries_serialized":true,"maximum_concurrent_native_entries":1}`

func releaseChainV9FailedArtifacts() map[string]string {
	return map[string]string{
		"proposal-diagnostics": "8d5b296bfacd45da1d578896435378e7151787ee4847f26d6953267da3af87d8",
		"final-audit":          "5573b9f25e13a4f3aefacacf142aeda7846b68962f8cd26df4498417097c580f", "high": "d319637dcd6a74e80c7cdb18749e9c35501bd35c590daa7d72b934543a04f9e8", "pg-ledger": "504884085e50a77f45c8057c062b195deb29a756f5d868d6dbb9ed235b07c333", "pg-product": "534411ccfcefe90b40ea22a890335602c909f66ff64b85197231ae6d09e504b9", "pg-initial": "6e1b70738d86f7f5eed465e9bf5b46ff4bdf862f036fdb06cc90470e736c545a", "runtime-state": "f705bd247f2d3b25f855694186a8ce9ca214e360784f527fbeb1facf730f94c7", "failure-correlation": "2e1eed8eadb5874299693bd28ea9857fbd6d96ce9a2e1865009f2368403761d3",
	}
}
func readReleaseChainV9ActualHistory() (nativeHistoricalEvidence, error) {
	prior, err := readReleaseChainV8ActualHistory()
	if err != nil {
		return nativeHistoricalEvidence{}, errNativeFixtureDenied
	}
	raw, err := readReleaseChainV5Artifact(highHistoryPath("release-chain-v8-history544-20261013.json"), releaseChainV9HistoryArtifactSHA)
	var h nativeHistoricalEvidence
	if err != nil || json.Unmarshal(raw, &h) != nil {
		return h, errNativeFixtureDenied
	}
	raw, err = readReleaseChainV5Artifact(highHistoryPath("release-chain-v8-carry544-independent-20261013.json"), releaseChainV9BridgeSHA)
	var bridge map[string]any
	if err != nil || json.Unmarshal(raw, &bridge) != nil {
		return h, errNativeFixtureDenied
	}
	all := map[string]map[string]any{}
	pins := releaseChainV9FailedArtifacts()
	for kind, pin := range pins {
		raw, err = readReleaseChainV5Artifact(highHistoryPath("release-chain-v8-"+kind+"-20261013.json"), pin)
		if err != nil || !json.Valid(raw) {
			return h, errNativeFixtureDenied
		}
		if kind == "pg-product" || kind == "pg-initial" {
			var a []json.RawMessage
			if json.Unmarshal(raw, &a) != nil || len(a) != 1 {
				return h, errNativeFixtureDenied
			}
			continue
		}
		var m map[string]any
		if json.Unmarshal(raw, &m) != nil {
			return h, errNativeFixtureDenied
		}
		all[kind] = m
	}
	if h.NativeAttempts != 544 || len(h.Usage) != 544 || h.Status != prior.Status || h.KnownUsagePairs != 516 || h.UnknownUsagePairs != 28 || h.BaselineSHA256 != releaseChainV9HistoryFold || !reflect.DeepEqual(h.Usage[:541], prior.Usage) || len(h.Artifacts) != len(prior.Artifacts)+1 || !reflect.DeepEqual(h.Artifacts[:len(prior.Artifacts)], prior.Artifacts) || h.Artifacts[len(prior.Artifacts)] != (nativePriorArtifact{ReportSHA256: pins["final-audit"], ReleaseRef: releaseChainV9FailedCode, NewAttempts: 3}) {
		return h, errNativeFixtureDenied
	}
	var in, out int64
	for _, u := range h.Usage {
		if (u.ActualInputTokens == nil) != (u.ActualOutputTokens == nil) {
			return h, errNativeFixtureDenied
		}
		if u.ActualInputTokens != nil {
			in += *u.ActualInputTokens
			out += *u.ActualOutputTokens
		}
	}
	if in != 348648 || out != 13138 {
		return h, errNativeFixtureDenied
	}
	vector := []string{prior.BaselineSHA256, pins["final-audit"], pins["high"], pins["pg-ledger"], pins["pg-product"], pins["pg-initial"], pins["runtime-state"], pins["failure-correlation"], releaseChainV9FailedCode, releaseChainV9FailedTree, releaseChainV9FailedApp}
	var actual []string
	for _, v := range releaseChainV6Array(bridge, "fold_vector") {
		s, ok := v.(string)
		if !ok {
			return h, errNativeFixtureDenied
		}
		actual = append(actual, s)
	}
	if !reflect.DeepEqual(vector, actual) || shadowV4Hash(vector) != releaseChainV9HistoryFold || !releaseChainV6Exact(bridge, map[string]any{"status": "INDEPENDENT_SEALED544_CARRY_BRIDGE", "fold_sha256": releaseChainV9HistoryFold, "prospective_model_admission": false, "old_originals_or_measurements_changed": false, "all_new_expense_measured": true, "issue33_ready": false}) {
		return h, errNativeFixtureDenied
	}
	c, s, pg, f := all["failure-correlation"], all["high"], all["pg-ledger"], all["final-audit"]
	if !releaseChainV6Exact(c, map[string]any{"status": "INDEPENDENT_ACTUAL_V8_CHAIN_NO_GO_CORRELATED", "code_commit": releaseChainV9FailedCode, "source_sha256": releaseChainV9FailedTree, "native_scope_sealed": true, "native_request_count": float64(3), "actual_original_result": nil, "actual_notice_count": float64(0), "actual_channel_requests": float64(0), "actual_model_retry": nil, "accepted_model_retry_failures": float64(0), "issue33_ready": false}) || !releaseChainV6Exact(s, map[string]any{"status": "failed", "baseline_attempts": float64(541), "maximum_new_attempts": float64(74), "maximum_total_attempts": float64(615), "issue33_ready": false}) || !releaseChainV6Exact(releaseChainV6Object(f, "chain_audit"), map[string]any{"status": "NO_GO", "carry_forward_attempts": float64(544)}) {
		return h, errNativeFixtureDenied
	}
	scope := releaseChainV6Object(s, "scope")
	if !reflect.DeepEqual(scope["window"], pg["window"]) || !releaseChainV6Exact(scope, map[string]any{"total_attempts": float64(544), "new_attempts": float64(3), "unknown_full_token_cost": float64(0), "failed_provider_entries": float64(0)}) {
		return h, errNativeFixtureDenied
	}
	rows, sqlRows := releaseChainV6Array(scope, "requests"), releaseChainV6Array(pg, "requests")
	if len(rows) != 3 || len(sqlRows) != 3 {
		return h, errNativeFixtureDenied
	}
	for i, item := range rows {
		row, ok := item.(map[string]any)
		sql, sok := sqlRows[i].(map[string]any)
		if !ok || !sok {
			return h, errNativeFixtureDenied
		}
		canonical, e := releaseChainV6CanonicalRow(sql)
		if e != nil || !reflect.DeepEqual(canonical, row) || !releaseChainV6Exact(row, map[string]any{"ordinal": float64(542 + i), "outcome": "completed", "provider_operation_stage": "response_stream", "model_version": "gemini-3.8-flash-n", "scenario": "normal_sent", "is_parent": true, "final_count": float64(0), "terminal_count": float64(0), "cancelled_count": float64(0), "timed_out_count": float64(0), "transport_error_count": float64(0)}) {
			return h, errNativeFixtureDenied
		}
		u := h.Usage[541+i]
		if u.RunID != row["run_id"] || u.ActualInputTokens == nil || u.ActualOutputTokens == nil || float64(*u.ActualInputTokens) != row["actual_input_tokens"] || float64(*u.ActualOutputTokens) != row["actual_output_tokens"] {
			return h, errNativeFixtureDenied
		}
	}
	if !reflect.DeepEqual(c["carry_forward"], bridge["carry_forward"]) || !releaseChainV6Exact(releaseChainV6Object(c, "carry_forward"), map[string]any{"attempts": float64(544), "known_input_tokens_subtotal": float64(348648), "known_output_tokens_subtotal": float64(13138), "known_total_tokens_subtotal": float64(647378), "unknown_total_attempts": float64(48), "old_unknown_266_cause": "UNKNOWN"}) || !releaseChainV6Exact(releaseChainV6Object(c, "failure_classification"), map[string]any{"specific_prior541_cause": "UNKNOWN_FROM_CURRENT_SAFE_EXPORTS", "child_pre_native_failure_reason": "UNKNOWN_FROM_CURRENT_SAFE_EXPORTS", "ordinal544_original_child_reference_equals_actual_admitted_child": true, "ordinal544_child_candidate_or_final_ready": false}) {
		return h, errNativeFixtureDenied
	}
	diagnostic := all["proposal-diagnostics"]
	if !releaseChainV6Exact(diagnostic, map[string]any{"status": "INDEPENDENT_THREE_ORIGINAL_PROPOSALS_CORRELATED_CHAIN_NO_GO", "ordinal544_requested_child_equals_actual_admitted_child": true, "ordinal544_action_intent_absent": true, "specific_prior541_cause": "UNKNOWN", "all_model_argument_values_remain_private": true, "verified_scenarios": float64(0)}) {
		return h, errNativeFixtureDenied
	}
	proofs := releaseChainV6Array(diagnostic, "proposals")
	if len(proofs) != 3 {
		return h, errNativeFixtureDenied
	}
	for i, item := range proofs {
		proof, ok := item.(map[string]any)
		if !ok || !releaseChainV6Exact(proof, map[string]any{"ordinal": float64(542 + i), "original_bytes_and_base64_match": true, "original_authorization_hash_matches_metadata": true, "actual_core_authorized_action_match": i < 2, "model_argument_values_emitted": false}) {
			return h, errNativeFixtureDenied
		}
	}
	return h, nil
}
func readReleaseChainV9Prerequisite() (releaseChainV5Prerequisite, error) {
	var result releaseChainV5Prerequisite
	if !nativeAuditHash.MatchString(releaseChainV9ReviewedPrerequisiteSHA) {
		return result, errNativeFixtureDenied
	}
	raw, err := readReleaseChainV5Artifact(highHistoryPath("release-chain-v9-high-prerequisite-20261013.json"), releaseChainV9ReviewedPrerequisiteSHA)
	if err != nil || !releaseChainV7ExactSemantic(raw, releaseChainV9PrerequisiteSemanticJSON) {
		return result, errNativeFixtureDenied
	}
	raw, err = readReleaseChainV5Artifact(highHistoryPath("release-chain-v9-user-policy-amendment-20261013.json"), releaseChainV9AuthoritySHA)
	if err != nil || !releaseChainV7ExactSemantic(raw, releaseChainV9AuthoritySemanticJSON) {
		return result, errNativeFixtureDenied
	}
	h, err := readReleaseChainV9ActualHistory()
	if err != nil {
		return result, errNativeFixtureDenied
	}
	result, err = readReleaseChainV5Prerequisite()
	if err != nil {
		return result, errNativeFixtureDenied
	}
	result.Baseline = h.NativeAttempts
	result.ChainHistorySHA = h.BaselineSHA256
	return result, nil
}
func newReleaseChainV9Window(ctx context.Context, pool *pgxpool.Pool) (*releaseChainV5Window, error) {
	p, err := readReleaseChainV9Prerequisite()
	if err != nil {
		return nil, errNativeFixtureDenied
	}
	deadline, err := time.Parse(time.RFC3339, os.Getenv("BUTLER_SCENARIO_CHAIN_V9_HIGH_DEADLINE"))
	if err != nil {
		return nil, errNativeFixtureDenied
	}
	return createReleaseChainHighWindowWithPolicy(ctx, pool, releaseChainV9ReviewedPrerequisiteSHA, p.ChainHistorySHA, 544, "fixture_chain_v9_high", deadline, 71, true, releaseChainV9Policy(), true)
}
func TestScenarioReleaseChainActualV9HighE2E(t *testing.T) {
	if os.Getenv("BUTLER_SCENARIO_CHAIN_V9_HIGH_ACCEPTANCE") == "1" {
		t.Fatal("V9 native scope is sealed failed; no new entry from current source")
	}
	t.Skip("V9 native scope is sealed failed; no new entry from current source")
}
func TestReleaseChainV9OperationalGate(t *testing.T) {
	if os.Getenv("BUTLER_SCENARIO_CHAIN_V9_HIGH_GATE") != "1" {
		t.Skip("explicit new pinned scope required")
	}
	if _, err := readReleaseChainV9Prerequisite(); err != nil {
		t.Fatal("V8 review/pin not frozen")
	}
}
func TestReleaseChainV9ActualHistoryPreserves544(t *testing.T) {
	h, err := readReleaseChainV9ActualHistory()
	if err != nil {
		t.Fatal("actual544 bridge denied", err)
	}
	if h.NativeAttempts != 544 || h.Usage[536].ActualInputTokens != nil || h.Usage[538].ActualInputTokens != nil {
		t.Fatal("historical UNKNOWN refunded or inferred")
	}
	if releaseChainV9ReviewedPrerequisiteSHA == "" {
		if _, err = readReleaseChainV9Prerequisite(); err == nil {
			t.Fatal("history authorized an unfrozen scope")
		}
	}
}
