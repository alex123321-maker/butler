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
compacted foreground runs. The prospective finite owner-day allowance is 16 calls;
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
