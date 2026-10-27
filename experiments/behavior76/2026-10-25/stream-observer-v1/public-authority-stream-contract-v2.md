# Prospective authority observer stream representation, v2

This is an additive public evidence contract for a future, separately frozen
study. It does not reopen the closed local-v4-r2 window, authorize another native
call, amend a quality criterion, replace its hidden controls, or change a prior
UNKNOWN to success. Original flat-v1 traces, failures, costs and seals remain
immutable. Original denominators remain 24 episodes, 48 arms, 96 evaluated owner
opportunities, 12 positive/negative scenario families. Automatic continuations
and their effects remain separate actual evidence.

The closed local-r2 boundary receipt is
`public-closed-local-r2-boundary-v1.json`, SHA256
`e4997e2b0ae724f420ef82bb520e15ad0f893d272be34d0d5d273bea97ba6738`.
Its primary GLOBAL fault was the old observer capacity, not an established SQL
deadline failure. It has 5/48 recorded arms and 10/96 owner opportunities; the
remaining 86 opportunities are unavailable. Its admission is consumed.

## Terminal index and independently pinned identity

Each case retains `authority-trace.json` as a small UTF-8 JSON terminal index:

```json
{
  "version": "behavior76-authority-pg-trace-stream-v2",
  "sealed": true,
  "event_count": 2,
  "last_sequence": 2,
  "maximum_buffered_events": 4096,
  "maximum_segments": 16384,
  "maximum_compressed_bytes": 536870912,
  "total_compressed_bytes": 123,
  "identity": {
    "window": "actual-window",
    "arm": "slot-01/actual-masked-arm",
    "schema": "actual-isolated-schema",
    "producer": "actual-source-commit",
    "protocol_sha256": "actual-protocol-sha256"
  },
  "segments": [{
    "path": "authority-segment-000001.jsonl.gz",
    "first_sequence": 1,
    "last_sequence": 2,
    "event_count": 2,
    "compressed_bytes": 123,
    "uncompressed_bytes": 456,
    "sha256": "compressed-file-sha256",
    "uncompressed_sha256": "exact-jsonl-bytes-sha256"
  }],
  "tail": [],
  "open_spans": 0,
  "dropped_events": 0,
  "operation_failures": 0,
  "observer_failed": false
}
```

Numbers and placeholder hashes in this public example are illustrative, not a
valid fixture. All specified properties are required, with exact case and no
duplicates, case aliases, or additional properties. No legacy `events` array is
silently substituted. The index is at most 16 MiB. Its complete bytes and all
segment bytes are covered by the immutable exact case inventory; the independent
reader also requires the pinned index SHA and expected actual identity.

Segments have consecutive six-digit names starting at 000001, each 1..4096
events. The writer synchronously creates each file exclusively, completes gzip,
syncs/closes and hashes it before publishing its descriptor. It flushes any final
partial batch before sealing, so the actual sealed writer normally has an empty
tail. The generic reader accepts a tail of fewer than 4096 original events,
following all segments, with the same correlation/sequence rules. Segment count
is at most 16384 and compressed bytes across all segments at most 512 MiB. No
asynchronous unbounded queue or silent lossy capacity reset is permitted.

Each decompressed segment is exact UTF-8 JSONL: one real event per nonempty line,
LF after every event, no CR or BOM, at most 8192 UTF-8 bytes excluding LF. Its
uncompressed length is consequently at most `4096 * 8193`. Compressed and raw
lengths and SHA256 values must match exactly. Relative paths must stay within
the case; absolute paths, empty/dot components, backslash/colon and reparse
points are denied. Descriptors cannot name a foreign or repeated segment.

## Original safe events and independent reconstruction

Events retain the original safe fields: sequence, span, phase, edge, at_utc,
elapsed_us, original_context_deadline_utc, original_context_time_left_us,
query_family, outcome, context_cause, pool, and optional
public_sql_template_sha256/sqlstate. Existing fixed family/error/SQLSTATE
allowlists apply. No SQL text, arguments, returned rows, connection string,
credentials or free-form error messages are added. Property names are exact and
unique, including nested pool metadata. The observer never changes SQL,
arguments, return values, pool topology, the original 750 ms authority context,
model/candidate instructions or budgets.

Reader consumes segments and then tail in order. Event sequence is exactly
1..N with no gaps, duplicates or synthetic endpoints. New start span IDs are
allocated exactly 1..S, as in the original mutex-protected recorder. End events
must match a currently open span's ID, phase, family and public-template hash.
Completed spans need not be stored indefinitely: monotonic start IDs prevent
reuse, and only the active map is retained, with at most 4096 active spans.
Starts have pending outcome/zero elapsed; ends have a recognized actual outcome.
Every non-ok end increments operation_failures. Time metadata is validated but
wall-clock UTC is not required to be monotonic; a wall-clock adjustment must not
invent a missing end or change an actual error.

The implementation reads compressed data incrementally with a 64 KiB buffer,
uses an at most 8 KiB line buffer and an at most 4096-entry active-span map. It
does not accumulate all event objects or all decompressed segments. Memory for
the bounded terminal index/descriptors is separate and at most 16 MiB input.

Qualification requires sealed=true, reconstructed event/sequence/error counters
exact, no open spans, no dropped events, observer_failed=false, and exact sealed
inventory. Seal occurs only after actual fixture/worker/Broker/runtime
quiescence. An event after sealing marks the trusted shared GLOBAL stop; a
previously serialized healthy header cannot excuse a later observed event.
Recorder failure, failed write/sync/hash, cap overflow, malformed/missing
segment, correlation error or uncertain quiescence remains GLOBAL/incomplete.
The reader does not manufacture success for a failed/unsealed trace.

Actual SQL/acquisition errors remain recorded with their exact safe categories
and count. Ordinary cancelled-query/rollback errors alone are not newly promoted
to an observer fault: the existing public driver regression intentionally
records two such errors. Existing source/authority/kernel failure handling
remains authoritative and GLOBAL where already required. There is no cleanup
amnesty, timeout increase, manual endpoint injection or observer-only PASS
substitution.

## Additive scoring composition

`authority-stream-reader-v2.cs` provides the read-only event validator;
`authority-stream-reader-v2.ps1` is its PowerShell entry. Failure messages contain
fixed categories only. `authority-stream-scoring-v2.ps1` is the new entry
`Invoke-Issue76StreamScoringFiles`. It pins and validates the stream before
calling the original `Invoke-Issue76LocalScoringFiles` verbatim.

Frozen old files are unchanged:

| File | SHA256 |
| --- | --- |
| score-mechanical-fresh-v3.ps1 | e481f9c062a3bc4500b9f0b94da8819fbf287bec075490583cad17d54198721d |
| local-tail-adapter-v1.ps1 | 8975943bd13c617557505eb8eb0536b23d6570e2568f2b09765c0fce52f142bf |
| local-scoring-composition-v1.ps1 | 5cb74cc78c66a6a8d30397f132d3b59fb0f455a2face28dbd1cb266fde873ec2 |

The old adapter accepts the same quiescent scalar fields, while independently
checking the complete case inventory. Therefore it needs no replacement. No
fake v1 trace/snapshot/index is generated. Original exact criterion functions
and 27 criterion AST statements, all availability/denial distinctions, remaining
96 opportunities and separate quality/cost/effect dimensions remain unchanged.
The new representation validator cannot turn critical UNKNOWN into PASS.

The reader and wrapper are frozen before new hidden authorship. Public selftests
use only explicitly public synthetic operational fixtures, with no PG, provider,
credentials, old/new mappings or hidden cases. Existing gates and a separate
independent admission are still required for any future native window.
