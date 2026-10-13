package main

import (
	"testing"
	"time"
)

func upgradeV7Witness(t *testing.T, x inputs) inputs {
	t.Helper()
	x = normalizedInputs(x)
	p := v7AuditPolicy()
	scope, ledger := obj(x.Scope), obj(x.Ledger)
	scope["window_id"], scope["baseline_attempts"], scope["maximum_new_attempts"] = p.WindowID, jsonNumber(539), jsonNumber(76)
	ledger["baseline_attempts"] = jsonNumber(539)
	for _, w := range []object{obj(obj(scope["scope"])["window"]), obj(ledger["window"])} {
		w["native_baseline"], w["attempts"], w["maximum_new_attempts"] = jsonNumber(539), jsonNumber(integer(w["attempts"])+2), jsonNumber(76)
		w["prior_history_hash"], w["prerequisite_hash"], w["protocol_policy"] = p.PriorSHA, p.PrerequisiteSHA, clone(p.WindowPolicy)
	}
	obj(scope["scope"])["total_attempts"] = jsonNumber(integer(obj(scope["scope"])["total_attempts"]) + 2)
	ledger["total_attempts"] = jsonNumber(integer(ledger["total_attempts"]) + 2)
	for _, list := range []any{obj(scope["scope"])["requests"], ledger["requests"]} {
		for _, row := range arr(list) {
			n := obj(row)
			n["ordinal"] = jsonNumber(integer(n["ordinal"]) + 2)
		}
	}
	for j := range arr(x.Product) {
		tamperProduct(&x, j, func(fin object) {
			pg := obj(fin["postgres"])
			for _, res := range rows(pg, "budget_reservations") {
				charge := obj(res["charge"])
				if integer(charge["model_turns"]) > 0 {
					charge["active_millis"] = jsonNumber(300000)
				}
			}
			for _, s := range rows(pg, "budget_scopes") {
				limit := obj(s["limits"])
				switch s["kind"] {
				case "task":
					limit["active_millis"] = jsonNumber(24 * 300000)
				case "owner_day":
					limit["model_turns"], limit["tokens"], limit["active_millis"] = jsonNumber(76), jsonNumber(76*1081344), jsonNumber(76*300000)
				case "delegation":
					limit["active_millis"] = jsonNumber(1320000)
				}
				used := obj(s["reserved"])
				used["active_millis"] = jsonNumber(0)
				for _, res := range rows(pg, "budget_reservations") {
					for _, id := range arr(res["scope_ids"]) {
						if id == s["scope_id"] {
							used["active_millis"] = jsonNumber(integer(used["active_millis"]) + integer(obj(res["charge"])["active_millis"]))
						}
					}
				}
			}
		})
	}
	x.Prerequisite = frozenObject(v7PrerequisiteSemantic)
	x.SafeTimeoutWaiver = frozenObject(v7HumanSemantic)
	return x
}

func TestV7AuditDefaultV6Compatibility(t *testing.T) {
	r, e := audit(syntheticWitness(t))
	if e != nil || r.Status != "CHAIN_V6_WITNESS_AUDIT_PASS" || r.Prior != 537 || r.Ready {
		t.Fatal("default prior protocol changed", e, r.Reasons)
	}
}
func TestV7AuditFiveCasesExplicitProfile(t *testing.T) {
	r, e := auditWithPolicy(upgradeV7Witness(t, syntheticWitness(t)), v7AuditPolicy())
	if e != nil || r.Status != "CHAIN_V7_WITNESS_AUDIT_PASS" || len(r.Cases) != 5 || r.Prior != 539 || r.Total != 609 || r.UnknownHistoricalTotal != 48 || r.Ready || r.AuditorCalls != 0 {
		t.Fatal("five-case profile rejected or release overclaimed", e, r.Reasons)
	}
}
func TestV7AuditProvedHTTPUnknownRetry(t *testing.T) {
	for _, index := range []int{0, 4} {
		x := upgradeV7Witness(t, waivedStreamTimeout(t, index, true, true))
		for j, v := range arr(obj(obj(x.Scope)["scope"])["requests"]) {
			if obj(v)["outcome"] != "completed" {
				changeNative(&x, j, func(n object) { n["provider_operation_stage"] = "http_round_trip" })
				break
			}
		}
		r, e := auditWithPolicy(x, v7AuditPolicy())
		if e != nil || r.AcceptedRetryFailures != 1 || r.AcceptedUnknownCost != 1 || r.Cost.Unknown != 1 || r.MeasuredComplete || r.Ready {
			t.Fatal("safe HTTP retry or stopped-tree policy", index, e, r.Reasons)
		}
	}
}
func TestV7AuditCurrentProtocolAndHumanTampering(t *testing.T) {
	for _, field := range []string{"baseline_attempts", "maximum_new_attempts", "maximum_attempt_ms", "human_policy_amendment_sha256", "all_completed_entries_require_measured_usage_and_selected_version", "all_other_quality_source_authority_stop_effect_delivery_gates_retained"} {
		t.Run(field, func(t *testing.T) {
			x := upgradeV7Witness(t, syntheticWitness(t))
			obj(x.Prerequisite)[field] = nil
			r, e := auditWithPolicy(x, v7AuditPolicy())
			if e == nil || r.Status != "NO_GO" {
				t.Fatal("changed prerequisite accepted")
			}
		})
	}
	for _, field := range []string{"user_messages", "maximum_child_active_ms", "critical_safety_or_original_result_criteria_changed", "keep_closed_537_and_539_unknown"} {
		t.Run("human_"+field, func(t *testing.T) {
			x := upgradeV7Witness(t, syntheticWitness(t))
			obj(x.SafeTimeoutWaiver)[field] = nil
			_, e := auditWithPolicy(x, v7AuditPolicy())
			if e == nil {
				t.Fatal("changed human scope accepted")
			}
		})
	}
}
func TestV7AuditCriticalBoundariesRemainClosed(t *testing.T) {
	cases := map[string]func(*inputs){
		"successful_HTTP": func(x *inputs) {
			changeNative(x, 0, func(n object) { n["provider_operation_stage"] = "http_round_trip" })
		},
		"successful_unknown": func(x *inputs) { changeNative(x, 0, func(n object) { n["actual_total_tokens"] = nil }) },
		"window_policy_downgrade": func(x *inputs) {
			for _, w := range []object{obj(obj(obj(x.Scope)["scope"])["window"]), obj(obj(x.Ledger)["window"])} {
				obj(w["protocol_policy"])["maximum_attempt_ms"] = jsonNumber(120000)
			}
		},
		"old_native_baseline": func(x *inputs) { obj(x.Scope)["baseline_attempts"] = jsonNumber(537) },
		"five_cases_missing":  func(x *inputs) { obj(obj(x.Scope)["scenarios"])["lost_ack"] = false },
		"old_model_quote": func(x *inputs) {
			tamperProduct(x, 0, func(p object) {
				obj(rows(obj(p["postgres"]), "budget_reservations")[0]["charge"])["active_millis"] = jsonNumber(120000)
			})
		},
		"child_no_tool_allowance": func(x *inputs) {
			tamperProduct(x, 0, func(p object) {
				for _, s := range rows(obj(p["postgres"]), "budget_scopes") {
					if s["kind"] == "delegation" {
						obj(s["limits"])["active_millis"] = jsonNumber(1200000)
					}
				}
			})
		},
		"duplicate_effect": func(x *inputs) {
			tamperProduct(x, 0, func(p object) {
				pg := obj(p["postgres"])
				pg["actions"] = append(arr(pg["actions"]), clone(arr(pg["actions"])[0]))
			})
		},
	}
	for name, change := range cases {
		t.Run(name, func(t *testing.T) {
			x := upgradeV7Witness(t, syntheticWitness(t))
			change(&x)
			r, e := auditWithPolicy(x, v7AuditPolicy())
			if e == nil || r.Status != "NO_GO" || r.Ready {
				t.Fatal("critical boundary accepted")
			}
		})
	}
	for _, stage := range []string{"authentication", "serialization", "", "unknown"} {
		t.Run("failed_stage_"+stage, func(t *testing.T) {
			x := upgradeV7Witness(t, waivedStreamTimeout(t, 0, true, true))
			for j, v := range arr(obj(obj(x.Scope)["scope"])["requests"]) {
				if obj(v)["outcome"] != "completed" {
					changeNative(&x, j, func(n object) { n["provider_operation_stage"] = stage })
					break
				}
			}
			_, e := auditWithPolicy(x, v7AuditPolicy())
			if e == nil {
				t.Fatal("unsupported timeout phase accepted")
			}
		})
	}
}
func TestV7AuditRetainedFailureAndPaidHistory(t *testing.T) {
	verifyRetainedFailedV5Source(candidateRepo(t))
	verifyRetainedFailedV6Source(candidateRepo(t))
	if selectedAuditPolicy("v6").Baseline != 537 || selectedAuditPolicy("v7").Baseline != 539 {
		t.Fatal("policy selection")
	}
	defer func() {
		if recover() == nil {
			t.Fatal("arbitrary protocol accepted")
		}
	}()
	selectedAuditPolicy("synthetic_rebase")
}

func TestV7AuditLongerMeasuredEntryDeadline(t *testing.T) {
	for _, milliseconds := range []int64{150000, 300001} {
		x := upgradeV7Witness(t, syntheticWitness(t))
		n := obj(arr(obj(obj(x.Scope)["scope"])["requests"])[0])
		oldFinish := moment(n["finished_at_utc"])
		delta := time.Duration(milliseconds-10) * time.Millisecond
		var shift func(any)
		shift = func(v any) {
			switch a := v.(type) {
			case object:
				for k, c := range a {
					if s, ok := c.(string); ok {
						if stamp, e := time.Parse(time.RFC3339Nano, s); e == nil && !stamp.Before(oldFinish) {
							a[k] = stamp.Add(delta).Format(time.RFC3339Nano)
							continue
						}
					}
					shift(c)
				}
			case []any:
				for _, c := range a {
					shift(c)
				}
			}
		}
		for _, v := range []any{x.Scope, x.Ledger, x.Initial, x.Product, x.Runtime} {
			shift(v)
		}
		n["elapsed_ms"] = jsonNumber(milliseconds)
		obj(arr(obj(x.Ledger)["requests"])[0])["elapsed_ms"] = jsonNumber(milliseconds)
		for _, list := range []any{x.Initial, x.Product} {
			for _, v := range arr(list) {
				w := obj(v)
				w["payload_sha256"] = hashValue(w["payload_json"])
			}
		}
		r, e := auditWithPolicy(x, v7AuditPolicy())
		if milliseconds == 150000 && e != nil {
			t.Fatal("legitimate measured thinking over old120seconds rejected", e, r.Reasons)
		}
		if milliseconds == 300001 && (e == nil || len(r.Reasons) != 1 || r.Reasons[0] != "request_deadline") {
			t.Fatal("new bounded deadline not enforced", e, r.Reasons)
		}
	}
}
