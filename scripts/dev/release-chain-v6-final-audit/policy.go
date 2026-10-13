package main

import "time"

// Policies are explicit reviewed protocols. There is no operator-supplied
// baseline, arbitrary deadline, unknown-cost allowance or generic waiver mode.
type auditPolicy struct {
	Baseline, Maximum                                         int
	KnownTotal                                                int64
	UnknownTotal                                              int
	WindowID, PriorSHA, PrerequisiteSHA, HumanSHA, PassStatus string
	Attempt, Case, Global                                     time.Duration
	HTTPTimeouts                                              bool
	WindowPolicy                                              object
	DayModelTurns, ChildActiveMillis                          int64
	RequireProposals                                          bool
	ReadPrivateProposal                                       func(object) []byte
	VerifyHuman                                               func(inputs)
	VerifyPrerequisite                                        func(object)
}

func v6AuditPolicy() auditPolicy {
	return auditPolicy{Baseline: 537, Maximum: 78, KnownTotal: 572459, UnknownTotal: 47,
		WindowID: "issue33-high-release-chain-v6-20261013-baseline537", PriorSHA: priorSHA,
		PrerequisiteSHA: prerequisiteSHA, HumanSHA: safeTimeoutWaiverSHA,
		PassStatus: "CHAIN_V6_WITNESS_AUDIT_PASS", Attempt: 120 * time.Second,
		Case: 20 * time.Minute, Global: 60 * time.Minute, DayModelTurns: 80, ChildActiveMillis: 600000,
		VerifyHuman: func(in inputs) {
			verifyExpenseException(in.ExpenseException)
			verifySafeTimeoutWaiver(in.SafeTimeoutWaiver)
		},
		VerifyPrerequisite: verifyV6Prerequisite}
}

func v7AuditPolicy() auditPolicy {
	return auditPolicy{Baseline: 539, Maximum: 76, KnownTotal: 580620, UnknownTotal: 48,
		WindowID: "issue33-high-release-chain-v7b-20261013-baseline539", PriorSHA: v7PriorSHA,
		PrerequisiteSHA: v7PrerequisiteSHA, HumanSHA: v7HumanSHA,
		PassStatus: "CHAIN_V7_WITNESS_AUDIT_PASS", Attempt: 300 * time.Second,
		Case: 40 * time.Minute, Global: 120 * time.Minute, HTTPTimeouts: true,
		WindowPolicy:  obj(clone(object{"maximum_generation_tokens": 32768, "safe_timeout_stages": "http_or_stream", "maximum_attempt_ms": 300000, "maximum_case_ms": 2400000, "maximum_global_ms": 7200000})),
		DayModelTurns: 76, ChildActiveMillis: 1320000,
		VerifyHuman: verifyV7Human, VerifyPrerequisite: verifyV7Prerequisite}
}

func selectedAuditPolicy(name string) auditPolicy {
	switch name {
	case "v6":
		return v6AuditPolicy()
	case "v7":
		return v7AuditPolicy()
	case "v8":
		return v8AuditPolicy()
	default:
		require(false, "explicit_reviewed_protocol_required")
		return auditPolicy{}
	}
}
