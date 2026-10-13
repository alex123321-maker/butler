package main

import (
	"path/filepath"
	"reflect"
)

const v20ClosedHistorySHA = "2a408c5986688618cf0afdb033216d83cc5d2b9bef456f6ab6a45b4449acc390"
const v20ClosedBridgeSHA = "46f2b7a5419657d9ecd32370c17db7e17864b12a5c2c9908e72c31bbe5f02d00"
const v20ClosedFinalSHA = "d5c177d99a9c07e3da84b5137c65885aec7150625742d7e13f696f76a597b4d0"
const v20ClosedCorrelationSHA = "7af16897bec9e3dd5ecfcc6db81ae45c34f7b224a2c04182b0467da92356a882"

var v20ClosedRawPins = map[string]string{
	"scope":   "1ec8adfd408ba96702284076f3c87b3f2e48d8e57096376c1f1865ca8d2b3fdb",
	"ledger":  "6d0151834cf839feb0be0790950fdf3eff9d23baab96a63964d25912ba691db5",
	"product": "8585640da11afaee3ce43c50cf032ce3bfcb4e638f9f0c1534a1820fb24a2355",
	"initial": "a70bf83fbc99b4891e8f36f16bf08413bf351931c3ec3623dcd4f4f6a0702402",
	"runtime": "9801188d6bd673c8730311f2217e43e31e3cc7e30d900d5ff5d1adb4afdf37b5",
}

func verifyV20UnavailableClassObservation(row object) {
	_, present := row["provider_unavailable_class"]
	require(present, "v20_diagnostic_class_presence")
	if row["diagnostic_code"] != "provider_unavailable" {
		require(row["provider_unavailable_class"] == nil, "v20_unavailable_class_on_other_outcome")
		return
	}
	class := text(row["provider_unavailable_class"])
	require(class == "backend_unavailable" || class == "account_access_rejected" || class == "unknown", "v20_unavailable_class_vocabulary")
	require(row["outcome"] == "transport_error" && (row["provider_operation_stage"] == "http_round_trip" || row["provider_operation_stage"] == "authentication" || row["provider_operation_stage"] == "request_serialization"), "v20_class_not_success_or_retry")
	if class != "unknown" {
		require(row["provider_operation_stage"] == "http_round_trip", "v20_exact_transport_class_phase")
	}
}
func verifyRetainedFailedV19Source(root string) {
	prior := obj(pinnedObject(root, "release-chain-v18-history705-20261013.json", v19ClosedV18HistorySHA))
	history := obj(pinnedObject(root, "release-chain-v19-history706-20261013.json", v20ClosedHistorySHA))
	bridge := obj(pinnedObject(root, "release-chain-v19-carry706-independent-20261013.json", v20ClosedBridgeSHA))
	final := obj(pinnedObject(root, "release-chain-v19-final-audit-20261013.json", v20ClosedFinalSHA))
	correlation := obj(pinnedObject(root, "release-chain-v19-failure-correlation-20261013.json", v20ClosedCorrelationSHA))
	verifyV20Closed706(prior, history, bridge, final, correlation)
	values := map[string]any{}
	for name, pin := range v20ClosedRawPins {
		raw, err := readExact(filepath.Join("/retained-v19", name+".json"), pin, 8<<20)
		require(err == nil, "v20_closed706_named_leaf")
		values[name], err = decode(raw)
		require(err == nil, "v20_closed706_named_json")
	}
	scope, ledger := obj(values["scope"]), obj(values["ledger"])
	require(scope["status"] == "failed" && integer(ledger["baseline_attempts"]) == 705 && integer(ledger["total_attempts"]) == 706 && integer(ledger["new_attempts"]) == 1, "v20_closed706_paid_scope")
	native, sql := rows(obj(scope["scope"]), "requests"), rows(ledger, "requests")
	require(len(native) == 1 && len(sql) == 1, "v20_closed706_single_paid")
	n, r := native[0], sql[0]
	// Correlate the original nullable packet with SQL before class interpretation.
	normalized := obj(clone(n))
	for from, to := range map[string]string{"input_sha256": "input_hash", "admitted_at_utc": "admitted_at", "finished_at_utc": "finished_at", "actual_input_tokens": "input_tokens", "actual_output_tokens": "output_tokens", "actual_total_tokens": "total_tokens"} {
		normalized[to] = normalized[from]
		delete(normalized, from)
	}
	for _, o := range []object{normalized, r} {
		o["admitted_at"] = moment(o["admitted_at"]).UTC().Format("2006-01-02T15:04:05.999999999Z07:00")
		o["finished_at"] = moment(o["finished_at"]).UTC().Format("2006-01-02T15:04:05.999999999Z07:00")
	}
	require(reflect.DeepEqual(normalized, r), "v20_closed706_native_pg_equal")
	require(integer(n["ordinal"]) == 706 && n["scenario"] == "lost_ack" && n["kind"] == "start" && n["outcome"] == "transport_error" && n["diagnostic_code"] == "provider_unavailable" && n["provider_operation_stage"] == "http_round_trip" && n["actual_input_tokens"] == nil && n["actual_output_tokens"] == nil && n["actual_total_tokens"] == nil && n["model_version"] == nil && integer(n["elapsed_ms"]) == 3473, "v20_closed706_unknown_not_refilled")
	require(n["provider_unavailable_class"] == nil, "v20_old706_subcause_not_recovered")
	_ = witnesses(values["initial"])
	_ = witnesses(values["product"])
	runtime := obj(values["runtime"])
	require(runtime["exit_code"] != jsonNumber(0) && runtime["oom_killed"] == false, "v20_closed706_runtime_no_go")
}
func verifyV20Closed706(prior, history, bridge, final, correlation object) {
	verifyV18ClosedHistoryObjects(prior, history, bridge, 19, 705, 706, v20PriorSHA, 2649327, 2098118, 42953, 53, 33)
	require(bridge["provider_http_status"] == nil && bridge["provider_unavailable_subcause"] == "UNKNOWN" && bridge["safe_nominal_timeout_or_retry"] == false && bridge["all_new_expense_measured"] == false && integer(bridge["remaining_original_absolute825_attempts"]) == 119, "v20_closed706_unknown_no_recovery")
	cost := obj(bridge["actual_native_cost"])
	require(integer(cost["attempts"]) == 1 && cost["measured_total_tokens"] == nil && cost["known_subtotals_are_not_request_cost"] == true && integer(cost["unknown_total_attempts"]) == 1, "v20_closed706_unknown_not_zero_cost")
	row := obj(arr(history["usage"])[705])
	require(row["actual_input_tokens"] == nil && row["actual_output_tokens"] == nil && row["reservation_id"] == "release-chain-v19-native-706", "v20_closed706_null_pair")
	report := obj(final["chain_audit"])
	require(report["status"] == "NO_GO" && integer(report["accepted_model_retry_failures"]) == 0 && integer(report["carry_forward_attempts"]) == 706 && correlation["native_scope_sealed"] == true, "v20_closed706_no_pass_or_retry")
}
