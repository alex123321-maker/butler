package app

import (
	"encoding/json"
	"reflect"
	"testing"
	"time"
)

func releaseChainV6Object(m map[string]any, key string) map[string]any {
	v, _ := m[key].(map[string]any)
	return v
}
func releaseChainV6Array(m map[string]any, key string) []any { v, _ := m[key].([]any); return v }
func releaseChainV6Exact(m map[string]any, expected map[string]any) bool {
	for key, want := range expected {
		got, ok := m[key]
		if !ok || !reflect.DeepEqual(got, want) {
			return false
		}
	}
	return true
}

func readReleaseChainV6FailureObjects() (map[string]map[string]any, error) {
	all := map[string]map[string]any{}
	for kind, pin := range releaseChainV6FailedArtifacts() {
		raw, err := readReleaseChainV5Artifact(highHistoryPath("release-chain-v5-"+kind+"-20261013.json"), pin)
		if err != nil || !json.Valid(raw) {
			return nil, errNativeFixtureDenied
		}
		// The separately frozen independent correlation owns product/initial
		// semantics. Their exact array bytes are bound, never rewritten here.
		if kind == "pg-product" || kind == "pg-initial" {
			var rows []json.RawMessage
			if json.Unmarshal(raw, &rows) != nil {
				return nil, errNativeFixtureDenied
			}
			continue
		}
		var body map[string]any
		if json.Unmarshal(raw, &body) != nil {
			return nil, errNativeFixtureDenied
		}
		all[kind] = body
	}
	return all, nil
}

func releaseChainV6CanonicalRow(pg map[string]any) (map[string]any, error) {
	result := map[string]any{}
	rename := map[string]string{"input_hash": "input_sha256", "input_tokens": "actual_input_tokens", "output_tokens": "actual_output_tokens", "total_tokens": "actual_total_tokens", "admitted_at": "admitted_at_utc", "finished_at": "finished_at_utc"}
	for key, value := range pg {
		if next, ok := rename[key]; ok {
			key = next
		}
		if key == "admitted_at_utc" || key == "finished_at_utc" {
			text, ok := value.(string)
			if !ok {
				return nil, errNativeFixtureDenied
			}
			at, err := time.Parse(time.RFC3339Nano, text)
			if err != nil {
				return nil, errNativeFixtureDenied
			}
			value = at.UTC().Format(time.RFC3339Nano)
		}
		result[key] = value
	}
	return result, nil
}

// Fixed, exact-hash safe sources remain the provenance boundary. These numeric
// and leaf checks additionally make the carry forward semantics reviewable.
func validateReleaseChainV6FailedArtifacts(all map[string]map[string]any) error {
	f, c, s, pg, rt := all["final-audit"], all["failure-correlation"], all["high"], all["pg-ledger"], all["runtime-state"]
	if !releaseChainV6Exact(s, map[string]any{"status": "failed", "window_id": releaseChainV5WindowID, "baseline_attempts": float64(535), "maximum_new_attempts": float64(80), "maximum_total_attempts": float64(615), "human_messages": float64(0), "issue33_ready": false, "planning": "actual_native_High"}) || !reflect.DeepEqual(s["scenarios"], map[string]any{"normal_sent": false}) {
		return errNativeFixtureDenied
	}
	scope := releaseChainV6Object(s, "scope")
	window := releaseChainV6Object(scope, "window")
	if !releaseChainV6Exact(scope, map[string]any{"total_attempts": float64(537), "new_attempts": float64(2), "unknown_full_token_cost": float64(1), "failed_provider_entries": float64(1), "overlong_provider_entries": float64(0)}) || !releaseChainV6Exact(window, map[string]any{"native_baseline": float64(535), "attempts": float64(537), "prior_history_hash": "2539239ea8905a677cb14b0256a9744c3354e365754cae127bc517c54c35d5af", "prerequisite_hash": releaseChainV5ReviewedPrerequisiteSHA, "expires_at": "2026-10-13T08:56:45+00:00"}) || len(window) != 5 || !reflect.DeepEqual(pg["window"], window) || !releaseChainV6Exact(pg, map[string]any{"baseline_attempts": float64(535), "total_attempts": float64(537), "new_attempts": float64(2)}) {
		return errNativeFixtureDenied
	}
	rows, sqlRows, corrRows := releaseChainV6Array(scope, "requests"), releaseChainV6Array(pg, "requests"), releaseChainV6Array(c, "requests")
	if len(rows) != 2 || len(sqlRows) != 2 || len(corrRows) != 2 {
		return errNativeFixtureDenied
	}
	for i, item := range rows {
		r, ok := item.(map[string]any)
		sql, sqlOK := sqlRows[i].(map[string]any)
		cr, corrOK := corrRows[i].(map[string]any)
		if !ok || !sqlOK || !corrOK {
			return errNativeFixtureDenied
		}
		canonical, err := releaseChainV6CanonicalRow(sql)
		if err != nil || !reflect.DeepEqual(canonical, r) || !releaseChainV6Exact(r, map[string]any{"ordinal": float64(536 + i), "scenario": "normal_sent", "is_parent": true, "root_task_id": "task_a724083871d24abb67cef04265bb5a31", "run_id": "run-1791878233983966157-7e47e927d33a91e9", "final_count": float64(0), "terminal_count": float64(0), "completed_count": float64(0), "cancelled_count": float64(0), "timed_out_count": float64(0), "transport_error_count": float64(0), "stop_final_count": float64(0), "max_tokens_final_count": float64(0), "finish_reason": nil, "usage_observation_uncertain": false, "version_observation_uncertain": false, "finish_observation_uncertain": false}) {
			return errNativeFixtureDenied
		}
		if _, exists := r["provider_operation_stage"]; exists {
			return errNativeFixtureDenied
		} // old537 UNKNOWN stays absent
		for key, value := range cr {
			if key == "input_tokens" || key == "output_tokens" || key == "total_tokens" {
				key = "actual_" + key
			}
			if !reflect.DeepEqual(r[key], value) {
				return errNativeFixtureDenied
			}
		}
		if i == 0 && !releaseChainV6Exact(r, map[string]any{"kind": "start", "outcome": "completed", "actual_input_tokens": float64(7106), "actual_output_tokens": float64(155), "actual_total_tokens": float64(8462), "model_version": "gemini-3.8-flash-n", "elapsed_ms": float64(13487), "failure_stage": nil, "diagnostic_code": nil, "input_sha256": "30ada785dd567631b75a500c9aaf90a4c18e1bb52707aa20c2dbed117aa3be34"}) {
			return errNativeFixtureDenied
		}
		if i == 1 && !releaseChainV6Exact(r, map[string]any{"kind": "submit", "outcome": "timed_out", "actual_input_tokens": nil, "actual_output_tokens": nil, "actual_total_tokens": nil, "model_version": nil, "elapsed_ms": float64(119977), "failure_stage": "stream", "diagnostic_code": "provider_timeout", "input_sha256": "a30151f4b6a54f7f5d4ced2df306515a0bc1570d18137392e1ff485be16b38bd"}) {
			return errNativeFixtureDenied
		}
	}
	if !releaseChainV6Exact(c, map[string]any{"status": "INDEPENDENT_ACTUAL_V5_CHAIN_NO_GO_CORRELATED", "scope_status": "failed", "code_commit": releaseChainV6FailedCode, "source_sha256": releaseChainV6FailedSourceSHA, "native_scope_sealed": true, "retry_or_reopen_performed": false, "auditor_native_calls": float64(0), "issue33_ready": false, "checker_report_sha256": releaseChainV6FailedAuditSHA, "checker_report_bytes": float64(2634), "actual_root_state": "blocked", "only_confirmed_tool": "converters.discover", "actual_project_patch_prepared": false, "actual_original_result": nil, "actual_notice_count": float64(0), "actual_channel_requests": float64(0)}) || len(releaseChainV6Array(c, "verified_scenarios")) != 0 || !reflect.DeepEqual(c["unattempted_scenarios"], []any{"restart_sent", "explicit_absence", "lost_ack", "owner_stop"}) {
		return errNativeFixtureDenied
	}
	if !releaseChainV6Exact(releaseChainV6Object(c, "carry_forward"), map[string]any{"attempts": float64(537), "known_input_tokens_subtotal": float64(294598), "known_output_tokens_subtotal": float64(11827), "known_total_tokens_subtotal": float64(572459), "unknown_input_attempts": float64(27), "unknown_output_attempts": float64(27), "unknown_total_attempts": float64(47), "old_unknown_266_cause": "UNKNOWN"}) {
		return errNativeFixtureDenied
	}
	cost := map[string]any{"attempts": float64(2), "known_input_tokens": float64(7106), "known_output_tokens": float64(155), "known_total_tokens": float64(8462), "unknown_input_attempts": float64(1), "unknown_output_attempts": float64(1), "unknown_total_attempts": float64(1)}
	if !reflect.DeepEqual(c["all_new_measured_cost"], cost) || !releaseChainV6Exact(releaseChainV6Object(f, "chain_audit"), map[string]any{"status": "NO_GO", "prior_attempts": float64(535), "carry_forward_attempts": float64(537), "prior_known_total_subtotal": float64(563997), "prior_unknown_total_attempts": float64(46), "auditor_native_calls": float64(0), "issue33_ready": false}) || !reflect.DeepEqual(releaseChainV6Object(f, "chain_audit")["all_new_measured_cost"], cost) || !reflect.DeepEqual(f["evidence_bindings"], c["evidence_bindings"]) || !releaseChainV6Exact(rt, map[string]any{"exit_code": float64(1), "oom_killed": false, "memory_limit_bytes": float64(1610612736)}) {
		return errNativeFixtureDenied
	}
	bindings := releaseChainV6Object(c, "evidence_bindings")
	if !releaseChainV6Exact(bindings, map[string]any{"code_commit": releaseChainV6FailedCode, "source_sha256": releaseChainV6FailedSourceSHA, "app_sha256": releaseChainV6FailedRuntimeSHA, "prerequisite_sha256": releaseChainV5ReviewedPrerequisiteSHA, "quality_sha256": releaseChainV5ReviewedOriginalAuditSHA}) {
		return errNativeFixtureDenied
	}
	for kind, pin := range releaseChainV6FailedArtifacts() {
		if kind == "final-audit" || kind == "failure-correlation" {
			continue
		}
		key := kind + "_sha256"
		if kind == "high" {
			key = "scope_sha256"
		}
		if bindings[key] != pin {
			return errNativeFixtureDenied
		}
	}
	if !releaseChainV6Exact(releaseChainV6Object(c, "failure_classification"), map[string]any{"ordinal": float64(537), "internal_cloud_cause": "UNKNOWN", "global_deadline_expired": false, "original_terminal_observed": false, "failure_stage": "stream", "diagnostic_code": "provider_timeout", "entry_elapsed_ms": float64(119977), "actual_exit_code": float64(1), "oom_killed": false}) {
		return errNativeFixtureDenied
	}
	return nil
}

func TestReleaseChainV6ActualHistoryPreservesClosedFailureAndNULL(t *testing.T) {
	h, err := readReleaseChainV6ActualHistory()
	if err != nil {
		t.Fatal("actual535 plus sealed536/537 did not bind", err)
	}
	if h.Usage[536].ActualInputTokens != nil || h.Usage[536].ActualOutputTokens != nil || h.Usage[535].ActualInputTokens == nil || *h.Usage[535].ActualInputTokens != 7106 {
		t.Fatal("lost expense became inferred or old row changed")
	}
	t.Log("Actual537 history fold:", h.BaselineSHA256)
}

func TestReleaseChainV6ActualBridgeTamperMatrix(t *testing.T) {
	base, err := readReleaseChainV6FailureObjects()
	if err != nil {
		t.Fatal("safe artifact unavailable")
	}
	if validateReleaseChainV6FailedArtifacts(base) != nil {
		t.Fatal("actual positive bridge denied")
	}
	for _, tc := range []struct {
		name   string
		change func(map[string]map[string]any)
	}{
		{"refunded_baseline", func(m map[string]map[string]any) { m["high"]["baseline_attempts"] = float64(537) }},
		{"old_window_deadline_extended", func(m map[string]map[string]any) {
			releaseChainV6Object(releaseChainV6Object(m["high"], "scope"), "window")["expires_at"] = "2026-10-14T08:56:45+00:00"
		}},
		{"scope_pg_metadata_divergence", func(m map[string]map[string]any) {
			releaseChainV6Object(m["pg-ledger"], "window")["attempts"] = float64(538)
		}},
		{"old_no_go_changed", func(m map[string]map[string]any) {
			releaseChainV6Object(m["final-audit"], "chain_audit")["status"] = "PASS"
		}},
		{"native_source_changed", func(m map[string]map[string]any) {
			releaseChainV6Object(m["failure-correlation"], "evidence_bindings")["source_sha256"] = "UNSET"
		}},
		{"known_total_inflated", func(m map[string]map[string]any) {
			releaseChainV6Object(m["failure-correlation"], "carry_forward")["known_total_tokens_subtotal"] = float64(600000)
		}},
		{"unknown_total_erased", func(m map[string]map[string]any) {
			releaseChainV6Object(m["failure-correlation"], "carry_forward")["unknown_total_attempts"] = float64(46)
		}},
		{"child_invented", func(m map[string]map[string]any) {
			releaseChainV6Array(releaseChainV6Object(m["high"], "scope"), "requests")[1].(map[string]any)["is_parent"] = false
		}},
		{"537_usage_filled", func(m map[string]map[string]any) {
			releaseChainV6Array(releaseChainV6Object(m["high"], "scope"), "requests")[1].(map[string]any)["actual_total_tokens"] = float64(0)
		}},
		{"537_version_inherited", func(m map[string]map[string]any) {
			releaseChainV6Array(releaseChainV6Object(m["high"], "scope"), "requests")[1].(map[string]any)["model_version"] = "gemini-3.8-flash-n"
		}},
		{"537_substage_guessed", func(m map[string]map[string]any) {
			releaseChainV6Array(releaseChainV6Object(m["high"], "scope"), "requests")[1].(map[string]any)["provider_operation_stage"] = "response_stream"
		}},
		{"missing_sql_row", func(m map[string]map[string]any) {
			m["pg-ledger"]["requests"] = releaseChainV6Array(m["pg-ledger"], "requests")[:1]
		}},
		{"retroactive_release_claim", func(m map[string]map[string]any) { m["failure-correlation"]["issue33_ready"] = true }},
	} {
		t.Run(tc.name, func(t *testing.T) {
			raw, _ := json.Marshal(base)
			var m map[string]map[string]any
			_ = json.Unmarshal(raw, &m)
			tc.change(m)
			if validateReleaseChainV6FailedArtifacts(m) == nil {
				t.Fatal("tampered failed history became admission evidence")
			}
		})
	}
}

func TestReleaseChainV6HumanExpenseExceptionOnly537(t *testing.T) {
	e, err := readReleaseChainV6ExpenseException()
	if err != nil {
		t.Fatal("trusted human exception did not bind")
	}
	for _, tc := range []struct {
		name   string
		change func(*releaseChainV6ExpenseException)
	}{
		{"future_unknown", func(e *releaseChainV6ExpenseException) { e.FutureExceptions = true }},
		{"wrong_ordinal", func(e *releaseChainV6ExpenseException) { e.Ordinal = 538 }},
		{"erased_null", func(e *releaseChainV6ExpenseException) { e.RetainNULL = false }},
		{"changed_no_go", func(e *releaseChainV6ExpenseException) { e.RetainNO_GO = false }},
		{"new_unknown_cost", func(e *releaseChainV6ExpenseException) { e.NewKnown = false }},
		{"refilled_attempts", func(e *releaseChainV6ExpenseException) { e.Remaining = 80 }},
		{"changed_threshold", func(e *releaseChainV6ExpenseException) { e.ChangedCriteria = true }},
	} {
		t.Run(tc.name, func(t *testing.T) {
			copy := e
			tc.change(&copy)
			if validateReleaseChainV6ExpenseException(copy) == nil {
				t.Fatal("human authority widened")
			}
		})
	}
}

func releaseChainV6ProspectivePrerequisite(h nativeHistoricalEvidence) releaseChainV6Prerequisite {
	return releaseChainV6Prerequisite{Version: "butler-chain-high-reviewed-v6-prerequisite-v1", Baseline: 537, Maximum: 78, Ceiling: 615, HistorySHA: h.BaselineSHA256, QualitySHA: releaseChainV5ReviewedPrerequisiteSHA, OriginalQualitySHA: releaseChainV5ReviewedOriginalAuditSHA, FailedAuditSHA: releaseChainV6FailedAuditSHA, FailedCorrelationSHA: releaseChainV6FailedCorrelationSHA, ExpenseExceptionSHA: releaseChainV6ExceptionSHA, KnownInput: 294598, KnownOutput: 11827, KnownTotal: 572459, UnknownTotal: 47, KnownPairs: 510, UnknownPairs: 27, ProviderStages: true, RequireKnown: true, Generation: 32768, Model: "gemini-3.8-flash-high", RetryPolicy: "one_nominal_timeout_per_accepted_input_v1", MaximumRetries: 1, RetryBackoffMillis: 2000, KernelOperations: true}
}

func TestReleaseChainV6PrerequisiteTamperMatrix(t *testing.T) {
	h, err := readReleaseChainV6ActualHistory()
	if err != nil {
		t.Fatal("actual history unavailable")
	}
	p := releaseChainV6ProspectivePrerequisite(h)
	if validateReleaseChainV6Prerequisite(p, h) != nil {
		t.Fatal("prospective metadata failed; this still grants no entry")
	}
	for _, tc := range []struct {
		name   string
		change func(*releaseChainV6Prerequisite)
	}{
		{"baseline_refund", func(p *releaseChainV6Prerequisite) { p.Baseline = 535 }},
		{"new_cap80", func(p *releaseChainV6Prerequisite) { p.Maximum = 80 }},
		{"absolute_ceiling617", func(p *releaseChainV6Prerequisite) { p.Ceiling = 617 }},
		{"prior_history_changed", func(p *releaseChainV6Prerequisite) { p.HistorySHA = "UNSET" }},
		{"quality_bridge_changed", func(p *releaseChainV6Prerequisite) { p.QualitySHA = "UNSET" }},
		{"quality_full_audit_changed", func(p *releaseChainV6Prerequisite) { p.OriginalQualitySHA = "UNSET" }},
		{"failed_no_go_audit_changed", func(p *releaseChainV6Prerequisite) { p.FailedAuditSHA = "UNSET" }},
		{"failed_correlation_changed", func(p *releaseChainV6Prerequisite) { p.FailedCorrelationSHA = "UNSET" }},
		{"human_exception_unbound", func(p *releaseChainV6Prerequisite) { p.ExpenseExceptionSHA = "UNSET" }},
		{"unknown537_removed", func(p *releaseChainV6Prerequisite) { p.UnknownTotal = 46 }},
		{"subtotal_guessed", func(p *releaseChainV6Prerequisite) { p.KnownTotal++ }},
		{"input_guessed", func(p *releaseChainV6Prerequisite) { p.KnownInput++ }},
		{"output_guessed", func(p *releaseChainV6Prerequisite) { p.KnownOutput++ }},
		{"new_unknown_expense_allowed", func(p *releaseChainV6Prerequisite) { p.RequireKnown = false }},
		{"observer_disabled", func(p *releaseChainV6Prerequisite) { p.ProviderStages = false }},
		{"retry_policy_changed", func(p *releaseChainV6Prerequisite) { p.RetryPolicy = "provider_hidden_retry" }},
		{"multiple_retries", func(p *releaseChainV6Prerequisite) { p.MaximumRetries = 2 }},
		{"backoff_removed", func(p *releaseChainV6Prerequisite) { p.RetryBackoffMillis = 0 }},
		{"kernel_binding_disabled", func(p *releaseChainV6Prerequisite) { p.KernelOperations = false }},
		{"generation_relaxed", func(p *releaseChainV6Prerequisite) { p.Generation = 65536 }},
		{"model_changed", func(p *releaseChainV6Prerequisite) { p.Model = "gemini-3.8-flash-low" }},
	} {
		t.Run(tc.name, func(t *testing.T) {
			copy := p
			tc.change(&copy)
			if validateReleaseChainV6Prerequisite(copy, h) == nil {
				t.Fatal("prospective gate widened")
			}
		})
	}
}
