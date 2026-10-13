package main

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"reflect"
	"regexp"
	"sort"
	"strings"
	"time"
	"unicode/utf8"
)

const qualitySHA = "f8248d19ec2a0c222b39d46fbc46c0d0cf4ccc3c1361c89965b597eedbb21cab"
const prerequisiteSHA = "9ce27e6d9a15f598fadc037e5de377869672937fef84849fc662610ec6379de3"
const priorSHA = "7100f33980905baea6dbcffe608d546a8544457b8260bac4d725952f415578df"

var scenarios = []string{"normal_sent", "restart_sent", "explicit_absence", "lost_ack", "owner_stop"}
var hashPattern = regexp.MustCompile(`^[a-f0-9]{64}$`)

type object = map[string]any
type inputs struct{ Scope, Ledger, Initial, Product, Runtime, Quality, Prerequisite, ExpenseException any }
type costs struct {
	Attempts      int   `json:"attempts"`
	Input         int64 `json:"known_input_tokens"`
	Output        int64 `json:"known_output_tokens"`
	Total         int64 `json:"known_total_tokens"`
	Unknown       int   `json:"unknown_total_attempts"`
	UnknownInput  int   `json:"unknown_input_attempts"`
	UnknownOutput int   `json:"unknown_output_attempts"`
}
type report struct {
	Status                 string   `json:"status"`
	Reasons                []string `json:"no_go_reasons"`
	Cost                   costs    `json:"all_new_measured_cost"`
	Cases                  []string `json:"verified_scenarios"`
	Prior                  int      `json:"prior_attempts"`
	Total                  int      `json:"carry_forward_attempts"`
	KnownHistoricalTotal   int64    `json:"prior_known_total_subtotal"`
	UnknownHistoricalTotal int      `json:"prior_unknown_total_attempts"`
	AuditorCalls           int      `json:"auditor_native_calls"`
	AcceptedRetryFailures  int      `json:"accepted_model_retry_failures"`
	Ready                  bool     `json:"issue33_ready"`
	Limits                 []string `json:"limits"`
}

func require(ok bool, code string) {
	if !ok {
		panic(code)
	}
}
func obj(v any) object  { o, ok := v.(map[string]any); require(ok, "object_contract"); return o }
func arr(v any) []any   { a, ok := v.([]any); require(ok, "array_contract"); return a }
func text(v any) string { s, ok := v.(string); require(ok, "string_contract"); return s }
func integer(v any) int64 {
	n, ok := v.(json.Number)
	require(ok, "integer_contract")
	x, e := n.Int64()
	require(e == nil && x >= 0 && x <= 1<<48, "integer_contract")
	return x
}
func moment(v any) time.Time {
	t, e := time.Parse(time.RFC3339Nano, text(v))
	require(e == nil && !t.IsZero(), "time_contract")
	return t
}
func keys(o object, inventory string) {
	fields := strings.Fields(inventory)
	require(len(o) == len(fields), "field_inventory")
	for _, f := range fields {
		_, ok := o[f]
		require(ok, "field_inventory")
	}
}
func digest(raw []byte) string { h := sha256.Sum256(raw); return hex.EncodeToString(h[:]) }
func canonical(v any) []byte {
	b, e := json.Marshal(v)
	require(e == nil, "canonical_encoding")
	return b
}
func hashValue(v any) string { return digest(canonical(v)) }
func sha(v any) string       { s := text(v); require(hashPattern.MatchString(s), "hash_contract"); return s }
func rows(p object, name string) []object {
	a := arr(p[name])
	out := make([]object, len(a))
	for i, v := range a {
		out[i] = obj(v)
	}
	return out
}
func byID(a []object, key string) map[string]object {
	m := map[string]object{}
	for _, r := range a {
		s := text(r[key])
		require(s != "" && m[s] == nil, "duplicate_identity")
		m[s] = r
	}
	return m
}
func decode(raw []byte) (result any, err error) {
	defer func() {
		if recover() != nil {
			result = nil
			err = errors.New("strict_safe_json_denied")
		}
	}()
	require(len(raw) > 0 && len(raw) <= 8<<20, "file_bound")
	require(utf8.Valid(raw) && json.Valid(raw), "json_encoding")
	d := json.NewDecoder(bytes.NewReader(raw))
	d.UseNumber()
	var visit func(int) any
	visit = func(depth int) any {
		require(depth <= 40, "json_depth")
		token, e := d.Token()
		require(e == nil, "json_token")
		delim, compound := token.(json.Delim)
		if !compound {
			if s, ok := token.(string); ok {
				require(len(s) <= 65536, "json_string_bound")
			}
			return token
		}
		if delim == '{' {
			m := object{}
			for d.More() {
				k, e := d.Token()
				require(e == nil, "json_key")
				name, ok := k.(string)
				require(ok, "json_key")
				_, seen := m[name]
				require(!seen, "duplicate_json_key")
				m[name] = visit(depth + 1)
			}
			end, e := d.Token()
			require(e == nil && end == json.Delim('}'), "json_end")
			return m
		}
		require(delim == '[', "json_delim")
		a := []any{}
		for d.More() {
			require(len(a) < 8192, "json_array_bound")
			a = append(a, visit(depth+1))
		}
		end, e := d.Token()
		require(e == nil && end == json.Delim(']'), "json_end")
		return a
	}
	result = visit(0)
	_, e := d.Token()
	require(e == io.EOF, "json_trailing")
	return result, nil
}

func audit(in inputs) (r report, err error) {
	r = report{Status: "NO_GO", Reasons: []string{}, Cases: []string{}, Prior: 537, KnownHistoricalTotal: 572459, UnknownHistoricalTotal: 47, Limits: []string{
		"Read-only witness correlation on a shared administrator-controlled host; no hardware isolation or blind benchmark claim.",
		"Owner-bound setup and cheap checks are model-free. Native ordinary Core begins at the promoted input; production finite-IR builder authors the adapter.",
		"Project and Telegram recipients are loopback fixtures. The original result is prepared_verified and unapplied; no human recipient or live project deployment.",
		"Kernel budget reservations are conservative quotes; only the separate native ledger owns measured complete token totals, including thinking.",
		"Cloud weights, server effort/hardware/RAM and subscription invoice remain unavailable. Old unknown spend and UNKNOWN266 are preserved.",
		"The exact human expense exception covers only the sealed failed537. Its NULL usage and V5 NO_GO remain; any unknown new-after537 expense denies PASS.",
		"This chain witness result does not replace final-composition tests or issue-wide independent acceptance and does not authorize merge or execution.",
	}}
	defer func() {
		if v := recover(); v != nil {
			r.Status = "NO_GO"
			code, ok := v.(string)
			if !ok {
				code = "typed_contract"
			}
			r.Reasons = append(r.Reasons, code)
			err = errors.New(code)
		}
	}()
	sc, pg := obj(in.Scope), obj(in.Ledger)
	keys(sc, "status window_id baseline_attempts maximum_total_attempts maximum_new_attempts scenarios scope planning human_messages issue33_ready")
	keys(pg, "baseline_attempts total_attempts new_attempts requests window")
	ss := obj(sc["scope"])
	keys(ss, "total_attempts new_attempts unknown_full_token_cost failed_provider_entries accepted_model_retry_failures overlong_provider_entries requests window")
	nr, pr := arr(ss["requests"]), arr(pg["requests"])
	r.Total = 537 + len(pr)
	// Owning PG records every admitted entry, even if a later identity, timing,
	// outcome or cap check fails. Quotes and collector success flags never
	// replace measured totals; a nullable dimension remains unknown.
	for _, v := range pr {
		a := obj(v)
		r.Cost.Attempts++
		if a["input_tokens"] != nil {
			r.Cost.Input += integer(a["input_tokens"])
		} else {
			r.Cost.UnknownInput++
		}
		if a["output_tokens"] != nil {
			r.Cost.Output += integer(a["output_tokens"])
		} else {
			r.Cost.UnknownOutput++
		}
		if a["total_tokens"] == nil {
			r.Cost.Unknown++
		} else {
			r.Cost.Total += integer(a["total_tokens"])
		}
	}
	q := obj(in.Quality)
	p := obj(in.Prerequisite)
	verifyExpenseException(in.ExpenseException)
	require(q["status"] == "V5_PRIVATE_FINAL_SQL_CORRELATION_MATCH" && integer(q["exact_diagnostic_rows"]) == 252 && integer(q["exact_private_originals"]) == 252 && integer(q["auditor_native_calls"]) == 0 && q["issue33_ready"] == false, "quality_original_provenance")
	wa := obj(q["window_audit"])
	require(wa["status"] == "CLOUD_V5_WINDOW_AUDIT_PASS" && integer(wa["carry_forward_attempts"]) == 535 && len(arr(wa["no_go_reasons"])) == 0, "quality_prior_denied")
	verifyV6Prerequisite(p)
	require(sc["window_id"] == "issue33-high-release-chain-v6-20261013-baseline537" && integer(sc["baseline_attempts"]) == 537 && integer(pg["baseline_attempts"]) == 537 && integer(sc["maximum_total_attempts"]) == 615 && integer(sc["maximum_new_attempts"]) == 78 && sc["planning"] == "actual_native_High" && sc["human_messages"] == json.Number("0") && sc["issue33_ready"] == false, "scope_identity")
	window, pw := obj(ss["window"]), obj(pg["window"])
	keys(window, "native_baseline attempts prior_history_hash prerequisite_hash expires_at maximum_new_attempts provider_stage_observation kernel_operation_binding")
	keys(pw, "native_baseline attempts prior_history_hash prerequisite_hash expires_at maximum_new_attempts provider_stage_observation kernel_operation_binding")
	wcopy, pcopy := obj(clone(window)), obj(clone(pw))
	wcopy["expires_at"] = moment(window["expires_at"]).UTC().Format(time.RFC3339Nano)
	pcopy["expires_at"] = moment(pw["expires_at"]).UTC().Format(time.RFC3339Nano)
	require(reflect.DeepEqual(wcopy, pcopy) && integer(window["native_baseline"]) == 537 && window["prior_history_hash"] == priorSHA && window["prerequisite_hash"] == prerequisiteSHA && integer(window["maximum_new_attempts"]) == 78 && window["provider_stage_observation"] == true && window["kernel_operation_binding"] == true, "durable_window_binding")
	runtime := obj(in.Runtime)
	keys(runtime, "started_at finished_at exit_code oom_killed memory_limit_bytes image_id postgres_image_id converter_image_id")
	require(runtime["image_id"] == "sha256:5cf287a799e6b94384bad13d16b14904c531f51ba65792237e122ce42b392f61", "actual_native_image_identity")
	begin, end := moment(runtime["started_at"]), moment(runtime["finished_at"])
	expiry := moment(window["expires_at"])
	require(runtime["exit_code"] == json.Number("0") && runtime["oom_killed"] == false && integer(runtime["memory_limit_bytes"]) == 1536<<20 && !end.Before(begin) && expiry.After(begin) && !expiry.After(begin.Add(time.Hour)), "runtime_or_global_deadline")
	require(len(nr) > 0 && len(nr) <= 78 && len(nr) == len(pr), "entry_inventory")
	require(integer(ss["new_attempts"]) == int64(len(nr)) && integer(pg["new_attempts"]) == int64(len(nr)) && integer(ss["total_attempts"]) == int64(r.Total) && integer(pg["total_attempts"]) == int64(r.Total) && integer(window["attempts"]) == int64(r.Total), "spent_counter")
	require(sc["status"] == "passed" && integer(ss["unknown_full_token_cost"])+integer(ss["overlong_provider_entries"]) == 0, "collector_outcome")
	results := obj(sc["scenarios"])
	require(len(results) == 5, "scenario_inventory")
	grouped := map[string][]object{}
	lastScenario := -1
	var previous time.Time
	operations := map[string]bool{}
	failedEntries := 0
	for i, v := range nr {
		a, b := obj(v), obj(pr[i])
		verifyRequest(a, b)
		op := text(a["kernel_operation_id"])
		require(op != "" && !operations[op], "native_operation_duplicate")
		operations[op] = true
		if a["outcome"] != "completed" {
			failedEntries++
		}
		require(integer(a["ordinal"]) == int64(538+i), "ordinal_replay_or_gap")
		name := text(a["scenario"])
		pos := -1
		for j, s := range scenarios {
			if s == name {
				pos = j
			}
		}
		require(pos >= lastScenario && pos >= 0, "scenario_release_order")
		lastScenario = pos
		at, finish := moment(a["admitted_at_utc"]), moment(a["finished_at_utc"])
		require(!at.Before(begin) && !finish.After(end) && at.Before(expiry) && !finish.After(expiry) && !at.Before(previous) && !finish.Before(at) && finish.Sub(at) <= 120*time.Second, "request_deadline")
		elapsed := integer(a["elapsed_ms"])
		require(elapsed <= 120000 && finish.Sub(at).Milliseconds()-elapsed >= -2 && finish.Sub(at).Milliseconds()-elapsed <= 2, "request_elapsed")
		previous = at
		grouped[name] = append(grouped[name], a)
	}
	initial, product := witnesses(in.Initial), witnesses(in.Product)
	require(len(initial) == 5 && len(product) == 5, "owning_product_inventory")
	var priorFinal time.Time
	for _, name := range scenarios {
		require(results[name] == true && len(grouped[name]) > 0, "scenario_missing")
		first, last := grouped[name][0], grouped[name][len(grouped[name])-1]
		ini, fin := initial[name], product[name]
		require(ini != nil && fin != nil, "scenario_witness_missing")
		require(!moment(ini["created_at"]).Before(begin) && moment(ini["created_at"]).Before(moment(first["admitted_at_utc"])) && !moment(ini["created_at"]).Before(priorFinal) && !moment(fin["created_at"]).Before(moment(last["finished_at_utc"])) && !moment(fin["created_at"]).After(end), "initial_final_causality")
		verifyProduct(name, obj(ini["payload_json"]), obj(fin["payload_json"]), grouped[name])
		r.AcceptedRetryFailures += verifyModelRetries(name, obj(obj(fin["payload_json"])["postgres"]), grouped[name])
		require(moment(fin["created_at"]).Sub(moment(ini["created_at"])) <= 20*time.Minute, "case_lifetime")
		priorFinal = moment(fin["created_at"])
		r.Cases = append(r.Cases, name)
	}
	require(integer(ss["failed_provider_entries"]) == int64(failedEntries) && integer(ss["accepted_model_retry_failures"]) == int64(r.AcceptedRetryFailures) && r.AcceptedRetryFailures == failedEntries, "retry_failure_accounting")
	require(r.Cost.Unknown == 0, "complete_new_cost_required")
	r.Status = "CHAIN_V6_WITNESS_AUDIT_PASS"
	return r, nil
}
func clone(v any) any { x, e := decode(canonical(v)); require(e == nil, "clone_contract"); return x }
func witnesses(v any) map[string]object {
	out := map[string]object{}
	for _, value := range arr(v) {
		w := obj(value)
		keys(w, "scenario payload_sha256 payload_json created_at")
		name := text(w["scenario"])
		require(out[name] == nil && sha(w["payload_sha256"]) == hashValue(w["payload_json"]), "canonical_witness_hash")
		out[name] = w
	}
	return out
}
func verifyRequest(a, b object) {
	keys(a, "ordinal scenario kind root_task_id run_id is_parent input_sha256 admitted_at_utc finished_at_utc elapsed_ms outcome failure_stage diagnostic_code actual_input_tokens actual_output_tokens actual_total_tokens model_version final_count terminal_count completed_count cancelled_count timed_out_count transport_error_count finish_reason usage_observation_uncertain version_observation_uncertain stop_final_count max_tokens_final_count finish_observation_uncertain provider_operation_stage kernel_operation_id")
	normalized := obj(clone(a))
	for from, to := range map[string]string{"input_sha256": "input_hash", "admitted_at_utc": "admitted_at", "finished_at_utc": "finished_at", "actual_input_tokens": "input_tokens", "actual_output_tokens": "output_tokens", "actual_total_tokens": "total_tokens"} {
		normalized[to] = normalized[from]
		delete(normalized, from)
	}
	for _, field := range []string{"admitted_at", "finished_at"} {
		normalized[field] = moment(normalized[field]).UTC().Format(time.RFC3339Nano)
	}
	sql := obj(clone(b))
	for _, field := range []string{"admitted_at", "finished_at"} {
		sql[field] = moment(sql[field]).UTC().Format(time.RFC3339Nano)
	}
	require(reflect.DeepEqual(normalized, sql), "independent_pg_entry_mismatch")
	require(a["model_version"] == "gemini-3.8-flash-n" && a["usage_observation_uncertain"] == false && a["version_observation_uncertain"] == false && a["finish_observation_uncertain"] == false, "native_terminal_contract")
	require(a["provider_operation_stage"] == "response_stream", "actual_provider_operation_stage")
	input, output, total := integer(a["actual_input_tokens"]), integer(a["actual_output_tokens"]), integer(a["actual_total_tokens"])
	require(input <= 1048576 && output <= 32768 && total >= input+output && total <= input+32768, "native_generation_allowance")
	require(a["kind"] == "start" || a["kind"] == "continue" || a["kind"] == "submit", "entry_kind")
	sha(a["input_sha256"])
	require(text(a["run_id"]) != "" && text(a["root_task_id"]) != "", "entry_owner")
	fc, tc, cc := integer(a["final_count"]), integer(a["terminal_count"]), integer(a["completed_count"])
	if a["outcome"] != "completed" {
		require((a["outcome"] == "timed_out" || a["outcome"] == "transport_error") && a["diagnostic_code"] == "provider_timeout" && (a["failure_stage"] == "entry" || a["failure_stage"] == "stream"), "only_nominal_timeout_retriable")
		require(fc == 0 && cc == 0 && integer(a["cancelled_count"]) == 0 && tc <= 1 && tc == integer(a["timed_out_count"])+integer(a["transport_error_count"]) && integer(a["stop_final_count"])+integer(a["max_tokens_final_count"]) == 0 && a["finish_reason"] == nil, "failed_retry_original_lifecycle")
		return
	}
	require(a["failure_stage"] == nil && a["diagnostic_code"] == nil, "native_success_diagnostic")
	require(fc <= 1 && tc == cc && integer(a["cancelled_count"])+integer(a["timed_out_count"])+integer(a["transport_error_count"]) == 0 && integer(a["max_tokens_final_count"]) == 0, "clean_original_lifecycle")
	if fc == 1 {
		require(tc == 1 && a["finish_reason"] == "STOP" && integer(a["stop_final_count"]) == 1, "final_stop_contract")
	} else {
		require(tc == 0 && a["finish_reason"] == nil && integer(a["stop_final_count"]) == 0, "tool_request_contract")
	}
}
func sortedByTime(rows []object, key string) {
	sort.SliceStable(rows, func(i, j int) bool { return moment(rows[i][key]).Before(moment(rows[j][key])) })
}
