package main

import (
	"encoding/json"
	"testing"
)

func upgradeV10Witness(t *testing.T, x inputs) (inputs, auditPolicy, map[int64][]byte) {
	t.Helper()
	x, _, oldPrivate := upgradeV9Witness(t, x)
	p := v10AuditPolicy()
	scope, ledger := obj(x.Scope), obj(x.Ledger)
	scope["window_id"], scope["baseline_attempts"], scope["maximum_total_attempts"] = p.WindowID, jsonNumber(556), jsonNumber(627)
	ledger["baseline_attempts"] = jsonNumber(556)
	for _, w := range []object{obj(obj(scope["scope"])["window"]), obj(ledger["window"])} {
		w["native_baseline"], w["attempts"] = jsonNumber(556), jsonNumber(integer(w["attempts"])+12)
		w["prior_history_hash"], w["prerequisite_hash"], w["protocol_policy"] = p.PriorSHA, p.PrerequisiteSHA, clone(p.WindowPolicy)
	}
	obj(scope["scope"])["total_attempts"] = jsonNumber(integer(obj(scope["scope"])["total_attempts"]) + 12)
	ledger["total_attempts"] = jsonNumber(integer(ledger["total_attempts"]) + 12)
	for _, list := range []any{obj(scope["scope"])["requests"], ledger["requests"]} {
		for _, value := range arr(list) {
			r := obj(value)
			r["ordinal"] = jsonNumber(integer(r["ordinal"]) + 12)
		}
	}
	private := map[int64][]byte{}
	for _, value := range arr(scope["proposal_observations"]) {
		m := obj(value)
		var record privateProposal
		if json.Unmarshal(oldPrivate[integer(m["ordinal"])], &record) != nil {
			t.Fatal("synthetic private record invalid")
		}
		record.Ordinal += 12
		raw, err := json.Marshal(record)
		if err != nil {
			t.Fatal(err)
		}
		private[record.Ordinal] = raw
		m["ordinal"], m["private_proposal_sha256"], m["private_proposal_bytes"] = jsonNumber(record.Ordinal), digest(raw), jsonNumber(int64(len(raw)))
	}
	x.Prerequisite, x.SafeTimeoutWaiver = frozenObject(v10PrerequisiteSemantic), frozenObject(v10HumanSemantic)
	x = normalizedInputs(x)
	p.ReadPrivateProposal = func(m object) []byte { return private[integer(m["ordinal"])] }
	return x, p, private
}

func TestV10AuditFiveCasesExplicitProspectiveAllocation(t *testing.T) {
	x, p, _ := upgradeV10Witness(t, syntheticWitness(t))
	r, err := auditWithPolicy(x, p)
	if err != nil || r.Status != "CHAIN_V10_WITNESS_AUDIT_PASS" || r.Prior != 556 || r.Total != 626 || r.KnownHistoricalTotal != 793703 || r.UnknownHistoricalTotal != 48 || len(r.Cases) != 5 || r.Ready || r.AuditorCalls != 0 {
		t.Fatal("five-case V10 rejected or overclaimed", err, r.Reasons)
	}
	if len(p.WindowPolicy) != 8 || p.Baseline+p.Maximum != 627 || p.DayModelTurns != 71 {
		t.Fatal("V10 implicit resource expansion")
	}
	selected := selectedAuditPolicy("v10")
	if selected.PrerequisiteSHA != p.PrerequisiteSHA || selected.HumanSHA != p.HumanSHA || selected.Baseline+selected.Maximum != 627 {
		t.Fatal("explicit V10 CLI policy selection changed")
	}
	for _, name := range []string{"v6", "v7", "v8", "v9"} {
		old := selectedAuditPolicy(name)
		if old.Baseline+old.Maximum != 615 {
			t.Fatal("old ceiling changed")
		}
	}
}

func TestV10AuditRecoveredTimeoutAtExact627Ceiling(t *testing.T) {
	x, p, _ := upgradeV10Witness(t, waivedStreamTimeout(t, 0, true, true))
	r, err := auditWithPolicy(x, p)
	if err != nil || r.Status != "CHAIN_V10_WITNESS_AUDIT_PASS" || r.Total != 627 || r.Cost.Unknown != 1 || r.AcceptedRetryFailures != 1 || r.AcceptedUnknownCost != 1 || r.AcceptedUnknownVersion != 1 || r.MeasuredComplete || r.Ready {
		t.Fatal("exact-limit recovered timeout not correlated", err, r.Reasons)
	}
}

func TestV10AuditCriticalAndProtocolTampering(t *testing.T) {
	for _, kind := range []string{"old_ceiling_reused", "expanded_ceiling", "changed_lifetime", "missing_case_clamp", "old_attempt_refunded", "private_proposal_changed", "unknown_retry_private_changed", "original_result_changed", "serialization_removed"} {
		t.Run(kind, func(t *testing.T) {
			base := syntheticWitness(t)
			if kind == "unknown_retry_private_changed" {
				base = waivedStreamTimeout(t, 0, true, true)
			}
			x, p, private := upgradeV10Witness(t, base)
			switch kind {
			case "old_ceiling_reused":
				obj(x.Scope)["maximum_total_attempts"] = jsonNumber(615)
			case "expanded_ceiling":
				obj(x.Scope)["maximum_total_attempts"] = jsonNumber(628)
			case "changed_lifetime":
				obj(x.Prerequisite)["maximum_extension_harness_lifetime_ms"] = jsonNumber(2400001)
			case "missing_case_clamp":
				obj(x.SafeTimeoutWaiver)["extension_harness_lifetime_source"] = "unbounded"
			case "old_attempt_refunded":
				obj(obj(obj(x.Scope)["scope"])["window"])["native_baseline"] = jsonNumber(544)
			case "private_proposal_changed", "unknown_retry_private_changed":
				for ordinal, raw := range private {
					private[ordinal] = append(raw, byte(' '))
					break
				}
			case "original_result_changed":
				tamperProduct(&x, 0, func(fin object) { obj(fin["original_result"])["checks_passed"] = false })
			case "serialization_removed":
				delete(obj(obj(obj(obj(x.Scope)["scope"])["window"])["protocol_policy"]), "provider_entries_serialized")
			}
			r, err := auditWithPolicy(x, p)
			if err == nil || r.Status != "NO_GO" || r.Ready || r.AcceptedRetryFailures != 0 || r.AcceptedUnknownCost != 0 || r.AcceptedUnknownVersion != 0 || r.AcceptedUnmeasured != 0 {
				t.Fatal("tampered V10 accepted", kind, err, r.Reasons)
			}
			if kind == "unknown_retry_private_changed" && (r.Cost.Unknown != 1 || r.Cost.UnknownInput != 1 || r.Cost.UnknownOutput != 1) {
				t.Fatal("factual unknown expense lost after denial")
			}
		})
	}
}

func TestV10AuditRetained556History(t *testing.T) { verifyRetainedFailedV9Source(candidateRepo(t)) }

func TestV10AuditRetained556Tampering(t *testing.T) {
	for _, kind := range []string{"old_ceiling_refunded", "invented_556_failure_cause", "paid_cost_removed", "last_unknown_action_promoted", "old_no_go_promoted"} {
		t.Run(kind, func(t *testing.T) {
			all := readRetainedFailedV9(candidateRepo(t))
			switch kind {
			case "old_ceiling_refunded":
				obj(all["bridge"])["remaining_original_absolute615_attempts"] = jsonNumber(71)
			case "invented_556_failure_cause":
				obj(obj(all["failure-correlation"])["failure_classification"])["exact_ordinal556_failure_subcause"] = "timer_confirmed"
			case "paid_cost_removed":
				obj(arr(obj(all["history"])["usage"])[555])["actual_input_tokens"] = jsonNumber(0)
			case "last_unknown_action_promoted":
				obj(arr(obj(all["proposal-diagnostics"])["proposals"])[9])["actual_action_state"] = "effect_confirmed"
			case "old_no_go_promoted":
				obj(obj(all["final-audit"])["chain_audit"])["status"] = "CHAIN_V9_WITNESS_AUDIT_PASS"
			}
			defer func() {
				if recover() == nil {
					t.Fatal("tampered closed history accepted")
				}
			}()
			verifyRetainedFailedV9(all)
		})
	}
}
