# Only unchecked release cases on fresh High

The human explicitly requested skipping work already performed and proceeding
directly to unchecked cases. V16 runs only restart_sent, explicit_absence,
lost_ack and owner_stop, in that order, immediately after its frozen build.
It does not run normal_sent again or require an offline/pre-native review cycle.

Acceptance is an explicit composite: preserved V14 actual original CSV
conversion and checked immutable README patch, plus these four actual fault
cases. Restart_sent must prove the complete current Core/Broker/independent
original verification/one-sent-notice chain. The old V14 normal_sent remains
NO_GO with no owner-verified result or notification; it is never renamed PASS.
The child author cannot change goldens or acceptance predicates. Final separate
independent source/PG/proposal/cost acceptance remains required before merge.

V15 ended at616, with four complete measured requests612–615 and one local
request_serialization/provider_input_bytes_exceeded denial616. The native
serialized size and usage of that failed entry were not returned and remain
UNKNOWN/NULL. The successful615 response took207.712s; it was not a timeout.
No retry or refunded attempt is invented. Closed731 has115 unused, sealed.

The new test-only serialized-input allowance is4MiB per request instead of the
legacy128KiB. Day allowance is120*4MiB=480MiB, child4*4MiB=16MiB; existing Task
and Mandate256MiB limits cover their24/40 finite quotes. Production default/max
8MiB and all old fixtures are unchanged. All eight resource dimensions, current
sources/authority, Stop, repeated-write prevention and delivery UNKNOWN remain.

Actual616 history retains known subtotals1135037input/26407output/1578335total,
586 known input/output pairs,30 unknown pairs and50 unknown full costs. Fresh
V16 is616+120=736. Its owning PostgreSQL counter/deadline and window remain
immutable. Old failures, normal-case status and raw artifacts remain sealed.

```powershell
pwsh -NoProfile -File scripts/dev/lab.ps1 -Action Sync -RemoteRoot /srv/storage/butler/agents/scenario-release-chain-v16-high-20261013
```

```bash
ROOT=/srv/storage/butler/agents/scenario-release-chain-v16-high-20261013
bash "$ROOT/current/scripts/dev/scenario-release-chain-v16-high-lab.sh" "$ROOT" EXACT_CLEAN_COMMIT
```

The command performs a build-only preparation then one fresh native execution.
No offline PASS is claimed. Existing source gate, TLS and credential metadata
checks precede marker/native entry. Narrow new helper and PostgreSQL checks run
in parallel and remain separate from factual native results and final acceptance.

The V16 window is now closed **NO_GO**. Its first `restart_sent` case stopped
at the exact Broker inventory assertion; the other three cases did not run.
Requests 617–636 all completed with measured usage: 237570 input, 4108 output
and 265857 total tokens. Native failure, timeout, retry and new unknown usage
counts are zero. These are successful model responses, not accepted product
results. The original product export is the literal three-byte `[]\n`; it
cannot substantiate effects, all eight budget dimensions or an owner notice.

The independently matched proposals contain two `extension.project.prepare-patch`
calls, 629 and 634, with the same authorization arguments hash. This tool is a
POST with commit effects; proposal equivalence alone does not prove identical
external artifacts or safe deduplication. The repeated read-only inspect and
check calls do not relax the requirement for one logical write. A future repair
must retain the actual owning PostgreSQL snapshot even when semantic result
capture fails, and must address the primary repeated-write behavior separately.

The [closed audit](../../deploy/acceptance/release-chain-v16-final-audit-20261013.json),
[failure correlation](../../deploy/acceptance/release-chain-v16-failure-correlation-20261013.json),
[exact history through636](../../deploy/acceptance/release-chain-v16-history636-20261013.json)
and [independent carry](../../deploy/acceptance/release-chain-v16-carry636-independent-20261013.json)
retain this result. Known cumulative subtotals are 1372607 input, 30515 output
and 1844192 total tokens; 30 input/output pairs and 50 full costs remain unknown.
The carry fold is
`314bf4a5a3b649f6dc6354a03ea4c536a85f6a8ea92c756ef50bcc288aeda951`.
The unused allowance in the closed736 window remains sealed.
