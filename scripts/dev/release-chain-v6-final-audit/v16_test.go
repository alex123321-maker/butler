package main

import (
	"encoding/json"
	"reflect"
	"strings"
	"testing"
)

func upgradeV16Witness(t *testing.T, base inputs) (inputs, auditPolicy) {
	t.Helper()
	x, oldPolicy := upgradeV15Witness(t, base)
	p := v16AuditPolicy()
	sc, ledger := obj(x.Scope), obj(x.Ledger)
	sc["window_id"], sc["baseline_attempts"], sc["maximum_total_attempts"] = p.WindowID, jsonNumber(616), jsonNumber(736)
	ledger["baseline_attempts"] = jsonNumber(616)
	delete(obj(sc["scenarios"]), "normal_sent")
	keepWitnesses := func(v any) []any {
		out := []any{}
		for _, w := range arr(v) {
			if obj(w)["scenario"] != "normal_sent" {
				out = append(out, w)
			}
		}
		return out
	}
	x.Initial, x.Product = keepWitnesses(x.Initial), keepWitnesses(x.Product)
	native, pgRows := []any{}, []any{}
	ordinals := map[int64]int64{}
	for i, value := range arr(obj(sc["scope"])["requests"]) {
		n := obj(value)
		if n["scenario"] == "normal_sent" {
			continue
		}
		old := integer(n["ordinal"])
		next := int64(617 + len(native))
		ordinals[old] = next
		n["ordinal"] = jsonNumber(next)
		sql := obj(arr(ledger["requests"])[i])
		sql["ordinal"] = jsonNumber(next)
		native, pgRows = append(native, n), append(pgRows, sql)
	}
	obj(sc["scope"])["requests"], ledger["requests"] = native, pgRows
	for _, w := range []object{obj(obj(sc["scope"])["window"]), obj(ledger["window"])} {
		w["native_baseline"], w["attempts"] = jsonNumber(616), jsonNumber(int64(616+len(native)))
		w["prior_history_hash"], w["prerequisite_hash"] = p.PriorSHA, p.PrerequisiteSHA
	}
	for _, o := range []object{obj(sc["scope"]), ledger} {
		o["total_attempts"], o["new_attempts"] = jsonNumber(int64(616+len(native))), jsonNumber(int64(len(native)))
	}
	private, metadata := map[int64][]byte{}, []any{}
	for _, v := range arr(sc["proposal_observations"]) {
		m := obj(v)
		if m["scenario"] == "normal_sent" {
			continue
		}
		var rec privateProposal
		if json.Unmarshal(oldPolicy.ReadPrivateProposal(m), &rec) != nil {
			t.Fatal("synthetic original private packet invalid")
		}
		rec.Ordinal = ordinals[rec.Ordinal]
		raw, _ := json.Marshal(rec)
		private[rec.Ordinal] = raw
		m["ordinal"], m["private_proposal_sha256"], m["private_proposal_bytes"] = jsonNumber(rec.Ordinal), digest(raw), jsonNumber(int64(len(raw)))
		metadata = append(metadata, m)
	}
	sc["proposal_observations"] = metadata
	for i := range arr(x.Product) {
		tamperProduct(&x, i, func(fin object) {
			db := obj(fin["postgres"])
			modelOps := byID(rows(db, "model_budget_attempts"), "operation_id")
			sums := map[string]int64{}
			for _, reservation := range rows(db, "budget_reservations") {
				if modelOps[text(reservation["operation_id"])] != nil {
					obj(reservation["charge"])["provider_input_bytes"] = jsonNumber(4 << 20)
				}
				for _, id := range arr(reservation["scope_ids"]) {
					sums[text(id)] += integer(obj(reservation["charge"])["provider_input_bytes"])
				}
			}
			for _, s := range rows(db, "budget_scopes") {
				obj(s["reserved"])["provider_input_bytes"] = jsonNumber(sums[text(s["scope_id"])])
				if s["kind"] == "owner_day" {
					obj(s["limits"])["provider_input_bytes"] = jsonNumber(120 * (4 << 20))
				} else if s["kind"] == "delegation" {
					obj(s["limits"])["provider_input_bytes"] = jsonNumber(4 * (4 << 20))
				} else if s["kind"] == "task" && s["subject_id"] == db["root_task_id"] {
					obj(s["limits"])["provider_input_bytes"] = jsonNumber(256 << 20)
				}
			}
		})
	}
	x.Prerequisite, x.SafeTimeoutWaiver = frozenObject(v16PrerequisiteSemantic), frozenObject(v16HumanSemantic)
	x = normalizedInputs(x)
	p.ReadPrivateProposal = func(m object) []byte { return private[integer(m["ordinal"])] }
	return x, p
}

func TestV16AuditPendingFourCompositeNoNormalAlias(t *testing.T) {
	x, p := upgradeV16Witness(t, syntheticWitness(t))
	r, err := auditWithPolicy(x, p)
	if err != nil || r.Status != "CHAIN_V16_WITNESS_AUDIT_PASS" || r.Prior != 616 || r.Total != 669 || r.KnownHistoricalTotal != 1578335 || r.UnknownHistoricalTotal != 50 || !reflect.DeepEqual(r.Cases, p.ScenarioNames) || r.CommonCompletionEvidence != "restart_sent_current_owner_result_and_single_sent_notice" || r.Ready || r.AuditorCalls != 0 {
		t.Fatal("pending four/current common tail proof changed", err, r.Reasons)
	}
	if selectedAuditPolicy("v15").ScenarioNames != nil || selectedAuditPolicy("v15").ModelInputBytes != 0 || selectedAuditPolicy("v15").Baseline+selectedAuditPolicy("v15").Maximum != 731 || strings.Contains(strings.Join(r.Cases, ","), "normal_sent") {
		t.Fatal("old five-case policy or normal outcome rewritten")
	}
}

func TestV16AuditCurrentRecoveryUnknownAndCriticalDeny(t *testing.T) {
	x, p := upgradeV16Witness(t, waivedStreamTimeout(t, 1, true, true))
	r, err := auditWithPolicy(x, p)
	if err != nil || r.Status != "CHAIN_V16_WITNESS_AUDIT_PASS" || r.Cost.Unknown != 1 || r.AcceptedRetryFailures != 1 || r.MeasuredComplete {
		t.Fatal("safe current retry changed or historical616 used as waiver", err, r.Reasons)
	}
	tamperProduct(&x, 0, func(fin object) { obj(arr(obj(fin["original_result"])["check_receipts"])[0])["applied"] = true })
	r, err = auditWithPolicy(x, p)
	if err == nil || r.Status != "NO_GO" || r.CommonCompletionEvidence != "" || r.Cost.Unknown != 1 || r.AcceptedRetryFailures != 0 || r.AcceptedUnknownCost != 0 || r.AcceptedUnknownVersion != 0 || r.AcceptedUnmeasured != 0 {
		t.Fatal("critical effect/result denial changed", err, r.Reasons)
	}
}

func TestV16AuditRawProtocolPins(t *testing.T) {
	p := v16AuditPolicy()
	p.VerifyPrerequisite(obj(actualPublic(t, "release-chain-v16-high-prerequisite-20261013.json", v16PrerequisiteSHA)))
	p.VerifyHuman(inputs{ExpenseException: syntheticWitness(t).ExpenseException, SafeTimeoutWaiver: actualPublic(t, "release-chain-v16-user-policy-amendment-20261013.json", v16HumanSHA)})
}

func TestV16AuditActual616SerializationUnknownPreserved(t *testing.T) {
	verifyRetainedFailedV15Source(candidateRepo(t))
}

func TestV16AuditFourCaseAndInputResourceTampering(t *testing.T) {
	for _, kind := range []string{"missing_restart", "normal_pass_alias", "no_restart_notice", "lost_ack_sent", "stop_result", "unmeasured_success", "wrong_model_input_quote", "enlarged_day_bytes", "enlarged_child_bytes", "enlarged_root_bytes"} {
		t.Run(kind, func(t *testing.T) {
			x, p := upgradeV16Witness(t, syntheticWitness(t))
			sc := obj(x.Scope)
			switch kind {
			case "missing_restart":
				delete(obj(sc["scenarios"]), "restart_sent")
			case "normal_pass_alias":
				obj(sc["scenarios"])["normal_sent"] = true
			case "unmeasured_success":
				changeNative(&x, 0, func(n object) { n["actual_input_tokens"] = nil })
			default:
				index := 0
				if kind == "lost_ack_sent" {
					index = 2
				} else if kind == "stop_result" {
					index = 3
				}
				tamperProduct(&x, index, func(fin object) {
					db := obj(fin["postgres"])
					switch kind {
					case "no_restart_notice":
						db["notices"] = []any{}
					case "lost_ack_sent":
						rows(db, "notices")[0]["status"] = "sent"
					case "stop_result":
						fin["original_result"] = object{"checks_passed": true}
					case "wrong_model_input_quote":
						obj(rows(db, "budget_reservations")[0]["charge"])["provider_input_bytes"] = jsonNumber(131072)
					case "enlarged_day_bytes", "enlarged_child_bytes", "enlarged_root_bytes":
						for _, scope := range rows(db, "budget_scopes") {
							if kind == "enlarged_day_bytes" && scope["kind"] == "owner_day" || kind == "enlarged_child_bytes" && scope["kind"] == "delegation" || kind == "enlarged_root_bytes" && scope["kind"] == "task" && scope["subject_id"] == db["root_task_id"] {
								limit := obj(scope["limits"])
								limit["provider_input_bytes"] = jsonNumber(integer(limit["provider_input_bytes"]) + 1)
							}
						}
					}
				})
			}
			r, err := auditWithPolicy(x, p)
			if err == nil || r.Status != "NO_GO" || r.CommonCompletionEvidence != "" || r.AcceptedRetryFailures != 0 || r.AcceptedUnknownCost != 0 || r.AcceptedUnknownVersion != 0 || r.AcceptedUnmeasured != 0 {
				t.Fatal("missing fault/current result or enlarged resource accepted", kind, err, r.Reasons)
			}
		})
	}
}

func TestV16AuditNoZeroSpendOrRetryForHistorical616(t *testing.T) {
	all := readRetainedFailedV15(candidateRepo(t))
	n := obj(arr(obj(obj(all["high"])["scope"])["requests"])[4])
	n["actual_input_tokens"], n["actual_output_tokens"], n["actual_total_tokens"] = jsonNumber(0), jsonNumber(0), jsonNumber(0)
	defer func() {
		if recover() == nil {
			t.Fatal("historical unobserved expense filled as zero")
		}
	}()
	verifyRetainedFailedV15(all)
}
