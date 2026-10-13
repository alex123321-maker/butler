# New V7 High chain from actual539

The direct user policy amendment allows longer High reasoning and safe timeout
recovery, and permits reuse of already verified unchanged checks. The original
quality and paired comparisons retain their measured120-second profile and all
thresholds. Production defaults are unchanged; this is a new isolated native
release-chain observation, not another entry into the sealed V5 or V6 windows.

The immutable scope starts from539 with76 new requests at most, ending at615.
Requested model is `gemini-3.8-flash-high`, successful observed version must be
`gemini-3.8-flash-n`, generation limit32768 including reasoning. A request may
take300 seconds, a case40 minutes and the whole new window120 minutes. Child
budgets retain4 model turns and4 tool calls; conservative active reservation is
4×(300000 model +30000 tool) =1320000ms. Core's budget, provider timeout, observer,
admission SQL and immutable window timing policy bind the same chosen limits.

One trusted nominal timeout in `http_round_trip` or `response_stream` may queue
one two-second recovery per accepted input. Admission requires current source,
owner/lease and exact kernel-operation correlation, a durable started marker,
separate spent reservations, no uncertain effects and a fresh provider session.
Completed requests require complete measured usage and the selected version.
A failed timeout may retain all-three-NULL usage only when independently bound
to safe same-run recovery and the fixed case result. Missing observations are
never filled from the successful retry. All old failures and NULLs remain.

The five ordinary Core/Tool Broker cases remain normal result, restart, proved
delivery absence, lost ACK and owner Stop. They must verify actual delegation,
isolated finite-IR adapter development/checks/activation, original conversion and
prepared checked patch, source and authority guards, absence of repeat effects
and one semantic final notice where permitted. Lost ACK remains UNKNOWN; stopped
descendants cannot act. Loopback fixture channels and an unapplied patch remain
the declared result boundaries.

Before native execution, check only the31 named changed-history, timing and
retry predicates with disposable PostgreSQL/race, compile and vet the final
fixture, then independently freeze the clean source, protocol and binaries.
The unchanged production retry/fault/broad suites retain their existing exact
proofs. The independent issue-wide audit remains necessary before PR readiness,
merge and issue closure.

```powershell
pwsh -NoProfile -File scripts/dev/lab.ps1 -Action Sync -RemoteRoot /srv/storage/butler/agents/scenario-release-chain-v7-high-20261013
```

```sh
bash ROOT/current/scripts/dev/scenario-release-chain-v7-high-lab.sh ROOT prepare EXACT_COMMIT
# Only after independent source/runtime admission, exactly once:
bash ROOT/current/scripts/dev/scenario-release-chain-v7-high-lab.sh ROOT execute EXACT_COMMIT
```

The [factual539 bridge](../../deploy/acceptance/release-chain-v7-carry539-independent-20261013.json),
[human amendment](../../deploy/acceptance/release-chain-v7-user-policy-amendment-20261013.json)
and [new protocol](../../deploy/acceptance/release-chain-v7-high-prerequisite-20261013.json)
pin the evidence and operational choices. This prospective plan itself claims no
successful native chain or issue readiness.
