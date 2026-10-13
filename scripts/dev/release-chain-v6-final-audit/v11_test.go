package main

import (
	"encoding/json"
	"strings"
	"testing"
	"time"
)

func upgradeV11Witness(t *testing.T, x inputs) (inputs, auditPolicy, map[int64][]byte) {
	t.Helper()
	x, _, oldPrivate := upgradeV10Witness(t, x)
	p := v11AuditPolicy()
	scope, ledger := obj(x.Scope), obj(x.Ledger)
	scope["window_id"], scope["baseline_attempts"], scope["maximum_total_attempts"] = p.WindowID, jsonNumber(557), jsonNumber(628)
	ledger["baseline_attempts"] = jsonNumber(557)
	for _, w := range []object{obj(obj(scope["scope"])["window"]), obj(ledger["window"])} {
		w["native_baseline"], w["attempts"] = jsonNumber(557), jsonNumber(integer(w["attempts"])+1)
		w["prior_history_hash"], w["prerequisite_hash"] = p.PriorSHA, p.PrerequisiteSHA
	}
	obj(scope["scope"])["total_attempts"] = jsonNumber(integer(obj(scope["scope"])["total_attempts"]) + 1)
	ledger["total_attempts"] = jsonNumber(integer(ledger["total_attempts"]) + 1)
	for _, list := range []any{obj(scope["scope"])["requests"], ledger["requests"]} {
		for _, value := range arr(list) {
			r := obj(value)
			r["ordinal"] = jsonNumber(integer(r["ordinal"]) + 1)
		}
	}
	private := map[int64][]byte{}
	for _, value := range arr(scope["proposal_observations"]) {
		m := obj(value)
		var record privateProposal
		if json.Unmarshal(oldPrivate[integer(m["ordinal"])], &record) != nil {
			t.Fatal("synthetic private record invalid")
		}
		record.Ordinal++
		raw, err := json.Marshal(record)
		if err != nil {
			t.Fatal(err)
		}
		private[record.Ordinal] = raw
		m["ordinal"], m["private_proposal_sha256"], m["private_proposal_bytes"] = jsonNumber(record.Ordinal), digest(raw), jsonNumber(int64(len(raw)))
	}
	for i := range arr(x.Product) {
		tamperProduct(&x, i, func(fin object) {
			fin["source_check_observation"] = object{"version": "butler-source-check-observation-v1", "failed_checks": []any{}, "dropped_failed_checks": jsonNumber(0)}
		})
	}
	x.Prerequisite, x.SafeTimeoutWaiver = frozenObject(v11PrerequisiteSemantic), frozenObject(v11HumanSemantic)
	x = normalizedInputs(x)
	p.ReadPrivateProposal = func(m object) []byte { return private[integer(m["ordinal"])] }
	return x, p, private
}

func TestV11AuditFiveCasesSourceProjectionAnd628(t *testing.T) {
	x, p, _ := upgradeV11Witness(t, syntheticWitness(t))
	r, err := auditWithPolicy(x, p)
	if err != nil || r.Status != "CHAIN_V11_WITNESS_AUDIT_PASS" || r.Prior != 557 || r.Total != 627 || r.KnownHistoricalTotal != 793703 || r.UnknownHistoricalTotal != 49 || len(r.Cases) != 5 || r.Ready || r.AuditorCalls != 0 {
		t.Fatal("V11 five cases rejected/overclaimed", err, r.Reasons)
	}
	if selectedAuditPolicy("v11").Baseline+selectedAuditPolicy("v11").Maximum != 628 || v10AuditPolicy().Baseline+v10AuditPolicy().Maximum != 627 || v9AuditPolicy().Baseline+v9AuditPolicy().Maximum != 615 {
		t.Fatal("old allocation changed")
	}
}

func TestV11AuditNominalRecoveryRetainsAllUnknownsAt628(t *testing.T) {
	x, p, _ := upgradeV11Witness(t, waivedStreamTimeout(t, 0, true, true))
	r, err := auditWithPolicy(x, p)
	if err != nil || r.Status != "CHAIN_V11_WITNESS_AUDIT_PASS" || r.Total != 628 || r.Cost.Unknown != 1 || r.UnknownHistoricalTotal != 49 || r.AcceptedRetryFailures != 1 || r.AcceptedUnknownCost != 1 || r.AcceptedUnknownVersion != 1 || r.MeasuredComplete || r.Ready {
		t.Fatal("frozen nominal recovery changed or old source uncertainty reclassified", err, r.Reasons)
	}
}

func v11DiagnosticRecord(pg object) object {
	run := rows(pg, "runs")[0]
	var op any
	for _, value := range rows(pg, "model_budget_attempts") {
		if value["run_id"] == run["run_id"] {
			op = value["operation_id"]
			break
		}
	}
	stats := object{"max_conns": 2, "total_conns": 2, "acquired_conns": 2, "idle_conns": 0, "empty_acquire_count": 1, "cancelled_acquire_count": 1, "acquire_duration_ms": 100}
	start := time.Date(2026, 10, 13, 20, 0, 0, 0, time.UTC)
	return obj(clone(object{"role": "source_check", "run_id": run["run_id"], "kernel_operation_id": op, "started_at_utc": start.Format(time.RFC3339Nano), "finished_at_utc": start.Add(80 * time.Millisecond).Format(time.RFC3339Nano), "elapsed_ms": 80, "maximum_check_ms": 2000, "context_time_left_ms_at_start": -1, "phase": "pool_acquire", "query_family": nil, "query_template_sha256": nil, "failure_category": "cancelled", "sqlstate": nil, "acquisition_count": 1, "acquisition_elapsed_ms": 80, "pool_before": stats, "pool_after": stats}))
}

func TestV11AuditTypedDiagnosticsDoNotChangeStopGuarantees(t *testing.T) {
	x, p, _ := upgradeV11Witness(t, syntheticWitness(t))
	tamperProduct(&x, 2, func(fin object) {
		pg := obj(fin["postgres"])
		ob := obj(fin["source_check_observation"])
		ob["failed_checks"] = []any{v11DiagnosticRecord(pg)}
	})
	r, err := auditWithPolicy(x, p)
	if err != nil || r.Status != "CHAIN_V11_WITNESS_AUDIT_PASS" {
		t.Fatal("typed owner-Stop data changed current predicates", err, r.Reasons)
	}
}

func TestV11AuditBoundedDiagnosticDropIsFactualData(t *testing.T) {
	x, p, _ := upgradeV11Witness(t, syntheticWitness(t))
	tamperProduct(&x, 2, func(fin object) {
		pg := obj(fin["postgres"])
		ob := obj(fin["source_check_observation"])
		records := []any{}
		for i := 0; i < 64; i++ {
			records = append(records, v11DiagnosticRecord(pg))
		}
		ob["failed_checks"], ob["dropped_failed_checks"] = records, jsonNumber(1)
	})
	r, err := auditWithPolicy(x, p)
	if err != nil || r.Status != "CHAIN_V11_WITNESS_AUDIT_PASS" {
		t.Fatal("bounded diagnostic loss became an invented source gate", err, r.Reasons)
	}
}

func TestV11AuditObserverPrivacyAndCriticalTampering(t *testing.T) {
	for _, kind := range []string{"private_sql_field", "unknown_role", "unknown_phase", "larger_source_bound", "arbitrary_sqlstate", "overflow64", "dropped_records", "wrong_operation", "negative_pool", "missing_observer", "successful_usage_missing", "source_uncertain_retry"} {
		t.Run(kind, func(t *testing.T) {
			base := syntheticWitness(t)
			if kind == "source_uncertain_retry" {
				base = waivedStreamTimeout(t, 0, true, true)
			}
			x, p, _ := upgradeV11Witness(t, base)
			tamperProduct(&x, 2, func(fin object) {
				pg := obj(fin["postgres"])
				ob := obj(fin["source_check_observation"])
				rec := v11DiagnosticRecord(pg)
				ob["failed_checks"] = []any{rec}
				switch kind {
				case "private_sql_field":
					rec["sql"] = "PRIVATE_VALUE"
				case "unknown_role":
					rec["role"] = "PRIVATE_VALUE"
				case "unknown_phase":
					rec["phase"] = "fallback_authorized"
				case "larger_source_bound":
					rec["maximum_check_ms"] = jsonNumber(2001)
				case "arbitrary_sqlstate":
					rec["sqlstate"] = "PRIVATE_VALUE"
				case "overflow64":
					values := []any{}
					for i := 0; i < 65; i++ {
						values = append(values, clone(rec))
					}
					ob["failed_checks"] = values
				case "dropped_records":
					ob["dropped_failed_checks"] = jsonNumber(1)
				case "wrong_operation":
					rec["kernel_operation_id"] = "foreign-operation"
				case "negative_pool":
					obj(rec["pool_after"])["acquired_conns"] = jsonNumber(-1)
				case "missing_observer":
					delete(fin, "source_check_observation")
				}
			})
			if kind == "successful_usage_missing" {
				changeNative(&x, 0, func(n object) {
					n["actual_input_tokens"], n["actual_output_tokens"], n["actual_total_tokens"] = nil, nil, nil
				})
			}
			if kind == "source_uncertain_retry" {
				changeNative(&x, 0, func(n object) { n["diagnostic_code"] = "memory_source_check_timeout" })
			}
			r, err := auditWithPolicy(x, p)
			if err == nil || r.Status != "NO_GO" || r.Ready || r.AcceptedRetryFailures != 0 || r.AcceptedUnknownCost != 0 || r.AcceptedUnknownVersion != 0 || r.AcceptedUnmeasured != 0 {
				t.Fatal("V11 tamper accepted", kind, err, r.Reasons)
			}
			if strings.Contains(strings.Join(r.Reasons, ","), "PRIVATE_VALUE") {
				t.Fatal("private diagnostic echoed")
			}
		})
	}
}

func TestV11AuditRetained557AndNarrowHistoricalException(t *testing.T) {
	verifyRetainedFailedV10Source(candidateRepo(t))
}

func TestV11AuditHistorical557CannotBecomeRetryOrRefund(t *testing.T) {
	for _, kind := range []string{"exception_applies_future", "exception_reclassifies_source", "old_usage_filled", "old_nominal_retry", "refunded_old627", "old_report_pass"} {
		t.Run(kind, func(t *testing.T) {
			all := readRetainedFailedV10(candidateRepo(t))
			switch kind {
			case "exception_applies_future":
				obj(all["historical-exception"])["applies_to_future_attempts"] = true
			case "exception_reclassifies_source":
				obj(all["historical-exception"])["nominal_provider_timeout_established"] = true
			case "old_usage_filled":
				obj(arr(obj(all["history"])["usage"])[556])["actual_input_tokens"] = jsonNumber(0)
			case "old_nominal_retry":
				obj(obj(all["failure-correlation"])["failure_classification"])["safe_model_retry_authorized_or_observed"] = true
			case "refunded_old627":
				obj(all["bridge"])["remaining_original_absolute627_attempts"] = jsonNumber(71)
			case "old_report_pass":
				obj(obj(all["final-audit"])["chain_audit"])["status"] = "CHAIN_V10_WITNESS_AUDIT_PASS"
			}
			defer func() {
				if recover() == nil {
					t.Fatal("old557 tamper accepted")
				}
			}()
			verifyRetainedFailedV10(all)
		})
	}
}
