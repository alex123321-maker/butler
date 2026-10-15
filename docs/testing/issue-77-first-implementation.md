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
