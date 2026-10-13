package main

import (
	"encoding/json"
	"testing"
)

func upgradeV15Witness(t *testing.T, base inputs) (inputs, auditPolicy) {
	t.Helper()
	x, p := upgradeV14Witness(t, base)
	oldPrivate := p.ReadPrivateProposal
	p = v15AuditPolicy()
	scope, ledger := obj(x.Scope), obj(x.Ledger)
	scope["window_id"], scope["baseline_attempts"], scope["maximum_total_attempts"] = p.WindowID, jsonNumber(611), jsonNumber(731)
	ledger["baseline_attempts"] = jsonNumber(611)
	for _, w := range []object{obj(obj(scope["scope"])["window"]), obj(ledger["window"])} {
		w["native_baseline"], w["attempts"] = jsonNumber(611), jsonNumber(integer(w["attempts"])+18)
		w["prior_history_hash"], w["prerequisite_hash"] = p.PriorSHA, p.PrerequisiteSHA
	}
	obj(scope["scope"])["total_attempts"] = jsonNumber(integer(obj(scope["scope"])["total_attempts"]) + 18)
	ledger["total_attempts"] = jsonNumber(integer(ledger["total_attempts"]) + 18)
	for _, list := range []any{obj(scope["scope"])["requests"], ledger["requests"]} {
		for _, v := range arr(list) {
			n := obj(v)
			n["ordinal"] = jsonNumber(integer(n["ordinal"]) + 18)
		}
	}
	private := map[int64][]byte{}
	for _, value := range arr(scope["proposal_observations"]) {
		m := obj(value)
		var rec privateProposal
		if json.Unmarshal(oldPrivate(m), &rec) != nil {
			t.Fatal("synthetic original private packet invalid")
		}
		rec.Ordinal += 18
		raw, _ := json.Marshal(rec)
		private[rec.Ordinal] = raw
		m["ordinal"], m["private_proposal_sha256"], m["private_proposal_bytes"] = jsonNumber(rec.Ordinal), digest(raw), jsonNumber(int64(len(raw)))
	}
	x.Prerequisite, x.SafeTimeoutWaiver = frozenObject(v15PrerequisiteSemantic), frozenObject(v15HumanSemantic)
	x = normalizedInputs(x)
	p.ReadPrivateProposal = func(m object) []byte { return private[integer(m["ordinal"])] }
	return x, p
}

func TestV15AuditFiveCasesSameCriticalPredicates(t *testing.T) {
	x, p := upgradeV15Witness(t, syntheticWitness(t))
	r, err := auditWithPolicy(x, p)
	if err != nil || r.Status != "CHAIN_V15_WITNESS_AUDIT_PASS" || r.Prior != 611 || r.Total != 681 || r.KnownHistoricalTotal != 1512064 || r.UnknownHistoricalTotal != 49 || len(r.Cases) != 5 || r.Ready || r.AuditorCalls != 0 {
		t.Fatal("prospective scope/result predicates changed", err, r.Reasons)
	}
	if p.Baseline+p.Maximum != 731 || selectedAuditPolicy("v14").RequireTaskScopedOriginalProof || selectedAuditPolicy("v14").Baseline+selectedAuditPolicy("v14").Maximum != 713 {
		t.Fatal("legacy scope/proof changed")
	}
}

func TestV15AuditSafeRecoveryCriticalDenyKeepsUnknown(t *testing.T) {
	x, p := upgradeV15Witness(t, waivedStreamTimeout(t, 0, true, true))
	r, err := auditWithPolicy(x, p)
	if err != nil || r.Status != "CHAIN_V15_WITNESS_AUDIT_PASS" || r.Cost.Unknown != 1 || r.AcceptedRetryFailures != 1 || r.MeasuredComplete {
		t.Fatal("safe nominal recovery changed", err, r.Reasons)
	}
	tamperProduct(&x, 0, func(fin object) { obj(arr(obj(fin["original_result"])["check_receipts"])[0])["applied"] = true })
	r, err = auditWithPolicy(x, p)
	if err == nil || r.Status != "NO_GO" || r.Cost.Unknown != 1 || r.AcceptedRetryFailures != 0 || r.AcceptedUnknownCost != 0 || r.AcceptedUnknownVersion != 0 || r.AcceptedUnmeasured != 0 {
		t.Fatal("critical effect/result deny changed", err, r.Reasons)
	}
}

func TestV15AuditRawProtocolPins(t *testing.T) {
	verify := v15AuditPolicy()
	verify.VerifyPrerequisite(obj(actualPublic(t, "release-chain-v15-high-prerequisite-20261013.json", v15PrerequisiteSHA)))
	verify.VerifyHuman(inputs{ExpenseException: syntheticWitness(t).ExpenseException, SafeTimeoutWaiver: actualPublic(t, "release-chain-v15-user-policy-amendment-20261013.json", v15HumanSHA)})
}

func TestV15AuditActual611PreservesUnverifiedOriginalAndCost(t *testing.T) {
	verifyRetainedFailedV14Source(candidateRepo(t))
}

func TestV15AuditQualifiedEarlierPhaseAndOwnerConversion(t *testing.T) {
	x, _ := upgradeV15Witness(t, syntheticWitness(t))
	fin := obj(witnesses(x.Product)["normal_sent"]["payload_json"])
	db, o := obj(fin["postgres"]), obj(fin["original_result"])
	var earlier string
	for _, a := range rows(db, "actions") {
		if a["tool_name"] == "converters.collect" {
			earlier = text(a["run_id"])
		}
	}
	if earlier == "" {
		t.Fatal("synthetic previous phase missing")
	}
	for _, a := range rows(db, "actions") {
		if a["tool_name"] == "extension.project.prepare-patch" {
			a["run_id"] = earlier
		}
	}
	for _, list := range []string{"check_receipts", "inspect_receipts"} {
		for _, r := range rows(o, list) {
			r["run_id"] = earlier
		}
	}
	// Pure phase predicate only: relocating this synthetic proof does not claim
	// new native admission, action dispatch or private-packet correspondence.
	verifyV15TaskScopedOriginalProof("normal_sent", obj(witnesses(x.Initial)["normal_sent"]["payload_json"]), fin)
	for _, kind := range []string{"stale_phase_revision", "foreign_read_run", "owner_wrong_final"} {
		t.Run(kind, func(t *testing.T) {
			bad := obj(clone(fin))
			pg := obj(bad["postgres"])
			switch kind {
			case "stale_phase_revision":
				byID(rows(pg, "runs"), "run_id")[earlier]["task_revision"] = jsonNumber(2)
			case "foreign_read_run":
				obj(arr(obj(bad["original_result"])["check_receipts"])[0])["run_id"] = "foreign"
			case "owner_wrong_final":
				for _, result := range rows(pg, "task_results") {
					if result["verdict"] == "verified" {
						result["run_id"] = earlier
					}
				}
			}
			defer func() {
				if recover() == nil {
					t.Fatal("foreign/stale phase or owner final accepted")
				}
			}()
			verifyV15TaskScopedOriginalProof("normal_sent", obj(witnesses(x.Initial)["normal_sent"]["payload_json"]), bad)
		})
	}
}

func TestV15AuditRetained611NoAcceptanceRewrite(t *testing.T) {
	all := readRetainedFailedV14(candidateRepo(t))
	obj(obj(all["final-audit"])["chain_audit"])["verified_scenarios"] = []any{"normal_sent"}
	obj(obj(all["final-audit"])["chain_audit"])["status"] = "PASS"
	defer func() {
		if recover() == nil {
			t.Fatal("old failed scope promoted")
		}
	}()
	verifyRetainedFailedV14(all)
}
