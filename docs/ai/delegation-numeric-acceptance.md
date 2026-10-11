# Selected numeric witness acceptance — 2026-10-12

Issue16 remains open. A bounded production acceptance guard now checks structured
numeric/date child observations against one exact selected original. This is
source-relative extraction verification, not a general adjudicator between
conflicting original sources, a model-voting system or native model acceptance.

The explicit contract source body is
`{"kind":"delegation_numeric_observation_v1","source_ref":<exact SourceRef>}`.
The exact original must occur in the child's immutable selected source list,
independent owner-result evidence and owner decision evidence. It contains only
`observed_value` and `observed_date`. The child final contains the matching
source identity/revision/hash, value, date and `uncertainty:"source_only"`.

The production `DelegationGuards.CheckDecisionTx` path reads the contract and
original through Catalog access in the existing manager transaction and compares
the owning immutable final. Existing owner/task/run/source/hash/access/DB-time,
criteria and action-evidence guards remain required. No model or child acquires
verification authority. Supported observations can pass an explicit owner
acceptance; a persisted owner-verified row cannot override a contradictory typed
final. Denied acceptance leaves the original collected result/report unchanged.
Unverified/unsupported observations can still be collected and explicitly rejected.
Ordinary narrative contracts retain their behavior. JSON object contracts with
malformed syntax, ambiguous opt-in markers or more than 8KiB deny acceptance.

## Actual PostgreSQL observations, with no inference

The tests use ordinary Task/Session/Catalog/completion/action-journal/budget stores
with a real disposable PostgreSQL, pool2 and the race detector. The child final
bodies, action responses, owner proofs and model outcomes are intentionally
synthetic; no model service, upstream HTTP recipient or working credential is used.

- Two saved child finals claim 17 and 23 for the same original/day. Both remain
  unverified until a separate authenticated owner result is persisted. 17 passes
  the production owner API acceptance, 23 fails even with its own owner-verified
  result and then is explicitly rejected. Original result/report references remain
  in decision history; parent and child task statuses stay active.
- Two independent saved finals agreeing on the wrong value both fail. Duplicate
  contract markers, unknown schema versions, an unselected witness, omitted/null
  zero and missing exact source evidence also deny; denied promotion cannot
  rewrite the original collected pointer or decision evidence.
- Strict unit checks cover exact int64 limits, real zero, date mismatch/invalidity,
  source identity/revision/hash mismatch, duplicate/escaped/case-alias keys,
  unknown/narrative/trailing data and incomplete originals. Legacy contracts stay
  outside the numeric schema.

An initial proposed locator discriminator failed an actual PostgreSQL test:
`duplicate_marker` was accepted because JSONB normalized duplicate keys. That
candidate design was discarded. The discriminator now lives in the preserved
contract source TEXT; the same actual PG denial test passes. The initial private
failed report remains retained, not rewritten as successful evidence.

## Reproduction and retained reports

Only `/srv/storage/butler/agents/delegation-conflict-evidence` was used on laba;
the main checkout/deployment/current pointer, credentials, sealed native history
and signed corpus were unchanged. Sync this checkout to a new isolated agent root
with `scripts/dev/lab.ps1 -Action Sync -RemoteRoot <root>`, then run:

```sh
bash <root>/current/scripts/dev/delegation-attempts-lab.sh <root> numeric
bash <root>/current/scripts/dev/delegation-attempts-lab.sh <root> verification
```

The numeric mode runs the package/parser checks first, then actual conflicting
and malformed-contract PG cases. Verification additionally covers owner decisions,
legacy verified promotion, transport failure classification and the owned audit.
Both modes run affected vet checks. Full mode covers the wider existing delegation,
converter, parent-context, owner API and transport suites; it performs no inference.

| Check | Snapshot | Immutable events SHA256 |
| --- | --- | --- |
| Initial rejected JSONB marker design |20261012-024242-738|`b3b8fb68a03f3bfde677bad8f12102e535fbd078e52f24f8304604ad71587f33`|
| TEXT marker numeric PG/race/vet |20261012-024514-114|`b9e48c9110fa37995f045d5b4611b24389bf038a8ae38af43a674d20fb913530`|
| Final-code verification PG/race/vet |20261012-024634-906|`f472ff917071db11ee7f6d5e5c1057f8b9f5483b7668dc982e16fef3f76777c1`|
| Final-code full delegation PG/race/vet and shell syntax |20261012-024822-644|`2e393600db90cd7187e6dd3171fb5e2644e5a84d9d9b0163461ba0b1e76258a9`|

Verification retained 88 distinct test/subtest names across 5 packages, 18 top-level
tests, 0 failures and 0 skips. Full mode retained 524 passed distinct test/subtest
names across 7 packages, 297 top-level tests, 0 failures and 7 explicit skips.
The skipped checks require separately launched actual Broker/converter harnesses
(3 tests) or the other PostgreSQL/Redis integration environment (4 tests).
They are not passed evidence; the selected numeric/owner/legacy PG gates all ran.
Reports are under this agent root's `reports/`; PostgreSQL data and the initial
failed report remain there. New model calls 0.
All prior native attempts and unknown token usage remain unchanged.

Full successful native scheduling/recovery, same-source single/parallel quality,
latency and complete measured usage, general contradictory-original research and
local generative-context quality remain independent gates for issue16.
