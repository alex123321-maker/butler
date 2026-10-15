# Issue77: first implementation block

This branch implements the next dependency block, issues83–86, on top of main
`cc751b2c2aef446ef8fffaad357cf3a57892e66a` (the merged issue82 implementation).
It does not close the umbrella roadmap. Adaptive retrieval87, temporal memory88,
consolidation89, verified procedures90, scale/cost91 and staged activation92 remain
separate subsequent work. Optional research93/94 and deferred local-model27/28
are outside this block.

## Result and boundaries

| Issue | Implemented behavior | Evidence and remaining gate |
| --- | --- | --- |
| 83 | Shared eight-case poisoning/withdrawal/deletion corpus; current source and ancestry checks across jobs, publication, caches, retrieval and restore | Real PG/race, actual Broker and Redis; actual old `pg_dump` restore with newest independent deletion anchor. [Kernel evidence](issue-83-memory-lifecycle.md). |
| 84 | Versioned extractive task compaction, primary-event coverage CAS, fresh tail, typed current constraints/tool outcomes, finite explicit owner policies, copied-source privacy and restore | Controlled ordinary Orchestrator continuation and repeated restart pass; [actual copied-summary restore](issue-84-summary-backup.md) passes. Native repeated-continuation/cost gate is separate; code-owned qualification remains false pending it. [Native evidence](issue-84-verified-summaries.md). |
| 85 | Native-provider candidate consumer with exact Unicode citations, scoped/temporal unknowns, atomic publication, owner acceptance, typed episode projection, dedup and correction/revoke | Real HTTP/PG extraction, owner decisions and Catalog eligibility pass. Ordinary scoped-context use and actual episodic backup/restore have separate required checks. Fixed Russian core and independent metadata holdout must complete with their original criteria before code-owned qualification. [Consumer contract](../../internal/memory/candidates/README.md). |
| 86 | Optional `memory.search`, `memory.open`, `memory.neighbors` through the actual Tool Broker and private reader; immutable input checkpoint, current lease/task/grants/budget/source proofs | Actual production Broker and native High foreground question pass, with original-source answer verification. This is a bounded native example, not broad answer-quality acceptance. [Native evidence](issue-86-memory-read-native.md). |

All process flags remain false by default. Summary and candidate consumers also
require code-owned qualification; environment flags cannot bypass an unpassed
gate. Read tools remain DATA, create no task/mandate and retain Broker policy,
grant, journal and budget enforcement. Owner acceptance is an explicit separate
action. PostgreSQL remains durable truth, Redis transient, pgvector derived.

The native adapter now accepts trusted operator output-ceiling configuration.
Default0 preserves8192 generated tokens; the summary qualification profile needs
High32768 and matching foreground/background output reservations. Thinking and
visible output share that ceiling. Model jobs use a fixed maximum six-minute
claim and configurable call duration capped at five minutes; earlier authority,
policy and budget deadlines win. Other job kinds keep two-minute claims. The
250ms supervisor with200ms proof deadline, fencing and no-refund accounting stay
in force. Abandoned model claims can therefore take six minutes to recover.

Native testing exposed self-contention in the background authority observer.
The periodic proof now checks complete current metadata without source-body
loads/publication locks, yields while the same job performs mandatory full
pre-HTTP proof, and ends its read-only transaction by bounded rollback. Full
network and publication checks/commits remain. Known usage is saved before output
validation; absent usage stays SQL NULL and charged reservations survive failure.

## Reproducible stand

Work is isolated under `/srv/storage/butler/agents/` on `ssh laba`, using immutable
source snapshots and storage-backed PostgreSQL binds. The dirty primary Windows
checkout and main deployed configuration are preserved. The main Orchestrator,
PostgreSQL and Redis remained healthy during these runs. Main health does not
qualify any new consumer or prove provider readiness.

Internet traffic, including GitHub and native subscription inference, uses the
server proxy at `127.0.0.1:18080`. Offline checks use cached Go/PG/Redis images,
`--pull never`, `GOPROXY=off` and `GOSUMDB=off`; no local internet fetch is needed.
Encrypted auth moves only through private server files; neither credential
material nor provider response errors are included in public observations.

The root drivers are `scripts/dev/memory-consumers-lab.sh` and
`scripts/dev/memory-lifecycle-lab.sh`. The additional copied-summary backup driver
is `scripts/dev/memory-summary-backup-lab.sh`. Required parent tests must pass and
cannot be replaced by package success or an optional skip. Each native window
seals its source/binary/corpus/finite allowance in advance and retains every prior
attempt, unknown usage and reservation. A new window never resets paid history.

One synthetic root lab PostgreSQL volume exhausted the host root filesystem and
crashed. Its failed logs, inspection and data archive remain private evidence;
that run is failed. Only that lab's own anonymous volume was removed after archive,
and subsequent lab data uses `/srv/storage` binds with CPU/memory/WAL bounds.
This does not establish the cause of earlier unrelated native storage failures.
No main data or unrelated containers were cleaned up.

Exact final combined and native totals are recorded with their source manifests
in the issue-specific reports. Partial native windows and optional skips do not
qualify missing acceptance criteria. No main consumer activation or umbrella77
acceptance follows from this implementation branch.

## Combined validation receipts

The consumer and kernel `combined-final-v5` snapshots are source `701d225a`.
They include the six-minute claim/five-minute quote and the complete consumer
implementations, reader metadata/redaction and large-original tail fixes.

| Isolated run | Result | Scope |
| --- | --- | --- |
| issue77 `combined-final-v5` | 296 PASS, 0 FAIL, 5 optional SKIP | Consumer packages/composition, actual production Broker and vet |
| issue83-combined77 `combined-final-v5` | 232 PASS, 0 FAIL, 4 optional SKIP | Lifecycle kernel, Redis, Broker and actual catalog dump/restore |
| issue83-summary-restore `attempt-v2` | 6 PASS, 0 FAIL, 0 SKIP | Actual copied-summary dump/restore, separate message/ancestor cases |
| issue77 `model-duration-v2` | 103 PASS, 0 FAIL, 1 optional SKIP | Source `a39944d8`: real `ProcessOne` provider deadline, earlier caller, authority supervisor, config/factory and vet |
| issue77 `utc-window` | PASS | Source `cf5bc323`: same deadline proof also honors actual UTC-day scope expiry |
| issue77 `scoped-context-v2` | 4 PASS, 0 FAIL, 0 SKIP | Source `5253a233`: owner-confirmed extracted preference in ordinary scoped foreground context; independent transcript-redaction/source-deletion regressions and vet |

These are test/subtest events, not independent task-success samples; overlapping
checks must not be added together as unique tests. Optional native, legacy and
external backup procedures are explicit skips, with their applicable gates
checked separately. Native semantic quality/cost remains separately measured.

The additional broad composition run (`composition-v1`, source `a39944d8`)
finished with1050 passing events,140 skips and16 failed test names. One failure
exposed an old converter test expecting a deleted tool event to leave the old
transcript pin usable. The fixture now requires the unchanged runtime fence to
deny that stale run; source deletion is tested in an independent current fixture.
Both corrected branches passed in `scoped-context-v2`.

The other15 failures are closed release-chain history/bootstrap checks. The exact
same15 names fail on unchanged main `cc751b2c` in the same isolated environment
(`baseline-history`,0 PASS/15 FAIL/0 SKIP). Their predicates/artifacts were not
changed or waived. This comparison separates them from the new memory regression;
it does not claim the whole repository suite is green or establish those older
release acceptances. The raw failed broad run and baseline remain retained.

Content hashes of the exact retained receipts:

| Receipt | SHA-256 |
| --- | --- |
| Consumer `combined-final-v5/evidence.sha256` | `af677241f5862f939fe1d0e524af90950f01914cc52399cee0e85e9e05d43902` |
| Kernel `combined-final-v5/evidence.sha256` | `b61b09b492c95d23cd4c0e8824bde776cb971d5743882a6bdd997b03c87eccf9` |
| `model-duration-v2/tests.jsonl` | `b6b747a9fcae3d3853c9f8eed43e5983a776384761a1a0f54d18732a54873a9b` |
| `scoped-context-v2/tests.jsonl` | `357d961b3860e30b36d7d26bd897c9d8a5fb01b3f4fbd54d0040c0a2ec1fdd21` |
| Failed broad `composition-v1/tests.jsonl` | `19c962e71a244517735b7427fac598a09893d995e367338217b91af1fe02ae27` |
| Unchanged-main `baseline-history/tests.jsonl` | `133223ae3bb725300c0576bfb873f6696be827b3d92eb6878a7c5a290e7912a8` |
