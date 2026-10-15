# Issue84 verified task summaries

The consumer is implemented behind a separate default-false process gate. No
primary deployment flags or architecture were changed for this evidence. The
main dirty checkout was preserved; tests use a managed branch and an isolated
PostgreSQL/container root on `ssh laba`:
`/srv/storage/butler/agents/issue84-20261016`.

## Controlled current-source evidence

`reports/guard.txt` passes exact-primary excerpt validation, repeated compaction,
bounded generation failure, threshold/coalescing, late correction/deletion/tool
change, root/derived catalog invalidation, complete original privacy ancestry,
cross-project/Secret rejection and privacy erasure of copied source revisions.
`reports/context-recovery.txt` passes actual scanner→worker recovery after tool
or message revision under the same current finite policy, with reconstructed
worker/store instances.

The same report passes `TestOrdinaryTaskContinuationAcrossRepeatedCompactionIntegration`:
three durable compactions, six ordinary `Orchestrator.Execute` foreground runs,
real PostgreSQL session leases, task store, native-shaped transport contract,
budget ledger and final-result coordinator. Service instances are reconstructed
between rounds. Candidate continuations fit the production default 65536 context
allowance and keep the current typed goal, amended constraints, scoped stop,
negated full stop, owner-completed export and UNKNOWN effect. Baseline uses full
history with a larger allowance. The transport in this controlled test is a
fixture; this evidence alone does not establish Gemini answer quality.

## Retained native window v1

The independent `summary_native_v1` schema retains all three attempt/reservation
rows. Native Antigravity High uses the existing subscription transport via the
server proxy 18080. Actual summary publication and a seven-field baseline
continuation passed. The compacted continuation's third attempt stopped with
`storage_unavailable` and NULL observed usage. Its reservation remains charged.
Known native totals were 5749 for summary and 7230 for baseline, including provider
total/thinking where returned; the third total remains unknown. This window is
partial component evidence, not issue acceptance. It must never be replayed or
erased to obtain a cleaner result.

## Native ordinary-flow comparison

`TestNativeOrdinaryTaskSummaryContinuityIntegration` is an independent one-use
persistent window. It runs the same actual foreground/service composition for
three rounds, with three summary jobs, three full-history baseline runs and three
compacted foreground runs. Baseline and candidate use independent session/task
identities and identical primary input blocks, so neither can read the other
arm's answers. The prospective finite owner-day allowance is 16 calls;
the planned workload is 9. There is no automatic retry/fallback. It retains v1's
three attempts separately, exports complete foreground/background ledger rows,
separate raw native usage totals, scope reservations, schema physical bytes,
answers and serialized context sizes. Subscription no-overage pricing is declared;
provider invoice cost remains unknown. Qualify this section only from its actual
report; creating the harness is not proof of a passing native window.

The frozen 28fd521e v2 execution stopped at its first summary after 8.83 seconds.
No ordinary foreground call ran. The attempt retains `storage_unavailable`, NULL
observed usage and its full conservative quote. The content-free authority probe
trace ends with a 201ms `catalog_guard` deadline and pool Acquired 1 / Idle 0 / Total 1
(MaxConns 2). Earlier probes were mostly 9–15ms, then 116/166/104ms. Connection
contention or expensive full guards remain hypotheses until independent offline
diagnosis; no timeout or security check was relaxed. Across v1/v2, two native
totals are known and two attempts have unknown usage. Production qualification
remains false, and the paired native acceptance criterion remains open.

The unchanged full supervisor also failed the independent synthetic cold/warm
comparison (`reports/warm-pool-diagnostic-fixed.txt`): both modes published round 0
and hit the 200/201ms authority deadline in round 1. Merely warming two connections
was insufficient. The generic worker now uses a metadata supervisor and joins it
before local publication, retaining full per-HTTP/publication proofs. On that
source the ordinary three-compaction/six-Execute PG/race scenario passed in 9.487s,
and the complete consumer PG/race suite passed in 34.808s
(`reports/v3-preflight.txt`, `reports/v3-guard-fixed.txt`). The persistence/CAS
fixture's one-second model deadline was separately replaced with a finite 10s
per-call / 40s four-call allowance; its security supervisor remains 200ms. The
prior 1s timeout report is retained as `reports/v3-guard.txt`.

The new independent one-use `summary_native_v3` window on frozen c1b1aa90 stopped
at its first summary after 33.00s with `storage_unavailable`, NULL usage, and no
foreground request. Its metadata probes were 4–10ms until the last 31ms/201ms
deadline (Acquired 1 / Idle 0 / Total 1). The full quote remains reserved. This is a
separate failure from the earlier full-guard diagnostic; the exact failed SQL
or pool phase was not captured and its cause is not yet proven. Follow-up bounded
SQL-hash/class and acquire/connect tracing has been added for diagnosis, without
recording SQL, arguments, credentials or raw errors. The consumed v3 window must
not be replayed. Its report is `reports/native-v3/{comparison,full-ledger,
native-usage,native-probes,retained-v1-v2-ledger,caps}.json`.

Across all three native windows there are five attempts: two known native totals
(12979 tokens combined) and three unknown-usage attempts with complete conservative
quotes retained. These are different prospective budgets in one persistent
isolated database; a new schema does not reset historical expense. No paired
native quality or full-cost benefit has been established. `ReleaseQualified`
remains false.

## Retained native window v4

The independent v4 window used frozen 996ea0b2 after the per-job full-proof /
periodic-supervisor mutex passed causal old-code-negative and current-code-positive
offline controls plus independent review. Current-source repeated compaction and
ordinary-flow PG/race preflights passed in 6.502s and 8.342s. It completed one
summary and both independent ordinary continuations, preserving all seven required
goal/constraint/stop/owner-done/UNKNOWN/next-action fields. The second summary
stopped after 101.45s total with `storage_unavailable` and NULL usage.

This failure has exact phase evidence: `supervisor_commit` took 201ms, and the
only slow/failed SQL hash identifies literal `commit` (196ms, deadline exceeded,
pool Acquired 1 / Idle 1 / Total 2). No pool acquire or metadata query was recorded
as slow/failing. The precise COMMIT failure is established; its underlying storage
latency cause is not. Before v4, the host root filesystem had recovered from full
to 821MB free; this isolated PostgreSQL uses a bind directory on the separate
storage filesystem. No deadline or source/security predicate was relaxed.

For the completed first round, native baseline total was 8703 tokens versus 5503
compacted foreground plus 9243 summary tokens: the full candidate cost was 14746,
which exceeds baseline despite its smaller foreground context. The second summary
adds an unknown expense with its complete quote retained. Subscription no-overage
is the declared price basis; provider invoice cost remains unknown.

The current v4 window retains four attempts (three observed, one unknown). All
windows together retain nine attempts, five observed native totals (36428 tokens),
and four unknown expenses. Reports are under `reports/native-v4`, including bounded
SQL/pool phase tracing and exact previous-window rows. No full three-round paired
quality/cost acceptance is claimed and the release qualification stays false.

## Consumer-specific lifecycle gate

`reports/lifecycle-eight-fixed.txt` passes all eight shared
`memorytest.LifecycleAttacks` cases through the real PostgreSQL source preparation,
model worker and atomic summary/result publication under `-race`. Both hostile
user/assistant history remain DATA. Current typed task goal/constraints/status/
working state stay byte-identical, unknown tool state stays unknown, and no memory
claim is accepted. Each case also corrects and deletes its original message,
rejects root/derivative retrieval, erases copied source generations, reconstructs
the Catalog reader and verifies that an unrelated source survives. This adds
consumer-specific proof to the generic lifecycle gate; it does not turn the
failed native window into a pass.
