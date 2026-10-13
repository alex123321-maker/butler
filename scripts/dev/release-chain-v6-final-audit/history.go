package main

import (
	"errors"
	"io"
	"os"
	"path/filepath"
	"reflect"
)

func retainedFailedV5Pins() map[string]string {
	return map[string]string{
		"final-audit":         "592d27ee34178a9b336eb0f9cce4784fac1ba214b22f05e1ad0b247a48f97c04",
		"high":                "e1101e9f1b8b435e9f420aeea7d1507326cd0f0ee3bc80311c417792c899f709",
		"pg-ledger":           "778b8cd9e204ba074b98ac677d3867522728d25e3f7566473adff6f312160c05",
		"pg-product":          "82a3608fa9e86014140da6ad2e5bb769cfc1747cdab2c98027ccfdfb64557181",
		"pg-initial":          "31a5e87d90569a8a950e76206b1825c818e9ebddec9be2c54e71111da05f78a0",
		"runtime-state":       "4fb2e8ba23eaaf64be7b9967d265fddf2e0ccea07363bbfecfc38d5163716aca",
		"failure-correlation": "fb8a430c327c4abec5984cad6fec6e79aa57d36866500dd588979722f282a424",
	}
}

// These exact old content-free artifacts are public repository inputs, already
// included in the independently matched source TreeHash. No private directory
// or credential file is read. Mode0600 is required only for staged current leaves.
func readPinnedPublic(path, pin string) ([]byte, error) {
	stat, e := os.Lstat(path)
	if e != nil || !stat.Mode().IsRegular() || stat.Size() <= 0 || stat.Size() > 8<<20 || !hashPattern.MatchString(pin) {
		return nil, errors.New("retained_public_identity_denied")
	}
	f, e := os.Open(path)
	if e != nil {
		return nil, errors.New("retained_public_read_denied")
	}
	defer f.Close()
	opened, e := f.Stat()
	if e != nil || !os.SameFile(stat, opened) {
		return nil, errors.New("retained_public_replaced")
	}
	raw, e := io.ReadAll(io.LimitReader(f, (8<<20)+1))
	if e != nil || int64(len(raw)) != stat.Size() || digest(raw) != pin {
		return nil, errors.New("retained_public_bytes_denied")
	}
	return raw, nil
}
func verifyRetainedFailedV5Source(root string) {
	objects := map[string]any{}
	for kind, pin := range retainedFailedV5Pins() {
		raw, e := readPinnedPublic(filepath.Join(root, "deploy", "acceptance", "release-chain-v5-"+kind+"-20261013.json"), pin)
		require(e == nil, "retained_sealed_failure_bytes")
		v, e := decode(raw)
		require(e == nil, "retained_sealed_failure_json")
		objects[kind] = v
	}
	verifyRetainedFailedV5(objects)
}
func verifyRetainedFailedV5(objects map[string]any) {
	pins := retainedFailedV5Pins()
	require(priorSHA == hashValue([]string{"2539239ea8905a677cb14b0256a9744c3354e365754cae127bc517c54c35d5af", pins["final-audit"], pins["high"], pins["pg-ledger"], pins["pg-product"], pins["pg-initial"], pins["runtime-state"], pins["failure-correlation"], "afa50b225b1341d2ff8a553188a69f6b6b2ac633", "862fafb7a38a372a288a363fd73260b1efa851837080bdafecf90545d75cf26b", "383729b5493e2038f95fa23924feac880552531537d06f072c3eba8f77768b00"}), "actual_failed_history_fold")
	audit := obj(obj(objects["final-audit"])["chain_audit"])
	require(audit["status"] == "NO_GO" && audit["issue33_ready"] == false && integer(audit["carry_forward_attempts"]) == 537 && integer(audit["prior_attempts"]) == 535 && integer(audit["prior_unknown_total_attempts"]) == 46, "old_no_go_never_promoted")
	cost := obj(audit["all_new_measured_cost"])
	for field, value := range map[string]int64{"attempts": 2, "known_input_tokens": 7106, "known_output_tokens": 155, "known_total_tokens": 8462, "unknown_total_attempts": 1, "unknown_input_attempts": 1, "unknown_output_attempts": 1} {
		require(integer(cost[field]) == value, "sealed_failed_usage_never_rewritten")
	}
	scope, pg := obj(objects["high"]), obj(objects["pg-ledger"])
	require(scope["status"] == "failed" && scope["issue33_ready"] == false && integer(pg["total_attempts"]) == 537 && integer(pg["new_attempts"]) == 2 && reflect.DeepEqual(obj(scope["scenarios"]), object{"normal_sent": false}), "old_failed_scenarios_preserved")
	requests := arr(pg["requests"])
	require(len(requests) == 2, "old_two_admissions_preserved")
	a, b := obj(requests[0]), obj(requests[1])
	require(integer(a["ordinal"]) == 536 && a["outcome"] == "completed" && integer(a["input_tokens"]) == 7106 && integer(a["output_tokens"]) == 155 && integer(a["total_tokens"]) == 8462 && a["model_version"] == "gemini-3.8-flash-n", "old_known536_preserved")
	require(integer(b["ordinal"]) == 537 && b["outcome"] == "timed_out" && b["input_tokens"] == nil && b["output_tokens"] == nil && b["total_tokens"] == nil && b["model_version"] == nil && b["failure_stage"] == "stream" && b["diagnostic_code"] == "provider_timeout" && integer(b["elapsed_ms"]) == 119977, "old_unknown537_preserved")
	require(a["run_id"] == b["run_id"], "old_multi_entry_run_identity")
	for _, kind := range []string{"pg-product", "pg-initial"} {
		witness := arr(objects[kind])
		require(len(witness) == 1, "old_one_case_witness")
		w := obj(witness[0])
		require(w["scenario"] == "normal_sent" && w["payload_sha256"] == hashValue(w["payload_json"]), "old_witness_semantic_hash")
	}
	product := obj(obj(arr(objects["pg-product"])[0])["payload_json"])
	require(product["test_failed"] == true && product["original_result"] == nil && integer(product["all_channel_requests"]) == 0, "old_no_original_result_or_notice")
	state := obj(objects["runtime-state"])
	require(integer(state["exit_code"]) == 1 && state["oom_killed"] == false, "old_actual_failed_exit")
	corr := obj(objects["failure-correlation"])
	require(corr["status"] == "INDEPENDENT_ACTUAL_V5_CHAIN_NO_GO_CORRELATED" && corr["issue33_ready"] == false && corr["actual_original_result"] == nil, "old_independent_rejection_preserved")
	forward := obj(corr["carry_forward"])
	require(integer(forward["attempts"]) == 537 && integer(forward["known_total_tokens_subtotal"]) == 572459 && integer(forward["unknown_total_attempts"]) == 47 && forward["old_unknown_266_cause"] == "UNKNOWN", "old_unknown_totals_not_laundered")
}
