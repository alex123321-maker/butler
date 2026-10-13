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
const prerequisiteSHA = "bef93bde0afa1251c67eb60f01dbbe6f8142b4f32b5a8076c0f34d5d9b402077"
const priorSHA = "7100f33980905baea6dbcffe608d546a8544457b8260bac4d725952f415578df"

var scenarios = []string{"normal_sent", "restart_sent", "explicit_absence", "lost_ack", "owner_stop"}
var hashPattern = regexp.MustCompile(`^[a-f0-9]{64}$`)

type object = map[string]any
type inputs struct{ Scope, Ledger, Initial, Product, Runtime, Quality, Prerequisite, ExpenseException, SafeTimeoutWaiver any }
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
	Status                   string   `json:"status"`
	Reasons                  []string `json:"no_go_reasons"`
	Cost                     costs    `json:"all_new_measured_cost"`
	Cases                    []string `json:"verified_scenarios"`
	Prior                    int      `json:"prior_attempts"`
	Total                    int      `json:"carry_forward_attempts"`
	KnownHistoricalTotal     int64    `json:"prior_known_total_subtotal"`
	UnknownHistoricalTotal   int      `json:"prior_unknown_total_attempts"`
	AuditorCalls             int      `json:"auditor_native_calls"`
	AcceptedRetryFailures    int      `json:"accepted_model_retry_failures"`
	AcceptedUnknownCost      int      `json:"accepted_unknown_timeout_cost_entries"`
	AcceptedUnknownVersion   int      `json:"accepted_unknown_timeout_version_entries"`
	AcceptedUnmeasured       int      `json:"accepted_unmeasured_timeout_entries"`
	MeasuredComplete         bool     `json:"all_new_expense_measured"`
	Ready                    bool     `json:"issue33_ready"`
	Limits                   []string `json:"limits"`
	CommonCompletionEvidence string   `json:"common_completion_evidence,omitempty"`
	RetainedCases            []string `json:"retained_verified_scenarios,omitempty"`
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
	return auditWithPolicy(in, v6AuditPolicy())
}

func auditWithPolicy(in inputs, policy auditPolicy) (r report, err error) {
	selectedScenarios := reviewedScenarioNames(policy)
	r = report{Status: "NO_GO", Reasons: []string{}, Cases: []string{}, Prior: policy.Baseline, KnownHistoricalTotal: policy.KnownTotal, UnknownHistoricalTotal: policy.UnknownTotal, Limits: []string{
		"Read-only witness correlation on a shared administrator-controlled host; no hardware isolation or blind benchmark claim.",
		"Owner-bound setup and cheap checks are model-free. Native ordinary Core begins at the promoted input; production finite-IR builder authors the adapter.",
		"Project and Telegram recipients are loopback fixtures. The original result is prepared_verified and unapplied; no human recipient or live project deployment.",
		"Kernel budget reservations are conservative quotes; only the separate native ledger owns measured complete token totals, including thinking.",
		"Cloud weights, server effort/hardware/RAM and subscription invoice remain unavailable. Old unknown spend and UNKNOWN266 are preserved.",
		"The original human exception preserves failed537. The separately pinned prospective human waiver permits only one safely recovered response-stream timeout per input with all-three NULL cost or NULL failed version; these remain UNKNOWN. All successful requests retain measured usage and expected version.",
		"This chain witness result does not replace final-composition tests or issue-wide independent acceptance and does not authorize merge or execution.",
	}}
	if policy.Baseline >= 539 {
		r.Limits[5] = "The explicit V7 human amendment permits only one safely recovered nominal HTTP or response-stream timeout per input, retaining NULL usage/version. The new chain has 300-second entries, 40-minute cases and a 120-minute window; retained V5 quality/comparison remain their original 120-second/32768 profile. All successful requests require measured expense and expected version."
	}
	if policy.SerializedEntries {
		r.Limits = append(r.Limits, "V9 serialization is checked from owning client request intervals and durable measurements. It does not establish provider GPU scheduling or server hardware isolation.")
	}
	if policy.PrerequisiteSHA == v10PrerequisiteSHA {
		r.Limits[5] = "The pinned V10 prospective policy retains one safely recovered nominal HTTP or response-stream timeout per input with NULL expense/version kept UNKNOWN, 300-second entries, 40-minute cases and a 120-minute window. All successful requests require measured expense and expected version; original quality and comparison profiles remain unchanged."
		r.Limits = append(r.Limits, "V10 is a new prospective 71-entry allocation after factual556, with ceiling627. Closed earlier615 windows keep all spent requests and UNKNOWNs. Its pinned test-only harness lifetime is bounded by the remaining forty-minute case; historical556 runtime exit is unavailable and its specific cause stays UNKNOWN.")
	}
	if policy.RequireSourceObservation {
		r.Limits[5] = "The fixed V11 protocol retains the same critical recovery and measured-success predicates. A separately pinned historical557 exception preserves its closed source-check cancellation and NULL expense; it does not establish a nominal provider timeout or authorize a source-uncertain retry."
		r.Limits = append(r.Limits, "V11 is a fresh 557+71=628 allocation; prior615 and627 scopes stay closed without refunds. Failure-only source/watchdog diagnostics are content-free observations of unchanged checks, not authorization or proof of the old557 underlying cause. Pool counters are shared snapshots; query digests are content hashes, not an isolated exact-template registry.")
	}
	if policy.AllowRepeatedPatchChecks {
		r.Limits[5] = "The explicit V12 policy retains the historical557 expense exception, fully measured completed requests and proved same-run timeout recovery. Source uncertainty never grants retry or authority."
		r.Limits = append(r.Limits, "V12 uses a new 575+120=695 allocation and permits root project GET inspection/check counts under the unchanged owning Core budgets. Every read retains qualified run/call, actual PG result digest and validated canonical body digest; source-pinned capture is not raw HTTP/SSE proof. Writes, owner verification, Stop and delivery gates remain unchanged; closed V11 two-read response booleans stay UNKNOWN and NO_GO.")
	}
	if policy.PassStatus == "CHAIN_V13_WITNESS_AUDIT_PASS" || (policy.PassStatus == "CHAIN_V14_WITNESS_AUDIT_PASS" || policy.PassStatus == "CHAIN_V15_WITNESS_AUDIT_PASS") {
		r.Limits[5] = strings.Replace(r.Limits[5], "V12 policy", "V13 policy", 1)
		for i, line := range r.Limits {
			if strings.HasPrefix(line, "V11 is a fresh") {
				r.Limits[i] = "The retained closed V11 scope used557+71=628 without refunding prior615/627 history. Current failure-only source/watchdog diagnostics still observe unchanged checks; pool counters are shared snapshots and query digests are content hashes, not an isolated exact-template registry."
			}
			if strings.HasPrefix(line, "V12 uses") {
				r.Limits[i] = strings.Replace(line, "V12 uses", "V13 uses", 1)
			}
		}
		r.Limits = append(r.Limits, "The sealed V12 bootstrap adds no paid ordinal: its separately pinned independent proof derives zero native requests from source ordering and the actual constructor failure. Its original NULL counter remains NULL; empty exports alone do not prove zero. V13 uses a new fixed120 fixture constructor without reopening that failed scope.")
	}
	if policy.PassStatus == "CHAIN_V14_WITNESS_AUDIT_PASS" || policy.PassStatus == "CHAIN_V15_WITNESS_AUDIT_PASS" {
		for i, line := range r.Limits {
			line = strings.ReplaceAll(line, "V13 uses a new 575+120=695", "V14 uses a new 593+120=713")
			line = strings.Replace(line, "V13 policy", "V14 policy", 1)
			r.Limits[i] = line
		}
		r.Limits = append(r.Limits, "V14 reloads a source-pinned current confirmed activation as technical DATA for the original-work phase. It grants no new authority and is not a measured copy of model context. Its explicit fixed120 metadata reader preserves original arguments; the sealed V13 NULL metadata and empty Go stdout remain unchanged, with its supplemental NO_GO explicitly distinct from canonical Go output.")
	}
	if policy.RequireTaskScopedOriginalProof {
		for i, line := range r.Limits {
			line = strings.ReplaceAll(line, "V14 uses a new 593+120=713", "V15 uses a new 611+120=731")
			r.Limits[i] = strings.Replace(line, "V14 policy", "V15 policy", 1)
		}
		r.Limits = append(r.Limits, "V15 verifies earlier completed project phases under the same current root task and qualified run/call identities; the owner result remains bound to the actual original conversion final. Private full binding/constraints checks and qualified owner evidence references are enforced by frozen fixture SQL, with current public qualified receipts independently checked. The closed V14 original conversion and checked patch are technical facts only: its owner verification/notice and four fault variants did not run; its supplemental report is not canonical Go output.")
	}
	if policy.ScenarioNames != nil {
		for i, line := range r.Limits {
			line = strings.ReplaceAll(line, "V12 uses a new 575+120=695", "V16 uses a new 616+120=736")
			line = strings.Replace(line, "V12 policy", "V16 policy", 1)
			if strings.HasPrefix(line, "V11 is a fresh") {
				line = "Retained closed V11 and all subsequent scopes preserve actual spent requests and UNKNOWNs without refunds. Current source/watchdog diagnostic bounds remain unchanged; historical causes are not retroactively recovered."
			}
			r.Limits[i] = line
		}
		r.Limits = append(r.Limits, "The explicit user amendment runs only restart_sent, explicit_absence, lost_ack and owner_stop. Retained V14 original CSV/checked patch are technical facts; normal_sent stays NO_GO with no owner result/notice. The current real restart_sent must independently prove the complete ordinary original-result/current-owner/single-sent-notice tail; verified_scenarios never invents a normal_sent PASS. Per-request 4MiB and day480MiB input quotes are finite fixture bounds checked from actual durable model reservations/scopes; production defaults and all critical guards stay unchanged. Closed616 serialization rejection retains NULL usage/version and is not an accepted nominal retry or a zero-expense observation.")
	}
	if policy.CurrentSourceSemantics {
		for i, line := range r.Limits {
			if strings.HasPrefix(line, "The explicit user amendment runs only") {
				r.Limits[i] = "Current selected scenarios are " + strings.Join(selectedScenarios, ",") + ". Independently pinned earlier Stop and explicit-absence components are separate retained evidence; no completed case or normal work is rerun, and historical overall NO_GO results remain unchanged. Actual restart_sent must prove the complete current owner/result/single-sent-notice tail before any current composite PASS. Provider input quotes stay4MiB per model,480MiB day and16MiB delegated child under all eight dimensions."
			}
		}
	}
	defer func() {
		if v := recover(); v != nil {
			r.Status = "NO_GO"
			if policy.RequireProposals {
				// Accepted subsets describe a complete validated V8 proof. Keep
				// factual cost/unknown counts even when a later proposal check fails.
				r.AcceptedRetryFailures, r.AcceptedUnknownCost = 0, 0
				r.AcceptedUnknownVersion, r.AcceptedUnmeasured = 0, 0
			}
			code, ok := v.(string)
			if !ok {
				code = "typed_contract"
			}
			r.Reasons = append(r.Reasons, code)
			err = errors.New(code)
		}
	}()
	sc, pg := obj(in.Scope), obj(in.Ledger)
	scopeKeys := "status window_id baseline_attempts maximum_total_attempts maximum_new_attempts scenarios scope planning human_messages issue33_ready"
	if policy.RequireProposals {
		scopeKeys += " proposal_observations"
	}
	keys(sc, scopeKeys)
	keys(pg, "baseline_attempts total_attempts new_attempts requests window")
	ss := obj(sc["scope"])
	keys(ss, "total_attempts new_attempts unknown_full_token_cost unknown_token_cost_entries unknown_model_version_entries failed_provider_entries accepted_model_retry_failures accepted_unknown_timeout_cost_entries accepted_unknown_timeout_version_entries accepted_unmeasured_timeout_entries overlong_provider_entries requests window")
	nr, pr := arr(ss["requests"]), arr(pg["requests"])
	r.Total = policy.Baseline + len(pr)
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
	r.MeasuredComplete = r.Cost.Unknown == 0 && r.Cost.UnknownInput == 0 && r.Cost.UnknownOutput == 0
	q := obj(in.Quality)
	p := obj(in.Prerequisite)
	policy.VerifyHuman(in)
	require(q["status"] == "V5_PRIVATE_FINAL_SQL_CORRELATION_MATCH" && integer(q["exact_diagnostic_rows"]) == 252 && integer(q["exact_private_originals"]) == 252 && integer(q["auditor_native_calls"]) == 0 && q["issue33_ready"] == false, "quality_original_provenance")
	wa := obj(q["window_audit"])
	require(wa["status"] == "CLOUD_V5_WINDOW_AUDIT_PASS" && integer(wa["carry_forward_attempts"]) == 535 && len(arr(wa["no_go_reasons"])) == 0, "quality_prior_denied")
	policy.VerifyPrerequisite(p)
	require(sc["window_id"] == policy.WindowID && integer(sc["baseline_attempts"]) == int64(policy.Baseline) && integer(pg["baseline_attempts"]) == int64(policy.Baseline) && integer(sc["maximum_total_attempts"]) == int64(policy.Baseline+policy.Maximum) && integer(sc["maximum_new_attempts"]) == int64(policy.Maximum) && sc["planning"] == "actual_native_High" && sc["human_messages"] == json.Number("0") && sc["issue33_ready"] == false, "scope_identity")
	window, pw := obj(ss["window"]), obj(pg["window"])
	windowFields := "native_baseline attempts prior_history_hash prerequisite_hash expires_at maximum_new_attempts provider_stage_observation kernel_operation_binding"
	if policy.WindowPolicy != nil {
		windowFields += " protocol_policy"
		require(reflect.DeepEqual(obj(window["protocol_policy"]), policy.WindowPolicy), "explicit_v7_policy_binding")
	}
	keys(window, windowFields)
	keys(pw, windowFields)
	wcopy, pcopy := obj(clone(window)), obj(clone(pw))
	wcopy["expires_at"] = moment(window["expires_at"]).UTC().Format(time.RFC3339Nano)
	pcopy["expires_at"] = moment(pw["expires_at"]).UTC().Format(time.RFC3339Nano)
	require(reflect.DeepEqual(wcopy, pcopy) && integer(window["native_baseline"]) == int64(policy.Baseline) && window["prior_history_hash"] == policy.PriorSHA && window["prerequisite_hash"] == policy.PrerequisiteSHA && integer(window["maximum_new_attempts"]) == int64(policy.Maximum) && window["provider_stage_observation"] == true && window["kernel_operation_binding"] == true, "durable_window_binding")
	runtime := obj(in.Runtime)
	keys(runtime, "started_at finished_at exit_code oom_killed memory_limit_bytes image_id postgres_image_id converter_image_id")
	require(runtime["image_id"] == "sha256:5cf287a799e6b94384bad13d16b14904c531f51ba65792237e122ce42b392f61", "actual_native_image_identity")
	begin, end := moment(runtime["started_at"]), moment(runtime["finished_at"])
	expiry := moment(window["expires_at"])
	require(runtime["exit_code"] == json.Number("0") && runtime["oom_killed"] == false && integer(runtime["memory_limit_bytes"]) == 1536<<20 && !end.Before(begin) && expiry.After(begin) && !expiry.After(begin.Add(policy.Global)), "runtime_or_global_deadline")
	require(len(nr) > 0 && len(nr) <= policy.Maximum && len(nr) == len(pr), "entry_inventory")
	require(integer(ss["new_attempts"]) == int64(len(nr)) && integer(pg["new_attempts"]) == int64(len(nr)) && integer(ss["total_attempts"]) == int64(r.Total) && integer(pg["total_attempts"]) == int64(r.Total) && integer(window["attempts"]) == int64(r.Total), "spent_counter")
	require(sc["status"] == "passed" && integer(ss["overlong_provider_entries"]) == 0, "collector_outcome")
	results := obj(sc["scenarios"])
	require(len(results) == len(selectedScenarios), "scenario_inventory")
	grouped := map[string][]object{}
	lastScenario := -1
	var previous time.Time
	var previousFinished time.Time
	operations := map[string]bool{}
	failedEntries := 0
	unknownCostEntries, unknownVersionEntries, unmeasuredEntries := 0, 0, 0
	for i, v := range nr {
		a, b := obj(v), obj(pr[i])
		verifyRequestWithPolicy(a, b, policy)
		op := text(a["kernel_operation_id"])
		require(op != "" && !operations[op], "native_operation_duplicate")
		operations[op] = true
		if a["outcome"] != "completed" {
			failedEntries++
			if a["actual_total_tokens"] == nil {
				unknownCostEntries++
			}
			if a["model_version"] == nil {
				unknownVersionEntries++
			}
			if a["actual_total_tokens"] == nil || a["model_version"] == nil {
				unmeasuredEntries++
			}
		}
		require(integer(a["ordinal"]) == int64(policy.Baseline+1+i), "ordinal_replay_or_gap")
		name := text(a["scenario"])
		pos := -1
		for j, s := range selectedScenarios {
			if s == name {
				pos = j
			}
		}
		require(pos >= lastScenario && pos >= 0, "scenario_release_order")
		lastScenario = pos
		at, finish := moment(a["admitted_at_utc"]), moment(a["finished_at_utc"])
		if policy.SerializedEntries {
			require(i == 0 || !at.Before(previousFinished), "v9_client_request_intervals_overlap")
		}
		require(!at.Before(begin) && !finish.After(end) && at.Before(expiry) && !finish.After(expiry) && !at.Before(previous) && !finish.Before(at) && finish.Sub(at) <= policy.Attempt, "request_deadline")
		elapsed := integer(a["elapsed_ms"])
		require(elapsed <= policy.Attempt.Milliseconds() && finish.Sub(at).Milliseconds()-elapsed >= -2 && finish.Sub(at).Milliseconds()-elapsed <= 2, "request_elapsed")
		previous = at
		previousFinished = finish
		grouped[name] = append(grouped[name], a)
	}
	initial, product := witnesses(in.Initial), witnesses(in.Product)
	require(len(initial) == len(selectedScenarios) && len(product) == len(selectedScenarios), "owning_product_inventory")
	var priorFinal time.Time
	for _, name := range selectedScenarios {
		require(results[name] == true && len(grouped[name]) > 0, "scenario_missing")
		first, last := grouped[name][0], grouped[name][len(grouped[name])-1]
		ini, fin := initial[name], product[name]
		require(ini != nil && fin != nil, "scenario_witness_missing")
		require(!moment(ini["created_at"]).Before(begin) && moment(ini["created_at"]).Before(moment(first["admitted_at_utc"])) && !moment(ini["created_at"]).Before(priorFinal) && !moment(fin["created_at"]).Before(moment(last["finished_at_utc"])) && !moment(fin["created_at"]).After(end), "initial_final_causality")
		verifyProductWithPolicy(name, obj(ini["payload_json"]), obj(fin["payload_json"]), grouped[name], policy)
		if policy.RequireTaskScopedOriginalProof {
			verifyV15TaskScopedOriginalProof(name, obj(ini["payload_json"]), obj(fin["payload_json"]))
		}
		r.AcceptedRetryFailures += verifyModelRetries(name, obj(obj(fin["payload_json"])["postgres"]), grouped[name])
		require(moment(fin["created_at"]).Sub(moment(ini["created_at"])) <= policy.Case, "case_lifetime")
		priorFinal = moment(fin["created_at"])
		r.Cases = append(r.Cases, name)
	}
	require(integer(ss["failed_provider_entries"]) == int64(failedEntries) && integer(ss["accepted_model_retry_failures"]) == int64(r.AcceptedRetryFailures) && r.AcceptedRetryFailures == failedEntries, "retry_failure_accounting")
	// Only after every failed predecessor has independently matched its exact
	// durable retry and safe final product may its missing observation receive
	// this narrow waiver. No successful request or unrelated UNKNOWN is covered.
	r.AcceptedUnknownCost, r.AcceptedUnknownVersion = unknownCostEntries, unknownVersionEntries
	r.AcceptedUnmeasured = unmeasuredEntries
	require(integer(ss["unknown_full_token_cost"]) == int64(unmeasuredEntries) && integer(ss["unknown_token_cost_entries"]) == int64(unknownCostEntries) && integer(ss["unknown_model_version_entries"]) == int64(unknownVersionEntries) && integer(ss["accepted_unknown_timeout_cost_entries"]) == int64(unknownCostEntries) && integer(ss["accepted_unknown_timeout_version_entries"]) == int64(unknownVersionEntries) && integer(ss["accepted_unmeasured_timeout_entries"]) == int64(unmeasuredEntries), "narrow_timeout_unknown_accounting")
	require(r.Cost.Unknown == unknownCostEntries && r.Cost.UnknownInput == unknownCostEntries && r.Cost.UnknownOutput == unknownCostEntries, "unwaived_unknown_cost")
	if policy.RequireProposals {
		verifyV8Proposals(in, policy)
	}
	if policy.ScenarioNames != nil {
		r.CommonCompletionEvidence = "restart_sent_current_owner_result_and_single_sent_notice"
		r.RetainedCases = append([]string(nil), policy.RetainedScenarioNames...)
	}
	r.Status = policy.PassStatus
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
	verifyRequestWithPolicy(a, b, v6AuditPolicy())
}
func verifyRequestWithPolicy(a, b object, policy auditPolicy) {
	requestFields := "ordinal scenario kind root_task_id run_id is_parent input_sha256 admitted_at_utc finished_at_utc elapsed_ms outcome failure_stage diagnostic_code actual_input_tokens actual_output_tokens actual_total_tokens model_version final_count terminal_count completed_count cancelled_count timed_out_count transport_error_count finish_reason usage_observation_uncertain version_observation_uncertain stop_final_count max_tokens_final_count finish_observation_uncertain provider_operation_stage kernel_operation_id"
	if policy.RequireUnavailableClass {
		requestFields += " provider_unavailable_class"
		verifyV20UnavailableClassObservation(a)
	}
	keys(a, requestFields)
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
	require(a["usage_observation_uncertain"] == false && a["version_observation_uncertain"] == false && a["finish_observation_uncertain"] == false, "native_terminal_contract")
	require(a["provider_operation_stage"] == "response_stream" || a["outcome"] != "completed" && policy.HTTPTimeouts && a["provider_operation_stage"] == "http_round_trip", "actual_provider_operation_stage")
	require(a["kind"] == "start" || a["kind"] == "continue" || a["kind"] == "submit", "entry_kind")
	sha(a["input_sha256"])
	require(text(a["run_id"]) != "" && text(a["root_task_id"]) != "", "entry_owner")
	fc, tc, cc := integer(a["final_count"]), integer(a["terminal_count"]), integer(a["completed_count"])
	if a["outcome"] != "completed" {
		require((a["outcome"] == "timed_out" || a["outcome"] == "transport_error") && a["diagnostic_code"] == "provider_timeout" && (a["failure_stage"] == "entry" || a["failure_stage"] == "stream"), "only_nominal_timeout_retriable")
		require(a["model_version"] == nil || a["model_version"] == "gemini-3.8-flash-n", "failed_timeout_version_contract")
		verifyNativeCost(a, true)
		require(fc == 0 && cc == 0 && integer(a["cancelled_count"]) == 0 && tc <= 1 && tc == integer(a["timed_out_count"])+integer(a["transport_error_count"]) && integer(a["stop_final_count"])+integer(a["max_tokens_final_count"]) == 0 && a["finish_reason"] == nil, "failed_retry_original_lifecycle")
		return
	}
	require(a["model_version"] == "gemini-3.8-flash-n", "native_terminal_contract")
	verifyNativeCost(a, false)
	require(a["failure_stage"] == nil && a["diagnostic_code"] == nil, "native_success_diagnostic")
	require(fc <= 1 && tc == cc && integer(a["cancelled_count"])+integer(a["timed_out_count"])+integer(a["transport_error_count"]) == 0 && integer(a["max_tokens_final_count"]) == 0, "clean_original_lifecycle")
	if fc == 1 {
		require(tc == 1 && a["finish_reason"] == "STOP" && integer(a["stop_final_count"]) == 1, "final_stop_contract")
	} else {
		require(tc == 0 && a["finish_reason"] == nil && integer(a["stop_final_count"]) == 0, "tool_request_contract")
	}
}

func verifyNativeCost(a object, allowUnmeasuredTimeout bool) {
	missing := 0
	for _, key := range []string{"actual_input_tokens", "actual_output_tokens", "actual_total_tokens"} {
		if a[key] == nil {
			missing++
		}
	}
	if missing == 3 {
		require(allowUnmeasuredTimeout, "successful_cost_must_be_measured")
		return
	}
	require(missing == 0, "partial_timeout_cost_denied")
	input, output, total := integer(a["actual_input_tokens"]), integer(a["actual_output_tokens"]), integer(a["actual_total_tokens"])
	require(input <= 1048576 && output <= 32768 && total >= input+output && total <= input+32768, "native_generation_allowance")
}
func sortedByTime(rows []object, key string) {
	sort.SliceStable(rows, func(i, j int) bool { return moment(rows[i][key]).Before(moment(rows[j][key])) })
}
