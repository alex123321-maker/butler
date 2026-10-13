package main

import (
	"encoding/json"
	"reflect"
	"testing"
)

func upgradeV14Witness(t *testing.T, base inputs) (inputs, auditPolicy) {
	t.Helper()
	x, _, oldPrivate := upgradeV12Witness(t, base)
	p := v14AuditPolicy()
	scope, ledger := obj(x.Scope), obj(x.Ledger)
	scope["window_id"], scope["baseline_attempts"], scope["maximum_total_attempts"] = p.WindowID, jsonNumber(593), jsonNumber(713)
	ledger["baseline_attempts"] = jsonNumber(593)
	for _, w := range []object{obj(obj(scope["scope"])["window"]), obj(ledger["window"])} {
		w["native_baseline"], w["attempts"] = jsonNumber(593), jsonNumber(integer(w["attempts"])+18)
		w["prior_history_hash"], w["prerequisite_hash"] = p.PriorSHA, p.PrerequisiteSHA
	}
	obj(scope["scope"])["total_attempts"] = jsonNumber(integer(obj(scope["scope"])["total_attempts"]) + 18)
	ledger["total_attempts"] = jsonNumber(integer(ledger["total_attempts"]) + 18)
	for _, list := range []any{obj(scope["scope"])["requests"], ledger["requests"]} {
		for _, value := range arr(list) {
			n := obj(value)
			n["ordinal"] = jsonNumber(integer(n["ordinal"]) + 18)
		}
	}
	private := map[int64][]byte{}
	for _, value := range arr(scope["proposal_observations"]) {
		m := obj(value)
		var rec privateProposal
		if json.Unmarshal(oldPrivate[integer(m["ordinal"])], &rec) != nil {
			t.Fatal("synthetic original private packet invalid")
		}
		rec.Ordinal += 18
		raw, _ := json.Marshal(rec)
		private[rec.Ordinal] = raw
		m["ordinal"], m["private_proposal_sha256"], m["private_proposal_bytes"] = jsonNumber(rec.Ordinal), digest(raw), jsonNumber(int64(len(raw)))
	}
	x.Prerequisite, x.SafeTimeoutWaiver = frozenObject(v14PrerequisiteSemantic), frozenObject(v14HumanSemantic)
	x = normalizedInputs(x)
	p.ReadPrivateProposal = func(m object) []byte { return private[integer(m["ordinal"])] }
	return x, p
}

func TestV14AuditFiveCasesSameCriticalPredicates(t *testing.T) {
	x, p := upgradeV14Witness(t, syntheticWitness(t))
	r, err := auditWithPolicy(x, p)
	if err != nil || r.Status != "CHAIN_V14_WITNESS_AUDIT_PASS" || r.Prior != 593 || r.Total != 663 || r.KnownHistoricalTotal != 1296671 || r.UnknownHistoricalTotal != 49 || len(r.Cases) != 5 || r.Ready || r.AuditorCalls != 0 {
		t.Fatal("prospective scope/result predicates changed", err, r.Reasons)
	}
	if !reflect.DeepEqual(p.WindowPolicy, v13AuditPolicy().WindowPolicy) || p.Maximum != 120 || p.Baseline+p.Maximum != 713 || selectedAuditPolicy("v13").PassStatus != "CHAIN_V13_WITNESS_AUDIT_PASS" {
		t.Fatal("legacy allocation/policy changed")
	}
}

func TestV14AuditSafeRecoveryCriticalDenyKeepsUnknown(t *testing.T) {
	x, p := upgradeV14Witness(t, waivedStreamTimeout(t, 0, true, true))
	r, err := auditWithPolicy(x, p)
	if err != nil || r.Status != "CHAIN_V14_WITNESS_AUDIT_PASS" || r.Cost.Unknown != 1 || r.AcceptedRetryFailures != 1 || r.MeasuredComplete || r.UnknownHistoricalTotal != 49 {
		t.Fatal("safe nominal recovery changed", err, r.Reasons)
	}
	tamperProduct(&x, 0, func(fin object) { obj(arr(obj(fin["original_result"])["check_receipts"])[0])["applied"] = true })
	r, err = auditWithPolicy(x, p)
	if err == nil || r.Status != "NO_GO" || r.Cost.Unknown != 1 || r.AcceptedRetryFailures != 0 || r.AcceptedUnknownCost != 0 || r.AcceptedUnknownVersion != 0 || r.AcceptedUnmeasured != 0 {
		t.Fatal("critical result/effect denial changed", err, r.Reasons)
	}
}

func TestV14AuditRawProtocolPins(t *testing.T) {
	for _, item := range []struct{ file, pin, semantic string }{{"release-chain-v14-high-prerequisite-20261013.json", v14PrerequisiteSHA, v14PrerequisiteSemantic}, {"release-chain-v14-user-policy-amendment-20261013.json", v14HumanSHA, v14HumanSemantic}} {
		if value := actualPublic(t, item.file, item.pin); !reflect.DeepEqual(obj(value), frozenObject(item.semantic)) {
			t.Fatal("raw prospective binding differs")
		}
	}
}

func TestV14AuditActual593ClosedNullAndCost(t *testing.T) {
	verifyRetainedFailedV13Source(candidateRepo(t))
}

func TestV14AuditRetained593Tampering(t *testing.T) {
	for _, kind := range []string{"old_unknown_refilled", "supplemental_promoted", "metadata_refilled", "go_stdout_invented", "stale_generation_relabelled", "paid_cost_changed", "refunded_old_scope"} {
		t.Run(kind, func(t *testing.T) {
			all := readRetainedFailedV13(candidateRepo(t))
			switch kind {
			case "old_unknown_refilled":
				obj(arr(obj(all["history"])["usage"])[536])["actual_input_tokens"] = jsonNumber(0)
			case "supplemental_promoted":
				obj(obj(all["final-audit"])["chain_audit"])["status"] = "PASS"
			case "metadata_refilled":
				obj(all["high"])["proposal_observations"] = []any{}
			case "go_stdout_invented":
				obj(all["final-audit"])["canonical_go_json_available"] = true
			case "stale_generation_relabelled":
				obj(obj(all["failure-correlation"])["failure_classification"])["denied_generation_matches_current_catalog"] = true
			case "paid_cost_changed":
				obj(arr(obj(obj(all["high"])["scope"])["requests"])[0])["actual_input_tokens"] = jsonNumber(0)
			case "refunded_old_scope":
				obj(all["bridge"])["remaining_original_absolute695_attempts"] = jsonNumber(120)
			}
			defer func() {
				if recover() == nil {
					t.Fatal("sealed evidence change accepted", kind)
				}
			}()
			verifyRetainedFailedV13(all)
		})
	}
}
