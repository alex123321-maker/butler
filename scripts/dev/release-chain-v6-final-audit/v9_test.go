package main

import (
	"encoding/json"
	"strings"
	"testing"
	"time"
)

func upgradeV9Witness(t *testing.T, x inputs) (inputs, auditPolicy, map[int64][]byte) {
	t.Helper()
	x, _, oldPrivate := upgradeV8Witness(t, x)
	p := v9AuditPolicy()
	scope, ledger := obj(x.Scope), obj(x.Ledger)
	scope["window_id"], scope["baseline_attempts"], scope["maximum_new_attempts"] = p.WindowID, jsonNumber(544), jsonNumber(71)
	ledger["baseline_attempts"] = jsonNumber(544)
	for _, w := range []object{obj(obj(scope["scope"])["window"]), obj(ledger["window"])} {
		w["native_baseline"], w["attempts"], w["maximum_new_attempts"] = jsonNumber(544), jsonNumber(integer(w["attempts"])+3), jsonNumber(71)
		w["prior_history_hash"], w["prerequisite_hash"], w["protocol_policy"] = p.PriorSHA, p.PrerequisiteSHA, clone(p.WindowPolicy)
	}
	obj(scope["scope"])["total_attempts"] = jsonNumber(integer(obj(scope["scope"])["total_attempts"]) + 3)
	ledger["total_attempts"] = jsonNumber(integer(ledger["total_attempts"]) + 3)
	for _, list := range []any{obj(scope["scope"])["requests"], ledger["requests"]} {
		for _, v := range arr(list) {
			r := obj(v)
			r["ordinal"] = jsonNumber(integer(r["ordinal"]) + 3)
		}
	}
	private := map[int64][]byte{}
	for _, value := range arr(scope["proposal_observations"]) {
		m := obj(value)
		old := integer(m["ordinal"])
		var record privateProposal
		if json.Unmarshal(oldPrivate[old], &record) != nil {
			t.Fatal("synthetic private record invalid")
		}
		record.Ordinal += 3
		raw, e := json.Marshal(record)
		if e != nil {
			t.Fatal(e)
		}
		private[record.Ordinal] = raw
		m["ordinal"], m["private_proposal_sha256"], m["private_proposal_bytes"] = jsonNumber(record.Ordinal), digest(raw), jsonNumber(int64(len(raw)))
	}
	for j := range arr(x.Product) {
		tamperProduct(&x, j, func(fin object) {
			for _, s := range rows(obj(fin["postgres"]), "budget_scopes") {
				if s["kind"] == "owner_day" {
					limits := obj(s["limits"])
					limits["model_turns"], limits["tokens"], limits["active_millis"] = jsonNumber(71), jsonNumber(71*1081344), jsonNumber(71*300000)
				}
			}
		})
	}
	x.Prerequisite, x.SafeTimeoutWaiver = frozenObject(v9PrerequisiteSemantic), frozenObject(v9HumanSemantic)
	x = normalizedInputs(x)
	p.ReadPrivateProposal = func(m object) []byte { return private[integer(m["ordinal"])] }
	return x, p, private
}

func TestV9AuditFiveCasesSerializedOriginalProposals(t *testing.T) {
	x, p, _ := upgradeV9Witness(t, syntheticWitness(t))
	r, e := auditWithPolicy(x, p)
	if e != nil || r.Status != "CHAIN_V9_WITNESS_AUDIT_PASS" || r.Prior != 544 || r.Total != 614 || r.UnknownHistoricalTotal != 48 || r.KnownHistoricalTotal != 647378 || len(r.Cases) != 5 || r.Ready || r.AuditorCalls != 0 {
		t.Fatal("five-case V9 rejected or overclaimed", e, r.Reasons)
	}
}

func TestV9AuditSerializedRecoveredUnknownTimeout(t *testing.T) {
	x, p, _ := upgradeV9Witness(t, waivedStreamTimeout(t, 0, true, true))
	for j, v := range arr(obj(obj(x.Scope)["scope"])["requests"]) {
		if obj(v)["outcome"] != "completed" {
			changeNative(&x, j, func(n object) { n["provider_operation_stage"] = "http_round_trip" })
			break
		}
	}
	r, e := auditWithPolicy(x, p)
	if e != nil || r.Status != "CHAIN_V9_WITNESS_AUDIT_PASS" || r.Total != 615 || r.Cost.Unknown != 1 || r.AcceptedRetryFailures != 1 || r.AcceptedUnknownCost != 1 || r.AcceptedUnknownVersion != 1 || r.MeasuredComplete || r.Ready {
		t.Fatal("safe serialized timeout not correlated", e, r.Reasons)
	}
}

func TestV9AuditIntervalsPolicyAndCriticalTampering(t *testing.T) {
	for _, kind := range []string{"parent_child_overlap", "failed_retry_overlap", "missing_serialization_policy", "expanded_cap", "different_prior_history", "changed_protocol", "changed_human", "private_proposal_changed", "unknown_timeout_private_proposal_changed", "original_result_changed"} {
		t.Run(kind, func(t *testing.T) {
			initial := syntheticWitness(t)
			if kind == "failed_retry_overlap" || kind == "unknown_timeout_private_proposal_changed" {
				initial = waivedStreamTimeout(t, 0, true, true)
			}
			x, p, private := upgradeV9Witness(t, initial)
			overlap := kind == "parent_child_overlap" || kind == "failed_retry_overlap"
			if overlap {
				native := arr(obj(obj(x.Scope)["scope"])["requests"])
				index := -1
				for i := 1; i < len(native); i++ {
					prior, next := obj(native[i-1]), obj(native[i])
					if kind == "parent_child_overlap" && prior["run_id"] != next["run_id"] || kind == "failed_retry_overlap" && prior["outcome"] != "completed" {
						index = i
						break
					}
				}
				if index < 0 {
					t.Fatal("missing meaningful overlap fixture")
				}
				at := moment(obj(native[index-1])["finished_at_utc"]).Add(-time.Millisecond)
				changeNative(&x, index, func(n object) {
					n["admitted_at_utc"] = at.UTC().Format(time.RFC3339Nano)
					n["elapsed_ms"] = jsonNumber(moment(n["finished_at_utc"]).Sub(at).Milliseconds())
				})
			} else {
				switch kind {
				case "missing_serialization_policy":
					delete(obj(obj(obj(obj(x.Scope)["scope"])["window"])["protocol_policy"]), "provider_entries_serialized")
				case "expanded_cap":
					obj(obj(obj(x.Scope)["scope"])["window"])["maximum_new_attempts"] = jsonNumber(74)
				case "different_prior_history":
					obj(obj(obj(x.Scope)["scope"])["window"])["prior_history_hash"] = v8PriorSHA
				case "changed_protocol":
					obj(x.Prerequisite)["maximum_concurrent_native_entries"] = jsonNumber(2)
				case "changed_human":
					obj(x.SafeTimeoutWaiver)["provider_entries_serialized"] = false
				case "private_proposal_changed", "unknown_timeout_private_proposal_changed":
					for ordinal, raw := range private {
						private[ordinal] = append(raw, byte(' '))
						break
					}
				case "original_result_changed":
					tamperProduct(&x, 0, func(fin object) { obj(fin["original_result"])["checks_passed"] = false })
				}
			}
			r, e := auditWithPolicy(x, p)
			if e == nil || r.Status != "NO_GO" || r.Ready || r.AcceptedRetryFailures != 0 || r.AcceptedUnknownCost != 0 || r.AcceptedUnknownVersion != 0 || r.AcceptedUnmeasured != 0 {
				t.Fatal("tampered V9 accepted", kind, e, r.Reasons)
			}
			if overlap && !strings.Contains(strings.Join(r.Reasons, ","), "v9_client_request_intervals_overlap") {
				t.Fatal("overlap not tested by interval predicate", r.Reasons)
			}
			if (kind == "failed_retry_overlap" || kind == "unknown_timeout_private_proposal_changed") && (r.Cost.Unknown != 1 || r.Cost.UnknownInput != 1 || r.Cost.UnknownOutput != 1) {
				t.Fatal("factual unknown expense lost after denial")
			}
		})
	}
}

func TestV9AuditRetained544History(t *testing.T) { verifyRetainedFailedV8Source(candidateRepo(t)) }
