package main

import (
	"reflect"
	"testing"
)

func upgradeV13Witness(t *testing.T, base inputs) (inputs, auditPolicy) {
	x, _, private := upgradeV12Witness(t, base)
	p := v13AuditPolicy()
	scope, ledger := obj(x.Scope), obj(x.Ledger)
	scope["window_id"] = p.WindowID
	for _, w := range []object{obj(obj(scope["scope"])["window"]), obj(ledger["window"])} {
		w["prerequisite_hash"] = p.PrerequisiteSHA
	}
	x.Prerequisite, x.SafeTimeoutWaiver = frozenObject(v13PrerequisiteSemantic), frozenObject(v13HumanSemantic)
	x = normalizedInputs(x)
	p.ReadPrivateProposal = func(m object) []byte { return private[integer(m["ordinal"])] }
	return x, p
}
func TestV13AuditFiveCasesSame575ScopeAndPredicates(t *testing.T) {
	x, p := upgradeV13Witness(t, syntheticWitness(t))
	r, err := auditWithPolicy(x, p)
	if err != nil || r.Status != "CHAIN_V13_WITNESS_AUDIT_PASS" || r.Prior != 575 || r.Total != 645 || r.KnownHistoricalTotal != 1039921 || r.UnknownHistoricalTotal != 49 || len(r.Cases) != 5 || r.Ready || r.AuditorCalls != 0 {
		t.Fatal("prospective V13 scope/result predicates changed", err, r.Reasons)
	}
	if !reflect.DeepEqual(p.WindowPolicy, v12AuditPolicy().WindowPolicy) || p.Baseline != 575 || p.Maximum != 120 || p.Baseline+p.Maximum != 695 || selectedAuditPolicy("v12").PassStatus != "CHAIN_V12_WITNESS_AUDIT_PASS" {
		t.Fatal("old allocation/policy changed")
	}
}
func TestV13AuditSafeRecoveryRetainsUnknownAndCriticalDeny(t *testing.T) {
	x, p := upgradeV13Witness(t, waivedStreamTimeout(t, 0, true, true))
	r, err := auditWithPolicy(x, p)
	if err != nil || r.Status != "CHAIN_V13_WITNESS_AUDIT_PASS" || r.Cost.Unknown != 1 || r.AcceptedRetryFailures != 1 || r.MeasuredComplete || r.UnknownHistoricalTotal != 49 {
		t.Fatal("safe nominal recovery expense changed", err, r.Reasons)
	}
	tamperProduct(&x, 0, func(fin object) { obj(arr(obj(fin["original_result"])["check_receipts"])[0])["applied"] = true })
	r, err = auditWithPolicy(x, p)
	if err == nil || r.Status != "NO_GO" || r.Cost.Unknown != 1 || r.AcceptedRetryFailures != 0 || r.AcceptedUnknownCost != 0 || r.AcceptedUnknownVersion != 0 || r.AcceptedUnmeasured != 0 {
		t.Fatal("critical effect/result deny changed", err, r.Reasons)
	}
}
func TestV13AuditBootstrapPreservesOriginalNullAndZeroSourceProof(t *testing.T) {
	verifyV13BootstrapSource(candidateRepo(t))
}
func TestV13AuditBootstrapRejectsTampering(t *testing.T) {
	for _, kind := range []string{"zero_from_absence", "original_null_overwritten", "old_no_go_promoted", "paid_ordinal_added", "provider_reachable", "unsealed_old_scope"} {
		t.Run(kind, func(t *testing.T) {
			b := frozenObject(v13BootstrapSemantic)
			switch kind {
			case "zero_from_absence":
				b["boundary_basis"] = []any{"empty exports alone"}
			case "original_null_overwritten":
				b["actual_native_calls_field_original"] = jsonNumber(0)
			case "old_no_go_promoted":
				b["status"] = "PASS"
			case "paid_ordinal_added":
				b["historical_attempts_unchanged"] = jsonNumber(576)
			case "provider_reachable":
				b["native_provider_constructor_reachable"] = true
			case "unsealed_old_scope":
				b["original_window_remains_sealed"] = false
			}
			defer func() {
				if recover() == nil {
					t.Fatal("sealed bootstrap evidence changed", kind)
				}
			}()
			verifyV13Bootstrap(b)
		})
	}
}
func TestV13AuditRawProtocolPins(t *testing.T) {
	for _, item := range []struct{ file, pin, semantic string }{{"release-chain-v13-high-prerequisite-20261013.json", v13PrerequisiteSHA, v13PrerequisiteSemantic}, {"release-chain-v13-user-policy-amendment-20261013.json", v13HumanSHA, v13HumanSemantic}} {
		value := actualPublic(t, item.file, item.pin)
		if !reflect.DeepEqual(obj(value), frozenObject(item.semantic)) {
			t.Fatal("raw prospective V13 binding differs")
		}
	}
}
