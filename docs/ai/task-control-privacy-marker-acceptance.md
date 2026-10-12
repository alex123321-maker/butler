# Owner controls after privacy deletion

Baseline: `7e2ad77`. This component check uses real disposable PG17, every
migration, two connections per pool, actual Catalog deletion, PostgreSQL ownership,
task Controller and permanent action journal APIs. Run creation and runtime
evidence are explicit fixtures; this is neither model nor external Broker evidence.

`TestOwnerControlsAfterSourceDeletionIntegration` first pins an original to a live
run and saves private working state. Actual owner deletion clears working content.
Publication from that still-live worker then fails with SQLSTATE40001. Owner Stop
and Amend must succeed while keeping the content-free deletion marker, advancing
revisions and retaining confirmed action evidence. Amend queues a content-free
continuation; a new lease attaches the current task revision, pins the current
transcript and passes the source guard, but cannot access the deleted original.

The production change preserves only the exact empty plan/pending JSON and its
same-owner existing deletion marker. All other working state takes the existing
clear path. No migration, publication guard or redaction authorization changes.

Isolated lab root: `/srv/storage/butler/agents/task-control-privacy-marker`.
Reports are append-only test evidence; inference count is zero throughout.

| Check | Source snapshot | Report | Result |
| --- | --- | --- | --- |
| Initial fixture setup | `20261012-115140-504` | `reports/narrow-20261012T045453Z-4164992` | Failed before the tested boundary: public task Create rejects a separate child session. Not bug RED. |
| Actual bug RED, before production fix | `20261012-115635-385` | `reports/narrow-20261012T045639Z-4171093` | Both Stop and Amend fail at Controller with `run transcript was redacted or corrected`, SQLSTATE40001. |
| Initial fix | `20261012-115721-288` | `reports/narrow-20261012T045725Z-4174066` | Three passing test events, zero fail/skip; race and vet pass. |
| Extended fresh continuation setup | `20261012-115900-321` | `reports/narrow-20261012T045904Z-4179928` | Stop passes; fresh Amend fixture incorrectly used the empty-transcript pin path after the control appended an instruction. Not a production failure. |
| Final focused check | `20261012-115936-145` | `reports/narrow-20261012T045940Z-4182112` | Three passing test events, zero fail/skip; race and vet pass, including fresh current-head pin and deleted source denial. |

RED `events.jsonl` SHA256:
`838f2c1b70080ed28423ad83a2423f05675d704fac665ca1d9b5a2735abba64e`.
Final focused `events.jsonl` SHA256:
`a1e60ebbacf378b7f6c4bbaacc6fb455df9cc2852a99517a388574423196040e`.
Final focused empty `vet.log` SHA256:
`e3b0c44298fc1c149afbf4c8996fb92427ae41e4649b934ca495991b7852b855`.

Focused commands inside the isolated lab container:

```sh
go test -json -race -p 1 -count=1 -run '^TestOwnerControlsAfterSourceDeletionIntegration$' ./apps/orchestrator/internal/task
go vet ./apps/orchestrator/internal/task ./internal/memory/catalog
```

The integrated delegation scenario must still rerun its ordinary preflight and
actual provider acceptance. This focused proof does not claim those gates.
