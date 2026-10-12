# Future complete tool-turn usage, scoped issue33 fix

The native issue15 experiment completed four requests, but its first three tool
turns remained NULL in the usage journal. The code path explains the future
wiring defect: the native SSE parser retained `modelAnswer.Usage`, the provider's
tool branch discarded it, and the owning budget wrapper read only text-final
usage. No old raw responses are available here to reconstruct their measurements.
Historical reports remain immutable: 45 attempts, 20 known and 25 NULL, known
input/output subtotal 74698/2210. This patch neither backfills them nor claims
their upstream values.

The direct logical Go `TransportEvent.Usage` now carries one complete request
observation for tool proposals, batches and complete terminal outcomes. Legacy
`AssistantFinal.Usage` remains compatible. Antigravity attaches native complete
usage to its tool proposal and safe invalid-proposal error without granting
execution. Complete text finals keep their previous field plus the logical event
field. No new RPC, schema migration, price, budget refund or model authority is
introduced; generic `ObserveModel` remains unchanged.

The budget wrapper validates nonnegative finite counts, overflow and total
consistency before persistence. It records one observation per admitted operation,
accepts identical legacy/event/terminal duplicates, rejects conflicting duplicates
without overwriting the first known measurement and stops allowance overruns
before tool or final release. Earlier streaming deltas are not rewound by a later
complete usage observation. An unexpected batch still has no execution permission;
its request usage is not multiplied by its number of proposals.

Native parser counts use exact keys. Case/Unicode aliases cannot overwrite counts
or the usage object. Missing/partial metadata, provisional nonterminal snapshots,
incomplete SSE and cancellation before a complete observation remain NULL. After
a complete terminal response, identical usage-only trailers preserve one
observation; a conflicting trailer fails protocol validation before a tool is
released. A terminal with no metadata followed by its first complete usage-only
trailer is supported for both Tiered and High. Already recorded complete request
usage survives later tool failure/cancellation as historical measurement; it grants
no further request or external effect.

## Owning zero-inference evidence

All checks ran through SSH laba in the separate
`/srv/storage/butler/agents/tool-turn-usage` root and disposable internal-network
PostgreSQL. Provider HTTP/SSE fixtures use fake in-process transports or local
test servers only. Native requests, auth/import/profile reads, shared stand
changes and history backfills: zero.

- Snapshot `20261012-161153-048`, all mode:
  `reports/tool-turn-usage-20261012T091222Z-834887/tests.jsonl`, 87 passing names,
  32 top-level tests, zero failures/skips, race and affected vet. Includes all
  common transport/Antigravity tests, actual PG wrapper cases and existing current
  source-guard/Core happy path.
  SHA-256 `e81ce8387e770879929faed3d696cdd4db1c1d25834aeaa5dfd45f31deb8118c`.
- Independent review then found the conflicting complete SSE trailer overwrite.
  It was reproduced as a two-frame provider regression and fixed. Final snapshot
  `20261012-162019-784`, narrow parser/PG mode:
  `reports/tool-turn-usage-20261012T092040Z-865313/tests.jsonl`, 38 passing names,
  six top-level tests, zero failures/skips, race/vet.
  SHA-256 `286b34de2532d0769bc3fcc373df7b4ca3601efe28cdeef0a82425aa562475a6`.

Exact owning tests:

- `TestToolTurnUsageSurvivesTieredAndHighContinuation` — synthetic native tool
  request, failed-tool result and next final retain one usage per request.
- `TestToolTurnUsageMissingIncompleteAndInvalidRemainUnobserved` — missing,
  partial, provisional, aliased, negative, overflowed, inconsistent and incomplete
  metadata stays unavailable or fails closed before a proposal.
- `TestToolTurnUsageOnlyTrailerPreservesOneConsistentCompleteObservation` —
  Tiered/High identical, conflicting and first late complete trailer cases.
- `TestCompleteInvalidToolProposalRetainsUsageWithoutGrantingExecution` and
  `TestCancelledIncompleteProviderTurnHasNoObservedUsage` — complete invalid
  function error retains measurement; cancelled incomplete request does not.
- `TestModelTurnUsageOwnsOnePGObservationAcrossToolsFinalsAndFailuresIntegration`
  — 14 actual PG cases, one operation/reservation, known/missing usage, tool/batch/
  complete error, legacy/duplicate/conflicting metadata, invalid/provisional
  events, cancellation and current source denial. Every case independently checks
  the journal and unchanged conservative allowance, then denies another request
  under stale task authority before provider entry.

This establishes future usage wiring and its accounting boundaries. It does not
complete the broader issue33 intelligence criteria or reopen any sealed native
acceptance window. Any future real High comparison is separately owned and bounded.

Followup independent review found that typed Cloud Code wrapper decoding could
replace `response` with a case/Unicode alias containing different valid counts.
The parser now rejects these aliases before decoding the wrapped response.
`TestToolTurnUsageHighWrapperAliasesCannotReplaceCompleteResponse` demonstrates
canonical+`Response` and canonical+long-s aliases cannot release a tool or usage.
Snapshot `20261012-162831-202`, network-none targeted race/vet proof
`reports/usage-wrapper-alias-20261012T092925Z.jsonl`: 11 passing names, three
top-level cases, zero failures/skips, including the canonical High continuation
and all trailer cases. SHA-256
`60d2226d347bd8dc30d845cabe92866bb4e474bf7d17861b1c452a9ceb4a518d`.
