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
