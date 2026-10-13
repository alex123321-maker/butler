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

const releaseChainV8WindowID = "issue33-high-release-chain-v8-20261013-baseline541"
const releaseChainV8ReviewedPrerequisiteSHA = "effa65a6e39fcecce2510c76554685d047ee5e8c7ed1b9e6d27e633133d402ec"
const releaseChainV8AuthoritySHA = "3560bb090a6b2c8b8bc78a987b2331eaf2eb55550b5e62572fbaf0bb7646973d"
const releaseChainV8HistoryArtifactSHA = "ebbc1a11df409a22c58e2b602fb60f5614c4ee7d71e0c0b9e13f9c055113b079"
const releaseChainV8HistoryFold = "c74760a2d757f14e9f953bf2195fb557334a9910f8da62cbb3699c96a8758332"
const releaseChainV8BridgeSHA = "a87d3628ae367507838a8d4fac3da937c39837e6e048ebd3ccaf7d5bc7361cef"
const releaseChainV8FailedCode = "6d874f6625af7f21ac9758dbea9b287b4d5252ea"
const releaseChainV8FailedTree = "0586a361fa246066244ffdcdce3269a2ce600ebf2bf35625383d551f0afd0e3c"
const releaseChainV8FailedApp = "97f6ad4ce71fc067f48555cadcba1ef68a92d98878d0a2694ff5484e6d731c51"
const releaseChainV8AuthoritySemanticJSON = `{"version":"butler-chain-v8-human-policy-amendment-v1","source":"direct_user_messages_in_current_task","user_messages":["Хорошо тогда чутьчуть ослабь требования и если прям критичных проблем не останется тогда можно закрывать пр","Ты можешь не делать сотни проверок перед проверкой на модели HIGH. У меня подписка и денег не списывается, потратить больше чем дано нельзя, смело тестируй модель","Так может убрать/увеличить таймаут, если модель думает, то пускай, какбудто лучше чтобы агент выполнил задачу чем сказал что не сможет. Или ты считаешь что слишком долго на такие задачи он тратит времени?"],"interpretation":"Continue the bounded native High chain with the same longer deadlines and safe timeout recovery. Correct the confirmed schema versus decoder mismatch losslessly at the typed argument boundary, retain exact original tool proposal bytes privately before release, and keep every closed failure, unknown measurement and safety guard.","window_id":"issue33-high-release-chain-v8-20261013-baseline541","baseline_attempts":541,"maximum_new_attempts":74,"maximum_total_attempts":615,"maximum_generation_tokens":32768,"maximum_attempt_ms":300000,"maximum_case_ms":2400000,"maximum_global_ms":7200000,"maximum_child_model_turns":4,"maximum_child_external_calls":4,"maximum_child_active_ms":1320000,"maximum_model_retries_per_input":1,"model_retry_backoff_ms":2000,"safe_timeout_stages":["http_round_trip","response_stream"],"keep_closed_537_and_539_unknown":true,"all_completed_entries_require_measured_usage_and_selected_version":true,"failed_unknown_requires_proved_safe_same_run_recovery":true,"critical_safety_or_original_result_criteria_changed":false,"quality_or_comparison_criteria_changed":false,"release_chain_latency_policy_changed":true,"independent_acceptance_required_before_ready_merge_closure":true,"native_window_reopened":false,"issue33_ready":false,"limitations":["This records the direct messages and the assistant's bounded operational choices; it is not a cryptographic human signature.","The new deadline does not establish that the model was thinking during an unanswered HTTP request.","Subscription-no-overage pricing does not supply missing token measurements or provider-side billing and runtime records."],"proposal_retention_required":true,"maximum_private_proposal_bytes":262144,"original_model_arguments_and_authorization_fingerprints_preserved":true}`
const releaseChainV8PrerequisiteSemanticJSON = `{"version":"butler-chain-high-reviewed-v8-prerequisite-v1","window_id":"issue33-high-release-chain-v8-20261013-baseline541","baseline_attempts":541,"maximum_new_attempts":74,"maximum_total_attempts":615,"prior_history_artifact_sha256":"ebbc1a11df409a22c58e2b602fb60f5614c4ee7d71e0c0b9e13f9c055113b079","prior_history_sha256":"c74760a2d757f14e9f953bf2195fb557334a9910f8da62cbb3699c96a8758332","retained_quality_prerequisite_sha256":"9480184b2f5586d8f9826b8db328c52a845ee812bd48afbf1432ca7f30bfd5d3","retained_quality_final_audit_sha256":"f8248d19ec2a0c222b39d46fbc46c0d0cf4ccc3c1361c89965b597eedbb21cab","prior_v6_prerequisite_sha256":"bef93bde0afa1251c67eb60f01dbbe6f8142b4f32b5a8076c0f34d5d9b402077","prior_v6_timeout_waiver_sha256":"44991353526966ab57cfd0155c3f65af00061b87308eed37c2b735f207e75180","historical_known_input_tokens_subtotal":318461,"historical_known_output_tokens_subtotal":12525,"historical_known_total_tokens_subtotal":611404,"historical_unknown_total_attempts":48,"historical_known_usage_pairs":513,"historical_unknown_usage_pairs":28,"provider_stage_observation":true,"capture_kernel_operation_id":true,"requested_model":"gemini-3.8-flash-high","observed_successful_model_version":"gemini-3.8-flash-n","maximum_generation_tokens":32768,"maximum_attempt_ms":300000,"maximum_case_ms":2400000,"maximum_global_ms":7200000,"allowed_provider_operation_stages":["http_round_trip","response_stream"],"maximum_model_retries_per_input":1,"model_retry_backoff_ms":2000,"all_completed_entries_require_measured_usage_and_selected_version":true,"failed_usage_shape":"valid_all_three_known_or_all_three_null","failed_model_version_shape":"selected_version_or_null","all_other_quality_source_authority_stop_effect_delivery_gates_retained":true,"issue33_ready":false,"human_policy_amendment_sha256":"3560bb090a6b2c8b8bc78a987b2331eaf2eb55550b5e62572fbaf0bb7646973d","actual541_bridge_sha256":"a87d3628ae367507838a8d4fac3da937c39837e6e048ebd3ccaf7d5bc7361cef","failed_v7b_audit_sha256":"a75e06331c6151a79726e94559fbe3c48f84e1af23dc0a8351d78ce3db517c28","failed_v7b_correlation_sha256":"d6d2aa53d46d54ec2efd32ad97be6e5594093545002d1693c315c631366ee12d","prior_v7b_prerequisite_sha256":"fc4f5cfd6fa7209f212cb47c767c4f80c60d304fb3c1513371c27008850636a6","proposal_retention_required":true,"maximum_private_proposal_bytes":262144,"preserve_original_proposal_bytes_and_authorization_fingerprint":true}`

func releaseChainV8FailedArtifacts() map[string]string {
	return map[string]string{
		"final-audit": "a75e06331c6151a79726e94559fbe3c48f84e1af23dc0a8351d78ce3db517c28", "high": "d954dfce56bca311f3b83ea427d709fd87cb0a7c14ddf385462c80affce982d2", "pg-ledger": "6f75edc22c093bba57fbde3b978edfb93177e6ad5c761ce868e8f56f6a8e8ed3", "pg-product": "f6eef31876d685fb1f778484f25d99b8c25cfa41565e6b71db70c308f2af3884", "pg-initial": "69077eea2e57bcd8cc90a37f6f1cc56ad5921877f4c5c26afa3f58d285d9c392", "runtime-state": "605eb28f639803df22ba43a24af289c263a0278f017a08ffa3bfd70fed779871", "failure-correlation": "d6d2aa53d46d54ec2efd32ad97be6e5594093545002d1693c315c631366ee12d",
	}
}
func readReleaseChainV8ActualHistory() (nativeHistoricalEvidence, error) {
	prior, err := readReleaseChainV7ActualHistory()
	if err != nil {
		return nativeHistoricalEvidence{}, errNativeFixtureDenied
	}
	raw, err := readReleaseChainV5Artifact(highHistoryPath("release-chain-v7b-history541-20261013.json"), releaseChainV8HistoryArtifactSHA)
	var h nativeHistoricalEvidence
	if err != nil || json.Unmarshal(raw, &h) != nil {
		return h, errNativeFixtureDenied
	}
	raw, err = readReleaseChainV5Artifact(highHistoryPath("release-chain-v7b-carry541-independent-20261013.json"), releaseChainV8BridgeSHA)
	var bridge map[string]any
	if err != nil || json.Unmarshal(raw, &bridge) != nil {
		return h, errNativeFixtureDenied
	}
	all := map[string]map[string]any{}
	pins := releaseChainV8FailedArtifacts()
	for kind, pin := range pins {
		raw, err = readReleaseChainV5Artifact(highHistoryPath("release-chain-v7b-"+kind+"-20261013.json"), pin)
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
	if h.NativeAttempts != 541 || len(h.Usage) != 541 || h.Status != prior.Status || h.KnownUsagePairs != 513 || h.UnknownUsagePairs != 28 || h.BaselineSHA256 != releaseChainV8HistoryFold || !reflect.DeepEqual(h.Usage[:539], prior.Usage) || len(h.Artifacts) != len(prior.Artifacts)+1 || !reflect.DeepEqual(h.Artifacts[:len(prior.Artifacts)], prior.Artifacts) || h.Artifacts[len(prior.Artifacts)] != (nativePriorArtifact{ReportSHA256: pins["final-audit"], ReleaseRef: releaseChainV8FailedCode, NewAttempts: 2}) {
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
	if in != 318461 || out != 12525 {
		return h, errNativeFixtureDenied
	}
	vector := []string{prior.BaselineSHA256, pins["final-audit"], pins["high"], pins["pg-ledger"], pins["pg-product"], pins["pg-initial"], pins["runtime-state"], pins["failure-correlation"], releaseChainV8FailedCode, releaseChainV8FailedTree, releaseChainV8FailedApp}
	var actual []string
	for _, v := range releaseChainV6Array(bridge, "fold_vector") {
		s, ok := v.(string)
		if !ok {
			return h, errNativeFixtureDenied
		}
		actual = append(actual, s)
	}
	if !reflect.DeepEqual(vector, actual) || shadowV4Hash(vector) != releaseChainV8HistoryFold || !releaseChainV6Exact(bridge, map[string]any{"status": "INDEPENDENT_SEALED541_CARRY_BRIDGE", "fold_sha256": releaseChainV8HistoryFold, "prospective_model_admission": false, "old_originals_or_measurements_changed": false, "all_new_expense_measured": true, "issue33_ready": false}) {
		return h, errNativeFixtureDenied
	}
	c, s, pg, f := all["failure-correlation"], all["high"], all["pg-ledger"], all["final-audit"]
	if !releaseChainV6Exact(c, map[string]any{"status": "INDEPENDENT_ACTUAL_V7B_CHAIN_NO_GO_CORRELATED", "code_commit": releaseChainV8FailedCode, "source_sha256": releaseChainV8FailedTree, "native_scope_sealed": true, "native_request_count": float64(2), "actual_original_result": nil, "actual_notice_count": float64(0), "actual_channel_requests": float64(0), "actual_model_retry": nil, "accepted_model_retry_failures": float64(0), "issue33_ready": false}) || !releaseChainV6Exact(s, map[string]any{"status": "failed", "baseline_attempts": float64(539), "maximum_new_attempts": float64(76), "maximum_total_attempts": float64(615), "issue33_ready": false}) || !releaseChainV6Exact(releaseChainV6Object(f, "chain_audit"), map[string]any{"status": "NO_GO", "carry_forward_attempts": float64(541)}) {
		return h, errNativeFixtureDenied
	}
	scope := releaseChainV6Object(s, "scope")
	if !reflect.DeepEqual(scope["window"], pg["window"]) || !releaseChainV6Exact(scope, map[string]any{"total_attempts": float64(541), "new_attempts": float64(2), "unknown_full_token_cost": float64(0), "failed_provider_entries": float64(0)}) {
		return h, errNativeFixtureDenied
	}
	rows, sqlRows := releaseChainV6Array(scope, "requests"), releaseChainV6Array(pg, "requests")
	if len(rows) != 2 || len(sqlRows) != 2 {
		return h, errNativeFixtureDenied
	}
	for i, item := range rows {
		row, ok := item.(map[string]any)
		sql, sok := sqlRows[i].(map[string]any)
		if !ok || !sok {
			return h, errNativeFixtureDenied
		}
		canonical, e := releaseChainV6CanonicalRow(sql)
		if e != nil || !reflect.DeepEqual(canonical, row) || !releaseChainV6Exact(row, map[string]any{"ordinal": float64(540 + i), "outcome": "completed", "provider_operation_stage": "response_stream", "model_version": "gemini-3.8-flash-n", "scenario": "normal_sent", "is_parent": true, "final_count": float64(0), "terminal_count": float64(0), "cancelled_count": float64(0), "timed_out_count": float64(0), "transport_error_count": float64(0)}) {
			return h, errNativeFixtureDenied
		}
		u := h.Usage[539+i]
		if u.RunID != row["run_id"] || u.ActualInputTokens == nil || u.ActualOutputTokens == nil || float64(*u.ActualInputTokens) != row["actual_input_tokens"] || float64(*u.ActualOutputTokens) != row["actual_output_tokens"] {
			return h, errNativeFixtureDenied
		}
	}
	if !reflect.DeepEqual(c["carry_forward"], bridge["carry_forward"]) || !releaseChainV6Exact(releaseChainV6Object(c, "carry_forward"), map[string]any{"attempts": float64(541), "known_input_tokens_subtotal": float64(318461), "known_output_tokens_subtotal": float64(12525), "known_total_tokens_subtotal": float64(611404), "unknown_total_attempts": float64(48), "old_unknown_266_cause": "UNKNOWN"}) || !releaseChainV6Exact(releaseChainV6Object(c, "failure_classification"), map[string]any{"specific_rejected_argument_or_contract_cause": "UNKNOWN_FROM_CURRENT_SAFE_EXPORTS"}) {
		return h, errNativeFixtureDenied
	}
	return h, nil
}
func readReleaseChainV8Prerequisite() (releaseChainV5Prerequisite, error) {
	var result releaseChainV5Prerequisite
	if !nativeAuditHash.MatchString(releaseChainV8ReviewedPrerequisiteSHA) {
		return result, errNativeFixtureDenied
	}
	raw, err := readReleaseChainV5Artifact(highHistoryPath("release-chain-v8-high-prerequisite-20261013.json"), releaseChainV8ReviewedPrerequisiteSHA)
	if err != nil || !releaseChainV7ExactSemantic(raw, releaseChainV8PrerequisiteSemanticJSON) {
		return result, errNativeFixtureDenied
	}
	raw, err = readReleaseChainV5Artifact(highHistoryPath("release-chain-v8-user-policy-amendment-20261013.json"), releaseChainV8AuthoritySHA)
	if err != nil || !releaseChainV7ExactSemantic(raw, releaseChainV8AuthoritySemanticJSON) {
		return result, errNativeFixtureDenied
	}
	h, err := readReleaseChainV8ActualHistory()
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
func newReleaseChainV8Window(ctx context.Context, pool *pgxpool.Pool) (*releaseChainV5Window, error) {
	p, err := readReleaseChainV8Prerequisite()
	if err != nil {
		return nil, errNativeFixtureDenied
	}
	deadline, err := time.Parse(time.RFC3339, os.Getenv("BUTLER_SCENARIO_CHAIN_V8_HIGH_DEADLINE"))
	if err != nil {
		return nil, errNativeFixtureDenied
	}
	return createReleaseChainHighWindowWithPolicy(ctx, pool, releaseChainV8ReviewedPrerequisiteSHA, p.ChainHistorySHA, 541, "fixture_chain_v8_high", deadline, 74, true, releaseChainV8Policy(), true)
}
func TestScenarioReleaseChainActualV8HighE2E(t *testing.T) {
	if os.Getenv("BUTLER_SCENARIO_CHAIN_V8_HIGH_ACCEPTANCE") == "1" {
		t.Fatal("V8 native scope is sealed failed; no new entry from current source")
	}
	t.Skip("V8 native scope is sealed failed; no new entry from current source")
}
func TestReleaseChainV8OperationalGate(t *testing.T) {
	if os.Getenv("BUTLER_SCENARIO_CHAIN_V8_HIGH_GATE") != "1" {
		t.Skip("explicit new pinned scope required")
	}
	if _, err := readReleaseChainV8Prerequisite(); err != nil {
		t.Fatal("V8 review/pin not frozen")
	}
}
func TestReleaseChainV8ActualHistoryPreserves541(t *testing.T) {
	h, err := readReleaseChainV8ActualHistory()
	if err != nil {
		t.Fatal("actual541 bridge denied", err)
	}
	if h.NativeAttempts != 541 || h.Usage[536].ActualInputTokens != nil || h.Usage[538].ActualInputTokens != nil {
		t.Fatal("historical UNKNOWN refunded or inferred")
	}
	if releaseChainV8ReviewedPrerequisiteSHA == "" {
		if _, err = readReleaseChainV8Prerequisite(); err == nil {
			t.Fatal("history authorized an unfrozen scope")
		}
	}
}
