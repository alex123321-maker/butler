package main

import (
	"fmt"
	"os"
	"path/filepath"
	"reflect"
)

const v17HistorySHA = "e464a77ce73b3a6e477e2441c87b32a17730917fe1bd5f4502555cda69f39185"
const v17BridgeSHA = "64ab4b86114a6a6782f6f6136ba6f9561eae41a5f6a771bba80ce09c63159d96"
const v16ClosedHistorySHA = "7599c090a8c4402f42b8f411e84d0c70a19c692c3334e8ec77fb50572fed6fb2"
const v16ClosedBridgeSHA = "24fb6c34e4a68da1291ab3868b18db289251b24e0c194ab62cb001a61e01b977"
const v17PriorSHA = "314bf4a5a3b649f6dc6354a03ea4c536a85f6a8ea92c756ef50bcc288aeda951"

// Empty V17 scope bytes remain empty: no fabricated object, actual counter or
// zero-cost claim can be derived from this identity check.
func verifyV18RetainedEmptyScope(historicalRoot string) {
	path := filepath.Join(historicalRoot, "release-chain-v17-high-20261013.json")
	stat, err := os.Lstat(path)
	require(err == nil && stat.Mode().IsRegular() && stat.Size() == 0, "v18_old_empty_scope_identity")
	f, err := os.Open(path)
	require(err == nil, "v18_old_empty_scope_read")
	defer f.Close()
	opened, err := f.Stat()
	require(err == nil && os.SameFile(stat, opened) && opened.Size() == 0, "v18_old_empty_scope_replaced")
}

func verifyV18RetainedHistory(root, historicalRoot string) {
	verifyRetainedFailedV15Source(root)
	verifyV18ClosedAppend(root, historicalRoot, 16, 616, 636, v16HistorySHA, v16ClosedHistorySHA, v16ClosedBridgeSHA, v17PriorSHA, 1844192, 1372607, 30515, 50, 30)
	verifyV18ClosedAppend(root, historicalRoot, 17, 636, 658, v16ClosedHistorySHA, v17HistorySHA, v17BridgeSHA, v18PriorSHA, 2099516, 1601910, 34311, 51, 31)
	verifyV18OwnerStopComponent(root)
}
func verifyV18ClosedAppend(root, historicalRoot string, version, priorCount, count int, priorPin, historyPin, bridgePin, fold string, total, input, output int64, unknownTotal, unknownPairs int) {
	priorVersion := version - 1
	prior := obj(pinnedObject(root, fmt.Sprintf("release-chain-v%d-history%d-20261013.json", priorVersion, priorCount), priorPin))
	current := obj(pinnedObject(root, fmt.Sprintf("release-chain-v%d-history%d-20261013.json", version, count), historyPin))
	bridge := obj(pinnedObject(root, fmt.Sprintf("release-chain-v%d-carry%d-independent-20261013.json", version, count), bridgePin))
	verifyV18ClosedHistoryObjects(prior, current, bridge, version, priorCount, count, fold, total, input, output, unknownTotal, unknownPairs)
	pins := obj(bridge[fmt.Sprintf("new_failed_v%d_artifacts", version)])
	for name, pin := range pins {
		if version == 17 && name == "high" {
			require(pin == digest(nil), "v18_empty_scope_original_hash")
			verifyV18RetainedEmptyScope(historicalRoot)
			continue
		}
		if name == "final-audit" || name == "failure-correlation" {
			pinnedObject(root, fmt.Sprintf("release-chain-v%d-%s-20261013.json", version, name), sha(pin))
		} else {
			v18NamedHistoricalArtifact(historicalRoot, fmt.Sprintf("release-chain-v%d-%s-20261013.json", version, name), sha(pin))
		}
	}
	ledger := obj(v18NamedHistoricalArtifact(historicalRoot, fmt.Sprintf("release-chain-v%d-pg-ledger-20261013.json", version), sha(pins["pg-ledger"])))
	require(integer(ledger["baseline_attempts"]) == int64(priorCount) && integer(ledger["total_attempts"]) == int64(count) && integer(ledger["new_attempts"]) == int64(count-priorCount), "v18_historical_paid_counter")
	requests := rows(ledger, "requests")
	require(len(requests) == count-priorCount, "v18_historical_paid_inventory")
	usage := arr(current["usage"])
	var measuredInput, measuredOutput, measuredTotal int64
	missing := 0
	for i, r := range requests {
		u := obj(usage[priorCount+i])
		require(integer(r["ordinal"]) == int64(priorCount+i+1) && u["run_id"] == r["run_id"] && u["reservation_id"] == fmt.Sprintf("release-chain-v%d-native-%d", version, priorCount+i+1) && u["actual_input_tokens"] == r["input_tokens"] && u["actual_output_tokens"] == r["output_tokens"], "v18_historical_usage_exact_pg")
		if r["input_tokens"] == nil {
			require(version == 17 && i == len(requests)-1 && r["ordinal"] == jsonNumber(658) && r["outcome"] == "cancelled" && r["diagnostic_code"] == "transport_connection_error" && r["provider_operation_stage"] == "response_stream" && r["output_tokens"] == nil && r["total_tokens"] == nil && r["model_version"] == nil, "v18_658_cancel_unknown_not_retry")
			missing++
		} else {
			require(r["outcome"] == "completed" && r["provider_operation_stage"] == "response_stream" && r["model_version"] == "gemini-3.8-flash-n" && r["output_tokens"] != nil && r["total_tokens"] != nil, "v18_historical_known_native")
			measuredInput += integer(r["input_tokens"])
			measuredOutput += integer(r["output_tokens"])
			measuredTotal += integer(r["total_tokens"])
		}
	}
	cost := obj(bridge["actual_native_cost"])
	require(integer(cost["attempts"]) == int64(len(requests)) && integer(cost["known_input_tokens"]) == measuredInput && integer(cost["known_output_tokens"]) == measuredOutput && integer(cost["known_total_tokens"]) == measuredTotal && integer(cost["unknown_total_attempts"]) == int64(missing), "v18_historical_all_paid_cost")
	report := obj(obj(pinnedObject(root, fmt.Sprintf("release-chain-v%d-final-audit-20261013.json", version), sha(pins["final-audit"])))["chain_audit"])
	require(report["status"] == "NO_GO" && integer(report["carry_forward_attempts"]) == int64(count) && integer(report["accepted_model_retry_failures"]) == 0, "v18_closed_windows_stay_no_go")
}
func verifyV18ClosedHistoryObjects(prior, current, bridge object, version, priorCount, count int, fold string, total, input, output int64, unknownTotal, unknownPairs int) {
	old, now := arr(prior["usage"]), arr(current["usage"])
	require(len(old) == priorCount && len(now) == count && integer(current["native_attempts"]) == int64(count) && current["baseline_sha256"] == fold && current["status"] == prior["status"] && reflect.DeepEqual(now[:priorCount], old), "v18_history_prefix_immutable")
	a, b := arr(prior["artifacts"]), arr(current["artifacts"])
	require(len(b) == len(a)+1 && reflect.DeepEqual(b[:len(a)], a), "v18_history_artifacts_immutable")
	pins := obj(bridge[fmt.Sprintf("new_failed_v%d_artifacts", version)])
	last := obj(b[len(a)])
	require(last["report_sha256"] == pins["final-audit"] && integer(last["new_attempts"]) == int64(count-priorCount), "v18_history_append_artifact")
	vector := arr(bridge["fold_vector"])
	require(len(vector) == 11 && hashValue(vector) == fold && bridge["fold_sha256"] == fold && vector[0] == prior["baseline_sha256"] && vector[1] == pins["final-audit"] && vector[2] == pins["high"] && vector[3] == pins["pg-ledger"] && vector[4] == pins["pg-product"] && vector[5] == pins["pg-initial"] && vector[6] == pins["runtime-state"] && vector[7] == pins["failure-correlation"] && vector[8] == last["release_ref"] && bridge["prospective_model_admission"] == false && bridge["issue33_ready"] == false && bridge["all_prior_scopes_unchanged_and_sealed"] == true, "v18_history_factual_bridge")
	carry := obj(bridge["carry_forward"])
	require(integer(carry["attempts"]) == int64(count) && integer(carry["known_total_tokens_subtotal"]) == total && integer(carry["unknown_total_attempts"]) == int64(unknownTotal) && carry["old_unknown_266_cause"] == "UNKNOWN", "v18_historical_total_unknown")
	var observedInput, observedOutput int64
	missing := 0
	for _, value := range now {
		u := obj(value)
		if u["actual_input_tokens"] == nil {
			require(u["actual_output_tokens"] == nil, "v18_history_pair")
			missing++
		} else {
			require(u["actual_output_tokens"] != nil, "v18_history_pair")
			observedInput += integer(u["actual_input_tokens"])
			observedOutput += integer(u["actual_output_tokens"])
		}
	}
	require(observedInput == input && observedOutput == output && missing == unknownPairs && integer(current["known_usage_pairs"]) == int64(count-unknownPairs) && integer(current["unknown_usage_pairs"]) == int64(unknownPairs), "v18_historical_pairs_recomputed")
}

func v18NamedHistoricalArtifact(root, leaf, pin string) any {
	raw, err := readPinnedPublic(filepath.Join(root, leaf), pin)
	require(err == nil, "v18_named_historical_safe_bytes")
	value, err := decode(raw)
	require(err == nil, "v18_named_historical_safe_json")
	return value
}
