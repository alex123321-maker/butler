# Memory capacity and recovery

The measured capacity profile is deliberately finite. Its sources and full
measurements are in [the capacity report](../testing/issue-91-memory-capacity.md)
and [the prospective protocol](../ai/memory-capacity-protocol.md). The owned
indexer accepts at most10000 current chunks in one generation. Partition larger
history into authorized scopes and query the relevant scope; the100000/64-scope
screen did not search all64 scopes at once. A100000 single-scope exact scan missed
the300 ms target. One million chunks, native cloud response p95 and an unlimited
history lifetime are unqualified.

The [combined collection](../testing/issue-91-combined-capacity.md) qualifies a
separate controlled foreground profile: deployment build without race detection,
client2 CPUs/2 GiB, PostgreSQL2 CPUs/1 GiB with64 MiB shared buffers and pool
maximum2, one10000-chunk user generation, two readers, one100-new-fragment
index job and admitted summary/consolidation workers. Warm/mixed p95 measured
1305.977/1413.040 ms against1500 ms; all requests stayed below5000 ms. The mixed
window completed97/100 requests with three explicit currentness denials during
two real deletions, within its predeclared floor96 and maximumfour typed denials.
Maintenance sources were outside the selected stable generation. Correction or
deletion of that selected generation may make its answers unavailable until a
new current generation is reviewed/activated. Admitted model jobs yield to
interactive priority, remain charged with NULL usage, and recover with a separate
finite reservation; this profile does not promise simultaneous native inference
throughput.1-CPU race and2+1/2+2-CPU race windows missed their targets and remain
unsupported at this SLO. Native cloud latency/quality requires its own evidence.

Record the effective resource settings when reproducing this candidate: the
foreground client has 2 CPUs/2 GiB, PostgreSQL has 2 CPUs/1 GiB, `shared_buffers=64MB`,
`max_connections=32` and `max_wal_size=128MB`. The application pool maximum is 2.
Orchestrator already exposes `BUTLER_POSTGRES_MAX_CONNS`; set it to `2` and keep
`BUTLER_POSTGRES_MIN_CONNS` at most `2` for a staged reproduction. Its default
maximum of 10 is a different pool profile. Capture the effective values rather
than inferring them from flags or a Compose filename. These settings describe
the controlled memory workload; full service/tool/provider latency still needs
its corresponding evidence. This task does not change the primary Compose
resources or restart its database.

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
