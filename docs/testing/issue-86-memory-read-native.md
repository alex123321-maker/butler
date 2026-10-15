# Issue #86 native memory read evidence

The isolated full-product acceptance passed on `ssh laba`, using the actual
Orchestrator, Tool Broker, PostgreSQL and existing Antigravity subscription
transport with `gemini-3.8-flash-high`. The server recorded the successful run at
2026-10-15 18:26:40–18:27:07 UTC. All provider traffic used the dedicated proxy;
the main deployment and its task/memory database were untouched.

## Exact acceptance

Production code and binaries came from `2a2b99f7ddea70280103ff9f1b31a065321ba147`.
The reproducible topology and driver are
[`deploy/memory-read-native`](../../deploy/memory-read-native/README.md) and
[`memory-read-native-lab.sh`](../../scripts/dev/memory-read-native-lab.sh).
The retained evidence root is
`/srv/storage/butler/agents/issue86-native-20261016/reports/native-v2`.

The owner input asked for the exact calibration code in the current original. It
contained neither the code nor a source ID. A synthetic 3,157-character source
put `МАЯК-742/33` after its compact search snippet. Without automatic memory
retrieval or a scripted provider, the model performed three `memory.search`
calls, selected the document from the third result and called `memory.open`
with that result's exact source ID, source revision, access revision and content
hash. Search results never contained the code. The open result contained it;
the final answer returned the correct code and identified the source/revision.

All four calls completed through Broker with `effect_confirmed` read effects,
durable evidence references and source pins. Results remained `DATA` with
`current_revision_unverified_fact`; the final result verdict remained
`unverified`. Successful retrieval did not promote the document to authority.

All five new model turns have observed durable input/output counts: **29,328
input tokens and 340 output tokens**. This foreground ledger does not capture
provider total or thinking counts, and its monetary invoice cost is unknown on
the subscription basis. These counts are not a full Google token/cost total.
The ordinary task and owner-day allowances admitted the calls. No provider
fallback, test-specific three-call cap or reservation refund was used.

## Retained failure and conservation

`native-v1` remains a separate failed window. Its first admitted model attempt
stopped with `provider_protocol_error` before any tool call. Offline preflight
reproduced the tool declaration's unsupported numeric/string bound annotations
at `convertTools`. The reviewed fix lowers these known annotations only in the
provider-facing native tool declaration. The original Broker schema, schema
hash and runtime checks remain intact; unknown keywords and the structured
response schema dialect stay closed.

The successful window used the owner control API to resume the **same** blocked
task and root budget. The original attempt row is byte-for-byte equivalent in
the before/after JSON ledger, with SQL NULL observed usage. Its expense was not
erased or reinterpreted as zero. The retained ledger therefore has six admitted
model attempts: five observed and one unknown. Conservative reservations total
6,340,608 tokens across those six attempts. A supplemental read-only SQL audit
confirmed every model attempt has its matching reservation and all 16 scope
dimension totals equal the sum of all ten model/tool reservations.

Provider refreshes were serialized with the other native labs. The exit trap
stopped this Orchestrator and exported its latest encrypted state through a
private file reference. Subsequent consumers own later rotations; do not copy
this lab's older return over their state or main auth.

## Evidence hashes

`complete` contains `PASS`. The manifest and supplemental audit hashes are:

| Artifact | SHA-256 |
| --- | --- |
| `source.sha256` | `cf8458275f26ad4fa9fb1df653f645e0ca7dfc918f5305f054da37448b519f3c` |
| `ledger.json` | `7248c90ee5ff59018371ebb069fda7f30e372ef83cf043b79e717b420044047d` |
| `evidence.sha256` | `0604cc72022eedf44502789f4296a0766d193aefd6b59d21aaf953855a318ac0` |
| `budget-audit.json` | `3c9129607061ff4eb809b1308ba3bcafef435b0b40765833a2fb4e02c7814eb4` |
| `budget-evidence.sha256` | `1862ff57624f9fccfbf6a28264766839a4caf2158c633bdfcf3d1c3e349c3a11` |

Offline validation of the transport fix passed the complete Antigravity and
read-control suites with `-race`, vet and production app builds (183 PASS
events, zero failures). One optional actual memory-job native adapter test was
skipped because that separate PG fixture was not configured. This native
foreground acceptance did execute the real provider and full product route.

The reader-specific shared eight-attack corpus also passed against actual PG
under `-race` (nine PASS events, no skips/failures), from
`98a271e91292816957ea819b2b94a2344be555be`; evidence is
`reports/reader-lifecycle-fixed.jsonl` under the same lab root. Its corrected
source handles fail stale, deleted handles are denied after store
reconstruction, exact DATA body/source pins survive reads, and no claim or
authority is minted. The wider restore/cache/late-publication gate is recorded
in [the lifecycle acceptance](issue-83-memory-lifecycle.md).

This is one controlled native read acceptance plus the exact offline lifecycle
tests. It establishes neither general answer quality nor acceptance of the
other memory consumers.

## Subsequent metadata boundary review

After the native window, an independent review found a reachable metadata path:
ordinary `messages` ingestion leaves an imported message ID out of normalized
text but uses it in its exact fragment key. Credential-bearing IDs could thus
bypass text masking. Publication now rejects these keys and titles over2048
bytes. Reads suppress pre-existing unsafe keys with `truncated=true`, deny
incoming unsafe handles and retain exact usable ordinary handles. The redaction
flag now includes recognized title-only credentials in search/open/neighbors.

The full ordinary ingestion/PG regression fails on the preceding code and
passes after the fix; direct legacy SQL rows exercise read defenses separately.
Retained `reports/metadata-baseline.jsonl` is the expected negative control;
`metadata-narrow.jsonl` passes. The complete catalog/reader/ingestion/sanitize
suite with `-race` passed65 test/subtest events, zero failures, with one separate
actual backup/restore test skipped because its phased fixture was not configured
in this runner. That restore scenario has its own earlier lifecycle acceptance.
`metadata-vet.log` is clean. This follow-up used no provider calls; the earlier
native window remains tied to its exact original source/binary manifest.
