package app

import (
	"context"
	"encoding/json"
	"os"
	"reflect"
	"strconv"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

const releaseChainV7WindowID = "issue33-high-release-chain-v7-20261013-baseline539"
const releaseChainV7ReviewedPrerequisiteSHA = "9a689c4a535c36524868d00a11f00a2a259d868b95d70fe2d64f0a852476e860"
const releaseChainV7HistoryArtifactSHA = "f5424aa422a1f018c7bf4b7e13684e9ea86d6a68f316c8f82e31e00443228f1d"
const releaseChainV7HistoryFold = "a8a0d81d750e08c7ba2c4ddd70d312a3d3776657b950a2e36a583041d0a24261"
const releaseChainV7CarrySHA = "a7f0dc24d6d30db0713a2f03bfc82d20ec03ab0fd0bed6bb2dbf05b061cc0c6d"
const releaseChainV7AuthorizationSHA = "169430ff19321b029196763a8aeae388a9d1d13c585ef8d07e59bbf55cdbf6e8"
const releaseChainV7FailedCode = "829d77582691b7e80a4fcbd6279b8db3c1046669"
const releaseChainV7FailedTree = "dbb3737b32824e9324ff82b34975e5a4b9c60ba5a676b965022b948e741ad017"
const releaseChainV7FailedApp = "b3601b408ffac951022d543906e92cc61abfeebbb9514dca0b6001ad8edf3a9f"
const releaseChainV7AuthorizationSemanticJSON = `{"version":"butler-chain-v7-human-policy-amendment-v1","source":"direct_user_messages_in_current_task","user_messages":["Хорошо тогда чутьчуть ослабь требования и если прям критичных проблем не останется тогда можно закрывать пр","Ты можешь не делать сотни проверок перед проверкой на модели HIGH. У меня подписка и денег не списывается, потратить больше чем дано нельзя, смело тестируй модель","Так может убрать/увеличить таймаут, если модель думает, то пускай, какбудто лучше чтобы агент выполнил задачу чем сказал что не сможет. Или ты считаешь что слишком долго на такие задачи он тратит времени?"],"interpretation":"Allow a new bounded native High chain with longer deadlines and one safe HTTP or response-stream timeout retry; retain every closed failure and unknown measurement. Reuse independently verified unchanged checks and run narrow checks of changed guards before model execution.","window_id":"issue33-high-release-chain-v7-20261013-baseline539","baseline_attempts":539,"maximum_new_attempts":76,"maximum_total_attempts":615,"maximum_generation_tokens":32768,"maximum_attempt_ms":300000,"maximum_case_ms":2400000,"maximum_global_ms":7200000,"maximum_child_model_turns":4,"maximum_child_external_calls":4,"maximum_child_active_ms":1320000,"maximum_model_retries_per_input":1,"model_retry_backoff_ms":2000,"safe_timeout_stages":["http_round_trip","response_stream"],"keep_closed_537_and_539_unknown":true,"all_completed_entries_require_measured_usage_and_selected_version":true,"failed_unknown_requires_proved_safe_same_run_recovery":true,"critical_safety_or_original_result_criteria_changed":false,"quality_or_comparison_criteria_changed":false,"release_chain_latency_policy_changed":true,"independent_acceptance_required_before_ready_merge_closure":true,"native_window_reopened":false,"issue33_ready":false,"limitations":["This records the direct messages and the assistant's bounded operational choices; it is not a cryptographic human signature.","The new deadline does not establish that the model was thinking during an unanswered HTTP request.","Subscription-no-overage pricing does not supply missing token measurements or provider-side billing and runtime records."]}`
const releaseChainV7PrerequisiteSemanticJSON = `{"version":"butler-chain-high-reviewed-v7-prerequisite-v1","window_id":"issue33-high-release-chain-v7-20261013-baseline539","baseline_attempts":539,"maximum_new_attempts":76,"maximum_total_attempts":615,"prior_history_artifact_sha256":"f5424aa422a1f018c7bf4b7e13684e9ea86d6a68f316c8f82e31e00443228f1d","prior_history_sha256":"a8a0d81d750e08c7ba2c4ddd70d312a3d3776657b950a2e36a583041d0a24261","actual539_bridge_sha256":"a7f0dc24d6d30db0713a2f03bfc82d20ec03ab0fd0bed6bb2dbf05b061cc0c6d","failed_v6_audit_sha256":"339e54f39e51c43972c54b87d2d335f4eefa8340ac6d51cfad05b8391772a0c2","failed_v6_correlation_sha256":"5d809525016869a2199339c3f8188594f643009448bb640caef37552692cfb2e","retained_quality_prerequisite_sha256":"9480184b2f5586d8f9826b8db328c52a845ee812bd48afbf1432ca7f30bfd5d3","retained_quality_final_audit_sha256":"f8248d19ec2a0c222b39d46fbc46c0d0cf4ccc3c1361c89965b597eedbb21cab","prior_v6_prerequisite_sha256":"bef93bde0afa1251c67eb60f01dbbe6f8142b4f32b5a8076c0f34d5d9b402077","prior_v6_timeout_waiver_sha256":"44991353526966ab57cfd0155c3f65af00061b87308eed37c2b735f207e75180","historical_known_input_tokens_subtotal":301714,"historical_known_output_tokens_subtotal":11982,"historical_known_total_tokens_subtotal":580620,"historical_unknown_total_attempts":48,"historical_known_usage_pairs":511,"historical_unknown_usage_pairs":28,"provider_stage_observation":true,"capture_kernel_operation_id":true,"requested_model":"gemini-3.8-flash-high","observed_successful_model_version":"gemini-3.8-flash-n","maximum_generation_tokens":32768,"maximum_attempt_ms":300000,"maximum_case_ms":2400000,"maximum_global_ms":7200000,"allowed_provider_operation_stages":["http_round_trip","response_stream"],"maximum_model_retries_per_input":1,"model_retry_backoff_ms":2000,"all_completed_entries_require_measured_usage_and_selected_version":true,"failed_usage_shape":"valid_all_three_known_or_all_three_null","failed_model_version_shape":"selected_version_or_null","all_other_quality_source_authority_stop_effect_delivery_gates_retained":true,"issue33_ready":false,"human_policy_amendment_sha256":"169430ff19321b029196763a8aeae388a9d1d13c585ef8d07e59bbf55cdbf6e8"}`

func releaseChainV7FailedArtifacts() map[string]string {
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
func releaseChainV7ExactSemantic(raw []byte, frozen string) bool {
	var actual, expected map[string]any
	return json.Unmarshal(raw, &actual) == nil && json.Unmarshal([]byte(frozen), &expected) == nil && reflect.DeepEqual(actual, expected)
}
func readReleaseChainV7Authorization() error {
	raw, err := readReleaseChainV5Artifact(highHistoryPath("release-chain-v7-user-policy-amendment-20261013.json"), releaseChainV7AuthorizationSHA)
	if err != nil || !releaseChainV7ExactSemantic(raw, releaseChainV7AuthorizationSemanticJSON) {
		return errNativeFixtureDenied
	}
	return nil
}

// Reuse the sealed535/537 reader; append only the two exact actual native rows.
// The quoted third Core reservation had no native dispatch and is never counted
// as attempt540 or used to reconstruct missing539 measurements.
func readReleaseChainV7ActualHistory() (nativeHistoricalEvidence, error) {
	prior, err := readReleaseChainV6ActualHistory()
	if err != nil {
		return nativeHistoricalEvidence{}, errNativeFixtureDenied
	}
	raw, err := readReleaseChainV5Artifact(highHistoryPath("release-chain-v7-prior-history539-20261013.json"), releaseChainV7HistoryArtifactSHA)
	var h nativeHistoricalEvidence
	if err != nil || json.Unmarshal(raw, &h) != nil {
		return h, errNativeFixtureDenied
	}
	var bridge map[string]any
	raw, err = readReleaseChainV5Artifact(highHistoryPath("release-chain-v7-carry539-independent-20261013.json"), releaseChainV7CarrySHA)
	if err != nil || json.Unmarshal(raw, &bridge) != nil {
		return h, errNativeFixtureDenied
	}
	all := map[string]map[string]any{}
	for kind, pin := range releaseChainV7FailedArtifacts() {
		raw, err = readReleaseChainV5Artifact(highHistoryPath("release-chain-v6-"+kind+"-20261013.json"), pin)
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
	if validateReleaseChainV7ActualHistory(prior, h, bridge, all) != nil {
		return h, errNativeFixtureDenied
	}
	return h, nil
}
func validateReleaseChainV7ActualHistory(prior, h nativeHistoricalEvidence, bridge map[string]any, all map[string]map[string]any) error {
	if prior.NativeAttempts != 537 || h.Status != prior.Status || h.NativeAttempts != 539 || len(h.Usage) != 539 || h.KnownUsagePairs != 511 || h.UnknownUsagePairs != 28 || h.BaselineSHA256 != releaseChainV7HistoryFold || !reflect.DeepEqual(h.Usage[:537], prior.Usage) || len(h.Artifacts) != len(prior.Artifacts)+1 || !reflect.DeepEqual(h.Artifacts[:len(prior.Artifacts)], prior.Artifacts) {
		return errNativeFixtureDenied
	}
	pins := releaseChainV7FailedArtifacts()
	expectedArtifact := nativePriorArtifact{ReportSHA256: pins["final-audit"], ReleaseRef: releaseChainV7FailedCode, NewAttempts: 2}
	if h.Artifacts[len(prior.Artifacts)] != expectedArtifact {
		return errNativeFixtureDenied
	}
	var in, out int64
	for _, u := range h.Usage {
		if (u.ActualInputTokens == nil) != (u.ActualOutputTokens == nil) {
			return errNativeFixtureDenied
		}
		if u.ActualInputTokens != nil {
			in += *u.ActualInputTokens
			out += *u.ActualOutputTokens
		}
	}
	if in != 301714 || out != 11982 {
		return errNativeFixtureDenied
	}
	vector := []string{prior.BaselineSHA256, pins["final-audit"], pins["high"], pins["pg-ledger"], pins["pg-product"], pins["pg-initial"], pins["runtime-state"], pins["failure-correlation"], releaseChainV7FailedCode, releaseChainV7FailedTree, releaseChainV7FailedApp}
	var actualVector []string
	for _, v := range releaseChainV6Array(bridge, "fold_vector") {
		s, ok := v.(string)
		if !ok {
			return errNativeFixtureDenied
		}
		actualVector = append(actualVector, s)
	}
	if !reflect.DeepEqual(vector, actualVector) || shadowV4Hash(vector) != releaseChainV7HistoryFold || !releaseChainV6Exact(bridge, map[string]any{"status": "INDEPENDENT_SEALED539_CARRY_BRIDGE", "fold_sha256": releaseChainV7HistoryFold, "prior_fold_sha256": prior.BaselineSHA256, "old_originals_or_measurements_changed": false, "prospective_model_admission": false, "auditor_native_calls": float64(0), "issue33_ready": false, "model_generation_32768_on_closed_v6": true}) {
		return errNativeFixtureDenied
	}
	actualPins := releaseChainV6Object(bridge, "new_failed_v6_artifacts")
	if len(actualPins) != len(pins) {
		return errNativeFixtureDenied
	}
	for k, p := range pins {
		if actualPins[k] != p {
			return errNativeFixtureDenied
		}
	}
	c, s, pg, f := all["failure-correlation"], all["high"], all["pg-ledger"], all["final-audit"]
	if !releaseChainV6Exact(c, map[string]any{"status": "INDEPENDENT_ACTUAL_V6_CHAIN_NO_GO_CORRELATED", "scope_status": "failed", "code_commit": releaseChainV7FailedCode, "source_sha256": releaseChainV7FailedTree, "native_scope_sealed": true, "retry_or_reopen_performed": false, "retry_fresh_native_admitted": false, "native_request_count": float64(2), "kernel_model_reservation_count": float64(3), "accepted_model_retry_failures": float64(0), "auditor_native_calls": float64(0), "actual_original_result": nil, "actual_notice_count": float64(0), "actual_channel_requests": float64(0), "actual_root_state": "blocked", "issue33_ready": false}) || !releaseChainV6Exact(s, map[string]any{"status": "failed", "window_id": releaseChainV6WindowID, "baseline_attempts": float64(537), "maximum_new_attempts": float64(78), "maximum_total_attempts": float64(615), "issue33_ready": false}) {
		return errNativeFixtureDenied
	}
	scope := releaseChainV6Object(s, "scope")
	if !reflect.DeepEqual(scope["window"], pg["window"]) || !releaseChainV6Exact(scope, map[string]any{"total_attempts": float64(539), "new_attempts": float64(2), "failed_provider_entries": float64(1), "unknown_full_token_cost": float64(1), "accepted_model_retry_failures": float64(1)}) {
		return errNativeFixtureDenied
	}
	// The old preliminary accepted1 label remains byte exact; independent factual
	// correlation expressly reports accepted0 because no fresh native540 existed.
	rows, sqlRows, corrRows := releaseChainV6Array(scope, "requests"), releaseChainV6Array(pg, "requests"), releaseChainV6Array(c, "requests")
	if len(rows) != 2 || len(sqlRows) != 2 || len(corrRows) != 2 {
		return errNativeFixtureDenied
	}
	for i, item := range rows {
		row, ok := item.(map[string]any)
		sql, sok := sqlRows[i].(map[string]any)
		cr, cok := corrRows[i].(map[string]any)
		if !ok || !sok || !cok {
			return errNativeFixtureDenied
		}
		canon, err := releaseChainV6CanonicalRow(sql)
		if err != nil || !reflect.DeepEqual(canon, row) || !reflect.DeepEqual(cr, sql) {
			return errNativeFixtureDenied
		}
		if !releaseChainV6Exact(row, map[string]any{"ordinal": float64(538 + i), "scenario": "normal_sent", "is_parent": true, "final_count": float64(0), "terminal_count": float64(0), "completed_count": float64(0), "cancelled_count": float64(0), "timed_out_count": float64(0), "transport_error_count": float64(0), "stop_final_count": float64(0), "max_tokens_final_count": float64(0), "finish_reason": nil, "usage_observation_uncertain": false, "version_observation_uncertain": false, "finish_observation_uncertain": false}) {
			return errNativeFixtureDenied
		}
		u := h.Usage[537+i]
		if u.RunID != row["run_id"] || u.ReservationID != "release-chain-v6-native-"+strconv.Itoa(538+i) {
			return errNativeFixtureDenied
		}
		if i == 0 {
			if u.ActualInputTokens == nil || *u.ActualInputTokens != 7116 || u.ActualOutputTokens == nil || *u.ActualOutputTokens != 155 || !releaseChainV6Exact(row, map[string]any{"kind": "start", "outcome": "completed", "actual_input_tokens": float64(7116), "actual_output_tokens": float64(155), "actual_total_tokens": float64(8161), "model_version": "gemini-3.8-flash-n", "provider_operation_stage": "response_stream", "diagnostic_code": nil}) {
				return errNativeFixtureDenied
			}
		} else {
			if u.ActualInputTokens != nil || u.ActualOutputTokens != nil || !releaseChainV6Exact(row, map[string]any{"kind": "submit", "outcome": "timed_out", "actual_input_tokens": nil, "actual_output_tokens": nil, "actual_total_tokens": nil, "model_version": nil, "provider_operation_stage": "http_round_trip", "diagnostic_code": "provider_timeout"}) {
				return errNativeFixtureDenied
			}
		}
	}
	if !releaseChainV6Exact(releaseChainV6Object(c, "carry_forward"), map[string]any{"attempts": float64(539), "known_input_tokens_subtotal": float64(301714), "known_output_tokens_subtotal": float64(11982), "known_total_tokens_subtotal": float64(580620), "unknown_total_attempts": float64(48), "known_usage_pairs": float64(511), "unknown_usage_pairs": float64(28), "old_unknown_266_cause": "UNKNOWN"}) || !reflect.DeepEqual(c["carry_forward"], bridge["carry_forward"]) || !releaseChainV6Exact(releaseChainV6Object(f, "chain_audit"), map[string]any{"status": "NO_GO", "carry_forward_attempts": float64(539), "issue33_ready": false}) {
		return errNativeFixtureDenied
	}
	bindings := releaseChainV6Object(c, "evidence_bindings")
	if !releaseChainV6Exact(bindings, map[string]any{"code_commit": releaseChainV7FailedCode, "source_sha256": releaseChainV7FailedTree, "app_sha256": releaseChainV7FailedApp, "prerequisite_sha256": releaseChainV6ReviewedPrerequisiteSHA, "quality_sha256": releaseChainV5ReviewedOriginalAuditSHA}) {
		return errNativeFixtureDenied
	}
	return nil
}

func readReleaseChainV7Prerequisite() (releaseChainV5Prerequisite, error) {
	var p releaseChainV5Prerequisite
	if !nativeAuditHash.MatchString(releaseChainV7ReviewedPrerequisiteSHA) {
		return p, errNativeFixtureDenied
	}
	raw, err := readReleaseChainV5Artifact(highHistoryPath("release-chain-v7-high-prerequisite-20261013.json"), releaseChainV7ReviewedPrerequisiteSHA)
	if err != nil || !releaseChainV7ExactSemantic(raw, releaseChainV7PrerequisiteSemanticJSON) || readReleaseChainV7Authorization() != nil {
		return p, errNativeFixtureDenied
	}
	h, err := readReleaseChainV7ActualHistory()
	if err != nil {
		return p, errNativeFixtureDenied
	}
	p, err = readReleaseChainV5Prerequisite()
	if err != nil {
		return p, errNativeFixtureDenied
	}
	p.Baseline = h.NativeAttempts
	p.ChainHistorySHA = h.BaselineSHA256
	return p, nil
}
func newReleaseChainV7Window(ctx context.Context, pool *pgxpool.Pool) (*releaseChainV5Window, error) {
	p, err := readReleaseChainV7Prerequisite()
	if err != nil {
		return nil, errNativeFixtureDenied
	}
	at, err := time.Parse(time.RFC3339, os.Getenv("BUTLER_SCENARIO_CHAIN_V7_HIGH_DEADLINE"))
	if err != nil {
		return nil, errNativeFixtureDenied
	}
	return createReleaseChainHighWindowWithPolicy(ctx, pool, releaseChainV7ReviewedPrerequisiteSHA, p.ChainHistorySHA, 539, "fixture_chain_v7_high", at, 76, true, releaseChainV7Policy(), true)
}
func TestScenarioReleaseChainActualV7HighE2E(t *testing.T) {
	testReleaseChainHighNative(t, releaseChainHighEntry{WindowID: releaseChainV7WindowID, AcceptanceEnv: "BUTLER_SCENARIO_CHAIN_V7_HIGH_ACCEPTANCE", PrivateEnv: "BUTLER_SCENARIO_CHAIN_V7_HIGH_PRIVATE", ImportEnv: "BUTLER_SCENARIO_CHAIN_V7_HIGH_IMPORT_FILE", Baseline: 539, Maximum: 76, ProviderStages: true, Policy: releaseChainV7Policy(), ReadQuality: readReleaseChainV7Prerequisite, NewWindow: newReleaseChainV7Window})
}
func TestReleaseChainV7OperationalGate(t *testing.T) {
	if os.Getenv("BUTLER_SCENARIO_CHAIN_V7_HIGH_GATE") != "1" {
		t.Skip("explicit new pinned scope required")
	}
	if _, err := readReleaseChainV7Prerequisite(); err != nil {
		t.Fatal("V7 independently reviewed admission not frozen")
	}
}
func TestReleaseChainV7ActualHistoryPreservesSealed539(t *testing.T) {
	h, err := readReleaseChainV7ActualHistory()
	if err != nil {
		t.Fatal("sealed actual539 unavailable", err)
	}
	if h.NativeAttempts != 539 || h.Usage[536].ActualInputTokens != nil || h.Usage[538].ActualInputTokens != nil {
		t.Fatal("old UNKNOWN became inferred or refunded")
	}
	if readReleaseChainV7Authorization() != nil {
		t.Fatal("human amendment not exact")
	}
	if releaseChainV7ReviewedPrerequisiteSHA == "" {
		if _, err = readReleaseChainV7Prerequisite(); err == nil {
			t.Fatal("actual history authorized an unfrozen native scope")
		}
	}
}

func TestReleaseChainV7PrerequisiteTamper(t *testing.T) {
	for _, key := range []string{"baseline_attempts", "maximum_attempt_ms", "maximum_global_ms", "allowed_provider_operation_stages", "all_completed_entries_require_measured_usage_and_selected_version", "historical_unknown_total_attempts", "prior_history_sha256", "human_policy_amendment_sha256"} {
		t.Run(key, func(t *testing.T) {
			var candidate map[string]any
			_ = json.Unmarshal([]byte(releaseChainV7PrerequisiteSemanticJSON), &candidate)
			candidate[key] = nil
			raw, _ := json.Marshal(candidate)
			if releaseChainV7ExactSemantic(raw, releaseChainV7PrerequisiteSemanticJSON) {
				t.Fatal("changed protocol accepted")
			}
		})
	}
}
