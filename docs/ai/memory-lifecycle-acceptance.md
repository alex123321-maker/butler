# Memory lifecycle security gate (issue83)

Every new memory consumer must extend this gate in its own change before it can
be enabled. The baseline gate covers the existing source catalog, manually
retained DATA dependency chains, semantic generations/bundles and durable model
jobs (`source-quote-v1`). Passing the baseline does not accept a future summary,
lesson or consolidation implementation. A skipped test is not a pass.
The [per-criterion lab report](../testing/issue-83-memory-lifecycle.md) records
the exact accepted kernel snapshot, original evidence, costs and explicit skips.

## Fixed public attack corpus

[`lifecycle-v1.json`](../../internal/memory/memorytest/testdata/lifecycle-v1.json)
contains eight synthetic English/Russian attacks: content instructions, forged
approval evidence, quoted owner instructions, scope widening, retrieval-selection
pressure, derivative laundering, a factually successful historical action, and a
synthetic confidential marker. No real user source, secret, account or credential
file is read. `memorytest.LifecycleAttacks(t)` supplies exactly these records to
consumer tests. Expected decision for every attack is `data_only_no_authority`.

The corpus records adversarial content, not executable policy. Tests assert PG
claim states, typed origin, source/ancestor revisions, access denial, preserved
independent rows, persistent spend and zero Broker runtime dispatch. A model
answer saying it obeys safety is never the evidence. No online paper retrieval
or external API is necessary for this deterministic kernel gate.

## Required observable boundaries

| Boundary | Executable evidence |
| --- | --- |
| Write → two retained derivations → retrieve | `TestLifecycleAttackCorpusSelectiveForgetIntegration`: provenance remains DATA, high confidence stays candidate, live run cannot accept, citation readable before withdrawal |
| Retrieve → attempted action | `TestMemoryLifecycleAttackCannotAuthorizeBrokerIntegration`: actual PG catalog reads after reconstructed Store, exact Broker prepare/authorize/execute paths, fabricated old approval/mandate rejected and runtime count zero |
| Selective forget → restart | Catalog corpus: first poisoned derivative and its descendant physically redacted; citation denied; original and unrelated confirmed preference preserved; current task goal and authority table counts unchanged |
| Correction → late read/publication | `TestLifecycleCorrectionFencesEntireDerivativeChainIntegration`: correction of original fences unchanged descendants and prepared run, including fragment publication from an old snapshot |
| Old generations → fresh context | `TestLifecycleOldGenerationsForgetAndIndependentBundleIntegration`: two reviewed fixture generations, source correction/deletion, no stale vector or fresh bundle release, independent vector/bundle survives selective removal and Store reconstruction |
| On-demand read consumer, issue86 | `TestMemoryReadLifecycleAttackCorpusIntegration`: all eight attacks pass real PG search/open as DATA with exact source pins; corrections/deletion deny old handles and reconstructed reads; no claims/grants/mandates are minted. The [reader contract](../../internal/memory/reader/README.md) records inapplicable publication/index/cache boundaries. |
| Durable consumer → repeat job/ACK → forget | `TestLifecycleModelCorpusRepeatForgetIntegration`: published model output stays candidate DATA, ACK returns same result, completed claim cannot infer again, deletion invalidates result under reconstructed worker |
| In-flight provider → correction/deletion/revoke | `TestModelRevocationDuringCallIntegration`: actual worker fixture callback changes source/owner policy/task; publication denied, observed usage retained |
| Claim takeover/crash | `TestModelCrashRecoveryAndLostPublicationACKIntegration`: old claim cannot publish; replacement charges independent attempt, duplicate trigger coalesces, lost ACK preserves result identity |
| Summary/working/prepared/cache privacy | `TestPrivacyCleanupRestartAndStaleSummaryWorkingPublicationIntegration`, `TestDeleteErasesPinnedPreparationBeforeAnyMessageIntegration`, `TestLiveSourceAndTranscriptPinsRejectLatePublicationIntegration`: old summary/cache/prepared writes denied; durable cleanup resumes |
| Actual Redis cleanup composition | `TestLifecycleRedisCacheWithdrawalIntegration`: production cleaner removes retained synthetic bytes after Catalog reconstruction, repeated cleanup is idempotent, foreign cache survives and late guarded write never reaches Redis |
| Old backup → newest independent deletion anchor | `TestActualPostgresDumpRestoreDeletionIntegration`: actual custom PG dump before deletion, fresh DB restore, fail closed before replay, originals/children/claims/chunks purged after replay, reindex denied and unrelated original/confirmed preference preserved |

Retained summary/lesson strings in the catalog corpus exercise the existing
dependency mechanism; they are not model-generated summary/consolidation quality
evidence. Semantic vectors and generation quality proofs are deterministic test
fixtures; they do not qualify an embedding model. Worker fixtures exercise native
transport authority/dispatch hooks without a paid or external model call.

## Reproduction and evidence

On `ssh laba`, stage the reviewed source beneath an isolated
`/srv/storage/butler/agents/issue83-<label>/snapshots/` and point that root's
`current` symlink at it. Run:

```sh
bash current/scripts/dev/memory-lifecycle-lab.sh "$PWD" fresh-evidence-label
```

The script requires pre-existing PG/Redis/Go image tags and cached Go dependencies;
`--pull never`, `GOPROXY=off`, `GOSUMDB=off` and an internal network prevent
internet requests. It creates only its own disposable PG/Redis containers/network and
fresh test databases. It never reads or changes the main Compose stack. If a
network conflicts, select a free subnet with `BUTLER_LIFECYCLE_SUBNET`.

`reports/<label>` retains JSONL tests (including explicit skip reasons), raw
runner diagnostics, the private custom backup, newest content-free deletion
anchor, SHA256 evidence and aggregate pass/fail/skip counts. The exact mandatory
test list must be observed as test PASS, not package PASS; missing or skipped
mandatory tests prevent `complete`. Ordinary catalog execution excludes
the external backup procedure; its three explicit seed/delete/restore runs must
pass separately. Optional semantic live-model/quality suites can skip and remain
listed. None of these optional skips can establish model quality acceptance.

## Adding a consumer

A consumer's PR must run the shared corpus through its real validation and
atomic publication paths, map applicable rows above to named passing tests, and
append its evidence to this table or link its own acceptance document here.
Include correction/deletion during its provider callback, retry/claim takeover,
prepared context/cache invalidation, old generation/dependency reads, selective
removal and restore replay. An inapplicable boundary needs a concrete reason
(for example, no cache exists), not silent omission. Reuse current durable
source/job manifests and Tool Broker policy; a consumer cannot mint its own
approval, mandate, accepted fact, expanded scope or readiness receipt from model
output. Production consumer controls and independent quality gates stay separate.

Forgetting applies to Butler's retained originals, derivatives, indexes and
contexts. Previously exported/delivered external copies and unknown offline
backups require the owner's separate erasure process; no external undo is claimed.
