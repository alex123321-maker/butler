# Fresh High chain from factual attempt 541

The closed V7b run completed both High requests with measured usage: 540 took
14.815 seconds and 541 took 103.269 seconds. Core then rejected the developer
proposal before delegation. Its arguments were not retained; the specific
rejection cause remains UNKNOWN. See the [independent report](scenario-release-chain-v7b-independent-audit-20261013.md).

V8 uses a fresh isolated root and PostgreSQL schema, never reopening V7 or V7b.
The protocol starts at factual 541, permits at most 74 new attempts and retains
all 48 historical unknown total measurements. The independently reviewed protocol
SHA is `effa65a6e39fcecce2510c76554685d047ee5e8c7ed1b9e6d27e633133d402ec`.
The direct user policy amendment SHA is
`3560bb090a6b2c8b8bc78a987b2331eaf2eb55550b5e62572fbaf0bb7646973d`.

High has a 300-second request bound, 40-minute case bound, 120-minute global
bound and 32768 generation tokens including provider thinking. A proved nominal
HTTP or response-stream timeout permits one durable two-second retry with fresh
ownership, source, authority and budget checks. Effects with lost confirmation
remain UNKNOWN. Counts, child rights, Stop, source checks, delivery semantics and
verification of the original result retain their existing requirements.

The production change aligns developer argument decoding with the existing JSON
integer schema: exact integral decimal and exponent notation is accepted without
changing original argument bytes or authorization hashes. Fractions, overflow,
missing and extra fields remain denied. This is a confirmed synthetic API
compatibility defect; it is not asserted to explain the lost V7b proposal.

The isolated V8 observer writes its own exact typed tool proposal and original
argument bytes into an exclusive 0600 private leaf before Core receives it.
An unavailable or oversized proof denies release. Public diagnostics contain
fixed field types, validation statuses, lengths and hashes only. No whole private
directory or credential material may enter the independent auditor.

Preparation runs the exact manifests in
`scripts/dev/scenario-release-chain-v8-integer-required-tests.txt` and
`scripts/dev/scenario-release-chain-v8-required-tests.txt`: 29 affected decoder
and schema checks, seven app checks including two PostgreSQL retention cases,
and a separate negative test of the sealed V7 native entry. Existing unchanged
Core, retry, quality, comparison and fault evidence remains attributed to its
original source. Final admission requires the new snapshot and runtime pins,
successful preparation and an independent review before native execution.

Reproduce in a fresh isolated root from a clean, committed release:

```powershell
pwsh -NoProfile -File scripts/dev/lab.ps1 -Action Sync -RemoteRoot /srv/storage/butler/agents/scenario-release-chain-v8-high-20261013
```

```bash
ROOT=/srv/storage/butler/agents/scenario-release-chain-v8-high-20261013
bash "$ROOT/current/scripts/dev/scenario-release-chain-v8-high-lab.sh" "$ROOT" prepare EXACT_COMMIT
# Only after independent admission of these prepared source/runtime bytes:
bash "$ROOT/current/scripts/dev/scenario-release-chain-v8-high-lab.sh" "$ROOT" execute EXACT_COMMIT
```

Preparation and protocol review establish no native chain PASS. Acceptance still
requires the five actual Core/Broker cases and independent final evidence review.
