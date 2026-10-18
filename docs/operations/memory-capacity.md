# Memory capacity and recovery

The measured capacity profile is deliberately finite. Its sources and full
measurements are in [the capacity report](../testing/issue-91-memory-capacity.md)
and [the prospective protocol](../ai/memory-capacity-protocol.md). The owned
indexer accepts at most10000 current chunks in one generation. Partition larger
history into authorized scopes and query the relevant scope; the100000/64-scope
screen did not search all64 scopes at once. A100000 single-scope exact scan missed
the300 ms target. One million chunks, native cloud response p95 and an unlimited
history lifetime are unqualified.

Keep PostgreSQL, the external deletion manifest and original assets together in
the backup plan. pgvector entries are rebuildable derivatives, whereas accepted
heads, primary transcript/effects, jobs and spend history are durable evidence.
Before backup, save an independently checksummed latest deletion-anchor copy
outside the database backup and record the exact source/image/migration version.
Use a PostgreSQL custom-format dump and independently preserve its asset tree.
Restore into an empty isolated database first. Compare effect and budget digests,
then supply the latest external anchor: an older restored database must deny
reads until Catalog reconciliation finishes, and deleted original/derived copies
must remain erased. Restoring an older manifest to make reads work is forbidden.
The complete actual restore/ENOSPC scenario is described in
[the coherent lifecycle report](../testing/issue-92-coherent-pilot.md); its
current qualification status remains authoritative.

For a rebuild, preserve the active generation and exact embedding Space identity.
Create one owner-authorized index job with finite source list,10000-or-smaller
chunk count, deadline and allowance. Resume only while the existing job and
current source/authority pins remain valid. Exact unchanged embedding hashes can
reuse vectors without another inference; corrections require the current source
snapshot. A done index job leaves a building generation. Review current coverage
and the quality artifact, then use `/api/v4/memory/index-review` with the exact
expected previous pointer. Keep the old still-current ready generation until the
new one is checked. The same activation CAS is the rollback primitive; a corrected
or deleted source can make the old generation ineligible, so rollback does not
override currentness. An incompatible Space change needs a separate staged
generation and current filtered recall comparison before activation.

Retire inactive terminal owner-job generations one at a time through
`/api/v4/memory/index-retire` with `generation_id` and `expected_active_id`.
This removes that generation's vectors and preserves original data, quality,
jobs, reservations and charged scope rows. Retirement is irreversible and grants
no source-erasure authority. Use normal Catalog deletion for privacy, including
the manifest and transitive copied bodies. Do not delete retained job or budget
rows to reclaim space or permit a retry. There is no automatic index-pruning
service. Track original, transcript, vector, WAL, asset and backup bytes separately;
reserve enough for one simultaneous full rebuild and one restore verification.
For the laba experiment, keep at least20 GiB available before starting another
large isolated window. This is an operational reserve, not a measured production
storage guarantee or an authorization to remove unrelated data.

If disk reserve is crossed or storage writes fail, pause new model work through
the authenticated `/api/v4/memory/model-worker-control` state `paused` and pause
the relevant finite policies through `/api/v4/memory/model-policy-control`.
Stop the isolated indexing worker process when its current claim cannot finish
safely; keep its durable job and reservation. Do not repeatedly restart workers
or widen allowances to hide ENOSPC. Measure the actual filesystem/mount first,
restore capacity under operator authorization, and then verify manifest/database
health before resuming still-current policies. Admission, cancellation and
publication may retain NULL usage after any failure. NULL remains unknown and
the original conservative quote stays charged; any admitted retry is a separate
reservation within the original policy/deadline. A pending priority wait is an
observable background delay, not a successful inference or a refunded attempt.

The bounded runner `scripts/dev/memory-capacity-mixed-lab.sh` creates only its own
PostgreSQL/container/network under a fresh agent report directory. It uses cached
Go/modules and the existing private pinned local token, mounts no cloud auth and
downloads nothing. Hardware, limits, request intervals, every latency/error and
complete ledgers survive failure. Reports and private PG data are retained after
only its named containers/network are removed. Do not run a host cache flush,
restart production, change the host clock or prune shared Docker resources to
improve a benchmark. Actual machine-cold measurements require a separate approved
hardware experiment; the first observed request has an explicitly bounded label.
