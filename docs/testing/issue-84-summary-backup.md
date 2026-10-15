# Summary consumer actual backup/restore gate

`TestActualPostgresSummarySnapshotDumpRestoreDeletionIntegration` supplements
the generic lifecycle restore test with migration072's copied transcript path.
It passed all six explicit PostgreSQL/race phases (two cases × seed/delete/restore),
with zero failures or skips. This is privacy/lifecycle evidence using a controlled
transport fixture; it is not native model quality or complete provider-cost proof.

## What was restored

Each case uses `newConsumerFixture(t, pool)` with the externally retained private
`BUTLER_SUMMARY_NATIVE_MANIFEST`. A primary message depends on a neutral candidate
whose parent is an original document. A new explicit owner policy captures that
complete original graph; the fixture's prior policy is revoked, never rebased.
The actual durable `ModelWorker` publishes one checked summary and coverage row.

The selected input is an **unparented** catalog source: it has no ordinary
`memory_source_dependencies` parent. Its original message, candidate and document
are pinned through `memory_summary_sources` / `memory_summary_dependencies`.
The result has its normal derivative dependency on that copied input. The test
asserts this topology and nonempty source/coverage before backup, preventing an
ordinary parent/child-only fixture from satisfying this consumer-specific gate.

The driver creates an actual custom `pg_dump -Fc` **before deletion**. One case
then deletes the original message; the other independently deletes the document
ancestor. It restores each old dump into its own fresh database, leaving the
newest external deletion anchor untouched. Catalog access must fail closed until
reconciliation. Replaying that anchor twice must then establish all of:

- The selected input's complete revision history and result text are physically
  erased, their original hashes survive, and Catalog reads deny them.
- The affected primary message and, for ancestor deletion, candidate/document
  are tombstoned. The earlier copied bootstrap input cannot regain access.
- `session_summaries` coverage is removed; old coverage cannot suppress history.
- The published attempt and its observed fixture usage remain present.
- An unrelated source retains its exact body, revision, access revision and hash.

Each phase has a separate process. Private0600 receipts contain only opaque source
IDs, revisions and hashes; no transcript/model text is copied into the receipt.
Backups, receipts, anchors and bind-backed PG data remain private lab artifacts.

## Reproduction

Stage the source under an isolated
`/srv/storage/butler/agents/issue83-<label>/snapshots/`, point `current` at it, then
run on `ssh laba`:

```sh
bash current/scripts/dev/memory-summary-backup-lab.sh "$PWD" fresh-label
```

The driver requires existing Go/PG images and cached modules, sets `--pull never`,
`GOPROXY=off`, `GOSUMDB=off`, and creates its own internal Docker network. It uses an
explicit PG storage bind rather than an anonymous volume. Cleanup removes only
its own container/network and retains evidence/data. No provider credential,
native inference, Redis, main database or another agent's container is used.
`BUTLER_SUMMARY_BACKUP_SUBNET` can select a free lab subnet. Ordinary package runs
skip the external procedure; a skip cannot qualify this restore gate.

## Retained exact evidence

The passing root is
`/srv/storage/butler/agents/issue83-summary-restore-20261016/reports/attempt-v2`.
`complete` contains `PASS`; `summary.json` records6 PASS,0 FAIL,0 SKIP. The source
is root snapshot `9fe07835` plus the added phased test/driver. `go vet` for the
compaction package also passed (`vet.log` is empty).

| Artifact | SHA-256 |
| --- | --- |
| Test source | `0e083f9d651f787f8fda04711905f6cee93e76df61d7748ef07d5248ef59f8a5` |
| Driver source | `9b885689b42e549bcdb54ca844a2303df74b88648a522754fd05fbd62abc0a6f` |
| `source.sha256` | `d3b81c622c41cb6f62d623ed418a92cc4e5c0c3b9093c38e7a0c2a7690b31a4e` |
| `evidence.sha256` | `f0744d7a5e1a516d37786c99ddae9e3fa30a426e7373277272b1d85497bd72ff` |
| Message-case old dump | `53504e7f25d4d7552b4513c521093d090dcfcef3c4937969587b273a4c141f87` |
| Ancestor-case old dump | `f583e7f4166b3b87d67ce2ace1920eeead4312812599152d50f6a3048008ce41` |

The original `attempt-v1` remains failed. Its fixture incorrectly required
`ErrDenied` after deletion, while the copied-original guard returned `ErrStale`.
The correction permits either denial status and still requires physical erasure,
preserved identities and selective retention. It did not alter production code
or replace the original failed evidence.
