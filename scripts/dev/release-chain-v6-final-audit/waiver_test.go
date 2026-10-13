package main

import (
	"testing"
)

func refreshNativeUnknownCounters(x *inputs) {
	scope := obj(obj(x.Scope)["scope"])
	cost, version, either := int64(0), int64(0), int64(0)
	for _, value := range arr(scope["requests"]) {
		n := obj(value)
		missingCost := n["actual_input_tokens"] == nil || n["actual_output_tokens"] == nil || n["actual_total_tokens"] == nil
		missingVersion := n["model_version"] == nil
		if missingCost {
			cost++
		}
		if missingVersion {
			version++
		}
		if missingCost || missingVersion {
			either++
		}
	}
	scope["unknown_token_cost_entries"], scope["accepted_unknown_timeout_cost_entries"] = jsonNumber(cost), jsonNumber(cost)
	scope["unknown_model_version_entries"], scope["accepted_unknown_timeout_version_entries"] = jsonNumber(version), jsonNumber(version)
	scope["unknown_full_token_cost"], scope["accepted_unmeasured_timeout_entries"] = jsonNumber(either), jsonNumber(either)
}

func changeNative(x *inputs, index int, change func(object)) {
	n := obj(arr(obj(obj(x.Scope)["scope"])["requests"])[index])
	change(n)
	arr(obj(x.Ledger)["requests"])[index] = sqlRequest(n)
	refreshNativeUnknownCounters(x)
}

func waivedStreamTimeout(t *testing.T, scenarioIndex int, unknownCost, unknownVersion bool) inputs {
	t.Helper()
	x := syntheticMeasuredRetryForScenario(t, scenarioIndex)
	index := -1
	for j, value := range arr(obj(obj(x.Scope)["scope"])["requests"]) {
		if obj(value)["outcome"] != "completed" {
			index = j
			break
		}
	}
	require(index >= 0, "waiver_fixture_failed_predecessor")
	var operation string
	changeNative(&x, index, func(n object) {
		operation = text(n["kernel_operation_id"])
		if unknownCost {
			n["actual_input_tokens"], n["actual_output_tokens"], n["actual_total_tokens"] = nil, nil, nil
		}
		if unknownVersion {
			n["model_version"] = nil
		}
	})
	if unknownCost {
		tamperProduct(&x, scenarioIndex, func(p object) {
			for _, m := range rows(obj(p["postgres"]), "model_budget_attempts") {
				if m["operation_id"] == operation {
					m["actual_input_tokens"], m["actual_output_tokens"], m["observed_at"] = nil, nil, nil
				}
			}
		})
	}
	return x
}

func TestV6HumanWaiverRetainsPriorProtocolAndEverySafetyGate(t *testing.T) {
	verifyPriorV6ProtocolSource(candidateRepo(t))
	w := actualPublic(t, "release-chain-v6-user-safe-timeout-waiver-20261013.json", safeTimeoutWaiverSHA)
	verifySafeTimeoutWaiver(w)
	cases := map[string]func(object){
		"fake_human_reply":         func(x object) { x["user_reply"] = "allow every failure" },
		"generic_unknown":          func(x object) { x["other_new_unknown_measurements_allowed"] = true },
		"other_thresholds_relaxed": func(x object) { x["other_safety_or_quality_criteria_changed"] = true },
		"partial_cost":             func(x object) { x["failed_usage_shape"] = "any_missing_dimension" },
		"http_only":                func(x object) { x["required_provider_operation_stage"] = "http_round_trip" },
		"any_failure":              func(x object) { x["required_diagnostic_code"] = "provider_unavailable" },
		"known_wrong_version":      func(x object) { x["failed_model_version_shape"] = "any_version" },
		"second_retry":             func(x object) { x["maximum_model_retries_per_input"] = jsonNumber(2) },
		"short_backoff":            func(x object) { x["model_retry_backoff_ms"] = jsonNumber(0) },
		"old_protocol_replaced":    func(x object) { x["prior_protocol_sha256"] = expenseExceptionSHA },
		"old537_removed":           func(x object) { x["retained_537_exception_sha256"] = nil },
		"old_nulls_recovered":      func(x object) { x["retain_original_null_measurements"] = false },
		"old_no_go_promoted":       func(x object) { x["retain_historical_no_go"] = false },
		"unknown_success":          func(x object) { x["all_completed_entries_require_measured_usage_and_selected_version"] = false },
		"unverified_result":        func(x object) { x["require_fixed_scenario_result_or_owner_stop_guarantees"] = false },
		"no_kernel_binding":        func(x object) { x["require_exact_kernel_operation_and_spent_reservations"] = false },
		"no_started_fresh_start":   func(x object) { x["require_started_retry_and_fresh_completed_start"] = false },
		"refill80":                 func(x object) { x["maximum_new_attempts"] = jsonNumber(80) },
		"larger_ceiling":           func(x object) { x["maximum_total_attempts"] = jsonNumber(617) },
		"longer_attempt":           func(x object) { x["maximum_attempt_ms"] = jsonNumber(121000) },
		"longer_case":              func(x object) { x["maximum_case_ms"] = jsonNumber(1200001) },
		"longer_global":            func(x object) { x["maximum_global_ms"] = jsonNumber(3600001) },
		"no_independent_review":    func(x object) { x["independent_native_admission_still_required"] = false },
		"premature_ready":          func(x object) { x["issue33_ready"] = true },
		"signature_invented":       func(x object) { x["external_signature"] = "unverified" },
	}
	for name, mutate := range cases {
		t.Run(name, func(t *testing.T) {
			x := obj(clone(w))
			mutate(x)
			denied := false
			func() { defer func() { denied = recover() != nil }(); verifySafeTimeoutWaiver(x) }()
			if !denied {
				t.Fatal("human waiver widened or old evidence rewritten")
			}
		})
	}
}

func TestV6WaivedStreamTimeoutKeepsUnknownWithSafeFixedScenario(t *testing.T) {
	for index, name := range scenarios {
		t.Run(name, func(t *testing.T) {
			x := waivedStreamTimeout(t, index, true, true)
			r, err := audit(x)
			if err != nil || r.Status != "CHAIN_V6_WITNESS_AUDIT_PASS" || r.Cost.Attempts != 71 || r.Cost.Input != 700 || r.Cost.Output != 140 || r.Cost.Total != 1050 || r.Cost.Unknown != 1 || r.Cost.UnknownInput != 1 || r.Cost.UnknownOutput != 1 || r.AcceptedUnknownCost != 1 || r.AcceptedUnknownVersion != 1 || r.AcceptedUnmeasured != 1 || r.AcceptedRetryFailures != 1 || r.MeasuredComplete || r.UnknownHistoricalTotal != 47 || r.KnownHistoricalTotal != 572459 || r.Ready {
				t.Fatal("safe timeout lost original NULLs or strict scenario guarantees", err, r)
			}
		})
	}
}

func TestV6WaivedCostAndVersionCountersRemainSeparate(t *testing.T) {
	for _, tc := range []struct {
		name          string
		cost, version bool
	}{{"missing_cost_only", true, false}, {"missing_version_only", false, true}, {"both_known", false, false}} {
		t.Run(tc.name, func(t *testing.T) {
			x := waivedStreamTimeout(t, 0, tc.cost, tc.version)
			r, err := audit(x)
			cost, version := 0, 0
			total := int64(1065)
			if tc.cost {
				cost, total = 1, 1050
			}
			if tc.version {
				version = 1
			}
			either := 0
			if tc.cost || tc.version {
				either = 1
			}
			if err != nil || r.Cost.Total != total || r.AcceptedUnknownCost != cost || r.AcceptedUnknownVersion != version || r.AcceptedUnmeasured != either || r.MeasuredComplete != !tc.cost || r.Ready {
				t.Fatal("unknown cost/version counters were conflated", err, r)
			}
		})
	}
}

// The wrapper's entry/stream field tells whether invoke returned an EventStream.
// Only the separate request-local observer can prove the actual provider phase.
func TestV6WaiverObservedStreamAndWrapperFailureStageAreDistinct(t *testing.T) {
	for _, tc := range []struct {
		name  string
		stage any
		pass  bool
	}{
		{"synchronous_entry_error", "entry", true},
		{"missing_failure_stage", nil, false},
		{"unsupported_failure_stage", "authentication", false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			x := waivedStreamTimeout(t, 0, true, true)
			changeNative(&x, 0, func(n object) { n["failure_stage"] = tc.stage })
			r, err := audit(x)
			if (err == nil) != tc.pass || r.Ready {
				t.Fatal("entry return phase confused with actual stream observation", tc.name, err, r)
			}
			if tc.pass && (r.Cost.Unknown != 1 || r.AcceptedUnmeasured != 1 || r.MeasuredComplete) {
				t.Fatal("synchronous stream timeout lost original UNKNOWN", r)
			}
		})
	}
}

func TestV6WaiverCannotCoverCriticalFailuresOrUnboundUnknown(t *testing.T) {
	cases := map[string]func(*inputs){
		"unknown_success_cost": func(x *inputs) {
			changeNative(x, 1, func(n object) {
				n["actual_input_tokens"], n["actual_output_tokens"], n["actual_total_tokens"] = nil, nil, nil
			})
		},
		"unknown_success_version": func(x *inputs) { changeNative(x, 1, func(n object) { n["model_version"] = nil }) },
		"wrong_failed_version":    func(x *inputs) { changeNative(x, 0, func(n object) { n["model_version"] = "gemini-3.8-flash-other" }) },
		"second_timeout": func(x *inputs) {
			changeNative(x, 1, func(n object) {
				n["outcome"], n["failure_stage"], n["diagnostic_code"] = "timed_out", "stream", "provider_timeout"
				n["terminal_count"], n["timed_out_count"] = jsonNumber(1), jsonNumber(1)
			})
		},
		"no_retry_binding": func(x *inputs) {
			tamperProduct(x, 0, func(p object) { rows(obj(p["postgres"]), "accepted_inputs")[0]["model_retry"] = nil })
		},
		"retry_not_started": func(x *inputs) {
			tamperProduct(x, 0, func(p object) {
				obj(rows(obj(p["postgres"]), "accepted_inputs")[0]["model_retry"])["state"] = "claimed"
			})
		},
		"failed_kernel_observed_at_invented": func(x *inputs) {
			tamperProduct(x, 0, func(p object) {
				for _, m := range rows(obj(p["postgres"]), "model_budget_attempts") {
					if m["operation_id"] == "measured-failed-operation" {
						m["observed_at"] = m["created_at"]
					}
				}
			})
		},
		"failed_kernel_usage_estimated": func(x *inputs) {
			tamperProduct(x, 0, func(p object) {
				for _, m := range rows(obj(p["postgres"]), "model_budget_attempts") {
					if m["operation_id"] == "measured-failed-operation" {
						m["actual_input_tokens"] = jsonNumber(10)
					}
				}
			})
		},
		"source_revoked": func(x *inputs) {
			tamperProduct(x, 0, func(p object) {
				rows(obj(p["postgres"]), "catalog_sources")[0]["status"] = "revoked"
			})
		},
		"unverified_original": func(x *inputs) {
			tamperProduct(x, 0, func(p object) { obj(p["original_result"])["checks_passed"] = false })
		},
		"effect_unknown": func(x *inputs) {
			tamperProduct(x, 0, func(p object) { rows(obj(p["postgres"]), "actions")[0]["state"] = "effect_unknown" })
		},
		"delivery_resend": func(x *inputs) { tamperProduct(x, 3, func(p object) { p["all_channel_requests"] = jsonNumber(2) }) },
	}
	for _, stage := range []string{"authentication", "request_serialization", "http_round_trip", ""} {
		stage := stage
		cases["stage_"+stage] = func(x *inputs) { changeNative(x, 0, func(n object) { n["provider_operation_stage"] = stage }) }
	}
	for _, diagnostic := range []string{"provider_unavailable", "provider_protocol_error", "rate_limited", "memory_source_stale"} {
		diagnostic := diagnostic
		cases["failure_"+diagnostic] = func(x *inputs) { changeNative(x, 0, func(n object) { n["diagnostic_code"] = diagnostic }) }
	}
	for _, flag := range []string{"usage_observation_uncertain", "version_observation_uncertain", "finish_observation_uncertain"} {
		flag := flag
		cases["conflicting_"+flag] = func(x *inputs) { changeNative(x, 0, func(n object) { n[flag] = true }) }
	}
	for _, counter := range []string{"unknown_full_token_cost", "unknown_token_cost_entries", "unknown_model_version_entries", "accepted_unknown_timeout_cost_entries", "accepted_unknown_timeout_version_entries", "accepted_unmeasured_timeout_entries"} {
		counter := counter
		cases["counter_"+counter] = func(x *inputs) { obj(obj(x.Scope)["scope"])[counter] = jsonNumber(0) }
	}
	for _, field := range []string{"actual_input_tokens", "actual_output_tokens", "actual_total_tokens"} {
		field := field
		cases["partial_"+field] = func(x *inputs) { changeNative(x, 0, func(n object) { n[field] = jsonNumber(1) }) }
	}
	for name, mutate := range cases {
		t.Run(name, func(t *testing.T) {
			x := waivedStreamTimeout(t, 0, true, true)
			mutate(&x)
			r, err := audit(x)
			if err == nil || r.Status != "NO_GO" || r.Ready {
				t.Fatal("critical or unbound unknown accepted", r)
			}
		})
	}
}
