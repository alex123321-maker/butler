package main

import (
	"testing"
	"time"
)

func sqlRequest(n object) object {
	x := obj(clone(n))
	for from, to := range map[string]string{"input_sha256": "input_hash", "admitted_at_utc": "admitted_at", "finished_at_utc": "finished_at", "actual_input_tokens": "input_tokens", "actual_output_tokens": "output_tokens", "actual_total_tokens": "total_tokens"} {
		x[to] = x[from]
		delete(x, from)
	}
	return x
}
func syntheticMeasuredRetry(t *testing.T) inputs {
	return syntheticMeasuredRetryForScenario(t, 0)
}
func syntheticMeasuredRetryForScenario(t *testing.T, scenarioIndex int) inputs {
	t.Helper()
	x := syntheticWitness(t)
	scope := obj(obj(x.Scope)["scope"])
	native := arr(scope["requests"])
	name := scenarios[scenarioIndex]
	first := -1
	for j, item := range native {
		if obj(item)["scenario"] == name {
			first = j
			break
		}
	}
	require(first >= 0, "synthetic_retry_scenario")
	failed := obj(clone(native[first]))
	firstAt, firstFinish := moment(failed["admitted_at_utc"]), moment(failed["finished_at_utc"])
	var shift func(any)
	shift = func(v any) {
		switch a := v.(type) {
		case object:
			for k, child := range a {
				if s, ok := child.(string); ok {
					if stamp, err := time.Parse(time.RFC3339Nano, s); err == nil {
						a[k] = stamp.Add(3 * time.Second).Format(time.RFC3339Nano)
						continue
					}
				}
				shift(child)
			}
		case []any:
			for _, child := range a {
				shift(child)
			}
		}
	}
	for _, item := range native {
		if obj(item)["scenario"] == name {
			shift(item)
		}
	}
	product := obj(arr(x.Product)[scenarioIndex])
	pg := obj(obj(product["payload_json"])["postgres"])
	shift(pg)
	product["created_at"] = moment(product["created_at"]).Add(3 * time.Second).Format(time.RFC3339Nano)
	failed["kernel_operation_id"] = "measured-failed-operation"
	failed["outcome"], failed["failure_stage"], failed["diagnostic_code"] = "timed_out", "stream", "provider_timeout"
	failed["terminal_count"], failed["timed_out_count"] = jsonNumber(1), jsonNumber(1)
	fresh := obj(native[first])
	var input object
	for _, candidate := range rows(pg, "accepted_inputs") {
		if candidate["run_id"] == fresh["run_id"] {
			input = candidate
			break
		}
	}
	require(input != nil, "synthetic_retry_input")
	input["attempts"] = jsonNumber(2)
	if name == "owner_stop" {
		input["status"], input["error_code"] = "cancelled", "owner_control"
		for _, run := range rows(pg, "runs") {
			if run["run_id"] == fresh["run_id"] {
				run["status"], run["current_state"] = "cancelled", "cancelled"
			}
		}
	}
	claimed := firstAt.Add(5 * time.Millisecond)
	ready := claimed.Add(2 * time.Second)
	started := moment(fresh["admitted_at_utc"]).Add(-time.Millisecond)
	input["model_retry"] = object{"count": jsonNumber(1), "reason": "model_turn_timeout", "prior_operation_id": failed["kernel_operation_id"], "state": "started", "claimed_at": claimed.Format(time.RFC3339Nano), "not_before": ready.Format(time.RFC3339Nano), "retry_operation_id": fresh["kernel_operation_id"], "started_at": started.Format(time.RFC3339Nano)}
	newModel := obj(clone(arr(pg["model_budget_attempts"])[0]))
	newModel["operation_id"], newModel["reservation_id"] = "measured-failed-operation", "measured-failed-reservation"
	newModel["created_at"], newModel["observed_at"] = firstAt.Add(-time.Millisecond).Format(time.RFC3339Nano), firstFinish.Format(time.RFC3339Nano)
	pg["model_budget_attempts"] = append(arr(pg["model_budget_attempts"]), newModel)
	newReservation := obj(clone(arr(pg["budget_reservations"])[0]))
	newReservation["operation_id"], newReservation["reservation_id"] = "measured-failed-operation", "measured-failed-reservation"
	newReservation["created_at"], newReservation["fingerprint"] = newModel["created_at"], digest([]byte("measured-failed-reservation"))
	pg["budget_reservations"] = append(arr(pg["budget_reservations"]), newReservation)
	for _, dimension := range []string{"model_turns", "tokens", "active_millis"} {
		for _, id := range arr(newReservation["scope_ids"]) {
			for _, budgetScope := range rows(pg, "budget_scopes") {
				if budgetScope["scope_id"] == id {
					used := obj(budgetScope["reserved"])
					used[dimension] = jsonNumber(integer(used[dimension]) + integer(obj(newReservation["charge"])[dimension]))
				}
			}
		}
	}
	joined := append([]any{}, native[:first]...)
	joined = append(joined, failed)
	native = append(joined, native[first:]...)
	ledger := obj(x.Ledger)
	sql := []any{}
	for j, value := range native {
		n := obj(value)
		n["ordinal"] = jsonNumber(int64(538 + j))
		sql = append(sql, sqlRequest(n))
	}
	scope["requests"], ledger["requests"] = native, sql
	scope["failed_provider_entries"], scope["accepted_model_retry_failures"] = jsonNumber(1), jsonNumber(1)
	for _, container := range []object{scope, ledger} {
		container["total_attempts"], container["new_attempts"] = jsonNumber(537+int64(len(native))), jsonNumber(int64(len(native)))
		obj(container["window"])["attempts"] = jsonNumber(537 + int64(len(native)))
	}
	product["payload_sha256"] = hashValue(product["payload_json"])
	return x
}

func TestV6MeasuredTimeoutRetryIsExplicitFreshAndPreservesAllCosts(t *testing.T) {
	x := syntheticMeasuredRetry(t)
	r, e := audit(x)
	if e != nil || r.Status != "CHAIN_V6_WITNESS_AUDIT_PASS" || r.AcceptedRetryFailures != 1 || r.Cost.Attempts != 71 || r.Cost.Total != 1065 || r.UnknownHistoricalTotal != 47 || r.Ready {
		t.Fatal("measured retry failed immutable predeclared contract", e, r.Reasons)
	}
}

func TestV6MeasuredRetryThenOwnerStopRetainsStrictStoppedTree(t *testing.T) {
	x := syntheticMeasuredRetryForScenario(t, 4)
	r, e := audit(x)
	if e != nil || r.Status != "CHAIN_V6_WITNESS_AUDIT_PASS" || r.AcceptedRetryFailures != 1 || r.Cost.Attempts != 71 || r.Cost.Total != 1065 || r.Ready {
		t.Fatal("known retry then actual owner Stop rejected", e, r.Reasons)
	}
}

func TestV6StoppedRetryCannotLaunderEffectsOrOwnerControl(t *testing.T) {
	cases := map[string]func(*inputs){
		"wrong_input_error": func(x *inputs) {
			tamperProduct(x, 4, func(p object) { rows(obj(p["postgres"]), "accepted_inputs")[0]["error_code"] = "execution_stopped" })
		},
		"completed_instead_of_cancelled": func(x *inputs) {
			tamperProduct(x, 4, func(p object) { rows(obj(p["postgres"]), "accepted_inputs")[0]["status"] = "completed" })
		},
		"live_retry_run": func(x *inputs) {
			tamperProduct(x, 4, func(p object) { rows(obj(p["postgres"]), "runs")[0]["current_state"] = "model_running" })
		},
		"completed_retry_run": func(x *inputs) {
			tamperProduct(x, 4, func(p object) { rows(obj(p["postgres"]), "runs")[0]["status"] = "completed" })
		},
		"new_child_effect": func(x *inputs) {
			tamperProduct(x, 4, func(p object) { pg := obj(p["postgres"]); rows(pg, "actions")[0]["run_id"] = "child-run-owner_stop" })
		},
		"new_notice": func(x *inputs) { tamperProduct(x, 4, func(p object) { p["recipient_requests"] = jsonNumber(1) }) },
		"source_guard_missing": func(x *inputs) {
			tamperProduct(x, 4, func(p object) { obj(p["fault"])["stopped_child_source_denied"] = false })
		},
		"broker_guard_missing": func(x *inputs) {
			tamperProduct(x, 4, func(p object) { obj(p["fault"])["stopped_child_broker_prepare_denied"] = false })
		},
	}
	for name, change := range cases {
		t.Run(name, func(t *testing.T) {
			x := syntheticMeasuredRetryForScenario(t, 4)
			change(&x)
			r, e := audit(x)
			if e == nil || r.Status != "NO_GO" || r.Ready {
				t.Fatal("stopped retry safety changed", r.Reasons)
			}
		})
	}
}

func TestV6MeasuredRetryRejectsCoherentTampering(t *testing.T) {
	cases := map[string]func(*inputs){
		"retry_not_fresh_start": func(x *inputs) {
			obj(arr(obj(obj(x.Scope)["scope"])["requests"])[1])["kind"] = "submit"
			obj(arr(obj(x.Ledger)["requests"])[1])["kind"] = "submit"
		},
		"third_input_claim": func(x *inputs) {
			tamperProduct(x, 0, func(p object) { rows(obj(p["postgres"]), "accepted_inputs")[0]["attempts"] = jsonNumber(3) })
		},
		"successful_retry_keeps_error": func(x *inputs) {
			tamperProduct(x, 0, func(p object) { rows(obj(p["postgres"]), "accepted_inputs")[0]["error_code"] = "execution_stopped" })
		},
		"input_expired_before_retry": func(x *inputs) {
			tamperProduct(x, 0, func(p object) {
				input := rows(obj(p["postgres"]), "accepted_inputs")[0]
				input["expires_at"] = obj(input["model_retry"])["claimed_at"]
			})
		},
		"unknown_kernel_measurement": func(x *inputs) {
			tamperProduct(x, 0, func(p object) {
				a := rows(obj(p["postgres"]), "model_budget_attempts")
				a[len(a)-1]["actual_input_tokens"] = nil
				a[len(a)-1]["actual_output_tokens"] = nil
			})
		},
		"premature_reservation": func(x *inputs) {
			tamperProduct(x, 0, func(p object) {
				pg := obj(p["postgres"])
				input := rows(pg, "accepted_inputs")[0]
				rows(pg, "model_budget_attempts")[0]["created_at"] = obj(input["model_retry"])["claimed_at"]
			})
		},
		"unbound_failed_operation": func(x *inputs) {
			tamperProduct(x, 0, func(p object) {
				rows(obj(p["postgres"]), "accepted_inputs")[0]["model_retry"] = nil
				rows(obj(p["postgres"]), "accepted_inputs")[0]["attempts"] = jsonNumber(1)
			})
		},
		"new_unknown_expense": func(x *inputs) {
			obj(arr(obj(obj(x.Scope)["scope"])["requests"])[0])["actual_total_tokens"] = nil
			obj(arr(obj(x.Ledger)["requests"])[0])["total_tokens"] = nil
		},
		"unknown_input": func(x *inputs) {
			obj(arr(obj(obj(x.Scope)["scope"])["requests"])[0])["actual_input_tokens"] = nil
			obj(arr(obj(x.Ledger)["requests"])[0])["input_tokens"] = nil
		},
		"failure_count_laundered": func(x *inputs) { obj(obj(x.Scope)["scope"])["failed_provider_entries"] = jsonNumber(0) },
		"retry_count_laundered":   func(x *inputs) { obj(obj(x.Scope)["scope"])["accepted_model_retry_failures"] = jsonNumber(0) },
		"different_kernel_operation": func(x *inputs) {
			obj(arr(obj(obj(x.Scope)["scope"])["requests"])[1])["kernel_operation_id"] = "other"
			obj(arr(obj(x.Ledger)["requests"])[1])["kernel_operation_id"] = "other"
		},
		"reused_kernel_operation": func(x *inputs) {
			a := obj(arr(obj(obj(x.Scope)["scope"])["requests"])[0])
			b := obj(arr(obj(obj(x.Scope)["scope"])["requests"])[1])
			b["kernel_operation_id"] = a["kernel_operation_id"]
			obj(arr(obj(x.Ledger)["requests"])[1])["kernel_operation_id"] = a["kernel_operation_id"]
		},
	}
	for _, field := range []string{"reason", "state", "count", "prior_operation_id", "retry_operation_id", "claimed_at", "not_before", "started_at"} {
		field := field
		cases["durable_"+field] = func(x *inputs) {
			tamperProduct(x, 0, func(p object) {
				r := obj(rows(obj(p["postgres"]), "accepted_inputs")[0]["model_retry"])
				switch field {
				case "count":
					r[field] = jsonNumber(2)
				case "reason":
					r[field] = "provider_retryable"
				case "state":
					r[field] = "claimed"
				case "prior_operation_id", "retry_operation_id":
					r[field] = "unbound"
				case "claimed_at":
					r[field] = moment(r[field]).Add(time.Millisecond).Format(time.RFC3339Nano)
				case "not_before":
					r[field] = moment(r[field]).Add(-time.Millisecond).Format(time.RFC3339Nano)
				case "started_at":
					r[field] = moment(r["not_before"]).Add(-time.Millisecond).Format(time.RFC3339Nano)
				}
			})
		}
	}
	for _, field := range []string{"diagnostic_code", "provider_operation_stage", "model_version", "outcome", "final_count", "completed_count", "cancelled_count", "finish_reason"} {
		field := field
		cases["failed_"+field] = func(x *inputs) {
			a := obj(arr(obj(obj(x.Scope)["scope"])["requests"])[0])
			b := obj(arr(obj(x.Ledger)["requests"])[0])
			var value any = "invented"
			switch field {
			case "final_count", "completed_count", "cancelled_count":
				value = jsonNumber(1)
			}
			a[field], b[field] = value, value
		}
	}
	for name, change := range cases {
		t.Run(name, func(t *testing.T) {
			x := syntheticMeasuredRetry(t)
			change(&x)
			r, e := audit(x)
			if e == nil || r.Status != "NO_GO" || r.Ready {
				t.Fatal("retry tamper passed", r.Reasons)
			}
		})
	}
}
